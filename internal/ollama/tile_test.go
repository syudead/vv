package ollama

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestTileRejectsInputsOverTheLimits(t *testing.T) {
	var tall bytes.Buffer
	if err := png.Encode(&tall, image.NewGray(image.Rect(0, 0, maxCellWidth, 3000))); err != nil {
		t.Fatal(err)
	}
	small := solidPNG(t, color.White)
	repeat := func(b []byte, n int) [][]byte {
		out := make([][]byte, n)
		for i := range out {
			out[i] = b
		}
		return out
	}
	cases := []struct {
		name   string
		images [][]byte
	}{
		{"too many images", repeat(small, maxTileImages+1)},
		{"canvas over the pixel limit", repeat(tall.Bytes(), 9)},
	}
	for _, c := range cases {
		if _, err := Tile(c.images); err == nil {
			t.Errorf("%s: Tile succeeded, want an error", c.name)
		}
	}
}
