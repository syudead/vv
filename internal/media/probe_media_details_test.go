package media

import (
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func ptr[T any](value T) *T { return &value }

func TestParseMediaInfoReadsVideoDetails(t *testing.T) {
	video := mediaVideoFixture(t, `{"index": 0, "codec_type": "video", "codec_name": "hevc", "profile": "Main 10",
		"codec_tag_string": "hvc1", "mime_codec_string": "hvc1.2.4.L153.B0", "width": 3840, "height": 2160,
		"pix_fmt": "yuv420p10le", "level": 153, "color_range": "tv", "color_transfer": "smpte2084",
		"r_frame_rate": "60/1", "avg_frame_rate": "30/1", "bit_rate": "24883200", "max_bit_rate": "40000000",
		"nb_frames": "300", "start_time": "0.500000", "duration": "10.000000",
		"disposition": {"default": 1, "multilayer": 1},
		"tags": {"encoder": "Lavc61 libx265", "handler_name": "VideoHandler", "vendor_id": "[0][0][0][0]",
			"creation_time": "2024-05-01T03:34:56.000000Z", "timecode": "01:00:00:00"},
		"side_data_list": [
			{"side_data_type": "Display Matrix", "displaymatrix": "\n00000000:       -65536           0           0\n00000001:            0       65536           0\n00000002:            0           0  1073741824\n", "rotation": 0},
			{"side_data_type": "Frame Cropping", "crop_top": 0, "crop_bottom": 8, "crop_left": 0, "crop_right": 0},
			{"side_data_type": "Mastering display metadata", "red_x": "34000/50000", "red_y": "16000/50000",
				"min_luminance": "50/10000", "max_luminance": "10000000/10000"},
			{"side_data_type": "Content light level metadata", "max_content": 1000, "max_average": 400},
			{"side_data_type": "HDR10+ Dynamic Metadata (SMPTE 2094-40)"},
			{"side_data_type": "DOVI configuration record", "dv_profile": 8, "dv_level": 6, "dv_bl_signal_compatibility_id": 1}
		]}`)
	want := domain.MediaVideo{
		FrameRate: 30, BitDepth: 10, BitRate: 24_883_200, HDR: domain.HDRKindDolbyVision,
		Profile: "Main 10", Level: 153, PixelFormat: "yuv420p10le", ChromaSubsampling: "4:2:0", ColorRange: "limited",
		CodecTag: "hvc1", CodecString: "hvc1.2.4.L153.B0", VariableFrameRate: ptr(true), FrameCount: 300,
		MaxBitRate: 40_000_000, BitsPerPixel: 0.1, Mirrored: true, Crop: &domain.MediaCrop{Bottom: 8},
		Mastering: &domain.MediaMasteringDisplay{RedX: ptr(0.68), RedY: ptr(0.32), MinLuminance: ptr(0.005),
			MaxLuminance: ptr(1000.0)},
		ContentLight: &domain.MediaContentLight{MaxContent: 1000, MaxAverage: 400}, HDR10Plus: true,
		DolbyVision: &domain.MediaDolbyVision{Profile: 8, Level: 6, Compatibility: 1}, MultiLayer: true,
		MediaTiming: domain.MediaTiming{StartMs: ptr[int64](500), DurationMs: ptr[int64](10_000)},
		MediaStreamOrigin: domain.MediaStreamOrigin{Encoder: "Lavc61 libx265", Handler: "VideoHandler",
			CreatedAt: "2024-05-01T03:34:56Z", Timecode: "01:00:00:00"},
	}
	if !reflect.DeepEqual(video, want) {
		t.Errorf("video =\n%+v\nwant\n%+v", video, want)
	}
}

func TestParseMediaInfoReadsTrackDetailsAndContainer(t *testing.T) {
	info := parseMediaFixture(t, `{
	  "streams": [
	    {"index": 0, "codec_type": "video", "codec_name": "h264", "codec_tag_string": "encv", "bit_rate": "4000000"},
	    {"index": 1, "codec_type": "audio", "codec_name": "flac", "sample_fmt": "s32", "sample_rate": "96000",
	     "bits_per_raw_sample": "24", "start_time": "1.200000",
	     "tags": {"NUMBER_OF_BYTES": "60000000", "DURATION": "00:00:09.500000000", "REPLAYGAIN_TRACK_GAIN": "-6.50 dB"},
	     "disposition": {"default": 1, "original": 1}},
	    {"index": 2, "codec_type": "audio", "codec_name": "aac", "sample_rate": "48000", "bit_rate": "128000",
	     "disposition": {"comment": 1, "visual_impaired": 1},
	     "side_data_list": [{"side_data_type": "Audio Service Type", "service_type": 5}]},
	    {"index": 3, "codec_type": "subtitle", "codec_name": "ass", "disposition": {"hearing_impaired": 1, "descriptions": 1}},
	    {"index": 4, "codec_type": "attachment", "codec_name": "ttf", "extradata_size": 51200,
	     "tags": {"filename": "font.ttf", "mimetype": "font/ttf"}},
	    {"index": 5, "codec_type": "data", "codec_name": "bin_data", "codec_tag_string": "gpmd",
	     "tags": {"handler_name": "GoPro MET"}}
	  ],
	  "format": {"duration": "10.0", "start_time": "-0.040000", "size": "70000000", "probe_score": 100,
	    "tags": {"major_brand": "isom", "compatible_brands": "isomiso2avc1mp41", "album": "Trip",
	      "show": "Show", "episode_id": "E01", "season_number": "2", "network": "NET", "copyright": "(c)",
	      "purl": "https://example.com/watch?v=1", "com.android.capture.fps": "240.0",
	      "com.android.version": "14"}}
	}`)
	wantAudio := domain.MediaAudioTrack{Index: 1, Codec: "flac", Default: true, SampleRate: 96_000, BitDepth: 24,
		SampleFormat: "s32", ReplayGain: &domain.MediaReplayGain{TrackGain: "-6.50 dB"},
		Roles:       domain.MediaRoles{Original: true},
		MediaTiming: domain.MediaTiming{StartMs: ptr[int64](1200), DurationMs: ptr[int64](9500)}}
	if len(info.AudioTracks) != 2 || !reflect.DeepEqual(info.AudioTracks[0], wantAudio) {
		t.Fatalf("AudioTracks = %+v", info.AudioTracks)
	}
	if second := info.AudioTracks[1]; second.ServiceType != "commentary" || second.BitRate != 128_000 ||
		second.Roles != (domain.MediaRoles{Comment: true, VisualImpaired: true}) {
		t.Errorf("2 本目の音声 = %+v", second)
	}
	if roles := info.SubtitleTracks[0].Roles; roles != (domain.MediaRoles{HearingImpaired: true, Descriptions: true}) {
		t.Errorf("字幕の役割 = %+v", roles)
	}
	// 4 Mbps over 10 s of video plus FLAC's exact bytes, with no subtitle bit rate: the subtitle and other parts stay unknown.
	wantContainer := domain.MediaContainer{
		StartMs: ptr[int64](-40), MajorBrand: "isom", CompatibleBrands: []string{"isom", "iso2", "avc1", "mp41"},
		ProbeScore: 100, Encrypted: true,
		Attachments: []domain.MediaAttachment{{Name: "font.ttf", MimeType: "font/ttf", Bytes: 51200}},
		DataStreams: []domain.MediaDataStream{{Index: 5, Codec: "gpmd", Handler: "GoPro MET"}},
		Sizes: &domain.MediaSizes{TotalBytes: 70_000_000, VideoBytes: ptr[int64](5_000_000),
			AudioBytes: ptr[int64](60_160_000)},
	}
	if !reflect.DeepEqual(info.Container, &wantContainer) {
		t.Errorf("Container =\n%+v\nwant\n%+v (sizes %+v)", info.Container, wantContainer, info.Container.Sizes)
	}
	embedded := info.Embedded
	if embedded == nil || embedded.Album != "Trip" || embedded.Show != "Show" || embedded.Episode != "E01" ||
		embedded.Season != "2" || embedded.Network != "NET" || embedded.Copyright != "(c)" ||
		embedded.SourceURL != "https://example.com/watch?v=1" || embedded.CaptureFrameRate != 240 ||
		embedded.CaptureSoftware != "14" {
		t.Errorf("Embedded = %+v", embedded)
	}
}

func TestParseFilmGrain(t *testing.T) {
	if got := parseFilmGrain([]byte(`{"streams": [{"film_grain": 1}]}`)); got == nil || !*got {
		t.Errorf("film_grain 1 = %v, want true", got)
	}
	if got := parseFilmGrain([]byte(`{"streams": [{"film_grain": 0}]}`)); got == nil || *got {
		t.Errorf("film_grain 0 = %v, want false", got)
	}
	// ffprobe without -analyze_frames prints nothing for the stream.
	if got := parseFilmGrain([]byte(`{"streams": [{}]}`)); got != nil {
		t.Errorf("項目なし = %v, want nil", *got)
	}
	args := filmGrainProbeArgs("-clip.mkv")
	joined := strings.Join(args, " ")
	for _, want := range []string{"-select_streams v:0", "-analyze_frames", "-export_side_data film_grain", "-read_intervals %+#1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("引数に %q が無い: %s", want, joined)
		}
	}
	if args[len(args)-2] != "--" || args[len(args)-1] != "-clip.mkv" {
		t.Errorf("パスが最後でない: %v", args)
	}
}

func TestParseMediaInfoDropsCountsBeyondInt32(t *testing.T) {
	video := mediaVideoFixture(t, `{"index": 0, "codec_type": "video", "codec_name": "hevc",
		"side_data_list": [{"side_data_type": "Frame Cropping", "crop_top": 4294967296, "crop_bottom": 8}]}`)
	if video.Crop == nil || *video.Crop != (domain.MediaCrop{Bottom: 8}) {
		t.Errorf("Crop = %+v, want 読めない上だけ省く", video.Crop)
	}
}
