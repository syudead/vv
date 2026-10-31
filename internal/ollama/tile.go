package ollama

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"

	_ "image/png" // Decode PNG frames, which the frame read answers.
)

// maxImages is the number of images Ollama takes on one request; more are tiled into one grid rather than split,
// because one request answers one set of images.
const maxImages = 8

// Nine cells produce about 1,900 x 1,100 pixels, within the 16-megapixel and 4 MiB limits Workers AI sets.
const maxCellWidth = 640

const tileQuality = 85

// encodeImages encodes images as base64, tiling them into one grid when there are more than limit.
func encodeImages(images [][]byte, limit int) ([]string, error) {
	if len(images) > limit {
		tiled, err := Tile(images)
		if err != nil {
			return nil, err
		}
		images = [][]byte{tiled}
	}
	var encoded []string
	for _, image := range images {
		encoded = append(encoded, base64.StdEncoding.EncodeToString(image))
	}
	return encoded, nil
}

// Tile draws the images in order into a near-square grid of cells sized from the first image, at most maxCellWidth
// wide, and encodes it as a JPEG; an image of another aspect ratio is fitted with black padding.
func Tile(images [][]byte) ([]byte, error) {
	decoded := make([]image.Image, len(images))
	for i, b := range images {
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("cannot decode image %d: %w", i, err)
		}
		decoded[i] = img
	}
	first := decoded[0].Bounds()
	cellW, cellH := first.Dx(), first.Dy()
	if cellW > maxCellWidth {
		cellH = cellH * maxCellWidth / cellW
		cellW = maxCellWidth
	}
	cols := int(math.Ceil(math.Sqrt(float64(len(decoded)))))
	rows := (len(decoded) + cols - 1) / cols
	out := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))
	for i, img := range decoded {
		cell := image.Rect(0, 0, cellW, cellH).Add(image.Pt((i%cols)*cellW, (i/cols)*cellH))
		drawFit(out, cell, img)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: tileQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawFit preserves aspect ratio and averages source pixels when downsampling.
func drawFit(dst *image.RGBA, cell image.Rectangle, src image.Image) {
	sb := src.Bounds()
	scale := math.Min(float64(cell.Dx())/float64(sb.Dx()), float64(cell.Dy())/float64(sb.Dy()))
	w := max(1, int(float64(sb.Dx())*scale))
	h := max(1, int(float64(sb.Dy())*scale))
	origin := cell.Min.Add(image.Pt((cell.Dx()-w)/2, (cell.Dy()-h)/2))
	for y := range h {
		y0 := sb.Min.Y + y*sb.Dy()/h
		y1 := max(y0+1, sb.Min.Y+(y+1)*sb.Dy()/h)
		for x := range w {
			x0 := sb.Min.X + x*sb.Dx()/w
			x1 := max(x0+1, sb.Min.X+(x+1)*sb.Dx()/w)
			var r, g, b, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, _ := src.At(sx, sy).RGBA()
					r, g, b, n = r+uint64(cr), g+uint64(cg), b+uint64(cb), n+1
				}
			}
			dst.SetRGBA(origin.X+x, origin.Y+y, color.RGBA{
				R: uint8(r / n >> 8), G: uint8(g / n >> 8), B: uint8(b / n >> 8), A: 0xff,
			})
		}
	}
}
