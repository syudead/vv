package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// People's images are kept with a fetch like the cover and become faces after the apply commits; see
// docs/design-docs/work-info-plugins.md.

// WorkInfoPersonImageFiller gives a person without a face a fetched image; implemented by PersonImages.
type WorkInfoPersonImageFiller interface {
	// FillPersonImage stores data as the face of a person who has none and returns its revision, or 0 when it stored
	// nothing. An image that cannot be fitted, a person who has a face or no longer exists is not an error.
	FillPersonImage(ctx context.Context, id int64, data []byte) (int64, error)
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

// imageBytes counts the cover, people's images and lookup images toward WorkInfoFetchCoverBudget.
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
	for _, run := range f.lookupRuns {
		if run != nil && run.image != nil {
			total += len(run.image.Data)
		}
	}
	return total
}

// personFace is a kept image the apply offers to a linked person without a face.
type personFace struct {
	personID int64
	image    *domain.WorkInfoCover
}

// appliedFaces lists the selected lookup images before the work-info people's images, so a person both name gets
// the lookup's face.
func (f *workInfoFetch) appliedFaces(
	lookups []domain.PersonLookupSelection, applied domain.FetchedWorkInfoApplied,
) []personFace {
	var faces []personFace
	for i, selection := range lookups {
		if i < len(applied.LookupPersonIDs) && slices.Contains(selection.Items, domain.PersonItemImage) {
			faces = append(faces, personFace{personID: applied.LookupPersonIDs[i], image: f.lookupRuns[selection.Index].image})
		}
	}
	for i, id := range applied.PersonIDs {
		if i < len(f.details.People) {
			faces = append(faces, personFace{personID: id, image: f.details.People[i].Image})
		}
	}
	return faces
}

// applyPersonImages runs after the apply commits, in faces order, and returns video with the revisions of the faces
// it stored; a failure drops that image without failing the apply, which is already written.
func (p *WorkInfoPlugins) applyPersonImages(ctx context.Context, video domain.Video, faces []personFace) domain.Video {
	if p.personImages == nil {
		return video
	}
	revisions := map[int64]int64{}
	for _, face := range faces {
		if face.personID == 0 || face.image == nil {
			continue
		}
		if revision, err := p.personImages.FillPersonImage(ctx, face.personID, face.image.Data); err == nil && revision != 0 {
			revisions[face.personID] = revision
		}
	}
	return withPersonImageRevisions(video, revisions)
}

// withPersonImageRevisions copies video's work information with the new face revisions of the people in revisions.
func withPersonImageRevisions(video domain.Video, revisions map[int64]int64) domain.Video {
	if len(revisions) == 0 || video.WorkInfo == nil {
		return video
	}
	info := *video.WorkInfo
	info.People = slices.Clone(info.People)
	for i, person := range info.People {
		if revision, ok := revisions[person.Person.ID]; ok {
			info.People[i].ImageRevision = revision
		}
	}
	video.WorkInfo = &info
	return video
}

// FillPersonImage fits data only when the person has no face, and stores it only if none was set meanwhile.
func (p *PersonImages) FillPersonImage(ctx context.Context, id int64, data []byte) (int64, error) {
	_, err := p.store.PersonImage(ctx, id)
	switch {
	case errors.Is(err, domain.ErrPersonImageNotFound):
	case err == nil, errors.Is(err, domain.ErrNotFound):
		return 0, nil
	default:
		return 0, err
	}
	fitted, err := p.fit(ctx, data)
	switch {
	case ctx.Err() != nil:
		return 0, ctx.Err()
	case err != nil:
		return 0, nil
	}
	revision, err := p.store.AddPersonImage(ctx, id, fitted)
	if errors.Is(err, domain.ErrNotFound) {
		return 0, nil
	}
	return revision, err
}
