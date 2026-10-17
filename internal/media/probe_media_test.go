package media

import (
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// ffprobe の JSON の fixture から domain.MediaInfo を読む。外部プロセスを起動せず、どの値をどこから読むかを固定する。

func parseMediaFixture(t *testing.T, output string) *domain.MediaInfo {
	t.Helper()
	probe, err := parseProbeOutput([]byte(output))
	if err != nil {
		t.Fatalf("解析に失敗した: %v", err)
	}
	if probe.Media == nil {
		t.Fatal("Media が nil")
	}
	return probe.Media
}

func mediaVideoFixture(t *testing.T, stream string) domain.MediaVideo {
	t.Helper()
	info := parseMediaFixture(t, `{"streams": [`+stream+`], "format": {"duration": "10.0"}}`)
	if info.Video == nil {
		t.Fatal("Video が nil")
	}
	return *info.Video
}

func TestParseMediaInfoClassifiesHDR(t *testing.T) {
	tests := []struct {
		name   string
		stream string
		want   domain.HDRKind
	}{
		{"HDR10", `{"index": 0, "codec_type": "video", "codec_name": "hevc", "codec_tag_string": "hvc1",
			"color_transfer": "smpte2084", "color_primaries": "bt2020"}`, domain.HDRKindHDR10},
		{"HLG", `{"index": 0, "codec_type": "video", "codec_name": "hevc", "color_transfer": "arib-std-b67"}`, domain.HDRKindHLG},
		{"Dolby Vision の構成記録", `{"index": 0, "codec_type": "video", "codec_name": "hevc", "color_transfer": "smpte2084",
			"side_data_list": [{"side_data_type": "DOVI configuration record", "dv_profile": 8}]}`, domain.HDRKindDolbyVision},
		{"Dolby Vision の codec tag", `{"index": 0, "codec_type": "video", "codec_name": "hevc", "codec_tag_string": "dvh1"}`,
			domain.HDRKindDolbyVision},
		{"10 bit の BT.2020 でも PQ でなければ SDR", `{"index": 0, "codec_type": "video", "codec_name": "hevc",
			"pix_fmt": "yuv420p10le", "color_primaries": "bt2020", "color_transfer": "bt709"}`, domain.HDRKindSDR},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mediaVideoFixture(t, tc.stream).HDR; got != tc.want {
				t.Errorf("HDR = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseMediaInfoReadsVideoItems(t *testing.T) {
	video := mediaVideoFixture(t, `{"index": 0, "codec_type": "video", "codec_name": "hevc", "width": 3840, "height": 1920,
		"pix_fmt": "yuv420p10le", "avg_frame_rate": "30000/1001", "bit_rate": "45000000", "field_order": "tt",
		"closed_captions": 1,
		"side_data_list": [
			{"side_data_type": "Spherical Mapping", "projection": "equirectangular", "yaw": 0, "pitch": 0, "roll": 0},
			{"side_data_type": "Stereo 3D", "type": "top and bottom", "inverted": 0}
		]}`)
	if video.Projection != "equirectangular" {
		t.Errorf("Projection = %q", video.Projection)
	}
	if video.Stereo3D != "top and bottom" {
		t.Errorf("Stereo3D = %q", video.Stereo3D)
	}
	if video.Interlaced == nil || !*video.Interlaced {
		t.Errorf("Interlaced = %v, want true", video.Interlaced)
	}
	if video.BitDepth != 10 || video.BitRate != 45_000_000 || !video.ClosedCaptions {
		t.Errorf("video = %+v", video)
	}
	if video.FrameRate < 29.97 || video.FrameRate > 29.98 {
		t.Errorf("FrameRate = %v", video.FrameRate)
	}
}

func TestParseMediaInfoOmitsUnknownFieldOrderAndReadsMatroskaBitRate(t *testing.T) {
	video := mediaVideoFixture(t, `{"index": 0, "codec_type": "video", "codec_name": "h264", "field_order": "unknown",
		"bits_per_raw_sample": "8", "avg_frame_rate": "0/0", "tags": {"BPS": "8000000", "DURATION": "00:00:10.000"}}`)
	if video.Interlaced != nil {
		t.Errorf("Interlaced = %v, want nil", *video.Interlaced)
	}
	if video.BitRate != 8_000_000 || video.BitDepth != 8 || video.FrameRate != 0 {
		t.Errorf("video = %+v", video)
	}
}

func TestParseMediaInfoReadsTracks(t *testing.T) {
	info := parseMediaFixture(t, `{
	  "streams": [
	    {"index": 0, "codec_type": "video", "codec_name": "h264"},
	    {"index": 1, "codec_type": "audio", "codec_name": "aac", "channels": 2, "channel_layout": "stereo",
	     "tags": {"language": "jpn", "title": "主音声"}, "disposition": {"default": 1}},
	    {"index": 2, "codec_type": "audio", "codec_name": "ac3", "channels": 6, "channel_layout": "5.1(side)",
	     "tags": {"LANGUAGE": "eng", "title": "Commentary"}, "disposition": {"default": 0}},
	    {"index": 3, "codec_type": "subtitle", "codec_name": "subrip",
	     "tags": {"language": "und", "title": "SDH"}, "disposition": {"default": 0, "forced": 1, "hearing_impaired": 1}}
	  ],
	  "format": {"duration": "10.0"}
	}`)
	wantAudio := []domain.MediaAudioTrack{
		{Index: 1, Codec: "aac", Channels: 2, ChannelLayout: "stereo", Language: "jpn", Title: "主音声", Default: true},
		{Index: 2, Codec: "ac3", Channels: 6, ChannelLayout: "5.1(side)", Language: "eng", Title: "Commentary"},
	}
	if !slices.Equal(info.AudioTracks, wantAudio) {
		t.Errorf("AudioTracks = %+v", info.AudioTracks)
	}
	wantSubtitle := []domain.MediaSubtitleTrack{
		{Index: 3, Codec: "subrip", Title: "SDH", Forced: true, HearingImpaired: true},
	}
	if !slices.Equal(info.SubtitleTracks, wantSubtitle) {
		t.Errorf("SubtitleTracks = %+v", info.SubtitleTracks)
	}
}

func TestParseMediaInfoReadsAppleShootingTags(t *testing.T) {
	info := parseMediaFixture(t, `{
	  "streams": [
	    {"index": 0, "codec_type": "video", "codec_name": "hevc", "codec_tag_string": "hvc1"},
	    {"index": 1, "codec_type": "audio", "codec_name": "aac"}
	  ],
	  "format": {"duration": "10.0", "tags": {
	    "creation_time": "2024-05-01T03:34:56.000000Z",
	    "com.apple.quicktime.creationdate": "2024-05-01T12:34:56+0900",
	    "com.apple.quicktime.make": "Apple",
	    "com.apple.quicktime.model": "iPhone 15 Pro",
	    "com.apple.quicktime.location.ISO6709": "+35.6586+139.7454+040.123/"
	  }}
	}`)
	embedded := info.Embedded
	if embedded == nil {
		t.Fatal("Embedded が nil")
	}
	if embedded.RecordedAt != "2024-05-01T12:34:56+09:00" {
		t.Errorf("RecordedAt = %q", embedded.RecordedAt)
	}
	if embedded.Device != "Apple iPhone 15 Pro" {
		t.Errorf("Device = %q", embedded.Device)
	}
	location := embedded.Location
	if location == nil || location.Latitude != 35.6586 || location.Longitude != 139.7454 ||
		location.Altitude == nil || *location.Altitude != 40.123 {
		t.Errorf("Location = %+v", location)
	}
	if embedded.CoverArt {
		t.Error("CoverArt が立った")
	}
}

func TestParseMediaInfoReadsEmbeddedTagsAndCoverArt(t *testing.T) {
	info := parseMediaFixture(t, `{
	  "streams": [
	    {"index": 0, "codec_type": "video", "codec_name": "h264"},
	    {"index": 1, "codec_type": "video", "codec_name": "mjpeg", "disposition": {"attached_pic": 1}}
	  ],
	  "format": {"duration": "10.0", "tags": {
	    "TITLE": " 夏の記録 ", "COMMENT": "c", "synopsis": "あらすじ", "GENRE": "Travel", "DATE": "2024",
	    "album_artist": "Someone", "creation_time": "not a date", "location": "+95.0+139.0/"
	  }}
	}`)
	want := domain.MediaEmbedded{
		Title: "夏の記録", Comment: "c", Description: "あらすじ", Genre: "Travel", Date: "2024",
		Author: "Someone", CoverArt: true,
	}
	if info.Embedded == nil || *info.Embedded != want {
		t.Errorf("Embedded = %+v, want %+v", info.Embedded, want)
	}
}

func TestParseMediaInfoNormalizesChapters(t *testing.T) {
	info := parseMediaFixture(t, `{
	  "streams": [{"index": 0, "codec_type": "video", "codec_name": "h264"}],
	  "chapters": [
	    {"id": 1, "start_time": "60.000000", "end_time": "130.000000", "tags": {"title": "後半"}},
	    {"id": 0, "start_time": "0.000000", "end_time": "60.000000", "tags": {"title": "前半"}},
	    {"id": 2, "start_time": "125.000000", "end_time": "140.000000", "tags": {"title": "尺の後"}}
	  ],
	  "format": {"duration": "120.0"}
	}`)
	want := []domain.MediaChapter{
		{StartMs: 0, EndMs: 60_000, Title: "前半"},
		{StartMs: 60_000, EndMs: 120_000, Title: "後半"},
	}
	if !slices.Equal(info.Chapters, want) {
		t.Errorf("Chapters = %+v", info.Chapters)
	}
}

// int64 に収まらない時刻は端の値にそろえ、尺の後の開始を冒頭のチャプターにしない。
func TestParseMediaInfoClampsHugeChapterTimes(t *testing.T) {
	info := parseMediaFixture(t, `{
	  "streams": [{"index": 0, "codec_type": "video", "codec_name": "h264"}],
	  "chapters": [
	    {"id": 0, "start_time": "10.000000", "end_time": "1e300", "tags": {"title": "終わりが巨大"}},
	    {"id": 1, "start_time": "1e300", "end_time": "2e300", "tags": {"title": "開始が巨大"}},
	    {"id": 2, "start_time": "-1e300", "end_time": "5.000000", "tags": {"title": "開始が負に巨大"}}
	  ],
	  "format": {"duration": "120.0"}
	}`)
	want := []domain.MediaChapter{
		{StartMs: 0, EndMs: 5_000, Title: "開始が負に巨大"},
		{StartMs: 10_000, EndMs: 120_000, Title: "終わりが巨大"},
	}
	if !slices.Equal(info.Chapters, want) {
		t.Errorf("Chapters = %+v, want %+v", info.Chapters, want)
	}
}

// 何も持たないファイルは、映像の HDR = sdr だけの Media になる。
func TestParseMediaInfoWithNothingGivesSDROnly(t *testing.T) {
	info := parseMediaFixture(t, `{
	  "streams": [{"index": 0, "codec_type": "video", "codec_name": "h264", "width": 640, "height": 360}],
	  "format": {"duration": "4.0", "format_name": "mov,mp4"}
	}`)
	want := domain.MediaInfo{Video: &domain.MediaVideo{HDR: domain.HDRKindSDR}}
	if info.Video == nil || *info.Video != *want.Video || info.AudioTracks != nil || info.SubtitleTracks != nil ||
		info.Embedded != nil || info.Chapters != nil {
		t.Errorf("Media = %+v (video %+v)", info, info.Video)
	}
}

// 型の揺れる値や tags の形が違っても、解析は失敗しない（項目を省くだけ）。
func TestParseMediaInfoToleratesUnexpectedShapes(t *testing.T) {
	info := parseMediaFixture(t, `{
	  "streams": [{"index": 0, "codec_type": "video", "codec_name": "h264",
	    "side_data_list": [{"side_data_type": "Stereo 3D", "type": 3}, {"side_data_type": "Spherical Mapping", "projection": {"x": 1}}],
	    "tags": ["not", "an", "object"]}],
	  "chapters": [{"start_time": "N/A", "end_time": "1.0"}],
	  "format": {"duration": "4.0", "tags": {"title": 12}}
	}`)
	if info.Video == nil || info.Video.Stereo3D != "3" || info.Video.Projection != "" {
		t.Errorf("Video = %+v", info.Video)
	}
	if info.Chapters != nil || info.Embedded != nil {
		t.Errorf("Media = %+v", info)
	}
}

// 取り込みの解析は TS だけ長く読み、どれにもチャプターを足す。MP4 には上限を足さない。
func TestImportProbeArgs(t *testing.T) {
	for _, path := range []string{"/media/rec.ts", "/media/rec.M2TS", "/media/rec.mts"} {
		joined := strings.Join(importProbeArgs(path), " ")
		for _, want := range []string{"-show_chapters", "-analyzeduration 30M", "-probesize 100M", "-show_streams"} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: 引数に %q が無い: %s", path, want, joined)
			}
		}
	}
	args := importProbeArgs("/media/clip.mp4")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-show_chapters") {
		t.Errorf("MP4 にチャプターが無い: %s", joined)
	}
	if strings.Contains(joined, "-analyzeduration") || strings.Contains(joined, "-probesize") {
		t.Errorf("MP4 に上限が足された: %s", joined)
	}
	if args[len(args)-2] != "--" || args[len(args)-1] != "/media/clip.mp4" {
		t.Errorf("パスが最後でない: %v", args)
	}
}
