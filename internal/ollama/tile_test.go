package ollama

import (
	"image/color"
	"testing"
)

func TestTileRejectsMoreImagesThanTheLimit(t *testing.T) {
	small := solidPNG(t, color.White)
	images := make([][]byte, maxTileImages+1)
	for i := range images {
		images[i] = small
	}
	if _, err := Tile(images); err == nil {
		t.Fatal("Tile succeeded, want an error")
	}
}

func TestGridForBoundsTheCanvas(t *testing.T) {
	// Nine 640x100 cells make a 1920x300 canvas: 576,000 pixels.
	g, err := gridFor(9, 1280, 200, 576_000)
	if err != nil {
		t.Fatalf("gridFor at the limit: %v", err)
	}
	if g != (grid{cols: 3, rows: 3, cellW: 640, cellH: 100}) {
		t.Errorf("grid = %+v, want 3x3 cells of 640x100", g)
	}
	if _, err := gridFor(9, 1280, 200, 575_999); err == nil {
		t.Error("gridFor one pixel over the limit succeeded, want an error")
	}
}
