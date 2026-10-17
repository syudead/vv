//go:build ignore

// Run from the repository root: go run ./internal/media/testdata/generate.go
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const root = "internal/media/testdata"

func ffmpeg(output string, args ...string) {
	args = append([]string{"-nostdin", "-v", "error", "-filter_threads", "1"}, args...)
	args = append(args, "-threads", "1", "-y", output)
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(fmt.Errorf("generate %s: %w", output, err))
	}
}

func gray(size, seconds string) string {
	return "color=c=black:s=" + size + ":r=10:d=" + seconds + ",format=yuv420p,geq=lum='16+7*T':cb=128:cr=128"
}

func scene(variant int, seconds string) string {
	terms := []string{"128"}
	for i, wave := range []struct{ amplitude, fx, fy string }{
		{"34", "0.5", "0.5"}, {"26", "1", "0.5"}, {"22", "0.5", "1"}, {"18", "1.5", "1"},
		{"14", "1", "2"}, {"12", "2", "1.5"}, {"10", "2.5", "2"}, {"8", "3", "3"},
	} {
		sign := fmt.Sprintf("(2*mod(floor(T/20)*%d+%d*%d,2)-1)", i+1, variant, i+2)
		terms = append(terms, fmt.Sprintf("%s*sin(2*PI*(X/W*%s+%s*Y/H*%s)+floor(T/20)*%d+%d*%d+T*0.02)",
			wave.amplitude, wave.fx, sign, wave.fy, i*3+1, variant, i*5+2))
	}
	return "color=c=gray:s=64x36:r=2:d=" + seconds + ",format=yuv420p,geq=lum='" + strings.Join(terms, "+") + "':cb=128:cr=128"
}

func fingerprints(tmp string) {
	// Reference inputs are independent of production sprite arguments. Vary
	// source compression, black bands, frame size and sample intervals.
	for _, c := range []struct {
		name, seconds, transform             string
		variant, ms, crf, gop, size, quality int
	}{
		{"short", "60", "scale=640:360", 0, 60000, 20, 10, 320, 2},
		{"letterbox", "60", "scale=320:180,pad=320:240:0:30", 0, 60000, 32, 15, 320, 2},
		{"other", "60", "scale=640:360", 1, 60000, 20, 10, 320, 2},
		{"long", "410", "scale=640:360", 0, 410000, 20, 10, 320, 2},
		{"long-reencoded", "410.4", "scale=320:180", 0, 410400, 30, 15, 320, 2},
		{"long-other", "410", "scale=640:360", 1, 410000, 20, 10, 320, 2},
		{"old-size", "60", "scale=640:360", 0, 60000, 20, 10, 160, 4},
	} {
		fmt.Println("fingerprints/" + c.name + ".jpg")
		video := filepath.Join(tmp, "scene.mp4")
		ffmpeg(video, "-f", "lavfi", "-i", scene(c.variant, c.seconds)+","+c.transform,
			"-c:v", "libx264", "-preset", "veryfast", "-crf", strconv.Itoa(c.crf), "-g", strconv.Itoa(c.gop),
			"-keyint_min", "1", "-pix_fmt", "yuv420p")
		interval := max(1000, (c.ms+80)/81)
		frames := (c.ms + interval - 1) / interval
		filter := fmt.Sprintf("tpad=stop_mode=clone:stop=-1,fps=1000/%d:round=up:eof_action=pass,trim=end_frame=%d,scale=min(%d\\,iw):min(%d\\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2,tile=9x9,format=yuvj420p", interval, frames, c.size, c.size)
		ffmpeg(filepath.Join(root, "fingerprints", c.name+".jpg"), "-i", video, "-map", "0:V:0", "-vf", filter,
			"-frames:v", "1", "-q:v", strconv.Itoa(c.quality))
	}
}

