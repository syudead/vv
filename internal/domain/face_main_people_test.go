package domain

import (
	"errors"
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

func TestFaceImageVectorTakesTheOnlyGoodFace(t *testing.T) {
	a, b := faceAxis(0, 1, 0), faceAxis(2, 3, 0)
	tiny := goodFace(600, 100, b)
	tiny.Box.W = 20
	for _, tc := range []struct {
		name  string
		faces []FaceDetection
		want  []float32
		err   error
	}{
		{"no face", nil, nil, ErrPersonImageNoFace},
		{"only a small face", []FaceDetection{tiny}, nil, ErrPersonImageNoFace},
		{"one good face beside a small one", []FaceDetection{goodFace(100, 100, a), tiny}, a, nil},
		{"two good faces", []FaceDetection{goodFace(100, 100, a), goodFace(600, 100, b)}, nil, ErrPersonImageManyFaces},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FaceImageVector(tc.faces)
			if !errors.Is(err, tc.err) || (got == nil) != (tc.want == nil) ||
				(tc.want != nil && FaceSimilarity(got, tc.want) < 0.999) {
				t.Fatalf("FaceImageVector = %v, %v; want %v, %v", got, err, tc.want, tc.err)
			}
		})
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
	th := FaceThresholds{Cluster: 0.45, Candidate: 0.363, Join: 0.45}
	// 8 good frames are 8 s at 1 fps; 7 are 7 s.
	for _, tc := range []struct {
		name   string
		tracks []FaceTrack
		kept   bool
	}{
		{"8 s in 2 tracks", []FaceTrack{goodTrack(faceAxis(0, 10, 0.2), 0, 4), goodTrack(faceAxis(0, 11, 0.2), 20, 4)}, true},
		{"7 s in 2 tracks", []FaceTrack{goodTrack(faceAxis(0, 10, 0.2), 0, 4), goodTrack(faceAxis(0, 11, 0.2), 20, 3)}, false},
		{"10 s in 1 track", []FaceTrack{goodTrack(faceAxis(0, 10, 0.2), 0, 10)}, false},
	} {
		people := FindMainPeople(th, tc.tracks, 60)
		if got := len(people) == 1; got != tc.kept {
			t.Errorf("%s: kept = %v (%d people), want %v", tc.name, got, len(people), tc.kept)
		}
	}
}

func TestFindMainPeopleCutoffFollowsAnalysedLength(t *testing.T) {
	th := FaceThresholds{Cluster: 0.45, Candidate: 0.363, Join: 0.45}
	vec := faceAxis(0, 10, 0.2)
	for _, tc := range []struct {
		name    string
		tracks  []FaceTrack
		seconds float64
		kept    bool
	}{
		{"one track through a 15 s video", []FaceTrack{goodTrack(vec, 0, 14)}, 15, true},
		{"3 s at the floor of a 4 s video", []FaceTrack{goodTrack(vec, 0, 3)}, 4, true},
		{"2 s cameo in a long video", []FaceTrack{goodTrack(vec, 0, 2)}, 600, false},
		{"2 s is under the floor", []FaceTrack{goodTrack(vec, 0, 2)}, 8, false},
		{"one 10 s track in a 100 s video", []FaceTrack{goodTrack(vec, 0, 10)}, 100, false},
		{"one 25 s track in a 100 s video", []FaceTrack{goodTrack(vec, 0, 25)}, 100, true},
		{"8 s in 2 tracks of a long video", []FaceTrack{goodTrack(vec, 0, 4), goodTrack(faceAxis(0, 11, 0.2), 20, 4)}, 600, true},
	} {
		if got := len(FindMainPeople(th, tc.tracks, tc.seconds)) == 1; got != tc.kept {
			t.Errorf("%s: kept = %v, want %v", tc.name, got, tc.kept)
		}
	}
}

func TestFindMainPeopleMergesClustersOfOnePerson(t *testing.T) {
	th := FaceThresholds{Cluster: 0.9, Candidate: 0.3, Join: 0.4}
	// Tilts put the two groups below Cluster of each other (similarity 0.5) but above Join.
	near := func(tilt float32, axis int) []float32 {
		v := make([]float32, 128)
		v[0], v[axis] = 1, tilt
		return NormalizeFaceVector(v)
	}
	a1, a2 := goodTrack(near(1, 1), 0, 5), goodTrack(near(1, 1), 30, 5)
	b1, b2 := goodTrack(near(1, 2), 60, 5), goodTrack(near(1, 2), 90, 5)
	if s := FaceSimilarity(a1.vector(), b1.vector()); s >= th.Cluster || s < th.Join {
		t.Fatalf("fixture similarity %v is not between Join and Cluster", s)
	}
	people := FindMainPeople(th, []FaceTrack{a1, a2, b1, b2}, 120)
	if len(people) != 1 || len(people[0].Tracks) != 4 || people[0].GoodSeconds != 20 {
		t.Fatalf("people = %+v, want one person of 4 tracks and 20 s", people)
	}
	// Distant people stay apart even though both pass the cutoff.
	c1, c2 := goodTrack(faceAxis(5, 6, 0.1), 0, 5), goodTrack(faceAxis(5, 7, 0.1), 30, 5)
	if got := FindMainPeople(th, []FaceTrack{a1, a2, c1, c2}, 120); len(got) != 2 {
		t.Fatalf("%d people, want 2 for orthogonal faces", len(got))
	}
}

