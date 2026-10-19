package scanner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

var ErrNoMediaFolders = errors.New("no media folders are configured")

// Includes symlink roots or parents.
var errNotDirectory = errors.New("not a directory")

var errNotRegular = errors.New("not a regular file")

// Index unsupported playback formats too, so users can still see everything in their library.
var mediaExtensions = map[string]struct{}{
	".mp4": {}, ".m4v": {}, ".webm": {}, ".mkv": {}, ".mov": {}, ".avi": {},
	".wmv": {}, ".flv": {}, ".ts": {}, ".m2ts": {}, ".mts": {}, ".mpg": {}, ".mpeg": {},
}

// Skip NAS metadata and filesystem recovery directories entirely.
var excludedNames = map[string]struct{}{
	"@eaDir":     {}, // Synology thumbnail storage.
	"#recycle":   {}, // Synology recycle bin.
	"lost+found": {}, // fsck recovery storage.
}

// Exclude incomplete writes to avoid deriving content keys from partial bytes.
var partialExtensions = map[string]struct{}{
	".part": {}, ".crdownload": {}, ".tmp": {},
}

// Index hides persistence from the scanner.
type Index interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	IndexedVideosByPath(ctx context.Context) (map[string]domain.IndexedVideo, error)
	UpsertVideo(ctx context.Context, file domain.VideoFile) (domain.UpsertResult, error)
	// DeleteVideoLocations removes missing filesystem locations and orphan videos.
	DeleteVideoLocations(ctx context.Context, ids []int64) error
	// A zero creation time clears the stored value to null.
	UpdateLocationCreatedAt(ctx context.Context, locationID int64, createdAt time.Time) error
	// MarkForced records that a force rescan queued the video's forced jobs; ForcedVideos lists those videos.
	MarkForced(ctx context.Context, scanID, videoID int64) error
	ForcedVideos(ctx context.Context, scanID int64) ([]int64, error)
}

// Queue all jobs for one video in one call so they share one import activity and decider; nil
// disables queuing.
type Queue interface {
	EnqueueJobs(ctx context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind) error
	EnsureJobs(ctx context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind) error
	// ForceJobs queues forced jobs that regenerate even when the stage states are done.
	ForceJobs(ctx context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind) error
}

// Reporter is optional and hides progress/error persistence from the scanner.
type Reporter interface {
	// Called after target enumeration and after each processed file.
	ReportScanProgress(ctx context.Context, result domain.ScanResult) error
	ReportScanIssue(ctx context.Context, issue domain.ScanFileIssue) error
	// videoID is zero when unknown; an empty path marks registration completion.
	ReportScanFile(path string, videoID int64)
}

// videoID is zero when the existing video is unknown.
type fileError struct {
	kind    domain.ScanIssueKind
	videoID int64
	err     error
}

func (e *fileError) Error() string { return e.err.Error() }
func (e *fileError) Unwrap() error { return e.err }

func failFile(kind domain.ScanIssueKind, videoID int64, err error) error {
	return &fileError{kind: kind, videoID: videoID, err: err}
}

type Options struct {
	Index    Index
	Queue    Queue
	Reporter Reporter
	// Nil uses slog.Default.
	Logger *slog.Logger
}

type Scanner struct {
	index      Index
	queue      Queue
	reporter   Reporter
	logger     *slog.Logger
	contentKey func(string) (string, error)
	walkDir    func(string, fs.WalkDirFunc) error
	lstat      func(string) (fs.FileInfo, error)
	// Replaceable creation-time reader for tests.
	createdAt func(string, fs.FileInfo) (time.Time, bool)
}

type scanTarget struct {
	path string
	// unchanged marks a force-mode target whose size and mtime matched the index during enumeration.
	unchanged bool
}

func New(opts Options) *Scanner {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{
		index:      opts.Index,
		queue:      opts.Queue,
		reporter:   opts.Reporter,
		logger:     logger,
		contentKey: ContentKey,
		walkDir:    filepath.WalkDir,
		lstat:      os.Lstat,
		createdAt:  fileCreatedAt,
	}
}

