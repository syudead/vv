package domain

import "errors"

// ErrFaceGenerationChanged refuses a result computed before the recognizer was switched; nothing of it is stored.
var ErrFaceGenerationChanged = errors.New("the face recognizer changed while the result was computed")

// FaceQueueScope chooses the videos a run queues for face analysis.
type FaceQueueScope string

const (
	// FaceQueueMissing queues videos never analysed or whose analysis failed.
	FaceQueueMissing FaceQueueScope = "missing"
	// FaceQueueAll queues every video again; a running analysis keeps running.
	FaceQueueAll FaceQueueScope = "all"
)

func (s FaceQueueScope) Valid() bool {
	return s == FaceQueueMissing || s == FaceQueueAll
}

// FaceJob is a claimed face analysis.
type FaceJob struct {
	ContentKey string
}

// FaceAnalysis is one video's finished analysis, as the face worker hands it to the store.
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
}

// MainPerson returns the person in the shape the appearance rules take; only its spans and centroid are set.
func (p FaceAnalysedPerson) MainPerson() MainPerson {
	tracks := make([]FaceTrack, len(p.Spans))
	for i, s := range p.Spans {
		tracks[i].FaceSpan = s
	}
	return MainPerson{Tracks: tracks, GoodSeconds: p.GoodSeconds, Centroid: p.Centroid}
}
