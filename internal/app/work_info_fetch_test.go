package app

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの検索・取得・反映の判断（specs/049-work-info-plugins/contracts/screen-api.md、
// research.md R-8・R-9・R-13）。保存先の取引の中の読み直しと書き込みは internal/store のテストが本物の
// SQLite で確かめる。

// runPluginStore はプラグインを id で持つ保存先である。登録の書き込みは使わない。
type runPluginStore struct {
	fakePluginStore
	plugins map[int64]*domain.WorkInfoPlugin
}

func (s *runPluginStore) WorkInfoPlugin(_ context.Context, id int64) (domain.WorkInfoPlugin, error) {
	plugin, ok := s.plugins[id]
	if !ok {
		return domain.WorkInfoPlugin{}, domain.ErrNotFound
	}
	return *plugin, nil
}

func (s *runPluginStore) WorkInfoPluginModule(ctx context.Context, id int64) (domain.WorkInfoPlugin, []byte, error) {
	plugin, err := s.WorkInfoPlugin(ctx, id)
	if err != nil {
		return domain.WorkInfoPlugin{}, nil, err
	}
	return plugin, []byte("module " + plugin.Digest), nil
}

// recordingRuntime は実行の要求を記録し、決めた結果を返す。本物の実行と同じく、stderr を標準エラーとした
// 実行のログを record に渡す。
type recordingRuntime struct {
	fakePluginRuntime
	requests  []domain.WorkInfoRunRequest
	digests   []string
	networks  []domain.WorkInfoRunNetwork
	result    domain.WorkInfoRunResult
	err       error
	stderr    string
	truncated bool
	// recordErrs は record が返した誤りである。本物の実行はサーバーのログに残すだけで、結果を変えない。
	recordErrs []error
}

func (r *recordingRuntime) Run(
	_ context.Context, digest string, _ []byte, req domain.WorkInfoRunRequest, network domain.WorkInfoRunNetwork,
	record func(domain.WorkInfoRunLog) error,
) (domain.WorkInfoRunResult, domain.WorkInfoRunLog, error) {
	r.requests = append(r.requests, req)
	r.networks = append(r.networks, network)
	r.digests = append(r.digests, digest)
	log := domain.WorkInfoRunLog{Operation: req.Operation, Stderr: r.stderr, Truncated: r.truncated}
	var runErr *domain.WorkInfoRunError
	if errors.As(r.err, &runErr) {
		log.Failure = runErr.Failure
	}
	if record != nil {
		r.recordErrs = append(r.recordErrs, record(log))
	}
	return r.result, log, r.err
}

type fakeVideos map[int64]bool

func (v fakeVideos) GetVideo(_ context.Context, _ domain.Audience, id int64) (domain.Video, error) {
	if !v[id] {
		return domain.Video{}, domain.ErrNotFound
	}
	return domain.Video{ID: id}, nil
}

// fakeFetchApplier は動画の表示名とタグを持ち、本物の保存先と同じく、プラグインの行を読み直してから書く。
type fakeFetchApplier struct {
	plugins  *runPluginStore
	titles   map[int64]string
	tags     map[int64][]string
	rejected []string
	skipped  []domain.SkippedWorkInfoValue
	err      error
	calls    []domain.FetchedWorkInfoApply
}

func (a *fakeFetchApplier) ApplyFetchedWorkInfo(ctx context.Context, apply domain.FetchedWorkInfoApply) (domain.FetchedWorkInfoApplied, error) {
	a.calls = append(a.calls, apply)
	if a.err != nil {
		return domain.FetchedWorkInfoApplied{}, a.err
	}
	plugin, err := a.plugins.WorkInfoPlugin(ctx, apply.PluginID)
	if err != nil || plugin.State() != domain.WorkInfoPluginApproved || plugin.Digest != apply.Digest ||
		plugin.Generation != apply.Generation {
		return domain.FetchedWorkInfoApplied{}, domain.ErrWorkInfoFetchStale
	}
	if apply.Title != nil {
		a.titles[apply.VideoID] = *apply.Title
	}
	rejected := []string{}
	for _, name := range apply.Tags {
		if slices.Contains(a.rejected, name) {
			rejected = append(rejected, name)
			continue
		}
		a.tags[apply.VideoID] = append(a.tags[apply.VideoID], name)
	}
	video := domain.Video{ID: apply.VideoID, DisplayName: a.titles[apply.VideoID]}
	return domain.FetchedWorkInfoApplied{Video: video, RejectedTags: rejected, Skipped: a.skipped}, nil
}

