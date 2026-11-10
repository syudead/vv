package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
)

// BackgroundCounts is the count read of the ingest role.
type BackgroundCounts interface {
	// BackgroundRemaining counts the unfinished jobs of each job group under the audience.
	BackgroundRemaining(ctx context.Context, audience domain.Audience) (map[domain.BackgroundGroup]int, error)
	VideoIDsByContentKey(ctx context.Context, keys []string) (map[string]int64, error)
}

// BackgroundScanStore reads the latest scan and the watch directories held through a pause.
type BackgroundScanStore interface {
	// CurrentScan returns domain.ErrNotFound before the first scan.
	CurrentScan(ctx context.Context) (domain.Scan, error)
	HeldWatchDirectories(ctx context.Context) ([]domain.DirtyDirectory, error)
}

// BackgroundVideos reads a video by id; a video the audience cannot see is domain.ErrNotFound.
type BackgroundVideos interface {
	GetVideo(ctx context.Context, audience domain.Audience, id int64) (domain.Video, error)
}

// RunningWorkSource is app.Scans, which tracks the running jobs and the scan's registration.
type RunningWorkSource interface {
	Running() []RunningWork
}

type BackgroundStatusOptions struct {
	Counts  BackgroundCounts
	Scans   BackgroundScanStore
	Videos  BackgroundVideos
	Running RunningWorkSource
	// FeatureOff reports the kinds a feature that is off holds back, as PausedJobKinds does; nil reports none.
	FeatureOff func(ctx context.Context) (map[domain.JobKind]bool, error)
}

// BackgroundStatus reads the running work and the work left per group for the owner's screens. See
// docs/design-docs/background-pause.md.
type BackgroundStatus struct {
	counts     BackgroundCounts
	scans      BackgroundScanStore
	videos     BackgroundVideos
	running    RunningWorkSource
	featureOff func(ctx context.Context) (map[domain.JobKind]bool, error)
}

func NewBackgroundStatus(opts BackgroundStatusOptions) *BackgroundStatus {
	return &BackgroundStatus{
		counts: opts.Counts, scans: opts.Scans, videos: opts.Videos, running: opts.Running, featureOff: opts.FeatureOff,
	}
}

// Read fills work's running list and groups for the audience.
func (s *BackgroundStatus) Read(ctx context.Context, audience domain.Audience, work *domain.BackgroundWork) error {
	running, err := s.runningList(ctx, audience)
	if err != nil {
		return err
	}
	groups, err := s.groups(ctx, audience)
	if err != nil {
		return err
	}
	work.Running, work.Groups = running, groups
	return nil
}

func (s *BackgroundStatus) groups(ctx context.Context, audience domain.Audience) ([]domain.BackgroundGroupStatus, error) {
	remaining, err := s.counts.BackgroundRemaining(ctx, audience)
	if err != nil {
		return nil, err
	}
	off := map[domain.JobKind]bool{}
	if s.featureOff != nil {
		if off, err = s.featureOff(ctx); err != nil {
			return nil, fmt.Errorf("cannot read the paused job kinds: %w", err)
		}
	}
	groups := domain.EmptyBackgroundGroups()
	for i := range groups {
		group := &groups[i]
		if group.Group == domain.BackgroundScan {
			if *group, err = s.scanGroup(ctx); err != nil {
				return nil, err
			}
			continue
		}
		group.Remaining = remaining[group.Group]
		for _, kind := range domain.GroupJobKinds(group.Group) {
			group.FeatureOff = group.FeatureOff || off[kind]
		}
	}
	return groups, nil
}

// scanGroup counts the files the latest scan has not read while it runs or waits as paused.
func (s *BackgroundStatus) scanGroup(ctx context.Context) (domain.BackgroundGroupStatus, error) {
	group := domain.BackgroundGroupStatus{Group: domain.BackgroundScan}
	scan, err := s.scans.CurrentScan(ctx)
	switch {
	case errors.Is(err, domain.ErrNotFound):
	case err != nil:
		return group, fmt.Errorf("cannot read the latest scan: %w", err)
	default:
		group.Remaining = scan.Tally(0, 0).PendingFiles()
		group.Waiting = scan.State == domain.ScanPaused
	}
	held, err := s.scans.HeldWatchDirectories(ctx)
	if err != nil {
		return group, err
	}
	group.Waiting = group.Waiting || len(held) > 0
	return group, nil
}

func (s *BackgroundStatus) runningList(ctx context.Context, audience domain.Audience) ([]domain.BackgroundActivity, error) {
	work := s.running.Running()
	var keys []string
	for _, w := range work {
		if namesVideoByKey(w) {
			keys = append(keys, w.ContentKey)
		}
	}
	ids, err := s.counts.VideoIDsByContentKey(ctx, keys)
	if err != nil {
		return nil, err
	}
	list := make([]domain.BackgroundActivity, 0, len(work))
	for _, w := range work {
		entry := domain.BackgroundActivity{Group: domain.BackgroundScan, Kind: domain.BackgroundRegistering}
		if w.Job != "" {
			group, ok := domain.JobGroup(w.Job)
			if !ok {
				continue
			}
			entry.Group, entry.Kind = group, string(w.Job)
		}
		if w.Path != "" {
			entry.FileName = filepath.Base(w.Path)
		}
		id := w.VideoID
		if namesVideoByKey(w) {
			id = ids[w.ContentKey]
		}
		if entry.Video, err = s.listedVideo(ctx, audience, id); err != nil {
			return nil, err
		}
		list = append(list, entry)
	}
	return list, nil
}

// namesVideoByKey reports work whose subject is one video's content key; a duplicate judgement names two.
func namesVideoByKey(w RunningWork) bool {
	return w.VideoID == 0 && w.ContentKey != "" && (w.Job == domain.JobAutoTag || w.Job == domain.JobFace)
}

// listedVideo returns the video when the audience lists it: it can see the video and a folder switched on holds it.
func (s *BackgroundStatus) listedVideo(ctx context.Context, audience domain.Audience, id int64) (*domain.BackgroundVideo, error) {
	if id == 0 {
		return nil, nil
	}
	video, err := s.videos.GetVideo(ctx, audience, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the video of running work (id=%d): %w", id, err)
	}
	if video.FolderOff {
		return nil, nil
	}
	return &domain.BackgroundVideo{ID: video.ID, Title: video.Title}, nil
}
