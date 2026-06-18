//go:build !onnx

package detect

import "image"

// NewObjectDetector returns nil when built without the onnx tag.
// main.go checks for nil and skips inference.
func NewObjectDetector(modelPath string, minScore float64) (ObjectDetector, error) {
	return nil, nil
}

// stubDetector satisfies the interface but is never instantiated in this build.
type stubDetector struct{}

func (s *stubDetector) Detect(_ image.Image) ([]Detection, error) { return nil, nil }
func (s *stubDetector) Close() error                               { return nil }
