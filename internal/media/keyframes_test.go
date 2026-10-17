package media

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func TestClassifyKeyframePackets(t *testing.T) {
	for _, tc := range []struct {
		name, packets string
		want          domain.KeyframeSpacing
	}{
		{"sparse without next keyframe", "pts_time=0|flags=K_\npts_time=15.04|flags=__", domain.KeyframesSparse},
		{"short", "pts_time=0|flags=K_\npts_time=2|flags=K_\npts_time=4|flags=K_", domain.KeyframesShort},
		{"boundary", "pts_time=20|flags=K_\npts_time=35|flags=K_", domain.KeyframesShort},
		{"nonzero start", "pts_time=3600|flags=K_\npts_time=3616|flags=__", domain.KeyframesSparse},
		{"short then sparse", "pts_time=0|flags=K_\npts_time=0.2|flags=K_\npts_time=15.8|flags=__", domain.KeyframesSparse},
		{"B frame order", "pts_time=0|flags=K_\npts_time=0.12|flags=__\npts_time=0.04|flags=__\npts_time=2|flags=K_", domain.KeyframesShort},
		{"no keyframes", "pts_time=0|flags=__\npts_time=16|flags=__", domain.KeyframesUnknown},
		{"short tail only", "pts_time=0|flags=K_\npts_time=10|flags=__", domain.KeyframesUnknown},
		{"missing timestamps", "pts_time=N/A|flags=K_", domain.KeyframesUnknown},
		{"nonfinite", "pts_time=NaN|flags=K_", domain.KeyframesUnknown},
		{"reset", "pts_time=1|flags=K_\npts_time=0|flags=K_", domain.KeyframesUnknown},
		{"malformed tail", "pts_time=0|flags=K_\npts_time=16|flags=__\ninvalid", domain.KeyframesUnknown},
		{"empty", "", domain.KeyframesUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyKeyframePackets(tc.packets); got != tc.want {
				t.Fatalf("spacing = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProbeKeyframesRealMKV(t *testing.T) {
	for _, command := range []string{probeCommand, transcodeCommand} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skip(err)
		}
	}
	for _, tc := range []struct {
		interval int
		want     domain.KeyframeSpacing
	}{
		{60, domain.KeyframesSparse}, {2, domain.KeyframesShort},
	} {
		t.Run(string(tc.want), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sample.mkv")
			cmd := exec.Command(transcodeCommand, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=25:duration=18",
				"-c:v", "libx264", "-preset", "ultrafast", "-g", fmt.Sprint(tc.interval*25), "-sc_threshold", "0", path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("generate: %v: %s", err, output)
			}
			probe, err := Probe(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if probe.Keyframes == nil || probe.Keyframes.Source != probe.Source {
				t.Fatalf("keyframes = %+v, want %s", probe.Keyframes, tc.want)
			}
			// The production 500 ms budget may expire on a busy test runner. Verify
			// packet interpretation separately with enough scheduling time.
			sampled := probeKeyframes(context.Background(), path, 5*time.Second)
			if sampled.Spacing != tc.want {
				t.Fatalf("sample = %+v, want %s", sampled, tc.want)
			}
			probe.Keyframes = &sampled
			transcoder := NewLiveTranscoder(nil)
			var codecs []string
			transcoder.commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				for i, arg := range args {
					if arg == "-c:v" {
						codecs = append(codecs, args[i+1])
					}
				}
				return command(ctx, name, args...)
			}
			started, err := transcoder.Start(context.Background(), domain.LiveTranscodeRequest{
				Path: path, Source: probe.Source, Probe: probe.Transcode, StartMs: 17000,
				SparseKeyframes: probe.Keyframes.SparseFor(probe.Source), StartupDeadline: time.Now().Add(6 * time.Second),
			})
			if err != nil {
				t.Fatal(err)
			}
			finishStarted(t, started)
			wantCodec := "copy"
			if tc.want == domain.KeyframesSparse {
				wantCodec = "libx264"
			}
			if !slices.Equal(codecs, []string{wantCodec}) {
				t.Fatalf("actual FFmpeg codecs = %v, want only %s", codecs, wantCodec)
			}
			if tc.want == domain.KeyframesSparse && started.StartMs != 17000 {
				t.Fatalf("encoded start = %d", started.StartMs)
			}
			if got := probeKeyframes(context.Background(), path, time.Nanosecond); got.Spacing != domain.KeyframesUnknown {
				t.Fatalf("timed out = %+v", got)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if got := ProbeKeyframes(ctx, path); got.Spacing != domain.KeyframesUnknown {
				t.Fatalf("cancelled = %+v", got)
			}
		})
	}
}

func TestLiveTranscoderSparseKeyframes(t *testing.T) {
	path, stamp := sourceFile(t)
	stored := helperProbe()
	for _, tc := range []struct {
		name              string
		start             int64
		sparse, normalize bool
		want              string
	}{
		{"sparse seek", 290000, true, false, "libx264"},
		{"ordinary seek", 290000, false, false, "copy"},
		{"sparse beginning", 0, true, false, "copy"},
		{"normalized seek", 290000, true, true, "libx264"},
		{"normalized beginning", 0, true, true, "libx264"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scripted := newScriptedTranscoder("ffmpeg-mp4:289000")
			started, err := scripted.Start(context.Background(), domain.LiveTranscodeRequest{
				Path: path, Source: stamp, Probe: &stored, StartMs: tc.start,
				SparseKeyframes: tc.sparse, Normalize: tc.normalize, StartupDeadline: time.Now().Add(5 * time.Second),
			})
			if err != nil {
				t.Fatal(err)
			}
			finishStarted(t, started)
			if got := scripted.videoCodecs(); !slices.Equal(got, []string{tc.want}) {
				t.Fatalf("ffmpeg video codecs = %v, want only %s", got, tc.want)
			}
		})
	}
}
