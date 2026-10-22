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
//	<root>/poster/<p>/<s>.jpg          Player poster, published with the thumbnail
//	<root>/seek/<p>/<s>/<n>.jpg         Three-digit sprite sheet
//	<root>/seek/<p>/<s>/sprite.json     Layout and completion marker
//	<root>/preview/<p>/<s>.mp4          Hover preview
//	<root>/preview/<p>/<s>.mp4.sha256   Integrity manifest
//	<root>/preview/.publish.lock        Publication lock
//	<root>/{seek,preview}/<p>/.*/       Forced rebuilds and their pointers (builds.go)
//	<root>/.tmp/                        Incomplete output
const (
	seekDirName      = "seek"
	posterDirName    = "poster"
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
	return s.buildPath(dir, contentKey, ext, 0)
}

func (s *Store) thumbnailPath(contentKey string) (string, bool) {
	return s.locate("", contentKey, thumbnailExt)
}

func (s *Store) posterPath(contentKey string) (string, bool) {
	return s.locate(posterDirName, contentKey, thumbnailExt)
}

func (s *Store) seekDir(contentKey string, build int) (string, bool) {
	return s.buildPath(seekDirName, contentKey, "", build)
}

func (s *Store) previewPaths(contentKey string, build int) (video, manifest string, ok bool) {
	video, ok = s.buildPath(previewDirName, contentKey, previewExt, build)
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

// PublishThumbnail gives write temporary paths and replaces the thumbnail only after validating
// nonempty output. The poster is optional: without one, the previous poster is removed so it never
// outlives the thumbnail it showed.
func (s *Store) PublishThumbnail(contentKey string, write func(output, poster string) error) error {
	target, err := s.publishTarget(s.thumbnailPath, contentKey)
	if err != nil {
		return err
	}
	posterTarget, err := s.publishTarget(s.posterPath, contentKey)
	if err != nil {
		return err
	}
	temporary, err := s.makeTemporaryDir("thumbnail-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	output := filepath.Join(temporary, "thumbnail"+thumbnailExt)
	poster := filepath.Join(temporary, "poster"+thumbnailExt)
	if err := write(output, poster); err != nil {
		return err
	}
	if !nonEmptyFile(output) {
		return fmt.Errorf("thumbnail was not generated (%s)", contentKey)
	}
	if nonEmptyFile(poster) {
		if err := moveInto(poster, posterTarget); err != nil {
			return fmt.Errorf("cannot publish the poster: %w", err)
		}
	} else if err := removeFile(posterTarget); err != nil {
		return fmt.Errorf("cannot delete the poster: %w", err)
	}
	if err := moveInto(output, target); err != nil {
		return fmt.Errorf("cannot publish the thumbnail: %w", err)
	}
	return nil
}

func nonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func moveInto(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return err
	}
	return os.Rename(source, target)
}

// StashThumbnail supports rollback if persistence fails after publication; restore puts back the
// previous thumbnail and poster, removing a replacement where none existed. Call restore or discard
// at least once, and serialize publication/deletion in the caller.
func (s *Store) StashThumbnail(contentKey string) (restore func() error, discard func(), err error) {
	thumbnail, err := s.publishTarget(s.thumbnailPath, contentKey)
	if err != nil {
		return nil, nil, err
	}
	poster, err := s.publishTarget(s.posterPath, contentKey)
	if err != nil {
		return nil, nil, err
	}
	temporary, err := s.makeTemporaryDir("thumbnail-stash-*")
	if err != nil {
		return nil, nil, err
	}
	discard = func() { _ = os.RemoveAll(temporary) }
	var restores []func() error
	for _, file := range []struct{ name, target string }{{"thumbnail", thumbnail}, {"poster", poster}} {
		put, err := stashFile(file.target, filepath.Join(temporary, file.name+thumbnailExt))
		if err != nil {
			discard()
			return nil, nil, fmt.Errorf("cannot keep the current %s (%s): %w", file.name, contentKey, err)
		}
		restores = append(restores, put)
	}
	restore = func() error {
		defer discard()
		var errs []error
		for _, put := range restores {
			if err := put(); err != nil {
				errs = append(errs, fmt.Errorf("cannot restore the previous thumbnail (%s): %w", contentKey, err))
			}
		}
		return errors.Join(errs...)
	}
	return restore, discard, nil
}

// stashFile copies target to backup; the returned function puts it back, or removes target when
// there was nothing to copy.
func stashFile(target, backup string) (func() error, error) {
	source, err := os.Open(target)
	if errors.Is(err, fs.ErrNotExist) {
		return func() error { return removeFile(target) }, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = source.Close() }()
	if err := copyToFile(backup, source); err != nil {
		return nil, err
	}
	return func() error { return moveInto(backup, target) }, nil
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
// directory publication, removing incomplete legacy output first. A forced publish never reuses: it
// publishes the next build and then switches the pointer to it. It returns the published/reused
// substitution status (unknown for older metadata); callers serialize publication and deletion.
func (s *Store) PublishSeekThumbnails(
	contentKey string, layout domain.SeekSpriteLayout, force bool,
	write func(outputDir string) (fullDecode bool, err error),
) (domain.Substitution, error) {
	unknown := domain.SubstitutionUnknown
	if _, err := s.publishTarget(func(key string) (string, bool) { return s.seekDir(key, 0) }, contentKey); err != nil {
		return unknown, err
	}
	build := s.currentBuild(seekDirName, contentKey)
	current, _ := s.seekDir(contentKey, build)
	target := current
	if force {
		target, _ = s.seekDir(contentKey, build+1)
	} else if file, _, err := readSpriteFile(current); err == nil {
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

	file, err := stageSeekSprite(temporary, contentKey, layout, write)
	if err != nil {
		return unknown, err
	}
	// A leftover is incomplete legacy output, or a forced build interrupted before its pointer switched.
	if err := os.RemoveAll(target); err != nil {
		return unknown, fmt.Errorf("cannot delete old seek thumbnails: %w", err)
	}
	// The directory rename publishes metadata and sheets together.
	if err := os.Rename(temporary, target); err != nil {
		if published, _, readErr := readSpriteFile(target); readErr == nil && !force {
			return published.substitution(), nil
		}
		return unknown, fmt.Errorf("cannot publish the seek thumbnails: %w", err)
	}
	if force {
		if err := s.switchBuild(seekDirName, contentKey, build+1); err != nil {
			_ = removeBuild(target)
			return unknown, err
		}
		// The pointer already names the new build; a failed removal leaves files RemoveContent deletes later.
		_ = removeBuild(current)
	}
	return file.substitution(), nil
}

// stageSeekSprite has write generate the sheets in dir and adds the metadata that marks them complete.
func stageSeekSprite(
	dir, contentKey string, layout domain.SeekSpriteLayout, write func(outputDir string) (bool, error),
) (spriteFile, error) {
	fullDecode, err := write(dir)
	if err != nil {
		return spriteFile{}, err
	}
	sprite, err := describeSheets(dir, layout)
	if err != nil {
		return spriteFile{}, fmt.Errorf("seek thumbnails were not generated (%s): %w", contentKey, err)
	}
	file := spriteFileFrom(sprite)
	file.FullDecode = &fullDecode
	data, err := json.Marshal(file)
	if err != nil {
		return spriteFile{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, spriteFileName), append(data, '\n'), 0o644); err != nil {
		return spriteFile{}, fmt.Errorf("cannot write the seek thumbnail manifest: %w", err)
	}
	return file, nil
}

// PublishPreview reuses verified MP4/manifest pairs or stages a new pair; current is checked
// immediately before publication and false returns domain.ErrPreviewStale. A forced publish never
// reuses: it keeps the published pair until the next build is in place and the pointer names it.
func (s *Store) PublishPreview(
	ctx context.Context, contentKey string, force bool,
	write func(output string) error,
	current func(context.Context) (bool, error),
) error {
	if _, err := s.publishTarget(func(key string) (string, bool) {
		video, _, ok := s.previewPaths(key, 0)
		return video, ok
	}, contentKey); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.root, previewDirName), dirPerm); err != nil {
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

	build := s.currentBuild(previewDirName, contentKey)
	published, publishedManifest, _ := s.previewPaths(contentKey, build)
	next := build
	if force {
		next = build + 1
	} else if verifyPreview(published, publishedManifest) == nil {
		return nil
	}
	target, manifest, _ := s.previewPaths(contentKey, next)
	_ = os.Remove(target)
	_ = os.Remove(manifest)
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("cannot create the preview directory: %w", err)
	}
	if err := s.stagePreview(ctx, target, write, current); err != nil {
		return err
	}
	if force {
		if err := s.switchBuild(previewDirName, contentKey, next); err != nil {
			_ = removeBuild(target, manifest)
			return err
		}
		// The pointer already names the new build; a failed removal leaves files RemoveContent deletes later.
		_ = removeBuild(published, publishedManifest)
	}
	return nil
}

// stagePreview has write encode the preview, adds its manifest and renames both to target and its manifest.
func (s *Store) stagePreview(
	ctx context.Context, target string, write func(output string) error, current func(context.Context) (bool, error),
) error {
	manifest := target + manifestExt
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

// ThumbnailAvailable reports whether a non-empty thumbnail file is published for the key.
func (s *Store) ThumbnailAvailable(contentKey string) bool {
	path, ok := s.thumbnailPath(contentKey)
	return ok && nonEmptyFile(path)
}

// PosterFile wraps fs.ErrNotExist when the thumbnail was published without a poster; the caller
// must close it.
func (s *Store) PosterFile(contentKey string) (*os.File, error) {
	path, ok := s.posterPath(contentKey)
	if !ok {
		return nil, errInvalidKey
	}
	return openRegular(path, nil)
}

// PreviewFile returns the MP4 and manifest SHA-256 after checking file size; missing or
// inconsistent output wraps fs.ErrNotExist, and the caller must close it.
func (s *Store) PreviewFile(contentKey string) (*os.File, string, error) {
	var digest string
	file, err := readResolved(s, previewDirName, contentKey, func(build int) (*os.File, error) {
		path, manifestPath, ok := s.previewPaths(contentKey, build)
		if !ok {
			return nil, errInvalidKey
		}
		return openRegular(path, func(info os.FileInfo) error {
			manifest, err := checkPreviewComplete(manifestPath, info)
			digest = manifest.SHA256
			return err
		})
	})
	if err != nil {
		return nil, "", err
	}
	return file, digest, nil
}

// SeekSprite distinguishes missing layouts (fs.ErrNotExist) from malformed layouts.
func (s *Store) SeekSprite(contentKey string) (domain.SeekSprite, error) {
	return readResolved(s, seekDirName, contentKey, func(build int) (domain.SeekSprite, error) {
		dir, ok := s.seekDir(contentKey, build)
		if !ok {
			return domain.SeekSprite{}, errInvalidKey
		}
		return readSeekSprite(dir)
	})
}

// SeekSpriteSheet uses zero-based indexes; callers check bounds using SeekSprite, and missing files
// wrap fs.ErrNotExist.
func (s *Store) SeekSpriteSheet(contentKey string, sheet int) ([]byte, error) {
	// Completed sprites from the previous six-sheet layout remain readable.
	if sheet < 0 || sheet >= 6 {
		return nil, fmt.Errorf("sheet %d: %w", sheet, fs.ErrNotExist)
	}
	return readResolved(s, seekDirName, contentKey, func(build int) ([]byte, error) {
		dir, ok := s.seekDir(contentKey, build)
		if !ok {
			return nil, errInvalidKey
		}
		return os.ReadFile(filepath.Join(dir, fmt.Sprintf(sheetNameFormat, sheet)))
	})
}

// PreviewAvailable checks regular-file status, nonempty size, and manifest agreement; full SHA-256
// verification happens only during publication to avoid reading every preview on each list request.
func (s *Store) PreviewAvailable(contentKey string) bool {
	_, err := readResolved(s, previewDirName, contentKey, func(build int) (struct{}, error) {
		path, manifest, ok := s.previewPaths(contentKey, build)
		if !ok {
			return struct{}{}, errInvalidKey
		}
		info, err := os.Stat(path)
		if err == nil && !info.Mode().IsRegular() {
			err = errors.New("not a regular file")
		}
		if err == nil {
			_, err = checkPreviewComplete(manifest, info)
		}
		if err != nil {
			return struct{}{}, fmt.Errorf("%w: %w", fs.ErrNotExist, err)
		}
		return struct{}{}, nil
	})
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
	if path, ok := s.posterPath(contentKey); ok {
		if err := removeFile(path); err != nil {
			errs = append(errs, fmt.Errorf("cannot delete the poster: %w", err))
		}
	}
	if err := s.removeAllBuilds(seekDirName, contentKey, ""); err != nil {
		errs = append(errs, fmt.Errorf("cannot delete the seek thumbnails: %w", err))
	}
	if err := s.removeAllBuilds(previewDirName, contentKey, previewExt, previewExt+manifestExt); err != nil {
		errs = append(errs, fmt.Errorf("cannot delete the preview: %w", err))
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
