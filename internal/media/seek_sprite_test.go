package media

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestSeekSpriteArgsFollowLayout(t *testing.T) {
	layout := domain.NewSeekSpriteLayout(7_200_000)
	args := seekSpriteArgs("/media/a.mp4", "/cache/%03d.jpg", layout)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-i /media/a.mp4",
		"-map 0:V:0?",
		"tpad=stop_mode=clone:stop=-1,fps=1000/88889:round=up:eof_action=pass,trim=end_frame=81,",
		"scale=min(160\\,iw):min(160\\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2,tile=9x9,format=yuvj420p",
		"-fps_mode passthrough",
		"-start_number 0",
		"/cache/%03d.jpg",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("生成引数に %q が無い: %v", want, args)
		}
	}
}

func runFFmpeg(t *testing.T, args ...string) {
	t.Helper()
	full := append([]string{"-nostdin", "-v", "error"}, args...)
	if output, err := exec.Command(seekThumbnailCommand, full...).CombinedOutput(); err != nil {
		t.Fatalf("fixture生成に失敗しました: %v: %s", err, output)
	}
}

// timeGraySource は時刻を明るさで描く入力で、t 秒の場面の輝度は制限範囲の 16+7t になる
// （30 秒で 226 まで）。
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

// frameSeconds は timeGraySource で作った入力のコマ k に描かれた時刻（秒）を返す。
// シートは全範囲（yuvj420p）の JPEG なので、入力の制限範囲の輝度に戻してから読む。
func frameSeconds(t *testing.T, sheets []image.Image, layout domain.SeekSpriteLayout, k int) float64 {
	t.Helper()
	limited := float64(frameLuma(t, sheets, layout, k))*219/255 + 16
	return (limited - 16) / 7
}

// frameLuma はシートに並んだコマ k の中心の輝度を返す。
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

// 先頭・中間・末尾のコマに描かれた時刻は、それぞれが受け持つ区間の中にある。
func TestGenerateSeekSpriteFramesStayInTheirIntervals(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "clock.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "30"), "-c:v", "mpeg4", "-q:v", "2", "-y", videoPath)

	output, layout := generateSprite(t, videoPath, 30_000)
	if layout.FrameCount != 6 || layout.IntervalMs != 5000 {
		t.Fatalf("配置 %+v", layout)
	}
	sheets := readSheets(t, output)
	if len(sheets) != 1 {
		t.Fatalf("シートが %d 枚（1 枚のはず）", len(sheets))
	}
	if b := sheets[0].Bounds(); b.Dx() != 576 || b.Dy() != 576 {
		t.Fatalf("シートの大きさ %v（576x576 のはず）", b)
	}
	for _, k := range []int{0, 3, 5} {
		startSec := float64(k) * float64(layout.IntervalMs) / 1000
		got := frameSeconds(t, sheets, layout, k)
		// 入力側シークは区間の先頭のフレームを選ぶ。圧縮と JPEG の誤差を許す。
		if got < startSec-0.5 || got > startSec+1 {
			t.Errorf("コマ %d の時刻 %.2f 秒が区間の先頭 %.0f 秒から離れている", k, got, startSec)
		}
	}
	// 使わない升目は黒のまま。
	if luma := frameLuma(t, sheets, layout, 6); luma > 20 {
		t.Errorf("空きの升目の輝度が %d", luma)
	}
}

