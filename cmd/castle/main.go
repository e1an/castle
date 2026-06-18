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
	var objDetector detect.ObjectDetector
	if cfg.Detect.EnableObjectDetect && cfg.Detect.ModelPath != "" {
		od, odErr := detect.NewObjectDetector(cfg.Detect.ModelPath, cfg.Detect.MinObjectScore)
		if odErr != nil {
			log.Fatalf("object detector: %v", odErr)
		}
		if od != nil {
			objDetector = od
			defer objDetector.Close()
			log.Printf("object detection enabled (model: %s, threshold: %.2f)", cfg.Detect.ModelPath, cfg.Detect.MinObjectScore)
		}
	}

	var cfgMu sync.RWMutex

	startCameras := func(ctx context.Context, c *config.Config) {
		notifier := notify.New(c.Notify.WebhookURL, c.Notify.NtfyTopic)
		recorder := record.New(c.Record.Path, c.Record.SegmentDuration)
		detectors := make(map[string]*detect.MotionDetector)

		for _, cam := range c.Cameras {
			if !cam.Enable {
				continue
			}
			detectors[cam.ID] = detect.NewMotionDetector(c.Detect.MotionThreshold)
			cam := cam

			onMotion := func(camID string, frame image.Image) {
				cfgMu.RLock()
				motionThreshold := c.Detect.MotionThreshold
				cfgMu.RUnlock()
				_ = motionThreshold

				d := detectors[camID]
				if !d.Detect(frame) {
					return
				}
				log.Printf("[%s] motion detected — recording clip", camID)

				clipPath, err := recorder.Clip(ctx, camID)
				if err != nil {
					log.Printf("[%s] clip error: %v", camID, err)
					return
				}

				evt := &events.Event{
					CameraID:   camID,
					Type:       events.EventMotion,
					ClipPath:   clipPath,
					OccurredAt: time.Now().UTC(),
				}

				if objDetector != nil {
					detections, err := objDetector.Detect(frame)
					if err != nil {
						log.Printf("[%s] object detection error: %v", camID, err)
					} else if len(detections) > 0 {
						best := detections[0]
						evt.Type = events.EventObject
						evt.Label = best.Label
						evt.Score = best.Score
						log.Printf("[%s] object detected: %s (%.0f%%)", camID, best.Label, best.Score*100)
					}
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
			}

			if err := mgr.Start(ctx, cam, c.Record.Path, c.Record.SegmentDuration, onMotion); err != nil {
				log.Printf("stream %s: %v", cam.ID, err)
				continue
			}
			log.Printf("stream started: %s (%s)", cam.Name, cam.ID)
		}
	}

	startCameras(ctx, cfg)

	// Reload stops all streams and restarts them with the new config.
	reloadFn := func(newCfg *config.Config) error {
		cfgMu.Lock()
		defer cfgMu.Unlock()
		log.Println("reloading config — restarting streams")
		mgr.StopAll()
		startCameras(ctx, newCfg)
		cfg = newCfg
		return nil
	}

	// Daily retention purge.
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
				cfgMu.RUnlock()
				r := record.New(recPath, 10)
				for _, cam := range cameras {
					_ = r.Purge(cam.ID, retDays)
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
