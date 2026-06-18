package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/e1an/castle/config"
	"github.com/e1an/castle/internal/api"
	"github.com/e1an/castle/internal/detect"
	"github.com/e1an/castle/internal/events"
	"github.com/e1an/castle/internal/notify"
	"github.com/e1an/castle/internal/record"
	"github.com/e1an/castle/internal/stream"
)

func main() {
	cfgPath := flag.String("config", "castle.yaml", "path to config file")
	healthcheck := flag.Bool("healthcheck", false, "hit /health and exit (for Docker healthcheck)")
	flag.Parse()

	if *healthcheck {
		resp, err := http.Get("http://localhost:8080/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := os.MkdirAll(cfg.Record.Path, 0o755); err != nil {
		log.Fatalf("recordings dir: %v", err)
	}

	store, err := events.Open(cfg.Record.Path + "/castle.db")
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer store.Close()

	mgr := stream.NewManager()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Object detector is initialised once — loading the ONNX model is expensive.
	// ONNX Runtime sessions are not goroutine-safe; all inference goes through detMu.
	var (
		objDetector detect.ObjectDetector
		detMu       sync.Mutex
	)
	if cfg.Detect.EnableObjectDetect && cfg.Detect.ModelPath != "" {
		od, odErr := detect.NewObjectDetector(cfg.Detect.ModelPath, cfg.Detect.MinObjectScore)
		if odErr != nil {
			log.Fatalf("object detector: %v", odErr)
		}
		if od != nil {
			objDetector = od
			defer objDetector.Close()
			log.Printf("object detection enabled (model: %s, min score: %.2f)",
				cfg.Detect.ModelPath, cfg.Detect.MinObjectScore)
		}
	}

	// runDetect serialises ONNX inference and returns the best detection or "","0".
	runDetect := func(frame image.Image) (label string, score float64) {
		if objDetector == nil {
			return "", 0
		}
		detMu.Lock()
		defer detMu.Unlock()
		dets, err := objDetector.Detect(frame)
		if err != nil || len(dets) == 0 {
			return "", 0
		}
		return dets[0].Label, dets[0].Score
	}

	var cfgMu sync.RWMutex

	startCameras := func(ctx context.Context, c *config.Config) {
		notifier := notify.New(c.Notify.WebhookURL, c.Notify.NtfyTopic)
		recorder := record.New(c.Record.Path, c.Record.SegmentDuration)
		detectors := make(map[string]*detect.MotionDetector)

		// perCam tracks per-camera clip state to prevent motion storms.
		type perCam struct {
			mu       sync.Mutex
			active   bool   // a clip goroutine is currently running
			detLabel string // best object label seen across all frames this clip
			detScore float64
		}
		camState := make(map[string]*perCam)

		for _, cam := range c.Cameras {
			if !cam.Enable {
				continue
			}
			detectors[cam.ID] = detect.NewMotionDetector(c.Detect.MotionThreshold)
			camState[cam.ID] = &perCam{}
			cam := cam

			onMotion := func(camID string, frame image.Image) {
				cfgMu.RLock()
				_ = c.Detect.MotionThreshold // read under lock if needed in future
				cfgMu.RUnlock()

				if !detectors[camID].Detect(frame) {
					return
				}

				pc := camState[camID]
				pc.mu.Lock()

				if pc.active {
					// Clip already in progress — still run detection on this frame
					// so we pick the best label across the whole event, not just
					// the (often blurry) trigger frame.
					pc.mu.Unlock()
					label, score := runDetect(frame)
					if label != "" {
						pc.mu.Lock()
						if score > pc.detScore {
							pc.detLabel = label
							pc.detScore = score
						}
						pc.mu.Unlock()
					}
					return
				}

				// No clip in progress — start one.
				pc.active = true
				pc.detLabel = ""
				pc.detScore = 0
				pc.mu.Unlock()

				go func() {
					defer func() {
						pc.mu.Lock()
						pc.active = false
						pc.mu.Unlock()
					}()

					log.Printf("[%s] motion detected — recording clip", camID)

					// Run detection on the trigger frame concurrently while the
					// clip records, so we don't delay the clip start.
					detDone := make(chan struct{})
					go func() {
						defer close(detDone)
						label, score := runDetect(frame)
						if label != "" {
							pc.mu.Lock()
							if score > pc.detScore {
								pc.detLabel = label
								pc.detScore = score
							}
							pc.mu.Unlock()
						}
					}()

					clipPath, clipErr := recorder.Clip(ctx, camID)
					<-detDone // ensure trigger-frame detection has finished

					if clipErr != nil {
						log.Printf("[%s] clip error: %v", camID, clipErr)
						return
					}

					pc.mu.Lock()
					label := pc.detLabel
					score := pc.detScore
					pc.mu.Unlock()

					evt := &events.Event{
						CameraID:   camID,
						Type:       events.EventMotion,
						ClipPath:   clipPath,
						OccurredAt: time.Now().UTC(),
					}
					if label != "" {
						evt.Type = events.EventObject
						evt.Label = label
						evt.Score = score
						log.Printf("[%s] best detection: %s (%.0f%%)", camID, label, score*100)
					}

					if _, err := store.Insert(evt); err != nil {
						log.Printf("[%s] db insert: %v", camID, err)
					}
					notifier.Send(notify.Payload{
						CameraID:  camID,
						EventType: string(evt.Type),
						Label:     evt.Label,
						Score:     evt.Score,
						ClipPath:  clipPath,
						Timestamp: evt.OccurredAt,
					})
				}()
			}

			if err := mgr.Start(ctx, cam, c.Record.Path, c.Record.SegmentDuration,
				c.Record.ContinuousMode, onMotion); err != nil {
				log.Printf("stream %s: %v", cam.ID, err)
				continue
			}
			mode := "motion-triggered"
			if c.Record.ContinuousMode {
				mode = "continuous"
			}
			log.Printf("stream started: %s (%s) [%s]", cam.Name, cam.ID, mode)
		}
	}

	startCameras(ctx, cfg)

	// Reload stops all streams and restarts with the new config.
	reloadFn := func(newCfg *config.Config) error {
		cfgMu.Lock()
		defer cfgMu.Unlock()
		log.Println("reloading config — restarting streams")
		mgr.StopAll()
		startCameras(ctx, newCfg)
		cfg = newCfg
		return nil
	}

	// Daily retention purge — covers both motion clips and continuous chunks.
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cfgMu.RLock()
				cameras := cfg.Cameras
				recPath := cfg.Record.Path
				retDays := cfg.Record.RetentionDays
				continuous := cfg.Record.ContinuousMode
				cfgMu.RUnlock()

				r := record.New(recPath, 10)
				for _, cam := range cameras {
					_ = r.Purge(cam.ID, retDays)
					if continuous {
						_ = r.PurgeContinuous(cam.ID, retDays)
					}
				}
			}
		}
	}()

	apiSrv := api.New(store, cfg.Record.Path, *cfgPath, cfg, reloadFn)
	if u := os.Getenv("CASTLE_USER"); u != "" {
		apiSrv.WithAuth(u, os.Getenv("CASTLE_PASS"))
		log.Printf("basic auth enabled for user %q", u)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: apiSrv,
	}

	go func() {
		log.Printf("API listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down…")
	mgr.StopAll()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
}

func loadConfig(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.Default(), nil
		}
		return nil, err
	}
	return cfg, nil
}
