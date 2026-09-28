package media

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// software の引数は方式を足す前と 1 文字も変わらない（親 Issue #370 受け入れ条件 1）。
// 空の方式と知らない方式も software である。
func TestVideoEncodeArgsSoftwareUnchanged(t *testing.T) {
	odd := domain.TranscodeVideo{Width: 641, Height: 359, FPS: 30, RealFPS: 30, SampleAspectNum: 1, SampleAspectDen: 1}
	for _, tc := range []struct {
		stream domain.TranscodeVideo
		want   string
	}{
		{compatibleMetadata().Video, "-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23 -force_key_frames expr:gte(t,n_forced*2)"},
		{odd, "-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23 -force_key_frames expr:gte(t,n_forced*2) -vf pad=642:360:0:0,setsar=1/1*641/359*360/642:max=1000000"},
	} {
		for _, encoder := range []domain.VideoEncoder{domain.VideoEncoderSoftware, "", "unknown"} {
			if got := strings.Join(videoEncodeArgs(tc.stream, encoder), " "); got != tc.want {
				t.Errorf("encoder %q: args = %s, want %s", encoder, got, tc.want)
			}
		}
	}
}

// ハードウェアの方式は符号化器の指定だけを差し替え、High・Level 5.1・8bit 4:2:0 への変換・
// 時刻基準のキーフレーム・MOV の二入力は software と同じに保つ（research.md R-7）。
func TestTranscodeArgsForHardwareEncoders(t *testing.T) {
	metadata := compatibleMetadata()
	metadata.FormatName = "mov,mp4,m4a,3gp,3g2,mj2"
	metadata.Video.PixelFormat = "yuv420p10le"
	metadata.Video.BitsPerRawSample = 10
	metadata.Video.Width, metadata.Video.Height = 7680, 4320

	for _, tc := range []struct {
		encoder domain.VideoEncoder
		want    []string
	}{
		{domain.VideoEncoderNVENC, []string{"-c:v h264_nvenc -profile:v high -level:v 5.1 -pix_fmt yuv420p ", "-forced-idr 1"}},
		{domain.VideoEncoderQSV, []string{"-c:v h264_qsv -profile:v high -level 51 -pix_fmt nv12 ", "-forced_idr 1"}},
		{domain.VideoEncoderVAAPI, []string{"-c:v h264_vaapi -profile:v high -level 5.1 ", ",format=nv12,hwupload -"}},
		{domain.VideoEncoderVideoToolbox, []string{"-c:v h264_videotoolbox -profile:v high -level:v 5.1 -pix_fmt yuv420p "}},
	} {
		t.Run(string(tc.encoder), func(t *testing.T) {
			argv := buildTranscodeArgs("movie.mov", 0, metadata, false, false, tc.encoder)
			args := strings.Join(argv, " ")
			common := []string{
				"-an -i movie.mov -vn -i movie.mov -map 0:1 -map 1:2 ",
				"-force_key_frames expr:gte(t,n_forced*2)",
				"-vf scale=3840:2160,setsar=1/1*7680/4320*2160/3840:max=1000000",
				"-c:a copy",
				"-movflags frag_keyframe+empty_moov+default_base_moof -f mp4 pipe:1",
			}
			for _, want := range append(common, tc.want...) {
				if !strings.Contains(args, want) {
					t.Errorf("argsに %q がない: %s", want, args)
				}
			}
			device := slices.Index(argv, "-vaapi_device")
			if tc.encoder == domain.VideoEncoderVAAPI {
				if device < 0 || argv[device+1] != "/dev/dri/renderD128" || device > slices.Index(argv, "-i") {
					t.Errorf("入力より前に VAAPI のデバイスが無い: %s", args)
				}
			} else if device >= 0 {
				t.Errorf("VAAPI 以外でデバイスを開く: %s", args)
			}
		})
	}
}

// 映像をコピーできる要求は方式に依らずコピーし、デバイスも開かない（要件 12）。
func TestTranscodeArgsCopyIgnoresVideoEncoder(t *testing.T) {
	for _, encoder := range domain.HardwareVideoEncoders {
		for _, startMs := range []int64{0, 25000} {
			args := strings.Join(buildTranscodeArgs("movie.mkv", startMs, compatibleMetadata(), false, true, encoder), " ")
			want := strings.Join(transcodeArgs("movie.mkv", startMs, compatibleMetadata(), false), " ")
			if args != want || !strings.Contains(args, "-c:v copy") {
				t.Errorf("%s at %d: args = %s, want %s", encoder, startMs, args, want)
			}
		}
	}
}

func startHardware(
	t *testing.T, scripted *scriptedTranscoder, ctx context.Context, request domain.LiveTranscodeRequest,
) (domain.LiveTranscode, error) {
	t.Helper()
	path, stamp := sourceFile(t)
	stored := helperProbe()
	request.Path, request.Source, request.Probe = path, stamp, &stored
	if request.StartupDeadline.IsZero() {
		request.StartupDeadline = time.Now().Add(5 * time.Second)
	}
	return scripted.Start(ctx, request)
}

// ハードウェアで始めた変換が最初のデータを出せば、そのまま使い、切り替えない。
func TestLiveTranscoderStartsWithHardwareEncoder(t *testing.T) {
	scripted := newScriptedTranscoder("ffmpeg")
	started, err := startHardware(t, scripted, context.Background(), domain.LiveTranscodeRequest{
		Normalize: true, VideoEncoder: domain.VideoEncoderNVENC,
	})
	if err != nil {
		t.Fatal(err)
	}
	finishStarted(t, started)
	if started.VideoEncoder != domain.VideoEncoderNVENC || started.HardwareFailure != nil {
		t.Errorf("VideoEncoder = %q, HardwareFailure = %v", started.VideoEncoder, started.HardwareFailure)
	}
	if got := scripted.videoCodecs(); !slices.Equal(got, []string{"h264_nvenc"}) {
		t.Errorf("映像の扱い = %v", got)
	}
}

