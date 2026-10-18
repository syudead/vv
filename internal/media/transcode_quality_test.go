package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func TestVideoEncodeArgsScalesToQuality(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stream  domain.TranscodeVideo
		quality domain.TranscodeQuality
		want    string
	}{
		{"landscape 480p", domain.TranscodeVideo{Width: 1920, Height: 1080}, "480p", "-vf scale=854:480,setsar=1/1*1920/1080*480/854:max=1000000"},
		{"portrait 480p", domain.TranscodeVideo{Width: 1080, Height: 1920}, "480p", "-vf scale=480:854,setsar=1/1*1080/1920*854/480:max=1000000"},
		{"rotated 480p", domain.TranscodeVideo{Width: 1920, Height: 1080, Rotation: 90}, "480p", "-vf scale=480:854,setsar=1/1*1080/1920*854/480:max=1000000"},
		{"portrait 720p", domain.TranscodeVideo{Width: 1080, Height: 1920}, "720p", "-vf scale=720:1280,"},
		{"4K 1080p", domain.TranscodeVideo{Width: 3840, Height: 2160}, "1080p", "-vf scale=1920:1080,"},
		{"narrow 1080p", domain.TranscodeVideo{Width: 1200, Height: 12000}, "1080p", "-vf scale=384:3840,"},
		{"narrow 720p", domain.TranscodeVideo{Width: 1200, Height: 12000}, "720p", "-vf scale=384:3840,"},
		{"narrow 480p", domain.TranscodeVideo{Width: 1200, Height: 12000}, "480p", "-vf scale=384:3840,"},
		{"narrow 360p", domain.TranscodeVideo{Width: 1200, Height: 12000}, "360p", "-vf scale=360:3600,"},
		{"wide 480p", domain.TranscodeVideo{Width: 12000, Height: 1200}, "480p", "-vf scale=3840:384,"},
		{"wide 360p", domain.TranscodeVideo{Width: 12000, Height: 1200}, "360p", "-vf scale=3600:360,"},
		{"non-square SAR", domain.TranscodeVideo{Width: 1440, Height: 1080, SampleAspectNum: 4, SampleAspectDen: 3}, "480p", "-vf scale=640:480,setsar=4/3*1440/1080*480/640:max=1000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.stream.FPS, tc.stream.RealFPS = 30, 30
			args := strings.Join(videoEncodeArgs(tc.stream, domain.VideoEncoderSoftware, tc.quality), " ")
			if !strings.Contains(args, tc.want) {
				t.Errorf("argsに %q がない: %s", tc.want, args)
			}
		})
	}
}

// Round to the nearest even dimensions preserving aspect ratio.
func TestQualityDimensionsRoundsToNearestEven(t *testing.T) {
	for _, tc := range []struct {
		width, height, shortSide int
		wantWidth, wantHeight    int
	}{
		{1920, 1080, 480, 854, 480}, // 853.33 → 854
		{1920, 1080, 720, 1280, 720},
		{1920, 1080, 360, 640, 360},
		{1280, 1024, 480, 600, 480},   // 600.0
		{1000, 998, 360, 360, 360},    // 360.72 → 360
		{1001, 719, 480, 668, 480},    // 668.26 → 668
		{12000, 1200, 480, 3840, 384}, // Global bounds take precedence.
	} {
		width, height := qualityDimensions(tc.width, tc.height, tc.shortSide)
		if width != tc.wantWidth || height != tc.wantHeight {
			t.Errorf("%dx%d → %d: %dx%d, want %dx%d", tc.width, tc.height, tc.shortSide, width, height, tc.wantWidth, tc.wantHeight)
		}
	}
}

func TestTranscodeArgsWithQualityEncodesCopyableVideo(t *testing.T) {
	for _, startMs := range []int64{0, 25000} {
		args := strings.Join(buildTranscodeArgs("movie.mkv", startMs, compatibleMetadata(), false, true, domain.VideoEncoderSoftware, "480p"), " ")
		for _, want := range []string{
			"-c:v libx264 -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset superfast -crf 23 -maxrate 1200k -bufsize 2400k -force_key_frames expr:gte(t,n_forced*2) -vf scale=854:480,",
			"-c:a aac -profile:a aac_low -ac 2 -b:a 96k -ar 48000",
			"-movflags frag_keyframe+empty_moov+default_base_moof -f mp4 pipe:1",
		} {
			if !strings.Contains(args, want) {
				t.Errorf("at %d: argsに %q がない: %s", startMs, want, args)
			}
		}
		for _, unwanted := range []string{"-c:v copy", "-c:a copy", "-copyts", "delay_moov", "-noaccurate_seek"} {
			if strings.Contains(args, unwanted) {
				t.Errorf("at %d: argsに %q がある: %s", startMs, unwanted, args)
			}
		}
	}
}

