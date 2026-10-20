package domain

import "math"

// Face recognition rules and their measured constants are described in docs/design-docs/face-recognition.md.

// FaceRecognizer names the model that turns an aligned face into a vector; vectors of two recognizers are never compared.
type FaceRecognizer string

const (
	FaceRecognizerSFaceInt8  FaceRecognizer = "sface_int8"
	FaceRecognizerArcFaceR50 FaceRecognizer = "arcface_r50"
)

// DefaultFaceRecognizer is used while the owner has not chosen one.
const DefaultFaceRecognizer = FaceRecognizerSFaceInt8

// FaceThresholds are cosine similarities between normalized vectors of one recognizer.
type FaceThresholds struct {
	// Track continues a track with a face whose box overlaps the track's last box.
	Track float64
	// TrackFar continues a track with a face whose box no longer overlaps it.
	TrackFar float64
	// Cluster is the lowest mean similarity at which average linkage joins two clusters of tracks.
	Cluster float64
	// Join and above joins two clusters of one video that passed the cutoff; a centroid of many faces is steadier
	// than the single faces Cluster compares, and one video holds few people.
	Join float64
	// Candidate up to Link offers the pair to the owner.
	Candidate float64
	// Link and above links a main person to a person of the library without the owner.
	Link float64
	// Margin is how far the most similar person must lead the next one for a link; a closer pair is the owner's call.
	Margin float64
	// Merge and above merges a provisional person into a named person whose face image matches.
	Merge float64
}

// FaceRecognizerSpec is one row of the recognizer table.
type FaceRecognizerSpec struct {
	Recognizer FaceRecognizer
	// Dimensions is the length of the recognizer's vector.
	Dimensions int
	Thresholds FaceThresholds
}

// The SFace values are the POC's; the ArcFace values give the same false rates on LFW. Link equals Merge because
// a link is as unattended as a merge, and the looser Join rate named strangers in real libraries (face-recognition.md).
var faceRecognizers = []FaceRecognizerSpec{
	{
		Recognizer: FaceRecognizerSFaceInt8,
		Dimensions: 128,
		Thresholds: FaceThresholds{Track: 0.45, TrackFar: 0.6, Cluster: 0.45, Join: 0.45, Candidate: 0.363, Link: 0.55, Margin: 0.1, Merge: 0.55},
	},
	{
		Recognizer: FaceRecognizerArcFaceR50,
		Dimensions: 512,
		Thresholds: FaceThresholds{Track: 0.288, TrackFar: 0.614, Cluster: 0.288, Join: 0.288, Candidate: 0.205, Link: 0.545, Margin: 0.1, Merge: 0.545},
	},
}

// LookupFaceRecognizer returns the table row of r and false for an unknown recognizer.
func LookupFaceRecognizer(r FaceRecognizer) (FaceRecognizerSpec, bool) {
	for _, spec := range faceRecognizers {
		if spec.Recognizer == r {
			return spec, true
		}
	}
	return FaceRecognizerSpec{}, false
}

func (r FaceRecognizer) Valid() bool {
	_, ok := LookupFaceRecognizer(r)
	return ok
}

// FaceSimilarity is the cosine similarity of two normalized vectors; vectors of different lengths are unrelated.
func FaceSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return -1
	}
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// NormalizeFaceVector scales v to unit length; a zero vector stays zero.
func NormalizeFaceVector(v []float32) []float32 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	n = math.Sqrt(n)
	out := make([]float32, len(v))
	if n == 0 {
		return out
	}
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out
}
