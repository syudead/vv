package main

import (
	"bufio"
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/faceinfer"
	"github.com/syudead/vv/internal/faceproto"
)

// fakeEngine finds one face in any frame and embeds it as its own x, so the reply shows which face
// was embedded; aligned faces fail.
type fakeEngine struct{}

func (fakeEngine) Detect(faceinfer.Image) ([]faceinfer.Face, error) {
	return []faceinfer.Face{{X: 4, Y: 5, W: 6, H: 7, Score: 0.5, Landmarks: [10]float64{1, 2}}}, nil
}

func (fakeEngine) Embed(_ faceinfer.Image, f faceinfer.Face) ([]float32, error) {
	return []float32{float32(f.X)}, nil
}

func (fakeEngine) EmbedAligned(faceinfer.Image) ([]float32, error) {
	return nil, errors.New("model failed")
}

func TestServeAnswersAfterHelloAndSurvivesAFailedRequest(t *testing.T) {
	var in bytes.Buffer
	for _, m := range []faceproto.Message{
		faceproto.EmbedRequest{Pix: make([]byte, faceproto.AlignedSide*faceproto.AlignedSide*3)},
		faceproto.DetectRequest{Width: 1, Height: 1, Pix: []byte{0, 0, 0}},
	} {
		if err := faceproto.Write(&in, m); err != nil {
			t.Fatal(err)
		}
	}
	hello := faceproto.Hello{Version: faceproto.Version, Detector: detectorID, Recognizer: "sface_int8", Dim: 128}
	var out bytes.Buffer
	if err := serve(&in, bufio.NewWriter(&out), fakeEngine{}, hello); err != nil {
		t.Fatalf("serve: %v", err)
	}
	want := []faceproto.Message{
		hello,
		faceproto.Error{Message: "model failed"},
		faceproto.Detections{Faces: []faceproto.Detection{
			{X: 4, Y: 5, W: 6, H: 7, Score: 0.5, Landmarks: [10]float32{1, 2}, Vector: []float32{4}},
		}},
	}
	for _, w := range want {
		got, err := faceproto.Read(&out)
		if err != nil {
			t.Fatalf("read reply: %v", err)
		}
		if !reflect.DeepEqual(got, w) {
			t.Fatalf("reply %#v, want %#v", got, w)
		}
	}
	if out.Len() != 0 {
		t.Fatalf("%d bytes after the expected replies", out.Len())
	}
}