const (
	fetchVideo = int64(7)
	otherVideo = int64(8)
)

type fetchEnv struct {
	plugins *WorkInfoPlugins
	store   *runPluginStore
	runtime *recordingRuntime
	applier *fakeFetchApplier
	matcher *fakeCreditMatcher
	now     time.Time
}

// newFetchEnv は承認済みのプラグイン 1（設定 apiKey が必須で値あり、lang は任意で値あり）と、別の値を持つ
// プラグイン 2 を持つ。
func newFetchEnv() *fetchEnv {
	settings := []domain.WorkInfoPluginSetting{
		{Key: "apiKey", Label: "API key", Secret: true, Required: true},
		{Key: "lang", Label: "Language"},
	}
	manifest := domain.WorkInfoPluginManifest{
		Contract: 1, Name: "one", Version: "1",
		Operations: []domain.WorkInfoOperation{domain.WorkInfoSearch, domain.WorkInfoFetch}, Settings: settings,
	}
	other := manifest
	other.Name = "two"
	store := &runPluginStore{plugins: map[int64]*domain.WorkInfoPlugin{
		1: {ID: 1, Manifest: manifest, Digest: "d1", ApprovedDigest: "d1",
			Values: map[string]string{"apiKey": "key-one", "lang": "ja", "stale": "left over"}},
		2: {ID: 2, Manifest: other, Digest: "d2", ApprovedDigest: "d2",
			Values: map[string]string{"apiKey": "key-two"}},
	}}
	env := &fetchEnv{store: store, runtime: &recordingRuntime{}, now: time.Unix(1_800_000_000, 0)}
	env.applier = &fakeFetchApplier{plugins: store, titles: map[int64]string{}, tags: map[int64][]string{}}
	env.matcher = &fakeCreditMatcher{}
	env.plugins = NewWorkInfoPlugins(WorkInfoPluginsOptions{
		Store: store, Runtime: env.runtime, Videos: fakeVideos{fetchVideo: true, otherVideo: true}, Applier: env.applier,
		Matcher: env.matcher, Now: func() time.Time { return env.now },
	})
	return env
}

func (e *fetchEnv) fetch(t *testing.T, title string, tags ...string) domain.WorkInfoFetchResult {
	t.Helper()
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: domain.FetchedWorkInfo{Title: title, Tags: tags}}
	e.runtime.err = nil
	result, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "W-1")
	if err != nil {
		t.Fatalf("FetchWorkInfo: %v", err)
	}
	return result
}

func TestSearchWorkInfoReturnsCandidatesWithOnlyThisPluginsSettings(t *testing.T) {
	e := newFetchEnv()
	e.runtime.result = domain.WorkInfoRunResult{Candidates: []domain.WorkInfoCandidate{
		{ID: "W-1", URL: "https://e.test/w/1", Title: "Work", Detail: "2024"},
	}}
	candidates, err := e.plugins.SearchWorkInfo(context.Background(), 1, "  work  ")
	if err != nil || len(candidates) != 1 || candidates[0].Title != "Work" {
		t.Fatalf("SearchWorkInfo = %+v, %v", candidates, err)
	}
	req := e.runtime.requests[0]
	if req.Operation != domain.WorkInfoSearch || req.Query != "work" || e.runtime.digests[0] != "d1" {
		t.Fatalf("request = %+v (digest %s); want a trimmed search with the plugin's digest", req, e.runtime.digests[0])
	}
	if want := map[string]string{"apiKey": "key-one", "lang": "ja"}; !maps.Equal(req.Settings, want) {
		t.Fatalf("settings = %v; want %v and nothing from another plugin or undeclared keys", req.Settings, want)
	}
	if _, err := e.plugins.SearchWorkInfo(context.Background(), 2, "work"); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"apiKey": "key-two"}; !maps.Equal(e.runtime.requests[1].Settings, want) {
		t.Fatalf("plugin 2 settings = %v; want %v", e.runtime.requests[1].Settings, want)
	}
}

