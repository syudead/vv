package app

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 適用したサムネイルの画像（specs/045-comfyui-thumbnails/data-model.md、research.md R-6・R-7・R-10）。

func sizedVideo(id int64, contentKey string, width, height int) domain.Video {
	video := probedVideo(id, contentKey)
	video.Width, video.Height = &width, &height
	return video
}

// 適用は画像を動画の縦横比に合わせ、今のファイルを写してから公開し、公開したのと同じバイト列を
// 記録する。
func TestApplyThumbnailImageFitsPublishesThenRecords(t *testing.T) {
	video := sizedVideo(1, "a", 1920, 1080)
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{recordPublish: true, thumbnailDir: t.TempDir()}
	store.order = generator
	ingest, _ := newTestIngest(store, generator)

	saved, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"))
	if err != nil {
		t.Fatal(err)
	}
	calls, removed := generator.snapshot()
	if want := []string{"fit", "publish-thumbnail", "apply-image"}; !slices.Equal(calls, want) {
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
	if saved.ThumbnailImageRevision == 0 || saved.ThumbnailState != domain.ThumbnailStateDone {
		t.Fatalf("返した動画 = %+v", saved)
	}
	if len(removed) != 0 {
		t.Fatalf("参照のある生成物を消した: %v", removed)
	}
}

// 大きさの分からない動画では縦横比 0（画像自身の縦横比）で合わせる。
func TestApplyThumbnailImageUnknownSize(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)

	if _, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(generator.aspects, []float64{0}) {
		t.Fatalf("縦横比 = %v", generator.aspects)
	}
}

// 動画が消えたか別の内容を持つなら content_changed で、合わせも公開も記録もしない。
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

			_, err := ingest.ApplyThumbnailImage(context.Background(), tc.videoID, tc.key, []byte("edited"))
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

// 読めない画像は ErrThumbnailImageInvalid で、公開も記録もしない。
func TestApplyThumbnailImageInvalid(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{recordPublish: true, fitErr: domain.ErrThumbnailImageInvalid}
	ingest, _ := newTestIngest(store, generator)

	_, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("broken"))
	if !errors.Is(err, domain.ErrThumbnailImageInvalid) {
		t.Fatalf("err = %v", err)
	}
	if calls, _ := generator.snapshot(); !slices.Equal(calls, []string{"fit"}) || len(store.images) != 0 {
		t.Fatalf("呼び出し = %v, 記録 = %v", calls, store.images)
	}
}

// 公開に失敗すれば記録せず、写しを捨てる（置き場は前の画像のまま）。記録に失敗すれば前の
// ファイルへ戻す。
func TestApplyThumbnailImageFailures(t *testing.T) {
	t.Run("公開の失敗", func(t *testing.T) {
		video := probedVideo(1, "a")
		store := newFakeIngestStore(video)
		generator := &fakeGenerator{thumbnailDir: t.TempDir(), publishErr: errors.New("disk full")}
		ingest, _ := newTestIngest(store, generator)

		_, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"))
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

		_, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"))
		if !errors.Is(err, store.applyImageErr) {
			t.Fatalf("err = %v", err)
		}
		if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "restore"}) {
			t.Fatalf("写しの操作 = %v", got)
		}
	})
}

// 取り消しは記録した位置があればその場面で、無ければ自動の位置で作り直し、画像の行を消す。
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

// 取り消しでコマを作れなければ何も記録せず ErrThumbnailFrameUnavailable を返し、画像の行は残る。
// 記録に失敗すれば前のファイルへ戻す。
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

// 取り消しの位置は生成の錠の中で読み直す。錠を待つ間に記録された位置の場面で作る。
func TestRevertThumbnailImageRereadsPositionInsideLock(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.images["a"] = []byte("fitted:edited")
	hold := newHoldGeneration("thumbnail-at")
	generator := &fakeGenerator{gate: hold.gate}
	ingest, _ := newTestIngest(store, generator)

	picked := make(chan error, 1)
	go func() {
		_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(9_000))
		picked <- err
	}()
	<-hold.started["thumbnail-at"]
	reverted := make(chan error, 1)
	go func() {
		_, err := ingest.RevertThumbnailImage(context.Background(), video.ID, video.Path, video.Path)
		reverted <- err
	}()
	time.Sleep(50 * time.Millisecond)
	close(hold.release["thumbnail-at"])
	if err := waitJob(t, picked); err != nil {
		t.Fatal(err)
	}
	if err := waitJob(t, reverted); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(generator.positions, []int64{9_000, 9_000}) {
		t.Fatalf("ThumbnailAt の位置 = %v, want 錠の中で読み直した位置", generator.positions)
	}
}

// 位置の変更の後ろで錠を待った適用は、位置の記録のあとに画像を公開・記録するので、画像が残る。
func TestApplyThumbnailImageWaitingBehindPositionWins(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	hold := newHoldGeneration("thumbnail-at")
	generator := &fakeGenerator{gate: hold.gate, recordPublish: true, thumbnailDir: t.TempDir()}
	store.order = generator
	ingest, _ := newTestIngest(store, generator)

	picked := make(chan error, 1)
	go func() {
		_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(9_000))
		picked <- err
	}()
	<-hold.started["thumbnail-at"]
	applied := make(chan error, 1)
	go func() {
		_, err := ingest.ApplyThumbnailImage(context.Background(), video.ID, "a", []byte("edited"))
		applied <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if calls, _ := generator.snapshot(); slices.Contains(calls, "fit") {
		t.Fatalf("適用が錠を待たずに進んだ: %v", calls)
	}
	close(hold.release["thumbnail-at"])
	if err := waitJob(t, picked); err != nil {
		t.Fatal(err)
	}
	if err := waitJob(t, applied); err != nil {
		t.Fatal(err)
	}
	calls, _ := generator.snapshot()
	want := []string{"thumbnail-at", "publish-thumbnail", "set-position", "fit", "publish-thumbnail", "apply-image"}
	if !slices.Equal(calls, want) {
		t.Fatalf("呼び出し = %v, want %v", calls, want)
	}
	got, _ := store.GetVideo(context.Background(), video.ID)
	if got.ThumbnailImageRevision == 0 || !bytes.Equal(generator.published["a"], []byte("fitted:edited")) {
		t.Fatalf("画像が残らない: 改版番号 %d, 公開 %q", got.ThumbnailImageRevision, generator.published["a"])
	}
	if got.ThumbnailPositionMs == nil || *got.ThumbnailPositionMs != 9_000 {
		t.Fatalf("位置が残らない: %v", got.ThumbnailPositionMs)
	}
}

// 画像を適用した内容の job は、コマを取らずにその画像を公開し直し、代用なしとして記録する。
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
