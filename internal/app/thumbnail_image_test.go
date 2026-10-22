package app

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func sizedVideo(id int64, contentKey string, width, height int) domain.Video {
	video := probedVideo(id, contentKey)
	video.Width, video.Height = &width, &height
	return video
}

// Fit to the video aspect ratio, snapshot the old file before publishing, and persist the exact
// published bytes.
func TestApplyThumbnailImageFitsPublishesThenRecords(t *testing.T) {
	video := sizedVideo(1, "a", 1920, 1080)
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{recordPublish: true, thumbnailDir: t.TempDir()}
	store.order = generator
	ingest, _ := newTestIngest(store, generator)

	saved, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"), domain.ThumbnailImageGenerated)
	if err != nil {
		t.Fatal(err)
	}
	calls, removed := generator.snapshot()
	if want := []string{"fit", "fit-source", "publish-thumbnail", "apply-image"}; !slices.Equal(calls, want) {
		t.Fatalf("呼び出し = %v, want %v", calls, want)
	}
	if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "discard"}) {
		t.Fatalf("写しの操作 = %v", got)
	}
	if !slices.Equal(generator.aspects, []float64{1920.0 / 1080.0}) {
		t.Fatalf("縦横比 = %v", generator.aspects)
	}
	if !bytes.Equal(store.images["a"], []byte("fitted:edited")) || !bytes.Equal(generator.published["a"], store.images["a"]) {
		t.Fatalf("記録 = %q, 公開 = %q", store.images["a"], generator.published["a"])
	}
	if saved.ThumbnailImageRevision == 0 || saved.ThumbnailState != domain.ThumbnailStateDone ||
		saved.ThumbnailImageOrigin != domain.ThumbnailImageGenerated {
		t.Fatalf("返した動画 = %+v", saved)
	}
	// Persist the original and origin atomically with the thumbnail.
	row := store.rows["a"]
	if !bytes.Equal(row.Image, []byte("fitted:edited")) || !bytes.Equal(row.Source, []byte("source:edited")) ||
		row.Origin != domain.ThumbnailImageGenerated {
		t.Fatalf("記録した行 = %q / %q / %q", row.Image, row.Source, row.Origin)
	}
	if len(removed) != 0 {
		t.Fatalf("参照のある生成物を消した: %v", removed)
	}
}

func TestApplyThumbnailImageRecordsOrigin(t *testing.T) {
	video := sizedVideo(1, "a", 1920, 1080)
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)

	saved, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("photo"), domain.ThumbnailImageUploaded)
	if err != nil {
		t.Fatal(err)
	}
	if store.rows["a"].Origin != domain.ThumbnailImageUploaded || saved.ThumbnailImageOrigin != domain.ThumbnailImageUploaded {
		t.Fatalf("出どころ = %q / %q", store.rows["a"].Origin, saved.ThumbnailImageOrigin)
	}
}

// Unknown video dimensions use zero aspect ratio to retain the image ratio.
func TestApplyThumbnailImageUnknownSize(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)

	if _, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"), domain.ThumbnailImageGenerated); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(generator.aspects, []float64{0}) {
		t.Fatalf("縦横比 = %v", generator.aspects)
	}
}

// A virtual video has no geometry; its upload is fitted to the 16:9 card frame and kept under its user key.
func TestUploadThumbnailImageFitsVirtualVideoTo16By9(t *testing.T) {
	video := domain.Video{ID: 3, ContentKey: "virtual:3", UserKey: "virtual:3"}
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)
	image := uploadPNG(t)

	if _, err := ingest.UploadThumbnailImage(context.Background(), video.ID, "virtual:3", image); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(generator.aspects, []float64{16.0 / 9.0}) {
		t.Fatalf("縦横比 = %v, want 16:9", generator.aspects)
	}
	row := store.rows["virtual:3"]
	if !bytes.Equal(row.Image, append([]byte("fitted:"), image...)) || row.Origin != domain.ThumbnailImageUploaded {
		t.Fatalf("仮想キーの行 = %q / %q", row.Image, row.Origin)
	}
	if !bytes.Equal(generator.published["virtual:3"], row.Image) {
		t.Fatal("仮想キーで公開していない")
	}
}

