//go:build !onnx

package detect

import "image"

// NewFaceDetector returns nil when built without the onnx tag.
func NewFaceDetector(modelPath string, minScore float64) (ObjectDetector, error) {
	return nil, nil
}

type stubFaceDetector struct{}

func (s *stubFaceDetector) Detect(_ image.Image) ([]Detection, error) { return nil, nil }
func (s *stubFaceDetector) Close() error                               { return nil }
