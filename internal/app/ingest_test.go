package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func newTestIngest(store *fakeIngestStore, generator *fakeGenerator) (*Ingest, *fakePublisher) {
	publisher := &fakePublisher{}
	return NewIngest(IngestOptions{
		Store: store, Generator: generator, Artifacts: generator, Publisher: publisher, Logger: discardLogger(),
	}), publisher
}

// 解析に成功したら結果を反映する。プレビューの仕事は保存側が同じ取引で積む。
func TestIngestProbeSuccess(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{probe: domain.Probe{DurationMs: 1000, VideoCodec: "h264", AudioCodec: "aac"}}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Handler(domain.JobProbe)(context.Background(), jobFor(domain.JobProbe, video)); err != nil {
		t.Fatal(err)
	}
	if len(store.appliedProbes) != 1 || store.appliedProbes[0].DurationMs != 1000 {
		t.Fatalf("反映した解析 = %+v", store.appliedProbes)
	}
}

// 解析に失敗したら、状態を書かずに失敗を返す（上限の判断は待ち行列が持つ）。
func TestIngestProbeFailure(t *testing.T) {
	video := probedVideo(1, "a")
	for _, tc := range []struct {
		name      string
		generator *fakeGenerator
	}{
		{name: "読めない元", generator: &fakeGenerator{sourceErr: errors.New("no such file")}},
		{name: "ffprobe の失敗", generator: &fakeGenerator{probeErr: errors.New("moov atom not found")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeIngestStore(video)
			ingest, _ := newTestIngest(store, tc.generator)
			if err := ingest.Probe(context.Background(), jobFor(domain.JobProbe, video)); err == nil {
				t.Fatal("失敗が返らない")
			}
			if len(store.appliedProbes) != 0 {
				t.Fatalf("失敗なのに書いた: probes=%v", store.appliedProbes)
			}
		})
	}
}

// 専有した後に所在や内容が変わっていたら、何も生成せず、結果も反映しない。
func TestIngestSkipsStaleJobs(t *testing.T) {
	video := probedVideo(1, "a")
	for _, kind := range domain.JobKinds {
		t.Run(string(kind), func(t *testing.T) {
			store := newFakeIngestStore(video)
			store.stale = true
			generator := &fakeGenerator{}
			ingest, _ := newTestIngest(store, generator)
			if err := ingest.Handler(kind)(context.Background(), jobFor(kind, video)); err != nil {
				t.Fatal(err)
			}
			if calls, _ := generator.snapshot(); len(calls) != 0 {
				t.Fatalf("生成を呼んだ: %v", calls)
			}
			if len(store.appliedProbes)+len(store.thumbnailStates)+len(store.seekStates)+store.previewsDone != 0 {
				t.Fatal("結果を反映した")
			}
		})
	}
}

// 代表サムネイルのジョブは代表の1枚だけを作り、done を記録する。シーク用
// サムネイルは作らない。代表の1枚がすでにあれば作り直さない。
func TestIngestThumbnailSuccess(t *testing.T) {
	for _, tc := range []struct {
		name      string
		state     domain.ThumbnailState
		wantCalls []string
		wantState int
	}{
		{name: "未作成", state: domain.ThumbnailStatePending, wantCalls: []string{"check", "thumbnail"}, wantState: 1},
		{name: "作成済み", state: domain.ThumbnailStateDone, wantCalls: []string{"check"}, wantState: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			video.ThumbnailState = tc.state
			store := newFakeIngestStore(video)
			generator := &fakeGenerator{}
			ingest, _ := newTestIngest(store, generator)

			if err := ingest.Handler(domain.JobThumbnail)(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
				t.Fatal(err)
			}
			calls, removed := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if len(store.thumbnailStates) != tc.wantState || len(store.seekStates) != 0 {
				t.Fatalf("記録した状態 = %v, シーク用 = %v", store.thumbnailStates, store.seekStates)
			}
			if len(removed) != 0 {
				t.Fatalf("参照のある生成物を消した: %v", removed)
			}
		})
	}
}

