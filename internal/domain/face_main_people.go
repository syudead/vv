package domain

import (
	"cmp"
	"math"
	"slices"
)

// Sampling and main-person constants, measured together in the POC; a setting could break their balance unseen.
const (
	// FaceSampleFPS is the rate at which frames of a video are analysed.
	FaceSampleFPS = 1.0
	// FaceFrameSide is the side of the square every sampled frame is fitted into, in pixels.
	FaceFrameSide = 1280

	faceMinSide     = 48.0 // smaller faces are not embedded or tracked
	faceTrackGap    = 2    // a track closes once it misses this many sampled frames in a row
	faceTrackMinLen = 2    // shorter tracks are noise
	faceNearIoU     = 0.05 // boxes overlapping more than this are near for tracking

	// The quality gate for the faces a track's vector and good seconds count.
	faceGoodSide      = 64.0
	faceGoodScore     = 0.85
	faceMaxYawOffset  = 0.22 // nose within 0.5±this of the way between the eyes
	faceMinGoodFrames = 2    // a track needs this many good faces for a vector

	// A cluster of tracks is a main person with min(faceMainMinSeconds, bar) good seconds, where the bar is
	// faceMainFraction of the analysed length but at least faceMainFloorSeconds. A cluster of one track must reach
	// the bar itself, so one brief track in a long video does not count.
	faceMainMinSeconds   = 8.0
	faceMainFraction     = 0.25
	faceMainFloorSeconds = 3.0
)

// FaceBox is a detected face in the pixels of a FaceFrameSide frame.
type FaceBox struct {
	X, Y, W, H float64
}

func (b FaceBox) centre() (float64, float64) {
	return b.X + b.W/2, b.Y + b.H/2
}

func (b FaceBox) iou(o FaceBox) float64 {
	iw := math.Min(b.X+b.W, o.X+o.W) - math.Max(b.X, o.X)
	ih := math.Min(b.Y+b.H, o.Y+o.H) - math.Max(b.Y, o.Y)
	if iw <= 0 || ih <= 0 {
		return 0
	}
	inter := iw * ih
	return inter / (b.W*b.H + o.W*o.H - inter)
}

// FaceDetection is one face found in one sampled frame.
type FaceDetection struct {
	Box FaceBox
	// Landmarks are the eyes, the nose and the mouth corners as x, y pairs, image-left first.
	Landmarks [10]float64
	Score     float64
	// Vector is the recognizer's normalized vector; faces smaller than the minimum side need none.
	Vector []float32
	// Cut marks a face whose box reaches past the edge of the picture, so a crop of it shows part of a face.
	Cut bool
}

// good reports whether the face is large, confident and frontal enough to count.
func (d FaceDetection) good() bool {
	return d.Box.W >= faceGoodSide && d.Score >= faceGoodScore && math.Abs(d.yawRatio()-0.5) <= faceMaxYawOffset
}

// yawRatio is where the nose sits between the eyes horizontally: 0.5 for a frontal face.
func (d FaceDetection) yawRatio() float64 {
	span := d.Landmarks[2] - d.Landmarks[0]
	if math.Abs(span) < 1e-6 {
		return math.Inf(1)
	}
	return (d.Landmarks[4] - d.Landmarks[0]) / span
}

// FaceSighting locates one face: the sampled frame's index and its box.
type FaceSighting struct {
	Frame int
	Box   FaceBox
}

// FaceSpan is what is kept of a track: where and when one face was on screen.
type FaceSpan struct {
	// Start and End are the seconds of the first and last sampled frame.
	Start, End float64
	// X, Y and W are the mean face centre and width as fractions of the frame side.
	X, Y, W float64
}

// FaceTrack is one face followed across sampled frames of one video.
type FaceTrack struct {
	FaceSpan
	// GoodFrames counts the faces that pass the quality gate.
	GoodFrames int
	// Best is the good face for a crop: one inside the picture before any cut by its edge, then the largest box
	// times score; zero when none is good.
	Best FaceSighting

	frames    int
	sumX      float64
	sumY      float64
	sumW      float64
	goodSum   []float64
	best      faceRank
	last      FaceDetection
	lastFrame int
}

