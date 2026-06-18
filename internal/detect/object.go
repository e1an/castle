//go:build onnx

package detect

import (
	"image"
	"os"
	"sort"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	inputW = 640
	inputH = 640
)

// ObjectDetector runs YOLOv8n inference via ONNX Runtime.
type objectDetector struct {
	session  *ort.AdvancedSession
	input    *ort.Tensor[float32]
	output   *ort.Tensor[float32]
	minScore float64
}

// NewObjectDetector loads the YOLOv8 ONNX model and initialises a session.
// The ONNX Runtime shared library must be discoverable: set ONNX_RUNTIME_LIB
// to its path, or ensure libonnxruntime.so is on LD_LIBRARY_PATH.
// Call Close() when done.
func NewObjectDetector(modelPath string, minScore float64) (ObjectDetector, error) {
	// onnxruntime_go requires the shared library path before InitializeEnvironment.
	libPath := os.Getenv("ONNX_RUNTIME_LIB")
	if libPath == "" {
		libPath = "/usr/lib/libonnxruntime.so"
	}
	ort.SetSharedLibraryPath(libPath)

	if err := ort.InitializeEnvironment(); err != nil {
		return nil, err
	}

	inputData := make([]float32, 1*3*inputH*inputW)
	input, err := ort.NewTensor(ort.NewShape(1, 3, inputH, inputW), inputData)
	if err != nil {
		return nil, err
	}

	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 84, 8400))
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

	return &objectDetector{
		session:  session,
		input:    input,
		output:   output,
		minScore: minScore,
	}, nil
}

func (d *objectDetector) Close() error {
	d.session.Destroy()
	d.input.Destroy()
	d.output.Destroy()
	return ort.DestroyEnvironment()
}

func (d *objectDetector) Detect(img image.Image) ([]Detection, error) {
	padX, padY := preprocess(img, d.input.GetData())

	if err := d.session.Run(); err != nil {
		return nil, err
	}

	return postprocess(d.output.GetData(), img.Bounds(), padX, padY, d.minScore), nil
}

// preprocess letterboxes img into a 640×640 canvas and writes normalised CHW
// float32 values into dst (R plane first).  Returns the x and y pixel offsets
// of the padded image within the canvas so postprocess can invert them.
func preprocess(img image.Image, dst []float32) (padX, padY int) {
	b := img.Bounds()
	srcW := b.Max.X - b.Min.X
	srcH := b.Max.Y - b.Min.Y

	// Scale to fit inside 640×640 keeping aspect ratio.
	scale := float64(inputW) / float64(srcW)
	if s := float64(inputH) / float64(srcH); s < scale {
		scale = s
	}
	scaledW := int(float64(srcW) * scale)
	scaledH := int(float64(srcH) * scale)
	padX = (inputW - scaledW) / 2
	padY = (inputH - scaledH) / 2

	planeSize := inputW * inputH

	// Fill canvas with grey (114/255 ≈ 0.447 — standard YOLO letterbox colour).
	const grey = float32(114.0 / 255.0)
	for i := range dst {
		dst[i] = grey
	}

	for y := 0; y < scaledH; y++ {
		for x := 0; x < scaledW; x++ {
			sx := b.Min.X + x*srcW/scaledW
			sy := b.Min.Y + y*srcH/scaledH
			r, g, bl, _ := img.At(sx, sy).RGBA()
			// RGBA() returns values in [0, 65535]; normalise to [0, 1].
			idx := (padY+y)*inputW + (padX + x)
			dst[idx] = float32(r) / 65535.0
			dst[planeSize+idx] = float32(g) / 65535.0
			dst[2*planeSize+idx] = float32(bl) / 65535.0
		}
	}

	return padX, padY
}

type candidate struct {
	Detection
	classIdx int
}