// シーク用サムネイルのジョブは、置き場へ書き終えてから done を記録する。
// 代表サムネイルは作らず、その状態も記録しない。
func TestIngestSeekThumbnailsSuccess(t *testing.T) {
	video := probedVideo(1, "a")
	video.ThumbnailState = domain.ThumbnailStateDone
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{}
	store.order = generator
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Handler(domain.JobSeekThumbnail)(context.Background(), jobFor(domain.JobSeekThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	calls, removed := generator.snapshot()
	if want := []string{"check", "seek", "seek-state:done"}; !slices.Equal(calls, want) {
		t.Fatalf("呼び出し = %v, want %v", calls, want)
	}
	if len(store.thumbnailStates) != 0 {
		t.Fatalf("代表サムネイルの状態を記録した: %v", store.thumbnailStates)
	}
	if len(removed) != 0 {
		t.Fatalf("参照のある生成物を消した: %v", removed)
	}
	if !slices.Equal(generator.outputs, []string{"tmp/seek/a"}) {
		t.Fatalf("書き出し先 = %v, want [tmp/seek/a]", generator.outputs)
	}
}

// シーク用サムネイルの配置は動画の長さから決め、公開と生成に同じ配置を渡す。
func TestIngestSeekThumbnailsLayoutFollowsDuration(t *testing.T) {
	video := probedVideo(1, "a")
	twoHours := int64(2 * 60 * 60 * 1000)
	video.DurationMs = &twoHours
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.SeekThumbnails(context.Background(), jobFor(domain.JobSeekThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	want := domain.SeekSpriteLayout{IntervalMs: 88_889, FrameCount: 81, Columns: 9, Rows: 9, SheetCount: 1}
	if !slices.Equal(generator.seekLayouts, []domain.SeekSpriteLayout{want}) ||
		!slices.Equal(generator.publishedLayouts, []domain.SeekSpriteLayout{want}) {
		t.Fatalf("生成の配置 = %+v, 公開の配置 = %+v, want %+v",
			generator.seekLayouts, generator.publishedLayouts, want)
	}
}

// シーク用サムネイルの失敗は、どちらの状態も書かずに返す（上限の判断と
// seek_thumbnail_state への記録は待ち行列が持つ）。代表サムネイルの状態には触れない。
func TestIngestSeekThumbnailsFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		generator *fakeGenerator
		wantCalls []string
	}{
		{name: "読めない元", generator: &fakeGenerator{sourceErr: errors.New("no such file")}, wantCalls: []string{"check"}},
		{name: "生成の失敗", generator: &fakeGenerator{seekErr: errors.New("ffmpeg exited 1")}, wantCalls: []string{"check", "seek"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			ingest, _ := newTestIngest(store, tc.generator)
			if err := ingest.SeekThumbnails(context.Background(), jobFor(domain.JobSeekThumbnail, video)); err == nil {
				t.Fatal("失敗が返らない")
			}
			calls, removed := tc.generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if len(store.thumbnailStates)+len(store.seekStates) != 0 || len(removed) != 0 {
				t.Fatalf("状態 = %v / %v, 消した生成物 = %v", store.thumbnailStates, store.seekStates, removed)
			}
		})
	}
}

// 生成中に動画が消えていたら、書き終えたシーク用サムネイルを残さない。
func TestIngestSeekThumbnailsRemovesArtifactsOfVanishedVideo(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.gone = true
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.SeekThumbnails(context.Background(), jobFor(domain.JobSeekThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	if _, removed := generator.snapshot(); !slices.Equal(removed, []string{"a"}) {
		t.Fatalf("消した生成物 = %v, want [a]", removed)
	}
	if len(store.seekStates) != 0 {
		t.Fatalf("消えた動画に状態を記録した: %v", store.seekStates)
	}
}

// 生成は、置き場が渡した一時置き場のパスへ書く。
func TestIngestWritesIntoPublishedOutputs(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	if err := ingest.Preview(context.Background(), jobFor(domain.JobPreview, video)); err != nil {
		t.Fatal(err)
	}
	if err := ingest.SeekThumbnails(context.Background(), jobFor(domain.JobSeekThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	want := []string{"tmp/thumbnail/a", "tmp/preview/a", "tmp/seek/a"}
	if !slices.Equal(generator.outputs, want) {
		t.Fatalf("書き出し先 = %v, want %v", generator.outputs, want)
	}
}

// 生成中に動画が消えていたら、書き終えたサムネイルを残さない。
func TestIngestThumbnailRemovesArtifactsOfVanishedVideo(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.gone = true
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
		t.Fatal(err)
	}
	if _, removed := generator.snapshot(); !slices.Equal(removed, []string{"a"}) {
		t.Fatalf("消した生成物 = %v, want [a]", removed)
	}
}

// プレビューに成功したら完了を記録する。生成中は元の同一性を確かめる。
func TestIngestPreviewSuccess(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Preview(context.Background(), jobFor(domain.JobPreview, video)); err != nil {
		t.Fatal(err)
	}
	if store.previewsDone != 1 || !slices.Equal(generator.validated, []bool{true}) {
		t.Fatalf("完了 = %d, 確かめた同一性 = %v", store.previewsDone, generator.validated)
	}
}

// プレビューの失敗は、状態を書かずに返す。生成中に動画が消えていたら、書き終えた
// 生成物を消して domain.ErrPreviewStale を返す（やり直してよい）。
func TestIngestPreviewFailure(t *testing.T) {
	t.Run("長さが無い", func(t *testing.T) {
		video := probedVideo(1, "a")
		video.DurationMs = nil
		ingest, _ := newTestIngest(newFakeIngestStore(video), &fakeGenerator{})
		if err := ingest.Preview(context.Background(), jobFor(domain.JobPreview, video)); !errors.Is(err, errMissingDuration) {
			t.Fatalf("err = %v, want 長さが無い", err)
		}
	})
	t.Run("生成の失敗", func(t *testing.T) {
		video := probedVideo(1, "a")
		store := newFakeIngestStore(video)
		ingest, _ := newTestIngest(store, &fakeGenerator{previewErr: errors.New("ffmpeg exited 1")})
		if err := ingest.Preview(context.Background(), jobFor(domain.JobPreview, video)); err == nil {
			t.Fatal("失敗が返らない")
		}
		if store.previewsDone != 0 {
			t.Fatal("失敗なのに完了を記録した")
		}
	})
	t.Run("生成中に消えた", func(t *testing.T) {
		video := probedVideo(1, "a")
		store := newFakeIngestStore(video)
		store.gone = true
		generator := &fakeGenerator{}
		ingest, _ := newTestIngest(store, generator)
		if err := ingest.Preview(context.Background(), jobFor(domain.JobPreview, video)); !errors.Is(err, domain.ErrPreviewStale) {
			t.Fatalf("err = %v, want ErrPreviewStale", err)
		}
		if _, removed := generator.snapshot(); !slices.Equal(removed, []string{"a"}) {
			t.Fatalf("消した生成物 = %v, want [a]", removed)
		}
	})
	t.Run("解析前", func(t *testing.T) {
		video := probedVideo(1, "a")
		video.ProbeState = domain.ProbeStatePending
		generator := &fakeGenerator{}
		ingest, _ := newTestIngest(newFakeIngestStore(video), generator)
		if err := ingest.Preview(context.Background(), jobFor(domain.JobPreview, video)); err != nil {
			t.Fatal(err)
		}
		if calls, _ := generator.snapshot(); len(calls) != 0 {
			t.Fatalf("解析前に生成した: %v", calls)
		}
	})
}

// 1件の成否が記録されたら、その動画の状態がどの段階で変わったかと、残りの
// 変化を発行する。どのワーカーを起こすかは購読する側が決める。
func TestJobFinishedPublishesStageAndProcessing(t *testing.T) {
	ingest, publisher := newTestIngest(newFakeIngestStore(), &fakeGenerator{})

	ingest.JobFinished(domain.Job{Kind: domain.JobProbe, VideoID: 7})
	ingest.JobFinished(domain.Job{Kind: domain.JobThumbnail, VideoID: 8})

	want := []domain.Event{
		domain.VideoIngestChanged{VideoID: 7, Stage: domain.JobProbe}, domain.ProcessingChanged{},
		domain.VideoIngestChanged{VideoID: 8, Stage: domain.JobThumbnail}, domain.ProcessingChanged{},
	}
	if got := publisher.published(); !reflect.DeepEqual(got, want) {
		t.Fatalf("発行 = %#v, want %#v", got, want)
	}
}

// 参照の無くなった内容の生成物だけを消す。消す直前に参照を確かめ直す。
func TestReleaseArtifactsRemovesOnlyUnreferencedContent(t *testing.T) {
	kept := probedVideo(1, "kept")
	store := newFakeIngestStore(kept)
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)

	ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{"kept", "released"}})
	ingest.Wait()

	if _, removed := generator.snapshot(); !slices.Equal(removed, []string{"released"}) {
		t.Fatalf("消した生成物 = %v, want [released]", removed)
	}
}

// 削除は同じ内容の生成と直列になる。生成の側が錠を持っている間に同じ内容の
// 動画が取り込まれて完了すれば、そのあとに走る削除は参照を見て消さない。
func TestReleaseWaitsForGenerationOfSameContent(t *testing.T) {
	store := newFakeIngestStore()
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)

	unlock := ingest.artifacts.lockGeneration("readded", artifactPreview)
	ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{"readded"}})
	time.Sleep(50 * time.Millisecond)
	if _, removed := generator.snapshot(); len(removed) != 0 {
		t.Fatalf("錠を待たずに消した: %v", removed)
	}

	// 錠の中で同じ内容の動画が取り込まれ、既存の生成物を採用して完了する。
	store.mu.Lock()
	store.referenced["readded"] = true
	store.mu.Unlock()
	unlock()
	ingest.Wait()

	if _, removed := generator.snapshot(); len(removed) != 0 {
		t.Fatalf("取り込み直した動画の生成物を消した: %v", removed)
	}
}