func videos(tmp string) {
	cases := []struct {
		name string
		args []string
	}{
		{"clock.mp4", []string{"-i", gray("64x64", "30"), "-c:v", "mpeg4", "-q:v", "2"}},
		{"short-video.mp4", []string{"-i", gray("64x64", "23"), "-f", "lavfi", "-i", "sine=frequency=440:duration=30", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac"}},
		{"sparse.mp4", []string{"-i", gray("64x64", "30"), "-vf", "select=eq(n\\,0)+eq(n\\,100)+eq(n\\,200)", "-fps_mode", "vfr", "-c:v", "mpeg4", "-q:v", "2"}},
		{"one-second.mp4", []string{"-i", "color=c=white:s=64x64:r=30:d=1", "-c:v", "mpeg4"}},
		{"portrait.mp4", []string{"-i", "color=c=gray:s=360x640:r=1:d=510", "-c:v", "mpeg4"}},
		{"frame-clock.mp4", []string{"-i", gray("64x48", "30"), "-c:v", "mpeg4", "-q:v", "2", "-g", "50"}},
		{"large.mp4", []string{"-i", "color=c=gray:s=2560x1440:r=1:d=2", "-c:v", "mpeg4", "-q:v", "10"}},
		{"short-clock.mp4", []string{"-i", gray("64x64", "3"), "-c:v", "mpeg4", "-q:v", "2"}},
		{"green.mp4", []string{"-i", "color=c=green:s=64x64:r=30:d=11", "-c:v", "mpeg4"}},
		{"preview.mp4", []string{"-i", "testsrc2=size=642x362:rate=8:duration=10", "-c:v", "libx264", "-pix_fmt", "yuv420p"}},
		{"preview-short.mp4", []string{"-i", "testsrc2=size=642x362:rate=8:duration=1", "-c:v", "libx264", "-pix_fmt", "yuv420p"}},
		{"longer-container.mp4", []string{"-i", "nullsrc=size=64x64:rate=8:duration=10,geq=lum='16+floor(N/8)*20':cb=128:cr=128", "-f", "lavfi", "-i", "sine=frequency=440:duration=12", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac"}},
		{"sparse-keyframes.ts", []string{"-i", "testsrc2=size=320x240:rate=4:duration=15", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-f", "mpegts"}},
	}
	for _, c := range cases {
		fmt.Println("videos/" + c.name)
		ffmpeg(filepath.Join(root, "videos", c.name), append([]string{"-f", "lavfi"}, c.args...)...)
	}
	muxedVideos(tmp)
}

func muxedVideos(tmp string) {
	landscape, cover := filepath.Join(tmp, "landscape.mp4"), filepath.Join(tmp, "cover.jpg")
	ffmpeg(landscape, "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=8:duration=12", "-c:v", "libx264", "-pix_fmt", "yuv420p")
	ffmpeg(filepath.Join(root, "videos", "rotated.mp4"), "-display_rotation", "90", "-i", landscape, "-c", "copy")
	green := filepath.Join(tmp, "green.mp4")
	ffmpeg(green, "-f", "lavfi", "-i", "color=c=green:s=64x64:r=1:d=5", "-c:v", "mpeg4")
	ffmpeg(cover, "-f", "lavfi", "-i", "color=c=red:s=600x600", "-frames:v", "1")
	ffmpeg(filepath.Join(root, "videos", "with-cover.mp4"), "-i", green, "-i", cover,
		"-map", "0:v:0", "-map", "1:v:0", "-c", "copy", "-disposition:v:1", "attached_pic")
	timeline := filepath.Join(tmp, "timeline.mp4")
	ffmpeg(timeline, "-f", "lavfi", "-i", "nullsrc=size=64x64:rate=4:duration=120,geq=lum='16+floor(N/40)*18':cb=128:cr=128", "-c:v", "libx264", "-pix_fmt", "yuv420p")
	ffmpeg(cover, "-f", "lavfi", "-i", "color=white:size=64x64", "-frames:v", "1")
	ffmpeg(filepath.Join(root, "videos", "timeline-with-cover.mp4"), "-i", cover, "-i", timeline,
		"-map", "0:v:0", "-map", "1:v:0", "-c", "copy", "-disposition:v:0", "attached_pic")
}

func checksums() {
	var lines []string
	for _, dir := range []string{"fingerprints", "videos"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				panic(err)
			}
			lines = append(lines, fmt.Sprintf("%x  %s\n", sha256.Sum256(data), filepath.ToSlash(path)))
		}
	}
	if err := os.WriteFile(filepath.Join(root, "SHA256SUMS"), []byte(strings.Join(lines, "")), 0o644); err != nil {
		panic(err)
	}
}

func main() {
	for _, dir := range []string{"fingerprints", "videos"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			panic(err)
		}
	}
	tmp, err := os.MkdirTemp("", "vv-media-fixtures-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	fingerprints(tmp)
	videos(tmp)
	checksums()
}
