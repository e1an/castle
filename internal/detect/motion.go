package detect

import (
	"image"
	"sync"
)

// MotionDetector compares successive frames using mean pixel difference.
// Lightweight — no native deps. Runs before object detection to gate expensive inference.
type MotionDetector struct {
	mu        sync.Mutex
	threshold float64 // fraction of changed pixels (0.0–1.0)
	prev      []uint8 // grayscale pixels of last frame
}

func NewMotionDetector(threshold float64) *MotionDetector {
	return &MotionDetector{threshold: threshold}
}

// Detect returns true if significant motion is found relative to the previous frame.
func (d *MotionDetector) Detect(img image.Image) bool {
	gray := toGray(img)

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.prev == nil {
		d.prev = gray
		return false
	}

	changed := 0
	total := len(gray)
	for i, v := range gray {
		diff := int(v) - int(d.prev[i])
		if diff < 0 {
			diff = -diff
		}
		if diff > 20 { // ignore minor lighting noise
			changed++
		}
	}
	d.prev = gray

	return float64(changed)/float64(total) > d.threshold
}

// toGray converts any image to a flat grayscale byte slice (luminance).
func toGray(img image.Image) []uint8 {
	b := img.Bounds()
	w, h := b.Max.X-b.Min.X, b.Max.Y-b.Min.Y
	out := make([]uint8, w*h)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, _ := img.At(x, y).RGBA()
			// ITU-R BT.601 luminance
			lum := (19595*r + 38470*g + 7471*bb + 1<<15) >> 24
			out[(y-b.Min.Y)*w+(x-b.Min.X)] = uint8(lum)
		}
	}
	return out
}
