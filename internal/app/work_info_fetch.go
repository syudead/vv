package app

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの検索・取得と、取得の結果の反映（specs/049-work-info-plugins/contracts/screen-api.md、
// data-model.md の Rules、research.md R-8〜R-10・R-13）。実行は WorkInfoPluginRuntime、反映の書き込みは
// FetchedWorkInfoApplier が行う。取得の結果はメモリに WorkInfoFetchLifetime の間だけ、動画とプラグインの
// ダイジェストに結び付けて持つ。取得では動画を変えない。

// WorkInfoVideos は取得の前に動画があるかを確かめる読み出しである。internal/store の *LibraryStore が満たす。
type WorkInfoVideos interface {
	// GetVideo は audience から見た動画 id を返す。無ければ domain.ErrNotFound を返す。
	GetVideo(ctx context.Context, audience domain.Audience, id int64) (domain.Video, error)
}

// FetchedWorkInfoApplier は取得の結果を動画に書く保存先である。internal/store の *OverrideStore が満たす。
type FetchedWorkInfoApplier interface {
	// ApplyFetchedWorkInfo は選んだ題名を表示名に、選んだタグを仮のタグとして 1 つの取引で書き、反映後の
	// 動画と、却下した名前なので付けなかったタグの名前を返す。取引の中でプラグインの行がまだあり、承認済みで、
	// ダイジェストが変わっていないことを読み直し、外れれば domain.ErrWorkInfoFetchStale で何も書かない。
	// 動画が無ければ domain.ErrNotFound を返す。
	ApplyFetchedWorkInfo(ctx context.Context, apply domain.FetchedWorkInfoApply) (domain.Video, []string, error)
}

// SearchWorkInfo は承認済みのプラグイン id の search で語 query を探し、候補を返す。語は前後の空白を除いて
// 1〜200 文字でなければ domain.ErrWorkInfoRunInvalid を返す。実行の前の誤りと失敗は run と同じである。
func (p *WorkInfoPlugins) SearchWorkInfo(ctx context.Context, id int64, query string) ([]domain.WorkInfoCandidate, error) {
	normalized, err := domain.NormalizeWorkInfoSearchQuery(query)
	if err != nil {
		return nil, err
	}
	_, result, err := p.run(ctx, id, domain.WorkInfoRunRequest{Operation: domain.WorkInfoSearch, Query: normalized})
	if err != nil {
		return nil, err
	}
	candidates := result.Candidates
	if candidates == nil {
		candidates = []domain.WorkInfoCandidate{}
	}
	return candidates, nil
}

// FetchWorkInfo は承認済みのプラグイン pluginID の fetch で作品の URL rawURL から作品情報を取り、動画
// videoID に結び付けてメモリに持ち、画面に見せる形を返す。動画は変えない。URL が規則に合わなければ
// domain.ErrWorkInfoRunInvalid、動画が無ければ domain.ErrNotFound（プラグインより先に確かめる）を返す。
// 実行の前の誤りと失敗は run と同じである。
func (p *WorkInfoPlugins) FetchWorkInfo(
	ctx context.Context, videoID, pluginID int64, rawURL string,
) (domain.WorkInfoFetchResult, error) {
	if err := domain.CheckWorkInfoFetchURL(rawURL); err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	if p.videos == nil || p.applier == nil {
		return domain.WorkInfoFetchResult{}, errors.New("work-info fetches are not configured")
	}
	if _, err := p.videos.GetVideo(ctx, domain.AudienceOwner, videoID); err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	plugin, result, err := p.run(ctx, pluginID, domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, URL: rawURL})
	if err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	title, _, err := domain.NormalizeDisplayName(result.WorkInfo.Title)
	if err != nil {
		// 表示名の規則に合わない題名は値が無いものとして扱い、選べなくする（contracts/screen-api.md）。
		title = ""
	}
	tags := result.WorkInfo.Tags
	if tags == nil {
		tags = []string{}
	}
	fetch := &workInfoFetch{
		videoID: videoID, pluginID: plugin.ID, digest: plugin.Digest, generation: plugin.Generation,
		title: title, tags: tags,
		expiresAt: p.now().Add(domain.WorkInfoFetchLifetime),
	}
	id := p.fetches.add(fetch, p.now())
	return domain.WorkInfoFetchResult{ID: id, ExpiresAt: fetch.expiresAt, Title: title, Tags: tags}, nil
}

