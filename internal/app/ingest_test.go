package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
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

// 解析に成功したら結果を反映し、プレビューの仕事を積む。
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
	if !slices.Equal(store.enqueued, []domain.JobKind{domain.JobPreview}) {
		t.Fatalf("積んだ仕事 = %v, want [preview]", store.enqueued)
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
			if len(store.appliedProbes) != 0 || len(store.enqueued) != 0 {
				t.Fatalf("失敗なのに書いた: probes=%v enqueued=%v", store.appliedProbes, store.enqueued)
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
			if len(store.appliedProbes)+len(store.thumbnailStates)+store.previewsDone != 0 {
				t.Fatal("結果を反映した")
			}
		})
	}
}

// サムネイルに成功したら、代表の1枚とシーク用プレビューを作り、done を記録する。
// 代表の1枚がすでにあれば作り直さない。
func TestIngestThumbnailSuccess(t *testing.T) {
	for _, tc := range []struct {
		name      string
		state     domain.ThumbnailState
		wantCalls []string
		wantState int
	}{
		{name: "未作成", state: domain.ThumbnailStatePending, wantCalls: []string{"check", "thumbnail", "seek"}, wantState: 1},
		{name: "作成済み", state: domain.ThumbnailStateDone, wantCalls: []string{"check", "seek"}, wantState: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := probedVideo(1, "a")
			video.ThumbnailState = tc.state
			store := newFakeIngestStore(video)
			generator := &fakeGenerator{}
			ingest, _ := newTestIngest(store, generator)

			if err := ingest.Thumbnail(context.Background(), jobFor(domain.JobThumbnail, video)); err != nil {
				t.Fatal(err)
			}
			calls, removed := generator.snapshot()
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("呼び出し = %v, want %v", calls, tc.wantCalls)
			}
			if len(store.thumbnailStates) != tc.wantState {
				t.Fatalf("記録した状態 = %v", store.thumbnailStates)
			}
			if len(removed) != 0 {
				t.Fatalf("参照のある生成物を消した: %v", removed)
			}
		})
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
	want := []string{"tmp/thumbnail/a", "tmp/seek/a", "tmp/preview/a"}
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

	unlock := ingest.artifacts.lock("readded")
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
		{name: "シーク用プレビューの失敗", generator: &fakeGenerator{seekErr: errors.New("ffmpeg exited 1")},
			wantCalls: []string{"check", "thumbnail", "seek"}},
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
			// 代表サムネイルを作れたあとでシーク用プレビューに失敗した場合だけ、
			// 代表サムネイルの done は記録済みである。
			wantStates := 0
			if tc.generator.seekErr != nil {
				wantStates = 1
			}
			if len(store.thumbnailStates) != wantStates || len(removed) != 0 {
				t.Fatalf("記録した状態 = %v, 消した生成物 = %v", store.thumbnailStates, removed)
			}
		})
	}
}
