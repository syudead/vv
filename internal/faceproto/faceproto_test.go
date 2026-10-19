package faceproto

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestEveryMessageRoundTrips(t *testing.T) {
	aligned := bytes.Repeat([]byte{1, 2, 3}, AlignedSide*AlignedSide)
	messages := []Message{
		Hello{Version: Version, Detector: "yunet_2023mar", Recognizer: "sface_int8", Dim: 128},
		DetectRequest{Width: 2, Height: 1, Pix: []byte{10, 20, 30, 40, 50, 60}},
		EmbedRequest{Pix: aligned},
		Detections{Faces: []Detection{
			{X: 1, Y: 2, W: 3, H: 4, Landmarks: [10]float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, Score: 0.9, Vector: []float32{0.6, -0.8}},
			{X: -1.5, Y: 0, W: 64, H: 80, Score: 0.7, Vector: []float32{1}},
		}},
		Vector{Values: []float32{0.25, -0.5, 0.75}},
		Error{Message: "no model"},
	}
	var stream bytes.Buffer
	for _, m := range messages {
		if err := Write(&stream, m); err != nil {
			t.Fatalf("write %T: %v", m, err)
		}
	}
	for _, want := range messages {
		got, err := Read(&stream)
		if err != nil {
			t.Fatalf("read %T: %v", want, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("read %#v, want %#v", got, want)
		}
	}
	if _, err := Read(&stream); !errors.Is(err, io.EOF) {
		t.Fatalf("read after the last message: %v, want io.EOF", err)
	}
}

func TestReadRejectsHelloOfAnotherVersion(t *testing.T) {
	var stream bytes.Buffer
	if err := Write(&stream, Hello{Version: Version + 1, Detector: "d", Recognizer: "r", Dim: 128}); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(&stream); !errors.Is(err, ErrVersion) {
		t.Fatalf("read: %v, want ErrVersion", err)
	}
}

func TestReadRejectsFrameShorterThanItsSize(t *testing.T) {
	var stream bytes.Buffer
	// 2x2 BGR needs 12 bytes.
	if err := Write(&stream, DetectRequest{Width: 2, Height: 2, Pix: make([]byte, 6)}); err != nil {
		t.Fatal(err)
	}
	if m, err := Read(&stream); err == nil {
		t.Fatalf("read %#v from a short frame", m)
	}
}
