package main

import (
	"fmt"
	"math"
	"sort"
)

// yunetSize is the fixed input side of face_detection_yunet_2023mar.onnx.
const yunetSize = 640

var yunetStrides = []int{8, 16, 32}

// bgrImage is packed 8-bit BGR, as ffmpeg's bgr24 and OpenCV write it.
type bgrImage struct {
	w, h int
	pix  []byte
}

// face is one detection in image pixels: box, five landmarks (eyes, nose, mouth corners;
// image-left first) and score.
type face struct {
	x, y, w, h float64
	kps        [10]float64
	score      float64
}

type yunet struct {
	s        *ortSession
	scoreMin float64
	nmsIoU   float64
}

func yunetOutputs() []string {
	var names []string
	for _, kind := range []string{"cls", "obj", "bbox", "kps"} {
		for _, s := range yunetStrides {
			names = append(names, fmt.Sprintf("%s_%d", kind, s))
		}
	}
	return names
}

func (y *yunet) detect(img bgrImage) ([]face, error) {
	// Only shrink: upscaling a small image moves the landmarks by several pixels against OpenCV's padding.
	scale := math.Min(1, math.Min(float64(yunetSize)/float64(img.w), float64(yunetSize)/float64(img.h)))
	out, err := y.s.run(letterbox(img, scale), []int64{1, 3, yunetSize, yunetSize})
	if err != nil {
		return nil, err
	}
	faces := nms(decodeYuNet(out, y.scoreMin), y.nmsIoU)
	for i := range faces {
		faces[i].x, faces[i].y, faces[i].w, faces[i].h = faces[i].x/scale, faces[i].y/scale, faces[i].w/scale, faces[i].h/scale
		for k := range faces[i].kps {
			faces[i].kps[k] /= scale
		}
	}
	return faces, nil
}

// letterbox scales img into the top-left of a black yunetSize square as planar BGR float32 in 0..255.
func letterbox(img bgrImage, scale float64) []float32 {
	plane := yunetSize * yunetSize
	out := make([]float32, 3*plane)
	w := min(yunetSize, int(math.Round(float64(img.w)*scale)))
	h := min(yunetSize, int(math.Round(float64(img.h)*scale)))
	for y := range h {
		for x := range w {
			b, g, r := sampleBGR(img, (float64(x)+0.5)/scale-0.5, (float64(y)+0.5)/scale-0.5)
			i := y*yunetSize + x
			out[i], out[plane+i], out[2*plane+i] = b, g, r
		}
	}
	return out
}

// sampleBGR reads img bilinearly at (x, y); points outside the image read as black.
func sampleBGR(img bgrImage, x, y float64) (b, g, r float32) {
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := float32(x-float64(x0)), float32(y-float64(y0))
	var c [3]float32
	for _, p := range [4]struct {
		dx, dy int
		w      float32
	}{{0, 0, (1 - fx) * (1 - fy)}, {1, 0, fx * (1 - fy)}, {0, 1, (1 - fx) * fy}, {1, 1, fx * fy}} {
		px, py := x0+p.dx, y0+p.dy
		if p.w == 0 || px < 0 || py < 0 || px >= img.w || py >= img.h {
			continue
		}
		i := (py*img.w + px) * 3
		for k := range c {
			c[k] += p.w * float32(img.pix[i+k])
		}
	}
	return c[0], c[1], c[2]
}

// decodeYuNet turns the anchor-free outputs (ordered as yunetOutputs) into faces in letterbox pixels,
// following OpenCV's FaceDetectorYN post-processing.
func decodeYuNet(out []tensor, scoreMin float64) []face {
	n := len(yunetStrides)
	var faces []face
	for si, stride := range yunetStrides {
		cls, obj, box, kps := out[si].data, out[n+si].data, out[2*n+si].data, out[3*n+si].data
		cols := yunetSize / stride
		for idx := range cls {
			score := math.Sqrt(clamp01(float64(cls[idx])) * clamp01(float64(obj[idx])))
			if score < scoreMin {
				continue
			}
			faces = append(faces, anchorFace(idx%cols, idx/cols, float64(stride), box[idx*4:idx*4+4], kps[idx*10:idx*10+10], score))
		}
	}
	return faces
}

func anchorFace(col, row int, stride float64, box, kps []float32, score float64) face {
	cx := (float64(col) + float64(box[0])) * stride
	cy := (float64(row) + float64(box[1])) * stride
	w := math.Exp(float64(box[2])) * stride
	h := math.Exp(float64(box[3])) * stride
	f := face{x: cx - w/2, y: cy - h/2, w: w, h: h, score: score}
	for k := 0; k < 10; k += 2 {
		f.kps[k] = (float64(kps[k]) + float64(col)) * stride
		f.kps[k+1] = (float64(kps[k+1]) + float64(row)) * stride
	}
	return f
}

func clamp01(v float64) float64 {
	return math.Min(1, math.Max(0, v))
}

// nms keeps the highest-scoring faces and drops any whose box overlaps a kept one above maxIoU.
func nms(faces []face, maxIoU float64) []face {
	sort.Slice(faces, func(i, j int) bool { return faces[i].score > faces[j].score })
	var kept []face
	for _, f := range faces {
		ok := true
		for _, k := range kept {
			if boxIoU(f, k) > maxIoU {
				ok = false
				break
			}
		}
		if ok {
			kept = append(kept, f)
		}
	}
	return kept
}

func boxIoU(a, b face) float64 {
	iw := math.Min(a.x+a.w, b.x+b.w) - math.Max(a.x, b.x)
	ih := math.Min(a.y+a.h, b.y+b.h) - math.Max(a.y, b.y)
	if iw <= 0 || ih <= 0 {
		return 0
	}
	inter := iw * ih
	return inter / (a.w*a.h + b.w*b.h - inter)
}
