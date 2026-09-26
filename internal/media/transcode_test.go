package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func compatibleMetadata() domain.TranscodeProbe {
	audio := domain.TranscodeAudio{Index: 2, CodecName: "aac", Profile: "LC", SampleRate: 48000, Channels: 2}
	return domain.TranscodeProbe{
		FormatName: "matroska,webm",
		Video:      domain.TranscodeVideo{Index: 1, CodecName: "h264", Profile: "High", PixelFormat: "yuv420p", BitsPerRawSample: 8, Width: 1920, Height: 1080, Level: 41, FPS: 30, RealFPS: 30, SampleAspectNum: 1, SampleAspectDen: 1},
		Audio:      &audio,
	}
}

func TestParseTranscodeProbeSkipsAttachedPicture(t *testing.T) {
	const output = `{"streams":[
		{"index":0,"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}},
		{"index":1,"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p","bits_per_raw_sample":"8","width":1920,"height":1080,"level":41,"avg_frame_rate":"30000/1001","r_frame_rate":"30000/1001","sample_aspect_ratio":"4:3","side_data_list":[{"side_data_type":"Display Matrix","rotation":-90}]},
		{"index":2,"codec_type":"audio","codec_name":"aac","profile":"LC","sample_rate":"48000","channels":2}
	],"format":{"duration":"12.5","format_name":"MOV,MP4,M4A,3GP,3G2,MJ2"}}`
	got, err := parseTranscodeProbe([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	if got.FormatName != "mov,mp4,m4a,3gp,3g2,mj2" || got.Video.Index != 1 || got.Video.Rotation != 270 || got.Video.SampleAspectNum != 4 || got.Video.SampleAspectDen != 3 || got.Audio == nil || got.Audio.Index != 2 {
		t.Fatalf("stream selection = %+v", got)
	}
}

func TestParseTranscodeProbeDisplayMatrixZeroOverridesRotateTag(t *testing.T) {
	const output = `{"streams":[{
		"index":0,"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p","bits_per_raw_sample":"8",
		"width":1920,"height":1080,"level":41,"avg_frame_rate":"30/1","r_frame_rate":"30/1","sample_aspect_ratio":"1:1",
		"tags":{"rotate":"90"},"side_data_list":[{"side_data_type":"Display Matrix","rotation":0}]
	}],"format":{"duration":"12.5"}}`
	got, err := parseTranscodeProbe([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	if got.Video.Rotation != 0 {
		t.Fatalf("rotation = %d, want 0", got.Video.Rotation)
	}
}

func TestParseTranscodeProbeRejectsAttachedPictureOnly(t *testing.T) {
	const output = `{"streams":[{"index":0,"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}}],"format":{"duration":"12.5"}}`
	if _, err := parseTranscodeProbe([]byte(output)); err == nil {
		t.Fatal("添付画像だけなのに成功した")
	}
}

func TestTranscodeArgsCopiesCompatibleStreams(t *testing.T) {
	args := strings.Join(transcodeArgs("movie.mkv", 0, compatibleMetadata(), false), " ")
	for _, want := range []string{"-map 0:1", "-map 0:2", "-c:v copy", "-c:a copy", "frag_keyframe+empty_moov+default_base_moof", "-f mp4 pipe:1"} {
		if !strings.Contains(args, want) {
			t.Errorf("argsに %q がない: %s", want, args)
		}
	}
}

func TestTranscodeArgsEncodesUnsafeStreams(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.TranscodeProbe)
		want   string
	}{
		{"10-bit H264", func(m *domain.TranscodeProbe) { m.Video.PixelFormat = "yuv420p10le" }, "-c:v libx264"},
		{"High 4:4:4 10-bit", func(m *domain.TranscodeProbe) {
			m.Video.Profile = "High 4:4:4 Predictive"
			m.Video.PixelFormat = "yuv444p10le"
			m.Video.BitsPerRawSample = 10
		}, "-c:v libx264"},
		{"96kHz AAC", func(m *domain.TranscodeProbe) { m.Audio.SampleRate = 96000 }, "-c:a aac -profile:a aac_low -ac 2 -b:a 192k -ar 48000"},
		{"unknown video attribute", func(m *domain.TranscodeProbe) { m.Video.BitsPerRawSample = 0 }, "-c:v libx264"},
		{"variable frame rate", func(m *domain.TranscodeProbe) { m.Video.RealFPS = 120 }, "-c:v libx264"},
		{"rotated oversized coded frame", func(m *domain.TranscodeProbe) {
			m.Video.Width = 2160
			m.Video.Height = 4096
			m.Video.Rotation = 90
		}, "-c:v libx264"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metadata := compatibleMetadata()
			tc.mutate(&metadata)
			args := strings.Join(transcodeArgs("movie.mkv", 0, metadata, false), " ")
			if !strings.Contains(args, tc.want) {
				t.Errorf("argsに %q がない: %s", tc.want, args)
			}
		})
	}
}

func TestTranscodeArgsNormalizesSeek(t *testing.T) {
	args := strings.Join(transcodeArgs("movie.mkv", 25000, compatibleMetadata(), false), " ")
	for _, want := range []string{"-ss 25.000 -i movie.mkv", "-c:v libx264", "-preset superfast", "-c:a aac"} {
		if !strings.Contains(args, want) {
			t.Errorf("argsに %q がない: %s", want, args)
		}
	}
}

func TestTranscodeArgsReadsMOVTracksSeparately(t *testing.T) {
	metadata := compatibleMetadata()
	metadata.FormatName = "mov,mp4,m4a,3gp,3g2,mj2"
	args := strings.Join(transcodeArgs("movie.mov", 25000, metadata, false), " ")
	for _, want := range []string{
		"-ss 25.000 -an -i movie.mov -ss 25.000 -vn -i movie.mov",
		"-map 0:1 -map 1:2",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("argsに %q がない: %s", want, args)
		}
	}
	if strings.Contains(args, "-interleaved_read") {
		t.Fatalf("再生を損なう可能性のあるMOV demuxer optionがある: %s", args)
	}
}

