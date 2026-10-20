package app

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// faceStore keeps faces by person id; a person absent from known does not exist.
type faceStore struct {
	known map[int64]bool
	faces map[int64][]byte
	added []int64
}

func (s *faceStore) PersonImage(_ context.Context, id int64) (domain.PersonImage, error) {
	switch {
	case !s.known[id]:
		return domain.PersonImage{}, domain.ErrNotFound
	case s.faces[id] == nil:
		return domain.PersonImage{}, domain.ErrPersonImageNotFound
	}
	return domain.PersonImage{JPEG: s.faces[id]}, nil
}

// AddPersonImage stores a face at revision 5.
func (s *faceStore) AddPersonImage(_ context.Context, id int64, image domain.PersonImage) (int64, error) {
	if s.faces[id] != nil {
		return 0, nil
	}
	s.faces[id] = image.JPEG
	s.added = append(s.added, id)
	return 5, nil
}

// SetPersonImage stores a face over any other at revision 7.
func (s *faceStore) SetPersonImage(_ context.Context, id int64, image domain.PersonImage) (domain.Credit, error) {
	if !s.known[id] {
		return domain.Credit{}, domain.ErrNotFound
	}
	s.faces[id] = image.JPEG
	return domain.Credit{ID: id, ImageRevision: 7}, nil
}

func (s *faceStore) DeletePersonImage(context.Context, int64) (domain.Credit, error) {
	return domain.Credit{}, errors.New("not used")
}

func fetchPeople(t *testing.T, e *fetchEnv, images ...[]byte) domain.WorkInfoFetchResult {
	t.Helper()
	people := make([]domain.FetchedPerson, len(images))
	for i := range people {
		people[i] = domain.FetchedPerson{Role: domain.PersonPerformer, Name: string(rune('A' + i)), ImageURL: "https://e.test/face"}
	}
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: domain.FetchedWorkInfo{People: people}, PersonImages: images}
	result, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "W-1")
	if err != nil {
		t.Fatalf("FetchWorkInfo: %v", err)
	}
	return result
}

func TestFetchedPersonImageIsServedByIndex(t *testing.T) {
	e := newFetchEnv()
	face := pngOf(t, 3, 4)
	result := fetchPeople(t, e, nil, face, []byte("not an image"))

	if result.Details.People[0].Image != nil || result.Details.People[2].Image != nil {
		t.Fatalf("people = %+v; want an image for the second person only", result.Details.People)
	}
	got, err := e.plugins.WorkInfoFetchPersonImage(fetchVideo, result.ID, 1)
	if err != nil || !bytes.Equal(got.Data, face) || got.ContentType != "image/png" {
		t.Fatalf("WorkInfoFetchPersonImage(1) = %d bytes %q, %v; want the PNG", len(got.Data), got.ContentType, err)
	}
	for _, index := range []int{0, 2, 3, -1} {
		if _, err := e.plugins.WorkInfoFetchPersonImage(fetchVideo, result.ID, index); !errors.Is(err, domain.ErrPersonImageNotFound) {
			t.Errorf("WorkInfoFetchPersonImage(%d): err = %v, want ErrPersonImageNotFound", index, err)
		}
	}
	if _, err := e.plugins.WorkInfoFetchPersonImage(otherVideo, result.ID, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("another video's image: err = %v, want ErrNotFound", err)
	}
}

