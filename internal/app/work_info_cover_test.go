package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type fakeCovers struct {
	images [][]byte
	err    error
}

func (c *fakeCovers) UploadThumbnailImage(_ context.Context, videoID int64, _ string, image []byte) (domain.Video, error) {
	if c.err != nil {
		return domain.Video{}, c.err
	}
	c.images = append(c.images, image)
	return domain.Video{ID: videoID}, nil
}

func pngOf(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newCoverEnv() (*fetchEnv, *fakeCovers) {
	e := newFetchEnv()
	covers := &fakeCovers{}
	e.plugins.covers = covers
	return e, covers
}

// fetchCover fetches a result whose cover i comes from coverURL(i) with covers[i] as its bytes.
func (e *fetchEnv) fetchCover(t *testing.T, covers ...[]byte) domain.WorkInfoFetchResult {
	t.Helper()
	urls := make([]string, len(covers))
	for i := range urls {
		urls[i] = coverURL(i)
	}
	e.runtime.result = domain.WorkInfoRunResult{
		WorkInfo: domain.FetchedWorkInfo{Title: "Title", CoverURLs: urls}, CoverImages: covers,
	}
	result, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "W-1")
	if err != nil {
		t.Fatalf("FetchWorkInfo: %v", err)
	}
	return result
}

func coverURL(i int) string { return "https://example.com/cover" + strconv.Itoa(i) + ".png" }

// Unreadable covers are dropped and the rest keep their order and indexes.
func TestFetchedCoversKeepOnlyImagesItCanRead(t *testing.T) {
	e, _ := newCoverEnv()
	var animated bytes.Buffer
	if err := gif.Encode(&animated, image.NewPaletted(image.Rect(0, 0, 2, 2), []color.Color{color.Black}), nil); err != nil {
		t.Fatal(err)
	}
	small, large := pngOf(t, 3, 2), pngOf(t, 5, 4)
	// Reject GIF even if decodable, matching upload format restrictions.
	result := e.fetchCover(t, nil, []byte("<svg/>"), small, animated.Bytes(),
		make([]byte, domain.WorkInfoCoverMaxBytes+1), large)

	if len(result.Covers) != 2 || result.Covers[0].ContentType != "image/png" || result.Covers[0].Width != 3 ||
		result.Covers[1].Width != 5 || result.Covers[1].Height != 4 {
		t.Fatalf("covers = %+v; want the 3x2 and 5x4 PNGs", result.Covers)
	}
	for index, want := range [][]byte{small, large} {
		got, err := e.plugins.WorkInfoFetchCover(fetchVideo, result.ID, index)
		if err != nil || !bytes.Equal(got.Data, want) {
			t.Fatalf("WorkInfoFetchCover(%d) = %d bytes, %v; want the fetched image", index, len(got.Data), err)
		}
	}
	for _, index := range []int{-1, 2} {
		if _, err := e.plugins.WorkInfoFetchCover(fetchVideo, result.ID, index); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("WorkInfoFetchCover(%d): err = %v; want ErrNotFound", index, err)
		}
	}
	if _, err := e.plugins.WorkInfoFetchCover(otherVideo, result.ID, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another video's cover: err = %v", err)
	}
}

func TestApplyCoverMakesItTheThumbnailAfterTheWorkInfo(t *testing.T) {
	e, covers := newCoverEnv()
	image := pngOf(t, 4, 3)
	result := e.fetchCover(t, pngOf(t, 2, 2), image)
	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, domain.WorkInfoSelection{
		Fields: []domain.WorkInfoField{domain.WorkInfoFieldCover, domain.WorkInfoFieldTitle}, CoverIndex: 1,
	}, false)
	if err != nil || len(applied.Skipped) != 0 {
		t.Fatalf("apply = %+v, %v", applied, err)
	}
	if len(covers.images) != 1 || !bytes.Equal(covers.images[0], image) || e.applier.titles[fetchVideo] != "Title" {
		t.Fatalf("covers %d, title %q; want the cover applied and the title written", len(covers.images), e.applier.titles[fetchVideo])
	}
	if len(e.applier.calls[0].DetailFields) != 0 {
		t.Fatalf("detail fields = %v; the cover is not a work detail", e.applier.calls[0].DetailFields)
	}
}

