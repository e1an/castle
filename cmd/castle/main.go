package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/e1an/castle/config"
	"github.com/e1an/castle/internal/api"
	"github.com/e1an/castle/internal/detect"
	"github.com/e1an/castle/internal/events"
	"github.com/e1an/castle/internal/notify"
	"github.com/e1an/castle/internal/push"
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

	// Generate VAPID key pair once and persist — required for web push.
	if cfg.Notify.VAPIDPublicKey == "" || cfg.Notify.VAPIDPrivateKey == "" {
		priv, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			log.Fatalf("vapid keygen: %v", err)
		}
		cfg.Notify.VAPIDPrivateKey = priv
		cfg.Notify.VAPIDPublicKey = pub
		if err := config.Save(*cfgPath, cfg); err != nil {
			log.Printf("warning: could not persist VAPID keys: %v", err)
		}
		log.Println("generated VAPID key pair")
	}
	pushSender := push.NewSender(cfg.Notify.VAPIDPublicKey, cfg.Notify.VAPIDPrivateKey)

	mgr := stream.NewManager()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Object detector is initialised once — loading the ONNX model is expensive.
	// ONNX Runtime sessions are not goroutine-safe; all inference goes through detMu.
	var (
		objDetector detect.ObjectDetector
		detMu       sync.Mutex
	)
	if cfg.Detect.ModelPath != "" {
		od, odErr := detect.NewObjectDetector(cfg.Detect.ModelPath, cfg.Detect.MinObjectScore)
		if odErr != nil {
			log.Printf("object detector: %v", odErr)
		} else if od != nil {
			objDetector = od
			defer objDetector.Close()
			log.Printf("object detection enabled (model: %s, min score: %.2f)",
				cfg.Detect.ModelPath, cfg.Detect.MinObjectScore)
		}
	}

	var faceDetector detect.ObjectDetector
	if cfg.Detect.FaceModelPath != "" {
		fd, fdErr := detect.NewFaceDetector(cfg.Detect.FaceModelPath, cfg.Detect.MinObjectScore)
		if fdErr != nil {
			log.Printf("face detector: %v", fdErr)
		} else if fd != nil {
			faceDetector = fd
			defer faceDetector.Close()
			log.Printf("face detection enabled (model: %s)", cfg.Detect.FaceModelPath)
		}
	}

	// runDetect serialises ONNX inference and returns all detections above the global threshold.
	runDetect := func(frame image.Image) []detect.Detection {
		if objDetector == nil {
			return nil
		}
		detMu.Lock()
		defer detMu.Unlock()
		dets, err := objDetector.Detect(frame)
		if err != nil {
			return nil
		}
		return dets
	}

	runFaceDetect := func(frame image.Image) []detect.Detection {
		if faceDetector == nil {
			return nil
		}
		detMu.Lock()
		defer detMu.Unlock()
		dets, err := faceDetector.Detect(frame)
		if err != nil {
			return nil
		}
		return dets
	}

	var cfgMu sync.RWMutex

	startCameras := func(ctx context.Context, c *config.Config) {
		notifier := notify.New(c.Notify.WebhookURL, c.Notify.NtfyTopic)
		recorder := record.New(c.Record.Path, c.Record.SegmentDuration)
		detectors := make(map[string]*detect.MotionDetector)

		// perCam tracks per-camera clip state to prevent motion storms.
		type perCam struct {
			mu          sync.Mutex
			active      bool   // a clip goroutine is currently running
			detLabel    string // best object label seen across all frames this clip
			detScore    float64
			lastFiredAt map[string]time.Time // key: label or "motion"; guards notification cooldown
		}
		camState := make(map[string]*perCam)

		for _, cam := range c.Cameras {
			if !cam.Enable {
				continue
			}
			camState[cam.ID] = &perCam{lastFiredAt: make(map[string]time.Time)}
			cam := cam

			// Effective per-camera detect settings (global defaults + per-camera overrides).
			// Object and face detection are enabled by default when their model is loaded;
			// individual cameras can opt out via enable_object_detect / enable_face_detect.
			motionThreshold := c.Detect.MotionThreshold
			enableOD := objDetector != nil
			enableFace := faceDetector != nil
			var camDetect config.CameraDetect
			if cam.Detect != nil {
				camDetect = *cam.Detect
				if camDetect.MotionThreshold != nil {
					motionThreshold = *camDetect.MotionThreshold
				}
				if camDetect.EnableObjectDetect != nil {
					enableOD = *camDetect.EnableObjectDetect && objDetector != nil
				}
				if camDetect.EnableFaceDetect != nil {
					enableFace = *camDetect.EnableFaceDetect && faceDetector != nil
				}
			}
			detectors[cam.ID] = detect.NewMotionDetector(motionThreshold)

			onMotion := func(camID string, frame image.Image) {
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
					if enableOD {
						dets := runDetect(frame)
						label, score := pickBestDetection(dets, camDetect)
						if label != "" {
							pc.mu.Lock()
							if score > pc.detScore {
								pc.detLabel = label
								pc.detScore = score
							}
							pc.mu.Unlock()
						}
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
					var (
						bestFaceBox image.Rectangle
						hasFace     bool
					)
					detDone := make(chan struct{})
					go func() {
						defer close(detDone)
						if enableOD {
							dets := runDetect(frame)
							label, score := pickBestDetection(dets, camDetect)
							if label != "" {
								pc.mu.Lock()
								if score > pc.detScore {
									pc.detLabel = label
									pc.detScore = score
								}
								pc.mu.Unlock()
							}
						}
						if enableFace {
							if faceDets := runFaceDetect(frame); len(faceDets) > 0 {
								bestFaceBox = faceDets[0].Box
								hasFace = true
							}
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

					if sp, err := saveSnapshot(c.Record.Path, camID, evt.OccurredAt, frame); err == nil {
						evt.SnapshotPath = sp
					} else {
						log.Printf("[%s] snapshot: %v", camID, err)
					}
					if hasFace {
						if cp, err := saveCrop(c.Record.Path, camID, evt.OccurredAt, frame, bestFaceBox); err == nil {
							evt.CropPath = cp
						} else {
							log.Printf("[%s] face crop: %v", camID, err)
						}
					}

					if _, err := store.Insert(evt); err != nil {
						log.Printf("[%s] db insert: %v", camID, err)
					}

					cooldownKey := "motion"
					if evt.Label != "" {
						cooldownKey = evt.Label
					}
					cooldown := time.Duration(cam.CooldownSeconds) * time.Second
					shouldNotify := true
					if cooldown > 0 {
						pc.mu.Lock()
						if t, ok := pc.lastFiredAt[cooldownKey]; ok && time.Since(t) < cooldown {
							shouldNotify = false
							log.Printf("[%s] notification suppressed (cooldown %ds, last fired %.0fs ago)",
								camID, cam.CooldownSeconds, time.Since(t).Seconds())
						} else {
							pc.lastFiredAt[cooldownKey] = time.Now()
						}
						pc.mu.Unlock()
					}

					if shouldNotify {
						notifier.Send(notify.Payload{
							CameraID:  camID,
							EventType: string(evt.Type),
							Label:     evt.Label,
							Score:     evt.Score,
							ClipPath:  clipPath,
							Timestamp: evt.OccurredAt,
						})
						subs, _ := store.ListPushSubscriptions()
						if len(subs) > 0 {
							imgURL := ""
							if evt.CropPath != "" {
								imgURL = "/recordings/" + evt.CropPath
							} else if evt.SnapshotPath != "" {
								imgURL = "/recordings/" + evt.SnapshotPath
							}
							pushSender.Send(subs, push.Payload{
								Title:    "Castle — " + cam.Name,
								Body:     pushEventBody(evt),
								CameraID: camID,
								URL:      "/",
								ImageURL: imgURL,
							})
						}
					}
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

func saveSnapshot(recPath, camID string, ts time.Time, img image.Image) (string, error) {
	dir := filepath.Join(recPath, camID, "clips")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := ts.Format("20060102T150405Z") + "_snap.jpg"
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		return "", err
	}
	return camID + "/clips/" + name, nil
}

func saveCrop(recPath, camID string, ts time.Time, img image.Image, box image.Rectangle) (string, error) {
	// Expand the bounding box by 20% on each side so the crop includes
	// forehead, chin, and ears rather than clipping at the raw detection edge.
	padX := box.Dx() / 5
	padY := box.Dy() / 5
	box = image.Rect(
		box.Min.X-padX, box.Min.Y-padY,
		box.Max.X+padX, box.Max.Y+padY,
	).Intersect(img.Bounds())

	cropped := subImage(img, box)

	dir := filepath.Join(recPath, camID, "clips")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := ts.Format("20060102T150405Z") + "_face.jpg"
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := jpeg.Encode(f, cropped, &jpeg.Options{Quality: 90}); err != nil {
		return "", err
	}
	return camID + "/clips/" + name, nil
}

func subImage(img image.Image, box image.Rectangle) image.Image {
	type subImager interface {
		SubImage(image.Rectangle) image.Image
	}
	if si, ok := img.(subImager); ok {
		return si.SubImage(box)
	}
	dst := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			dst.Set(x-box.Min.X, y-box.Min.Y, img.At(x, y))
		}
	}
	return dst
}

func pushEventBody(evt *events.Event) string {
	if evt.Label != "" {
		return fmt.Sprintf("%s detected (%.0f%%)", evt.Label, evt.Score*100)
	}
	return "Motion detected"
}

// pickBestDetection filters detections by the camera's label allow-list and
// per-label thresholds, then returns the highest-scoring allowed detection.
func pickBestDetection(dets []detect.Detection, camDetect config.CameraDetect) (string, float64) {
	var bestLabel string
	var bestScore float64
	for _, d := range dets {
		if len(camDetect.Labels) > 0 {
			lc, ok := camDetect.Labels[d.Label]
			if !ok {
				continue // not in allow-list
			}
			if lc.MinScore > 0 && d.Score < lc.MinScore {
				continue
			}
			if lc.MinArea > 0 && d.Box.Dx()*d.Box.Dy() < lc.MinArea {
				continue
			}
		}
		if camDetect.MinObjectScore != nil && d.Score < *camDetect.MinObjectScore {
			continue
		}
		if d.Score > bestScore {
			bestScore = d.Score
			bestLabel = d.Label
		}
	}
	return bestLabel, bestScore
}
