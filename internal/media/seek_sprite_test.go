package media

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func runFFmpeg(t *testing.T, args ...string) {
	t.Helper()
	full := append([]string{"-nostdin", "-v", "error"}, args...)
	if output, err := exec.Command(seekThumbnailCommand, full...).CombinedOutput(); err != nil {
		t.Fatalf("fixture生成に失敗しました: %v: %s", err, output)
	}
}

// Encode time t as limited-range luma 16+7t, reaching 226 at thirty seconds.
func timeGraySource(size string, seconds string) string {
	return "color=c=black:s=" + size + ":r=10:d=" + seconds + ",format=yuv420p,geq=lum='16+7*T':cb=128:cr=128"
}

func generateSprite(t *testing.T, videoPath string, durationMs int64) (string, domain.SeekSpriteLayout) {
	t.Helper()
	layout := domain.NewSeekSpriteLayout(durationMs)
	output := t.TempDir()
	if err := GenerateSeekSprite(context.Background(), videoPath, output, layout); err != nil {
		t.Fatal(err)
	}
	return output, layout
}

func readSheets(t *testing.T, dir string) []image.Image {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	sheets := make([]image.Image, 0, len(entries))
	for i, entry := range entries {
		if want := []string{"000.jpg", "001.jpg", "002.jpg", "003.jpg", "004.jpg", "005.jpg"}[i]; entry.Name() != want {
			t.Fatalf("シート %d の名前が %s（%s のはず）", i, entry.Name(), want)
		}
		file, err := os.Open(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(file)
		_ = file.Close()
		if err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		sheets = append(sheets, img)
	}
	return sheets
}

// Convert full-range JPEG luma back to the source's limited range before reading time.
func frameSeconds(t *testing.T, sheets []image.Image, layout domain.SeekSpriteLayout, k int) float64 {
	t.Helper()
	limited := float64(frameLuma(t, sheets, layout, k))*219/255 + 16
	return (limited - 16) / 7
}

func frameLuma(t *testing.T, sheets []image.Image, layout domain.SeekSpriteLayout, k int) int {
	t.Helper()
	sheet := sheets[k/domain.SeekSpriteFramesPerSheet]
	bounds := sheet.Bounds()
	w, h := bounds.Dx()/layout.Columns, bounds.Dy()/layout.Rows
	cell := k % domain.SeekSpriteFramesPerSheet
	x := (cell%layout.Columns)*w + w/2
	y := (cell/layout.Columns)*h + h/2
	switch img := sheet.(type) {
	case *image.YCbCr:
		return int(img.Y[img.YOffset(x, y)])
	case *image.Gray:
		return int(img.GrayAt(x, y).Y)
	default:
		t.Fatalf("想定外の画像の型 %T", sheet)
		return 0
	}
}

func TestGenerateSeekSpriteFramesStayInTheirIntervals(t *testing.T) {
	t.Parallel()

	requireFFmpeg(t)
	videoPath := mediaFixture(t, "clock.mp4")

	output, layout := generateSprite(t, videoPath, 30_000)
	if layout.FrameCount != 30 || layout.IntervalMs != 1000 {
		t.Fatalf("配置 %+v", layout)
	}
	sheets := readSheets(t, output)
	if len(sheets) != 1 {
		t.Fatalf("シートが %d 枚（1 枚のはず）", len(sheets))
	}
	if b := sheets[0].Bounds(); b.Dx() != 576 || b.Dy() != 576 {
		t.Fatalf("シートの大きさ %v（576x576 のはず）", b)
	}
	for _, k := range []int{0, 15, 29} {
		startSec := float64(k) * float64(layout.IntervalMs) / 1000
		got := frameSeconds(t, sheets, layout, k)
		// Allow codec/JPEG error when checking interval-start seeks.
		if got < startSec-0.5 || got > startSec+1 {
			t.Errorf("コマ %d の時刻 %.2f 秒が区間の先頭 %.0f 秒から離れている", k, got, startSec)
		}
	}
	// Unused cells stay black.
	if luma := frameLuma(t, sheets, layout, 30); luma > 20 {
		t.Errorf("空きの升目の輝度が %d", luma)
	}
}

func TestGenerateSeekSpriteRepeatsLastSceneWhenVideoEndsEarly(t *testing.T) {
	t.Parallel()

	requireFFmpeg(t)
	videoPath := mediaFixture(t, "short-video.mp4")

	output, layout := generateSprite(t, videoPath, 30_000)
	if layout.FrameCount != 30 {
		t.Fatalf("配置 %+v", layout)
	}
	sheets := readSheets(t, output)
	for _, k := range []int{0, 5, 15, 22} {
		startSec := float64(k) * float64(layout.IntervalMs) / 1000
		got := frameSeconds(t, sheets, layout, k)
		if got < startSec-0.5 || got > startSec+1 {
			t.Errorf("コマ %d は %.2f 秒（区間先頭 %.0f 秒のはず）", k, got, startSec)
		}
	}
	// Intervals 23-30 repeat the frame at second 22.
	if last := frameSeconds(t, sheets, layout, layout.FrameCount-1); last < 21.5 || last > 23 {
		t.Fatalf("末尾のコマの時刻が %.2f 秒（22 秒の場面のはず）", last)
	}
}

// Missing intervals reuse the preceding frame without borrowing the next interval or falling back
// to full decoding.
func TestGenerateSeekSpriteRepeatsPreviousFrameForEmptyInterval(t *testing.T) {
	t.Parallel()

	requireFFmpeg(t)
	videoPath := mediaFixture(t, "sparse.mp4")

	layout := domain.NewSeekSpriteLayout(15_000)
	output := t.TempDir()
	if err := generateSeekSpriteParallel(context.Background(), videoPath, output, layout); err != nil {
		t.Fatalf("フレームのない区間で区間抽出が失敗した: %v", err)
	}
	sheets := readSheets(t, output)
	for _, check := range []struct {
		frame int
		want  float64
	}{{1, 0}, {9, 0}, {10, 10}} {
		if got := frameSeconds(t, sheets, layout, check.frame); got < check.want-0.5 || got > check.want+1 {
			t.Errorf("コマ %d は %.2f 秒（%.0f 秒の場面のはず）", check.frame, got, check.want)
		}
	}
}

func TestGenerateSeekSpriteIgnoresAttachedCover(t *testing.T) {
	requireFFmpeg(t)
	videoPath := mediaFixture(t, "with-cover.mp4")

	output, _ := generateSprite(t, videoPath, 5000)
	sheets := readSheets(t, output)
	bounds := sheets[0].Bounds()
	r, g, b, _ := color.RGBAModel.Convert(sheets[0].At(bounds.Min.X+bounds.Dx()/18, bounds.Min.Y+bounds.Dy()/18)).RGBA()
	if g <= r || g <= b {
		t.Errorf("表紙画像の色を抽出した: R=%d G=%d B=%d", r, g, b)
	}
}

func TestGenerateSeekSpriteWritesSingleFrameForOneSecondInput(t *testing.T) {
	requireFFmpeg(t)
	videoPath := mediaFixture(t, "one-second.mp4")

	output, layout := generateSprite(t, videoPath, 1000)
	if layout.FrameCount != 1 {
		t.Fatalf("配置 %+v", layout)
	}
	sheets := readSheets(t, output)
	if len(sheets) != 1 {
		t.Fatalf("シートが %d 枚（1 枚のはず）", len(sheets))
	}
	if luma := frameLuma(t, sheets, layout, 0); luma < 200 {
		t.Fatalf("先頭のコマの輝度が %d（白のはず）", luma)
	}
}

func TestGenerateSeekSpriteKeepsPortraitAspect(t *testing.T) {
	t.Parallel()

	requireFFmpeg(t)
	videoPath := mediaFixture(t, "portrait.mp4")

	output, layout := generateSprite(t, videoPath, 510_000)
	if layout.FrameCount != 81 || layout.SheetCount != 1 {
		t.Fatalf("配置 %+v", layout)
	}
	sheets := readSheets(t, output)
	if len(sheets) != layout.SheetCount {
		t.Fatalf("シートが %d 枚（%d 枚のはず）", len(sheets), layout.SheetCount)
	}
	for i, sheet := range sheets {
		b := sheet.Bounds()
		// Fitting 360x640 inside 320x320 yields 180x320.
		if b.Dx() != 180*layout.Columns || b.Dy() != 320*layout.Rows {
			t.Errorf("シート %d の大きさ %v（コマ 180x320 のはず）", i, b)
		}
	}
	if luma := frameLuma(t, sheets, layout, layout.FrameCount-1); luma < 60 {
		t.Errorf("末尾のコマの輝度が %d", luma)
	}
}

// An interval over the limit fails generation at once, marked so the job is not retried.
func TestGenerateSeekSpriteFailsIntervalOverFrameTimeout(t *testing.T) {
	t.Parallel()

	requireFFmpeg(t)
	videoPath := mediaFixture(t, "sparse.mp4")

	err := generateSeekSpriteWithin(context.Background(), videoPath, t.TempDir(), domain.NewSeekSpriteLayout(15_000), time.Nanosecond)
	if !errors.Is(err, domain.ErrFailureRepeats) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("上限を超えた区間で %v（再試行しない失敗のはず）", err)
	}
}