// run はプラグイン id を確かめて要求 req で 1 回動かし、プラグインと結果を返す。無ければ domain.ErrNotFound、
// 承認済みでなければ domain.ErrWorkInfoPluginNotApproved、操作を宣言しなければ
// domain.ErrWorkInfoOperationUnsupported、必須の設定の値が無ければ *domain.WorkInfoSettingsMissingError を
// 返し、どれもモジュールを動かさない。要求の settings にはこのプラグインの宣言にあって値のあるものだけを
// 入れる（research.md R-13）。実行の失敗は *domain.WorkInfoRunError である。
func (p *WorkInfoPlugins) run(
	ctx context.Context, id int64, req domain.WorkInfoRunRequest,
) (domain.WorkInfoPlugin, domain.WorkInfoRunResult, error) {
	plugin, module, err := p.store.WorkInfoPluginModule(ctx, id)
	if err != nil {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, err
	}
	if plugin.State() != domain.WorkInfoPluginApproved {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, domain.ErrWorkInfoPluginNotApproved
	}
	if !plugin.Manifest.Supports(req.Operation) {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, domain.ErrWorkInfoOperationUnsupported
	}
	if missing := domain.MissingWorkInfoPluginSettings(plugin.Manifest.Settings, plugin.Values); len(missing) > 0 {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, &domain.WorkInfoSettingsMissingError{Keys: missing}
	}
	req.Settings = domain.DeclaredWorkInfoPluginValues(plugin.Manifest.Settings, plugin.Values)
	result, err := p.runtime.Run(ctx, plugin.Digest, module, req)
	if err != nil {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, err
	}
	return plugin, result, nil
}

// ApplyWorkInfoFetch は動画 videoID に、取得の結果 fetchID のうち fields の項目を反映し、反映後の動画と
// 付けなかったタグの名前を返す。結果は使い切りで、成功すると消える。
//
// 結果が無いか期限切れなら domain.ErrWorkInfoFetchStale、結果が別の動画のものなら domain.ErrNotFound、
// fields が空か知らない項目か結果に値の無い項目を含めば domain.ErrWorkInfoApplyInvalid を返す。取得の後に
// プラグインが差し替えられたか、承認を取り消されたか、消されていれば domain.ErrWorkInfoFetchStale で、
// 保存先も取引の中で同じことを確かめ直す（research.md R-8）。タグはタグ名の規則で整え、外れた名前は
// invalid、保存先が却下した名前として飛ばしたものは rejected として、取得の結果の順に報告する
// （research.md R-9）。失敗した反映は何も書かない。
func (p *WorkInfoPlugins) ApplyWorkInfoFetch(
	ctx context.Context, videoID int64, fetchID string, fields []domain.WorkInfoField,
) (domain.WorkInfoApplied, error) {
	if p.applier == nil {
		return domain.WorkInfoApplied{}, errors.New("work-info fetches are not configured")
	}
	fetch, err := p.fetches.claim(fetchID, videoID, fields, p.now())
	if err != nil {
		return domain.WorkInfoApplied{}, err
	}
	applied, err := p.applyFetch(ctx, fetch, fields)
	if err != nil && !errors.Is(err, domain.ErrWorkInfoFetchStale) && !errors.Is(err, domain.ErrNotFound) {
		// 書き込みの一時的な失敗では、所有者がもう一度反映できるように結果を戻す。
		p.fetches.restore(fetchID, fetch)
	}
	return applied, err
}

func (p *WorkInfoPlugins) applyFetch(
	ctx context.Context, fetch *workInfoFetch, fields []domain.WorkInfoField,
) (domain.WorkInfoApplied, error) {
	plugin, err := p.store.WorkInfoPlugin(ctx, fetch.pluginID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return domain.WorkInfoApplied{}, domain.ErrWorkInfoFetchStale
	case err != nil:
		return domain.WorkInfoApplied{}, err
	case plugin.State() != domain.WorkInfoPluginApproved || plugin.Digest != fetch.digest ||
		plugin.Generation != fetch.generation:
		return domain.WorkInfoApplied{}, domain.ErrWorkInfoFetchStale
	}
	apply := domain.FetchedWorkInfoApply{
		VideoID: fetch.videoID, PluginID: fetch.pluginID, Digest: fetch.digest, Generation: fetch.generation,
	}
	if slices.Contains(fields, domain.WorkInfoFieldTitle) {
		title := fetch.title
		apply.Title = &title
	}
	var invalid map[string]bool
	if slices.Contains(fields, domain.WorkInfoFieldTags) {
		apply.Tags, invalid = splitTagNames(fetch.tags)
	}
	video, rejected, err := p.applier.ApplyFetchedWorkInfo(ctx, apply)
	if err != nil {
		return domain.WorkInfoApplied{}, err
	}
	return domain.WorkInfoApplied{Video: video, SkippedTags: skippedTags(fetch.tags, invalid, rejected)}, nil
}

