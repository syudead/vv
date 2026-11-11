package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type fakeImageSearchIndex struct {
	snapshot  domain.ImageSearchSnapshot
	audiences []domain.Audience
	loaded    []int64
	locations map[int64][]string
}

func (f *fakeImageSearchIndex) VideoLocations(_ context.Context, videoID int64) ([]domain.VideoLocation, error) {
	var locations []domain.VideoLocation
	for _, path := range f.locations[videoID] {
		locations = append(locations, domain.VideoLocation{VideoID: videoID, Path: path})
	}
	return locations, nil
}

func (f *fakeImageSearchIndex) ImageSearchSnapshot(_ context.Context, audience domain.Audience) (domain.ImageSearchSnapshot, error) {
	f.audiences = append(f.audiences, audience)
	return f.snapshot, nil
}

func (f *fakeImageSearchIndex) VideosByIDs(_ context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	f.audiences = append(f.audiences, audience)
	f.loaded = ids
	videos := make([]domain.Video, len(ids))
	for i, id := range ids {
		videos[i] = domain.Video{ID: id}
	}
	return videos, nil
}

// dirUploads keeps uploads in a test directory, so a test sees what is left behind.
type dirUploads struct{ dir string }

func (u dirUploads) Create() (*os.File, error) { return os.CreateTemp(u.dir, "query-*") }
func (u dirUploads) Remove(path string) error  { return os.Remove(path) }

