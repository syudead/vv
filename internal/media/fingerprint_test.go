package media

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// sceneSource は 20 秒ごとに場面の変わる映像の入力である。場面 s は、低い周波数の縞を
// いくつか重ねた模様で、向き・細かさ・位相が variant と s で決まり、場面の中でもゆっくり
// 動く。実際の映像のように低い周波数ほど強くする。
func sceneSource(variant int, seconds string) string {
	v := strconv.Itoa(variant)
	s := "floor(T/20)"
	terms := []string{"128"}
	for i, wave := range []struct{ amplitude, fx, fy string }{
		{"34", "0.5", "0.5"}, {"26", "1", "0.5"}, {"22", "0.5", "1"}, {"18", "1.5", "1"},
		{"14", "1", "2"}, {"12", "2", "1.5"}, {"10", "2.5", "2"}, {"8", "3", "3"},
	} {
		n := strconv.Itoa(i + 1)
		// 場面と variant で位相と向きを変える（係数の符号が場面ごとに入れ替わる）。
		sign := "(2*mod(" + s + "*" + n + "+" + v + "*" + strconv.Itoa(i+2) + ",2)-1)"
		terms = append(terms, wave.amplitude+"*sin(2*PI*(X/W*"+wave.fx+"+"+sign+"*Y/H*"+wave.fy+")+"+
			s+"*"+strconv.Itoa(i*3+1)+"+"+v+"*"+strconv.Itoa(i*5+2)+"+T*0.02)")
	}
	expr := strings.Join(terms, "+")
	return "color=c=gray:s=64x36:r=2:d=" + seconds + ",format=yuv420p,geq=lum='" + expr + "':cb=128:cr=128"
}

// encodeScene は sceneSource を尺 seconds・映像のフィルタ filter・キーフレームの間隔 gop で
// H.264 の MP4 に書く。
func encodeScene(t *testing.T, variant int, seconds, filter string, crf, gop int) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "video.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", sceneSource(variant, seconds)+","+filter,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", strconv.Itoa(crf), "-g", strconv.Itoa(gop), "-keyint_min", "1",
		"-pix_fmt", "yuv420p", output)
	return output
}

// spriteFingerprintOf は動画のスプライトを尺 durationMs の配置で作り、指紋を返す。
func spriteFingerprintOf(t *testing.T, videoPath string, durationMs int64) domain.Fingerprint {
	t.Helper()
	dir, layout := generateSprite(t, videoPath, durationMs)
	sheets := make([][]byte, layout.SheetCount)
	for i := range sheets {
		data, err := os.ReadFile(filepath.Join(dir, "00"+strconv.Itoa(i)+".jpg"))
		if err != nil {
			t.Fatal(err)
		}
		sheets[i] = data
	}
	sheet, err := jpeg.Decode(bytes.NewReader(sheets[0]))
	if err != nil {
		t.Fatal(err)
	}
	sprite := domain.SeekSprite{
		SeekSpriteLayout: layout,
		FrameWidth:       sheet.Bounds().Dx() / layout.Columns,
		FrameHeight:      sheet.Bounds().Dy() / layout.Rows,
	}
	fingerprint, err := SpriteFingerprint(sprite, sheets)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint.Version != domain.FingerprintVersion || fingerprint.IntervalMs != layout.IntervalMs ||
		len(fingerprint.Frames) != layout.FrameCount {
		t.Fatalf("指紋 %d/%d/%d コマ、配置 %+v", fingerprint.Version, fingerprint.IntervalMs, len(fingerprint.Frames), layout)
	}
	return fingerprint
}

// logDistance は閾値との余裕が分かるよう距離を記録する。
func logDistance(t *testing.T, name string, a, b domain.Fingerprint) {
	t.Helper()
	distance, ok := domain.CompareFingerprints(a, b)
	t.Logf("%s: 中央値 %d（比べられた %v、閾値 %d）", name, distance, ok, domain.FingerprintMatchMaxDistance)
}

func requireLibx264(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(seekThumbnailCommand); err != nil {
		t.Skip("ffmpeg が無い")
	}
	out, err := exec.Command(seekThumbnailCommand, "-hide_banner", "-encoders").Output()
	if err != nil || !strings.Contains(string(out), "libx264") {
		t.Skip("ffmpeg に libx264 が無い")
	}
}

// 同じ動画を解像度・画質・キーフレームの間隔を変えて再エンコードした 2 本は一致し、
// 尺だけ同じ別の動画は一致しない（受け入れ条件 3・4 の判定の部分）。黒い帯を足した版も
// 帯を落として比べる。
func TestSpriteFingerprintMatchesReencodes(t *testing.T) {
	requireLibx264(t)
	large := spriteFingerprintOf(t, encodeScene(t, 0, "60", "scale=640:360", 20, 10), 60_000)
	small := spriteFingerprintOf(t, encodeScene(t, 0, "60", "scale=320:180,pad=320:240:0:30", 32, 15), 60_000)
	other := spriteFingerprintOf(t, encodeScene(t, 1, "60", "scale=640:360", 20, 10), 60_000)

	logDistance(t, "再エンコードどうし", large, small)
	logDistance(t, "別の動画", large, other)
	if distance, ok := domain.CompareFingerprints(large, small); !ok || distance > domain.FingerprintMatchMaxDistance {
		t.Errorf("再エンコードどうし: distance=%d ok=%v", distance, ok)
	}
	if distance, ok := domain.CompareFingerprints(large, other); !ok || distance <= domain.FingerprintMatchMaxDistance {
		t.Errorf("尺だけ同じ別の動画: distance=%d ok=%v", distance, ok)
	}
}

