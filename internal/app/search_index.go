package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// SearchIndexStore keeps each content's keyframe hashes for image search, apart from import progress.
type SearchIndexStore interface {
	GetVideo(ctx context.Context, id int64) (domain.Video, error)
	// ApplySearchIndexForJob writes only while the claimed content and location still match.
	ApplySearchIndexForJob(ctx context.Context, job domain.Job, index domain.SearchIndex) (bool, error)
}

// KeyframeHashReader reads one video file in one sequential pass, giving keep each keyframe hash in order.
type KeyframeHashReader func(ctx context.Context, path string, keep func(domain.KeyframeHash)) error

// SearchIndexBuilder handles frame_index jobs: a background kind outside import tracking whose failure only leaves
// the content unsearched. See docs/design-docs/job-scheduling.md.
type SearchIndexBuilder struct {
	store   SearchIndexStore
	sources MediaFiles
	read    KeyframeHashReader
}

func NewSearchIndexBuilder(store SearchIndexStore, sources MediaFiles, read KeyframeHashReader) *SearchIndexBuilder {
	return &SearchIndexBuilder{store: store, sources: sources, read: read}
}

// Build reads the job's file and saves the content's index; a file that is unavailable fails the job, which the
// scheduler retries.
func (b *SearchIndexBuilder) Build(ctx context.Context, job domain.Job) error {
	video, err := b.store.GetVideo(ctx, job.VideoID)
	if err != nil {
		return err
	}
	path, err := b.sources.Resolve(ctx, job.LocationPath)
	if err != nil {
		return err
	}
	index := domain.NewSearchIndexCollector(durationOf(video))
	if err := b.read(ctx, path, index.Add); err != nil {
		return err
	}
	_, err = b.store.ApplySearchIndexForJob(ctx, job, index.Index())
	return err
}
