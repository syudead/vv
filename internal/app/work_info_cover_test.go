package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
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

func (e *fetchEnv) fetchCover(t *testing.T, cover []byte) domain.WorkInfoFetchResult {
	t.Helper()
	e.runtime.result = domain.WorkInfoRunResult{
		WorkInfo:   domain.FetchedWorkInfo{Title: "Title", CoverURL: "https://example.com/cover.png"},
		CoverImage: cover,
	}
	result, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "W-1")
	if err != nil {
		t.Fatalf("FetchWorkInfo: %v", err)
	}
	return result
}

func TestFetchedCoverKeepsOnlyImagesItCanRead(t *testing.T) {
	e, _ := newCoverEnv()
	want := pngOf(t, 3, 2)
	result := e.fetchCover(t, want)
	if result.Cover == nil || result.Cover.ContentType != "image/png" || result.Cover.Width != 3 || result.Cover.Height != 2 {
		t.Fatalf("cover = %+v; want a 3x2 PNG", result.Cover)
	}
	got, err := e.plugins.WorkInfoFetchCover(fetchVideo, result.ID)
	if err != nil || !bytes.Equal(got.Data, want) {
		t.Fatalf("WorkInfoFetchCover = %d bytes, %v; want the fetched image", len(got.Data), err)
	}
	if _, err := e.plugins.WorkInfoFetchCover(otherVideo, result.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another video's cover: err = %v", err)
	}
	var animated bytes.Buffer
	if err := gif.Encode(&animated, image.NewPaletted(image.Rect(0, 0, 2, 2), []color.Color{color.Black}), nil); err != nil {
		t.Fatal(err)
	}
	// Reject GIF even if decodable, matching upload format restrictions.
	for _, data := range [][]byte{nil, []byte("<svg/>"), animated.Bytes(), make([]byte, domain.WorkInfoCoverMaxBytes+1)} {
		if r := e.fetchCover(t, data); r.Cover != nil {
			t.Fatalf("cover of %d bytes = %+v; want none", len(data), r.Cover)
		}
		if _, err := e.plugins.WorkInfoFetchCover(fetchVideo, e.fetchCover(t, data).ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing cover: err = %v", err)
		}
	}
}

func TestApplyCoverMakesItTheThumbnailAfterTheWorkInfo(t *testing.T) {
	e, covers := newCoverEnv()
	image := pngOf(t, 4, 3)
	result := e.fetchCover(t, image)
	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
		[]domain.WorkInfoField{domain.WorkInfoFieldCover, domain.WorkInfoFieldTitle}, nil, false, nil)
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
			[]domain.WorkInfoField{domain.WorkInfoFieldCover}, nil, false, nil); !errors.Is(err, failure) || len(covers.images) != 0 {
			t.Fatalf("%v: err = %v, %d covers applied; want the failure and no cover", failure, err, len(covers.images))
		}
	}
}

// Report cover failures as skipped values after metadata commits.
func TestApplyCoverThatCannotBeUsedIsSkippedAfterTheRestIsApplied(t *testing.T) {
	for _, failure := range []error{domain.ErrContentChanged, domain.ErrThumbnailUploadInvalid, errors.New("disk full")} {
		e, covers := newCoverEnv()
		covers.err = failure
		result := e.fetchCover(t, pngOf(t, 2, 2))
		applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
			[]domain.WorkInfoField{domain.WorkInfoFieldCover, domain.WorkInfoFieldTitle}, nil, false, nil)
		want := domain.SkippedWorkInfoValue{
			Field: domain.WorkInfoFieldCover, Value: "https://example.com/cover.png", Reason: domain.SkippedWorkInfoInvalid,
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
	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, cover, nil, true, nil)
	if err != nil || len(applied.Skipped) != 0 || len(covers.images) != 0 || len(e.applier.calls) != 1 {
		t.Fatalf("apply = %+v, %v, %d covers, %d commits; want one commit and no cover", applied, err, len(covers.images), len(e.applier.calls))
	}
	if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, cover, nil, true, nil); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("second apply: err = %v; want the result consumed", err)
	}
	stale := e.fetchCover(t, pngOf(t, 2, 2))
	e.applier.err = domain.ErrWorkInfoFetchStale
	if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, stale.ID, cover, nil, true, nil); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("revoked plugin: err = %v; want ErrWorkInfoFetchStale", err)
	}
}

func TestApplyCoverIsRefusedWithoutACover(t *testing.T) {
	e, _ := newCoverEnv()
	result := e.fetchCover(t, nil)
	if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
		[]domain.WorkInfoField{domain.WorkInfoFieldCover}, nil, false, nil); !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
		t.Fatalf("err = %v; want ErrWorkInfoApplyInvalid", err)
	}
}

// Evict the oldest cover-bearing results when the memory budget is exceeded.
func TestFetchedCoversStayWithinTheBudget(t *testing.T) {
	fetches := newWorkInfoFetches()
	e := newFetchEnv()
	cover := &domain.WorkInfoCover{Data: make([]byte, domain.WorkInfoCoverMaxBytes)}
	var ids []string
	for i := range domain.WorkInfoFetchCoverBudget/domain.WorkInfoCoverMaxBytes + 1 {
		fetch := &workInfoFetch{videoID: fetchVideo, cover: cover, expiresAt: e.now.Add(domain.WorkInfoFetchLifetime).Add(time.Duration(i))}
		ids = append(ids, fetches.add(fetch, e.now))
	}
	plain := fetches.add(&workInfoFetch{videoID: fetchVideo, expiresAt: e.now}, e.now.Add(-time.Second))
	if _, err := fetches.cover(ids[0], fetchVideo, e.now.Add(-time.Second)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("the oldest cover was kept: %v", err)
	}
	if _, err := fetches.cover(ids[len(ids)-1], fetchVideo, e.now.Add(-time.Second)); err != nil {
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
			[]domain.WorkInfoField{domain.WorkInfoFieldCover, domain.WorkInfoFieldTitle}, nil, false, nil)
		if err != nil || len(applied.Skipped) != 1 || applied.Skipped[0].Field != domain.WorkInfoFieldCover ||
			len(covers.images) != 0 || e.applier.titles[fetchVideo] != "Title" {
			t.Fatalf("%s: apply = %+v, %v; want the cover skipped and the title written", name, applied, err)
		}
	}
}
