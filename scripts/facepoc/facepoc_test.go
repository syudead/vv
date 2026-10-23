package main

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestSimilarityRecoversRotationScaleAndShift(t *testing.T) {
	// dst = 2 * R(30°) * src + (5, -3)
	cs, sn := 2*math.Cos(math.Pi/6), 2*math.Sin(math.Pi/6)
	src := [10]float64{0, 0, 10, 0, 5, 5, 2, 9, 8, 9}
	var dst [10]float64
	for i := 0; i < 10; i += 2 {
		dst[i] = cs*src[i] - sn*src[i+1] + 5
		dst[i+1] = sn*src[i] + cs*src[i+1] - 3
	}
	m := similarity(src, dst)
	for i := 0; i < 10; i += 2 {
		x, y := m.apply(src[i], src[i+1])
		near(t, "x", x, dst[i])
		near(t, "y", y, dst[i+1])
		bx, by := m.inverse().apply(x, y)
		near(t, "inverse x", bx, src[i])
		near(t, "inverse y", by, src[i+1])
	}
}

// yunetOutput builds the twelve outputs with one anchor of stride 16 set at (col 3, row 2).
func yunetOutput(cls, obj float32) []tensor {
	out := make([]tensor, 12)
	for si, s := range yunetStrides {
		cells := (yunetSize / s) * (yunetSize / s)
		out[si] = tensor{data: make([]float32, cells)}
		out[3+si] = tensor{data: make([]float32, cells)}
		out[6+si] = tensor{data: make([]float32, cells*4)}
		out[9+si] = tensor{data: make([]float32, cells*10)}
	}
	idx := 2*(yunetSize/16) + 3
	out[1].data[idx], out[4].data[idx] = cls, obj
	copy(out[7].data[idx*4:], []float32{0.5, 0.25, float32(math.Log(4)), float32(math.Log(2))})
	copy(out[10].data[idx*10:], []float32{0, 0, 1, 0, 0.5, 0.5, 0, 1, 1, 1})
	return out
}

func TestDecodeYuNetPlacesAnchorFace(t *testing.T) {
	faces := decodeYuNet(yunetOutput(0.9, 0.4), 0.5)
	if len(faces) != 1 {
		t.Fatalf("got %d faces, want 1", len(faces))
	}
	f := faces[0]
	near(t, "score", f.score, 0.6) // sqrt(0.9 * 0.4)
	// centre (3.5, 2.25) * 16 = (56, 36); size (4, 2) * 16 = (64, 32)
	near(t, "x", f.x, 24)
	near(t, "y", f.y, 20)
	near(t, "w", f.w, 64)
	near(t, "h", f.h, 32)
	want := [10]float64{48, 32, 64, 32, 56, 40, 48, 48, 64, 48}
	for k := range want {
		near(t, "kps", f.kps[k], want[k])
	}
}

func TestDecodeYuNetDropsLowScores(t *testing.T) {
	if faces := decodeYuNet(yunetOutput(0.9, 0.2), 0.5); len(faces) != 0 {
		t.Fatalf("score sqrt(0.18) kept: %+v", faces)
	}
}

func TestNMSDropsOverlapsKeepsSeparateFaces(t *testing.T) {
	faces := nms([]face{
		{x: 0, y: 0, w: 10, h: 10, score: 0.7},
		{x: 1, y: 1, w: 10, h: 10, score: 0.9},
		{x: 50, y: 50, w: 10, h: 10, score: 0.6},
	}, 0.3)
	if len(faces) != 2 || faces[0].score != 0.9 || faces[1].score != 0.6 {
		t.Fatalf("got %+v, want the 0.9 and 0.6 faces", faces)
	}
}

// frontal is a large, confident face whose nose sits midway between the eyes.
func frontal() face {
	return face{w: 80, h: 80, score: 0.95, kps: [10]float64{30, 40, 50, 40, 40, 50, 32, 60, 48, 60}}
}

func TestIsGoodRejectsProfileSmallAndWeakFaces(t *testing.T) {
	profile := frontal()
	profile.kps[4] = 49 // nose next to the right eye
	small := frontal()
	small.w = 40
	weak := frontal()
	weak.score = 0.7
	for name, tc := range map[string]struct {
		f    face
		want bool
	}{"frontal": {frontal(), true}, "profile": {profile, false}, "small": {small, false}, "weak": {weak, false}} {
		if got := isGood(tc.f); got != tc.want {
			t.Errorf("%s: isGood = %v, want %v", name, got, tc.want)
		}
	}
}

// unit returns a vector along axis a, tilted slightly by axis b so tracks of one person differ.
func unit(a, b int, tilt float32) []float32 {
	v := make([]float32, sfaceDim)
	v[a], v[b] = 1, tilt
	return normalize(v)
}

func fakeTrack(vec []float32, good int) *track {
	t := &track{}
	for i := range good {
		t.times = append(t.times, float64(i)/sampleFPS)
		t.vecs = append(t.vecs, vec)
		t.good = append(t.good, true)
	}
	return t
}

func TestMainPeopleKeepsLongClustersApart(t *testing.T) {
	// Person A: 3 tracks x 6 good frames = 9s; person B: 2 tracks x 10 = 10s;
	// person C: 2 tracks x 4 = 4s, under the cut-off.
	var tracks []*track
	for i := range 3 {
		tracks = append(tracks, fakeTrack(unit(0, 10+i, 0.2), 6))
	}
	for i := range 2 {
		tracks = append(tracks, fakeTrack(unit(1, 20+i, 0.2), 10))
	}
	for i := range 2 {
		tracks = append(tracks, fakeTrack(unit(2, 30+i, 0.2), 4))
	}
	people := mainPeople(tracks)
	if len(people) != 2 {
		t.Fatalf("got %d people, want 2 (A and B)", len(people))
	}
	near(t, "first person seconds", people[0].goodSec, 10)
	near(t, "second person seconds", people[1].goodSec, 9)
	if len(people[0].tracks) != 2 || len(people[1].tracks) != 3 {
		t.Fatalf("track counts %d, %d; want 2, 3", len(people[0].tracks), len(people[1].tracks))
	}
}

func TestTrackVectorIgnoresBadDetections(t *testing.T) {
	tr := fakeTrack(unit(0, 1, 0), 2)
	tr.vecs = append(tr.vecs, unit(5, 6, 0))
	tr.good = append(tr.good, false)
	if got := dot(tr.vector(), unit(0, 1, 0)); math.Abs(got-1) > 1e-6 {
		t.Fatalf("vector includes the bad detection: cos to good = %v", got)
	}
	single := fakeTrack(unit(0, 1, 0), 1)
	if single.vector() != nil {
		t.Fatal("a track with one good detection has a vector")
	}
}
