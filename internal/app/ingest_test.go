package app

import (
	"context"
	"errors"
	"io/fs"
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
		Store: store, Generator: generator, Artifacts: fakeArtifactStore{generator}, Posters: generator,
		Publisher: publisher, Logger: discardLogger(),
	}), publisher
}

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

// Pass persisted substitution provenance for reused sprites; older metadata supplies Unknown.
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
			// Simulate restart after full-decode publication but before completion recording.
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

// Block only the first matching generation; started signals entry and release resumes it.
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

func TestIngestFingerprintReadsSpriteAndSavesFingerprint(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	layout := domain.NewSeekSpriteLayout(600_000)
	generator := &fakeGenerator{
		sprites: map[string]domain.SeekSprite{"a": {SeekSpriteLayout: layout, FrameWidth: 160, FrameHeight: 90}},
		sheets:  map[string][][]byte{"a": {[]byte("sheet-0")}},
	}
	ingest, _ := newTestIngest(store, generator)

	if err := ingest.Handler(domain.JobFingerprint)(context.Background(), jobFor(domain.JobFingerprint, video)); err != nil {
		t.Fatal(err)
	}
	if len(store.fingerprints) != 1 {
		t.Fatalf("記録した指紋 = %+v", store.fingerprints)
	}
	if got := store.fingerprints[0]; got.Version != domain.FingerprintVersion || got.IntervalMs != layout.IntervalMs ||
		len(got.Frames) != 1 || got.Frames[0].Hash != uint64(len("sheet-0")) {
		t.Fatalf("記録した指紋 = %+v", got)
	}
	if len(generator.fingerprintSheets) != 1 || string(generator.fingerprintSheets[0][0]) != "sheet-0" {
		t.Fatalf("指紋に渡したシート = %q", generator.fingerprintSheets)
	}
	calls, _ := generator.snapshot()
	if !slices.Equal(calls, []string{"fingerprint"}) {
		t.Fatalf("呼び出し = %v（元の動画を読んではいけない）", calls)
	}
}

func TestIngestFingerprintFailsWithoutSprite(t *testing.T) {
	video := probedVideo(1, "a")
	layout := domain.NewSeekSpriteLayout(600_000)
	for name, generator := range map[string]*fakeGenerator{
		"スプライトが無い": {},
		"シートが無い": {
			sprites: map[string]domain.SeekSprite{"a": {SeekSpriteLayout: layout, FrameWidth: 160, FrameHeight: 90}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := newFakeIngestStore(video)
			ingest, _ := newTestIngest(store, generator)
			err := ingest.Handler(domain.JobFingerprint)(context.Background(), jobFor(domain.JobFingerprint, video))
			if !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("err = %v", err)
			}
			if len(store.fingerprints) != 0 {
				t.Fatalf("記録した指紋 = %+v", store.fingerprints)
			}
		})
	}
}

func TestIngestFingerprintSkipsStaleJob(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	store.stale = true
	generator := &fakeGenerator{}
	ingest, _ := newTestIngest(store, generator)
	if err := ingest.Handler(domain.JobFingerprint)(context.Background(), jobFor(domain.JobFingerprint, video)); err != nil {
		t.Fatal(err)
	}
	if calls, _ := generator.snapshot(); len(calls) != 0 || len(store.fingerprints) != 0 {
		t.Fatalf("呼び出し = %v、指紋 = %+v", calls, store.fingerprints)
	}
}