// サムネイルの失敗は、状態を書かずに返す（上限の判断は待ち行列が持つ）。
// 生成物は、参照する動画が残っている限り消さない。
func TestIngestThumbnailFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		generator *fakeGenerator
		wantCalls []string
	}{
		{name: "読めない元", generator: &fakeGenerator{sourceErr: errors.New("no such file")},
			wantCalls: []string{"check"}},
		{name: "代表サムネイルの失敗", generator: &fakeGenerator{thumbnailErr: errors.New("ffmpeg exited 1")},
			wantCalls: []string{"check", "thumbnail"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			ingest, _ := newTestIngest(store, tc.generator)
			if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err == nil {
				t.Fatal("失敗が返らない")
			}
			calls, removed := tc.generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if len(store.thumbnailStates)+len(store.seekStates) != 0 || len(removed) != 0 {
				t.Fatalf("記録した状態 = %v / %v, 消した生成物 = %v", store.thumbnailStates, store.seekStates, removed)
			}
		})
	}
}

// 生成が返した代用を、成功を書く保存側へ渡す（specs/024-import-progress/research.md R-7）。
// 置き場に完成したシーク用サムネイルがあって生成しなかったときは、置き場が生成時に
// 残した記録を渡す。記録の無い旧いものは、代用したかが分からないので Unknown を渡す。
func TestIngestPassesSubstitutionToStore(t *testing.T) {
	for _, tc := range []struct {
		name          string
		generator     *fakeGenerator
		wantThumbnail domain.Substitution
		wantSeek      domain.Substitution
	}{
		{name: "代用なし", generator: &fakeGenerator{}, wantThumbnail: domain.SubstitutionNone, wantSeek: domain.SubstitutionNone},
		{
			name: "先頭のコマと全編から", generator: &fakeGenerator{firstFrame: true, fullDecode: true},
			wantThumbnail: domain.SubstitutionUsed, wantSeek: domain.SubstitutionUsed,
		},
		{
			name: "記録の無い既存のシーク用サムネイルを採用", generator: &fakeGenerator{seekPublished: true, fullDecode: true},
			wantThumbnail: domain.SubstitutionNone, wantSeek: domain.SubstitutionUnknown,
		},
		{
			// 全編から作って公開した後、完了を記録する前に止まった再実行
			name:          "全編から作った既存のシーク用サムネイルを採用",
			generator:     &fakeGenerator{seekPublished: true, publishedSubstitution: domain.SubstitutionUsed},
			wantThumbnail: domain.SubstitutionNone, wantSeek: domain.SubstitutionUsed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			ingest, _ := newTestIngest(store, tc.generator)

			if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
				t.Fatal(err)
			}
			if err := ingest.SeekThumbnails(context.Background(), jobFor(domain.JobSeekThumbnail, video)); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(store.thumbnailSubstitutions, []domain.Substitution{tc.wantThumbnail}) {
				t.Fatalf("代表サムネイルの代用 = %v, want %v", store.thumbnailSubstitutions, tc.wantThumbnail)
			}
			if !slices.Equal(store.seekSubstitutions, []domain.Substitution{tc.wantSeek}) {
				t.Fatalf("シーク用サムネイルの代用 = %v, want %v", store.seekSubstitutions, tc.wantSeek)
			}
		})
	}
}

