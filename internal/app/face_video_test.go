package app

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// letterboxedFrame is a frame whose picture is a grey band of rows [top, bottom), black elsewhere.
func letterboxedFrame(top, bottom int) []byte {
	pix := make([]byte, domain.FaceFrameSide*domain.FaceFrameSide*3)
	for i := top * domain.FaceFrameSide * 3; i < bottom*domain.FaceFrameSide*3; i++ {
		pix[i] = 128
	}
	return pix
}

func frameFace(x, y, side float64, vec []float32) domain.FaceDetection {
	return domain.FaceDetection{
		Box: domain.FaceBox{X: x, Y: y, W: side, H: side},
		Landmarks: [10]float64{
			x + side/4, y + side/3, x + 3*side/4, y + side/3, x + side/2, y + side/2,
			x + side/3, y + 3*side/4, x + 2*side/3, y + 3*side/4,
		},
		Score: 0.95, Vector: vec,
	}
}

func TestLitBoundsLeavesOutBlackPadding(t *testing.T) {
	if got, want := litBounds(letterboxedFrame(100, 720)), image.Rect(0, 100, domain.FaceFrameSide, 720); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}
	if got := litBounds(letterboxedFrame(0, 0)); !got.Empty() {
		t.Fatalf("bounds of a black frame = %v, want empty", got)
	}
}

// A dark frame lit only in its middle keeps the picture an earlier frame lit, so a face near the picture's
// edge in that frame is not taken as cut.
func TestFaceVideoKeepsThePictureOfEarlierFrames(t *testing.T) {
	v := newFaceVideo(domain.FaceThresholds{Track: 0.45, TrackFar: 0.6})
	for _, pix := range [][]byte{letterboxedFrame(0, 720), letterboxedFrame(300, 400)} {
		if err := v.add(pix, nil); err != nil {
			t.Fatal(err)
		}
	}
	if want := image.Rect(0, 0, domain.FaceFrameSide, 720); v.lit != want {
		t.Fatalf("picture = %v, want %v", v.lit, want)
	}
}

// The main person's crop comes from the whole face although a larger face cut by the picture's bottom edge
// scores higher, and the crop window near that edge moves up into the picture instead of taking the padding.
func TestFaceVideoCropsAWholeFaceInsideThePicture(t *testing.T) {
	vec := domain.NormalizeFaceVector([]float32{1, 0, 0, 0})
	pix := letterboxedFrame(0, 720)
	v := newFaceVideo(domain.FaceThresholds{Track: 0.45, TrackFar: 0.6, Cluster: 0.45, Join: 0.45})
	for _, f := range []domain.FaceDetection{
		frameFace(500, 500, 300, vec), frameFace(600, 560, 120, vec), frameFace(500, 500, 300, vec), frameFace(500, 500, 300, vec),
	} {
		if err := v.add(pix, []domain.FaceDetection{f}); err != nil {
			t.Fatal(err)
		}
	}
	people := v.people()
	if len(people) != 1 {
		t.Fatalf("%d people, want 1", len(people))
	}
	crop, err := jpeg.Decode(bytes.NewReader(people[0].Crop.JPEG))
	if err != nil {
		t.Fatal(err)
	}
	if got := crop.Bounds().Size(); got != image.Pt(240, 240) {
		t.Fatalf("crop is %v, want the whole face doubled (240 x 240)", got)
	}
	if r, _, _, _ := crop.At(120, 236).RGBA(); r>>8 < 100 {
		t.Fatalf("crop's bottom row is %d, want the grey picture, not the black padding", r>>8)
	}
}
