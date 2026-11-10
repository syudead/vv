package app

import (
	"bytes"
	"context"
	"errors"
	"image"
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
