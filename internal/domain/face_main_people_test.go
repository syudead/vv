package domain

import (
	"math"
	"testing"
)

// faceAxis is a unit vector along axis a, tilted by axis b so two tracks of one person differ slightly.
func faceAxis(a, b int, tilt float32) []float32 {
	v := make([]float32, 128)
	v[a], v[b] = 1, tilt
	return NormalizeFaceVector(v)
}

// goodFace is a large, confident, frontal face at (x, y) carrying vec.
func goodFace(x, y float64, vec []float32) FaceDetection {
	return FaceDetection{
		Box:       FaceBox{X: x, Y: y, W: 80, H: 80},
		Landmarks: [10]float64{x + 20, y + 30, x + 60, y + 30, x + 40, y + 50, x + 25, y + 65, x + 55, y + 65},
		Score:     0.95,
		Vector:    vec,
	}
}

// goodTrack is a track of n good faces carrying vec, starting at frame start.
func goodTrack(vec []float32, start, n int) FaceTrack {
	var t FaceTrack
	for i := range n {
		t.add(start+i, goodFace(100, 100, vec))
	}
	return t
}

func TestFindMainPeopleKeepsEightSecondsInTwoTracks(t *testing.T) {
	th := FaceThresholds{Cluster: 0.45}
	// 16 good frames are 8 s at 2 fps; 15 are 7.5 s.
	for _, tc := range []struct {
		name   string
		tracks []FaceTrack
		kept   bool
	}{
		{"8 s in 2 tracks", []FaceTrack{goodTrack(faceAxis(0, 10, 0.2), 0, 8), goodTrack(faceAxis(0, 11, 0.2), 20, 8)}, true},
		{"7.5 s in 2 tracks", []FaceTrack{goodTrack(faceAxis(0, 10, 0.2), 0, 8), goodTrack(faceAxis(0, 11, 0.2), 20, 7)}, false},
		{"10 s in 1 track", []FaceTrack{goodTrack(faceAxis(0, 10, 0.2), 0, 20)}, false},
	} {
		people := FindMainPeople(th, tc.tracks)
		if got := len(people) == 1; got != tc.kept {
			t.Errorf("%s: kept = %v (%d people), want %v", tc.name, got, len(people), tc.kept)
		}
	}
}

func TestFindMainPeopleSeparatesPeopleBelowClusterThreshold(t *testing.T) {
	// Person A: 3 tracks of 6 good frames (9 s); person B: 2 tracks of 10 (10 s), orthogonal to A.
	var tracks []FaceTrack
	for i := range 3 {
		tracks = append(tracks, goodTrack(faceAxis(0, 10+i, 0.2), i*10, 6))
	}
	for i := range 2 {
		tracks = append(tracks, goodTrack(faceAxis(1, 20+i, 0.2), i*20, 10))
	}
	people := FindMainPeople(FaceThresholds{Cluster: 0.45}, tracks)
	if len(people) != 2 || people[0].GoodSeconds != 10 || people[1].GoodSeconds != 9 {
		t.Fatalf("people = %+v, want B (10 s) then A (9 s)", people)
	}
	if len(people[0].Tracks) != 2 || len(people[1].Tracks) != 3 {
		t.Fatalf("track counts %d, %d; want 2, 3", len(people[0].Tracks), len(people[1].Tracks))
	}
	if s := FaceSimilarity(people[1].Centroid, faceAxis(0, 1, 0)); s < 0.9 {
		t.Fatalf("A's centroid is %v from A's axis, want close", s)
	}
}

func TestFaceQualityGate(t *testing.T) {
	vec := faceAxis(0, 1, 0)
	profile := goodFace(0, 0, vec)
	profile.Landmarks[4] = profile.Landmarks[2] - 1 // nose next to the right eye
	small := goodFace(0, 0, vec)
	small.Box.W = 63
	weak := goodFace(0, 0, vec)
	weak.Score = 0.8
	for name, tc := range map[string]struct {
		d    FaceDetection
		want bool
	}{"frontal": {goodFace(0, 0, vec), true}, "profile": {profile, false}, "63 px": {small, false}, "weak": {weak, false}} {
		if got := tc.d.good(); got != tc.want {
			t.Errorf("%s: good = %v, want %v", name, got, tc.want)
		}
	}
}

func TestTrackVectorCountsOnlyGoodFaces(t *testing.T) {
	var tr FaceTrack
	tr.add(0, goodFace(0, 0, faceAxis(0, 1, 0)))
	tr.add(1, goodFace(0, 0, faceAxis(0, 1, 0)))
	bad := goodFace(0, 0, faceAxis(5, 6, 0))
	bad.Score = 0.7
	tr.add(2, bad)
	if tr.GoodFrames != 2 || math.Abs(FaceSimilarity(tr.vector(), faceAxis(0, 1, 0))-1) > 1e-6 {
		t.Fatalf("good = %d, vector includes the weak face", tr.GoodFrames)
	}
	var single FaceTrack
	single.add(0, goodFace(0, 0, faceAxis(0, 1, 0)))
	if single.vector() != nil {
		t.Fatal("a track with one good face has a vector")
	}
}

func TestFaceTrackerFollowsTwoFacesAndClosesAfterGap(t *testing.T) {
	a, b := faceAxis(0, 1, 0), faceAxis(2, 3, 0)
	k := NewFaceTracker(FaceThresholds{Track: 0.45, TrackFar: 0.6})
	for range 4 {
		k.Add([]FaceDetection{goodFace(100, 100, a), goodFace(600, 100, b)})
	}
	for range 2 {
		k.Add(nil)
	}
	// A returns after more than the allowed gap, and a 40 px face is never tracked.
	tiny := goodFace(900, 100, b)
	tiny.Box.W = 40
	for range 3 {
		k.Add([]FaceDetection{goodFace(100, 100, a), tiny})
	}
	tracks := k.Tracks()
	if len(tracks) != 3 {
		t.Fatalf("got %d tracks, want A, B and A again", len(tracks))
	}
	want := []FaceSpan{
		{Start: 0, End: 1.5, X: 140.0 / FaceFrameSide, Y: 140.0 / FaceFrameSide, W: 80.0 / FaceFrameSide},
		{Start: 0, End: 1.5, X: 640.0 / FaceFrameSide, Y: 140.0 / FaceFrameSide, W: 80.0 / FaceFrameSide},
		{Start: 3, End: 4, X: 140.0 / FaceFrameSide, Y: 140.0 / FaceFrameSide, W: 80.0 / FaceFrameSide},
	}
	for i, w := range want {
		if tracks[i].FaceSpan != w {
			t.Errorf("track %d = %+v, want %+v", i, tracks[i].FaceSpan, w)
		}
	}
}
