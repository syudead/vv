package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	seekThumbnailCommand = "ffmpeg"
	seekThumbnailTimeout = 30 * time.Minute
	seekSpriteFastScale  = "scale=min(160\\,iw):min(160\\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2"
	seekSpriteParallel   = 4
)

// GenerateSeekSprite は各区間の先頭からコマを並列に取り、シートを生成する。
// シークできない入力では全編復号へ戻る。
// 一時データは outputDir 内で片付け、完成物の公開は呼び出し側が受け持つ。
func GenerateSeekSprite(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	processCtx, cancel := context.WithTimeout(ctx, seekThumbnailTimeout)
	defer cancel()
	err := generateSeekSprite(processCtx, videoPath, outputDir, layout)
	if processCtx.Err() != nil {
		return fmt.Errorf("シークサムネイル生成を中断しました: %w", processCtx.Err())
	}
	return err
}

func generateSeekSprite(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	if err := generateSeekSpriteParallel(ctx, videoPath, outputDir, layout); err == nil {
		return nil
	} else if ctx.Err() != nil {
		return ctx.Err()
	} else {
		slog.WarnContext(ctx, "シークサムネイルの区間抽出が不完全なため全編から生成します", "error", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	_, err := runSeekFFmpeg(ctx, seekSpriteArgs(videoPath, filepath.Join(outputDir, "%03d.jpg"), layout))
	return err
}

func generateSeekSpriteParallel(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	temporary, err := os.MkdirTemp(outputDir, ".seek-sprite-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	semaphore := make(chan struct{}, seekSpriteParallel)
	var workers sync.WaitGroup
	var firstErr error
	var recordError sync.Once
frames:
	for frame := range layout.FrameCount {
		select {
		case semaphore <- struct{}{}:
		case <-workCtx.Done():
			break frames
		}
		if workCtx.Err() != nil {
			<-semaphore
			break frames
		}
		workers.Add(1)
		go func(frame int) {
			defer workers.Done()
			defer func() { <-semaphore }()
			at := float64(int64(frame)*layout.IntervalMs) / 1000
			interval := float64(layout.IntervalMs) / 1000
			args := []string{
				"-nostdin", "-v", "error", "-ss", strconv.FormatFloat(at, 'f', 3, 64),
				"-t", strconv.FormatFloat(interval, 'f', 3, 64), "-i", videoPath,
				"-map", "0:V:0?", "-frames:v", "1",
				"-vf", "trim=end=" + strconv.FormatFloat(interval, 'f', 3, 64) + "," + seekSpriteFastScale,
				"-c:v", "bmp", "-f", "rawvideo", "pipe:1",
			}
			data, err := runSeekFFmpeg(workCtx, args)
			if err == nil && (len(data) < 54 || string(data[:2]) != "BM") {
				err = errors.New("シーク位置から画像を抽出できませんでした")
			}
			if err == nil {
				err = os.WriteFile(filepath.Join(temporary, fmt.Sprintf("%03d.bmp", frame)), data, 0600)
			}
			if err != nil {
				recordError.Do(func() { firstErr = fmt.Errorf("コマ %d: %w", frame, err); cancel() })
			}
		}(frame)
	}
	workers.Wait()
	if firstErr != nil {
		return firstErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = runSeekFFmpeg(ctx, []string{
		"-nostdin", "-v", "error", "-framerate", "1", "-i", filepath.Join(temporary, "%03d.bmp"),
		"-vf", fmt.Sprintf("tile=%dx%d,format=yuvj420p", layout.Columns, layout.Rows),
		"-frames:v", "1", "-q:v", "4", "-y", filepath.Join(outputDir, "000.jpg"),
	})
	return err
}

func runSeekFFmpeg(ctx context.Context, args []string) ([]byte, error) {
	data, err := exec.CommandContext(ctx, seekThumbnailCommand, args...).Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("ffmpeg がシークサムネイル生成に失敗しました: %w: %s", err, firstLine(exitErr.Stderr))
		}
		return nil, fmt.Errorf("ffmpeg を実行できません: %w", err)
	}
	return data, nil
}

func seekSpriteArgs(videoPath, outputPattern string, layout domain.SeekSpriteLayout) []string {
	filters := []string{
		"tpad=stop_mode=clone:stop=-1",
		"fps=1000/" + strconv.FormatInt(layout.IntervalMs, 10) + ":round=up:eof_action=pass",
		"trim=end_frame=" + strconv.Itoa(layout.FrameCount),
		seekSpriteFastScale,
		"tile=" + strconv.Itoa(layout.Columns) + "x" + strconv.Itoa(layout.Rows),
		"format=yuvj420p",
	}
	return []string{
		"-nostdin",
		"-v", "error",
		"-i", videoPath,
		"-map", "0:V:0?",
		"-vf", strings.Join(filters, ","),
		"-fps_mode", "passthrough",
		"-q:v", "4",
		"-start_number", "0",
		"-y",
		outputPattern,
	}
}

// GenerateSeekThumbnailSet はシーク用サムネイルの一式を outputDir（既にある
// ディレクトリ）へ書く。scripts/previewbench がシーク用の生成を測る境界で、
// 生成方式を変えるときはこの関数の中身を変える。今は durationMs から
// domain.NewSeekSpriteLayout で配置を決め、GenerateSeekSprite でシートを書く。
func GenerateSeekThumbnailSet(ctx context.Context, videoPath, outputDir string, durationMs int64) error {
	return GenerateSeekSprite(ctx, videoPath, outputDir, domain.NewSeekSpriteLayout(durationMs))
}
