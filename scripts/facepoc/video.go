package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os/exec"
)

// Sampling and track-linking constants measured in the Python POC.
const (
	sampleFPS   = 2.0
	frameSide   = 1280 // longer side of a sampled frame
	minFaceSide = 48.0
	trackGap    = 2    // sampled frames a track may miss before it closes
	linkSim     = 0.45 // SFace similarity needed to continue a track
	linkSimFar  = 0.6  // ... when the boxes no longer overlap
	minTrackLen = 3
)

// Quality gate for the detections a track's vector is built from.
const (
	goodFaceSide = 64.0
	goodScore    = 0.85
	maxYawOffset = 0.22 // nose within 0.5±this of the way between the eyes
)

// track is one face followed across sampled frames of one video.
type track struct {
	video     string
	times     []float64
	faces     []face
	vecs      [][]float32
	good      []bool
	thumb     bgrImage
	thumbArea float64
	last      int
}

func (t *track) goodCount() int {
	n := 0
	for _, g := range t.good {
		if g {
			n++
		}
	}
	return n
}

// vector is the normalised mean of the track's good detections, or nil when fewer than two are good.
func (t *track) vector() []float32 {
	if t.goodCount() < 2 {
		return nil
	}
	sum := make([]float32, sfaceDim)
	for i, v := range t.vecs {
		if !t.good[i] {
			continue
		}
		for k := range sum {
			sum[k] += v[k]
		}
	}
	return normalize(sum)
}

func isGood(f face) bool {
	return f.w >= goodFaceSide && f.score >= goodScore && math.Abs(yawRatio(f)-0.5) <= maxYawOffset
}

// indexVideo samples the video, detects and embeds faces and links them into tracks.
func indexVideo(m *models, path string) ([]*track, int, error) {
	frames, wait, err := ffmpegFrames(path)
	if err != nil {
		return nil, 0, err
	}
	var active, done []*track
	n := 0
	for img := range frames {
		faces, err := m.det.detect(img)
		if err != nil {
			return nil, 0, err
		}
		for i, t := range active {
			if n-t.last > trackGap {
				done = append(done, t)
				active[i] = nil
			}
		}
		active = compact(active)
		if active, err = linkFaces(m, path, img, n, faces, active); err != nil {
			return nil, 0, err
		}
		n++
	}
	if err := wait(); err != nil {
		return nil, 0, err
	}
	var kept []*track
	for _, t := range append(done, active...) {
		if len(t.times) >= minTrackLen {
			kept = append(kept, t)
		}
	}
	return kept, n, nil
}

func compact(ts []*track) []*track {
	out := ts[:0]
	for _, t := range ts {
		if t != nil {
			out = append(out, t)
		}
	}
	return out
}

// linkFaces appends each large-enough face to the most similar open track, or starts a new one.
func linkFaces(m *models, path string, img bgrImage, n int, faces []face, active []*track) ([]*track, error) {
	used := map[*track]bool{}
	for _, f := range faces {
		if f.w < minFaceSide {
			continue
		}
		v, err := m.rec.embed(img, f)
		if err != nil {
			return nil, err
		}
		t := bestTrack(active, used, f, v)
		if t == nil {
			t = &track{video: path}
			active = append(active, t)
		}
		used[t] = true
		t.times = append(t.times, float64(n)/sampleFPS)
		t.faces, t.vecs, t.good, t.last = append(t.faces, f), append(t.vecs, v), append(t.good, isGood(f)), n
		if area := f.w * f.h * f.score; area > t.thumbArea {
			t.thumb, t.thumbArea = cropThumb(img, f), area
		}
	}
	return active, nil
}

func bestTrack(active []*track, used map[*track]bool, f face, v []float32) *track {
	var best *track
	bestSim := 0.0
	for _, t := range active {
		if used[t] {
			continue
		}
		sim := dot(v, t.vecs[len(t.vecs)-1])
		near := boxIoU(f, t.faces[len(t.faces)-1]) > 0.05
		if sim > linkSim && (near || sim > linkSimFar) && sim > bestSim {
			best, bestSim = t, sim
		}
	}
	return best
}

// ffmpegFrames decodes the video at sampleFPS into frameSide x frameSide frames: ffmpeg rotates by the
// display matrix first, then the frame is fitted without distortion and padded with black.
func ffmpegFrames(path string) (<-chan bgrImage, func() error, error) {
	vf := fmt.Sprintf("fps=%g,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d",
		sampleFPS, frameSide, frameSide, frameSide, frameSide)
	cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-vf", vf, "-f", "rawvideo", "-pix_fmt", "bgr24", "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	ch := make(chan bgrImage)
	go func() {
		defer close(ch)
		r := bufio.NewReaderSize(stdout, 1<<20)
		for {
			buf := make([]byte, frameSide*frameSide*3)
			if _, err := io.ReadFull(r, buf); err != nil {
				return
			}
			ch <- bgrImage{w: frameSide, h: frameSide, pix: buf}
		}
	}()
	return ch, cmd.Wait, nil
}

// cropThumb cuts a 96x96 square around the face with a quarter-side margin.
func cropThumb(img bgrImage, f face) bgrImage {
	const side = 96
	pad := 0.25 * math.Max(f.w, f.h)
	x0, y0, span := f.x-pad, f.y-pad, math.Max(f.w, f.h)+2*pad
	out := bgrImage{w: side, h: side, pix: make([]byte, side*side*3)}
	for y := range side {
		for x := range side {
			b, g, r := sampleBGR(img, x0+(float64(x)+0.5)*span/side, y0+(float64(y)+0.5)*span/side)
			i := (y*side + x) * 3
			out.pix[i], out.pix[i+1], out.pix[i+2] = byte(b), byte(g), byte(r)
		}
	}
	return out
}
