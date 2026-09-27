package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPreviewSegmentsUsesWholeVideo(t *testing.T) {
	short := PreviewSegments(9000)
	if len(short) != 1 || short[0][0] != 0 || short[0][1] != 9 {
		t.Fatalf("short segments = %#v", short)
	}
	long := PreviewSegments(120000)
	if len(long) != 12 || long[0][0] != 0 || long[11][1] != 120 {
		t.Fatalf("long segments = %#v", long)
	}
	for i := 1; i < len(long); i++ {
		if long[i][0] <= long[i-1][0] || long[i][1]-long[i][0] != previewSegmentSec {
			t.Fatalf("segments are not equally spaced: %#v", long)
		}
		wantStart := (120.0 - previewSegmentSec) * float64(i) / float64(previewSegmentCount-1)
		if long[i][0] != wantStart {
			t.Fatalf("segment %d starts at %f, want %f", i, long[i][0], wantStart)
		}
	}
}

func TestPreviewArgsAreSilentBrowserCompatibleAndFastStart(t *testing.T) {
	args := previewArgs("input.mkv", "output.mp4", 120000)
	joined := " "
	for _, arg := range args {
		joined += arg + " "
	}
	for _, want := range []string{"libx264", "yuv420p", "+faststart", "-an", "[0:V:0]", "concat=n=12:v=1:a=0"} {
		if !contains(joined, want) {
			t.Errorf("args do not contain %q: %v", want, args)
		}
	}
}

func TestGeneratePreviewProducesBrowserCompatibleFastStartAsset(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	source := makePreviewSource(t, dir, 10)
	target := filepath.Join(dir, "preview.mp4")
	if err := GeneratePreview(context.Background(), source, target, 10000); err != nil {
		t.Fatal(err)
	}

	type stream struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Pixel     string `json:"pix_fmt"`
		Width     int    `json:"width"`
	}
	var probe struct {
		Streams []stream `json:"streams"`
		Format  struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"stream=codec_type,codec_name,pix_fmt,width:format=duration", "-of", "json", target).Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatal(err)
	}
	if len(probe.Streams) != 1 || probe.Streams[0].CodecType != "video" ||
		probe.Streams[0].CodecName != "h264" || probe.Streams[0].Pixel != "yuv420p" ||
		probe.Streams[0].Width != 640 || probe.Streams[0].Width%2 != 0 {
		t.Fatalf("unexpected streams: %+v", probe.Streams)
	}
	duration, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil || duration < 8.5 || duration > 9.5 {
		t.Fatalf("duration = %q, err = %v", probe.Format.Duration, err)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if moov, mdat := bytes.Index(payload, []byte("moov")), bytes.Index(payload, []byte("mdat")); moov < 0 || mdat < 0 || moov > mdat {
		t.Fatalf("faststart atoms are not ordered: moov=%d mdat=%d", moov, mdat)
	}
}

func TestGeneratePreviewSamplesAcrossWholeTimeline(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	timeline := filepath.Join(dir, "timeline.mp4")
	filter := "nullsrc=size=64x64:rate=4:duration=120,geq=lum='16+floor(N/40)*18':cb=128:cr=128"
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", filter,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", timeline).CombinedOutput(); err != nil {
		t.Fatalf("create timeline source: %v: %s", err, out)
	}
	cover := filepath.Join(dir, "cover.jpg")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=white:size=64x64",
		"-frames:v", "1", "-y", cover).CombinedOutput(); err != nil {
		t.Fatalf("create cover: %v: %s", err, out)
	}
	source := filepath.Join(dir, "timeline-with-cover.mp4")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-i", cover, "-i", timeline,
		"-map", "0:v:0", "-map", "1:v:0", "-c", "copy", "-disposition:v:0", "attached_pic", "-y", source).CombinedOutput(); err != nil {
		t.Fatalf("mux attached cover: %v: %s", err, out)
	}
	disposition, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:1",
		"-show_entries", "stream_disposition=attached_pic", "-of", "default=nw=1:nk=1", source).Output()
	if err != nil || strings.TrimSpace(string(disposition)) != "1" {
		t.Fatalf("fixture does not contain an attached cover: %q, %v", disposition, err)
	}
	target := filepath.Join(dir, "preview.mp4")
	if err := GeneratePreview(context.Background(), source, target, 120000); err != nil {
		t.Fatal(err)
	}
	samples, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-i", target,
		"-vf", "fps=4/3,scale=1:1", "-frames:v", "12", "-f", "rawvideo", "-pix_fmt", "gray", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != previewSegmentCount {
		t.Fatalf("sample bytes = %d, want %d", len(samples), previewSegmentCount)
	}
	unique := map[byte]struct{}{}
	for _, sample := range samples {
		unique[sample] = struct{}{}
	}
	if len(unique) < 10 {
		t.Fatalf("preview did not sample across the source timeline: values=%v", samples)
	}
}