func (u dirUploads) left(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(u.dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

const queryHash = 0x0123_4567_89ab_cdef

func searchFrame(timeMs int64, distance int) domain.ImageSearchFrame {
	return domain.ImageSearchFrame{TimeMs: timeMs, Hash: queryHash ^ (1<<distance - 1)}
}

func candidate(id, listed int64, frames ...domain.ImageSearchFrame) domain.ImageSearchCandidate {
	return domain.ImageSearchCandidate{ListedID: listed, Content: domain.ImageSearchContent{
		VideoID: id, AddedAt: time.Unix(id, 0), DurationMs: 60_000, Frames: frames,
	}}
}

func noOnlyFileCheck() error { return nil }

// A bundle's member and representative collapse to the listed representative, keeping the closer member's match;
// the owner's audience reaches both reads and the unindexed count comes back.
func TestImageSearchCollapsesBundleMembersToTheListedVideo(t *testing.T) {
	t.Parallel()
	index := &fakeImageSearchIndex{snapshot: domain.ImageSearchSnapshot{
		UnindexedCount: 3,
		Candidates: []domain.ImageSearchCandidate{
			candidate(12, 12, searchFrame(0, 10)),
			candidate(13, 12, searchFrame(0, 30), searchFrame(4000, 0)),
			candidate(20, 20, searchFrame(2000, 5)),
		},
	}}
	search := NewImageSearch(ImageSearchOptions{Index: index, Media: ImageSearchMedia{
		HashImage: func(image.Image) domain.FrameHash { return domain.FrameHash{Hash: queryHash} },
	}})
	audience := domain.AudienceOwner.WithExcludedFolders([]int64{4})

	result, err := search.Search(context.Background(), audience, bytes.NewReader(testPNG(t, 8, 6)), noOnlyFileCheck)
	if err != nil {
		t.Fatal(err)
	}
	type listed struct{ id, matched, positionMs int64 }
	var got []listed
	for _, item := range result.Items {
		got = append(got, listed{item.Video.ID, item.Match.VideoID, item.Match.PositionMs})
	}
	if want := []listed{{12, 13, 4000}, {20, 20, 2000}}; !slices.Equal(got, want) {
		t.Fatalf("items = %+v, want %+v", got, want)
	}
	if result.Kind != domain.ImageSearchQueryImage || result.UnindexedCount != 3 {
		t.Fatalf("result = %+v", result)
	}
	if len(index.audiences) != 2 || index.audiences[0] != audience || index.audiences[1] != audience {
		t.Fatalf("audiences = %v, want the request's for both reads", index.audiences)
	}
}

// A second search waits while one runs, and stops waiting when cancelled; each removes its upload whether it ends by
// cancellation or by result.
func TestImageSearchRemovesTheUploadWhenCancelled(t *testing.T) {
	t.Parallel()
	uploads := dirUploads{dir: t.TempDir()}
	decoding := make(chan struct{})
	search := NewImageSearch(ImageSearchOptions{
		Index: &fakeImageSearchIndex{}, Uploads: uploads,
		Media: ImageSearchMedia{
			Duration: func(context.Context, string) (int64, error) { return 60_000, nil },
			Frames: func(ctx context.Context, _ string, _ int) ([]domain.FrameHash, error) {
				close(decoding)
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := search.Search(ctx, domain.AudienceOwner, bytes.NewReader([]byte("video")), noOnlyFileCheck)
		first <- err
	}()
	<-decoding

	waiting, stopWaiting := context.WithCancel(context.Background())
	stopWaiting()
	if _, err := search.Search(waiting, domain.AudienceOwner, bytes.NewReader([]byte("video")), noOnlyFileCheck); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting search = %v, want context.Canceled", err)
	}
	if n := uploads.left(t); n != 1 {
		t.Fatalf("uploads while the first search decodes = %d, want only its own", n)
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first search = %v, want context.Canceled", err)
	}
	if n := uploads.left(t); n != 0 {
		t.Fatalf("uploads after both searches = %d, want none", n)
	}
}

// A search cancelled once its video is hashed stops matching and returns the cancellation instead of a result.
func TestImageSearchStopsMatchingWhenCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	index := &fakeImageSearchIndex{snapshot: domain.ImageSearchSnapshot{Candidates: []domain.ImageSearchCandidate{
		candidate(1, 1, searchFrame(0, 0), searchFrame(1000, 0), searchFrame(2000, 0)),
	}}}
	search := NewImageSearch(ImageSearchOptions{
		Index: index, Uploads: dirUploads{dir: t.TempDir()},
		Media: ImageSearchMedia{
			Duration: func(context.Context, string) (int64, error) { return 3000, nil },
			Frames: func(context.Context, string, int) ([]domain.FrameHash, error) {
				cancel()
				return []domain.FrameHash{{Hash: queryHash}, {Hash: queryHash}, {Hash: queryHash}}, nil
			},
		},
	})
	if _, err := search.Search(ctx, domain.AudienceOwner, bytes.NewReader([]byte("video")), noOnlyFileCheck); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if index.loaded != nil {
		t.Fatalf("loaded %v after cancellation", index.loaded)
	}
}

// Stopping the server ends a running search, whose request context shutdown leaves alone, and removes its upload.
func TestImageSearchEndsWhenTheServerStops(t *testing.T) {
	t.Parallel()
	uploads := dirUploads{dir: t.TempDir()}
	serverDone := make(chan struct{})
	decoding := make(chan struct{})
	search := NewImageSearch(ImageSearchOptions{
		Index: &fakeImageSearchIndex{}, Uploads: uploads, ServerDone: serverDone,
		Media: ImageSearchMedia{
			Duration: func(context.Context, string) (int64, error) { return 60_000, nil },
			Frames: func(ctx context.Context, _ string, _ int) ([]domain.FrameHash, error) {
				close(decoding)
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
	})
	ended := make(chan error, 1)
	go func() {
		_, err := search.Search(context.Background(), domain.AudienceOwner, bytes.NewReader([]byte("video")), noOnlyFileCheck)
		ended <- err
	}()
	<-decoding
	close(serverDone)
	select {
	case err := <-ended:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the search did not end when the server stopped")
	}
	if n := uploads.left(t); n != 0 {
		t.Fatalf("%d uploads left", n)
	}
}

// A file that holds more than the request allows, or too little to search by, is refused before matching.
func TestImageSearchRefusesQueriesItCannotMatch(t *testing.T) {
	t.Parallel()
	frames := func(flat ...bool) func(context.Context, string, int) ([]domain.FrameHash, error) {
		return func(context.Context, string, int) ([]domain.FrameHash, error) {
			out := make([]domain.FrameHash, len(flat))
			for i, f := range flat {
				out[i] = domain.FrameHash{Hash: uint64(i), Flat: f}
			}
			return out, nil
		}
	}
	duration := func(ms int64) func(context.Context, string) (int64, error) {
		return func(context.Context, string) (int64, error) { return ms, nil }
	}
	secondPart := errors.New("a second part")
	cases := []struct {
		name     string
		media    ImageSearchMedia
		file     []byte
		onlyFile func() error
		want     error
	}{
		{"flat image", ImageSearchMedia{HashImage: func(image.Image) domain.FrameHash { return domain.FrameHash{Flat: true} }},
			testPNG(t, 8, 6), noOnlyFileCheck, domain.ErrImageSearchUninformative},
		{"two non-flat frames", ImageSearchMedia{Duration: duration(60_000), Frames: frames(false, true, false)},
			[]byte("video"), noOnlyFileCheck, domain.ErrImageSearchUninformative},
		{"over three hours", ImageSearchMedia{Duration: duration(domain.ImageSearchVideoMaxMs + 1)},
			[]byte("video"), noOnlyFileCheck, domain.ErrImageSearchTooLong},
		{"under a second", ImageSearchMedia{Duration: duration(999)}, []byte("video"), noOnlyFileCheck,
			domain.ErrImageSearchUnreadable},
		{"second part", ImageSearchMedia{}, []byte("video"), func() error { return secondPart }, secondPart},
	}
	for _, tc := range cases {
		uploads := dirUploads{dir: t.TempDir()}
		search := NewImageSearch(ImageSearchOptions{Index: &fakeImageSearchIndex{}, Uploads: uploads, Media: tc.media})
		if _, err := search.Search(context.Background(), domain.AudienceOwner, bytes.NewReader(tc.file), tc.onlyFile); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if n := uploads.left(t); n != 0 {
			t.Errorf("%s: %d uploads left", tc.name, n)
		}
	}
}

// Of twelve near misses the ten closest are refined; one of them has no file the media folders allow, so it is left
// out without failing the search, one is refined from its second location after the first fails to decode, and the
// others are listed at their refined frame.
func TestImageSearchRefinesTheTenClosestNearMisses(t *testing.T) {
	t.Parallel()
	index := &fakeImageSearchIndex{locations: map[int64][]string{
		1: {"/elsewhere/1.mp4"}, 2: {"/media/broken/2.mp4", "/media/2.mp4"},
	}}
	for i, distance := range []int{13, 13, 14, 14, 15, 15, 16, 16, 17, 17, 18, 18} {
		id := int64(i + 1)
		index.snapshot.Candidates = append(index.snapshot.Candidates,
			candidate(id, id, searchFrame(0, distance), searchFrame(8000, 40)))
		if id > 2 {
			index.locations[id] = []string{fmt.Sprintf("/media/%d.mp4", id)}
		}
	}
	type decode struct {
		path       string
		start, end int64
	}
	var decoded []decode
	search := NewImageSearch(ImageSearchOptions{
		Index: index, Files: MediaFiles{Folders: &fakeMediaFolders{}, Resolver: fakeMediaFiles{}},
		Logger: slog.New(slog.DiscardHandler),
		Media: ImageSearchMedia{
			HashImage: func(image.Image) domain.FrameHash { return domain.FrameHash{Hash: queryHash} },
			Refine: func(_ context.Context, path string, startMs, endMs int64) ([]domain.KeyframeHash, error) {
				decoded = append(decoded, decode{path, startMs, endMs})
				if path == "resolved:/media/broken/2.mp4" {
					return nil, errors.New("invalid data found when processing input")
				}
				return []domain.KeyframeHash{
					{TimeMs: 0, Frame: domain.FrameHash{Hash: searchFrame(0, 20).Hash}},
					{TimeMs: 2750, Frame: domain.FrameHash{Hash: searchFrame(0, 3).Hash}},
				}, nil
			},
		},
	})

	result, err := search.Search(context.Background(), domain.AudienceOwner, bytes.NewReader(testPNG(t, 8, 6)), noOnlyFileCheck)
	if err != nil {
		t.Fatal(err)
	}
	want := []decode{{"resolved:/media/broken/2.mp4", 0, 8000}}
	for id := 2; id <= 10; id++ {
		want = append(want, decode{fmt.Sprintf("resolved:/media/%d.mp4", id), 0, 8000})
	}
	if !slices.Equal(decoded, want) {
		t.Fatalf("decoded %+v, want %+v", decoded, want)
	}
	var listed []int64
	for _, item := range result.Items {
		listed = append(listed, item.Match.VideoID)
		if item.Match.PositionMs != 2750 || item.Match.Rate != domain.ImageMatchRate(3) {
			t.Errorf("video %d matched at %d ms, rate %v, want 2750 ms at the refined frame's rate",
				item.Match.VideoID, item.Match.PositionMs, item.Match.Rate)
		}
	}
	slices.Sort(listed)
	if wantListed := []int64{2, 3, 4, 5, 6, 7, 8, 9, 10}; !slices.Equal(listed, wantListed) {
		t.Fatalf("listed %v, want %v", listed, wantListed)
	}
}

// A media folder list that cannot be read fails the search instead of leaving the near miss out as a missing file.
func TestImageSearchFailsWhenTheMediaFoldersCannotBeRead(t *testing.T) {
	t.Parallel()
	index := &fakeImageSearchIndex{locations: map[int64][]string{1: {"/media/1.mp4"}}}
	index.snapshot.Candidates = []domain.ImageSearchCandidate{candidate(1, 1, searchFrame(0, 13))}
	storeErr := errors.New("database is locked")
	refined := false
	search := NewImageSearch(ImageSearchOptions{
		Index: index, Files: MediaFiles{Folders: &fakeMediaFolders{err: storeErr}, Resolver: fakeMediaFiles{}},
		Logger: slog.New(slog.DiscardHandler),
		Media: ImageSearchMedia{
			HashImage: func(image.Image) domain.FrameHash { return domain.FrameHash{Hash: queryHash} },
			Refine: func(context.Context, string, int64, int64) ([]domain.KeyframeHash, error) {
				refined = true
				return nil, nil
			},
		},
	})

	_, err := search.Search(context.Background(), domain.AudienceOwner, bytes.NewReader(testPNG(t, 8, 6)), noOnlyFileCheck)
	if !errors.Is(err, storeErr) {
		t.Fatalf("Search error = %v, want %v", err, storeErr)
	}
	if refined {
		t.Fatal("refined a file the media folders were not read for")
	}
}