// Reject changed content before fitting, publishing or recording.
func TestApplyThumbnailImageContentChanged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		videoID int64
		key     string
	}{
		{name: "どの動画も持たない内容", videoID: 1, key: "gone"},
		{name: "動画が無い", videoID: 9, key: "a"},
		{name: "空の内容", videoID: 1, key: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			generator := &fakeGenerator{recordPublish: true, thumbnailDir: t.TempDir()}
			ingest, _ := newTestIngest(store, generator)

			_, err := ingest.ApplyThumbnailImage(context.Background(), tc.videoID, tc.key, []byte("edited"), domain.ThumbnailImageGenerated)
			if !errors.Is(err, domain.ErrContentChanged) {
				t.Fatalf("err = %v, want ErrContentChanged", err)
			}
			if calls, _ := generator.snapshot(); len(calls) != 0 || len(generator.stashCalls()) != 0 {
				t.Fatalf("呼び出し = %v, 写し = %v", calls, generator.stashCalls())
			}
			if len(store.images) != 0 {
				t.Fatalf("記録した: %v", store.images)
			}
		})
	}
}

func TestApplyThumbnailImageInvalid(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{recordPublish: true, fitErr: domain.ErrThumbnailImageInvalid}
	ingest, _ := newTestIngest(store, generator)

	_, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("broken"), domain.ThumbnailImageGenerated)
	if !errors.Is(err, domain.ErrThumbnailImageInvalid) {
		t.Fatalf("err = %v", err)
	}
	if calls, _ := generator.snapshot(); !slices.Equal(calls, []string{"fit"}) || len(store.images) != 0 {
		t.Fatalf("呼び出し = %v, 記録 = %v", calls, store.images)
	}
}

// Publishing failure leaves storage unchanged; recording failure restores the previous file.
func TestApplyThumbnailImageFailures(t *testing.T) {
	t.Run("公開の失敗", func(t *testing.T) {
		video := probedVideo(1, "a")
		store := newFakeIngestStore(video)
		generator := &fakeGenerator{thumbnailDir: t.TempDir(), publishErr: errors.New("disk full")}
		ingest, _ := newTestIngest(store, generator)

		_, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"), domain.ThumbnailImageGenerated)
		if !errors.Is(err, generator.publishErr) {
			t.Fatalf("err = %v", err)
		}
		if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "discard"}) {
			t.Fatalf("写しの操作 = %v", got)
		}
		if len(store.images) != 0 {
			t.Fatalf("記録した: %v", store.images)
		}
		if got, _ := store.GetVideo(context.Background(), video.ID); got.ThumbnailImageRevision != 0 {
			t.Fatalf("改版番号 = %d", got.ThumbnailImageRevision)
		}
	})
	t.Run("記録の失敗", func(t *testing.T) {
		video := probedVideo(1, "a")
		store := newFakeIngestStore(video)
		store.applyImageErr = errors.New("database is locked")
		generator := &fakeGenerator{thumbnailDir: t.TempDir()}
		ingest, _ := newTestIngest(store, generator)

		_, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"), domain.ThumbnailImageGenerated)
		if !errors.Is(err, store.applyImageErr) {
			t.Fatalf("err = %v", err)
		}
		if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "restore"}) {
			t.Fatalf("写しの操作 = %v", got)
		}
	})
}

// Reverting regenerates the selected or automatic frame and removes the image override.
func TestRevertThumbnailImageRebuildsFrame(t *testing.T) {
	withPosition := probedVideo(1, "a")
	withPosition.ThumbnailPositionMs = int64Ptr(7_000)
	for _, tc := range []struct {
		name      string
		video     domain.Video
		wantCalls []string
	}{
		{name: "位置あり", video: withPosition,
			wantCalls: []string{"thumbnail-at", "publish-thumbnail", "revert-image"}},
		{name: "位置なし", video: probedVideo(1, "a"),
			wantCalls: []string{"thumbnail", "publish-thumbnail", "revert-image"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeIngestStore(tc.video)
			store.images["a"] = []byte("fitted:edited")
			generator := &fakeGenerator{recordPublish: true}
			store.order = generator
			ingest, _ := newTestIngest(store, generator)

			if _, err := ingest.RevertThumbnailImage(context.Background(), tc.video.ID, tc.video.Path, "/real/a.mp4"); err != nil {
				t.Fatal(err)
			}
			calls, _ := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if tc.video.ThumbnailPositionMs != nil && !slices.Equal(generator.positions, []int64{7_000}) {
				t.Fatalf("ThumbnailAt の位置 = %v", generator.positions)
			}
			if len(store.images) != 0 {
				t.Fatalf("画像の行が残った: %v", store.images)
			}
			if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "discard"}) {
				t.Fatalf("写しの操作 = %v", got)
			}
		})
	}
}

