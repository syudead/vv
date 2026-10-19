package faceinfer

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestSimilarityRecoversRotationScaleAndShift(t *testing.T) {
	// dst = 2 * R(30°) * src + (5, -3)
	cs, sn := 2*math.Cos(math.Pi/6), 2*math.Sin(math.Pi/6)
	src := [10]float64{0, 0, 10, 0, 5, 5, 2, 9, 8, 9}
	var dst [10]float64
	for i := 0; i < 10; i += 2 {
		dst[i] = cs*src[i] - sn*src[i+1] + 5
		dst[i+1] = sn*src[i] + cs*src[i+1] - 3
	}
	m := similarity(src, dst)
	for i := 0; i < 10; i += 2 {
		x, y := m.apply(src[i], src[i+1])
		near(t, "x", x, dst[i])
		near(t, "y", y, dst[i+1])
		bx, by := m.inverse().apply(x, y)
		near(t, "inverse x", bx, src[i])
		near(t, "inverse y", by, src[i+1])
	}
}

// yunetOutput builds the twelve outputs with one anchor of stride 16 set at (col 3, row 2).
func yunetOutput(cls, obj float32) []tensor {
	out := make([]tensor, 12)
	for si, s := range yunetStrides {
		cells := (yunetSize / s) * (yunetSize / s)
		out[si] = tensor{data: make([]float32, cells)}
		out[3+si] = tensor{data: make([]float32, cells)}
		out[6+si] = tensor{data: make([]float32, cells*4)}
		out[9+si] = tensor{data: make([]float32, cells*10)}
	}
	idx := 2*(yunetSize/16) + 3
	out[1].data[idx], out[4].data[idx] = cls, obj
	copy(out[7].data[idx*4:], []float32{0.5, 0.25, float32(math.Log(4)), float32(math.Log(2))})
	copy(out[10].data[idx*10:], []float32{0, 0, 1, 0, 0.5, 0.5, 0, 1, 1, 1})
	return out
}

func TestDecodeYuNetPlacesAnchorFace(t *testing.T) {
	faces := decodeYuNet(yunetOutput(0.9, 0.4), 0.5)
	if len(faces) != 1 {
		t.Fatalf("got %d faces, want 1", len(faces))
	}
	f := faces[0]
	near(t, "score", f.Score, 0.6) // sqrt(0.9 * 0.4)
	// centre (3.5, 2.25) * 16 = (56, 36); size (4, 2) * 16 = (64, 32)
	near(t, "x", f.X, 24)
	near(t, "y", f.Y, 20)
	near(t, "w", f.W, 64)
	near(t, "h", f.H, 32)
	want := [10]float64{48, 32, 64, 32, 56, 40, 48, 48, 64, 48}
	for k := range want {
		near(t, "landmarks", f.Landmarks[k], want[k])
	}
}

func TestDecodeYuNetDropsLowScores(t *testing.T) {
	if faces := decodeYuNet(yunetOutput(0.9, 0.2), 0.5); len(faces) != 0 {
		t.Fatalf("score sqrt(0.18) kept: %+v", faces)
	}
}

func TestNMSDropsOverlapsKeepsSeparateFaces(t *testing.T) {
	faces := nms([]Face{
		{X: 0, Y: 0, W: 10, H: 10, Score: 0.7},
		{X: 1, Y: 1, W: 10, H: 10, Score: 0.9},
		{X: 50, Y: 50, W: 10, H: 10, Score: 0.6},
	}, 0.3)
	if len(faces) != 2 || faces[0].Score != 0.9 || faces[1].Score != 0.6 {
		t.Fatalf("got %+v, want the 0.9 and 0.6 faces", faces)
	}
}

// Both recognizers take planar RGB; SFace takes raw 0..255 and ArcFace maps it to -1..1.
func TestModelInputNormalizesPerRecognizer(t *testing.T) {
	pix := make([]byte, AlignedSide*AlignedSide*3)
	pix[0], pix[1], pix[2] = 0, 127, 255 // first pixel, BGR
	a, err := alignedImage(Image{W: AlignedSide, H: AlignedSide, Pix: pix})
	if err != nil {
		t.Fatal(err)
	}
	plane := AlignedSide * AlignedSide
	for _, tc := range []struct {
		model   Model
		r, g, b float64
	}{
		{SFaceInt8, 255, 127, 0},
		{ArcFaceR50, 1, -0.5 / 127.5, -1},
	} {
		in := tc.model.input(a)
		near(t, tc.model.ID+" R", float64(in[0]), tc.r)
		near(t, tc.model.ID+" G", float64(in[plane]), tc.g)
		near(t, tc.model.ID+" B", float64(in[2*plane]), tc.b)
	}
}

func TestAlignedImageRejectsOtherSizes(t *testing.T) {
	if _, err := alignedImage(Image{W: 100, H: 100, Pix: make([]byte, 100*100*3)}); err == nil {
		t.Fatal("a 100x100 image was accepted as an aligned face")
	}
}
