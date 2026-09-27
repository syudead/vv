package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestAppendSeekFramesRejectsMissingAndCorruptCells(t *testing.T) {
	header := "YUV4MPEG2 W2 H2 F1:1 Ip A1:1 C420jpeg XCOLORRANGE=FULL"
	frame := append([]byte("FRAME\n"), make([]byte, 6)...)
	valid := append([]byte(header+"\n"), frame...)
	for _, tc := range []struct {
		name     string
		data     []byte
		count    int
		previous string
	}{
		{"missing-cell", valid, 2, ""},
		{"extra-cell", append(append([]byte{}, valid...), frame...), 1, ""},
		{"truncated", valid[:len(valid)-1], 1, ""},
		{"no-header", frame, 1, ""},
		{"unsupported-pixels", bytes.ReplaceAll(valid, []byte("C420jpeg"), []byte("C420p10")), 1, ""},
		{"oversized", bytes.ReplaceAll(valid, []byte("W2 "), []byte("W322 ")), 1, ""},
		{"range-lost", bytes.ReplaceAll(valid, []byte("XCOLORRANGE=FULL"), []byte("XCOLORRANGE=LIMITED")), 1, ""},
		{"changed-shape", valid, 1, strings.ReplaceAll(header, "W2 ", "W4 ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if _, err := appendSeekFrames(&out, tc.data, tc.previous, tc.count); err == nil {
				t.Fatal("invalid frame batch accepted")
			}
			if out.Len() != 0 {
				t.Fatal("incomplete batch was appended")
			}
		})
	}
	var out bytes.Buffer
	first, err := appendSeekFrames(&out, valid, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appendSeekFrames(&out, valid, first, 1); err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte{}, valid...), frame...)
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatal("YUV header repeated or frame bytes changed")
	}
}

func TestSeekSpriteStrategyKeepsCheapInputsSequential(t *testing.T) {
	probe := domain.Probe{Width: 1920, Height: 1080,
		Transcode: &domain.TranscodeProbe{Video: domain.TranscodeVideo{FPS: 30}}}
	if !useSeekSpriteBatches(domain.NewSeekSpriteLayout(7_200_000), probe) {
		t.Fatal("long HD video should use input seeks")
	}
	if useSeekSpriteBatches(domain.NewSeekSpriteLayout(120_000), probe) {
		t.Fatal("short video should not pay for seeks")
	}
	probe.Width, probe.Height = 640, 360
	if useSeekSpriteBatches(domain.NewSeekSpriteLayout(7_200_000), probe) {
		t.Fatal("low resolution video should decode once")
	}
	probe.Width, probe.Height = 1920, 1080
	probe.Transcode.Video.FPS = 1
	if useSeekSpriteBatches(domain.NewSeekSpriteLayout(7_200_000), probe) {
		t.Fatal("sparse video should decode once")
	}
}

func assertSameSeekSheets(t *testing.T, left, right string, count int) {
	t.Helper()
	for i := range count {
		name := fmt.Sprintf("%03d.jpg", i)
		a, err := os.ReadFile(filepath.Join(left, name))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(right, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("sheet %d differs from the sequential decoder", i)
		}
	}
	entries, err := os.ReadDir(right)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != count {
		t.Fatalf("temporary data left in output: %v", entries)
	}
}