type scanPass struct {
	indexed map[string]domain.IndexedVideo
	result  domain.ScanResult
	// Unseen indexed paths become deletion candidates.
	seen map[string]struct{}
	// Collect all roots before processing to establish the progress denominator; unchanged files are
	// excluded.
	targets []scanTarget
	decider domain.ActivityDecider
	// Scoped scans defer files changed or removed mid-ingestion to the next watcher event.
	scoped bool
	// Keep locations under directories that vanish mid-scan until the next watcher scan handles their
	// move/deletion.
	vanished []domain.DirtyDirectory
	// Nil outside a force rescan.
	force *forcePass
}

// Scan updates only the index, never source files; unchanged size/mtime avoids rehashing, and
// content identity preserves moves. Individual file failures do not stop the pass, and decider
// propagates to queued work.
func (s *Scanner) Scan(ctx context.Context, decider domain.ActivityDecider) (domain.ScanResult, error) {
	return s.scan(ctx, nil, false, decider)
}

// ScanScoped adds all discovered locations before deleting unseen ones within its actual scope,
// preserving moves between directories and leaving outside locations untouched. Superseded scans
// stop between files without deleting and return ErrScanSuperseded.
func (s *Scanner) ScanScoped(ctx context.Context, dirs []domain.DirtyDirectory) (domain.ScanResult, error) {
	return s.scan(ctx, domain.NormalizeDirtyDirectories(dirs), true, domain.ActivityByRule)
}

func (s *Scanner) scan(
	ctx context.Context, dirs []domain.DirtyDirectory, scoped bool, decider domain.ActivityDecider,
) (domain.ScanResult, error) {
	result, err := s.scanOnce(ctx, dirs, scoped, decider)
	return result, supersededError(ctx, err)
}

func supersededError(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil && errors.Is(context.Cause(ctx), domain.ErrScanSuperseded) {
		return domain.ErrScanSuperseded
	}
	return err
}