func (t *FaceTrack) add(frame int, d FaceDetection) {
	if t.frames == 0 {
		t.Start = float64(frame) / FaceSampleFPS
	}
	t.frames++
	t.End = float64(frame) / FaceSampleFPS
	cx, cy := d.Box.centre()
	t.sumX, t.sumY, t.sumW = t.sumX+cx, t.sumY+cy, t.sumW+d.Box.W
	t.X, t.Y, t.W = t.sumX/float64(t.frames)/FaceFrameSide, t.sumY/float64(t.frames)/FaceFrameSide, t.sumW/float64(t.frames)/FaceFrameSide
	t.last, t.lastFrame = d, frame
	if !d.good() {
		return
	}
	t.GoodFrames++
	if t.goodSum == nil {
		t.goodSum = make([]float64, len(d.Vector))
	}
	for i, x := range d.Vector {
		t.goodSum[i] += float64(x)
	}
	if r := (faceRank{whole: !d.Cut, value: d.Box.W * d.Box.H * d.Score}); r.beats(t.best) {
		t.Best, t.best = FaceSighting{Frame: frame, Box: d.Box}, r
	}
}

// faceRank orders the good faces for a crop; the zero value loses to any face.
type faceRank struct {
	whole bool
	value float64
}

func (r faceRank) beats(o faceRank) bool {
	if r.whole != o.whole {
		return r.whole
	}
	return r.value > o.value
}

// vector is the normalized mean of the good faces, or nil when too few are good.
func (t *FaceTrack) vector() []float32 {
	if t.GoodFrames < faceMinGoodFrames {
		return nil
	}
	v := make([]float32, len(t.goodSum))
	for i, x := range t.goodSum {
		v[i] = float32(x)
	}
	return NormalizeFaceVector(v)
}

// FaceTracker links the faces of consecutive sampled frames into tracks, one frame at a time.
type FaceTracker struct {
	th     FaceThresholds
	frame  int
	active []*FaceTrack
	done   []*FaceTrack
}

func NewFaceTracker(th FaceThresholds) *FaceTracker {
	return &FaceTracker{th: th}
}

// Add takes the faces of the next sampled frame.
func (k *FaceTracker) Add(faces []FaceDetection) {
	open := k.active[:0]
	for _, t := range k.active {
		if k.frame-t.lastFrame > faceTrackGap {
			k.done = append(k.done, t)
		} else {
			open = append(open, t)
		}
	}
	k.active = open
	var kept []FaceDetection
	for _, d := range faces {
		if d.Box.W >= faceMinSide && len(d.Vector) > 0 {
			kept = append(kept, d)
		}
	}
	tracks := k.assign(kept)
	for i, d := range kept {
		t := tracks[i]
		if t == nil {
			t = &FaceTrack{}
			k.active = append(k.active, t)
		}
		t.add(k.frame, d)
	}
	k.frame++
}

// assign gives each face the open track it continues, or nil; the most similar pairs go first, so the detector's face order does not decide a contested track.
func (k *FaceTracker) assign(faces []FaceDetection) []*FaceTrack {
	type pair struct {
		face, track int
		sim         float64
	}
	var pairs []pair
	for fi, d := range faces {
		for ti, t := range k.active {
			sim := FaceSimilarity(d.Vector, t.last.Vector)
			near := d.Box.iou(t.last.Box) > faceNearIoU
			if sim > k.th.Track && (near || sim > k.th.TrackFar) {
				pairs = append(pairs, pair{fi, ti, sim})
			}
		}
	}
	slices.SortStableFunc(pairs, func(a, b pair) int { return cmp.Compare(b.sim, a.sim) })
	out := make([]*FaceTrack, len(faces))
	taken := make([]bool, len(k.active))
	for _, p := range pairs {
		if out[p.face] == nil && !taken[p.track] {
			out[p.face], taken[p.track] = k.active[p.track], true
		}
	}
	return out
}

// Tracks returns the tracks long enough to keep, in the order they started.
func (k *FaceTracker) Tracks() []FaceTrack {
	all := append(append([]*FaceTrack(nil), k.done...), k.active...)
	var out []FaceTrack
	for _, t := range all {
		if t.frames >= faceTrackMinLen {
			out = append(out, *t)
		}
	}
	slices.SortStableFunc(out, func(a, b FaceTrack) int { return cmp.Compare(a.Start, b.Start) })
	return out
}

// BestSightings returns the best good face of every track that may still become part of a main person: the open
// tracks and the closed tracks long enough to keep with a vector. A caller keeps crops only for these.
func (k *FaceTracker) BestSightings() []FaceSighting {
	var out []FaceSighting
	for _, t := range k.active {
		if t.GoodFrames > 0 {
			out = append(out, t.Best)
		}
	}
	for _, t := range k.done {
		if t.frames >= faceTrackMinLen && t.GoodFrames >= faceMinGoodFrames {
			out = append(out, t.Best)
		}
	}
	return out
}

// MainPerson is one cluster of a video's tracks that passed the main-person cutoff.
type MainPerson struct {
	Tracks      []FaceTrack
	GoodSeconds float64
	// Centroid is the normalized sum of the tracks' vectors.
	Centroid []float32
	// Best is the best good face of the cluster's tracks, for its crop.
	Best FaceSighting
}

