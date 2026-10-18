package media

import (
	"math"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Parse fixtures without ffprobe so output-schema changes have focused failures.
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

// Skip unnamed index codecs so later unsupported AC3 is not overlooked; transcoding still uses the
// first audio stream.
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

func TestStampProbeDropsTranscodeWhenFileChanged(t *testing.T) {
	probe := domain.Probe{Transcode: &domain.TranscodeProbe{}, Media: &domain.MediaInfo{}, Keyframes: &domain.KeyframeProbe{}}
	before := domain.FileStamp{SizeBytes: 10, ModTimeNs: 1}

	same := stampProbe(probe, before, before)
	if same.Source != before || same.Transcode == nil || same.Media == nil || same.Keyframes == nil {
		t.Errorf("変わらないファイル: %+v", same)
	}
	changed := stampProbe(probe, before, domain.FileStamp{SizeBytes: 12, ModTimeNs: 2})
	if changed.Source != before || changed.Transcode != nil || changed.Media != nil || changed.Keyframes != nil {
		t.Errorf("差し替わったファイル: %+v", changed)
	}
}

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

// Single-process probing avoids repeated process startup overhead.
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

	// The -- separator protects dash-prefixed filenames.
	if got[len(got)-2] != "--" {
		t.Errorf("パスの直前が -- ではない: %v", got)
	}
	if got[len(got)-1] != "/media/夏休みの旅行.mp4" {
		t.Errorf("最後の引数がパスではない: %v", got)
	}
}

// Bound corrupt-file probing to thirty seconds so the ingestion queue can advance.
func TestProbeTimeout(t *testing.T) {
	if probeTimeout.Seconds() != 30 {
		t.Errorf("probeTimeout = %v, want 30s", probeTimeout)
	}
}
