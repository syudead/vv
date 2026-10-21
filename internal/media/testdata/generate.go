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

// scene draws a 64x36 pattern that changes every 20 seconds and drifts at speed radians per second, so a shifted
// start no longer lines up frame for frame.
func scene(variant int, seconds string, speed float64) string {
	terms := []string{"128"}
	for i, wave := range []struct{ amplitude, fx, fy string }{
		{"34", "0.5", "0.5"}, {"26", "1", "0.5"}, {"22", "0.5", "1"}, {"18", "1.5", "1"},
		{"14", "1", "2"}, {"12", "2", "1.5"}, {"10", "2.5", "2"}, {"8", "3", "3"},
	} {
		sign := fmt.Sprintf("(2*mod(floor(T/20)*%d+%d*%d,2)-1)", i+1, variant, i+2)
		terms = append(terms, fmt.Sprintf("%s*sin(2*PI*(X/W*%s+%s*Y/H*%s)+floor(T/20)*%d+%d*%d+T*%g)",
			wave.amplitude, wave.fx, sign, wave.fy, i*3+1, variant, i*5+2, speed))
	}
	return "color=c=gray:s=64x36:r=4:d=" + seconds + ",format=yuv420p,geq=lum='" + strings.Join(terms, "+") + "':cb=128:cr=128"
}

func fingerprints() {
	// Versions an owner would bundle: a re-encode with black bands, a watermark, and a start one second later.
	for _, c := range []struct {
		name, transform string
		variant, crf    int
	}{
		{"base", "scale=320:180", 0, 20},
		{"letterbox", "scale=240:136,pad=240:180:0:22", 0, 32},
		{"watermark", "scale=320:180,drawbox=x=200:y=10:w=110:h=40:color=white@0.6:t=fill", 0, 20},
		{"shifted", "trim=start=1,setpts=PTS-STARTPTS,scale=320:180", 0, 30},
		{"other", "scale=320:180", 1, 20},
	} {
		fmt.Println("fingerprints/" + c.name + ".mp4")
		ffmpeg(filepath.Join(root, "fingerprints", c.name+".mp4"), "-f", "lavfi", "-i", scene(c.variant, "61", 0.8)+","+c.transform,
			"-c:v", "libx264", "-preset", "veryfast", "-crf", strconv.Itoa(c.crf), "-pix_fmt", "yuv420p")
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
	fingerprints()
	videos(tmp)
	checksums()
}
