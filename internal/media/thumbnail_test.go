package media

import (
	"math"
	"slices"
	"strings"
	"testing"
)

func TestAutoThumbnailPositions(t *testing.T) {
	tests := []struct {
		name       string
		durationMs int64
		want       []int64
	}{
		{"尺が不明なら下限、次に先頭", 0, []int64{1000, 0}},
		{"100 秒の動画は 10 秒、次に先頭", 100_000, []int64{10_000, 0}},
		{"0.5 秒の動画は 10% 地点、次に先頭", 500, []int64{50, 0}},
		{"10 分の動画は上限、次に先頭", 600_000, []int64{60_000, 0}},
		{"自動の位置が先頭に丸まれば先頭だけ", 3, []int64{0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AutoThumbnailPositions(tc.durationMs)
			if !slices.Equal(got, tc.want) {
				t.Errorf("AutoThumbnailPositions(%d) = %v, want %v", tc.durationMs, got, tc.want)
			}
		})
	}
}

// Keep extraction inside short videos; clamping to one second can make ffmpeg succeed without
// output at or beyond the end.
func TestThumbnailOffset(t *testing.T) {
	tests := []struct {
		name       string
		durationMs int64
		wantSec    float64
	}{
		{"尺が不明なら下限", 0, 1},
		{"3 秒の動画は下限に丸める", 3000, 1},
		{"10 秒の動画はちょうど下限", 10000, 1},
		// Use ten percent when the minimum reaches or exceeds duration.
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
		"-nostdin", "-v error", "-frames:v 1", "-vf scale=640:640:force_original_aspect_ratio=decrease:force_divisible_by=2", "-q:v 4", "-y",
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
