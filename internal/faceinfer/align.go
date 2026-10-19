package faceinfer

import "fmt"

// AlignedSide is the side of the aligned face both recognizers take.
const AlignedSide = 112

// arcTemplate holds the 112x112 reference landmarks shared by SFace and ArcFace, in YuNet's order.
var arcTemplate = [10]float64{
	38.2946, 51.6963, 73.5318, 51.5014, 56.0252, 71.7366, 41.5493, 92.3655, 70.7299, 92.2041,
}

// aligned is one AlignedSide square face as planar B, G and R samples in 0..255.
type aligned [3][]float32

// affine maps (x, y) to (a*x + b*y + c, d*x + e*y + f).
type affine struct{ a, b, c, d, e, f float64 }

// similarity is the least-squares rotation, uniform scale and translation from src to dst points
// (Umeyama without reflection), the transform OpenCV's FaceRecognizerSF.alignCrop uses.
func similarity(src, dst [10]float64) affine {
	var sx, sy, dx, dy float64
	for i := 0; i < 10; i += 2 {
		sx, sy, dx, dy = sx+src[i], sy+src[i+1], dx+dst[i], dy+dst[i+1]
	}
	sx, sy, dx, dy = sx/5, sy/5, dx/5, dy/5
	var num, cross, den float64
	for i := 0; i < 10; i += 2 {
		px, py, qx, qy := src[i]-sx, src[i+1]-sy, dst[i]-dx, dst[i+1]-dy
		num += px*qx + py*qy
		cross += px*qy - py*qx
		den += px*px + py*py
	}
	cs, sn := num/den, cross/den
	return affine{a: cs, b: -sn, c: dx - cs*sx + sn*sy, d: sn, e: cs, f: dy - sn*sx - cs*sy}
}

func (m affine) inverse() affine {
	det := m.a*m.e - m.b*m.d
	a, b, d, e := m.e/det, -m.b/det, -m.d/det, m.a/det
	return affine{a: a, b: b, c: -(a*m.c + b*m.f), d: d, e: e, f: -(d*m.c + e*m.f)}
}

func (m affine) apply(x, y float64) (float64, float64) {
	return m.a*x + m.b*y + m.c, m.d*x + m.e*y + m.f
}

// alignFace warps the face onto the template, sampling bilinearly without rounding to bytes.
func alignFace(img Image, f Face) aligned {
	inv := similarity(f.Landmarks, arcTemplate).inverse()
	out := newAligned()
	for y := range AlignedSide {
		for x := range AlignedSide {
			sx, sy := inv.apply(float64(x), float64(y))
			i := y*AlignedSide + x
			out[0][i], out[1][i], out[2][i] = sampleBGR(img, sx, sy)
		}
	}
	return out
}

// alignedImage reads a face that the caller already aligned to the template.
func alignedImage(img Image) (aligned, error) {
	if img.W != AlignedSide || img.H != AlignedSide || len(img.Pix) != AlignedSide*AlignedSide*3 {
		return aligned{}, fmt.Errorf("aligned face is %dx%d with %d bytes, want %dx%d BGR", img.W, img.H, len(img.Pix), AlignedSide, AlignedSide)
	}
	out := newAligned()
	for i := range AlignedSide * AlignedSide {
		for c := range out {
			out[c][i] = float32(img.Pix[i*3+c])
		}
	}
	return out, nil
}

func newAligned() aligned {
	var a aligned
	for c := range a {
		a[c] = make([]float32, AlignedSide*AlignedSide)
	}
	return a
}
