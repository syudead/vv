package main

import (
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

const (
	recurringHolds  = 9
	recurringHoldMs = 3_000
)

// recurringShotFixture renders a held shot that returns nine times, 3 s each, with 3 s of a different stretch of the
// pan between, a keyframe every second; hold k spans [6k, 6k+3) s of the 54 s video.
func recurringShotFixture(t *testing.T, dir string) string {
	t.Helper()
	work := t.TempDir()
	pan := panFixture(t, work)
	still, video := filepath.Join(work, "shot.png"), filepath.Join(dir, "recurring.mp4")
	ffmpegFixture(t, "-f", "lavfi", "-i", "testsrc=s=320x180:r=1", "-frames:v", "1", still)
	var graph, order strings.Builder
	fmt.Fprintf(&graph, "[1:v]format=yuv420p,setsar=1,split=%d", recurringHolds)
	for k := range recurringHolds {
		fmt.Fprintf(&graph, "[a%d]", k)
	}
	fmt.Fprintf(&graph, ";[0:v]split=%d", recurringHolds)
	for k := range recurringHolds {
		fmt.Fprintf(&graph, "[p%d]", k)
	}
	for k := range recurringHolds {
		fmt.Fprintf(&graph, ";[p%d]trim=start=%d:duration=3,setpts=PTS-STARTPTS,format=yuv420p,setsar=1[m%d]", k, 3*k, k)
		fmt.Fprintf(&order, "[a%d][m%d]", k, k)
	}
	fmt.Fprintf(&graph, ";%sconcat=n=%d:v=1:a=0[out]", order.String(), 2*recurringHolds)
	ffmpegFixture(t, "-i", pan, "-loop", "1", "-framerate", "25", "-t", "3", "-i", still,
		"-filter_complex", graph.String(), "-map", "[out]", "-r", "25", "-c:v", "libx264", "-preset", "veryfast",
		"-crf", "23", "-g", "25", "-keyint_min", "25", "-sc_threshold", "0", "-pix_fmt", "yuv420p", video)
	return video
}

// cutFrame returns the frame of video at the given time as a PNG.
func cutFrame(t *testing.T, video, at string) []byte {
	t.Helper()
	frame, err := exec.Command("ffmpeg", "-v", "error", "-ss", at, "-i", video, "-frames:v", "1",
		"-f", "image2pipe", "-c:v", "png", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

var (
	imageBands = []gen.ImageSearchBand{
		{Id: gen.Strong, MinPercent: 90, MaxPercent: 100}, {Id: gen.Similar, MinPercent: 81, MaxPercent: 89},
	}
	videoBands = []gen.ImageSearchBand{
		{Id: gen.Strong, MinPercent: 90, MaxPercent: 100}, {Id: gen.Similar, MinPercent: 50, MaxPercent: 89},
	}
)

// assertBands checks that the answer lists the bands of its kind and that each item's band holds its rate rounded to
// a whole percent, halves up.
func assertBands(t *testing.T, result gen.ImageSearchResult, want []gen.ImageSearchBand) {
	t.Helper()
	if !slices.Equal(result.Bands, want) {
		t.Fatalf("bands = %+v, want %+v", result.Bands, want)
	}
	for _, item := range result.Items {
		percent := int(math.Floor(item.Match.Rate*100 + 0.5))
		i := slices.IndexFunc(want, func(b gen.ImageSearchBand) bool { return b.Id == item.Match.Band })
		if i < 0 || percent < want[i].MinPercent || percent > want[i].MaxPercent {
			t.Fatalf("video %d at %d%% is in band %q of %+v", item.Video.Id, percent, item.Match.Band, want)
		}
	}
}

// A frame 1.5 s into a held shot that returns nine times answers one row with one frame inside each appearance,
// best first.
func TestImageSearchReturnsAFramePerAppearanceOfARecurringShot(t *testing.T) {
	fixture := newImageSearchFixture(t)
	video := recurringShotFixture(t, fixture.mediaDir)
	target := fixture.indexVideo(t, video, 2*recurringHolds*recurringHoldMs)

	result := postSearchFile(t, fixture.handler, "shot.png", cutFrame(t, video, "25.5"))

	assertBands(t, result, imageBands)
	if len(result.Items) != 1 || result.Items[0].Video.Id != target {
		t.Fatalf("items = %+v, want only video %d", result.Items, target)
	}
	match := result.Items[0].Match
	if len(match.Frames) != recurringHolds || match.FrameCount != recurringHolds {
		t.Fatalf("frames = %+v (count %d), want %d", match.Frames, match.FrameCount, recurringHolds)
	}
	if match.Frames[0].Rate != match.Rate || match.Frames[0].PositionMs != match.PositionMs {
		t.Fatalf("frames[0] = %+v, want the match's rate %v at %d ms", match.Frames[0], match.Rate, match.PositionMs)
	}
	holds := map[int64]bool{}
	for i, frame := range match.Frames {
		hold := frame.PositionMs / (2 * recurringHoldMs)
		if frame.PositionMs-hold*2*recurringHoldMs >= recurringHoldMs {
			t.Fatalf("frame %+v is outside every hold", frame)
		}
		holds[hold] = true
		if prev := match.Frames[max(i-1, 0)]; prev.Rate < frame.Rate ||
			prev.Rate == frame.Rate && prev.PositionMs > frame.PositionMs {
			t.Fatalf("frames %+v are not best first, then earlier", match.Frames)
		}
	}
	if len(holds) != recurringHolds {
		t.Fatalf("frames %+v cover %d holds, want one in each of %d", match.Frames, len(holds), recurringHolds)
	}
}