// Failed regeneration preserves the override; failed recording restores the old file.
func TestRevertThumbnailImageFailures(t *testing.T) {
	t.Run("生成の失敗", func(t *testing.T) {
		video := probedVideo(1, "a")
		store := newFakeIngestStore(video)
		store.images["a"] = []byte("fitted:edited")
		generator := &fakeGenerator{thumbnailErr: errors.New("no frame")}
		ingest, _ := newTestIngest(store, generator)

		_, err := ingest.RevertThumbnailImage(context.Background(), video.ID, video.Path, video.Path)
		if !errors.Is(err, domain.ErrThumbnailFrameUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if !bytes.Equal(store.images["a"], []byte("fitted:edited")) {
			t.Fatal("画像の行が消えた")
		}
		if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "discard"}) {
			t.Fatalf("写しの操作 = %v", got)
		}
	})
	t.Run("記録の失敗", func(t *testing.T) {
		video := probedVideo(1, "a")
		store := newFakeIngestStore(video)
		store.images["a"] = []byte("fitted:edited")
		store.revertImageErr = errors.New("database is locked")
		generator := &fakeGenerator{}
		ingest, _ := newTestIngest(store, generator)

		_, err := ingest.RevertThumbnailImage(context.Background(), video.ID, video.Path, video.Path)
		if !errors.Is(err, store.revertImageErr) {
			t.Fatalf("err = %v", err)
		}
		if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "restore"}) {
			t.Fatalf("写しの操作 = %v", got)
		}
	})
}

// Reversion must use the position returned inside the lock, not the initial snapshot.
func TestRevertThumbnailImageRereadsPositionInsideLock(t *testing.T) {
	video := probedVideo(1, "a")
	video.ThumbnailPositionMs = int64Ptr(1_000)
	store := newFakeIngestStore(video)
	store.images["a"] = []byte("fitted:edited")
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)
	store.afterRead = func(reads int) {
		if reads == 1 {
			current := video
			current.ThumbnailPositionMs = int64Ptr(9_000)
			store.videos[video.ID] = current
		} else {
			assertThumbnailLocksHeld(t, ingest.artifacts, "a")
		}
	}
	if _, err := ingest.RevertThumbnailImage(context.Background(), video.ID, video.Path, video.Path); err != nil {
		t.Fatal(err)
	}
	if store.reads != 2 {
		t.Fatalf("video reads = %d, want 2", store.reads)
	}
	if !slices.Equal(generator.positions, []int64{9_000}) {
		t.Fatalf("positions = %v, want [9000]", generator.positions)
	}
}

// Fitting and recording an image must use the same locks as position changes.
func TestApplyThumbnailImageHoldsGenerationLock(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)
	var phases []string
	check := func(phase string) {
		assertThumbnailLocksHeld(t, ingest.artifacts, "a")
		phases = append(phases, phase)
	}
	generator.gate = check
	ingest.store = &thumbnailLockStore{IngestStore: store, check: check}
	if _, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"), domain.ThumbnailImageGenerated); err != nil {
		t.Fatal(err)
	}
	if want := []string{"fit:edited", "record-image"}; !slices.Equal(phases, want) {
		t.Fatalf("phases = %v, want %v", phases, want)
	}
}

// Regenerate image overrides without extracting frames or recording a fallback.
func TestIngestThumbnailRepublishesAppliedImage(t *testing.T) {
	video := probedVideo(1, "a")
	video.ThumbnailPositionMs = int64Ptr(7_000)
	store := newFakeIngestStore(video)
	store.images["a"] = []byte("fitted:edited")
	generator := &fakeGenerator{thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	calls, _ := generator.snapshot()
	if slices.Contains(calls, "thumbnail") || slices.Contains(calls, "thumbnail-at") {
		t.Fatalf("コマを取った: %v", calls)
	}
	if !bytes.Equal(generator.published["a"], []byte("fitted:edited")) {
		t.Fatalf("公開 = %q", generator.published["a"])
	}
	if !slices.Equal(store.thumbnailSubstitutions, []domain.Substitution{domain.SubstitutionNone}) {
		t.Fatalf("代用 = %v", store.thumbnailSubstitutions)
	}
}

// Image overrides remain usable when source media is unreadable.
func TestIngestThumbnailRepublishesAppliedImageWithoutSource(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.images["a"] = []byte("fitted:edited")
	generator := &fakeGenerator{thumbnailDir: t.TempDir(), sourceErr: domain.ErrMediaFileUnavailable}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generator.published["a"], []byte("fitted:edited")) {
		t.Fatalf("公開 = %q", generator.published["a"])
	}
	if !slices.Equal(store.thumbnailSubstitutions, []domain.Substitution{domain.SubstitutionNone}) {
		t.Fatalf("代用 = %v", store.thumbnailSubstitutions)
	}
}
