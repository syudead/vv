package media

import (
	"strings"
	"testing"
)

// ffprobe を起動せずに解析部分だけを検証する。外部プロセスに依存しない形で
// 「JSON からどの値を取り出すか」を固定しておくと、ffprobe の出力が変わった
// ときに壊れ方が分かりやすい（R-102）。
func TestParseProbeOutput(t *testing.T) {
	const output = `{
	  "streams": [
	    {"codec_type": "audio", "codec_name": "aac"},
	    {"index": 1, "codec_type": "video", "codec_name": "mjpeg", "width": 600, "height": 600,
	     "disposition": {"attached_pic": 1}},
	    {"codec_type": "video", "codec_name": "h264", "width": 1280, "height": 720},
	    {"codec_type": "video", "codec_name": "mjpeg", "width": 320, "height": 180}
	  ],
	  "format": {"duration": "8.533000", "format_name": "mov,mp4,m4a,3gp,3g2,mj2"}
	}`

	got, err := parseProbeOutput([]byte(output))
	if err != nil {
		t.Fatalf("解析に失敗した: %v", err)
	}

	if got.DurationMs != 8533 {
		t.Errorf("DurationMs = %d, want 8533", got.DurationMs)
	}
	if got.FormatName != "mov,mp4,m4a,3gp,3g2,mj2" {
		t.Errorf("FormatName = %q", got.FormatName)
	}
	// 添付画像を飛ばし、最初の非添付映像を本編として採る。
	if got.VideoCodec != "h264" {
		t.Errorf("VideoCodec = %q, want h264", got.VideoCodec)
	}
	if got.Width != 1280 || got.Height != 720 {
		t.Errorf("解像度 = %dx%d, want 1280x720", got.Width, got.Height)
	}
	if got.AudioCodec != "aac" {
		t.Errorf("AudioCodec = %q, want aac", got.AudioCodec)
	}
}

func TestParseProbeOutputDoesNotTreatAttachedPictureAsVideo(t *testing.T) {
	const output = `{
	  "streams": [
	    {"index": 0, "codec_type": "video", "codec_name": "mjpeg",
	     "disposition": {"attached_pic": 1}}
	  ],
	  "format": {"duration": "12.0", "format_name": "mov,mp4"}
	}`

	got, err := parseProbeOutput([]byte(output))
	if err != nil {
		t.Fatalf("解析に失敗した: %v", err)
	}
	if got.VideoCodec != "" || got.Width != 0 || got.Height != 0 {
		t.Errorf("添付画像を本編として採った: %+v", got)
	}
}

// 音声が無い動画も取り込める。音声コーデックが空になるだけで、失敗ではない。
func TestParseProbeOutputWithoutAudio(t *testing.T) {
	const output = `{
	  "streams": [{"codec_type": "video", "codec_name": "vp9", "width": 640, "height": 360}],
	  "format": {"duration": "12.0", "format_name": "matroska,webm"}
	}`

	got, err := parseProbeOutput([]byte(output))
	if err != nil {
		t.Fatalf("音声の無い動画で失敗した: %v", err)
	}
	if got.AudioCodec != "" {
		t.Errorf("AudioCodec = %q, want 空", got.AudioCodec)
	}
	if got.DurationMs != 12000 {
		t.Errorf("DurationMs = %d, want 12000", got.DurationMs)
	}
}

// 解析できない出力は、その1件の失敗として返す。取り込み全体は止めない
// （FR-008）ので、呼び出し側は次のファイルへ進める。
func TestParseProbeOutputRejectsBrokenInput(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"JSON ではない", `<html>404</html>`},
		{"途中で切れている", `{"format": {"duration":`},
		{"空", ``},
		{"duration が無い", `{"streams": [], "format": {"format_name": "mp4"}}`},
		{"duration が数値ではない", `{"format": {"duration": "N/A", "format_name": "mp4"}}`},
		{"duration が負", `{"format": {"duration": "-1.0", "format_name": "mp4"}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseProbeOutput([]byte(tc.output)); err == nil {
				t.Error("誤りとして返らなかった")
			}
		})
	}
}

// 組み立てる引数が R-102 の1回分であること。値ごとに複数回起動すると、
// プロセス起動が支配的なコストなので取り込みが遅くなる。
func TestProbeArgs(t *testing.T) {
	got := probeArgs("/media/夏休みの旅行.mp4")
	joined := strings.Join(got, " ")

	for _, want := range []string{
		"-v error", "-print_format json", "-show_format", "-show_streams",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("引数に %q が無い: %v", want, got)
		}
	}

	// パスは -- の後ろに置く。"-" で始まる名前のファイルを選択肢と
	// 取り違えないため。
	if got[len(got)-2] != "--" {
		t.Errorf("パスの直前が -- ではない: %v", got)
	}
	if got[len(got)-1] != "/media/夏休みの旅行.mp4" {
		t.Errorf("最後の引数がパスではない: %v", got)
	}
}

// タイムアウトは 30 秒。壊れたファイルで ffprobe が戻らない場合に取り込み
// 全体を止めないための上限である（R-102）。
func TestProbeTimeout(t *testing.T) {
	if probeTimeout.Seconds() != 30 {
		t.Errorf("probeTimeout = %v, want 30s", probeTimeout)
	}
}
