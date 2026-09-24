package media

import (
	"bytes"
	"context"
	"encoding/json"
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