// holdGeneration は生成の呼び出し held の最初の1回を止める門である。started は
// 止まったことを、release は先へ進めることを表す。
type holdGeneration struct {
	started map[string]chan struct{}
	release map[string]chan struct{}

	mu   sync.Mutex
	used map[string]bool
}

func newHoldGeneration(held ...string) *holdGeneration {
	h := &holdGeneration{
		started: map[string]chan struct{}{}, release: map[string]chan struct{}{}, used: map[string]bool{},
	}
	for _, call := range held {
		h.started[call] = make(chan struct{})
		h.release[call] = make(chan struct{})
	}
	return h
}

func (h *holdGeneration) gate(call string) {
	started, ok := h.started[call]
	h.mu.Lock()
	first := ok && !h.used[call]
	h.used[call] = true
	h.mu.Unlock()
	if first {
		close(started)
		<-h.release[call]
	}
}

func runJob(ingest *Ingest, kind domain.JobKind, video domain.Video) <-chan error {
	done := make(chan error, 1)
	job := jobFor(kind, video)
	go func() {
		switch kind {
		case domain.JobThumbnail:
			done <- ingest.Thumbnail(context.Background(), job)
		case domain.JobSeekThumbnail:
			done <- ingest.SeekThumbnails(context.Background(), job)
		default:
			done <- ingest.Preview(context.Background(), job)
		}
	}()
	return done
}