func (s *Scanner) startPass(
	ctx context.Context, scoped bool, decider domain.ActivityDecider,
) ([]domain.MediaFolder, *scanPass, error) {
	folders, err := s.index.ListMediaFolders(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(folders) == 0 {
		return nil, nil, ErrNoMediaFolders
	}
	indexed, err := s.index.IndexedVideosByPath(ctx)
	if err != nil {
		return nil, nil, err
	}
	pass := &scanPass{
		indexed: indexed, seen: map[string]struct{}{}, targets: []scanTarget{}, scoped: scoped, decider: decider,
	}
	return folders, pass, nil
}

func (s *Scanner) scanOnce(
	ctx context.Context, dirs []domain.DirtyDirectory, scoped bool, decider domain.ActivityDecider,
) (domain.ScanResult, error) {
	folders, pass, err := s.startPass(ctx, scoped, decider)
	if err != nil {
		return domain.ScanResult{}, err
	}
	if !scoped {
		return s.walkAndFinish(ctx, pass, folders, folders, func(string) bool { return true })
	}

	// Ignore unrelated unreadable media folders during scoped scans.
	var scanned []domain.MediaFolder
	for _, folder := range folders {
		if slices.ContainsFunc(dirs, func(d domain.DirtyDirectory) bool {
			return domain.PathWithinRoot(folder.Path, d.Path)
		}) {
			scanned = append(scanned, folder)
		}
	}

	// Derive deletion scope only from directories actually traversed, excluding out-of-root and
	// ancestor entries.
	var collected []domain.DirtyDirectory
	for _, folder := range scanned {
		root := folder.Path
		if err := checkMediaFolder(root); err != nil {
			return domain.ScanResult{}, err
		}
		for _, dir := range dirs {
			if !domain.PathWithinRoot(root, dir.Path) {
				continue
			}
			collected = append(collected, dir)
			if err := s.collectDirectory(ctx, pass, root, dir); err != nil {
				if ctx.Err() != nil {
					return domain.ScanResult{}, ctx.Err()
				}
				return domain.ScanResult{}, err
			}
		}
	}
	return s.finishPass(ctx, pass, folders, scanned, func(path string) bool {
		return domain.DirtyDirectoriesContain(collected, path) &&
			!domain.DirtyDirectoriesContain(pass.vanished, path)
	})
}

// walkAndFinish walks every scanned folder as a manual scan does, then removes the missing locations inScope
// accepts.
func (s *Scanner) walkAndFinish(
	ctx context.Context, pass *scanPass, folders, scanned []domain.MediaFolder, inScope func(string) bool,
) (domain.ScanResult, error) {
	for _, folder := range scanned {
		root := folder.Path
		if err := checkMediaFolder(root); err != nil {
			return domain.ScanResult{}, err
		}
		err := s.walkDir(root, func(path string, entry fs.DirEntry, err error) error {
			return s.visit(ctx, pass, root, root, true, path, entry, err)
		})
		if err != nil {
			if ctx.Err() != nil {
				return domain.ScanResult{}, ctx.Err()
			}
			return domain.ScanResult{}, fmt.Errorf("could not finish scanning the media folder (%s): %w", root, err)
		}
	}
	return s.finishPass(ctx, pass, folders, scanned, inScope)
}

// ingestPass fixes the progress denominator and ingests the collected targets.
func (s *Scanner) ingestPass(ctx context.Context, pass *scanPass) (domain.ScanResult, error) {
	result := pass.result
	result.Total += len(pass.targets)
	// Enumeration uses indeterminate total=0; processing starts with a fixed denominator.
	if err := s.report(ctx, result); err != nil {
		return result, err
	}
	err := s.ingestTargets(ctx, pass, &result)
	return result, err
}

func (s *Scanner) finishPass(
	ctx context.Context, pass *scanPass, folders, scanned []domain.MediaFolder, inScope func(string) bool,
) (domain.ScanResult, error) {
	result, err := s.ingestPass(ctx, pass)
	if err != nil {
		return result, err
	}
	if err := s.dropVanished(pass, scanned); err != nil {
		return result, err
	}

	// Superseded scans keep additions and perform no deletions.
	if err := ctx.Err(); err != nil {
		return result, err
	}
	removed, err := s.removeMissing(ctx, folders, pass.seen, inScope)
	if err != nil {
		return result, err
	}
	result.Removed = removed

	return result, nil
}

func (s *Scanner) ingestTargets(ctx context.Context, pass *scanPass, result *domain.ScanResult) error {
	for _, target := range pass.targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		videoID := pass.indexed[target.path].ID
		s.reportFile(target.path, videoID)
		if err := s.ingest(ctx, pass, target, result); err != nil {
			if ctx.Err() != nil {
				return err
			}
			if err := s.recordIngestFailure(ctx, pass, result, target.path, videoID, err); err != nil {
				return err
			}
		} else {
			result.Processed++
		}

		// Connections coalesce progress so slow consumers do not accumulate stale updates.
		if err := s.report(ctx, *result); err != nil {
			return err
		}
	}
	s.reportFile("", 0)
	return nil
}

// Individual failures do not stop scanning; watcher scans defer changed/vanished files to
// subsequent events rather than counting failures.
func (s *Scanner) recordIngestFailure(
	ctx context.Context, pass *scanPass, result *domain.ScanResult, path string, videoID int64, err error,
) error {
	issue := domain.ScanFileIssue{VideoID: videoID, Path: path, Kind: domain.IssueRegisterFailed}
	var failed *fileError
	if errors.As(err, &failed) {
		issue.Kind = failed.kind
		if failed.videoID != 0 {
			issue.VideoID = failed.videoID
		}
	}
	if pass.scoped && issue.Kind == domain.IssueChangedDuringImport {
		s.logger.Info("a file changed during a watch scan; the next watch scan reads it again",
			slog.String("path", path), slog.Any("error", err))
		result.Total--
		return nil
	}
	s.logger.Warn("could not ingest a file", slog.String("path", path), slog.Any("error", err))
	result.Failed++
	return s.reportIssue(ctx, issue)
}

