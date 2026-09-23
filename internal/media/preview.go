package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

const (
	previewSegmentCount = 12
	previewSegmentSec   = 0.75
	previewShortSec     = 9.0
	previewTimeout      = 30 * time.Minute
	previewTempGrace    = time.Minute
)

type previewManifest struct {
	Version int    `json:"version"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

// ErrPreviewStale means the source identity changed while ffmpeg was running.
// The caller may safely retry the job; no generated asset was published.
var ErrPreviewStale = errors.New("preview source identity changed")

// PreviewSegments returns the deterministic [start,end) windows used for a
// preview. Short videos are converted once so their timing is preserved.
func PreviewSegments(durationMs int64) [][2]float64 {
	duration := float64(durationMs) / 1000
	if duration <= previewShortSec {
		return [][2]float64{{0, max(0, duration)}}
	}
	segments := make([][2]float64, previewSegmentCount)
	for i := range segments {
		start := (duration - previewSegmentSec) * float64(i) / float64(previewSegmentCount-1)
		segments[i] = [2]float64{start, start + previewSegmentSec}
	}
	return segments
}

// GeneratePreview atomically publishes a content-keyed MP4 and its integrity
// manifest. Files in the temporary sibling are invisible to readers.
func GeneratePreview(
	ctx context.Context,
	videoPath, thumbnailsDir, contentKey string,
	durationMs int64,
	validate func(context.Context) (bool, error),
) error {
	target := PreviewPath(thumbnailsDir, contentKey)
	manifest := PreviewManifestPath(thumbnailsDir, contentKey)
	if err := os.MkdirAll(filepath.Dir(target), thumbnailDirPerm); err != nil {
		return fmt.Errorf("プレビューの置き場所を作れません: %w", err)
	}
	// Publishing two files cannot be atomic as a pair, so all processes that
	// share this preview root use one bounded, persistent publication lock.
	assetLock := flock.New(filepath.Join(thumbnailsDir, "preview", ".publish.lock"))
	locked, err := assetLock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("プレビューの生成ロックを取得できません: %w", err)
	}
	if !locked {
		return errors.New("プレビューの生成ロックを取得できません")
	}
	defer func() { _ = assetLock.Unlock() }()
	if _, err := VerifyPreview(target, manifest); err == nil {
		return nil
	}
	_ = os.Remove(target)
	_ = os.Remove(manifest)
	temporary, err := os.MkdirTemp(filepath.Dir(target), ".preview-*")
	if err != nil {
		return fmt.Errorf("プレビューの一時領域を作れません: %w", err)
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	tmpVideo := filepath.Join(temporary, "preview.mp4")
	processCtx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	if combined, runErr := exec.CommandContext(processCtx, "ffmpeg", previewArgs(videoPath, tmpVideo, durationMs)...).CombinedOutput(); runErr != nil {
		if processCtx.Err() != nil {
			return fmt.Errorf("プレビュー生成を中断しました: %w", processCtx.Err())
		}
		return fmt.Errorf("ffmpeg がプレビュー生成に失敗しました: %w: %s", runErr, firstLine(combined))
	}
	info, err := os.Stat(tmpVideo)
	if err != nil {
		return fmt.Errorf("プレビューを確認できません: %w", err)
	}
	if info.Size() == 0 {
		return errors.New("プレビューが生成されませんでした: ファイルが空です")
	}
	digest, err := fileSHA256(tmpVideo)
	if err != nil {
		return err
	}
	data, err := json.Marshal(previewManifest{Version: 1, Size: info.Size(), SHA256: digest})
	if err != nil {
		return err
	}
	tmpManifest := filepath.Join(temporary, "preview.mp4.sha256")
	if err := os.WriteFile(tmpManifest, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if validate != nil {
		current, err := validate(ctx)
		if err != nil {
			return fmt.Errorf("プレビューの公開条件を確認できません: %w", err)
		}
		if !current {
			return ErrPreviewStale
		}
	}
	if err := os.Rename(tmpVideo, target); err != nil {
		if _, verifyErr := VerifyPreview(target, manifest); verifyErr != nil {
			return fmt.Errorf("プレビューを確定できません: %w", err)
		}
		return nil
	}
	if err := os.Rename(tmpManifest, manifest); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("プレビューのmanifestを確定できません: %w", err)
	}
	return nil
}

func PreviewPath(thumbnailsDir, contentKey string) string {
	safe := thumbnailFileName(contentKey)
	prefix := safe
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return filepath.Join(thumbnailsDir, "preview", prefix, safe+".mp4")
}

func PreviewManifestPath(thumbnailsDir, contentKey string) string {
	return PreviewPath(thumbnailsDir, contentKey) + ".sha256"
}

// RemovePreview removes both parts of one content-keyed preview asset.
func RemovePreview(thumbnailsDir, contentKey string) error {
	var errs []error
	for _, path := range []string{
		PreviewPath(thumbnailsDir, contentKey),
		PreviewManifestPath(thumbnailsDir, contentKey),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// VerifyPreview checks both the manifest and the complete MP4 payload.
func VerifyPreview(path, manifestPath string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return 0, errors.New("preview is not a regular file")
	}
	var expected previewManifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return 0, err
	}
	if err := json.Unmarshal(data, &expected); err != nil {
		return 0, err
	}
	if expected.Version != 1 || expected.Size != info.Size() || len(expected.SHA256) != sha256.Size*2 {
		return 0, errors.New("preview manifest does not match size")
	}
	actual, err := fileSHA256(path)
	if err != nil {
		return 0, err
	}
	if !strings.EqualFold(expected.SHA256, actual) {
		return 0, errors.New("preview manifest does not match digest")
	}
	return info.Size(), nil
}

func RemoveOrphanPreviews(thumbnailsDir string, contentKeys map[string]struct{}) (int, error) {
	root := filepath.Join(thumbnailsDir, "preview")
	prefixes, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	keep := map[string]struct{}{}
	for key := range contentKeys {
		keep[thumbnailFileName(key)+".mp4"] = struct{}{}
	}
	removed := 0
	for _, prefix := range prefixes {
		if !prefix.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, prefix.Name()))
		if err != nil {
			return removed, err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".preview-") {
				continue
			}
			name := entry.Name()
			base := strings.TrimSuffix(name, ".sha256")
			if !strings.HasSuffix(base, ".mp4") {
				continue
			}
			if _, ok := keep[base]; ok {
				continue
			}
			if err := os.Remove(filepath.Join(root, prefix.Name(), name)); err != nil && !os.IsNotExist(err) {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

// RemoveAbandonedPreviewTemps removes temporary generation directories older
// than the supplied cutoff. Callers use the current time before workers start,
// and a timeout-adjusted cutoff while workers may be active.
func RemoveAbandonedPreviewTemps(thumbnailsDir string, cutoff time.Time) (int, error) {
	root := filepath.Join(thumbnailsDir, "preview")
	prefixes, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, prefix := range prefixes {
		if !prefix.IsDir() {
			continue
		}
		dir := filepath.Join(root, prefix.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			return removed, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".preview-") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return removed, err
			}
			if !info.ModTime().Before(cutoff) {
				continue
			}
			if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

func PreviewTempCutoff(now time.Time) time.Time {
	return now.Add(-previewTimeout - previewTempGrace)
}

func previewArgs(videoPath, output string, durationMs int64) []string {
	args := []string{"-nostdin", "-v", "error", "-i", videoPath}
	segments := PreviewSegments(durationMs)
	if len(segments) == 1 {
		args = append(args, "-map", "0:V:0?", "-vf", "scale='trunc(min(640,iw)/2)*2':-2", "-an")
	} else {
		filters := make([]string, 0, len(segments)+1)
		labels := make([]string, 0, len(segments))
		for i, segment := range segments {
			label := fmt.Sprintf("v%d", i)
			filters = append(filters, fmt.Sprintf("[0:V:0]trim=start=%s:end=%s,setpts=PTS-STARTPTS,scale='trunc(min(640,iw)/2)*2':-2,format=yuv420p[%s]", previewFormatSeconds(segment[0]), previewFormatSeconds(segment[1]), label))
			labels = append(labels, "["+label+"]")
		}
		filters = append(filters, strings.Join(labels, "")+fmt.Sprintf("concat=n=%d:v=1:a=0[v]", len(labels)))
		args = append(args, "-filter_complex", strings.Join(filters, ";"), "-map", "[v]")
	}
	return append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-an", "-y", output)
}

func previewFormatSeconds(value float64) string { return strconv.FormatFloat(value, 'f', 3, 64) }

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
