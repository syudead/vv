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
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func compatibleMetadata() transcodeMetadata {
	audio := transcodeStream{Index: 2, CodecType: "audio", CodecName: "aac", Profile: "LC", SampleRate: 48000, Channels: 2}
	return transcodeMetadata{
		FormatName: "matroska,webm",
		Video:      transcodeStream{Index: 1, CodecType: "video", CodecName: "h264", Profile: "High", PixelFormat: "yuv420p", BitsPerRawSample: 8, Width: 1920, Height: 1080, Level: 41, FPS: 30, RealFPS: 30, SampleAspectNum: 1, SampleAspectDen: 1},
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
		mutate func(*transcodeMetadata)
		want   string
	}{
		{"10-bit H264", func(m *transcodeMetadata) { m.Video.PixelFormat = "yuv420p10le" }, "-c:v libx264"},
		{"High 4:4:4 10-bit", func(m *transcodeMetadata) {
			m.Video.Profile = "High 4:4:4 Predictive"
			m.Video.PixelFormat = "yuv444p10le"
			m.Video.BitsPerRawSample = 10
		}, "-c:v libx264"},
		{"96kHz AAC", func(m *transcodeMetadata) { m.Audio.SampleRate = 96000 }, "-c:a aac -profile:a aac_low -ac 2 -b:a 192k -ar 48000"},
		{"unknown video attribute", func(m *transcodeMetadata) { m.Video.BitsPerRawSample = 0 }, "-c:v libx264"},
		{"variable frame rate", func(m *transcodeMetadata) { m.Video.RealFPS = 120 }, "-c:v libx264"},
		{"rotated coded height", func(m *transcodeMetadata) {
			m.Video.Width = 2160
			m.Video.Height = 3840
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

func TestVideoEncodeArgsNormalizesDimensionsAndRate(t *testing.T) {
	tests := []struct {
		name   string
		stream transcodeStream
		want   string
	}{
		{"odd dimensions", transcodeStream{Width: 641, Height: 359, FPS: 30, RealFPS: 30, SampleAspectNum: 1, SampleAspectDen: 1}, "-vf pad=642:360:0:0,setsar=1/1*641/359*360/642:max=1000000"},
		{"8K60", transcodeStream{Width: 7680, Height: 4320, FPS: 60, RealFPS: 60}, "-vf scale=3840:2160,setsar=1/1*7680/4320*2160/3840:max=1000000,fps=30.340"},
		{"rotated 4K", transcodeStream{Width: 3840, Height: 2160, Rotation: 90, FPS: 30, RealFPS: 30}, "-vf scale=1214:2160,setsar=1/1*2160/3840*2160/1214:max=1000000"},
		{"rotated HD", transcodeStream{Width: 1920, Height: 1080, Rotation: 270, FPS: 30, RealFPS: 30}, "-vf setsar=1/1*1080/1920*1920/1080:max=1000000"},
		{"unknown rate", transcodeStream{Width: 1920, Height: 1080}, "-vf fps=30.000"},
		{"VFR peak", transcodeStream{Width: 1920, Height: 1080, FPS: 30, RealFPS: 120}, "-vf fps=30"},
		{"very low VFR", transcodeStream{Width: 1920, Height: 1080, FPS: 0.0005, RealFPS: 1}, "-vf fps=0.0005"},
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

func TestVideoEncodeRotated4KStaysWithinEnvelopeWithFFmpeg(t *testing.T) {
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
	if video.Width > maxVideoWidth || video.Height > maxVideoHeight {
		t.Fatalf("出力寸法 = %dx%d", video.Width, video.Height)
	}
	normalized, err := parseTranscodeProbe(probeOutputJSON)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Video.Rotation != 0 {
		t.Fatalf("出力に回転metadataが残っています: %d", normalized.Video.Rotation)
	}
	numerator, denominator := parseAspectRatio(video.SampleAspectRatio)
	inputDAR := float64(2160) / float64(3840)
	outputDAR := float64(video.Width) * float64(numerator) / (float64(video.Height) * float64(denominator))
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

			stream, wait, _, err := transcoder.Start(requestCtx, "movie.mkv", 0, false, time.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stream.Close() }()
			if target == "request" {
				cancelRequest()
			} else {
				cancelServer()
			}

			done := make(chan error, 1)
			go func() { done <- wait() }()
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
	_, _, _, err := transcoder.Start(
		context.Background(), "movie.mkv", 0, false, time.Now().Add(30*time.Millisecond),
	)
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
	if mode != "ffmpeg" {
		_, _ = fmt.Fprintln(os.Stderr, "unknown helper mode")
		os.Exit(2)
	}
	_, _ = io.WriteString(os.Stdout, "fragment")
	for {
		time.Sleep(time.Second)
	}
}
