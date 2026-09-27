package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	seekThumbnailCommand = "ffmpeg"
	seekThumbnailTimeout = 30 * time.Minute
	seekSpriteScale      = "scale=min(320\\,iw):min(320\\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2"
)

// GenerateSeekSprite は layout の各区間の中ほどからコマを取り、シートを生成する。
// 長尺の HD 入力は独立した入力側シークをまとめて処理し、全編復号を避ける。
// 短尺・低解像度と、シークでコマが不足した入力は従来の全編復号を使う。
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
	// The interval is widened only beyond 50 minutes. Do not add a probe or
	// process-start overhead to the short path.
	if layout.IntervalMs > domain.SeekThumbnailInterval.Milliseconds() {
		probe, err := Probe(ctx, videoPath)
		if err == nil && useSeekSpriteBatches(layout, probe) {
			err = generateSeekSpriteBatches(ctx, videoPath, outputDir, layout, seekSpriteFPS(probe))
			if err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.WarnContext(ctx, "シークサムネイルの区間抽出が不完全なため全編から生成します", "error", err)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	_, err := runSeekFFmpeg(ctx, seekSpriteArgs(videoPath, filepath.Join(outputDir, "%03d.jpg"), layout))
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
		"fps=1000/" + strconv.FormatInt(layout.IntervalMs, 10) + ":eof_action=pass",
		"tpad=stop_mode=clone:stop=-1",
		"trim=end_frame=" + strconv.Itoa(layout.FrameCount),
		seekSpriteScale,
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
