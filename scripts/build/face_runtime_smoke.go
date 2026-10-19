package main

// Run a Linux face runtime archive's helper in the image vv ships, through the bundled loader, on one photo.

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	_ "image/jpeg" // Decode the fixture photo.

	"github.com/syudead/vv/internal/faceproto"
)

// alpineImage is the Dockerfile's runtime base, kept equal to it by hand: the helper must start there without a host glibc.
const alpineImage = "alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6"

// smokePhoto is Eileen Collins' NASA portrait (public domain), as scikit-image ships it in
// skimage/data/astronaut.png, re-encoded as JPEG: one frontal face.
const smokePhoto = "scripts/build/testdata/face.jpg"

type pinnedModel struct{ name, url, sha256 string }

// The smoke test runs the default recognizer, whose vector has 128 values.
var smokeModels = []pinnedModel{
	{"face_detection_yunet_2023mar.onnx",
		"https://media.githubusercontent.com/media/opencv/opencv_zoo/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx",
		"8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4"},
	{"face_recognition_sface_2021dec_int8.onnx",
		"https://huggingface.co/opencv/face_recognition_sface/resolve/main/face_recognition_sface_2021dec_int8.onnx",
		"2b0e941e6f16cc048c20aee0c8e31f569118f65d702914540f7bfdc14048d78a"},
}

const smokeDim = 128

// smokeFaceRuntime unpacks the archive for platform and checks that its helper finds one face with a
// smokeDim-value vector in the fixture photo.
func smokeFaceRuntime(root, platform string) error {
	p, ok := facePlatforms[platform]
	if !ok || !p.linux() {
		return fmt.Errorf("no smoke test for face runtime platform %q", platform)
	}
	unpacked, err := os.MkdirTemp(filepath.Join(root, windowsAppDir), "face-smoke-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(unpacked) }()
	archive := filepath.Join(root, windowsAppDir, faceRuntimeArchive(platform))
	if out, err := exec.Command("tar", "-xzf", archive, "-C", unpacked).CombinedOutput(); err != nil {
		return fmt.Errorf("unpack %s: %w: %s", archive, err, out)
	}
	cache := filepath.Join(root, ffmpegCacheDir)
	client := &http.Client{Timeout: downloadTimeout}
	for _, m := range smokeModels {
		if _, err := fetchPinned(client, m.url, cache, m.name, m.sha256); err != nil {
			return err
		}
	}
	frame, err := readBGR(filepath.Join(root, smokePhoto))
	if err != nil {
		return err
	}
	runtime := filepath.Join(unpacked, faceRuntimeName(platform))
	cmd := exec.Command("docker", "run", "--rm", "-i", "--platform", p.goos+"/"+p.goarch,
		"-v", runtime+":/rt:ro", "-v", cache+":/models:ro", alpineImage,
		"/rt/lib/"+p.loader, "--library-path", "/rt/lib", "/rt/mdm-face",
		"-ort", "/rt/lib/libonnxruntime.so",
		"-detector", "/models/"+smokeModels[0].name,
		"-recognizer", "/models/"+smokeModels[1].name, "-model", "sface_int8")
	cmd.Stderr = os.Stderr
	faces, err := askHelper(cmd, frame)
	if err != nil {
		return err
	}
	if len(faces.Faces) != 1 || len(faces.Faces[0].Vector) != smokeDim {
		return fmt.Errorf("helper found %d faces, want one face with a %d-value vector: %+v", len(faces.Faces), smokeDim, faces)
	}
	fmt.Printf("%s: one face, %d-value vector\n", faceRuntimeArchive(platform), smokeDim)
	return nil
}

// askHelper reads the helper's hello, sends one frame and returns the reply after the helper exits.
func askHelper(cmd *exec.Cmd, frame faceproto.DetectRequest) (faceproto.Detections, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return faceproto.Detections{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return faceproto.Detections{}, err
	}
	if err := cmd.Start(); err != nil {
		return faceproto.Detections{}, err
	}
	reply, askErr := exchange(stdin, bufio.NewReader(stdout), frame)
	_ = stdin.Close()
	_, _ = io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil || askErr != nil {
		return faceproto.Detections{}, errors.Join(askErr, err)
	}
	return reply, nil
}

func exchange(in io.Writer, out io.Reader, frame faceproto.DetectRequest) (faceproto.Detections, error) {
	hello, err := faceproto.Read(out)
	if err != nil {
		return faceproto.Detections{}, fmt.Errorf("hello: %w", err)
	}
	fmt.Printf("helper: %+v\n", hello)
	if err := faceproto.Write(in, frame); err != nil {
		return faceproto.Detections{}, err
	}
	reply, err := faceproto.Read(out)
	if err != nil {
		return faceproto.Detections{}, err
	}
	faces, ok := reply.(faceproto.Detections)
	if !ok {
		return faceproto.Detections{}, fmt.Errorf("helper answered %#v", reply)
	}
	return faces, nil
}

func readBGR(name string) (faceproto.DetectRequest, error) {
	file, err := os.Open(name)
	if err != nil {
		return faceproto.DetectRequest{}, err
	}
	defer func() { _ = file.Close() }()
	img, _, err := image.Decode(file)
	if err != nil {
		return faceproto.DetectRequest{}, fmt.Errorf("%s: %w", name, err)
	}
	b := img.Bounds()
	frame := faceproto.DetectRequest{Width: b.Dx(), Height: b.Dy(), Pix: make([]byte, 0, b.Dx()*b.Dy()*3)}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			frame.Pix = append(frame.Pix, byte(bl>>8), byte(g>>8), byte(r>>8))
		}
	}
	return frame, nil
}
