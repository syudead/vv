package app

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 代表サムネイルの位置の指定（specs/029-video-overrides/research.md R-4・R-5・R-11）。

func int64Ptr(value int64) *int64 { return &value }

// 指定では ThumbnailAt、解除では自動の位置の Thumbnail で画像を作り、公開してから位置を
// 記録する。
func TestSetThumbnailPositionPublishesThenRecords(t *testing.T) {
	for _, tc := range []struct {
		name      string
		position  *int64
		wantCalls []string
	}{
		{name: "指定", position: int64Ptr(12_000),
			wantCalls: []string{"thumbnail-at", "publish-thumbnail", "set-position"}},
		{name: "解除", position: nil,
			wantCalls: []string{"thumbnail", "publish-thumbnail", "set-position"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			generator := &fakeGenerator{recordPublish: true}
			store.order = generator
			ingest, _ := newTestIngest(store, generator)

			saved, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, "/media/a.mp4", tc.position)
			if err != nil {
				t.Fatal(err)
			}
			calls, removed := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if tc.position != nil && !slices.Equal(generator.positions, []int64{*tc.position}) {
				t.Fatalf("ThumbnailAt の位置 = %v", generator.positions)
			}
			if len(store.positions) != 1 || saved.ThumbnailState != domain.ThumbnailStateDone {
				t.Fatalf("記録 = %v, 返した動画 = %+v", store.positions, saved)
			}
			if len(removed) != 0 {
				t.Fatalf("参照のある生成物を消した: %v", removed)
			}
		})
	}
}

// 生成に失敗したら記録せず ErrThumbnailFrameUnavailable を返す。
func TestSetThumbnailPositionGenerationFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		position  *int64
		generator *fakeGenerator
	}{
		{name: "指定", position: int64Ptr(1_000), generator: &fakeGenerator{thumbnailAtErr: errors.New("no frame")}},
		{name: "解除", position: nil, generator: &fakeGenerator{thumbnailErr: errors.New("no frame")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			ingest, _ := newTestIngest(store, tc.generator)

			_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, "/media/a.mp4", tc.position)
			if !errors.Is(err, domain.ErrThumbnailFrameUnavailable) {
				t.Fatalf("err = %v, want ErrThumbnailFrameUnavailable", err)
			}
			if len(store.positions) != 0 {
				t.Fatalf("失敗なのに記録した: %v", store.positions)
			}
		})
	}
}

// 解析前は ErrDurationUnknown、尺以上は ErrThumbnailPositionOutOfRange で、生成も記録もしない。
func TestSetThumbnailPositionRejectsBeforeGenerating(t *testing.T) {
	pending := probedVideo(1, "a")
	pending.ProbeState = domain.ProbeStatePending
	pending.DurationMs = nil
	probed := probedVideo(2, "b")
	for _, tc := range []struct {
		name     string
		video    domain.Video
		position int64
		want     error
	}{
		{name: "解析前", video: pending, position: 0, want: domain.ErrDurationUnknown},
		{name: "尺ちょうど", video: probed, position: *probed.DurationMs, want: domain.ErrThumbnailPositionOutOfRange},
		{name: "負", video: probed, position: -1, want: domain.ErrThumbnailPositionOutOfRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeIngestStore(tc.video)
			generator := &fakeGenerator{recordPublish: true}
			ingest, _ := newTestIngest(store, generator)

			_, err := ingest.SetThumbnailPosition(context.Background(), tc.video.ID, tc.video.Path, "/media/x.mp4", &tc.position)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if calls, _ := generator.snapshot(); len(calls) != 0 || len(store.positions) != 0 {
				t.Fatalf("呼び出し = %v, 記録 = %v", calls, store.positions)
			}
		})
	}
}