// Remove vanished files from seen; an unreadable parent invalidates a full scan, while watcher
// scans preserve its locations until the next event so moved targets can be added first.
func (s *Scanner) dropVanished(pass *scanPass, scanned []domain.MediaFolder) error {
	checkedDirs := map[string]struct{}{}
	for _, folder := range scanned {
		if err := ensureReadableDirectory(folder.Path); err != nil {
			code := domain.ScanErrorMediaFolderUnreadable
			if errors.Is(err, errNotDirectory) {
				code = domain.ScanErrorMediaFolderNotDirectory
			}
			return domain.NewScanFailure(code, folder.Path,
				fmt.Errorf("could not check the media folder after the scan (%s): %w", folder.Path, err))
		}
		checkedDirs[folder.Path] = struct{}{}
	}
	keptDirs := map[string]struct{}{}
	for path := range pass.seen {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			keep, err := s.checkVanishedParent(pass.scoped, filepath.Dir(path), checkedDirs, keptDirs)
			if err != nil {
				return err
			}
			if !keep {
				delete(pass.seen, path)
			}
			continue
		}
		if err != nil {
			s.logger.Warn("could not check a file after the scan",
				slog.String("path", path), slog.Any("error", err))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			delete(pass.seen, path)
		}
	}
	return nil
}

// Cache parent checks; scoped scans retain locations when the parent vanished too.
func (s *Scanner) checkVanishedParent(
	scoped bool, parent string, checkedDirs, keptDirs map[string]struct{},
) (keep bool, err error) {
	if _, ok := keptDirs[parent]; ok {
		return true, nil
	}
	if _, ok := checkedDirs[parent]; ok {
		return false, nil
	}
	parentErr := ensureReadableDirectory(parent)
	if parentErr == nil {
		checkedDirs[parent] = struct{}{}
		return false, nil
	}
	if scoped && (errors.Is(parentErr, fs.ErrNotExist) || errors.Is(parentErr, errNotDirectory)) {
		s.logger.Info("a directory vanished during a watch scan; the next watch scan reads it again",
			slog.String("path", parent), slog.Any("error", parentErr))
		keptDirs[parent] = struct{}{}
		return true, nil
	}
	return false, domain.NewScanFailure(domain.ScanErrorLocationUnreadable, parent,
		fmt.Errorf("could not check the parent directory after the scan (%s): %w", parent, parentErr))
}

func checkMediaFolder(root string) error {
	rootInfo, rootErr := os.Lstat(root)
	if rootErr != nil {
		return domain.NewScanFailure(domain.ScanErrorMediaFolderUnreadable, root,
			fmt.Errorf("could not read the media folder (%s): %w", root, rootErr))
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return domain.NewScanFailure(domain.ScanErrorMediaFolderNotDirectory, root,
			fmt.Errorf("the media folder is not a directory (%s)", root))
	}
	return nil
}

// Treat vanished or replaced directories as empty and skip excluded subtrees.
func (s *Scanner) collectDirectory(ctx context.Context, pass *scanPass, root string, dir domain.DirtyDirectory) error {
	if rel, err := filepath.Rel(root, dir.Path); err == nil && rel != "." {
		for _, name := range strings.Split(rel, string(filepath.Separator)) {
			if isExcludedDir(name) {
				return nil
			}
		}
	}
	info, err := s.lstat(dir.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return domain.NewScanFailure(domain.ScanErrorLocationUnreadable, dir.Path,
			fmt.Errorf("could not read a location (%s): %w", dir.Path, err))
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}
	err = s.walkDir(dir.Path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil && path == dir.Path && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return s.visit(ctx, pass, root, dir.Path, dir.Recursive, path, entry, err)
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var failure *domain.ScanFailure
		if errors.As(err, &failure) {
			return err
		}
		return fmt.Errorf("could not finish scanning the directory (%s): %w", dir.Path, err)
	}
	return nil
}