// splitTagNames は取得したタグの名前を domain.NormalizeTagName で整え、整えた名前（重複なし）と、規則に
// 合わない元の名前の集合を返す。
func splitTagNames(names []string) ([]string, map[string]bool) {
	valid := make([]string, 0, len(names))
	invalid := map[string]bool{}
	for _, name := range names {
		normalized, err := domain.NormalizeTagName(name)
		if err != nil {
			invalid[name] = true
			continue
		}
		if !slices.Contains(valid, normalized) {
			valid = append(valid, normalized)
		}
	}
	return valid, invalid
}

// skippedTags は付けなかったタグの名前を、取得の結果の順に重複なく返す。規則に合わない名前は元の名前で
// invalid、却下した名前は整えた名前で rejected である。
func skippedTags(names []string, invalid map[string]bool, rejected []string) []domain.SkippedTag {
	out := []domain.SkippedTag{}
	seen := map[string]bool{}
	add := func(name string, reason domain.SkippedTagReason) {
		if !seen[name] {
			seen[name] = true
			out = append(out, domain.SkippedTag{Name: name, Reason: reason})
		}
	}
	for _, name := range names {
		if invalid[name] {
			add(name, domain.SkippedTagInvalid)
			continue
		}
		normalized, err := domain.NormalizeTagName(name)
		if err == nil && slices.Contains(rejected, normalized) {
			add(normalized, domain.SkippedTagRejected)
		}
	}
	return out
}

// workInfoFetch はメモリに持つ取得の結果である。title は表示名の規則を通った題名で、空は値が無い。
// digest と generation は取得した時のプラグインの行の値である（research.md R-8）。
type workInfoFetch struct {
	videoID    int64
	pluginID   int64
	digest     string
	generation int64
	title      string
	tags       []string
	expiresAt  time.Time
}

// hasValue は結果に項目 field の値があるかを返す。知らない項目は偽である。
func (f *workInfoFetch) hasValue(field domain.WorkInfoField) bool {
	switch field {
	case domain.WorkInfoFieldTitle:
		return f.title != ""
	case domain.WorkInfoFieldTags:
		return len(f.tags) > 0
	}
	return false
}

// workInfoFetches は取得の結果を識別子で持つ。期限の過ぎた結果は、追加と取り出しのたびに捨てる。
type workInfoFetches struct {
	mu      sync.Mutex
	entries map[string]*workInfoFetch
}

func newWorkInfoFetches() *workInfoFetches {
	return &workInfoFetches{entries: map[string]*workInfoFetch{}}
}

// add は結果 fetch を持ち、その識別子（128 ビットの乱数）を返す。
func (f *workInfoFetches) add(fetch *workInfoFetch, now time.Time) string {
	id := rand.Text()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	f.entries[id] = fetch
	return id
}

// claim は識別子 id の結果を確かめて取り出す。無いか期限切れなら domain.ErrWorkInfoFetchStale、別の動画の
// ものなら domain.ErrNotFound、fields が空か値の無い項目を含めば domain.ErrWorkInfoApplyInvalid を返し、
// どれも結果を残す（期限切れは捨てる）。同じ結果を 2 回反映しないよう、確かめた結果は取り出す。
func (f *workInfoFetches) claim(
	id string, videoID int64, fields []domain.WorkInfoField, now time.Time,
) (*workInfoFetch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	fetch, ok := f.entries[id]
	switch {
	case !ok:
		return nil, domain.ErrWorkInfoFetchStale
	case fetch.videoID != videoID:
		return nil, domain.ErrNotFound
	case len(fields) == 0:
		return nil, domain.ErrWorkInfoApplyInvalid
	}
	for _, field := range fields {
		if !fetch.hasValue(field) {
			return nil, domain.ErrWorkInfoApplyInvalid
		}
	}
	delete(f.entries, id)
	return fetch, nil
}

// restore は取り出した結果を戻す。期限が過ぎていれば、次の追加か取り出しで捨てる。
func (f *workInfoFetches) restore(id string, fetch *workInfoFetch) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[id] = fetch
}

// prune は期限の過ぎた結果を捨てる。呼び出し側が mu を持つ。
func (f *workInfoFetches) prune(now time.Time) {
	for id, fetch := range f.entries {
		if !now.Before(fetch.expiresAt) {
			delete(f.entries, id)
		}
	}
}