func TestPreviewArgsSeekEachSegmentOnInput(t *testing.T) {
	args := previewArgs("input.mkv", "output.mp4", 120000)
	segments := PreviewSegments(120000)
	var seeks, inputs int
	for i, arg := range args {
		switch arg {
		case "-ss":
			if i+5 >= len(args) || args[i+2] != "-t" || args[i+4] != "-i" || args[i+5] != "input.mkv" {
				t.Fatalf("-ss is not an input-side seek with -t at %d: %v", i, args)
			}
			want := segments[seeks]
			if args[i+1] != previewFormatSeconds(want[0]) || args[i+3] != previewFormatSeconds(want[1]-want[0]) {
				t.Fatalf("segment %d seek = %s/%s, want %v", seeks, args[i+1], args[i+3], want)
			}
			seeks++
		case "-i":
			inputs++
		}
		if strings.Contains(arg, "trim") {
			t.Fatalf("args still use trim: %v", args)
		}
		if arg == "-noaccurate_seek" {
			t.Fatalf("args disable accurate seek: %v", args)
		}
	}
	if seeks != previewSegmentCount || inputs != previewSegmentCount {
		t.Fatalf("seeks = %d, inputs = %d, want %d: %v", seeks, inputs, previewSegmentCount, args)
	}
	filter := args[slicesIndex(args, "-filter_complex")+1]
	for i := range previewSegmentCount {
		if want := "[" + strconv.Itoa(i) + ":V:0]setpts=PTS-STARTPTS,"; !strings.Contains(filter, want) {
			t.Errorf("filter does not read input %d: %s", i, filter)
		}
	}
}

func TestGeneratePreviewSkipsSegmentsPastVideoEnd(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	// 映像 10 秒・音声 12 秒。容器の長さ 12 秒で区間を選ぶと、末尾の 2 区間は映像の後ろになる。
	source := filepath.Join(dir, "longer-container.mp4")
	video := "nullsrc=size=64x64:rate=8:duration=10,geq=lum='16+floor(N/8)*20':cb=128:cr=128"
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", video,
		"-f", "lavfi", "-i", "sine=frequency=440:duration=12",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-y", source).CombinedOutput(); err != nil {
		t.Fatalf("create source: %v: %s", err, out)
	}
	target := filepath.Join(dir, "preview.mp4")
	if err := GeneratePreview(context.Background(), source, target, 12000); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", target).Output()
	if err != nil {
		t.Fatal(err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	// 区間の開始は 0, 1.02, ..., 9.20 秒の 10 本が映像の中にある。
	if err != nil || duration < 7.0 || duration > 8.2 {
		t.Fatalf("duration = %q, err = %v, want about 10 segments", out, err)
	}
	samples, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-i", target,
		"-vf", "fps=4/3,scale=1:1", "-frames:v", "10", "-f", "rawvideo", "-pix_fmt", "gray", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 10 {
		t.Fatalf("sample bytes = %d, want 10", len(samples))
	}
	for i := 1; i < len(samples); i++ {
		if samples[i] < samples[i-1] {
			t.Fatalf("segments are not in timeline order: %v", samples)
		}
	}
	if samples[len(samples)-1] <= samples[0] {
		t.Fatalf("segments do not advance through the video: %v", samples)
	}
}

func TestGeneratePreviewFailsWhenNoSegmentHasFrames(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	// 4fps・15 秒でキーフレームが先頭だけの MPEG-TS。索引が無いのでシークが
	// キーフレームに着かず、どの区間もフレームを出さない。
	source := filepath.Join(dir, "sparse-keyframes.ts")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi",
		"-i", "testsrc2=size=320x240:rate=4:duration=15",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-f", "mpegts", "-y", source).CombinedOutput(); err != nil {
		t.Fatalf("create source: %v: %s", err, out)
	}
	if err := GeneratePreview(context.Background(), source, filepath.Join(dir, "preview.mp4"), 15000); err == nil {
		t.Fatal("generation without any video frame unexpectedly succeeded")
	}
}

func TestGeneratePreviewKeepsRotatedPortrait(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	landscape := filepath.Join(dir, "landscape.mp4")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi",
		"-i", "testsrc2=size=1280x720:rate=8:duration=12",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", landscape).CombinedOutput(); err != nil {
		t.Fatalf("create landscape: %v: %s", err, out)
	}
	source := filepath.Join(dir, "portrait.mp4")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-display_rotation", "90",
		"-i", landscape, "-c", "copy", "-y", source).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot write display rotation: %v: %s", err, out)
	}
	target := filepath.Join(dir, "preview.mp4")
	if err := GeneratePreview(context.Background(), source, target, 12000); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", target).Output()
	if err != nil {
		t.Fatal(err)
	}
	var width, height int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d,%d", &width, &height); err != nil {
		t.Fatalf("parse size %q: %v", out, err)
	}
	if width != 360 || height != 640 {
		t.Fatalf("preview size = %dx%d, want portrait 360x640", width, height)
	}
}

func slicesIndex(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

func TestGeneratePreviewCancellationFails(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	source := makePreviewSource(t, dir, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := GeneratePreview(ctx, source, filepath.Join(dir, "preview.mp4"), 1000); err == nil {
		t.Fatal("cancelled generation unexpectedly succeeded")
	}
}

func requireFFmpeg(t *testing.T) {
	t.Helper()
	for _, command := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is unavailable", command)
		}
	}
}

func makePreviewSource(t *testing.T, dir string, seconds int) string {
	t.Helper()
	path := filepath.Join(dir, "source.mp4")
	filter := "testsrc2=size=642x362:rate=8:duration=" + strconv.Itoa(seconds)
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", filter,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", path).CombinedOutput(); err != nil {
		t.Fatalf("create source: %v: %s", err, out)
	}
	return path
}

func contains(value, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
