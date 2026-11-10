package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// forceKinds are the stages a force rescan queues; preview and fingerprint follow from their forced results.
var forceKinds = []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail}

// forcePass carries force mode through one pass: done holds the videos the scan has already forced and read the
// files it has already read, including those of a run stopped before a resume.
type forcePass struct {
	scanID int64
	done   map[int64]struct{}
	read   map[string]struct{}
}

// Rescan is the manual scan in force mode over scope, recorded in scan scanID. It reads the content key of an
// unchanged file again unless the scan already read that file, and every video it reaches that the scan has not
// forced yet gets forced probe, thumbnail and seek_thumbnail jobs and is marked forced. A folder scope removes
// missing locations only under that folder; a video scope removes nothing.
func (s *Scanner) Rescan(
	ctx context.Context, scanID int64, scope domain.RescanScope, decider domain.ActivityDecider,
) (domain.ScanResult, error) {
	result, err := s.rescanOnce(ctx, scanID, scope, decider)
	return result, supersededError(ctx, err)
}

func (s *Scanner) rescanOnce(
	ctx context.Context, scanID int64, scope domain.RescanScope, decider domain.ActivityDecider,
) (domain.ScanResult, error) {
	folders, pass, err := s.startPass(ctx, false, decider)
	if err != nil {
		return domain.ScanResult{}, err
	}
	forced, err := s.index.ForcedVideos(ctx, scanID)
	if err != nil {
		return domain.ScanResult{}, err
	}
	read, err := s.index.ReadLocations(ctx, scanID)
	if err != nil {
		return domain.ScanResult{}, err
	}
	pass.force = &forcePass{
		scanID: scanID, done: make(map[int64]struct{}, len(forced)), read: make(map[string]struct{}, len(read)),
	}
	for _, id := range forced {
		pass.force.done[id] = struct{}{}
	}
	for _, path := range read {
		pass.force.read[path] = struct{}{}
	}

	switch scope.Kind {
	case domain.RescanLibrary:
		return s.walkAndFinish(ctx, pass, folders, folders, func(string) bool { return true })
	case domain.RescanFolder:
		i := slices.IndexFunc(folders, func(f domain.MediaFolder) bool { return f.ID == scope.FolderID })
		if i < 0 {
			return domain.ScanResult{}, fmt.Errorf("the media folder of the force rescan is not registered (%s)", scope.Path)
		}
		// A resumed rescan reads the tree recorded at start, or stops if the folder now points elsewhere.
		if folders[i].Path != scope.Path {
			return domain.ScanResult{}, fmt.Errorf("the media folder of the force rescan moved from %s to %s",
				scope.Path, folders[i].Path)
		}
		return s.walkAndFinish(ctx, pass, folders, folders[i:i+1], func(path string) bool {
			return domain.PathWithinRoot(scope.Path, path)
		})
	case domain.RescanVideo:
		return s.rescanVideo(ctx, pass, scope.VideoID)
	default:
		return domain.ScanResult{}, domain.ErrInvalidRescanScope
	}
}

// rescanVideo reads each registered location of the video as one target. A missing or unreadable location is
// reported with the video id and kept.
func (s *Scanner) rescanVideo(ctx context.Context, pass *scanPass, videoID int64) (domain.ScanResult, error) {
	var paths []string
	for path, row := range pass.indexed {
		if row.ID == videoID {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	for _, path := range paths {
		info, err := s.stableTargetInfo(path)
		if err != nil {
			pass.result.Total++
			pass.result.Failed++
			s.logger.Warn("could not read a location of the video", slog.String("path", path), slog.Any("error", err))
			issue := domain.ScanFileIssue{VideoID: videoID, Path: path, Kind: unreadableKind(err)}
			if err := s.reportIssue(ctx, issue); err != nil {
				return pass.result, err
			}
			continue
		}
		existing := pass.indexed[path]
		if !sameFile(existing, info) {
			pass.targets = append(pass.targets, scanTarget{path: path})
			continue
		}
		if pass.rereadsUnchanged(path) {
			pass.targets = append(pass.targets, scanTarget{path: path, unchanged: true})
			continue
		}
		if err := s.keepUnchanged(ctx, pass, path, info, existing); err != nil {
			return pass.result, err
		}
	}
	return s.ingestPass(ctx, pass)
}

// rereadsUnchanged reports whether the pass reads the content key of the unchanged file at path again: force mode
// does, except for a file the scan already read, such as one read before a pause of background work. The video's
// forced mark does not decide it, because the video's other locations may not have been read yet.
func (p *scanPass) rereadsUnchanged(path string) bool {
	if p.force == nil {
		return false
	}
	_, read := p.force.read[path]
	return !read
}

// forceUnchanged handles a force-mode file whose size, mtime and content key all match the index: it takes the
// unchanged-file repair of an ordinary scan, without UpsertVideo, and is then forced.
func (s *Scanner) forceUnchanged(
	ctx context.Context, pass *scanPass, path string, info fs.FileInfo, existing domain.IndexedVideo,
) error {
	if err := s.repairUnchanged(ctx, path, info, existing, pass.decider); err != nil {
		return failFile(domain.IssueRegisterFailed, existing.ID, err)
	}
	return s.forceRead(ctx, pass, path, existing.ID)
}

// forceRead forces the video of the file read at path and then marks the file read in the scan. A mark that cannot be
// saved only makes a resumed pass read the file again, so it is logged instead of failing the file.
func (s *Scanner) forceRead(ctx context.Context, pass *scanPass, path string, videoID int64) error {
	if err := s.force(ctx, pass, videoID); err != nil {
		return err
	}
	if pass.force == nil {
		return nil
	}
	if err := s.index.MarkLocationRead(ctx, pass.force.scanID, path); err != nil {
		s.logger.Warn("could not mark a file read by the force rescan; a resumed pass reads it again",
			slog.String("path", path), slog.Any("error", err))
		return nil
	}
	pass.force.read[path] = struct{}{}
	return nil
}

// force queues the forced jobs of a video once per scan; it does nothing outside force mode or without a queue.
func (s *Scanner) force(ctx context.Context, pass *scanPass, videoID int64) error {
	if pass.force == nil || s.queue == nil {
		return nil
	}
	if _, ok := pass.force.done[videoID]; ok {
		return nil
	}
	if err := s.queue.ForceScanJobs(ctx, pass.force.scanID, videoID, pass.decider, forceKinds...); err != nil {
		return failFile(domain.IssueRegisterFailed, videoID, err)
	}
	pass.force.done[videoID] = struct{}{}
	return nil
}
