package record

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Recorder captures clips by assembling HLS segments already on disk,
// avoiding a second RTSP connection to the camera.
type Recorder struct {
	recDir string
	segDur int // target clip length in seconds
}

func New(recDir string, clipDuration int) *Recorder {
	return &Recorder{recDir: recDir, segDur: clipDuration}
}

// Clip assembles a clip for camID from the live HLS segments on disk.
// It copies the current pre-event segments immediately (before delete_segments
// removes them), then polls for post-event segments, and finally concats all
// into an MP4.  Returns the path relative to recDir.
func (r *Recorder) Clip(ctx context.Context, camID string) (string, error) {
	hlsDir := filepath.Join(r.recDir, camID)
	clipDir := filepath.Join(hlsDir, "clips")
	playlist := filepath.Join(hlsDir, "live.m3u8")

	if err := os.MkdirAll(clipDir, 0o755); err != nil {
		return "", err
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	clipID := fmt.Sprintf("%s_%s", camID, ts)

	// ── pre-event buffer ──────────────────────────────────────────────────
	// HLS segments are 2s each; grab last ~6s worth as context before motion.
	const preEventSegs = 3

	curSegs, err := readM3U8Segments(playlist)
	if err != nil {
		curSegs = nil // playlist may not exist yet — proceed with post-only
	}
	if len(curSegs) > preEventSegs {
		curSegs = curSegs[len(curSegs)-preEventSegs:]
	}

	// Copy pre-event segments immediately so delete_segments can't remove them.
	seen := make(map[string]bool)
	var collected []string
	for _, seg := range curSegs {
		seen[filepath.Base(seg)] = true
		dst, cpErr := copySegment(seg, clipDir, clipID)
		if cpErr == nil {
			collected = append(collected, dst)
		}
	}

	// ── post-event segments ───────────────────────────────────────────────
	// Poll until we have segDur seconds of post-event footage (or ctx cancels).
	deadline := time.Now().Add(time.Duration(r.segDur)*time.Second + 5*time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			goto concat
		case <-ticker.C:
			newSegs, lErr := readM3U8Segments(playlist)
			if lErr != nil {
				continue
			}
			for _, seg := range newSegs {
				base := filepath.Base(seg)
				if seen[base] {
					continue
				}
				seen[base] = true
				dst, cpErr := copySegment(seg, clipDir, clipID)
				if cpErr == nil {
					collected = append(collected, dst)
				}
			}
		}
	}

concat:
	if len(collected) == 0 {
		return "", fmt.Errorf("no segments available for clip")
	}

	outPath, err := concatSegments(ctx, collected, clipDir, clipID)

	// Clean up the temporary segment copies regardless of concat outcome.
	for _, seg := range collected {
		os.Remove(seg)
	}

	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(r.recDir, outPath)
	if err != nil {
		return outPath, nil
	}
	return rel, nil
}

// concatSegments merges .ts files into a single MP4 via FFmpeg's concat demuxer.
func concatSegments(ctx context.Context, segs []string, dir, clipID string) (string, error) {
	listPath := filepath.Join(dir, clipID+".txt")
	var sb strings.Builder
	for _, seg := range segs {
		// Use absolute paths; escape single quotes for the concat format.
		safe := strings.ReplaceAll(seg, "'", "'\\''")
		sb.WriteString(fmt.Sprintf("file '%s'\n", safe))
	}
	if err := os.WriteFile(listPath, []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	defer os.Remove(listPath)

	outPath := filepath.Join(dir, clipID+".mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-loglevel", "error",
		"-f", "concat", "-safe", "0",
		"-i", listPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outPath,
	)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg concat: %w", err)
	}
	return outPath, nil
}

// readM3U8Segments parses a HLS playlist and returns the absolute paths of
// all listed .ts segment files.
func readM3U8Segments(playlist string) ([]string, error) {
	data, err := os.ReadFile(playlist)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(playlist)
	var segs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, ".ts") {
			continue
		}
		if !filepath.IsAbs(line) {
			line = filepath.Join(dir, line)
		}
		segs = append(segs, line)
	}
	return segs, nil
}

// copySegment copies src to dir/prefix_basename and returns the destination path.
func copySegment(src, dir, prefix string) (string, error) {
	dst := filepath.Join(dir, prefix+"_"+filepath.Base(src))
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		os.Remove(dst)
		return "", err
	}
	return dst, nil
}

// PurgeContinuous removes continuous-recording MP4 chunks older than retentionDays.
func (r *Recorder) PurgeContinuous(camID string, retentionDays int) error {
	return purgeDir(filepath.Join(r.recDir, camID, "continuous"), retentionDays)
}

// Purge removes motion-triggered clips older than retentionDays for camID.
func (r *Recorder) Purge(camID string, retentionDays int) error {
	return purgeDir(filepath.Join(r.recDir, camID, "clips"), retentionDays)
}

func purgeDir(dir string, retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}