func TestTranscodeArgsQualityLimits(t *testing.T) {
	metadata := compatibleMetadata()
	metadata.Video.Width, metadata.Video.Height = 3840, 2160
	for _, tc := range []struct {
		quality domain.TranscodeQuality
		want    []string
	}{
		{"1080p", []string{"-maxrate 5000k -bufsize 10000k", "-b:a 128k"}},
		{"720p", []string{"-maxrate 2500k -bufsize 5000k", "-b:a 128k"}},
		{"480p", []string{"-maxrate 1200k -bufsize 2400k", "-b:a 96k"}},
		{"360p", []string{"-maxrate 700k -bufsize 1400k", "-b:a 64k"}},
	} {
		args := strings.Join(buildTranscodeArgs("movie.mkv", 0, metadata, false, true, domain.VideoEncoderSoftware, tc.quality), " ")
		for _, want := range tc.want {
			if !strings.Contains(args, want) {
				t.Errorf("%s: argsに %q がない: %s", tc.quality, want, args)
			}
		}
	}
}

func TestTranscodeArgsQualityForHardwareEncoders(t *testing.T) {
	metadata := compatibleMetadata()
	metadata.FormatName = "mov,mp4,m4a,3gp,3g2,mj2"
	for _, tc := range []struct {
		encoder domain.VideoEncoder
		want    string
		removed string
	}{
		{domain.VideoEncoderNVENC, "-c:v h264_nvenc -profile:v high -level:v 5.1 -pix_fmt yuv420p -preset p4 -rc vbr -cq 23 -b:v 0 -forced-idr 1 -maxrate 1200k -bufsize 2400k ", ""},
		{domain.VideoEncoderQSV, "-c:v h264_qsv -profile:v high -level 51 -pix_fmt nv12 -preset veryfast -b:v 1200k -maxrate 1200k -bufsize 2400k -look_ahead 0 -forced_idr 1 ", "-global_quality"},
		{domain.VideoEncoderVAAPI, "-c:v h264_vaapi -profile:v high -level 5.1 -rc_mode VBR -b:v 1200k -maxrate 1200k -bufsize 2400k ", "-qp"},
		{domain.VideoEncoderVideoToolbox, "-c:v h264_videotoolbox -profile:v high -level:v 5.1 -pix_fmt yuv420p -b:v 1200k -maxrate 1200k -bufsize 2400k -realtime 1 ", "-q:v"},
		{domain.VideoEncoderSoftware, "-crf 23 -maxrate 1200k -bufsize 2400k ", ""},
	} {
		t.Run(string(tc.encoder), func(t *testing.T) {
			args := strings.Join(buildTranscodeArgs("movie.mov", 25000, metadata, false, true, tc.encoder, "480p"), " ")
			vaapiFilter := ""
			if tc.encoder == domain.VideoEncoderVAAPI {
				vaapiFilter = ",format=nv12,hwupload"
			}
			for _, want := range []string{
				tc.want,
				"-an -i movie.mov -ss 25.000 -vn -i movie.mov -map 0:1 -map 1:2 ",
				"-force_key_frames expr:gte(t,n_forced*2) -vf scale=854:480,setsar=1/1*1920/1080*480/854:max=1000000" + vaapiFilter + " ",
				"-c:a aac -profile:a aac_low -ac 2 -b:a 96k -ar 48000",
				"-movflags frag_keyframe+empty_moov+default_base_moof -f mp4 pipe:1",
			} {
				if !strings.Contains(args, want) {
					t.Errorf("argsに %q がない: %s", want, args)
				}
			}
			if tc.removed != "" && strings.Contains(args, tc.removed) {
				t.Errorf("argsに一定品質の %q が残る: %s", tc.removed, args)
			}
		})
	}
}

func TestLiveTranscoderQualityEncodesAndKeepsLimitsOnFallback(t *testing.T) {
	path, stamp := sourceFile(t)
	stored := compatibleMetadata()
	scripted := newScriptedTranscoder("ffmpeg-fail", "ffmpeg")
	started, err := scripted.Start(context.Background(), domain.LiveTranscodeRequest{
		Path: path, Source: stamp, Probe: &stored, StartupDeadline: time.Now().Add(5 * time.Second),
		StartMs: 4000, VideoEncoder: domain.VideoEncoderVAAPI, Quality: "360p",
	})
	if err != nil {
		t.Fatal(err)
	}
	finishStarted(t, started)
	if got := scripted.videoCodecs(); !slices.Equal(got, []string{"h264_vaapi", "libx264"}) || started.StartMs != 4000 {
		t.Fatalf("映像の扱い = %v, StartMs = %d", got, started.StartMs)
	}
	software := strings.Join(scripted.arguments()[1], " ")
	for _, want := range []string{"-maxrate 700k -bufsize 1400k", "scale=640:360,", "-b:a 64k"} {
		if !strings.Contains(software, want) {
			t.Errorf("software の argsに %q がない: %s", want, software)
		}
	}
}