func TestRunRefusesBeforeTheModuleRuns(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(e *fetchEnv)
		query  string
		check  func(error) bool
	}{
		{"missing plugin", func(e *fetchEnv) { delete(e.store.plugins, 1) }, "work",
			func(err error) bool { return errors.Is(err, domain.ErrNotFound) }},
		{"unapproved", func(e *fetchEnv) { e.store.plugins[1].ApprovedDigest = "" }, "work",
			func(err error) bool { return errors.Is(err, domain.ErrWorkInfoPluginNotApproved) }},
		{"replaced after approval", func(e *fetchEnv) { e.store.plugins[1].Digest = "d1b" }, "work",
			func(err error) bool { return errors.Is(err, domain.ErrWorkInfoPluginNotApproved) }},
		{"no search", func(e *fetchEnv) {
			e.store.plugins[1].Manifest.Operations = []domain.WorkInfoOperation{domain.WorkInfoFetch}
		}, "work", func(err error) bool { return errors.Is(err, domain.ErrWorkInfoOperationUnsupported) }},
		{"required setting unset", func(e *fetchEnv) { delete(e.store.plugins[1].Values, "apiKey") }, "work",
			func(err error) bool {
				var missing *domain.WorkInfoSettingsMissingError
				return errors.As(err, &missing) && slices.Equal(missing.Keys, []string{"apiKey"})
			}},
		{"blank query", func(*fetchEnv) {}, "   ",
			func(err error) bool { return errors.Is(err, domain.ErrWorkInfoRunInvalid) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newFetchEnv()
			tc.change(e)
			_, err := e.plugins.SearchWorkInfo(context.Background(), 1, tc.query)
			if !tc.check(err) {
				t.Fatalf("err = %v", err)
			}
			if len(e.runtime.requests) != 0 {
				t.Fatal("the module ran")
			}
		})
	}
}

func TestFetchWorkInfoSendsTheTrimmedIDToFetch(t *testing.T) {
	ctx := context.Background()
	e := newFetchEnv()
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: domain.FetchedWorkInfo{Title: "Title", Code: "ABC-123"}}
	result, err := e.plugins.FetchWorkInfo(ctx, fetchVideo, 1, "  ABC-123  ")
	if err != nil || result.ID == "" || result.Title != "Title" || result.Details.Code != "ABC-123" {
		t.Fatalf("FetchWorkInfo = %+v, %v", result, err)
	}
	req := e.runtime.requests[0]
	if req.Operation != domain.WorkInfoFetch || req.ID != "ABC-123" || req.Settings["apiKey"] != "key-one" {
		t.Fatalf("request = %+v", req)
	}
	if len(e.applier.calls) != 0 {
		t.Fatal("fetching by ID wrote to the video")
	}
	// URL の形の入力も作品 ID としてそのまま渡し、vv が URL として読むことはない（親 Issue #1232 の要件 14）。
	if _, err := e.plugins.FetchWorkInfo(ctx, fetchVideo, 1, " https://example.com/works/1 "); err != nil {
		t.Fatalf("URL-shaped id: err = %v", err)
	}
	if req := e.runtime.requests[1]; req.ID != "https://example.com/works/1" {
		t.Fatalf("URL-shaped id request = %+v", req)
	}
	runs := len(e.runtime.requests)
	if _, err := e.plugins.FetchWorkInfo(ctx, fetchVideo, 1, "   "); !errors.Is(err, domain.ErrWorkInfoRunInvalid) {
		t.Fatalf("blank id: err = %v", err)
	}
	if _, err := e.plugins.FetchWorkInfo(ctx, 999, 1, "ABC-123"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing video: err = %v", err)
	}
	// 検索だけを宣言するプラグインには取得を送らない。
	e.store.plugins[1].Manifest.Operations = []domain.WorkInfoOperation{domain.WorkInfoSearch}
	if _, err := e.plugins.FetchWorkInfo(ctx, fetchVideo, 1, "ABC-123"); !errors.Is(err, domain.ErrWorkInfoOperationUnsupported) {
		t.Fatalf("no fetch: err = %v", err)
	}
	if len(e.runtime.requests) != runs {
		t.Fatal("the module ran for a refused fetch")
	}
}