// FindMainPeople clusters the tracks that have a vector and keeps clusters with enough good face time, longest
// first. analysedSeconds is the length of the frames actually sampled. Kept clusters whose centroids reach the
// join threshold are one person the clustering split, and are merged; the looser candidate threshold only offers a pair.
func FindMainPeople(th FaceThresholds, tracks []FaceTrack, analysedSeconds float64) []MainPerson {
	var kept []FaceTrack
	var vecs [][]float32
	for _, t := range tracks {
		if v := t.vector(); v != nil {
			kept, vecs = append(kept, t), append(vecs, v)
		}
	}
	var clusters [][]int
	for _, c := range averageLinkage(vecs, th.Cluster) {
		if isMainPerson(mainPerson(kept, vecs, c), analysedSeconds) {
			clusters = append(clusters, c)
		}
	}
	people := make([]MainPerson, 0, len(clusters))
	for _, c := range mergeSplitClusters(kept, vecs, clusters, th.Join) {
		people = append(people, mainPerson(kept, vecs, c))
	}
	slices.SortStableFunc(people, func(a, b MainPerson) int { return cmp.Compare(b.GoodSeconds, a.GoodSeconds) })
	return people
}

// isMainPerson applies the length-aware cutoff.
func isMainPerson(p MainPerson, analysedSeconds float64) bool {
	bar := math.Max(faceMainFloorSeconds, faceMainFraction*analysedSeconds)
	if len(p.Tracks) >= 2 {
		bar = math.Min(bar, faceMainMinSeconds)
	}
	return p.GoodSeconds >= bar
}

// mergeSplitClusters joins the clusters whose centroids are the most similar while any pair reaches minSim.
func mergeSplitClusters(tracks []FaceTrack, vecs [][]float32, clusters [][]int, minSim float64) [][]int {
	for {
		centroids := make([][]float32, len(clusters))
		for i, c := range clusters {
			centroids[i] = mainPerson(tracks, vecs, c).Centroid
		}
		bi, bj, best := -1, -1, minSim
		for i := range clusters {
			for j := i + 1; j < len(clusters); j++ {
				if s := FaceSimilarity(centroids[i], centroids[j]); s >= best {
					bi, bj, best = i, j, s
				}
			}
		}
		if bi < 0 {
			return clusters
		}
		clusters[bi] = append(clusters[bi], clusters[bj]...)
		clusters = append(clusters[:bj], clusters[bj+1:]...)
	}
}

func mainPerson(tracks []FaceTrack, vecs [][]float32, members []int) MainPerson {
	p := MainPerson{}
	sum := make([]float32, len(vecs[members[0]]))
	var best faceRank
	for _, i := range members {
		t := tracks[i]
		p.Tracks = append(p.Tracks, t)
		p.GoodSeconds += float64(t.GoodFrames) / FaceSampleFPS
		for k, x := range vecs[i] {
			sum[k] += x
		}
		if t.best.beats(best) {
			p.Best, best = t.Best, t.best
		}
	}
	slices.SortStableFunc(p.Tracks, func(a, b FaceTrack) int { return cmp.Compare(a.Start, b.Start) })
	p.Centroid = NormalizeFaceVector(sum)
	return p
}

// Spans returns the spans of the person's tracks, in the order they started.
func (p MainPerson) Spans() []FaceSpan {
	out := make([]FaceSpan, len(p.Tracks))
	for i, t := range p.Tracks {
		out[i] = t.FaceSpan
	}
	return out
}

// averageLinkage merges the two clusters with the highest mean pairwise similarity until no pair reaches minSim.
func averageLinkage(vecs [][]float32, minSim float64) [][]int {
	clusters := make([][]int, len(vecs))
	for i := range vecs {
		clusters[i] = []int{i}
	}
	sim := make([][]float64, len(vecs))
	for i := range vecs {
		sim[i] = make([]float64, len(vecs))
		for j := range vecs {
			sim[i][j] = FaceSimilarity(vecs[i], vecs[j])
		}
	}
	for {
		bi, bj, best := -1, -1, minSim
		for i := range clusters {
			for j := i + 1; j < len(clusters); j++ {
				if s := meanSimilarity(sim, clusters[i], clusters[j]); s >= best {
					bi, bj, best = i, j, s
				}
			}
		}
		if bi < 0 {
			return clusters
		}
		clusters[bi] = append(clusters[bi], clusters[bj]...)
		clusters = append(clusters[:bj], clusters[bj+1:]...)
	}
}

func meanSimilarity(sim [][]float64, a, b []int) float64 {
	var s float64
	for _, i := range a {
		for _, j := range b {
			s += sim[i][j]
		}
	}
	return s / float64(len(a)*len(b))
}