func TestApplyStoresAFetchedFaceOnlyForAPersonWithoutOne(t *testing.T) {
	e := newFetchEnv()
	faces := &faceStore{known: map[int64]bool{1: true, 2: true, 3: true}, faces: map[int64][]byte{1: []byte("own face")}}
	e.plugins.personImages = &PersonImages{store: faces, uploads: newUploadGate()}
	// Person 3's image passes the header check but cannot be decoded.
	broken := pngOf(t, 2, 2)[:40]
	result := fetchPeople(t, e, pngOf(t, 2, 2), pngOf(t, 5, 4), broken, pngOf(t, 2, 2))
	e.applier.personIDs = []int64{1, 2, 3, 0}
	performer := func(id, revision int64) domain.WorkPerson {
		return domain.WorkPerson{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: id}, ImageRevision: revision}
	}
	e.applier.workInfo = &domain.WorkInfo{People: []domain.WorkPerson{performer(1, 2), performer(2, 0), performer(3, 0)}}

	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
		[]domain.WorkInfoField{domain.PersonField(domain.PersonPerformer)}, nil, false, nil)
	if err != nil || len(applied.Skipped) != 0 {
		t.Fatalf("apply = %+v, %v; want success with nothing skipped", applied, err)
	}
	if len(faces.added) != 1 || faces.added[0] != 2 || !bytes.Equal(faces.faces[1], []byte("own face")) {
		t.Fatalf("added faces for %v, person 1 has %q; want person 2 alone", faces.added, faces.faces[1])
	}
	want := []domain.WorkPerson{performer(1, 2), performer(2, 5), performer(3, 0)}
	if applied.Video.WorkInfo == nil || !slices.Equal(applied.Video.WorkInfo.People, want) {
		t.Errorf("applied people = %+v; want person 2 at the stored face's revision", applied.Video.WorkInfo)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(faces.faces[2]))
	if err != nil || config.Width != 5 || config.Height != 4 {
		t.Fatalf("person 2's face = %+v, %v; want the 5x4 image as JPEG", config, err)
	}
}

func TestReplacePersonImageStoresOverAFace(t *testing.T) {
	faces := &faceStore{known: map[int64]bool{1: true}, faces: map[int64][]byte{1: []byte("own face")}}
	images := &PersonImages{store: faces, uploads: newUploadGate()}

	revision, err := images.ReplacePersonImage(context.Background(), 1, pngOf(t, 5, 4))
	if err != nil || revision != 7 {
		t.Fatalf("ReplacePersonImage = %d, %v; want revision 7", revision, err)
	}
	if config, err := jpeg.DecodeConfig(bytes.NewReader(faces.faces[1])); err != nil || config.Width != 5 {
		t.Fatalf("person 1's face = %+v, %v; want the fetched image over the stored face", config, err)
	}
	if revision, err := images.ReplacePersonImage(context.Background(), 2, pngOf(t, 2, 2)); err != nil || revision != 0 {
		t.Fatalf("missing person: ReplacePersonImage = %d, %v; want 0 without an error", revision, err)
	}
}

// One fetch holding a cover, seven people's images and five lookup images at the size limit fills the budget, and a
// person page result added next evicts it.
func TestFetchedImagesCountTowardTheBudgetWithTheCoverAndLookups(t *testing.T) {
	fetches := newWorkInfoFetches()
	now := time.Unix(1_800_000_000, 0)
	full := &domain.WorkInfoCover{Data: make([]byte, domain.WorkInfoCoverMaxBytes)}
	people := make([]domain.FetchedPerson, domain.WorkInfoPersonImagesMaxCount)
	for i := range people {
		people[i].Image = full
	}
	lookups := make([]*keptLookup, domain.PersonLookupsMaxCount)
	for i := range lookups {
		lookups[i] = &keptLookup{image: full}
	}
	first := fetches.add(&workInfoFetch{
		videoID: fetchVideo, cover: full, details: domain.FetchedWorkInfo{People: people}, lookupRuns: lookups,
		expiresAt: now.Add(domain.WorkInfoFetchLifetime),
	}, now)
	if _, ok := fetches.entries[first]; !ok {
		t.Fatal("a fetch within the budget was dropped")
	}
	fetches.addPerson(&personPluginFetch{images: []domain.WorkInfoCover{*full}, expiresAt: now.Add(domain.WorkInfoFetchLifetime + 1)}, now)
	if _, ok := fetches.entries[first]; ok {
		t.Fatal("the fetch that expires first was kept over the budget")
	}
}
