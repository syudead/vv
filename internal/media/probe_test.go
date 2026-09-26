package media

import (
	"math"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// ffprobe を起動せずに解析部分だけを検証する。外部プロセスに依存しない形で
// 「JSON からどの値を取り出すか」を固定しておくと、ffprobe の出力が変わった
// ときに壊れ方が分かりやすい。
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

// 索引のコーデックは codec_name が空の stream を飛ばして次の同種の stream を採る。
// 名前の無い先頭の音声のために後ろの ac3 を見落とすと、再生できない動画を
// 直接再生できると判定してしまう。ライブ変換用の値は data-model.md §2 のとおり
// 最初の非添付映像・最初の音声のままである。
func TestParseProbeOutputSkipsStreamsWithoutCodecName(t *testing.T) {
	const output = `{
	  "streams": [
	    {"index": 0, "codec_type": "video", "codec_name": "", "width": 640, "height": 360},
	    {"index": 1, "codec_type": "video", "codec_name": "h264", "width": 1280, "height": 720},
	    {"index": 2, "codec_type": "audio", "codec_name": ""},
	    {"index": 3, "codec_type": "audio", "codec_name": "ac3"}
	  ],
	  "format": {"duration": "4.0", "format_name": "mov,mp4"}
	}`

	got, err := parseProbeOutput([]byte(output))
	if err != nil {
		t.Fatalf("解析に失敗した: %v", err)
	}
	if got.VideoCodec != "h264" || got.Width != 1280 || got.Height != 720 {
		t.Errorf("映像 = %q %dx%d, want h264 1280x720", got.VideoCodec, got.Width, got.Height)
	}
	if got.AudioCodec != "ac3" {
		t.Errorf("AudioCodec = %q, want ac3", got.AudioCodec)
	}
	if play := domain.EvaluatePlayability(domain.ContainerFromPath("clip.mp4"), got); play.Playable {
		t.Errorf("ac3 の音声を持つ動画を直接再生できると判定した: %+v", play)
	}
	if got.Transcode == nil || got.Transcode.Video.Index != 0 || got.Transcode.Audio == nil || got.Transcode.Audio.Index != 2 {
		t.Errorf("Transcode = %+v, want 映像 0・音声 2", got.Transcode)
	}
}

// ffprobe の前後でファイルの印が変わったら、ライブ変換用の解析情報を持たせない。
func TestStampProbeDropsTranscodeWhenFileChanged(t *testing.T) {
	probe := domain.Probe{Transcode: &domain.TranscodeProbe{}}
	before := domain.FileStamp{SizeBytes: 10, ModTimeNs: 1}

	same := stampProbe(probe, before, before)
	if same.Source != before || same.Transcode == nil {
		t.Errorf("変わらないファイル: %+v", same)
	}
	changed := stampProbe(probe, before, domain.FileStamp{SizeBytes: 12, ModTimeNs: 2})
	if changed.Source != before || changed.Transcode != nil {
		t.Errorf("差し替わったファイル: %+v", changed)
	}
}

// 回転の印が付いた動画は、表示される向きの解像度を記録する。
func TestParseProbeOutputAppliesRotation(t *testing.T) {
	tests := []struct {
		name   string
		stream string
		width  int
		height int
	}{
		{"display matrix", `"side_data_list": [{"side_data_type": "Display Matrix", "rotation": -90}]`, 1080, 1920},
		{"rotate tag", `"tags": {"rotate": "270"}`, 1080, 1920},
		{"upside down", `"side_data_list": [{"side_data_type": "Display Matrix", "rotation": 180}]`, 1920, 1080},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := `{
			  "streams": [
			    {"index": 0, "codec_type": "video", "codec_name": "h264",
			     "width": 1920, "height": 1080, ` + tc.stream + `}
			  ],
			  "format": {"duration": "3.0", "format_name": "mov,mp4"}
			}`
			got, err := parseProbeOutput([]byte(output))
			if err != nil {
				t.Fatalf("解析に失敗した: %v", err)
			}
			if got.Width != tc.width || got.Height != tc.height {
				t.Errorf("解像度 = %dx%d, want %dx%d", got.Width, got.Height, tc.width, tc.height)
			}
		})
	}
}

// 表示の縦横比は、画素の縦横比（SAR）と回転を反映する。
func TestParseProbeOutputDisplayAspectRatio(t *testing.T) {
	tests := []struct {
		name   string
		stream string
		want   float64
	}{
		{"square pixels", `"width": 1920, "height": 1080, "sample_aspect_ratio": "1:1"`, 16.0 / 9},
		{"no SAR", `"width": 1920, "height": 1080`, 16.0 / 9},
		{"anamorphic PAL", `"width": 720, "height": 576, "sample_aspect_ratio": "16:15"`, 4.0 / 3},
		{"rotated anamorphic", `"width": 720, "height": 576, "sample_aspect_ratio": "16:15", "tags": {"rotate": "90"}`, 3.0 / 4},
		{"unknown SAR", `"width": 1280, "height": 720, "sample_aspect_ratio": "0:1"`, 16.0 / 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := `{
			  "streams": [{"index": 0, "codec_type": "video", "codec_name": "h264", ` + tc.stream + `}],
			  "format": {"duration": "3.0", "format_name": "mov,mp4"}
			}`
			got, err := parseProbeOutput([]byte(output))
			if err != nil {
				t.Fatalf("解析に失敗した: %v", err)
			}
			if math.Abs(got.DisplayAspectRatio-tc.want) > 1e-9 {
				t.Errorf("DisplayAspectRatio = %f, want %f", got.DisplayAspectRatio, tc.want)
			}
		})
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
// ので、呼び出し側は次のファイルへ進める。
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

// 組み立てる引数が1回分であること。値ごとに複数回起動すると、
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
// 全体を止めないための上限である。
func TestProbeTimeout(t *testing.T) {
	if probeTimeout.Seconds() != 30 {
		t.Errorf("probeTimeout = %v, want 30s", probeTimeout)
	}
}
