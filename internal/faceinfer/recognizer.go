package faceinfer

import (
	"fmt"
	"math"
)

// Model describes one recognizer: both take the same aligned face and differ only in the input
// normalization and the vector length.
type Model struct {
	ID  string
	Dim int
	// Each RGB sample enters the model as (v - mean) / std.
	mean, std float32
}

// Recognizer models by the ids the settings store.
var (
	SFaceInt8  = Model{ID: "sface_int8", Dim: 128, mean: 0, std: 1}
	ArcFaceR50 = Model{ID: "arcface_r50", Dim: 512, mean: 127.5, std: 127.5}
)

// ModelByID returns the recognizer model with the given id.
func ModelByID(id string) (Model, bool) {
	for _, m := range []Model{SFaceInt8, ArcFaceR50} {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// Recognizer turns an aligned face into an L2-normalized vector.
type Recognizer struct {
	s     *ortSession
	model Model
}

// NewRecognizer creates a session for m from the model file's bytes.
func (r *Runtime) NewRecognizer(m Model, model []byte, threads int) (*Recognizer, error) {
	s, err := r.newSession(model, threads, "", nil)
	if err != nil {
		return nil, fmt.Errorf("recognizer %s: %w", m.ID, err)
	}
	return &Recognizer{s: s, model: m}, nil
}

// Embed aligns one detected face of img and returns its vector.
func (rc *Recognizer) Embed(img Image, f Face) ([]float32, error) {
	return rc.run(alignFace(img, f))
}

// EmbedAligned returns the vector of a face already aligned to the AlignedSide template.
func (rc *Recognizer) EmbedAligned(img Image) ([]float32, error) {
	a, err := alignedImage(img)
	if err != nil {
		return nil, err
	}
	return rc.run(a)
}

func (rc *Recognizer) run(a aligned) ([]float32, error) {
	out, err := rc.s.run(rc.model.input(a), []int64{1, 3, AlignedSide, AlignedSide})
	if err != nil {
		return nil, err
	}
	if len(out[0].data) != rc.model.Dim {
		return nil, fmt.Errorf("%s returned %d values, want %d", rc.model.ID, len(out[0].data), rc.model.Dim)
	}
	return normalize(out[0].data), nil
}

// input lays the face out as the planar RGB tensor the model takes.
func (m Model) input(a aligned) []float32 {
	plane := AlignedSide * AlignedSide
	out := make([]float32, 3*plane)
	for c, src := range [3][]float32{a[2], a[1], a[0]} {
		for i, v := range src {
			out[c*plane+i] = (v - m.mean) / m.std
		}
	}
	return out
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