func TestTranscodeQualityCapsBitrateWithFFmpeg(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath(transcodeCommand); err != nil {
		t.Skip("ffmpegがありません")
	}
	if _, err := exec.LookPath(probeCommand); err != nil {
		t.Skip("ffprobeがありません")
	}

	for _, tc := range []struct {
		name                  string
		size                  string
		wantWidth, wantHeight int
	}{
		{"landscape", "960x540", 854, 480},
		{"portrait", "540x960", 480, 854},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			input := filepath.Join(directory, "input.mp4")
			// Use full-frame noise to exceed uncapped constant-quality bitrate; copyable H.264/AAC also
			// verifies quality forces encoding.
			generate := exec.Command(transcodeCommand,
				"-hide_banner", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%s:rate=30:duration=20,noise=alls=60:allf=t", tc.size),
				"-f", "lavfi", "-i", "sine=frequency=440:duration=20",
				"-c:v", "libx264", "-preset", "ultrafast", "-crf", "18", "-pix_fmt", "yuv420p",
				"-c:a", "aac", "-b:a", "256k", "-shortest", input,
			)
			if output, err := generate.CombinedOutput(); err != nil {
				t.Fatalf("fixture生成: %v: %s", err, output)
			}
			probeInput, err := exec.Command(probeCommand, probeArgs(input)...).Output()
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := parseTranscodeProbe(probeInput)
			if err != nil {
				t.Fatal(err)
			}

			output := filepath.Join(directory, "output.mp4")
			outputFile, err := os.Create(output)
			if err != nil {
				t.Fatal(err)
			}
			args := buildTranscodeArgs(input, 0, metadata, false, true, domain.VideoEncoderSoftware, domain.TranscodeQuality480p)
			command := exec.Command(transcodeCommand, args...)
			command.Stdout = outputFile
			var stderr strings.Builder
			command.Stderr = &stderr
			runErr := command.Run()
			closeErr := outputFile.Close()
			if runErr != nil || closeErr != nil {
				t.Fatalf("変換: run=%v close=%v stderr=%s", runErr, closeErr, stderr.String())
			}

			probeOutputJSON, err := exec.Command(probeCommand, probeArgs(output)...).Output()
			if err != nil {
				t.Fatal(err)
			}
			var probed probeOutput
			if err := json.Unmarshal(probeOutputJSON, &probed); err != nil {
				t.Fatal(err)
			}
			if len(probed.Streams) == 0 {
				t.Fatal("出力に映像streamがありません")
			}
			if video := probed.Streams[0]; video.Width != tc.wantWidth || video.Height != tc.wantHeight {
				t.Fatalf("出力寸法 = %dx%d, want %dx%d", video.Width, video.Height, tc.wantWidth, tc.wantHeight)
			}

			videoKbps := averagePacketKbps(t, output, "v:0")
			if limit := 1200 * 1.2; videoKbps > limit {
				t.Errorf("映像の平均ビットレート %.0f kbps が %.0f kbps を超えます", videoKbps, limit)
			}
			if audioKbps := averagePacketKbps(t, output, "a:0"); audioKbps < 96*0.8 || audioKbps > 96*1.2 {
				t.Errorf("音声の平均ビットレート %.0f kbps が 96 kbps 付近ではありません", audioKbps)
			}
		})
	}
}

// Compute bitrate from packet bytes and PTS span because fragmented MP4 may omit bit_rate.
func averagePacketKbps(t *testing.T, path, stream string) float64 {
	t.Helper()
	packets, err := exec.Command(probeCommand,
		"-v", "error", "-select_streams", stream,
		"-show_entries", "packet=pts_time,size", "-of", "csv=p=0", path,
	).Output()
	if err != nil {
		t.Fatal(err)
	}
	first, last, bytes := -1.0, 0.0, 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(packets)), "\n") {
		ptsText, sizeText, _ := strings.Cut(line, ",")
		pts := parseFrameRate(ptsText)
		if first < 0 || pts < first {
			first = pts
		}
		last = max(last, pts)
		bytes += parsePositiveInt(strings.TrimSpace(sizeText))
	}
	if last <= first {
		t.Fatalf("%s のパケットの時刻が足りません: %s", stream, packets)
	}
	return float64(bytes) * 8 / 1000 / (last - first)
}
