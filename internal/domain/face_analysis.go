package domain

import "errors"

var (
	// ErrFaceGenerationChanged refuses a result computed before the recognizer was switched; nothing of it is stored.
	ErrFaceGenerationChanged = errors.New("the face recognizer changed while the result was computed")
	// ErrFaceHelperStart reports a face helper that did not start or greet; the runtime, not the video, is at fault.
	ErrFaceHelperStart = errors.New("the face helper did not start")
	// ErrFaceImageChanged refuses a vector computed for a face that was replaced since it was read; nothing is stored.
	ErrFaceImageChanged = errors.New("the face image changed while its vector was computed")
)

// FaceQueueScope chooses the videos a run queues for face analysis.
type FaceQueueScope string

const (
	// FaceQueueMissing queues videos never analysed or whose analysis failed; it is the only scope, since an analysed
	// video carries its appearances over and would find the same people again.
	FaceQueueMissing FaceQueueScope = "missing"
)

func (s FaceQueueScope) Valid() bool {
	return s == FaceQueueMissing
}

// FaceJob is a face analysis with the recognizer and face.generation current when it started, read together so a
// result is never stored under a generation its recognizer does not belong to.
type FaceJob struct {
	ContentKey string
	Recognizer FaceRecognizer
	Generation int64
}

// PeopleCleared is what deleting every person did: the people deleted and the videos queued for face analysis.
type PeopleCleared struct {
	People int
	Queued int
}

// FaceAnalysis is one video's finished analysis, as the face job hands it to the store.
type FaceAnalysis struct {
	ContentKey string
	Recognizer FaceRecognizer
	// Generation is the face.generation the analysis started under.
	Generation int64
	People     []FaceAnalysedPerson
}

// FaceAnalysedPerson is one main person of the analysed video.
type FaceAnalysedPerson struct {
	Spans       []FaceSpan
	GoodSeconds float64
	Centroid    []float32
	// Crop is the best good face, the face box doubled around its centre, fitted as a person's face image.
	Crop PersonImage
	// FaceVector is the vector of the crop's face, stored as the face's vector of a person created from it.
	FaceVector []float32
}

// MainPerson returns the person in the shape the appearance rules take; only its spans and centroid are set.
func (p FaceAnalysedPerson) MainPerson() MainPerson {
	tracks := make([]FaceTrack, len(p.Spans))
	for i, s := range p.Spans {
		tracks[i].FaceSpan = s
	}
	return MainPerson{Tracks: tracks, GoodSeconds: p.GoodSeconds, Centroid: p.Centroid}
}

var (
	// ErrPersonImageNoFace reports a face image without a good face, which gets no vector.
	ErrPersonImageNoFace = errors.New("the image shows no usable face")
	// ErrPersonImageManyFaces reports a face image with more than one good face, which gets no vector.
	ErrPersonImageManyFaces = errors.New("the image shows more than one face")
)

// FaceImageJob is a person's face image for its face_image job, read with the recognizer and face.generation current
// at the read.
type FaceImageJob struct {
	PersonID   int64
	Revision   int64
	Recognizer FaceRecognizer
	Generation int64
	JPEG       []byte
	// Embedded is true when a vector of this revision and recognizer is stored; Vector is that vector, nil when the
	// image had no single good face.
	Embedded bool
	Vector   []float32
}

// FaceImageVector returns the normalized vector of the only good face of a person's face image, or
// ErrPersonImageNoFace or ErrPersonImageManyFaces.
func FaceImageVector(faces []FaceDetection) ([]float32, error) {
	var good []FaceDetection
	for _, d := range faces {
		if d.good() {
			good = append(good, d)
		}
	}
	switch len(good) {
	case 0:
		return nil, ErrPersonImageNoFace
	case 1:
		return NormalizeFaceVector(good[0].Vector), nil
	}
	return nil, ErrPersonImageManyFaces
}