// 映像が容器の長さより短い入力では、映像の後ろのコマは最後の場面を複製する。
func TestGenerateSeekSpriteRepeatsLastSceneWhenVideoEndsEarly(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "short-video.mp4")
	runFFmpeg(t,
		"-f", "lavfi", "-i", timeGraySource("64x64", "23"),
		"-f", "lavfi", "-i", "sine=frequency=440:duration=30",
		"-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac", "-y", videoPath)

	output, layout := generateSprite(t, videoPath, 30_000)
	if layout.FrameCount != 6 {
		t.Fatalf("配置 %+v", layout)
	}
	sheets := readSheets(t, output)
	for _, k := range []int{0, 1, 3} {
		startSec := float64(k) * float64(layout.IntervalMs) / 1000
		got := frameSeconds(t, sheets, layout, k)
		if got < startSec-0.5 || got > startSec+1 {
			t.Errorf("フォールバックのコマ %d は %.2f 秒（区間先頭 %.0f 秒のはず）", k, got, startSec)
		}
	}
	// 最後の場面は映像の終わり（23 秒）の近くで、黒で埋まっていない。
	if last := frameSeconds(t, sheets, layout, layout.FrameCount-1); last < 22 || last > 23.5 {
		t.Fatalf("末尾のコマの時刻が %.2f 秒（22〜23 秒のはず）", last)
	}
}

func TestGenerateSeekSpriteRejectsFramesOutsideTheirIntervals(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "sparse.mp4")
	runFFmpeg(t,
		"-f", "lavfi", "-i", timeGraySource("64x64", "30"),
		"-vf", "select=eq(n\\,0)+eq(n\\,100)+eq(n\\,200)",
		"-fps_mode", "vfr", "-c:v", "mpeg4", "-q:v", "2", "-y", videoPath)

	layout := domain.NewSeekSpriteLayout(15_000)
	if err := generateSeekSpriteParallel(context.Background(), videoPath, t.TempDir(), layout); err == nil || !strings.Contains(err.Error(), "コマ 1:") {
		t.Fatalf("フレームのない区間で失敗しなかった: %v", err)
	}
	output, layout := generateSprite(t, videoPath, 15_000)
	sheets := readSheets(t, output)
	for _, check := range []struct {
		frame int
		want  float64
	}{{1, 0}, {2, 10}} {
		if got := frameSeconds(t, sheets, layout, check.frame); got < check.want-0.5 || got > check.want+1 {
			t.Errorf("コマ %d は %.2f 秒（%.0f 秒の場面のはず）", check.frame, got, check.want)
		}
	}
}

func TestGenerateSeekSpriteIgnoresAttachedCover(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	mainVideo := filepath.Join(dir, "main.mp4")
	cover := filepath.Join(dir, "cover.jpg")
	videoPath := filepath.Join(dir, "with-cover.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=green:s=64x64:r=1:d=5", "-c:v", "mpeg4", "-y", mainVideo)
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=red:s=600x600", "-frames:v", "1", "-y", cover)
	runFFmpeg(t, "-i", mainVideo, "-i", cover, "-map", "0:v:0", "-map", "1:v:0", "-c", "copy", "-disposition:v:1", "attached_pic", "-y", videoPath)

	output, _ := generateSprite(t, videoPath, 5000)
	sheets := readSheets(t, output)
	bounds := sheets[0].Bounds()
	r, g, b, _ := color.RGBAModel.Convert(sheets[0].At(bounds.Min.X+bounds.Dx()/18, bounds.Min.Y+bounds.Dy()/18)).RGBA()
	if g <= r || g <= b {
		t.Errorf("表紙画像の色を抽出した: R=%d G=%d B=%d", r, g, b)
	}
}

// 間隔の半分より短い入力でも、1 コマのシートができる。
func TestGenerateSeekSpriteWritesSingleFrameForOneSecondInput(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "one-second.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=white:s=64x64:r=30:d=1", "-c:v", "mpeg4", "-y", videoPath)

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

// 縦長の入力でも、コマは縦横比を保って1枚のシートになる。
func TestGenerateSeekSpriteKeepsPortraitAspect(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "portrait.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=gray:s=360x640:r=1:d=510", "-c:v", "mpeg4", "-y", videoPath)

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
		// 360x640 を 160x160 の枠に収めると 90x160 になる。
		if b.Dx() != 90*layout.Columns || b.Dy() != 160*layout.Rows {
			t.Errorf("シート %d の大きさ %v（コマ 90x160 のはず）", i, b)
		}
	}
	if luma := frameLuma(t, sheets, layout, layout.FrameCount-1); luma < 60 {
		t.Errorf("末尾のコマの輝度が %d", luma)
	}
}