// Independent decoder state matters for HEVC open GOPs, including batches
// crossing keyframes. Compare all cells and the final black padding losslessly.
func TestSeekSpriteBatchesMatchSequential(t *testing.T) {
	requireFFmpeg(t)
	for _, codec := range []string{"libx264", "libx265"} {
		t.Run(codec, func(t *testing.T) {
			video := filepath.Join(t.TempDir(), "clock.mp4")
			args := []string{"-f", "lavfi", "-i", timeGraySource("64x96", "30"),
				"-c:v", codec, "-preset", "ultrafast", "-g", "25"}
			if codec == "libx265" {
				args = append(args, "-x265-params", "pools=1:frame-threads=1:log-level=error", "-pix_fmt", "yuv420p10le")
			}
			runFFmpeg(t, append(args, "-y", video)...)
			// 15 cells exercise more than one ten-input batch, without a long fixture.
			layout := domain.SeekSpriteLayout{IntervalMs: 2000, FrameCount: 15, Columns: 10, Rows: 10, SheetCount: 1}
			slow, fast := t.TempDir(), t.TempDir()
			if _, err := runSeekFFmpeg(context.Background(), seekSpriteArgs(video, filepath.Join(slow, "%03d.jpg"), layout)); err != nil {
				t.Fatal(err)
			}
			if err := generateSeekSpriteBatches(context.Background(), video, fast, layout, 10); err != nil {
				t.Fatal(err)
			}
			assertSameSeekSheets(t, slow, fast, 1)
		})
	}
}

func TestSeekSpriteBatchesKeepRotatedAspectAndNonzeroStart(t *testing.T) {
	requireFFmpeg(t)
	base := filepath.Join(t.TempDir(), "base.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x96", "30"),
		"-vf", "setsar=4/3", "-c:v", "libx264", "-y", base)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"rotated", []string{"-c", "copy", "-metadata:s:v:0", "rotate=90"}},
		{"offset", []string{"-c", "copy", "-output_ts_offset", "7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := filepath.Join(t.TempDir(), "input.mp4")
			runFFmpeg(t, append(append([]string{"-i", base}, tc.args...), "-y", video)...)
			layout := domain.NewSeekSpriteLayout(30_000)
			slow, fast := t.TempDir(), t.TempDir()
			if _, err := runSeekFFmpeg(context.Background(), seekSpriteArgs(video, filepath.Join(slow, "%03d.jpg"), layout)); err != nil {
				t.Fatal(err)
			}
			if err := generateSeekSpriteBatches(context.Background(), video, fast, layout, 10); err != nil {
				t.Fatal(err)
			}
			assertSameSeekSheets(t, slow, fast, 1)
		})
	}
}

func TestSeekSpriteBatchesFailRatherThanShiftCellsAcrossVFRGap(t *testing.T) {
	requireFFmpeg(t)
	video := filepath.Join(t.TempDir(), "gap.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", strings.Replace(timeGraySource("64x64", "30"), ":r=10:", ":r=30:", 1),
		"-vf", "select='lt(t,6)+gte(t,15)'", "-fps_mode", "vfr", "-c:v", "libx264", "-y", video)
	output := t.TempDir()
	err := generateSeekSpriteBatches(context.Background(), video, output, domain.NewSeekSpriteLayout(30_000), 30)
	if err == nil {
		t.Fatal("missing VFR cell was accepted")
	}
	entries, readErr := os.ReadDir(output)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("incomplete data left behind: %v, %v", entries, readErr)
	}
}

func TestSeekSpriteFallsBackWhenVideoEndsBeforeRequestedCells(t *testing.T) {
	requireFFmpeg(t)
	video := filepath.Join(t.TempDir(), "early-end.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=white:s=1280x720:r=30:d=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-y", video)
	// A widened interval selects the fast path; the deliberately longer layout
	// represents a container whose audio outlasts its video.
	layout := domain.SeekSpriteLayout{IntervalMs: 6000, FrameCount: 6, Columns: 10, Rows: 10, SheetCount: 1}
	slow, actual := t.TempDir(), t.TempDir()
	if _, err := runSeekFFmpeg(context.Background(), seekSpriteArgs(video, filepath.Join(slow, "%03d.jpg"), layout)); err != nil {
		t.Fatal(err)
	}
	if err := GenerateSeekSprite(context.Background(), video, actual, layout); err != nil {
		t.Fatal(err)
	}
	assertSameSeekSheets(t, slow, actual, 1)
}