// walkRoot itself bypasses exclusions; recursive=false skips child directories.
func (s *Scanner) visit(
	ctx context.Context, pass *scanPass, folderRoot, walkRoot string, recursive bool,
	path string, entry fs.DirEntry, err error,
) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return s.visitError(pass, folderRoot, path, err)
	}

	if entry.IsDir() {
		if path != walkRoot && !recursive {
			return fs.SkipDir
		}
		if path != walkRoot && isExcludedDir(entry.Name()) {
			return fs.SkipDir
		}
		return nil
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return nil
	}

	if !isMediaFile(entry.Name()) {
		return nil
	}

	pass.seen[path] = struct{}{}
	info, infoErr := entry.Info()
	if infoErr != nil && pass.scoped && errors.Is(infoErr, fs.ErrNotExist) {
		// Keep vanished entries in seen until dropVanished checks whether their parent also vanished.
		return nil
	}
	if infoErr == nil && !info.Mode().IsRegular() {
		// A nonregular entry discovered during enumeration is an invalid video, not a mid-scan change.
		infoErr = fmt.Errorf("%w (%s)", errNotRegular, path)
	}
	if infoErr != nil {
		// Count media candidates and failures even if metadata cannot be read.
		pass.result.Total++
		pass.result.Failed++
		s.logger.Warn("could not read the file information of a scan target",
			slog.String("path", path), slog.Any("error", infoErr))
		return s.reportIssue(ctx, domain.ScanFileIssue{
			VideoID: pass.indexed[path].ID, Path: path, Kind: domain.IssueUnreadable,
		})
	}

	if existing, ok := pass.indexed[path]; ok && sameFile(existing, info) {
		if pass.force != nil {
			// Force mode reads the content key of an unchanged file again.
			pass.targets = append(pass.targets, scanTarget{path: path, unchanged: true})
			return nil
		}
		// For unchanged files, update creation time and repair missing pending jobs without reviving
		// terminal failures.
		createdErr := s.refreshCreatedAt(ctx, path, info, existing)
		jobsErr := s.ensurePendingJobs(ctx, existing, pass.decider)
		if err := errors.Join(createdErr, jobsErr); err != nil {
			if ctx.Err() != nil {
				return err
			}
			s.logger.Warn("could not update an indexed file",
				slog.String("path", path), slog.Any("error", err))
			pass.result.Total++
			pass.result.Failed++
			return s.reportIssue(ctx, domain.ScanFileIssue{
				VideoID: existing.ID, Path: path, Kind: domain.IssueRegisterFailed,
			})
		}
		return nil
	}

	pass.targets = append(pass.targets, scanTarget{path: path})
	return nil
}

// Watcher scans remember vanished directories and skip their subtrees instead of failing.
func (s *Scanner) visitError(pass *scanPass, folderRoot, path string, err error) error {
	if pass.scoped && path != folderRoot && errors.Is(err, fs.ErrNotExist) {
		s.logger.Info("a directory vanished during a watch scan; the next watch scan reads it again",
			slog.String("path", path), slog.Any("error", err))
		pass.vanished = append(pass.vanished, domain.DirtyDirectory{Path: path, Recursive: true})
		return nil
	}
	s.logger.Warn("could not read a location during the scan",
		slog.String("path", path), slog.Any("error", err))
	// An unreadable root is a media-folder failure, not a nested-path failure.
	code := domain.ScanErrorLocationUnreadable
	if path == folderRoot {
		code = domain.ScanErrorMediaFolderUnreadable
	}
	return domain.NewScanFailure(code, path, fmt.Errorf("could not read a location (%s): %w", path, err))
}

func ensureReadableDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errNotDirectory
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	_, readErr := dir.Readdirnames(1)
	closeErr := dir.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	return closeErr
}

