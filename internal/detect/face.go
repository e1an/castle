//go:build onnx

package detect

import (
	"image"

	ort "github.com/yalue/onnxruntime_go"
)

// faceDetector wraps a YOLOv8-face ONNX model.
// Expected output tensor: [1, 20, 8400]
// Layout per anchor: cx, cy, w, h, conf, [15 keypoint values — ignored].
type faceDetector struct {
	session  *ort.AdvancedSession
	input    *ort.Tensor[float32]
	output   *ort.Tensor[float32]
	minScore float64
}

// NewFaceDetector loads a YOLOv8-face ONNX model. The ONNX Runtime
// environment must already be initialised by NewObjectDetector.
func NewFaceDetector(modelPath string, minScore float64) (ObjectDetector, error) {
	inputData := make([]float32, 1*3*inputH*inputW)
	input, err := ort.NewTensor(ort.NewShape(1, 3, inputH, inputW), inputData)
	if err != nil {
		return nil, err
	}

	// 4 box coords + 1 conf + 5 landmarks × 3 = 20 features per anchor.
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 20, 8400))
	if err != nil {
		input.Destroy()
		return nil, err
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"images"},
		[]string{"output0"},
		[]ort.Value{input},
		[]ort.Value{output},
		nil,
	)
	if err != nil {
		input.Destroy()
		output.Destroy()
		return nil, err
	}

	return &faceDetector{
		session:  session,
		input:    input,
		output:   output,
		minScore: minScore,
	}, nil
}

func (d *faceDetector) Close() error {
	d.session.Destroy()
	d.input.Destroy()
	d.output.Destroy()
	return nil
}

func (d *faceDetector) Detect(img image.Image) ([]Detection, error) {
	padX, padY := preprocess(img, d.input.GetData())
	if err := d.session.Run(); err != nil {
		return nil, err
	}
	return facePostprocess(d.output.GetData(), img.Bounds(), padX, padY, d.minScore), nil
}

func facePostprocess(raw []float32, orig image.Rectangle, padX, padY int, minScore float64) []Detection {
	const numAnchors = 8400

	origW := float32(orig.Max.X - orig.Min.X)
	origH := float32(orig.Max.Y - orig.Min.Y)

	scale := float64(inputW) / float64(origW)
	if s := float64(inputH) / float64(origH); s < scale {
		scale = s
	}
	invScale := float32(1.0 / scale)
	fpX := float32(padX)
	fpY := float32(padY)

	var candidates []candidate

	for a := 0; a < numAnchors; a++ {
		conf := float64(raw[4*numAnchors+a])
		if conf < minScore {
			continue
		}
		cx := raw[0*numAnchors+a]
		cy := raw[1*numAnchors+a]
		w := raw[2*numAnchors+a]
		h := raw[3*numAnchors+a]

		x1 := int((cx - w/2 - fpX) * invScale)
		y1 := int((cy - h/2 - fpY) * invScale)
		x2 := int((cx + w/2 - fpX) * invScale)
		y2 := int((cy + h/2 - fpY) * invScale)

		candidates = append(candidates, candidate{
			Detection: Detection{
				Label: "face",
				Score: conf,
				Box:   image.Rect(x1, y1, x2, y2),
			},
			classIdx: 0,
		})
	}

	return nms(candidates, 0.45)
}
