package app

import (
	"context"
	"errors"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// People's images are kept with a fetch like the cover and become faces after the apply commits; see
// docs/design-docs/work-info-plugins.md.

// WorkInfoPersonImageFiller gives a person without a face a fetched image; implemented by PersonImages.
type WorkInfoPersonImageFiller interface {
	// FillPersonImage stores data as the face of a person who has none. An image that cannot be fitted, a person who
	// has a face or no longer exists is not an error.
	FillPersonImage(ctx context.Context, id int64, data []byte) error
}

// keepPersonImages attaches each header-checked image to the person it was fetched for; images aligns with people.
func keepPersonImages(people []domain.FetchedPerson, images [][]byte) {
	for i, data := range images {
		if i < len(people) {
			people[i].Image = fetchedCover(data)
		}
	}
}

// WorkInfoFetchPersonImage returns ErrNotFound for missing/expired results or wrong videos, and
// ErrPersonImageNotFound when the person at index has no kept image or index is outside the result's people.
func (p *WorkInfoPlugins) WorkInfoFetchPersonImage(videoID int64, fetchID string, index int) (domain.WorkInfoCover, error) {
	return p.fetches.personImage(fetchID, videoID, index, p.now())
}

func (f *workInfoFetches) personImage(id string, videoID int64, index int, now time.Time) (domain.WorkInfoCover, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	fetch, ok := f.entries[id]
	switch {
	case !ok || fetch.videoID != videoID:
		return domain.WorkInfoCover{}, domain.ErrNotFound
	case index < 0 || index >= len(fetch.details.People) || fetch.details.People[index].Image == nil:
		return domain.WorkInfoCover{}, domain.ErrPersonImageNotFound
	}
	return *fetch.details.People[index].Image, nil
}

// imageBytes counts the cover and people's images toward WorkInfoFetchCoverBudget.
func (f *workInfoFetch) imageBytes() int {
	total := 0
	if f.cover != nil {
		total += len(f.cover.Data)
	}
	for _, person := range f.details.People {
		if person.Image != nil {
			total += len(person.Image.Data)
		}
	}
	return total
}

// applyPersonImages runs after the apply commits, for the people the apply linked; a failure drops that image
// without failing the apply, which is already written.
func (p *WorkInfoPlugins) applyPersonImages(ctx context.Context, people []domain.FetchedPerson, personIDs []int64) {
	if p.personImages == nil {
		return
	}
	for i, id := range personIDs {
		if id == 0 || i >= len(people) || people[i].Image == nil {
			continue
		}
		_ = p.personImages.FillPersonImage(ctx, id, people[i].Image.Data)
	}
}

// FillPersonImage fits data only when the person has no face, and stores it only if none was set meanwhile.
func (p *PersonImages) FillPersonImage(ctx context.Context, id int64, data []byte) error {
	_, err := p.store.PersonImage(ctx, id)
	switch {
	case errors.Is(err, domain.ErrPersonImageNotFound):
	case err == nil, errors.Is(err, domain.ErrNotFound):
		return nil
	default:
		return err
	}
	fitted, err := p.fit(ctx, data)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		return nil
	}
	_, err = p.store.AddPersonImage(ctx, id, fitted)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}