func TestSeekSpriteBatchesCleanUpAfterLaterBatchFailureAndCancel(t *testing.T) {
	requireFFmpeg(t)
	video := filepath.Join(t.TempDir(), "short.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=white:s=64x64:r=10:d=6", "-c:v", "libx264", "-y", video)
	layout := domain.SeekSpriteLayout{IntervalMs: 500, FrameCount: 30, Columns: 10, Rows: 10, SheetCount: 1}
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			if cancel {
				stop()
			}
			output := t.TempDir()
			err := generateSeekSpriteBatches(ctx, video, output, layout, 10)
			if err == nil {
				t.Fatal("expected incomplete or canceled generation")
			}
			if cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			entries, readErr := os.ReadDir(output)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("temporary output leaked: %v, %v", entries, readErr)
			}
		})
	}
}

func TestSeekSpriteBatchesKeepFractionalRateAndSheetOrder(t *testing.T) {
	requireFFmpeg(t)
	video := filepath.Join(t.TempDir(), "fractional.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", strings.Replace(timeGraySource("64x64", "30"), ":r=10:", ":r=30000/1001:", 1), "-c:v", "libx264", "-y", video)
	layout := domain.SeekSpriteLayout{IntervalMs: 250, FrameCount: 120, Columns: 10, Rows: 10, SheetCount: 2}
	slow, fast := t.TempDir(), t.TempDir()
	if _, err := runSeekFFmpeg(context.Background(), seekSpriteArgs(video, filepath.Join(slow, "%03d.jpg"), layout)); err != nil {
		t.Fatal(err)
	}
	if err := generateSeekSpriteBatches(context.Background(), video, fast, layout, 30000.0/1001); err != nil {
		t.Fatal(err)
	}
	assertSameSeekSheets(t, slow, fast, 2)
}

func TestSeekSpriteBatchesKeepVFRScenesInTheirIntervals(t *testing.T) {
	requireFFmpeg(t)
	video := filepath.Join(t.TempDir(), "vfr.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", strings.Replace(timeGraySource("64x64", "30"), ":r=10:", ":r=30:", 1), "-vf", "select='lt(t,15)+gte(t,15)*not(mod(n,3))'", "-fps_mode", "vfr", "-c:v", "libx264", "-y", video)
	probe, err := Probe(context.Background(), video)
	if err != nil {
		t.Fatal(err)
	}
	layout := domain.NewSeekSpriteLayout(30_000)
	output := t.TempDir()
	if err := generateSeekSpriteBatches(context.Background(), video, output, layout, seekSpriteFPS(probe)); err != nil {
		t.Fatal(err)
	}
	sheets := readSheets(t, output)
	for k := range layout.FrameCount {
		got := frameSeconds(t, sheets, layout, k)
		start := float64(k) * 5
		if got < start+0.5 || got > start+4.5 {
			t.Errorf("cell %d shows %.2fs outside its interval", k, got)
		}
	}
}

func TestSeekSpriteBatchesMatchBeforeHEVCOpenGOPKeyframe(t *testing.T) {
	requireFFmpeg(t)
	video := filepath.Join(t.TempDir(), "open-gop.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=30:duration=30", "-c:v", "libx265", "-preset", "veryfast", "-g", "75", "-x265-params", "keyint=75:min-keyint=75:scenecut=0:pools=1:frame-threads=1:log-level=error", "-pix_fmt", "yuv420p10le", "-y", video)
	// The second midpoint is just before the CRA picture at 12.5 seconds.
	// A direct seek can discard leading pictures and land on that future CRA.
	layout := domain.SeekSpriteLayout{IntervalMs: 8269, FrameCount: 3, Columns: 10, Rows: 10, SheetCount: 1}
	slow, fast := t.TempDir(), t.TempDir()
	if _, err := runSeekFFmpeg(context.Background(), seekSpriteArgs(video, filepath.Join(slow, "%03d.jpg"), layout)); err != nil {
		t.Fatal(err)
	}
	if err := generateSeekSpriteBatches(context.Background(), video, fast, layout, 30); err != nil {
		t.Fatal(err)
	}
	assertSameSeekSheets(t, slow, fast, 1)
}
