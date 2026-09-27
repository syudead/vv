package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

const seekSpriteBatchSize = 10

// Sparse seeking pays off when long HD inputs would otherwise decode thousands
// of frames between samples. Keep the existing path for short/low-resolution
// inputs, where opening each seek input can cost more than decoding once.
func useSeekSpriteBatches(layout domain.SeekSpriteLayout, probe domain.Probe) bool {
	return layout.IntervalMs > domain.SeekThumbnailInterval.Milliseconds() &&
		int64(probe.Width)*int64(probe.Height) >= 1280*720 &&
		seekSpriteFPS(probe) >= 10
}

// generateSeekSpriteBatches keeps a separate decoder for every seek. Reusing one
// decoder through the concat demuxer loses frames for some open-GOP HEVC inputs.
// Each batch is lossless full-range YUV; only the final sheets are JPEG encoded.
func generateSeekSpriteBatches(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout, fps float64) error {
	temporary, err := os.MkdirTemp(outputDir, ".seek-batch-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	framesPath := filepath.Join(temporary, "frames.y4m")
	frames, err := os.Create(framesPath)
	if err != nil {
		return err
	}
	// Also close on early return, before removing the directory on Windows.
	defer func() { _ = frames.Close() }()
	var header string
	for first := 0; first < layout.FrameCount; first += seekSpriteBatchSize {
		count := min(seekSpriteBatchSize, layout.FrameCount-first)
		data, runErr := runSeekFFmpeg(ctx, seekSpriteBatchArgs(videoPath, layout, fps, first, count))
		if runErr != nil {
			return runErr
		}
		if header, err = appendSeekFrames(frames, data, header, count); err != nil {
			return fmt.Errorf("シークサムネイルのコマ %d: %w", first, err)
		}
	}
	if err := frames.Close(); err != nil {
		return err
	}
	_, err = runSeekFFmpeg(ctx, []string{
		"-nostdin", "-v", "error", "-i", framesPath,
		"-vf", fmt.Sprintf("tile=%dx%d,format=yuvj420p", layout.Columns, layout.Rows),
		"-fps_mode", "passthrough", "-q:v", "4", "-start_number", "0",
		"-y", filepath.Join(outputDir, "%03d.jpg"),
	})
	return err
}

func seekSpriteBatchArgs(videoPath string, layout domain.SeekSpriteLayout, fps float64, first, count int) []string {
	args := []string{"-nostdin", "-v", "error", "-filter_complex_threads", "1"}
	filters := make([]string, 0, count+1)
	labels := make([]string, 0, count)
	interval := float64(layout.IntervalMs) / 1000
	for i := range count {
		bucket := float64(first + i)
		// fps in the sequential path picks the frame just before the bucket's
		// midpoint. Accurate input seeking rounds forward to that same frame
		// for CFR. VFR still selects within the bucket: trim excludes a frame
		// beyond its end, and an empty bucket makes the whole batch fail.
		target := max(0, (bucket+0.5)*interval-1/fps)
		// HEVC open GOPs can seek to a CRA picture after target and discard
		// its leading pictures. Decode a short preroll before selecting a cell.
		start := max(0, target-1)
		window := (bucket+1)*interval - start
		args = append(args,
			"-threads", "2", "-ss", strconv.FormatFloat(start, 'f', 9, 64),
			"-t", strconv.FormatFloat(window, 'f', 9, 64), "-i", videoPath)
		label := fmt.Sprintf("v%d", i)
		filters = append(filters, fmt.Sprintf(
			"[%d:V:0]trim=start=%s:end=%s,trim=end_frame=1,setpts=PTS-STARTPTS,%s,format=yuvj420p[%s]",
			i, strconv.FormatFloat(target-start, 'f', 9, 64), strconv.FormatFloat(window, 'f', 9, 64), seekSpriteScale, label))
		labels = append(labels, "["+label+"]")
	}
	filters = append(filters, strings.Join(labels, "")+
		fmt.Sprintf("concat=n=%d:v=1:a=0,setpts=N/TB[v]", count))
	return append(args,
		"-filter_complex", strings.Join(filters, ";"), "-map", "[v]",
		"-fps_mode", "passthrough",
		"-strict", "-1", "-pix_fmt", "yuvj420p", "-f", "yuv4mpegpipe", "pipe:1")
}

// appendSeekFrames checks every raw frame before appending it. Checking only
// the number of JPEG sheets misses empty inputs: concat skips them and shifts
// all later cells. Y4M carries the actual scaled dimensions and sample aspect
// ratio, so they need not be guessed from the original (possibly rotated) video.
func appendSeekFrames(dst io.Writer, data []byte, previous string, count int) (string, error) {
	header, payload, ok := bytes.Cut(data, []byte("\n"))
	if !ok {
		return "", errors.New("YUVヘッダがありません")
	}
	line := string(header)
	size, err := seekFrameSize(line)
	if err != nil {
		return "", err
	}
	if previous != "" && previous != line {
		return "", errors.New("コマの寸法または画素形式が変わりました")
	}
	remaining := payload
	for range count {
		marker, tail, found := bytes.Cut(remaining, []byte("\n"))
		if !found || (string(marker) != "FRAME" && !bytes.HasPrefix(marker, []byte("FRAME "))) || len(tail) < size {
			return "", errors.New("抽出したコマが不足または不完全です")
		}
		remaining = tail[size:]
	}
	if len(remaining) != 0 {
		return "", errors.New("抽出したコマが予定より多すぎます")
	}
	if previous == "" {
		if _, err := dst.Write(append(header, '\n')); err != nil {
			return "", err
		}
	}
	if _, err := dst.Write(payload); err != nil {
		return "", err
	}
	return line, nil
}

func seekFrameSize(header string) (int, error) {
	fields := strings.Fields(header)
	if len(fields) == 0 || fields[0] != "YUV4MPEG2" {
		return 0, errors.New("YUVヘッダが不正です")
	}
	var width, height int
	var chroma, fullRange bool
	for _, field := range fields[1:] {
		switch {
		case strings.HasPrefix(field, "W"):
			width, _ = strconv.Atoi(field[1:])
		case strings.HasPrefix(field, "H"):
			height, _ = strconv.Atoi(field[1:])
		case field == "C420jpeg":
			chroma = true
		case field == "XCOLORRANGE=FULL":
			fullRange = true
		}
	}
	if width < 2 || width > 320 || height < 2 || height > 320 ||
		width%2 != 0 || height%2 != 0 || !chroma || !fullRange {
		return 0, errors.New("YUVの寸法または画素形式が不正です")
	}
	return width * height * 3 / 2, nil
}

// A broken/unknown rate cannot be used to place the accurate seek.
func seekSpriteFPS(probe domain.Probe) float64 {
	if probe.Transcode == nil {
		return 0
	}
	fps := probe.Transcode.Video.FPS
	if math.IsNaN(fps) || math.IsInf(fps, 0) || fps <= 0 {
		return 0
	}
	return fps
}