func TestMainPersonCropPrefersAWholeFaceToALargerCutOne(t *testing.T) {
	v := faceAxis(0, 1, 0)
	cut := func(side float64) FaceDetection {
		d := goodFace(100, 100, v)
		d.Box.W, d.Box.H, d.Cut = side, side, true
		return d
	}
	var mixed, allCut FaceTrack
	mixed.add(0, cut(300))
	mixed.add(1, goodFace(100, 100, v))
	mixed.add(2, cut(300))
	for i := range 3 {
		allCut.add(10+i, cut(400))
	}
	people := FindMainPeople(FaceThresholds{Cluster: 0.45, Join: 0.45}, []FaceTrack{mixed, allCut}, 20)
	want := FaceSighting{Frame: 1, Box: FaceBox{X: 100, Y: 100, W: 80, H: 80}}
	if len(people) != 1 || people[0].Best != want {
		t.Fatalf("people = %+v, want one person whose best face is %+v", people, want)
	}
	if allCut.Best.Frame != 10 {
		t.Fatalf("a track of cut faces keeps best %+v, want its largest face", allCut.Best)
	}
}

// A person keeps the frontal face over a larger turned one, with that face's own vector as their face's vector.
func TestMainPersonFaceIsTheFrontalFaceWithItsVector(t *testing.T) {
	frontal, turned := faceAxis(0, 1, 0.1), faceAxis(0, 2, 0.1)
	side := goodFace(100, 100, turned)
	side.Box.W, side.Box.H = 100, 100
	side.Landmarks[4] = side.Landmarks[0] + 0.7*(side.Landmarks[2]-side.Landmarks[0])
	var tr FaceTrack
	tr.add(0, side)
	tr.add(1, goodFace(100, 100, frontal))
	tr.add(2, side)
	people := FindMainPeople(FaceThresholds{Cluster: 0.45, Join: 0.45}, []FaceTrack{tr}, 10)
	if len(people) != 1 || people[0].Best.Frame != 1 || FaceSimilarity(people[0].FaceVector, frontal) < 0.999 {
		t.Fatalf("people = %+v, want the frontal face of frame 1 and its vector", people)
	}
}

func TestFindMainPeopleSeparatesPeopleBelowClusterThreshold(t *testing.T) {
	// Person A: 3 tracks of 3 good frames (9 s); person B: 2 tracks of 5 (10 s), orthogonal to A.
	var tracks []FaceTrack
	for i := range 3 {
		tracks = append(tracks, goodTrack(faceAxis(0, 10+i, 0.2), i*10, 3))
	}
	for i := range 2 {
		tracks = append(tracks, goodTrack(faceAxis(1, 20+i, 0.2), i*20, 5))
	}
	people := FindMainPeople(FaceThresholds{Cluster: 0.45, Candidate: 0.363, Join: 0.45}, tracks, 60)
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
	// A returns after more than the allowed gap for the shortest track kept, and a 40 px face is never tracked.
	tiny := goodFace(900, 100, b)
	tiny.Box.W = 40
	for range 2 {
		k.Add([]FaceDetection{goodFace(100, 100, a), tiny})
	}
	tracks := k.Tracks()
	if len(tracks) != 3 {
		t.Fatalf("got %d tracks, want A, B and A again", len(tracks))
	}
	want := []FaceSpan{
		{Start: 0, End: 3, X: 140.0 / FaceFrameSide, Y: 140.0 / FaceFrameSide, W: 80.0 / FaceFrameSide},
		{Start: 0, End: 3, X: 640.0 / FaceFrameSide, Y: 140.0 / FaceFrameSide, W: 80.0 / FaceFrameSide},
		{Start: 6, End: 7, X: 140.0 / FaceFrameSide, Y: 140.0 / FaceFrameSide, W: 80.0 / FaceFrameSide},
	}
	for i, w := range want {
		if tracks[i].FaceSpan != w {
			t.Errorf("track %d = %+v, want %+v", i, tracks[i].FaceSpan, w)
		}
	}
}

func TestFaceTrackerGivesContestedTrackToMostSimilarFace(t *testing.T) {
	a, b := faceAxis(0, 1, 0), faceAxis(2, 3, 0)
	// first is 0.7 to A and 0.65 to B; second is 0.9 to A and unlike B, so only A to second and B to first continues both.
	first := make([]float32, 128)
	first[0], first[2], first[4] = 0.7, 0.65, float32(math.Sqrt(1-0.49-0.4225))
	second := make([]float32, 128)
	second[0], second[5] = 0.9, float32(math.Sqrt(1-0.81))
	orders := map[string][]FaceDetection{
		"first listed first":  {goodFace(600, 100, first), goodFace(100, 100, second)},
		"second listed first": {goodFace(100, 100, second), goodFace(600, 100, first)},
	}
	want := []FaceSpan{
		{Start: 0, End: 3, X: 140.0 / FaceFrameSide, Y: 140.0 / FaceFrameSide, W: 80.0 / FaceFrameSide},
		{Start: 0, End: 3, X: 640.0 / FaceFrameSide, Y: 140.0 / FaceFrameSide, W: 80.0 / FaceFrameSide},
	}
	for name, frame := range orders {
		t.Run(name, func(t *testing.T) {
			k := NewFaceTracker(FaceThresholds{Track: 0.45, TrackFar: 0.6})
			for range 3 {
				k.Add([]FaceDetection{goodFace(100, 100, a), goodFace(600, 100, b)})
			}
			k.Add(frame)
			tracks := k.Tracks()
			if len(tracks) != len(want) {
				t.Fatalf("got %d tracks, want A and B continued", len(tracks))
			}
			for i, w := range want {
				if tracks[i].FaceSpan != w {
					t.Errorf("track %d = %+v, want %+v", i, tracks[i].FaceSpan, w)
				}
			}
		})
	}
}
