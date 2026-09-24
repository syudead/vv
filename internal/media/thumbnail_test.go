package media

import (
	"math"
	"strings"
	"testing"
)

// 抽出位置は「尺の 10%、ただし下限 1 秒・上限 60 秒」。
//
// 下限が尺を越える短い動画だけは例外で、10% 地点をそのまま使う。下限へ
// 丸めると末尾ちょうど（またはその先）を指し、ffmpeg が終了コード 0 のまま
// 1枚も出力しない。1,000 本規模の検証で、1 秒の動画すべてがこれで
// 失敗した。
// 先頭が黒画面やロゴであることが多いので先頭は避け、長い動画でも待たされない
// ように上限を切る。
func TestThumbnailOffset(t *testing.T) {
	tests := []struct {
		name       string
		durationMs int64
		wantSec    float64
	}{
		{"尺が不明なら下限", 0, 1},
		{"3 秒の動画は下限に丸める", 3000, 1},
		{"10 秒の動画はちょうど下限", 10000, 1},
		// 下限より短い動画では、下限に丸めると尺の外（または末尾ちょうど）を
		// 指してしまい、ffmpeg が1枚も取り出せない。そこだけは 10% 地点へ戻す。
		{"1 秒の動画は下限より手前を採る", 1000, 0.1},
		{"下限ちょうどの動画も末尾を指さない", 1000, 0.1},
		{"0.5 秒の動画", 500, 0.05},
		{"100 秒の動画は 10 秒", 100_000, 10},
		{"10 分の動画は 60 秒", 600_000, 60},
		{"2 時間の動画も上限で頭打ち", 7_200_000, 60},
		{"負の尺は下限", -5000, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := thumbnailOffset(tc.durationMs); math.Abs(got-tc.wantSec) > 1e-9 {
				t.Errorf("thumbnailOffset(%d) = %v 秒, want %v 秒", tc.durationMs, got, tc.wantSec)
			}
		})
	}
}

// 抽出位置は必ず尺の内側になる。境界で1枚も取れないと、一覧に画像が
// 出ない理由が「生成に失敗した」としか分からなくなる。
func TestThumbnailOffsetStaysInsideDuration(t *testing.T) {
	for _, durationMs := range []int64{100, 500, 999, 1000, 1001, 2000, 10_000, 600_000} {
		offset := thumbnailOffset(durationMs)
		if offset*1000 >= float64(durationMs) {
			t.Errorf("尺 %dms に対する抽出位置 %v 秒は末尾以降を指している", durationMs, offset)
		}
		if offset <= 0 {
			t.Errorf("尺 %dms に対する抽出位置 %v 秒が先頭以前", durationMs, offset)
		}
	}
}

// 組み立てる引数が意図どおりであること。-ss を -i の前に置くと
// キーフレーム単位の高速シークになり、長い動画でも一定時間で終わる。
func TestThumbnailArgs(t *testing.T) {
	got := thumbnailArgs("/media/a.mp4", 10, "/data/thumbnails/ab/ab.jpg")

	ssIndex := indexOf(got, "-ss")
	iIndex := indexOf(got, "-i")
	if ssIndex < 0 || iIndex < 0 {
		t.Fatalf("-ss か -i が無い: %v", got)
	}
	if ssIndex > iIndex {
		t.Errorf("-ss が -i の後ろにある（高速シークにならない）: %v", got)
	}

	joined := strings.Join(got, " ")
	for _, want := range []string{
		"-nostdin", "-v error", "-frames:v 1", "-vf scale=640:-2", "-q:v 4", "-y",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("引数に %q が無い: %v", want, got)
		}
	}

	if got[len(got)-1] != "/data/thumbnails/ab/ab.jpg" {
		t.Errorf("最後の引数が出力先ではない: %v", got)
	}
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}