// ハードウェアが最初のデータを出さずに終わると、同じ要求の中で libx264 で始め直し、
// 切り替えの事実と FFmpeg の誤りを返す（要件 11）。コピーから始めた要求でも同じである。
func TestLiveTranscoderFallsBackToSoftwareWhenHardwareFails(t *testing.T) {
	for _, tc := range []struct {
		name      string
		normalize bool
		modes     []string
		want      []string
	}{
		{"encode", true, []string{"ffmpeg-fail", "ffmpeg"}, []string{"h264_vaapi", "libx264"}},
		{"after copy", false, []string{"ffmpeg-fail", "ffmpeg-fail", "ffmpeg"}, []string{"copy", "h264_vaapi", "libx264"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scripted := newScriptedTranscoder(tc.modes...)
			started, err := startHardware(t, scripted, context.Background(), domain.LiveTranscodeRequest{
				StartMs: 4000, Normalize: tc.normalize, VideoEncoder: domain.VideoEncoderVAAPI,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := string(finishStarted(t, started)); got != "fragment" || started.StartMs != 4000 {
				t.Errorf("初期データ = %q, StartMs = %d", got, started.StartMs)
			}
			if started.VideoEncoder != domain.VideoEncoderSoftware {
				t.Errorf("VideoEncoder = %q", started.VideoEncoder)
			}
			if started.HardwareFailure == nil || !strings.Contains(started.HardwareFailure.Error(), "decode failed") ||
				!strings.Contains(started.HardwareFailure.Error(), "vaapi") {
				t.Errorf("HardwareFailure = %v", started.HardwareFailure)
			}
			if got := scripted.videoCodecs(); !slices.Equal(got, tc.want) {
				t.Errorf("映像の扱い = %v, want %v", got, tc.want)
			}
			if started.Probed != nil {
				t.Errorf("その場の解析をした: %+v", started.Probed)
			}
		})
	}
}

// ハードウェアの期限切れは切り替えずに失敗にする。期限は要求全体で 1 つである。
func TestLiveTranscoderDoesNotFallBackAfterHardwareTimeout(t *testing.T) {
	scripted := newScriptedTranscoder("ffmpeg-silent", "ffmpeg")
	began := time.Now()
	_, err := startHardware(t, scripted, context.Background(), domain.LiveTranscodeRequest{
		Normalize: true, VideoEncoder: domain.VideoEncoderQSV, StartupDeadline: time.Now().Add(200 * time.Millisecond),
	})
	if err == nil {
		t.Fatal("データを出さないハードウェアで成功した")
	}
	if elapsed := time.Since(began); elapsed > 2*time.Second {
		t.Errorf("期限切れまで %s かかった", elapsed)
	}
	if got := scripted.videoCodecs(); !slices.Equal(got, []string{"h264_qsv"}) {
		t.Errorf("映像の扱い = %v", got)
	}
}

// 要求の取り消しでは切り替えない。
func TestLiveTranscoderDoesNotFallBackAfterCancellation(t *testing.T) {
	scripted := newScriptedTranscoder("ffmpeg-silent", "ffmpeg")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := startHardware(t, scripted, ctx, domain.LiveTranscodeRequest{
		Normalize: true, VideoEncoder: domain.VideoEncoderNVENC,
	})
	if err == nil {
		t.Fatal("取り消した要求が成功した")
	}
	if got := scripted.videoCodecs(); !slices.Equal(got, []string{"h264_nvenc"}) {
		t.Errorf("映像の扱い = %v", got)
	}
}

// 最初のデータを出したあとの失敗は切り替えず、Wait が誤りを返す。
func TestLiveTranscoderDoesNotFallBackAfterInitialData(t *testing.T) {
	scripted := newScriptedTranscoder("ffmpeg-data-then-fail", "ffmpeg")
	started, err := startHardware(t, scripted, context.Background(), domain.LiveTranscodeRequest{
		Normalize: true, VideoEncoder: domain.VideoEncoderNVENC,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = started.Stream.Close() }()
	if started.VideoEncoder != domain.VideoEncoderNVENC || started.HardwareFailure != nil {
		t.Errorf("VideoEncoder = %q, HardwareFailure = %v", started.VideoEncoder, started.HardwareFailure)
	}
	if err := started.Wait(); err == nil || !strings.Contains(err.Error(), "device lost") {
		t.Errorf("Wait = %v", err)
	}
	if got := scripted.videoCodecs(); !slices.Equal(got, []string{"h264_nvenc"}) {
		t.Errorf("映像の扱い = %v", got)
	}
}

// 映像をコピーできる要求は方式に依らずコピーで始め、VideoEncoder は空である（要件 12）。
func TestLiveTranscoderCopiesRegardlessOfVideoEncoder(t *testing.T) {
	scripted := newScriptedTranscoder("ffmpeg")
	started, err := startHardware(t, scripted, context.Background(), domain.LiveTranscodeRequest{
		VideoEncoder: domain.VideoEncoderNVENC,
	})
	if err != nil {
		t.Fatal(err)
	}
	finishStarted(t, started)
	if started.VideoEncoder != "" || started.HardwareFailure != nil {
		t.Errorf("VideoEncoder = %q, HardwareFailure = %v", started.VideoEncoder, started.HardwareFailure)
	}
	if got := scripted.videoCodecs(); !slices.Equal(got, []string{"copy"}) {
		t.Errorf("映像の扱い = %v", got)
	}
}
