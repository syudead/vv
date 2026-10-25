package domain

import (
	"math"
	"reflect"
	"testing"
)

// Thresholds that float32 vectors reach exactly, so each boundary is tested at its value.
var testFaceThresholds = FaceThresholds{Candidate: 0.5, Merge: 0.875}

// atCosine is a unit vector whose cosine with faceAxis(0, 1, 0) is exactly s.
func atCosine(s float32) []float32 {
	v := make([]float32, 128)
	v[0], v[1] = s, float32(math.Sqrt(float64(1-s*s)))
	return v
}

func TestFaceCandidatesAtEachThreshold(t *testing.T) {
	probe := faceAxis(0, 1, 0)
	for _, tc := range []struct {
		name    string
		vectors []FacePersonVector
		want    []FaceCandidate
	}{
		{"at candidate", []FacePersonVector{{PersonID: 1, Vector: atCosine(0.5)}},
			[]FaceCandidate{{PersonID: 1, Similarity: 0.5}}},
		{"below candidate", []FacePersonVector{{PersonID: 1, Vector: atCosine(0.4375)}}, nil},
		// Even an identical face is only offered; nobody is linked without the owner.
		{"most similar first", []FacePersonVector{{PersonID: 2, Vector: atCosine(0.75)}, {PersonID: 1, Vector: atCosine(1)}, {PersonID: 3, Vector: atCosine(0.4375)}},
			[]FaceCandidate{{PersonID: 1, Similarity: 1}, {PersonID: 2, Similarity: 0.75}}},
	} {
		if got := FaceCandidates(testFaceThresholds, probe, tc.vectors); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// spanAt is a track of the whole first minute with its face centre at (x, 0.5) and width w.
func spanAt(x, w float64) FaceSpan {
	return FaceSpan{Start: 0, End: 60, X: x, Y: 0.5, W: w}
}

// mainPersonAt is a main person whose only track is span, with centroid vec.
func mainPersonAt(span FaceSpan, vec []float32) MainPerson {
	return MainPerson{Tracks: []FaceTrack{{FaceSpan: span}}, Centroid: vec}
}

func TestCarryOverKeepsPeopleOnScreenTogether(t *testing.T) {
	left, right := spanAt(0.3, 0.1), spanAt(0.7, 0.1)
	stored := []StoredFaceAppearance{{ID: 10, PersonID: 1, Spans: []FaceSpan{left}}, {ID: 11, PersonID: 2, Spans: []FaceSpan{right}}}
	// The re-analysis numbers them the other way round.
	got := CarryOverFaceAppearances(stored, [][]FaceSpan{{right}, {left}})
	if !reflect.DeepEqual(got, []int64{11, 10}) {
		t.Fatalf("carried from %v, want [11 10]", got)
	}
}

func TestCarryOverRefusesTwoClaimsOnOneAppearance(t *testing.T) {
	first := FaceSpan{Start: 0, End: 30, X: 0.5, Y: 0.5, W: 0.1}
	second := FaceSpan{Start: 30.5, End: 60, X: 0.5, Y: 0.5, W: 0.1}
	stored := []StoredFaceAppearance{{ID: 10, PersonID: 1, Spans: []FaceSpan{first}}, {ID: 11, PersonID: 2, Spans: []FaceSpan{second}}}
	got := CarryOverFaceAppearances(stored, [][]FaceSpan{{spanAt(0.5, 0.1)}})
	if !reflect.DeepEqual(got, []int64{0}) {
		t.Fatalf("carried from %v, want nothing", got)
	}
}

func TestCarryOverKeepsSmallFaceBesideLargeOne(t *testing.T) {
	// The small face's centre is 0.15 from the large one's, closer than the large width 0.3 but farther than 0.05.
	large, small := spanAt(0.4, 0.3), spanAt(0.55, 0.05)
	stored := []StoredFaceAppearance{{ID: 10, PersonID: 1, Spans: []FaceSpan{large}}, {ID: 11, PersonID: 2, Spans: []FaceSpan{small}}}
	got := CarryOverFaceAppearances(stored, [][]FaceSpan{{small}, {large}})
	if !reflect.DeepEqual(got, []int64{11, 10}) {
		t.Fatalf("carried from %v, want [11 10]", got)
	}
}

func TestPlanFaceAppearancesKeepsCarriedPersonAndOffersTheRest(t *testing.T) {
	span, other := spanAt(0.3, 0.1), spanAt(0.7, 0.1)
	stored := []StoredFaceAppearance{
		{ID: 10, PersonID: 1, Spans: []FaceSpan{span}},
		{ID: 11, PersonID: 2, Dropped: true, Spans: []FaceSpan{other}},
	}
	// Every new centroid equals person 3's face, yet only an appearance nothing carries over to is offered with them.
	person3 := faceAxis(0, 1, 0)
	vectors := []FacePersonVector{{PersonID: 1, Vector: faceAxis(5, 6, 0)}, {PersonID: 3, Vector: person3}}
	people := []MainPerson{mainPersonAt(span, person3), mainPersonAt(other, person3), mainPersonAt(spanAt(0.5, 0.1), person3)}
	got := PlanFaceAppearances(testFaceThresholds, stored, people, vectors)
	want := []FaceAppearancePlan{
		{CarriedFrom: 10},
		{CarriedFrom: 11},
		{Candidates: []FaceCandidate{{PersonID: 3, Similarity: 1}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plans = %+v, want %+v", got, want)
	}
}

func TestDecideFaceImageMerge(t *testing.T) {
	face := faceAxis(0, 1, 0)
	for _, tc := range []struct {
		name        string
		provisional bool
		vectors     []FacePersonVector
		want        FaceImageDecision
	}{
		{"one at merge", false, []FacePersonVector{{PersonID: 7, Vector: atCosine(0.875)}, {PersonID: 8, Vector: atCosine(0.5)}},
			FaceImageDecision{MergeID: 7, Candidates: []FaceCandidate{{PersonID: 8, Similarity: 0.5}}}},
		{"two at merge", false, []FacePersonVector{{PersonID: 7, Vector: atCosine(0.875)}, {PersonID: 8, Vector: atCosine(0.9375)}},
			FaceImageDecision{Candidates: []FaceCandidate{{PersonID: 8, Similarity: 0.9375}, {PersonID: 7, Similarity: 0.875}}}},
		{"below merge", false, []FacePersonVector{{PersonID: 7, Vector: atCosine(0.8125)}},
			FaceImageDecision{Candidates: []FaceCandidate{{PersonID: 7, Similarity: 0.8125}}}},
		{"provisional person's own face", true, []FacePersonVector{{PersonID: 1, Vector: face}, {PersonID: 7, Vector: atCosine(0.875)}},
			FaceImageDecision{Candidates: []FaceCandidate{{PersonID: 7, Similarity: 0.875}}}},
	} {
		if got := DecideFaceImageMerge(testFaceThresholds, 1, tc.provisional, face, tc.vectors); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestDecideCastMerge(t *testing.T) {
	named := FaceCastPerformer{PersonID: 1}
	provisional := FaceCastPerformer{PersonID: 9, Provisional: true, FaceLinked: true}
	for _, tc := range []struct {
		name       string
		performers []FaceCastPerformer
		dismissed  map[FacePersonPair]bool
		ok         bool
	}{
		{"one performer, one provisional", []FaceCastPerformer{named, provisional, {PersonID: 2, FaceLinked: true}}, nil, true},
		{"two performers without a face", []FaceCastPerformer{named, {PersonID: 2}, provisional}, nil, false},
		{"two provisional people", []FaceCastPerformer{named, provisional, {PersonID: 10, Provisional: true, FaceLinked: true}}, nil, false},
		{"dismissed pair", []FaceCastPerformer{named, provisional}, map[FacePersonPair]bool{{ProvisionalID: 9, PersonID: 1}: true}, false},
		{"provisional performer without a face", []FaceCastPerformer{{PersonID: 3, Provisional: true}, provisional}, nil, false},
	} {
		pair, ok := DecideCastMerge(tc.performers, tc.dismissed)
		if ok != tc.ok || (ok && pair != FacePersonPair{ProvisionalID: 9, PersonID: 1}) {
			t.Errorf("%s: got %+v, %v; want ok %v", tc.name, pair, ok, tc.ok)
		}
	}
}

func TestFaceRecognizerTable(t *testing.T) {
	for _, r := range []FaceRecognizer{FaceRecognizerSFaceInt8, FaceRecognizerArcFaceR50} {
		spec, ok := LookupFaceRecognizer(r)
		th := spec.Thresholds
		if !ok || th.Candidate >= th.Merge || th.Join > th.Merge {
			t.Errorf("%s: %+v, %v; want candidate < merge and join <= merge", r, spec, ok)
		}
	}
	if FaceRecognizer("sface").Valid() {
		t.Error("an unknown recognizer is valid")
	}
}

func TestSiblingFacePairsReachCandidateInOrder(t *testing.T) {
	people := []FacePersonVector{
		{PersonID: 1, Vector: faceAxis(0, 1, 0)},
		{PersonID: 2, Vector: atCosine(0.5)},
		{PersonID: 3, Vector: faceAxis(7, 8, 0)},
	}
	got := SiblingFacePairs(testFaceThresholds, people)
	want := []FaceSiblingPair{{ProvisionalID: 2, PersonID: 1, Similarity: 0.5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pairs = %+v, want %+v", got, want)
	}
}
