package media

import (
	"path/filepath"
	"strings"
	"testing"
)

// 抽出位置は「尺の 10%、ただし下限 1 秒・上限 60 秒」（R-104）。
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
		{"100 秒の動画は 10 秒", 100_000, 10},
		{"10 分の動画は 60 秒", 600_000, 60},
		{"2 時間の動画も上限で頭打ち", 7_200_000, 60},
		{"負の尺は下限", -5000, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := thumbnailOffset(tc.durationMs); got != tc.wantSec {
				t.Errorf("thumbnailOffset(%d) = %v 秒, want %v 秒", tc.durationMs, got, tc.wantSec)
			}
		})
	}
}

// 保存先は <MDM_DATA_DIR>/thumbnails/<content_key の先頭2文字>/<content_key>.jpg。
//
// content_key で名前を決めるので、移動・改名では作り直さない（FR-025）。
// 2文字のディレクトリに分けるのは、1ディレクトリに数万ファイルを置かないため。
func TestThumbnailPath(t *testing.T) {
	got := ThumbnailPath("/data/thumbnails", "ab12cd34:5678")
	want := filepath.Join("/data/thumbnails", "ab", "ab12cd34_5678.jpg")
	if got != want {
		t.Errorf("ThumbnailPath = %q, want %q", got, want)
	}
}

// content_key に含まれる ":" はファイル名に使わない。Windows 共有や一部の
// ファイルシステムで扱えず、置き場所ごと失敗する。
func TestThumbnailPathAvoidsPathSeparators(t *testing.T) {
	got := ThumbnailPath("/data/thumbnails", "aa/bb:cc")
	if strings.Contains(filepath.Base(got), ":") {
		t.Errorf("ファイル名に : が残っている: %q", got)
	}
	if strings.Count(strings.TrimPrefix(got, "/data/thumbnails/"), "/") != 1 {
		t.Errorf("2文字のディレクトリ1段に収まっていない: %q", got)
	}
}

// 短すぎる鍵でも置き場所を決められること。実際には 64 文字の 16 進が入るが、
// ここで落ちると取り込みが止まる。
func TestThumbnailPathWithShortKey(t *testing.T) {
	if got := ThumbnailPath("/data/thumbnails", "a"); got == "" {
		t.Error("置き場所を決められなかった")
	}
}

// 組み立てる引数が R-104 のとおりであること。-ss を -i の前に置くと
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