// postprocess converts raw YOLOv8 output [1,84,8400] into Detection slices.
// YOLOv8 output layout (per anchor): cx, cy, w, h, class0..class79.
// padX/padY are the letterbox offsets returned by preprocess.
func postprocess(raw []float32, orig image.Rectangle, padX, padY int, minScore float64) []Detection {
	const numAnchors = 8400
	const numClasses = 80

	origW := float32(orig.Max.X - orig.Min.X)
	origH := float32(orig.Max.Y - orig.Min.Y)

	// Scale from padded-canvas coords back to original image coords.
	scale := float64(inputW) / float64(origW)
	if s := float64(inputH) / float64(origH); s < scale {
		scale = s
	}
	invScale := float32(1.0 / scale)

	fpX := float32(padX)
	fpY := float32(padY)

	var candidates []candidate

	for a := 0; a < numAnchors; a++ {
		cx := raw[0*numAnchors+a]
		cy := raw[1*numAnchors+a]
		w := raw[2*numAnchors+a]
		h := raw[3*numAnchors+a]

		bestScore := float32(0)
		bestClass := 0
		for c := 0; c < numClasses; c++ {
			s := raw[(4+c)*numAnchors+a]
			if s > bestScore {
				bestScore = s
				bestClass = c
			}
		}

		if float64(bestScore) < minScore {
			continue
		}

		// Remove letterbox offset then scale to original image size.
		x1 := int((cx - w/2 - fpX) * invScale)
		y1 := int((cy - h/2 - fpY) * invScale)
		x2 := int((cx + w/2 - fpX) * invScale)
		y2 := int((cy + h/2 - fpY) * invScale)

		candidates = append(candidates, candidate{
			Detection: Detection{
				Label: cocoClasses[bestClass],
				Score: float64(bestScore),
				Box:   image.Rect(x1, y1, x2, y2),
			},
			classIdx: bestClass,
		})
	}

	return nms(candidates, 0.45)
}

// nms applies non-maximum suppression per class.
func nms(candidates []candidate, iouThreshold float64) []Detection {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	kept := make([]bool, len(candidates))
	var out []Detection

	for i := range candidates {
		if kept[i] {
			continue
		}
		out = append(out, candidates[i].Detection)
		for j := i + 1; j < len(candidates); j++ {
			if kept[j] || candidates[j].classIdx != candidates[i].classIdx {
				continue
			}
			if iou(candidates[i].Box, candidates[j].Box) > iouThreshold {
				kept[j] = true
			}
		}
	}
	return out
}

func iou(a, b image.Rectangle) float64 {
	inter := a.Intersect(b)
	if inter.Empty() {
		return 0
	}
	areaI := float64(inter.Dx() * inter.Dy())
	areaA := float64(a.Dx() * a.Dy())
	areaB := float64(b.Dx() * b.Dy())
	return areaI / (areaA + areaB - areaI)
}

// cocoClasses are the 80 MS-COCO object labels in YOLOv8 order.
var cocoClasses = [80]string{
	"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train",
	"truck", "boat", "traffic light", "fire hydrant", "stop sign",
	"parking meter", "bench", "bird", "cat", "dog", "horse", "sheep", "cow",
	"elephant", "bear", "zebra", "giraffe", "backpack", "umbrella", "handbag",
	"tie", "suitcase", "frisbee", "skis", "snowboard", "sports ball", "kite",
	"baseball bat", "baseball glove", "skateboard", "surfboard",
	"tennis racket", "bottle", "wine glass", "cup", "fork", "knife", "spoon",
	"bowl", "banana", "apple", "sandwich", "orange", "broccoli", "carrot",
	"hot dog", "pizza", "donut", "cake", "chair", "couch", "potted plant",
	"bed", "dining table", "toilet", "tv", "laptop", "mouse", "remote",
	"keyboard", "cell phone", "microwave", "oven", "toaster", "sink",
	"refrigerator", "book", "clock", "vase", "scissors", "teddy bear",
	"hair drier", "toothbrush",
}
