// Package artifacts owns content-addressed artifact paths, atomic publication, validation, reads,
// and cleanup; media generates bytes and app controls lifecycle through their own interfaces. Only
// domain is an internal dependency; see ARCHITECTURE.md.
package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/syudead/vv/internal/domain"
)

// Preserve this layout for compatibility with existing MDM_DATA_DIR artifacts.
// root is the store root, s the sanitized key, and p its first two characters.
//
//	<root>/<p>/<s>.jpg                 Library thumbnail
//	<root>/seek/<p>/<s>/<n>.jpg         Three-digit sprite sheet
//	<root>/seek/<p>/<s>/sprite.json     Layout and completion marker
//	<root>/preview/<p>/<s>.mp4          Hover preview
//	<root>/preview/<p>/<s>.mp4.sha256   Integrity manifest
//	<root>/preview/.publish.lock        Publication lock
//	<root>/.tmp/                        Incomplete output
const (
	seekDirName      = "seek"
	previewDirName   = "preview"
	temporaryDirName = ".tmp"
	publishLockName  = ".publish.lock"
	thumbnailExt     = ".jpg"
	previewExt       = ".mp4"
	manifestExt      = ".sha256"
	// Must match internal/media.GenerateSeekSprite output names.
	sheetNameFormat = "%03d.jpg"
	spriteFileName  = "sprite.json"
)

const spriteVersion = 1

const dirPerm os.FileMode = 0o755

const manifestVersion = 1

const publishLockWait = 100 * time.Millisecond

var errNotConfigured = errors.New("the artifact store is not configured")

// Wrap fs.ErrNotExist so reads treat unsafe keys as missing artifacts.
var errInvalidKey = fmt.Errorf("content key cannot be used in the artifact store: %w", fs.ErrNotExist)

type Store struct {
	root string
}

// New treats an empty root as an absent store: reads miss and publication fails.
func New(root string) *Store {
	return &Store{root: root}
}