// Do not apply covers when the transaction rejects changed plugin state.
func TestApplyCoverIsNotAppliedWhenTheWorkInfoIsRefused(t *testing.T) {
	for _, failure := range []error{domain.ErrWorkInfoFetchStale, errors.New("database is locked")} {
		e, covers := newCoverEnv()
		result := e.fetchCover(t, pngOf(t, 2, 2))
		e.applier.err = failure
		if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
			domain.WorkInfoSelection{Fields: []domain.WorkInfoField{domain.WorkInfoFieldCover}}, false); !errors.Is(err, failure) || len(covers.images) != 0 {
			t.Fatalf("%v: err = %v, %d covers applied; want the failure and no cover", failure, err, len(covers.images))
		}
	}
}

// Report cover failures as skipped values after metadata commits.
func TestApplyCoverThatCannotBeUsedIsSkippedAfterTheRestIsApplied(t *testing.T) {
	for _, failure := range []error{domain.ErrContentChanged, domain.ErrThumbnailUploadInvalid, errors.New("disk full")} {
		e, covers := newCoverEnv()
		covers.err = failure
		// The unreadable first cover is dropped, so cover 0 is the second URL's.
		result := e.fetchCover(t, nil, pngOf(t, 2, 2))
		applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
			domain.WorkInfoSelection{Fields: []domain.WorkInfoField{domain.WorkInfoFieldCover, domain.WorkInfoFieldTitle}}, false)
		want := domain.SkippedWorkInfoValue{
			Field: domain.WorkInfoFieldCover, Value: coverURL(1), Reason: domain.SkippedWorkInfoInvalid,
		}
		if err != nil || len(applied.Skipped) != 1 || applied.Skipped[0] != want || e.applier.titles[fetchVideo] != "Title" {
			t.Fatalf("%v: apply = %+v, %v; want the cover skipped and the title written", failure, applied, err)
		}
	}
}

// A page that frames the cover itself still needs the result checked and consumed, but no thumbnail written.
func TestApplyCoverByPageChecksTheResultWithoutApplyingTheCover(t *testing.T) {
	e, covers := newCoverEnv()
	result := e.fetchCover(t, pngOf(t, 2, 2))
	cover := []domain.WorkInfoField{domain.WorkInfoFieldCover}
	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, domain.WorkInfoSelection{Fields: cover}, true)
	if err != nil || len(applied.Skipped) != 0 || len(covers.images) != 0 || len(e.applier.calls) != 1 {
		t.Fatalf("apply = %+v, %v, %d covers, %d commits; want one commit and no cover", applied, err, len(covers.images), len(e.applier.calls))
	}
	if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, domain.WorkInfoSelection{Fields: cover}, true); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("second apply: err = %v; want the result consumed", err)
	}
	stale := e.fetchCover(t, pngOf(t, 2, 2))
	e.applier.err = domain.ErrWorkInfoFetchStale
	if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, stale.ID, domain.WorkInfoSelection{Fields: cover}, true); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("revoked plugin: err = %v; want ErrWorkInfoFetchStale", err)
	}
}

func TestApplyCoverIsRefusedWithoutACoverAtTheIndex(t *testing.T) {
	cover := []domain.WorkInfoField{domain.WorkInfoFieldCover}
	for _, tc := range []struct {
		name   string
		covers [][]byte
		index  int
	}{
		{"no cover", [][]byte{nil}, 0},
		{"past the last cover", [][]byte{pngOf(t, 2, 2)}, 1},
		{"a negative index", [][]byte{pngOf(t, 2, 2)}, -1},
	} {
		e, _ := newCoverEnv()
		result := e.fetchCover(t, tc.covers...)
		_, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
			domain.WorkInfoSelection{Fields: cover, CoverIndex: tc.index}, false)
		if !errors.Is(err, domain.ErrWorkInfoApplyInvalid) || len(e.applier.calls) != 0 {
			t.Fatalf("%s: err = %v, %d commits; want ErrWorkInfoApplyInvalid and none", tc.name, err, len(e.applier.calls))
		}
	}
}

