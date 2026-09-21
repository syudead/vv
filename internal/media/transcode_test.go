package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func compatibleMetadata() transcodeMetadata {
	audio := transcodeStream{Index: 2, CodecType: "audio", CodecName: "aac", Profile: "LC", SampleRate: 48000, Channels: 2}
	return transcodeMetadata{
		Video: transcodeStream{Index: 1, CodecType: "video", CodecName: "h264", Profile: "High", PixelFormat: "yuv420p", BitsPerRawSample: 8, Width: 1920, Height: 1080, Level: 41, FPS: 30, RealFPS: 30},
		Audio: &audio,
	}
}

func TestParseTranscodeProbeSkipsAttachedPicture(t *testing.T) {
	const output = `{"streams":[
		{"index":0,"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}},
		{"index":1,"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p","bits_per_raw_sample":"8","width":1920,"height":1080,"level":41,"avg_frame_rate":"30000/1001","r_frame_rate":"30000/1001"},
		{"index":2,"codec_type":"audio","codec_name":"aac","profile":"LC","sample_rate":"48000","channels":2}
	],"format":{"duration":"12.5"}}`
	got, err := parseTranscodeProbe([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	if got.Video.Index != 1 || got.Audio == nil || got.Audio.Index != 2 {
		t.Fatalf("stream selection = %+v", got)
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
	args := strings.Join(transcodeArgs("movie.mp4", 25000, compatibleMetadata(), false), " ")
	for _, want := range []string{"-ss 25.000 -i movie.mp4", "-c:v libx264", "-c:a aac"} {
		if !strings.Contains(args, want) {
			t.Errorf("argsに %q がない: %s", want, args)
		}
	}
}

func TestVideoEncodeArgsNormalizesDimensionsAndRate(t *testing.T) {
	tests := []struct {
		name   string
		stream transcodeStream
		want   string
	}{
		{"odd dimensions", transcodeStream{Width: 641, Height: 359, FPS: 30, RealFPS: 30}, "-vf pad=642:360:0:0"},
		{"8K60", transcodeStream{Width: 7680, Height: 4320, FPS: 60, RealFPS: 60}, "-vf scale=3840:2160,fps=30.340"},
		{"unknown rate", transcodeStream{Width: 1920, Height: 1080}, "-vf fps=30.000"},
		{"VFR peak", transcodeStream{Width: 1920, Height: 1080, FPS: 30, RealFPS: 120}, "-vf fps=30.000"},
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

			stream, wait, _, err := transcoder.Start(requestCtx, "movie.mkv", 0, false)
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