func TestFetchWorkInfoKeepsTheResultAndLeavesTheVideo(t *testing.T) {
	ctx := context.Background()
	e := newFetchEnv()
	result := e.fetch(t, "  Title  ", "drama", "comedy")
	if result.ID == "" || result.Title != "Title" || !slices.Equal(result.Tags, []string{"drama", "comedy"}) ||
		!result.ExpiresAt.Equal(e.now.Add(10*time.Minute)) {
		t.Fatalf("fetch = %+v", result)
	}
	req := e.runtime.requests[0]
	if req.Operation != domain.WorkInfoFetch || req.ID != "W-1" || req.Settings["apiKey"] != "key-one" {
		t.Fatalf("request = %+v", req)
	}
	if len(e.applier.calls) != 0 {
		t.Fatal("fetching wrote to the video")
	}

	// 表示名の規則に合わない題名は値が無いものになる。
	if bad := e.fetch(t, "bad\ttitle"); bad.Title != "" {
		t.Fatalf("title = %q; want none", bad.Title)
	}
	// 動画が無ければプラグインを動かさない。
	runs := len(e.runtime.requests)
	if _, err := e.plugins.FetchWorkInfo(ctx, 999, 1, "W-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing video: err = %v", err)
	}
	for _, id := range []string{"", "  ", strings.Repeat("i", 201)} {
		if _, err := e.plugins.FetchWorkInfo(ctx, fetchVideo, 1, id); !errors.Is(err, domain.ErrWorkInfoRunInvalid) {
			t.Fatalf("id %.30q: err = %v; want ErrWorkInfoRunInvalid", id, err)
		}
	}
	if len(e.runtime.requests) != runs {
		t.Fatal("the module ran for a refused fetch")
	}
	// 実行の失敗はそのまま返り、結果を持たない。
	e.runtime.err = &domain.WorkInfoRunError{Failure: domain.WorkInfoRunBlockedAddress, Host: "127.0.0.1"}
	var failed *domain.WorkInfoRunError
	if _, err := e.plugins.FetchWorkInfo(ctx, fetchVideo, 1, "W-1"); !errors.As(err, &failed) ||
		failed.Host != "127.0.0.1" {
		t.Fatalf("failed run: err = %v", err)
	}
}

func TestApplyWorkInfoFetchWritesSelectedFieldsAndReportsSkippedTags(t *testing.T) {
	ctx := context.Background()
	e := newFetchEnv()
	e.applier.rejected = []string{"spoiler"}
	result := e.fetch(t, "Title", "drama", " drama ", "spoiler", "bad\x01name", "comedy")

	applied, err := e.plugins.ApplyWorkInfoFetch(ctx, fetchVideo, result.ID,
		[]domain.WorkInfoField{domain.WorkInfoFieldTitle, domain.WorkInfoFieldTags}, nil)
	if err != nil {
		t.Fatalf("ApplyWorkInfoFetch: %v", err)
	}
	if e.applier.titles[fetchVideo] != "Title" || !slices.Equal(e.applier.tags[fetchVideo], []string{"drama", "comedy"}) {
		t.Fatalf("video = %q %v", e.applier.titles[fetchVideo], e.applier.tags[fetchVideo])
	}
	want := []domain.SkippedTag{{Name: "spoiler", Reason: domain.SkippedTagRejected}, {Name: "bad\x01name", Reason: domain.SkippedTagInvalid}}
	if !slices.Equal(applied.SkippedTags, want) {
		t.Fatalf("skipped = %+v; want %+v", applied.SkippedTags, want)
	}
	if e.applier.calls[0].Digest != "d1" || e.applier.calls[0].PluginID != 1 {
		t.Fatalf("apply = %+v; want the fetch's plugin and digest", e.applier.calls[0])
	}
	// 結果は使い切りである。
	if _, err := e.plugins.ApplyWorkInfoFetch(ctx, fetchVideo, result.ID, []domain.WorkInfoField{domain.WorkInfoFieldTitle}, nil); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("second apply: err = %v; want ErrWorkInfoFetchStale", err)
	}
}

