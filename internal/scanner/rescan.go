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

// forcePass carries force mode through one pass: done holds the videos the scan has already forced, including
// those an interrupted run forced before a resume.
type forcePass struct {
	scanID int64
	done   map[int64]struct{}
}

// Rescan is the manual scan in force mode over scope, recorded in scan scanID. It reads the content key of an
// unchanged file again, and every video it reaches that the scan has not forced yet gets forced probe,
// thumbnail and seek_thumbnail jobs and is marked forced. A folder scope removes missing locations only under
// that folder; a video scope removes nothing.
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
	pass.force = &forcePass{scanID: scanID, done: make(map[int64]struct{}, len(forced))}
	for _, id := range forced {
		pass.force.done[id] = struct{}{}
	}

	switch scope.Kind {
	case domain.RescanLibrary:
		return s.walkAndFinish(ctx, pass, folders, folders, func(string) bool { return true })
	case domain.RescanFolder:
		i := slices.IndexFunc(folders, func(f domain.MediaFolder) bool { return f.ID == scope.FolderID })
		if i < 0 {
			return domain.ScanResult{}, fmt.Errorf("the media folder of the force rescan is not registered (%s)", scope.Path)
		}
		// Walk the path recorded at start, so a resumed rescan reads the same tree.
		folder := folders[i]
		folder.Path = scope.Path
		return s.walkAndFinish(ctx, pass, folders, []domain.MediaFolder{folder}, func(path string) bool {
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
		pass.targets = append(pass.targets, scanTarget{path: path, unchanged: sameFile(pass.indexed[path], info)})
	}
	return s.ingestPass(ctx, pass)
}

// forceUnchanged handles a force-mode file whose size, mtime and content key all match the index: it takes the
// unchanged-file repair of an ordinary scan, without UpsertVideo, and is then forced.
func (s *Scanner) forceUnchanged(
	ctx context.Context, pass *scanPass, path string, info fs.FileInfo, existing domain.IndexedVideo,
) error {
	if err := s.refreshCreatedAt(ctx, path, info, existing); err != nil {
		return failFile(domain.IssueRegisterFailed, existing.ID, err)
	}
	return s.force(ctx, pass, existing.ID)
}

// force queues the forced jobs of a video once per scan; it does nothing outside force mode.
func (s *Scanner) force(ctx context.Context, pass *scanPass, videoID int64) error {
	if pass.force == nil {
		return nil
	}
	if _, ok := pass.force.done[videoID]; ok {
		return nil
	}
	if s.queue != nil {
		if err := s.queue.ForceJobs(ctx, videoID, pass.decider, forceKinds...); err != nil {
			return failFile(domain.IssueRegisterFailed, videoID, err)
		}
	}
	if err := s.index.MarkForced(ctx, pass.force.scanID, videoID); err != nil {
		return failFile(domain.IssueRegisterFailed, videoID, err)
	}
	pass.force.done[videoID] = struct{}{}
	return nil
}