// 位置の指定は生成の錠の中で行う。生成の途中で始めた同じ内容の削除は、記録を待ってから
// 参照を確かめる。
func TestSetThumbnailPositionHoldsGenerationLock(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	hold := newHoldGeneration("thumbnail-at")
	generator := &fakeGenerator{gate: hold.gate, recordPublish: true}
	store.order = generator
	ingest, _ := newTestIngest(store, generator)

	done := make(chan error, 1)
	go func() {
		_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(5_000))
		done <- err
	}()
	<-hold.started["thumbnail-at"]
	store.mu.Lock()
	delete(store.referenced, "a")
	store.mu.Unlock()
	ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{"a"}})
	time.Sleep(50 * time.Millisecond)
	if _, removed := generator.snapshot(); len(removed) != 0 {
		t.Fatalf("記録を待たずに消した: %v", removed)
	}

	close(hold.release["thumbnail-at"])
	if err := waitJob(t, done); err != nil {
		t.Fatal(err)
	}
	ingest.Wait()
	calls, _ := generator.snapshot()
	if !slices.Equal(calls, []string{"thumbnail-at", "publish-thumbnail", "set-position"}) {
		t.Fatalf("呼び出し = %v", calls)
	}
}

// job は位置のある動画ではその位置で ThumbnailAt を、無い動画では Thumbnail を呼ぶ。指定の
// 位置では先頭のコマの代用をしないので、代用なしとして記録する。
func TestIngestThumbnailUsesRecordedPosition(t *testing.T) {
	withPosition := probedVideo(1, "a")
	withPosition.ThumbnailPositionMs = int64Ptr(7_000)
	without := probedVideo(2, "b")
	for _, tc := range []struct {
		name      string
		video     domain.Video
		wantCalls []string
		want      domain.Substitution
	}{
		{name: "位置あり", video: withPosition, wantCalls: []string{"check", "thumbnail-at"}, want: domain.SubstitutionNone},
		{name: "位置なし", video: without, wantCalls: []string{"check", "thumbnail"}, want: domain.SubstitutionNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeIngestStore(tc.video)
			generator := &fakeGenerator{}
			ingest, _ := newTestIngest(store, generator)

			if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, tc.video)); err != nil {
				t.Fatal(err)
			}
			calls, _ := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if tc.video.ThumbnailPositionMs != nil && !slices.Equal(generator.positions, []int64{7_000}) {
				t.Fatalf("ThumbnailAt の位置 = %v", generator.positions)
			}
			if !slices.Equal(store.thumbnailSubstitutions, []domain.Substitution{tc.want}) {
				t.Fatalf("代用 = %v, want [%v]", store.thumbnailSubstitutions, tc.want)
			}
		})
	}
}

// 指定の位置で取れなければ job は失敗を返し（再試行と failed の記録は待ち行列が持つ）、
// 位置の行は消さない。
func TestIngestThumbnailAtPositionFailureKeepsPosition(t *testing.T) {
	video := probedVideo(1, "a")
	video.ThumbnailPositionMs = int64Ptr(7_000)
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{thumbnailAtErr: errors.New("no frame")}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err == nil {
		t.Fatal("失敗が返らない")
	}
	if calls, _ := generator.snapshot(); slices.Contains(calls, "thumbnail") {
		t.Fatalf("先頭のコマで作り直した: %v", calls)
	}
	if len(store.thumbnailStates) != 0 || len(store.positions) != 0 {
		t.Fatalf("状態 = %v, 位置の記録 = %v", store.thumbnailStates, store.positions)
	}
	if got, _ := store.GetVideo(context.Background(), video.ID); got.ThumbnailPositionMs == nil {
		t.Fatal("位置が消えた")
	}
}

// job が錠の外で動画を読んだあと、錠を待つ間に SetThumbnailPosition が位置と done を記録
// したら、job は錠の中で読み直して生成を飛ばす。記録された位置の画像が残る。
func TestIngestThumbnailRereadsInsideLock(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	hold := newHoldGeneration("thumbnail-at")
	generator := &fakeGenerator{gate: hold.gate}
	ingest, _ := newTestIngest(store, generator)

	picked := make(chan error, 1)
	go func() {
		_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(9_000))
		picked <- err
	}()
	<-hold.started["thumbnail-at"]

	job := runJob(ingest, domain.JobThumbnail, video)
	// job が錠の外で動画を読み（pending のまま）、錠を待つところまで進める。
	deadline := time.Now().Add(2 * time.Second)
	for {
		store.mu.Lock()
		reads := store.reads
		store.mu.Unlock()
		calls, _ := generator.snapshot()
		if reads >= 2 && slices.Contains(calls, "check") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job が錠の前まで進まない")
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(hold.release["thumbnail-at"])
	if err := waitJob(t, picked); err != nil {
		t.Fatal(err)
	}
	if err := waitJob(t, job); err != nil {
		t.Fatal(err)
	}
	calls, _ := generator.snapshot()
	if slices.Contains(calls, "thumbnail") || !slices.Equal(generator.positions, []int64{9_000}) {
		t.Fatalf("job が画像を作り直した: 呼び出し %v, 位置 %v", calls, generator.positions)
	}
	if len(store.thumbnailStates) != 0 {
		t.Fatalf("job が状態を書いた: %v", store.thumbnailStates)
	}
}