func waitJob(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("生成が終わらない")
		return nil
	}
}

// 同じ内容でも種類の違う生成は、互いの生成の終わりを待たずに完了を記録する。
func TestGenerationKindsDoNotWaitForEachOther(t *testing.T) {
	for _, tc := range []struct {
		name       string
		held       string
		heldKind   domain.JobKind
		other      domain.JobKind
		otherCall  string
		otherState func(*fakeIngestStore) int
	}{
		{
			name: "プレビューの生成中の代表サムネイル", held: "preview", heldKind: domain.JobPreview,
			other: domain.JobThumbnail, otherState: func(s *fakeIngestStore) int { return len(s.thumbnailStates) },
		},
		{
			name: "プレビューの生成中のシーク用サムネイル", held: "preview", heldKind: domain.JobPreview,
			other: domain.JobSeekThumbnail, otherState: func(s *fakeIngestStore) int { return len(s.seekStates) },
		},
		{
			name: "シーク用サムネイルの生成中のプレビュー", held: "seek", heldKind: domain.JobSeekThumbnail,
			other: domain.JobPreview, otherState: func(s *fakeIngestStore) int { return s.previewsDone },
		},
		{
			name: "シーク用サムネイルの生成中の代表サムネイル", held: "seek", heldKind: domain.JobSeekThumbnail,
			other: domain.JobThumbnail, otherState: func(s *fakeIngestStore) int { return len(s.thumbnailStates) },
		},
		{
			name: "代表サムネイルの生成中のシーク用サムネイル", held: "thumbnail", heldKind: domain.JobThumbnail,
			other: domain.JobSeekThumbnail, otherState: func(s *fakeIngestStore) int { return len(s.seekStates) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			store := newFakeIngestStore(video)
			hold := newHoldGeneration(tc.held)
			ingest, _ := newTestIngest(store, &fakeGenerator{gate: hold.gate})

			held := runJob(ingest, tc.heldKind, video)
			<-hold.started[tc.held]
			if err := waitJob(t, runJob(ingest, tc.other, video)); err != nil {
				t.Fatal(err)
			}
			store.mu.Lock()
			recorded := tc.otherState(store)
			store.mu.Unlock()
			if recorded != 1 {
				t.Fatalf("記録した完了 = %d, want 1", recorded)
			}

			close(hold.release[tc.held])
			if err := waitJob(t, held); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// 生成の途中で同じ内容の削除を始めると、削除はその生成の完了の記録を待ち、
// そのあと参照が無ければ消す。
func TestReleaseWaitsForGenerationInProgress(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	hold := newHoldGeneration("preview")
	generator := &fakeGenerator{gate: hold.gate}
	ingest, _ := newTestIngest(store, generator)

	preview := runJob(ingest, domain.JobPreview, video)
	<-hold.started["preview"]
	store.mu.Lock()
	delete(store.referenced, "a")
	store.mu.Unlock()
	ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{"a"}})
	time.Sleep(50 * time.Millisecond)
	if _, removed := generator.snapshot(); len(removed) != 0 {
		t.Fatalf("生成の完了を待たずに消した: %v", removed)
	}

	close(hold.release["preview"])
	if err := waitJob(t, preview); err != nil {
		t.Fatal(err)
	}
	ingest.Wait()
	if _, removed := generator.snapshot(); store.previewsDone != 1 || !slices.Equal(removed, []string{"a"}) {
		t.Fatalf("完了 = %d, 消した生成物 = %v, want 1 / [a]", store.previewsDone, removed)
	}
}

// 生成中に動画が消えたとき、先に終わった生成の後始末は、同じ内容の別の種類の
// 生成が終わるまで消さない。最後には参照の無くなった生成物が消える。
func TestCleanupAfterVanishWaitsForOtherKinds(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.gone = true
	hold := newHoldGeneration("preview", "seek")
	generator := &fakeGenerator{gate: hold.gate}
	ingest, _ := newTestIngest(store, generator)

	preview := runJob(ingest, domain.JobPreview, video)
	seek := runJob(ingest, domain.JobSeekThumbnail, video)
	<-hold.started["preview"]
	<-hold.started["seek"]

	close(hold.release["seek"])
	time.Sleep(50 * time.Millisecond)
	if _, removed := generator.snapshot(); len(removed) != 0 {
		t.Fatalf("別の種類の生成を待たずに消した: %v", removed)
	}

	close(hold.release["preview"])
	if err := waitJob(t, preview); !errors.Is(err, domain.ErrPreviewStale) {
		t.Fatalf("err = %v, want ErrPreviewStale", err)
	}
	if err := waitJob(t, seek); err != nil {
		t.Fatal(err)
	}
	if _, removed := generator.snapshot(); len(removed) == 0 {
		t.Fatal("参照の無くなった生成物が残った")
	}
}

// 同じ内容の同じ種類の生成（重複ファイルの別の動画など）は直列になる。
func TestGenerationOfSameKindIsSerialized(t *testing.T) {
	first := probedVideo(1, "a")
	second := probedVideo(2, "a")
	store := newFakeIngestStore(first, second)
	hold := newHoldGeneration("preview")
	generator := &fakeGenerator{gate: hold.gate}
	ingest, _ := newTestIngest(store, generator)

	held := runJob(ingest, domain.JobPreview, first)
	<-hold.started["preview"]
	// 門は最初の1回だけを止めるので、2本目は錠が無ければそのまま完了する。
	other := runJob(ingest, domain.JobPreview, second)
	select {
	case err := <-other:
		t.Fatalf("同じ種類の生成を待たずに終わった: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(hold.release["preview"])
	if err := waitJob(t, held); err != nil {
		t.Fatal(err)
	}
	if err := waitJob(t, other); err != nil {
		t.Fatal(err)
	}
}
