package main

import "math"

// arcTemplate holds the 112x112 reference landmarks shared by SFace and ArcFace, in YuNet's order.
var arcTemplate = [10]float64{
	38.2946, 51.6963, 73.5318, 51.5014, 56.0252, 71.7366, 41.5493, 92.3655, 70.7299, 92.2041,
}

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

// alignCrop warps the face onto the 112x112 template as planar RGB float32 in 0..255, SFace's input.
func alignCrop(img bgrImage, f face) []float32 {
	const side = 112
	inv := similarity(f.kps, arcTemplate).inverse()
	out := make([]float32, 3*side*side)
	for y := range side {
		for x := range side {
			sx, sy := inv.apply(float64(x), float64(y))
			b, g, r := sampleBGR(img, sx, sy)
			i := y*side + x
			out[i], out[side*side+i], out[2*side*side+i] = r, g, b
		}
	}
	return out
}

// yawRatio is where the nose sits between the eyes horizontally: 0.5 for a frontal face.
func yawRatio(f face) float64 {
	span := f.kps[2] - f.kps[0]
	if math.Abs(span) < 1e-6 {
		return math.Inf(1)
	}
	return (f.kps[4] - f.kps[0]) / span
}