// 公開のあとで記録に失敗したら（取引の失敗・要求の取り消し）、前の画像へ戻し、記録の位置と
// 版に画像を揃える。記録できたときは写しを捨てる。
func TestSetThumbnailPositionRestoresPreviousImageWhenRecordingFails(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.setPositionErr = errors.New("database is locked")
	generator := &fakeGenerator{recordPublish: true}
	ingest, _ := newTestIngest(store, generator)

	_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(4_000))
	if err == nil || !errors.Is(err, store.setPositionErr) {
		t.Fatalf("err = %v, want 記録の失敗", err)
	}
	if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "restore"}) {
		t.Fatalf("写しの操作 = %v, want [stash restore]", got)
	}

	store.setPositionErr = nil
	generator.stashes = nil
	if _, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(4_000)); err != nil {
		t.Fatal(err)
	}
	if got := generator.stashCalls(); !slices.Equal(got, []string{"stash", "discard"}) {
		t.Fatalf("写しの操作 = %v, want [stash discard]", got)
	}
}

// 所在を決めたあとに走査がそれを別の内容へ付け替えたら、別の動画のコマをこの内容の画像として
// 公開・記録せずに ErrMediaFileUnavailable を返す。生成の直前に付け替わっていれば生成しない。
// 生成の間に付け替わっていれば、生成した画像を公開しない（並ぶ要求にも見せない）。公開と記録の
// 間に付け替わっていれば、記録せずに前の画像へ戻す。
func TestSetThumbnailPositionRevalidatesSource(t *testing.T) {
	for _, tc := range []struct {
		name        string
		staleAt     int
		wantCalls   []string
		wantStashes []string
	}{
		{name: "生成の前", staleAt: 1, wantCalls: nil, wantStashes: nil},
		{name: "生成の間", staleAt: 2,
			wantCalls: []string{"thumbnail-at"}, wantStashes: []string{"stash", "discard"}},
		{name: "公開と記録の間", staleAt: 3,
			wantCalls: []string{"thumbnail-at", "publish-thumbnail"}, wantStashes: []string{"stash", "restore"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			store.staleSourceAt = tc.staleAt
			generator := &fakeGenerator{recordPublish: true}
			ingest, _ := newTestIngest(store, generator)

			_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, "/media/a.mp4", "/real/a.mp4", int64Ptr(4_000))
			if !errors.Is(err, domain.ErrMediaFileUnavailable) {
				t.Fatalf("err = %v, want ErrMediaFileUnavailable", err)
			}
			calls, _ := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if got := generator.stashCalls(); !slices.Equal(got, tc.wantStashes) {
				t.Fatalf("写しの操作 = %v, want %v", got, tc.wantStashes)
			}
			if len(store.positions) != 0 {
				t.Fatalf("付け替わった所在で記録した: %v", store.positions)
			}
			for _, location := range store.sourceLocations {
				if location != "/media/a.mp4" {
					t.Fatalf("確かめ直した所在 = %v, want 辿る前の所在", store.sourceLocations)
				}
			}
		})
	}
}

// 生成の錠を待つ間に動画が消えていたら、所在の確かめ直しは ErrNotFound を返し、生成しない。
func TestSetThumbnailPositionVideoGoneBeforeGenerating(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.staleSourceAt = 1
	// 錠の外の最初の読み出しのあとに動画を消す。
	store.afterRead = func(reads int) {
		if reads == 1 {
			delete(store.videos, video.ID)
		}
	}
	generator := &fakeGenerator{recordPublish: true}
	ingest, _ := newTestIngest(store, generator)

	_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(4_000))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if calls, _ := generator.snapshot(); len(calls) != 0 {
		t.Fatalf("消えた動画で生成した: %v", calls)
	}
}