// 405 秒を超える動画は間隔が ceil(尺 / 81) なので、尺が少し違う再エンコードは間隔が違う。
// コマを時刻で組にするので、それでも一致する。
func TestSpriteFingerprintMatchesLongReencodesWithDifferentIntervals(t *testing.T) {
	requireLibx264(t)
	a := spriteFingerprintOf(t, encodeScene(t, 0, "410", "scale=640:360", 20, 10), 410_000)
	b := spriteFingerprintOf(t, encodeScene(t, 0, "410.4", "scale=320:180", 30, 15), 410_400)
	if a.IntervalMs == b.IntervalMs {
		t.Fatalf("間隔が同じ %d", a.IntervalMs)
	}
	for _, pair := range [][2]domain.Fingerprint{{a, b}, {b, a}} {
		logDistance(t, "間隔の違う再エンコード", pair[0], pair[1])
		if distance, ok := domain.CompareFingerprints(pair[0], pair[1]); !ok || distance > domain.FingerprintMatchMaxDistance {
			t.Errorf("間隔 %d と %d: distance=%d ok=%v", pair[0].IntervalMs, pair[1].IntervalMs, distance, ok)
		}
	}
	other := spriteFingerprintOf(t, encodeScene(t, 1, "410", "scale=640:360", 20, 10), 410_000)
	logDistance(t, "別の動画", a, other)
	if domain.FingerprintsMatch(a, other) {
		t.Error("尺だけ同じ別の動画が一致する")
	}
}

// spriteFingerprintWith は全編復号の経路で、コマの大きさと JPEG の画質だけを差し替えた
// スプライトを作り、指紋を返す。生成の設定が変わる前後のスプライトを再現して比べるのに使う。
func spriteFingerprintWith(t *testing.T, videoPath string, durationMs int64, longSide, quality string) domain.Fingerprint {
	t.Helper()
	layout := domain.NewSeekSpriteLayout(durationMs)
	scale := strings.ReplaceAll(seekSpriteFastScale, "320", longSide)
	args := seekSpriteArgs(videoPath, filepath.Join(t.TempDir(), "%03d.jpg"), layout)
	for i, arg := range args {
		switch {
		case strings.Contains(arg, seekSpriteFastScale):
			args[i] = strings.Replace(arg, seekSpriteFastScale, scale, 1)
		case i > 0 && args[i-1] == "-q:v":
			args[i] = quality
		}
	}
	// runFFmpeg が先頭に付ける -nostdin -v error を除く。
	runFFmpeg(t, args[3:]...)
	data, err := os.ReadFile(strings.Replace(args[len(args)-1], "%03d", "000", 1))
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	sprite := domain.SeekSprite{
		SeekSpriteLayout: layout,
		FrameWidth:       sheet.Bounds().Dx() / layout.Columns,
		FrameHeight:      sheet.Bounds().Dy() / layout.Rows,
	}
	fingerprint, err := SpriteFingerprint(sprite, [][]byte{data})
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

// 既存の 160px・-q:v 4 のスプライトから作った指紋と、今の 320px・-q:v 2 のスプライトから
// 作った指紋は、同じ動画なら一致し、尺だけ同じ別の動画とは一致しない。指紋は 32×32 に
// 面積平均で縮めた輝度の低周波から作るので、コマの大きさと画質の違いはほぼ残らない。
func TestSpriteFingerprintMatchesAcrossSpriteSizes(t *testing.T) {
	requireLibx264(t)
	video := encodeScene(t, 0, "60", "scale=640:360", 20, 10)
	old := spriteFingerprintWith(t, video, 60_000, "160", "4")
	current := spriteFingerprintWith(t, video, 60_000, "320", seekSpriteQuality)
	other := spriteFingerprintWith(t, encodeScene(t, 1, "60", "scale=640:360", 20, 10), 60_000, "320", seekSpriteQuality)

	logDistance(t, "160px と 320px", old, current)
	logDistance(t, "160px と別の動画の 320px", old, other)
	if distance, ok := domain.CompareFingerprints(old, current); !ok || distance > domain.FingerprintMatchMaxDistance/2 {
		t.Errorf("160px と 320px: distance=%d ok=%v（閾値の半分 %d 以下のはず）", distance, ok, domain.FingerprintMatchMaxDistance/2)
	}
	if domain.FingerprintsMatch(old, other) {
		t.Error("尺だけ同じ別の動画が一致する")
	}
}

// シートの枚数や大きさが配置情報と合わなければ誤りを返す。
func TestSpriteFingerprintRejectsMismatchedSheets(t *testing.T) {
	layout := domain.NewSeekSpriteLayout(60_000)
	sprite := domain.SeekSprite{SeekSpriteLayout: layout, FrameWidth: 160, FrameHeight: 90}
	if _, err := SpriteFingerprint(sprite, nil); err == nil {
		t.Error("シートが無いのに誤りにならない")
	}
	var small bytes.Buffer
	if err := jpeg.Encode(&small, image.NewGray(image.Rect(0, 0, 160, 90)), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SpriteFingerprint(sprite, [][]byte{small.Bytes()}); err == nil {
		t.Error("コマがシートの外なのに誤りにならない")
	}
	if _, err := SpriteFingerprint(sprite, [][]byte{[]byte("not a jpeg")}); err == nil {
		t.Error("読めないシートで誤りにならない")
	}
}
