package main

import (
	"fmt"
	"math"
)

// sfaceDim is the length of an SFace embedding.
const sfaceDim = 128

type sface struct{ s *ortSession }

// embed returns the L2-normalised SFace vector of one detected face.
func (m *sface) embed(img bgrImage, f face) ([]float32, error) {
	out, err := m.s.run(alignCrop(img, f), []int64{1, 3, 112, 112})
	if err != nil {
		return nil, err
	}
	if len(out[0].data) != sfaceDim {
		return nil, fmt.Errorf("sface returned %d values, want %d", len(out[0].data), sfaceDim)
	}
	return normalize(out[0].data), nil
}

func normalize(v []float32) []float32 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	n = math.Max(math.Sqrt(n), 1e-9)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}