// Replace colon and path separators for portable filenames; reject empty or dot-prefixed names to
// prevent traversal and collisions with store-owned directories.
func fileName(contentKey string) (string, bool) {
	name := strings.NewReplacer(":", "_", "/", "_", `\`, "_").Replace(contentKey)
	if name == "" || strings.HasPrefix(name, ".") {
		return "", false
	}
	return name, true
}

func (s *Store) publishTarget(locate func(string) (string, bool), contentKey string) (string, error) {
	if s.root == "" {
		return "", errNotConfigured
	}
	target, ok := locate(contentKey)
	if !ok {
		return "", errInvalidKey
	}
	return target, nil
}

// Shard by two filename characters to avoid tens of thousands of entries in one directory.
func (s *Store) locate(dir, contentKey, ext string) (string, bool) {
	if s.root == "" {
		return "", false
	}
	name, ok := fileName(contentKey)
	if !ok {
		return "", false
	}
	prefix := name
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return filepath.Join(s.root, dir, prefix, name+ext), true
}

func (s *Store) thumbnailPath(contentKey string) (string, bool) {
	return s.locate("", contentKey, thumbnailExt)
}

func (s *Store) seekDir(contentKey string) (string, bool) {
	return s.locate(seekDirName, contentKey, "")
}

func (s *Store) previewPaths(contentKey string) (video, manifest string, ok bool) {
	video, ok = s.locate(previewDirName, contentKey, previewExt)
	return video, video + manifestExt, ok
}

// Stage under the same filesystem for atomic renames; incomplete artifacts remain invisible to
// readers.
func (s *Store) makeTemporaryDir(pattern string) (string, error) {
	if s.root == "" {
		return "", errNotConfigured
	}
	root := filepath.Join(s.root, temporaryDirName)
	if err := os.MkdirAll(root, dirPerm); err != nil {
		return "", fmt.Errorf("cannot create the temporary directory for generation: %w", err)
	}
	dir, err := os.MkdirTemp(root, pattern)
	if err != nil {
		return "", fmt.Errorf("cannot create the temporary directory for generation: %w", err)
	}
	return dir, nil
}

// RemoveTemporary must run before workers start; it clears leftovers from interrupted generation
// without scanning the whole store.
func (s *Store) RemoveTemporary() error {
	if s.root == "" {
		return nil
	}
	if err := os.RemoveAll(filepath.Join(s.root, temporaryDirName)); err != nil {
		return fmt.Errorf("cannot delete the temporary directories for generation: %w", err)
	}
	return nil
}

// PublishThumbnail gives write a temporary path and replaces the target only after validating
// nonempty output.
func (s *Store) PublishThumbnail(contentKey string, write func(output string) error) error {
	target, err := s.publishTarget(s.thumbnailPath, contentKey)
	if err != nil {
		return err
	}
	temporary, err := s.makeTemporaryDir("thumbnail-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	output := filepath.Join(temporary, "thumbnail"+thumbnailExt)
	if err := write(output); err != nil {
		return err
	}
	if info, err := os.Stat(output); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("thumbnail was not generated (%s)", contentKey)
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("cannot create the thumbnail directory (%s): %w", filepath.Dir(target), err)
	}
	if err := os.Rename(output, target); err != nil {
		return fmt.Errorf("cannot publish the thumbnail: %w", err)
	}
	return nil
}

// StashThumbnail supports rollback if persistence fails after publication; restore removes the new
// image when no old one existed. Call restore or discard at least once, and serialize
// publication/deletion in the caller.
func (s *Store) StashThumbnail(contentKey string) (restore func() error, discard func(), err error) {
	target, err := s.publishTarget(s.thumbnailPath, contentKey)
	if err != nil {
		return nil, nil, err
	}
	source, err := os.Open(target)
	if errors.Is(err, fs.ErrNotExist) {
		restore = func() error {
			if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("cannot withdraw the thumbnail (%s): %w", contentKey, err)
			}
			return nil
		}
		return restore, func() {}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the current thumbnail (%s): %w", contentKey, err)
	}
	defer func() { _ = source.Close() }()

	temporary, err := s.makeTemporaryDir("thumbnail-stash-*")
	if err != nil {
		return nil, nil, err
	}
	discard = func() { _ = os.RemoveAll(temporary) }
	backup := filepath.Join(temporary, "thumbnail"+thumbnailExt)
	if err := copyToFile(backup, source); err != nil {
		discard()
		return nil, nil, fmt.Errorf("cannot keep the current thumbnail (%s): %w", contentKey, err)
	}
	restore = func() error {
		defer discard()
		if err := os.Rename(backup, target); err != nil {
			return fmt.Errorf("cannot restore the previous thumbnail (%s): %w", contentKey, err)
		}
		return nil
	}
	return restore, discard, nil
}

func copyToFile(path string, source io.Reader) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, source); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// PublishSeekThumbnails reuses complete sprites or stages sheets and metadata before atomic
// directory publication, removing incomplete legacy output first. It returns the published/reused
// substitution status (unknown for older metadata); callers serialize publication and deletion.
func (s *Store) PublishSeekThumbnails(
	contentKey string, layout domain.SeekSpriteLayout, write func(outputDir string) (fullDecode bool, err error),
) (domain.Substitution, error) {
	unknown := domain.SubstitutionUnknown
	target, err := s.publishTarget(s.seekDir, contentKey)
	if err != nil {
		return unknown, err
	}
	if file, _, err := readSpriteFile(target); err == nil {
		return file.substitution(), nil
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return unknown, fmt.Errorf("cannot create the seek thumbnail directory: %w", err)
	}
	temporary, err := s.makeTemporaryDir("seek-*")
	if err != nil {
		return unknown, err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	fullDecode, err := write(temporary)
	if err != nil {
		return unknown, err
	}
	sprite, err := describeSheets(temporary, layout)
	if err != nil {
		return unknown, fmt.Errorf("seek thumbnails were not generated (%s): %w", contentKey, err)
	}
	file := spriteFileFrom(sprite)
	file.FullDecode = &fullDecode
	data, err := json.Marshal(file)
	if err != nil {
		return unknown, err
	}
	if err := os.WriteFile(filepath.Join(temporary, spriteFileName), append(data, '\n'), 0o644); err != nil {
		return unknown, fmt.Errorf("cannot write the seek thumbnail manifest: %w", err)
	}
	if err := os.RemoveAll(target); err != nil {
		return unknown, fmt.Errorf("cannot delete old seek thumbnails: %w", err)
	}
	// The directory rename publishes metadata and sheets together.
	if err := os.Rename(temporary, target); err != nil {
		if published, _, readErr := readSpriteFile(target); readErr == nil {
			return published.substitution(), nil
		}
		return unknown, fmt.Errorf("cannot publish the seek thumbnails: %w", err)
	}
	return file.substitution(), nil
}

// PublishPreview reuses verified MP4/manifest pairs or stages a new pair; current is checked
// immediately before publication and false returns domain.ErrPreviewStale.
func (s *Store) PublishPreview(
	ctx context.Context, contentKey string,
	write func(output string) error,
	current func(context.Context) (bool, error),
) error {
	target, err := s.publishTarget(func(key string) (string, bool) {
		video, _, ok := s.previewPaths(key)
		return video, ok
	}, contentKey)
	if err != nil {
		return err
	}
	manifest := target + manifestExt
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("cannot create the preview directory: %w", err)
	}
	// All processes sharing the store serialize publication with one lock because the two files cannot
	// be renamed atomically as a pair.
	assetLock := flock.New(filepath.Join(s.root, previewDirName, publishLockName))
	locked, err := assetLock.TryLockContext(ctx, publishLockWait)
	if err != nil {
		return fmt.Errorf("cannot acquire the preview generation lock: %w", err)
	}
	if !locked {
		return errors.New("cannot acquire the preview generation lock")
	}
	defer func() { _ = assetLock.Unlock() }()

	if verifyPreview(target, manifest) == nil {
		return nil
	}
	_ = os.Remove(target)
	_ = os.Remove(manifest)

	temporary, err := s.makeTemporaryDir("preview-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	tmpVideo := filepath.Join(temporary, "preview"+previewExt)
	if err := write(tmpVideo); err != nil {
		return err
	}
	info, err := os.Stat(tmpVideo)
	if err != nil {
		return fmt.Errorf("cannot check the preview: %w", err)
	}
	if info.Size() == 0 {
		return errors.New("preview was not generated: the file is empty")
	}
	digest, err := fileSHA256(tmpVideo)
	if err != nil {
		return err
	}
	data, err := json.Marshal(previewManifest{Version: manifestVersion, Size: info.Size(), SHA256: digest})
	if err != nil {
		return err
	}
	tmpManifest := tmpVideo + manifestExt
	if err := os.WriteFile(tmpManifest, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if current != nil {
		ok, err := current(ctx)
		if err != nil {
			return fmt.Errorf("cannot check whether the preview can be published: %w", err)
		}
		if !ok {
			return domain.ErrPreviewStale
		}
	}
	if err := os.Rename(tmpVideo, target); err != nil {
		if verifyPreview(target, manifest) != nil {
			return fmt.Errorf("cannot publish the preview: %w", err)
		}
		return nil
	}
	if err := os.Rename(tmpManifest, manifest); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("cannot publish the preview manifest: %w", err)
	}
	return nil
}

// ThumbnailFile wraps fs.ErrNotExist on missing files; the caller must close it.
func (s *Store) ThumbnailFile(contentKey string) (*os.File, error) {
	path, ok := s.thumbnailPath(contentKey)
	if !ok {
		return nil, errInvalidKey
	}
	return openRegular(path, nil)
}

// PreviewFile returns the MP4 and manifest SHA-256 after checking file size; missing or
// inconsistent output wraps fs.ErrNotExist, and the caller must close it.
func (s *Store) PreviewFile(contentKey string) (*os.File, string, error) {
	path, manifestPath, ok := s.previewPaths(contentKey)
	if !ok {
		return nil, "", errInvalidKey
	}
	var digest string
	file, err := openRegular(path, func(info os.FileInfo) error {
		manifest, err := checkPreviewComplete(manifestPath, info)
		digest = manifest.SHA256
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return file, digest, nil
}

// SeekSprite distinguishes missing layouts (fs.ErrNotExist) from malformed layouts.
func (s *Store) SeekSprite(contentKey string) (domain.SeekSprite, error) {
	dir, ok := s.seekDir(contentKey)
	if !ok {
		return domain.SeekSprite{}, errInvalidKey
	}
	return readSeekSprite(dir)
}

// SeekSpriteSheet uses zero-based indexes; callers check bounds using SeekSprite, and missing files
// wrap fs.ErrNotExist.
func (s *Store) SeekSpriteSheet(contentKey string, sheet int) ([]byte, error) {
	dir, ok := s.seekDir(contentKey)
	if !ok {
		return nil, errInvalidKey
	}
	// Completed sprites from the previous six-sheet layout remain readable.
	if sheet < 0 || sheet >= 6 {
		return nil, fmt.Errorf("sheet %d: %w", sheet, fs.ErrNotExist)
	}
	return os.ReadFile(filepath.Join(dir, fmt.Sprintf(sheetNameFormat, sheet)))
}

// PreviewAvailable checks regular-file status, nonempty size, and manifest agreement; full SHA-256
// verification happens only during publication to avoid reading every preview on each list request.
func (s *Store) PreviewAvailable(contentKey string) bool {
	path, manifest, ok := s.previewPaths(contentKey)
	if !ok {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	_, err = checkPreviewComplete(manifest, info)
	return err == nil
}

// Readable sprite.json marks completion because the directory is published atomically; legacy
// JPEG-only directories remain incomplete.
func (s *Store) SeekThumbnailsAvailable(contentKey string) bool {
	_, err := s.SeekSprite(contentKey)
	return err == nil
}

// RemoveContent ignores missing artifacts and invalid content keys.
func (s *Store) RemoveContent(contentKey string) error {
	var errs []error
	if path, ok := s.thumbnailPath(contentKey); ok {
		if err := removeFile(path); err != nil {
			errs = append(errs, fmt.Errorf("cannot delete the thumbnail: %w", err))
		}
	}
	if dir, ok := s.seekDir(contentKey); ok {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("cannot delete the seek thumbnails: %w", err))
		}
	}
	if path, manifest, ok := s.previewPaths(contentKey); ok {
		for _, p := range []string{path, manifest} {
			if err := removeFile(p); err != nil {
				errs = append(errs, fmt.Errorf("cannot delete the preview: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}

type spriteFile struct {
	Version     int   `json:"version"`
	IntervalMs  int64 `json:"intervalMs"`
	FrameCount  int   `json:"frameCount"`
	Columns     int   `json:"columns"`
	Rows        int   `json:"rows"`
	FrameWidth  int   `json:"frameWidth"`
	FrameHeight int   `json:"frameHeight"`
	SheetCount  int   `json:"sheetCount"`
	// Absent in metadata written before full-decode tracking.
	FullDecode *bool `json:"fullDecode,omitempty"`
}

func (f spriteFile) substitution() domain.Substitution {
	if f.FullDecode == nil {
		return domain.SubstitutionUnknown
	}
	return domain.SubstitutionOf(*f.FullDecode)
}

func spriteFileFrom(sprite domain.SeekSprite) spriteFile {
	return spriteFile{
		Version:     spriteVersion,
		IntervalMs:  sprite.IntervalMs,
		FrameCount:  sprite.FrameCount,
		Columns:     sprite.Columns,
		Rows:        sprite.Rows,
		FrameWidth:  sprite.FrameWidth,
		FrameHeight: sprite.FrameHeight,
		SheetCount:  sprite.SheetCount,
	}
}

func readSeekSprite(dir string) (domain.SeekSprite, error) {
	_, sprite, err := readSpriteFile(dir)
	return sprite, err
}

func readSpriteFile(dir string) (spriteFile, domain.SeekSprite, error) {
	data, err := os.ReadFile(filepath.Join(dir, spriteFileName))
	if err != nil {
		return spriteFile{}, domain.SeekSprite{}, err
	}
	var file spriteFile
	if err := json.Unmarshal(data, &file); err != nil {
		return spriteFile{}, domain.SeekSprite{}, fmt.Errorf("cannot read the seek thumbnail manifest: %w", err)
	}
	sprite := domain.SeekSprite{
		SeekSpriteLayout: domain.SeekSpriteLayout{
			IntervalMs: file.IntervalMs,
			FrameCount: file.FrameCount,
			Columns:    file.Columns,
			Rows:       file.Rows,
			SheetCount: file.SheetCount,
		},
		FrameWidth:  file.FrameWidth,
		FrameHeight: file.FrameHeight,
	}
	if file.Version != spriteVersion || !validSeekSprite(sprite) {
		return spriteFile{}, domain.SeekSprite{}, errors.New("the seek thumbnail manifest has an unexpected shape")
	}
	return file, sprite, nil
}

func validSeekSprite(sprite domain.SeekSprite) bool {
	if sprite.IntervalMs < domain.SeekThumbnailInterval.Milliseconds() ||
		sprite.FrameCount < 1 || sprite.FrameCount > 600 ||
		// Divide before multiplying to bound frames per sheet without integer overflow.
		sprite.Columns < 1 || sprite.Columns > 600 ||
		sprite.Rows < 1 || sprite.Rows > 600/sprite.Columns ||
		sprite.FrameWidth < 2 || sprite.FrameHeight < 2 ||
		sprite.SheetCount < 1 || sprite.SheetCount > 6 {
		return false
	}
	perSheet := sprite.Columns * sprite.Rows
	return sprite.SheetCount == (sprite.FrameCount+perSheet-1)/perSheet
}

// Derive frame dimensions from actual JPEG sheets, avoiding duplication of ffmpeg
// rotation/rounding; require every sheet to decode with the same dimensions before publishing.
func describeSheets(dir string, layout domain.SeekSpriteLayout) (domain.SeekSprite, error) {
	var width, height int
	for sheet := range layout.SheetCount {
		config, err := sheetConfig(filepath.Join(dir, fmt.Sprintf(sheetNameFormat, sheet)))
		if err != nil {
			return domain.SeekSprite{}, fmt.Errorf("sheet %d: %w", sheet, err)
		}
		if sheet == 0 {
			width, height = config.Width, config.Height
		} else if config.Width != width || config.Height != height {
			return domain.SeekSprite{}, fmt.Errorf("sheet %d is %dx%d, but sheet 0 is %dx%d",
				sheet, config.Width, config.Height, width, height)
		}
	}
	if layout.Columns < 1 || layout.Rows < 1 || layout.SheetCount < 1 ||
		width%layout.Columns != 0 || height%layout.Rows != 0 {
		return domain.SeekSprite{}, fmt.Errorf("sheet 0 size %dx%d does not divide into a %dx%d grid",
			width, height, layout.Columns, layout.Rows)
	}
	sprite := domain.SeekSprite{
		SeekSpriteLayout: layout,
		FrameWidth:       width / layout.Columns,
		FrameHeight:      height / layout.Rows,
	}
	if !validSeekSprite(sprite) {
		return domain.SeekSprite{}, errors.New("sheet layout is out of range")
	}
	return sprite, nil
}

func sheetConfig(path string) (image.Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return image.Config{}, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return image.Config{}, errors.New("empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return image.Config{}, err
	}
	defer func() { _ = file.Close() }()
	config, err := jpeg.DecodeConfig(file)
	if err != nil {
		return image.Config{}, fmt.Errorf("cannot read as JPEG: %w", err)
	}
	return config, nil
}

type previewManifest struct {
	Version int    `json:"version"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

func readManifest(path string) (previewManifest, error) {
	var manifest previewManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Version != manifestVersion || len(manifest.SHA256) != sha256.Size*2 {
		return manifest, errors.New("preview manifest has an unknown format")
	}
	return manifest, nil
}

// Use the same size-and-manifest completeness rule for availability and serving.
func checkPreviewComplete(manifestPath string, video os.FileInfo) (previewManifest, error) {
	if video.Size() == 0 {
		return previewManifest{}, errors.New("preview is empty")
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return previewManifest{}, err
	}
	if manifest.Size != video.Size() {
		return previewManifest{}, errors.New("preview manifest does not match size")
	}
	return manifest, nil
}

func verifyPreview(path, manifestPath string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("preview is not a regular file")
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return err
	}
	if manifest.Size != info.Size() {
		return errors.New("preview manifest does not match size")
	}
	actual, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(manifest.SHA256, actual) {
		return errors.New("preview manifest does not match digest")
	}
	return nil
}

// openRegular closes files failing validation and wraps fs.ErrNotExist.
func openRegular(path string, check func(os.FileInfo) error) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("not a regular file")
	}
	if err == nil && check != nil {
		err = check(info)
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("%s: %w: %w", path, fs.ErrNotExist, err)
	}
	return file, nil
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

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