func TestApplyWorkInfoFetchTitleOnlyLeavesTags(t *testing.T) {
	e := newFetchEnv()
	result := e.fetch(t, "Title", "drama")
	if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
		[]domain.WorkInfoField{domain.WorkInfoFieldTitle}, nil); err != nil {
		t.Fatal(err)
	}
	if e.applier.calls[0].Tags != nil || len(e.applier.tags[fetchVideo]) != 0 {
		t.Fatalf("title-only apply touched tags: %+v", e.applier.calls[0])
	}
}

func TestApplyWorkInfoFetchRefusals(t *testing.T) {
	title := []domain.WorkInfoField{domain.WorkInfoFieldTitle}
	for _, tc := range []struct {
		name   string
		video  int64
		fields []domain.WorkInfoField
		change func(e *fetchEnv)
		want   error
		keeps  bool
	}{
		{"10 minutes later", fetchVideo, title, func(e *fetchEnv) { e.now = e.now.Add(10 * time.Minute) },
			domain.ErrWorkInfoFetchStale, false},
		{"replaced", fetchVideo, title, func(e *fetchEnv) {
			e.store.plugins[1].Digest, e.store.plugins[1].ApprovedDigest = "d1b", ""
		}, domain.ErrWorkInfoFetchStale, false},
		{"replaced and approved again", fetchVideo, title, func(e *fetchEnv) {
			e.store.plugins[1].Digest, e.store.plugins[1].ApprovedDigest = "d1b", "d1b"
		}, domain.ErrWorkInfoFetchStale, false},
		{"revoked", fetchVideo, title, func(e *fetchEnv) { e.store.plugins[1].ApprovedDigest = "" },
			domain.ErrWorkInfoFetchStale, false},
		// 同じバイト列の差し替えか取り消しのあとに同じダイジェストを再承認すると、行の世代だけが変わる（R-8）。
		{"approved again with the same digest", fetchVideo, title, func(e *fetchEnv) { e.store.plugins[1].Generation++ },
			domain.ErrWorkInfoFetchStale, false},
		{"deleted", fetchVideo, title, func(e *fetchEnv) { delete(e.store.plugins, 1) },
			domain.ErrWorkInfoFetchStale, false},
		{"another video", otherVideo, title, func(*fetchEnv) {}, domain.ErrNotFound, true},
		{"no fields", fetchVideo, nil, func(*fetchEnv) {}, domain.ErrWorkInfoApplyInvalid, true},
		{"tags without a value", fetchVideo, []domain.WorkInfoField{domain.WorkInfoFieldTags}, func(*fetchEnv) {},
			domain.ErrWorkInfoApplyInvalid, true},
		{"write fails", fetchVideo, title, func(e *fetchEnv) { e.applier.err = errors.New("disk full") }, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newFetchEnv()
			result := e.fetch(t, "Title")
			tc.change(e)
			_, err := e.plugins.ApplyWorkInfoFetch(context.Background(), tc.video, result.ID, tc.fields, nil)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
			if e.applier.titles[fetchVideo] != "" || len(e.applier.tags[fetchVideo]) != 0 {
				t.Fatal("a refused apply changed the video")
			}
			// 残る結果は、直したあとで反映できる。
			e.applier.err = nil
			_, err = e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, title, nil)
			if tc.keeps && err != nil {
				t.Fatalf("the result was not kept: %v", err)
			}
			if !tc.keeps && !errors.Is(err, domain.ErrWorkInfoFetchStale) {
				t.Fatalf("retry: err = %v; want ErrWorkInfoFetchStale", err)
			}
		})
	}
}
