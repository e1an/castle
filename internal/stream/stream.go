package stream

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/e1an/castle/config"
)

// Manager owns all camera streams.
type Manager struct {
	mu      sync.RWMutex
	streams map[string]*Stream
}

func NewManager() *Manager {
	return &Manager{streams: make(map[string]*Stream)}
}

func (m *Manager) Start(ctx context.Context, cam config.Camera, recDir string, segDur int, continuous bool, onMotion func(camID string, frame image.Image)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.streams[cam.ID]; ok {
		return fmt.Errorf("stream %s already running", cam.ID)
	}

	s := newStream(cam, recDir, segDur, continuous, onMotion)
	m.streams[cam.ID] = s
	go s.run(ctx)
	return nil
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.streams[id]; ok {
		s.cancel()
		delete(m.streams, id)
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.streams {
		s.cancel()
		delete(m.streams, id)
	}
}

// Stream manages a single camera feed via FFmpeg.
type Stream struct {
	cam        config.Camera
	recDir     string
	segDur     int
	continuous bool // whether to run a separate continuous-recording process
	onMotion   func(camID string, frame image.Image)
	cancel     context.CancelFunc
}

func newStream(cam config.Camera, recDir string, segDur int, continuous bool, onMotion func(string, image.Image)) *Stream {
	return &Stream{cam: cam, recDir: recDir, segDur: segDur, continuous: continuous, onMotion: onMotion}
}

// run starts FFmpeg subprocesses for this camera:
//  1. HLS segmenter — rolling live buffer for the UI and motion-triggered clips
//  2. Frame pipe — raw frames for motion/object detection
//  3. Continuous recorder — 5-minute MP4 chunks (only when continuous_mode: true)
func (s *Stream) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.runHLS(ctx) }()
	go func() { defer wg.Done(); s.runFramePipe(ctx) }()
	if s.continuous {
		wg.Add(1)
		go func() { defer wg.Done(); s.runContinuous(ctx) }()
	}
	wg.Wait()
}

// runHLS records the stream to a rolling HLS buffer used by the live view
// and as the source for motion-triggered clip assembly.
func (s *Stream) runHLS(ctx context.Context) {
	outDir := filepath.Join(s.recDir, s.cam.ID)
	playlist := filepath.Join(outDir, "live.m3u8")

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Printf("[%s] hls mkdir: %v", s.cam.ID, err)
		return
	}

	args := []string{
		"-loglevel", "warning",
		"-rtsp_transport", "tcp",
		"-i", s.cam.URL,
		"-c:v", "copy",
		"-an",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "6",
		"-hls_flags", "delete_segments+append_list",
		"-hls_segment_filename", filepath.Join(outDir, "seg%05d.ts"),
		playlist,
	}

	for {
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			log.Printf("[%s] hls ffmpeg exited: %v — restarting in 3s", s.cam.ID, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// runContinuous writes back-to-back 5-minute MP4 chunks into recDir/<id>/continuous/.
// These are kept separately from motion clips and are purged on retention schedule.
// NOTE: this opens a second RTSP connection to the camera.
func (s *Stream) runContinuous(ctx context.Context) {
	dir := filepath.Join(s.recDir, s.cam.ID, "continuous")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[%s] continuous mkdir: %v", s.cam.ID, err)
		return
	}

	args := []string{
		"-loglevel", "warning",
		"-rtsp_transport", "tcp",
		"-i", s.cam.URL,
		"-c:v", "copy",
		"-an",
		"-f", "segment",
		"-segment_time", "300", // 5-minute chunks
		"-segment_format", "mp4",
		"-strftime", "1",
		"-reset_timestamps", "1",
		filepath.Join(dir, "%Y%m%dT%H%M%SZ.mp4"),
	}

	for {
		if ctx.Err() != nil {
			return
		}
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			log.Printf("[%s] continuous ffmpeg exited: %v — restarting in 5s", s.cam.ID, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// runFramePipe pipes raw RGB frames from FFmpeg for motion/object analysis.
func (s *Stream) runFramePipe(ctx context.Context) {
	const (
		width  = 640
		height = 360
		stride = width * 3
	)

	args := []string{
		"-loglevel", "warning",
		"-rtsp_transport", "tcp",
		"-i", s.cam.URL,
		"-vf", "fps=2,scale=640:360",
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"pipe:1",
	}

	buf := make([]byte, stride*height)

	for {
		if ctx.Err() != nil {
			return
		}

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return
		}
		if err := cmd.Start(); err != nil {
			log.Printf("[%s] frame pipe start: %v — restarting in 3s", s.cam.ID, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}

		for {
			if _, err := readFull(stdout, buf); err != nil {
				break
			}
			frame := bufToImage(buf, width, height)
			if s.onMotion != nil {
				s.onMotion(s.cam.ID, frame)
			}
		}

		cmd.Wait()
		if ctx.Err() != nil {
			return
		}
		log.Printf("[%s] frame pipe exited — restarting in 3s", s.cam.ID)
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// bufToImage converts raw RGB24 bytes to an image.NRGBA.
func bufToImage(buf []byte, w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := (y*w + x) * 3
			img.SetNRGBA(x, y, color.NRGBA{R: buf[idx], G: buf[idx+1], B: buf[idx+2], A: 255})
		}
	}
	return img
}

// readFull reads exactly len(buf) bytes from r.
func readFull(r interface{ Read([]byte) (int, error) }, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
