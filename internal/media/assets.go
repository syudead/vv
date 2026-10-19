package media

import (
	"context"
	"fmt"
	"os"

	"github.com/syudead/vv/internal/domain"
)

// Assets runs ffprobe/ffmpeg and writes caller-selected paths; artifacts owns layout, publication,
// and deletion, while app uses its own interface.
type Assets struct{}

func NewAssets() *Assets {
	return &Assets{}
}

// CheckSource wraps failures with domain.ProbeErrorFileUnavailable; only probe jobs persist that
// code.
func (a *Assets) CheckSource(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return sourceUnavailable(err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return sourceUnavailable(err)
	}
	if !info.Mode().IsRegular() {
		return sourceUnavailable(fmt.Errorf("not a regular file: %s", path))
	}
	return nil
}

func sourceUnavailable(err error) error {
	return domain.NewProbeFailure(domain.ProbeErrorFileUnavailable, err)
}

func (a *Assets) Probe(ctx context.Context, path string) (domain.Probe, error) {
	return Probe(ctx, path)
}

// Thumbnail reports whether it substituted the first frame.
func (a *Assets) Thumbnail(ctx context.Context, path string, durationMs int64, output string) (bool, error) {
	return Thumbnail(ctx, path, durationMs, output)
}

// ThumbnailAt never substitutes the first frame.
func (a *Assets) ThumbnailAt(ctx context.Context, path string, positionMs int64, output string) error {
	return ThumbnailAt(ctx, path, positionMs, output)
}

func (a *Assets) Poster(ctx context.Context, path string, durationMs int64, output string) error {
	return Poster(ctx, path, durationMs, output)
}

func (a *Assets) PosterAt(ctx context.Context, path string, positionMs int64, output string) error {
	return PosterAt(ctx, path, positionMs, output)
}

// SeekSprite reports whether full decoding was required.
func (a *Assets) SeekSprite(ctx context.Context, path, outputDir string, layout domain.SeekSpriteLayout) (bool, error) {
	return GenerateSeekSprite(ctx, path, outputDir, layout)
}

func (a *Assets) Preview(ctx context.Context, path, output string, durationMs int64) error {
	return GeneratePreview(ctx, path, output, durationMs)
}

// SpriteFingerprint uses completed sprite sheets without invoking ffmpeg.
func (a *Assets) SpriteFingerprint(sprite domain.SeekSprite, sheets [][]byte) (domain.Fingerprint, error) {
	return SpriteFingerprint(sprite, sheets)
}