func TestMOVTranscodeReadsVideoAndAudioFromSeparateInputs(t *testing.T) {
	if _, err := exec.LookPath(transcodeCommand); err != nil {
		t.Skip("ffmpegがありません")
	}
	if _, err := exec.LookPath(probeCommand); err != nil {
		t.Skip("ffprobeがありません")
	}

	directory := t.TempDir()
	input := filepath.Join(directory, "interleaved.mov")
	generate := exec.Command(transcodeCommand,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=15:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", input,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("MOV fixture生成: %v: %s", err, output)
	}

	probeInput, err := exec.Command(probeCommand, probeArgs(input)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := parseTranscodeProbe(probeInput)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Audio == nil || !usesMOVDemuxer(metadata.FormatName) {
		t.Fatalf("MOV fixtureのmetadataが不正です: %+v", metadata)
	}

	output := filepath.Join(directory, "output.mp4")
	outputFile, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(transcodeCommand, transcodeArgs(input, 500, metadata, false)...)
	command.Stdout = outputFile
	var stderr strings.Builder
	command.Stderr = &stderr
	runErr := command.Run()
	closeErr := outputFile.Close()
	if runErr != nil || closeErr != nil {
		t.Fatalf("MOV変換: run=%v close=%v stderr=%s", runErr, closeErr, stderr.String())
	}

	probeOutputJSON, err := exec.Command(probeCommand, probeArgs(output)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	converted, err := parseTranscodeProbe(probeOutputJSON)
	if err != nil {
		t.Fatal(err)
	}
	if converted.Audio == nil {
		t.Fatal("別入力から変換した出力に音声streamがありません")
	}
}

func TestMOVTranscodeStopsAfterClientCancellation(t *testing.T) {
	if _, err := exec.LookPath(transcodeCommand); err != nil {
		t.Skip("ffmpegがありません")
	}
	if _, err := exec.LookPath(probeCommand); err != nil {
		t.Skip("ffprobeがありません")
	}

	directory := t.TempDir()
	input := filepath.Join(directory, "long.mov")
	generate := exec.Command(transcodeCommand,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=160x90:rate=15:duration=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=30",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", input,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("MOV fixture生成: %v: %s", err, output)
	}

	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	transcoder := NewLiveTranscoder(nil)
	started, err := transcoder.Start(context.Background(), domain.LiveTranscodeRequest{
		Path: input, Source: domain.FileStampOf(info), Normalize: true, StartupDeadline: time.Now().Add(5 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.Probed == nil {
		t.Error("その場で解析したのに結果を返さない")
	}
	if _, err := readInitialBytes(started.Stream); err != nil {
		t.Fatalf("初期データ取得: %v", err)
	}

	stoppedAt := time.Now()
	started.Stop()
	_ = started.Stream.Close()
	if err := started.Wait(); err == nil {
		t.Fatal("cancelしたFFmpegが成功終了しました")
	}
	if elapsed := time.Since(stoppedAt); elapsed > 2*time.Second {
		t.Fatalf("cancel後のFFmpeg終了に %s かかりました", elapsed)
	}
}

func readInitialBytes(stream io.Reader) ([]byte, error) {
	buffer := make([]byte, 32*1024)
	length, err := io.ReadAtLeast(stream, buffer, 1)
	return buffer[:length], err
}

func TestVideoEncodeArgsNormalizesDimensionsAndRate(t *testing.T) {
	tests := []struct {
		name   string
		stream domain.TranscodeVideo
		want   string
	}{
		{"odd dimensions", domain.TranscodeVideo{Width: 641, Height: 359, FPS: 30, RealFPS: 30, SampleAspectNum: 1, SampleAspectDen: 1}, "-vf pad=642:360:0:0,setsar=1/1*641/359*360/642:max=1000000"},
		{"8K60", domain.TranscodeVideo{Width: 7680, Height: 4320, FPS: 60, RealFPS: 60}, "-vf scale=3840:2160,setsar=1/1*7680/4320*2160/3840:max=1000000,fps=30.340"},
		{"rotated 4K", domain.TranscodeVideo{Width: 3840, Height: 2160, Rotation: 90, FPS: 30, RealFPS: 30}, "-vf setsar=1/1*2160/3840*3840/2160:max=1000000"},
		{"portrait 8K", domain.TranscodeVideo{Width: 4320, Height: 7680, FPS: 30, RealFPS: 30}, "-vf scale=2160:3840,setsar=1/1*4320/7680*3840/2160:max=1000000"},
		{"rotated 8K", domain.TranscodeVideo{Width: 7680, Height: 4320, Rotation: 90, FPS: 30, RealFPS: 30}, "-vf scale=2160:3840,setsar=1/1*4320/7680*3840/2160:max=1000000"},
		{"rotated HD", domain.TranscodeVideo{Width: 1920, Height: 1080, Rotation: 270, FPS: 30, RealFPS: 30}, "-vf setsar=1/1*1080/1920*1920/1080:max=1000000"},
		{"unknown rate", domain.TranscodeVideo{Width: 1920, Height: 1080}, "-vf fps=30.000"},
		{"VFR peak", domain.TranscodeVideo{Width: 1920, Height: 1080, FPS: 30, RealFPS: 120}, "-vf fps=30"},
		{"very low VFR", domain.TranscodeVideo{Width: 1920, Height: 1080, FPS: 0.0005, RealFPS: 1}, "-vf fps=0.0005"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := strings.Join(videoEncodeArgs(tc.stream), " ")
			if !strings.Contains(args, tc.want) {
				t.Errorf("argsに %q がない: %s", tc.want, args)
			}
		})
	}
}

func TestVideoEncodeArgsForcesKeyframesByTime(t *testing.T) {
	args := strings.Join(videoEncodeArgs(compatibleMetadata().Video), " ")
	if want := "-force_key_frames expr:gte(t,n_forced*2)"; !strings.Contains(args, want) {
		t.Fatalf("argsに %q がない: %s", want, args)
	}
}

func TestTranscodeKeyframeIntervalWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath(transcodeCommand); err != nil {
		t.Skip("ffmpegがありません")
	}
	if _, err := exec.LookPath(probeCommand); err != nil {
		t.Skip("ffprobeがありません")
	}

	for _, rate := range []int{24, 30, 60} {
		t.Run(fmt.Sprintf("%dfps", rate), func(t *testing.T) {
			directory := t.TempDir()
			input := filepath.Join(directory, "input.mkv")
			// 入力のキーフレームは疎にし、出力の間隔がエンコード側で決まることを確かめる。
			generate := exec.Command(transcodeCommand,
				"-hide_banner", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=320x180:rate=%d:duration=10", rate),
				"-c:v", "libx264", "-preset", "ultrafast", "-g", "600", "-pix_fmt", "yuv420p", input,
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
			command := exec.Command(transcodeCommand, transcodeArgs(input, 1500, metadata, true)...)
			command.Stdout = outputFile
			var stderr strings.Builder
			command.Stderr = &stderr
			runErr := command.Run()
			closeErr := outputFile.Close()
			if runErr != nil || closeErr != nil {
				t.Fatalf("変換: run=%v close=%v stderr=%s", runErr, closeErr, stderr.String())
			}

			packets, err := exec.Command(probeCommand,
				"-v", "error", "-select_streams", "v:0",
				"-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", output,
			).Output()
			if err != nil {
				t.Fatal(err)
			}
			var keyframes []float64
			last := 0.0
			for line := range strings.SplitSeq(strings.TrimSpace(string(packets)), "\n") {
				ptsText, flags, _ := strings.Cut(line, ",")
				pts := parseFrameRate(ptsText)
				last = max(last, pts)
				if strings.Contains(flags, "K") {
					keyframes = append(keyframes, pts)
				}
			}
			if len(keyframes) < 2 {
				t.Fatalf("キーフレームが少なすぎます: %v", keyframes)
			}
			keyframes = append(keyframes, last)
			for i := 1; i < len(keyframes); i++ {
				if gap := keyframes[i] - keyframes[i-1]; gap > liveKeyframeInterval+0.001 {
					t.Fatalf("キーフレームの間隔 %.3f 秒が %d 秒を超えます: %v", gap, liveKeyframeInterval, keyframes)
				}
			}
		})
	}
}

func TestVideoEncodePreservesDisplayAspectRatioWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath(transcodeCommand); err != nil {
		t.Skip("ffmpegがありません")
	}
	if _, err := exec.LookPath(probeCommand); err != nil {
		t.Skip("ffprobeがありません")
	}

	directory := t.TempDir()
	input := filepath.Join(directory, "odd-sar.mkv")
	generate := exec.Command(transcodeCommand,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=641x359:rate=15:duration=1",
		"-vf", "setsar=4/3", "-c:v", "ffv1", input,
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

	output := filepath.Join(directory, "normalized.mp4")
	outputFile, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(transcodeCommand, transcodeArgs(input, 0, metadata, false)...)
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
	video := probed.Streams[0]
	if video.Width != 642 || video.Height != 360 {
		t.Fatalf("出力寸法 = %dx%d", video.Width, video.Height)
	}
	numerator, denominator := parseAspectRatio(video.SampleAspectRatio)
	inputDAR := float64(641*4) / float64(359*3)
	outputDAR := float64(video.Width) * float64(numerator) / (float64(video.Height) * float64(denominator))
	if math.Abs(inputDAR-outputDAR) > 0.00001 {
		t.Fatalf("display aspect ratio: input=%f output=%f (SAR=%s)", inputDAR, outputDAR, video.SampleAspectRatio)
	}
}

func TestTranscodeRotated4KStaysWithinEnvelopeWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath(transcodeCommand); err != nil {
		t.Skip("ffmpegがありません")
	}
	if _, err := exec.LookPath(probeCommand); err != nil {
		t.Skip("ffprobeがありません")
	}

	directory := t.TempDir()
	base := filepath.Join(directory, "base.mp4")
	generate := exec.Command(transcodeCommand,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=blue:size=3840x2160:rate=1:duration=1",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", base,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("4K fixture生成: %v: %s", err, output)
	}

	input := filepath.Join(directory, "rotated.mp4")
	rotate := exec.Command(transcodeCommand,
		"-hide_banner", "-loglevel", "error", "-y",
		"-display_rotation:v:0", "90", "-i", base, "-c", "copy", input,
	)
	if output, err := rotate.CombinedOutput(); err != nil {
		t.Fatalf("回転metadata付与: %v: %s", err, output)
	}

	probeInput, err := exec.Command(probeCommand, probeArgs(input)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := parseTranscodeProbe(probeInput)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Video.Rotation != 90 && metadata.Video.Rotation != 270 {
		t.Fatalf("rotation = %d", metadata.Video.Rotation)
	}

	output := filepath.Join(directory, "normalized.mp4")
	outputFile, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(transcodeCommand, transcodeArgs(input, 0, metadata, false)...)
	command.Stdout = outputFile
	var stderr strings.Builder
	command.Stderr = &stderr
	runErr := command.Run()
	closeErr := outputFile.Close()
	if runErr != nil || closeErr != nil {
		t.Fatalf("回転動画変換: run=%v close=%v stderr=%s", runErr, closeErr, stderr.String())
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
	video := probed.Streams[0]
	if exceedsVideoBounds(video.Width, video.Height) {
		t.Fatalf("出力寸法 = %dx%d", video.Width, video.Height)
	}
	normalized, err := parseTranscodeProbe(probeOutputJSON)
	if err != nil {
		t.Fatal(err)
	}
	// 縦長の 4K は上限に収まるのでそのまま流す。回転の印が残っても、表示される比率は保つ。
	displayWidth, displayHeight, numerator, denominator := displayGeometry(normalized.Video)
	inputDAR := float64(2160) / float64(3840)
	outputDAR := float64(displayWidth) * float64(numerator) / (float64(displayHeight) * float64(denominator))
	if math.Abs(inputDAR-outputDAR) > 0.00001 {
		t.Fatalf("display aspect ratio: input=%f output=%f (SAR=%s)", inputDAR, outputDAR, video.SampleAspectRatio)
	}
}

func TestTailWriterKeepsOnlyTail(t *testing.T) {
	w := &tailWriter{limit: 5}
	_, _ = w.Write([]byte("1234"))
	_, _ = w.Write([]byte("5678"))
	if got := w.String(); got != "45678" {
		t.Errorf("tail = %q", got)
	}
}

func TestLiveTranscoderStopsOnCancellation(t *testing.T) {
	for _, target := range []string{"request", "server"} {
		t.Run(target, func(t *testing.T) {
			requestCtx, cancelRequest := context.WithCancel(context.Background())
			defer cancelRequest()
			serverCtx, cancelServer := context.WithCancel(context.Background())
			defer cancelServer()
			transcoder := helperTranscoder(serverCtx.Done())

			started, err := transcoder.Start(requestCtx, domain.LiveTranscodeRequest{
				Path: "movie.mkv", StartupDeadline: time.Now().Add(10 * time.Second),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = started.Stream.Close() }()
			if target == "request" {
				cancelRequest()
			} else {
				cancelServer()
			}

			done := make(chan error, 1)
			go func() { done <- started.Wait() }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancel後なのにprocessが成功終了した")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancel後もprocessが終了しない")
			}
		})
	}
}

func TestLiveTranscoderBoundsProbeByStartupDeadline(t *testing.T) {
	transcoder := NewLiveTranscoder(nil)
	transcoder.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestTranscodeHelperProcess", "--", "probe-hang")
		cmd.Env = append(os.Environ(), "VV_TRANSCODE_HELPER=1")
		return cmd
	}

	started := time.Now()
	_, err := transcoder.Start(context.Background(), domain.LiveTranscodeRequest{
		Path: "movie.mkv", StartupDeadline: time.Now().Add(30 * time.Millisecond),
	})
	if err == nil {
		t.Fatal("停止するprobeが成功した")
	}
	if errors.Is(err, domain.ErrUnprocessableMedia) {
		t.Fatalf("timeoutを動画固有errorとして返しました: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("startup deadlineまで %s かかった", elapsed)
	}
}

func helperTranscoder(serverDone <-chan struct{}) *LiveTranscoder {
	transcoder := NewLiveTranscoder(serverDone)
	commands := 0
	transcoder.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		commands++
		mode := "ffmpeg"
		if commands == 1 {
			mode = "probe"
		}
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestTranscodeHelperProcess", "--", mode)
		cmd.Env = append(os.Environ(), "VV_TRANSCODE_HELPER=1")
		return cmd
	}
	return transcoder
}

func TestTranscodeHelperProcess(t *testing.T) {
	if os.Getenv("VV_TRANSCODE_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	if mode == "probe-hang" {
		for {
			time.Sleep(time.Second)
		}
	}
	if mode == "probe" {
		_, _ = io.WriteString(os.Stdout, `{"streams":[{"index":0,"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p","bits_per_raw_sample":"8","width":640,"height":360,"level":31,"avg_frame_rate":"30/1","r_frame_rate":"30/1"}],"format":{"duration":"10.0"}}`)
		os.Exit(0)
	}
	if mode == "ffmpeg-fail" {
		_, _ = fmt.Fprintln(os.Stderr, "decode failed")
		os.Exit(1)
	}
	if mode == "ffmpeg-silent" {
		for {
			time.Sleep(time.Second)
		}
	}
	if mode != "ffmpeg" {
		_, _ = fmt.Fprintln(os.Stderr, "unknown helper mode")
		os.Exit(2)
	}
	_, _ = io.WriteString(os.Stdout, "fragment")
	for {
		time.Sleep(time.Second)
	}
}

// 同じ ffprobe の出力から、取り込みの解析が保存する値と要求時の解析が作る値が等しく、
// 保存（JSON）を経て読み戻した値も同じ ffmpeg の引数を生む（親 Issue #371 受け入れ条件 10）。
func TestIngestAndRequestProbeShareTranscodeProbe(t *testing.T) {
	outputs := map[string]string{
		"MOV with cover art and rotation": `{"streams":[
			{"index":0,"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}},
			{"index":1,"codec_type":"video","codec_name":"H264","profile":"High","pix_fmt":"YUV420P","bits_per_raw_sample":"8","width":1920,"height":1080,"level":41,"avg_frame_rate":"30000/1001","r_frame_rate":"30000/1001","sample_aspect_ratio":"4:3","side_data_list":[{"side_data_type":"Display Matrix","rotation":-90}]},
			{"index":2,"codec_type":"audio","codec_name":"aac","profile":"LC","sample_rate":"48000","channels":2}
		],"format":{"duration":"12.5","format_name":"MOV,MP4,M4A,3GP,3G2,MJ2"}}`,
		"MKV without audio": `{"streams":[
			{"index":0,"codec_type":"video","codec_name":"hevc","profile":"Main 10","pix_fmt":"yuv420p10le","width":3840,"height":2160,"level":153,"avg_frame_rate":"24/1","r_frame_rate":"48/1","tags":{"rotate":"180"}}
		],"format":{"duration":"60.0","format_name":"matroska,webm"}}`,
	}
	for name, output := range outputs {
		t.Run(name, func(t *testing.T) {
			ingest, err := parseProbeOutput([]byte(output))
			if err != nil {
				t.Fatal(err)
			}
			if ingest.Transcode == nil {
				t.Fatal("取り込みの解析が変換用の情報を持たない")
			}
			request, err := parseTranscodeProbe([]byte(output))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*ingest.Transcode, request) {
				t.Fatalf("ingest = %+v, request = %+v", *ingest.Transcode, request)
			}

			encoded, err := json.Marshal(ingest.Transcode)
			if err != nil {
				t.Fatal(err)
			}
			stamp := domain.FileStamp{SizeBytes: 1024, ModTimeNs: 1_700_000_000_123_456_789}
			stored, ok := domain.TranscodeProbeUsable(&domain.StoredTranscodeProbe{
				Version: domain.TranscodeProbeVersion, Source: stamp, Probe: string(encoded),
			}, stamp)
			if !ok || !reflect.DeepEqual(stored, request) {
				t.Fatalf("stored = %+v (usable=%v), request = %+v", stored, ok, request)
			}
			for _, startMs := range []int64{0, 25000} {
				want := transcodeArgs("movie", startMs, request, false)
				if got := transcodeArgs("movie", startMs, stored, false); !reflect.DeepEqual(got, want) {
					t.Fatalf("args differ at %d: stored=%v request=%v", startMs, got, want)
				}
			}
		})
	}
}

// 使える映像 stream（非添付で寸法がある）が無い動画では、取り込みの解析は成功しても
// 変換用の情報を持たない。
func TestParseProbeOutputOmitsTranscodeWithoutUsableVideo(t *testing.T) {
	outputs := map[string]string{
		"attached picture only": `{"streams":[{"index":0,"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}},{"index":1,"codec_type":"audio","codec_name":"aac"}],"format":{"duration":"12.5"}}`,
		"no dimensions":         `{"streams":[{"index":0,"codec_type":"video","codec_name":"h264"}],"format":{"duration":"12.5"}}`,
		"audio only":            `{"streams":[{"index":0,"codec_type":"audio","codec_name":"mp3"}],"format":{"duration":"12.5"}}`,
	}
	for name, output := range outputs {
		t.Run(name, func(t *testing.T) {
			got, err := parseProbeOutput([]byte(output))
			if err != nil {
				t.Fatal(err)
			}
			if got.Transcode != nil {
				t.Fatalf("Transcode = %+v, want nil", got.Transcode)
			}
			if _, err := parseTranscodeProbe([]byte(output)); err == nil {
				t.Fatal("要求時の解析が成功した")
			}
		})
	}
}

// Probe は ffprobe の直前に取ったファイルの大きさと更新時刻を Source に載せる。
func TestProbeRecordsSourceStamp(t *testing.T) {
	if _, err := exec.LookPath(transcodeCommand); err != nil {
		t.Skip("ffmpegがありません")
	}
	if _, err := exec.LookPath(probeCommand); err != nil {
		t.Skip("ffprobeがありません")
	}
	input := filepath.Join(t.TempDir(), "clip.mp4")
	generate := exec.Command(transcodeCommand,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=160x90:rate=15:duration=1",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", input,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("fixture生成: %v: %s", err, output)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Probe(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != domain.FileStampOf(info) || got.Source.ModTimeNs == 0 {
		t.Fatalf("Source = %+v, want %+v", got.Source, domain.FileStampOf(info))
	}
	if got.Transcode == nil || got.Transcode.Video.Width != 160 || got.Transcode.Audio != nil {
		t.Fatalf("Transcode = %+v", got.Transcode)
	}
}

// scriptedTranscoder は起動したコマンドを数える。ffprobe は probeMode、FFmpeg は
// ffmpegModes を順に使い（尽きたら最後のもの）、helper process で代わりに動かす。
type scriptedTranscoder struct {
	*LiveTranscoder
	mu          sync.Mutex
	commands    []string
	probeMode   string
	ffmpegModes []string
}

func newScriptedTranscoder(ffmpegModes ...string) *scriptedTranscoder {
	scripted := &scriptedTranscoder{LiveTranscoder: NewLiveTranscoder(nil), probeMode: "probe", ffmpegModes: ffmpegModes}
	scripted.commandContext = func(ctx context.Context, name string, _ ...string) *exec.Cmd {
		scripted.mu.Lock()
		mode := scripted.probeMode
		if name != probeCommand {
			index := min(len(scripted.ffmpegCommands()), len(scripted.ffmpegModes)-1)
			mode = scripted.ffmpegModes[index]
		}
		scripted.commands = append(scripted.commands, name)
		scripted.mu.Unlock()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestTranscodeHelperProcess", "--", mode)
		cmd.Env = append(os.Environ(), "VV_TRANSCODE_HELPER=1")
		return cmd
	}
	return scripted
}

func (s *scriptedTranscoder) ffmpegCommands() []string {
	var ffmpeg []string
	for _, name := range s.commands {
		if name != probeCommand {
			ffmpeg = append(ffmpeg, name)
		}
	}
	return ffmpeg
}

func (s *scriptedTranscoder) started() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.commands)
}

// sourceFile は開いたファイルの代わりに実在するファイルを作り、その印を返す。
func sourceFile(t *testing.T) (string, domain.FileStamp) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, domain.FileStampOf(info)
}

// helperProbe は helper process の "probe" が出す ffprobe の出力を解釈した値である。
func helperProbe() domain.TranscodeProbe {
	return domain.TranscodeProbe{Video: domain.TranscodeVideo{
		CodecName: "h264", Profile: "High", PixelFormat: "yuv420p", BitsPerRawSample: 8,
		Width: 640, Height: 360, Level: 31, FPS: 30, RealFPS: 30, SampleAspectNum: 1, SampleAspectDen: 1,
	}}
}

func finishStarted(t *testing.T, started domain.LiveTranscode) []byte {
	t.Helper()
	first, err := readInitialBytes(started.Stream)
	if err != nil {
		t.Fatalf("初期データ: %v", err)
	}
	started.Stop()
	_ = started.Stream.Close()
	_ = started.Wait()
	return first
}

// 保存済みの解析情報があれば ffprobe を起動しない。先頭からでも途中からでも同じ
// （親 Issue #371 受け入れ条件 7）。
func TestLiveTranscoderSkipsProbeWithStoredProbe(t *testing.T) {
	path, stamp := sourceFile(t)
	stored := helperProbe()
	for _, startMs := range []int64{0, 4000} {
		scripted := newScriptedTranscoder("ffmpeg")
		started, err := scripted.Start(context.Background(), domain.LiveTranscodeRequest{
			Path: path, Source: stamp, Probe: &stored, StartMs: startMs, StartupDeadline: time.Now().Add(5 * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := string(finishStarted(t, started)); got != "fragment" {
			t.Errorf("startMs=%d: 初期データ = %q", startMs, got)
		}
		if started.Probed != nil {
			t.Errorf("startMs=%d: 解析していないのに Probed がある", startMs)
		}
		if got := scripted.started(); !slices.Equal(got, []string{transcodeCommand}) {
			t.Errorf("startMs=%d: 起動したコマンド = %v", startMs, got)
		}
	}
}

// 解析情報が無ければ ffprobe を 1 回だけ実行し、その結果を返す（受け入れ条件 8）。
func TestLiveTranscoderProbesWithoutStoredProbe(t *testing.T) {
	path, stamp := sourceFile(t)
	scripted := newScriptedTranscoder("ffmpeg")
	started, err := scripted.Start(context.Background(), domain.LiveTranscodeRequest{
		Path: path, Source: stamp, StartupDeadline: time.Now().Add(5 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	finishStarted(t, started)
	if started.Probed == nil || !reflect.DeepEqual(*started.Probed, helperProbe()) {
		t.Errorf("Probed = %+v", started.Probed)
	}
	if got := scripted.started(); !slices.Equal(got, []string{probeCommand, transcodeCommand}) {
		t.Errorf("起動したコマンド = %v", got)
	}
}

// 解析のあとにファイルが開いたときと違っていたら、変換はするが結果を保存用に返さない。
func TestLiveTranscoderDoesNotReturnProbeOfChangedFile(t *testing.T) {
	path, stamp := sourceFile(t)
	stamp.ModTimeNs++
	scripted := newScriptedTranscoder("ffmpeg")
	started, err := scripted.Start(context.Background(), domain.LiveTranscodeRequest{
		Path: path, Source: stamp, StartupDeadline: time.Now().Add(5 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	finishStarted(t, started)
	if started.Probed != nil {
		t.Errorf("変わったファイルの解析結果を返した: %+v", started.Probed)
	}
}

// 保存値で始めた FFmpeg がデータを出さずに終わったら、その場で解析して 1 回だけ
// やり直す（Structural Decision 3）。
func TestLiveTranscoderRetriesWithFreshProbeWhenStoredProbeFails(t *testing.T) {
	path, stamp := sourceFile(t)
	stored := helperProbe()
	stored.Video.Index = 7
	scripted := newScriptedTranscoder("ffmpeg-fail", "ffmpeg")
	started, err := scripted.Start(context.Background(), domain.LiveTranscodeRequest{
		Path: path, Source: stamp, Probe: &stored, StartupDeadline: time.Now().Add(5 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	finishStarted(t, started)
	if started.Probed == nil || !reflect.DeepEqual(*started.Probed, helperProbe()) {
		t.Errorf("Probed = %+v", started.Probed)
	}
	want := []string{transcodeCommand, probeCommand, transcodeCommand}
	if got := scripted.started(); !slices.Equal(got, want) {
		t.Errorf("起動したコマンド = %v, want %v", got, want)
	}
}

// やり直しは 1 回だけで、その場の解析で始めた FFmpeg の失敗は切り替えない。
func TestLiveTranscoderRetriesOnlyOnce(t *testing.T) {
	path, stamp := sourceFile(t)
	for _, tc := range []struct {
		name   string
		stored bool
		want   []string
	}{
		{"stored", true, []string{transcodeCommand, probeCommand, transcodeCommand}},
		{"fresh", false, []string{probeCommand, transcodeCommand}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := domain.LiveTranscodeRequest{Path: path, Source: stamp, StartupDeadline: time.Now().Add(5 * time.Second)}
			if tc.stored {
				stored := helperProbe()
				request.Probe = &stored
			}
			scripted := newScriptedTranscoder("ffmpeg-fail")
			if _, err := scripted.Start(context.Background(), request); err == nil {
				t.Fatal("失敗し続ける FFmpeg で成功した")
			}
			if got := scripted.started(); !slices.Equal(got, tc.want) {
				t.Errorf("起動したコマンド = %v, want %v", got, tc.want)
			}
		})
	}
}

// 最初のデータを待つ期限切れはやり直さずに失敗にする。期限は切り替え全体で 1 つである。
func TestLiveTranscoderDoesNotRetryAfterStartupTimeout(t *testing.T) {
	path, stamp := sourceFile(t)
	stored := helperProbe()
	scripted := newScriptedTranscoder("ffmpeg-silent")
	began := time.Now()
	_, err := scripted.Start(context.Background(), domain.LiveTranscodeRequest{
		Path: path, Source: stamp, Probe: &stored, StartupDeadline: time.Now().Add(200 * time.Millisecond),
	})
	if err == nil {
		t.Fatal("データを出さない FFmpeg で成功した")
	}
	if elapsed := time.Since(began); elapsed > 2*time.Second {
		t.Errorf("期限切れまで %s かかった", elapsed)
	}
	if got := scripted.started(); !slices.Equal(got, []string{transcodeCommand}) {
		t.Errorf("起動したコマンド = %v", got)
	}
}

// 要求の取り消しで打ち切られた ffprobe は誤りで終わり、結果を返さない。
func TestLiveTranscoderReturnsNoProbeWhenProbeIsCanceled(t *testing.T) {
	path, stamp := sourceFile(t)
	scripted := newScriptedTranscoder("ffmpeg")
	scripted.probeMode = "probe-hang"
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	started, err := scripted.Start(ctx, domain.LiveTranscodeRequest{
		Path: path, Source: stamp, StartupDeadline: time.Now().Add(5 * time.Second),
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if started.Probed != nil {
		t.Errorf("打ち切られた解析の結果を返した: %+v", started.Probed)
	}
	if got := scripted.started(); !slices.Equal(got, []string{probeCommand}) {
		t.Errorf("起動したコマンド = %v", got)
	}
}
