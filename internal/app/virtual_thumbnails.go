package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/syudead/vv/internal/domain"
)

// A virtual video's thumbnail is only an uploaded image, fitted to 16:9 by ApplyThumbnailImage; no job generates or
// republishes it.

// RepublishVirtualThumbnails publishes, from its stored image, each virtual video's thumbnail whose file is missing.
// The virtual_thumbnails job each start queues runs it, because no thumbnail job exists for a virtual video.
func (i *Ingest) RepublishVirtualThumbnails(ctx context.Context) error {
	keys, err := i.store.VirtualThumbnailKeys(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, key := range keys {
		if i.files.ThumbnailAvailable(key) {
			continue
		}
		// Clean up afterwards in case the virtual video was deleted while this waited for the lock.
		if err := i.artifacts.generate(ctx, key, artifactThumbnail, func() (bool, error) {
			return true, i.republishThumbnailImage(ctx, key)
		}); err != nil {
			errs = append(errs, fmt.Errorf("cannot republish the thumbnail of %s: %w", key, err))
		}
	}
	return errors.Join(errs...)
}

// The image is reread under the generation lock, so an upload or removal that committed meanwhile wins.
func (i *Ingest) republishThumbnailImage(ctx context.Context, key string) error {
	image, err := i.store.ThumbnailImage(ctx, key)
	if err != nil || image == nil {
		return err
	}
	source, err := i.store.ThumbnailImageSource(ctx, key)
	if err != nil {
		return err
	}
	return i.files.PublishThumbnail(key, func(output, poster string) error {
		return writeThumbnailImages(output, image, poster, source)
	})
}

// removeVirtualThumbnailImage records the removal, then deletes the files under the generation lock. A file left
// behind by a failed deletion is never served, because the thumbnail is no longer done.
func (i *Ingest) removeVirtualThumbnailImage(ctx context.Context, video domain.Video) (domain.Video, error) {
	var saved domain.Video
	err := i.artifacts.generate(ctx, video.ContentKey, artifactThumbnail, func() (bool, error) {
		recorded, err := i.store.RemoveVirtualThumbnailImage(ctx, video.ID, video.ContentKey)
		if errors.Is(err, domain.ErrContentChanged) || errors.Is(err, domain.ErrNotVirtual) {
			// The virtual video was deleted and its row id reused.
			return false, domain.ErrNotFound
		}
		if err != nil {
			return false, err
		}
		saved = recorded
		if err := i.files.RemoveContent(video.ContentKey); err != nil {
			i.artifacts.logger.Warn("could not delete the removed thumbnail of a virtual video",
				slog.String("contentKey", video.ContentKey), slog.Any("error", err))
		}
		return false, nil
	})
	if err != nil {
		return domain.Video{}, err
	}
	return saved, nil
}
