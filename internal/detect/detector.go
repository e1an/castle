package detect

import "image"

// Detection is a single object found in a frame.
type Detection struct {
	Label string
	Score float64 // 0.0–1.0 confidence
	Box   image.Rectangle
}

// ObjectDetector runs inference on a frame and returns detected objects above
// a confidence threshold.  The interface lets main.go remain build-tag-agnostic.
type ObjectDetector interface {
	Detect(img image.Image) ([]Detection, error)
	Close() error
}