// Evict the oldest cover-bearing results when the memory budget is exceeded.
func TestFetchedCoversStayWithinTheBudget(t *testing.T) {
	fetches := newWorkInfoFetches()
	e := newFetchEnv()
	cover := domain.WorkInfoCover{Data: make([]byte, domain.WorkInfoCoverMaxBytes)}
	var ids []string
	for i := range domain.WorkInfoFetchCoverBudget/domain.WorkInfoCoverMaxBytes + 1 {
		fetch := &workInfoFetch{videoID: fetchVideo, covers: []domain.WorkInfoCover{cover}, expiresAt: e.now.Add(domain.WorkInfoFetchLifetime).Add(time.Duration(i))}
		ids = append(ids, fetches.add(fetch, e.now))
	}
	plain := fetches.add(&workInfoFetch{videoID: fetchVideo, expiresAt: e.now}, e.now.Add(-time.Second))
	if _, err := fetches.cover(ids[0], fetchVideo, 0, e.now.Add(-time.Second)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("the oldest cover was kept: %v", err)
	}
	if _, err := fetches.cover(ids[len(ids)-1], fetchVideo, 0, e.now.Add(-time.Second)); err != nil {
		t.Fatalf("the newest cover was dropped: %v", err)
	}
	if _, ok := fetches.entries[plain]; !ok {
		t.Fatal("a result without a cover was dropped")
	}
}

// Report a skipped cover if the video or applier becomes unavailable after metadata commits.
func TestApplyCoverIsSkippedWhenTheVideoOrTheApplierIsMissing(t *testing.T) {
	for name, change := range map[string]func(e *fetchEnv){
		"video gone": func(e *fetchEnv) { delete(e.plugins.videos.(fakeVideos), fetchVideo) },
		"no applier": func(e *fetchEnv) { e.plugins.covers = nil },
	} {
		e, covers := newCoverEnv()
		e.plugins.videos = fakeVideos{fetchVideo: true}
		result := e.fetchCover(t, pngOf(t, 2, 2))
		change(e)
		applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
			domain.WorkInfoSelection{Fields: []domain.WorkInfoField{domain.WorkInfoFieldCover, domain.WorkInfoFieldTitle}}, false)
		if err != nil || len(applied.Skipped) != 1 || applied.Skipped[0].Field != domain.WorkInfoFieldCover ||
			len(covers.images) != 0 || e.applier.titles[fetchVideo] != "Title" {
			t.Fatalf("%s: apply = %+v, %v; want the cover skipped and the title written", name, applied, err)
		}
	}
}

type virtualWorkVideo domain.Video

func (v virtualWorkVideo) GetVideo(_ context.Context, _ domain.Audience, id int64) (domain.Video, error) {
	if id != v.ID {
		return domain.Video{}, domain.ErrNotFound
	}
	return domain.Video(v), nil
}

// A virtual video has no geometry, so its chosen cover goes through the upload path fitted to 16:9 under its key.
func TestApplyCoverToVirtualVideoFitsItTo16By9UnderItsKey(t *testing.T) {
	video := domain.Video{ID: fetchVideo, ContentKey: "virtual:3", UserKey: "virtual:3"}
	generator := &fakeGenerator{thumbnailDir: t.TempDir()}
	store := newFakeIngestStore(video)
	ingest, _ := newTestIngest(store, generator)
	e := newFetchEnv()
	e.plugins.videos, e.plugins.covers = virtualWorkVideo(video), ingest
	cover := pngOf(t, 4, 3)
	result := e.fetchCover(t, cover)

	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
		domain.WorkInfoSelection{Fields: []domain.WorkInfoField{domain.WorkInfoFieldCover}, CoverIndex: 0}, false)
	if err != nil || len(applied.Skipped) != 0 {
		t.Fatalf("apply = %+v, %v; want the cover applied", applied, err)
	}
	// The store refuses the apply when the video under the ID no longer has the key the fetch saw.
	if len(e.applier.calls) != 1 || e.applier.calls[0].ContentKey != "virtual:3" {
		t.Fatalf("apply calls = %+v; want one bound to the fetched key virtual:3", e.applier.calls)
	}
	if want := []float64{domain.VirtualThumbnailAspect}; !slices.Equal(generator.aspects, want) {
		t.Fatalf("aspects = %v, want %v", generator.aspects, want)
	}
	if row := store.rows["virtual:3"]; !bytes.Equal(row.Image, append([]byte("fitted:"), cover...)) {
		t.Fatalf("image under the virtual key = %q; want the fitted cover", row.Image)
	}
}