func (s *Scanner) ingest(ctx context.Context, pass *scanPass, target scanTarget, result *domain.ScanResult) error {
	file, info, err := s.readTarget(target.path, pass.decider)
	if err != nil {
		return err
	}
	if existing := pass.indexed[target.path]; target.unchanged &&
		existing.ContentKey == file.ContentKey && sameFile(existing, info) {
		return s.forceUnchanged(ctx, pass, target.path, info, existing)
	}

	upserted, err := s.index.UpsertVideo(ctx, file)
	if err != nil {
		return failFile(domain.IssueRegisterFailed, 0, err)
	}

	switch upserted.Outcome {
	case domain.OutcomeAdded:
		result.Added++
	case domain.OutcomeUpdated:
		result.Updated++
	case domain.OutcomeMoved:
		result.Moved++
	}
	if upserted.Outcome != domain.OutcomeUnchanged {
		if err := s.enqueue(ctx, upserted, pass.decider); err != nil {
			return failFile(domain.IssueRegisterFailed, upserted.ID, err)
		}
	}
	return s.force(ctx, pass, upserted.ID)
}

// readTarget computes the content key between two stable metadata reads, so a file changed while it is read is
// rejected.
func (s *Scanner) readTarget(path string, decider domain.ActivityDecider) (domain.VideoFile, fs.FileInfo, error) {
	info, err := s.stableTargetInfo(path)
	if err != nil {
		return domain.VideoFile{}, nil, failFile(unreadableKind(err), 0, err)
	}

	key, err := s.contentKey(path)
	if err != nil {
		return domain.VideoFile{}, nil, failFile(unreadableKind(err), 0, err)
	}
	after, err := s.stableTargetInfo(path)
	if err != nil {
		return domain.VideoFile{}, nil, failFile(unreadableKind(err), 0, err)
	}
	if !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return domain.VideoFile{}, nil, failFile(domain.IssueChangedDuringImport, 0,
			fmt.Errorf("the file changed while it was being ingested (%s)", path))
	}

	file := domain.VideoFile{
		Path:       path,
		Title:      titleOf(path),
		ContentKey: key,
		SizeBytes:  info.Size(),
		MTime:      info.ModTime(),
		Container:  domain.ContainerFromPath(path),
		Decider:    decider,
	}
	// Missing creation time is optional and remains zero.
	if createdAt, ok := s.createdAt(path, info); ok {
		file.FileCreatedAt = createdAt
	}
	return file, info, nil
}

func sameFile(existing domain.IndexedVideo, info fs.FileInfo) bool {
	return existing.SizeBytes == info.Size() && existing.MTime.Equal(info.ModTime())
}

// Only disappearance or replacement by a nonregular file proves a mid-import change; permission and
// I/O failures remain unreadable.
func unreadableKind(err error) domain.ScanIssueKind {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errNotRegular) {
		return domain.IssueChangedDuringImport
	}
	return domain.IssueUnreadable
}

func (s *Scanner) stableTargetInfo(path string) (fs.FileInfo, error) {
	info, err := s.lstat(path)
	if err != nil {
		return nil, fmt.Errorf("could not read the file information (%s): %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w (%s)", errNotRegular, path)
	}
	return info, nil
}

// Compare creation time at stored second precision, clearing unavailable values to null without
// touching content keys or jobs.
func (s *Scanner) refreshCreatedAt(
	ctx context.Context, path string, info fs.FileInfo, existing domain.IndexedVideo,
) error {
	createdAt, ok := s.createdAt(path, info)
	if !ok {
		createdAt = time.Time{}
	}
	if sameCreatedAt(createdAt, existing.FileCreatedAt) {
		return nil
	}
	locationID := existing.LocationID
	if locationID == 0 {
		locationID = existing.ID
	}
	return s.index.UpdateLocationCreatedAt(ctx, locationID, createdAt)
}

func sameCreatedAt(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return a.IsZero() == b.IsZero()
	}
	return a.Unix() == b.Unix()
}

