// facepoc is a proof of concept for naming people in videos by face, run from Go without cgo.
// It loads onnxruntime through purego and runs YuNet (detection) and SFace (embedding).
//
// go run ./scripts/facepoc -ort libonnxruntime.so -models DIR embed a.jpg b.jpg
// go run ./scripts/facepoc -ort libonnxruntime.so -models DIR index -out DIR video.mp4...
//
// DIR holds face_detection_yunet_2023mar.onnx and face_recognition_sface_2021dec[_int8].onnx from
// opencv_zoo. index needs ffmpeg and ffprobe on PATH and writes people.json and people.png to -out.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"

	_ "image/jpeg" // Decode JPEG inputs to embed.
	_ "image/png"  // Decode PNG inputs to embed.
)

type models struct {
	det *yunet
	rec *sface
}

func main() {
	ortPath := flag.String("ort", "libonnxruntime.so", "onnxruntime shared library")
	dir := flag.String("models", "models", "directory with the YuNet and SFace ONNX files")
	sfaceFile := flag.String("sface", "face_recognition_sface_2021dec_int8.onnx", "SFace model file in -models")
	threads := flag.Int("threads", 4, "onnxruntime intra-op threads")
	out := flag.String("out", ".", "index: output directory")
	flag.Parse()
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}
	m, err := loadModels(*ortPath, *dir, *sfaceFile, *threads)
	if err != nil {
		log.Fatal(err)
	}
	switch flag.Arg(0) {
	case "embed":
		err = embedFiles(m, flag.Args()[1:])
	case "index":
		err = indexVideos(m, flag.Args()[1:], *out)
	default:
		err = fmt.Errorf("unknown command %q", flag.Arg(0))
	}
	if err != nil {
		log.Fatal(err)
	}
}

func loadModels(ortPath, dir, sfaceFile string, threads int) (*models, error) {
	r, err := loadORT(ortPath)
	if err != nil {
		return nil, err
	}
	det, err := loadSession(r, filepath.Join(dir, "face_detection_yunet_2023mar.onnx"), threads, "input", yunetOutputs())
	if err != nil {
		return nil, err
	}
	rec, err := loadSession(r, filepath.Join(dir, sfaceFile), threads, "data", []string{"fc1"})
	if err != nil {
		return nil, err
	}
	return &models{det: &yunet{s: det, scoreMin: 0.6, nmsIoU: 0.3}, rec: &sface{s: rec}}, nil
}

func loadSession(r *ortRuntime, path string, threads int, input string, outputs []string) (*ortSession, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := r.newSession(b, threads, input, outputs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// embedFiles prints, per image, the face nearest the centre with its SFace vector as JSON lines.
func embedFiles(m *models, paths []string) error {
	enc := json.NewEncoder(os.Stdout)
	for _, p := range paths {
		img, err := readImage(p)
		if err != nil {
			return err
		}
		faces, err := m.det.detect(img)
		if err != nil {
			return err
		}
		row := map[string]any{"file": p, "faces": len(faces)}
		if len(faces) > 0 {
			f := nearestCentre(img, faces)
			v, err := m.rec.embed(img, f)
			if err != nil {
				return err
			}
			row["box"], row["kps"], row["score"], row["vector"] = []float64{f.x, f.y, f.w, f.h}, f.kps, f.score, v
		}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

func nearestCentre(img bgrImage, faces []face) face {
	cx, cy := float64(img.w)/2, float64(img.h)/2
	best, bestD := faces[0], -1.0
	for _, f := range faces {
		dx, dy := f.x+f.w/2-cx, f.y+f.h/2-cy
		if d := dx*dx + dy*dy; bestD < 0 || d < bestD {
			best, bestD = f, d
		}
	}
	return best
}

func readImage(path string) (bgrImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return bgrImage{}, err
	}
	defer func() { _ = f.Close() }()
	src, _, err := image.Decode(f)
	if err != nil {
		return bgrImage{}, fmt.Errorf("%s: %w", path, err)
	}
	return toBGR(src), nil
}

func toBGR(src image.Image) bgrImage {
	b := src.Bounds()
	img := bgrImage{w: b.Dx(), h: b.Dy(), pix: make([]byte, b.Dx()*b.Dy()*3)}
	for y := range img.h {
		for x := range img.w {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*img.w + x) * 3
			img.pix[i], img.pix[i+1], img.pix[i+2] = byte(bl>>8), byte(g>>8), byte(r>>8)
		}
	}
	return img
}