func (s *Scanner) ensurePendingJobs(ctx context.Context, video domain.IndexedVideo, decider domain.ActivityDecider) error {
	if s.queue == nil {
		return nil
	}
	states := []struct {
		kind    domain.JobKind
		pending bool
	}{
		{kind: domain.JobProbe, pending: video.ProbeState == domain.ProbeStatePending},
		{kind: domain.JobThumbnail, pending: video.ThumbnailState == domain.ThumbnailStatePending},
		// Only RetryProbe revives failed stages.
		{kind: domain.JobSeekThumbnail, pending: video.SeekThumbnailState == domain.SeekThumbnailPending},
		{kind: domain.JobPreview, pending: video.ProbeState == domain.ProbeStateDone && video.PreviewState == domain.PreviewStatePending},
		// Fingerprints have no status column, so scans also requeue exhausted fingerprint jobs.
		{kind: domain.JobFingerprint, pending: video.FingerprintMissing},
	}
	var kinds []domain.JobKind
	for _, state := range states {
		if state.pending {
			kinds = append(kinds, state.kind)
		}
	}
	if len(kinds) == 0 {
		return nil
	}
	return s.queue.EnsureJobs(ctx, video.ID, decider, kinds...)
}

func (s *Scanner) enqueue(ctx context.Context, result domain.UpsertResult, decider domain.ActivityDecider) error {
	if s.queue == nil {
		return nil
	}
	kinds := []domain.JobKind{}
	if result.NeedsProbe {
		kinds = append(kinds, domain.JobProbe)
	}
	if result.NeedsThumbnail {
		kinds = append(kinds, domain.JobThumbnail)
	}
	if result.NeedsSeekThumbnail {
		kinds = append(kinds, domain.JobSeekThumbnail)
	}
	if result.NeedsPreview {
		kinds = append(kinds, domain.JobPreview)
	}
	if len(kinds) == 0 {
		return nil
	}
	return s.queue.EnqueueJobs(ctx, result.ID, decider, kinds...)
}

// Additions already relocate matching content keys before deletion; never remove paths outside
// inScope.
func (s *Scanner) removeMissing(
	ctx context.Context, folders []domain.MediaFolder, seen map[string]struct{}, inScope func(string) bool,
) (int, error) {
	current, err := s.index.IndexedVideosByPath(ctx)
	if err != nil {
		return 0, err
	}

	var missing []int64
	for path, row := range current {
		if _, ok := seen[path]; ok || !inScope(path) {
			continue
		}
		managed := false
		for _, folder := range folders {
			if domain.PathWithinRoot(folder.Path, path) {
				managed = true
				break
			}
		}
		if !managed {
			continue
		}
		locationID := row.LocationID
		if locationID == 0 {
			locationID = row.ID
		}
		missing = append(missing, locationID)
	}
	if len(missing) == 0 {
		return 0, nil
	}

	if err := s.index.DeleteVideoLocations(ctx, missing); err != nil {
		return 0, err
	}
	return len(missing), nil
}

// Stop on reporting failure to keep recorded progress consistent with results.
func (s *Scanner) report(ctx context.Context, result domain.ScanResult) error {
	if s.reporter == nil {
		return nil
	}
	if err := s.reporter.ReportScanProgress(ctx, result); err != nil {
		return fmt.Errorf("could not record the scan progress: %w", err)
	}
	return nil
}

// An empty path marks registration completion.
func (s *Scanner) reportFile(path string, videoID int64) {
	if s.reporter != nil {
		s.reporter.ReportScanFile(path, videoID)
	}
}

// Stop if a failure cannot be recorded; otherwise it becomes invisible to users.
func (s *Scanner) reportIssue(ctx context.Context, issue domain.ScanFileIssue) error {
	if s.reporter == nil {
		return nil
	}
	if err := s.reporter.ReportScanIssue(ctx, issue); err != nil {
		return fmt.Errorf("could not record a file the scan could not import (%s): %w", issue.Path, err)
	}
	return nil
}

func isMediaFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}

	ext := strings.ToLower(filepath.Ext(name))

	// Do not hash partially written contents.
	if _, ok := partialExtensions[ext]; ok {
		return false
	}

	_, ok := mediaExtensions[ext]
	return ok
}

func isExcludedDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	_, ok := excludedNames[name]
	return ok
}

// Use the full filename if removing its extension would leave an empty title.
func titleOf(path string) string {
	name := filepath.Base(path)
	title := strings.TrimSuffix(name, filepath.Ext(name))
	if title == "" {
		return name
	}
	return norm.NFC.String(title)
}
