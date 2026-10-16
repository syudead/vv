package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの追加・差し替え・承認・取り消し・削除と、設定の値の書き込み
// （specs/049-work-info-plugins/data-model.md の Rules、research.md R-6・R-7・R-13、
// contracts/screen-api.md）。モジュールの検査は WorkInfoPluginRuntime、保存は WorkInfoPluginStore が行う。
// 秘密の設定の値は、このパッケージの外へは返さない（domain.WorkInfoPlugin.Entry で落とす）。

// workInfoPluginDefaultFileName は、ファイル名の無いアップロードに付ける名前である。名前を読めない
// プラグインの一覧はファイル名で見分けるので、空のままにしない。
const workInfoPluginDefaultFileName = "plugin.wasm"

// WorkInfoPluginStore は作品情報プラグインの行と設定の値の保存先である。internal/store の
// *WorkInfoPluginStore が満たす。
type WorkInfoPluginStore interface {
	// ListWorkInfoPlugins はすべてのプラグインを、名前、次にファイル名の順で返す。
	ListWorkInfoPlugins(ctx context.Context) ([]domain.WorkInfoPlugin, error)
	// WorkInfoPlugin はプラグイン id を返す。無ければ domain.ErrNotFound を返す。
	WorkInfoPlugin(ctx context.Context, id int64) (domain.WorkInfoPlugin, error)
	// WorkInfoPluginModule はプラグイン id の行と、そのモジュールのバイト列を返す。無ければ
	// domain.ErrNotFound を返す。
	WorkInfoPluginModule(ctx context.Context, id int64) (domain.WorkInfoPlugin, []byte, error)
	// SaveWorkInfoPlugin はアップロードを保存し、保存した行と、同じ名前の行を差し替えたかを返す。
	SaveWorkInfoPlugin(ctx context.Context, upload domain.WorkInfoPluginUpload, now time.Time) (domain.WorkInfoPlugin, bool, error)
	// ApproveWorkInfoPlugin はダイジェスト digest を承認する。取引の中で読み直し、無ければ
	// domain.ErrNotFound、問題があれば domain.ErrWorkInfoPluginUnusable、ダイジェストが違えば
	// domain.ErrWorkInfoPluginChanged を返す。
	ApproveWorkInfoPlugin(ctx context.Context, id int64, digest string, now time.Time) (domain.WorkInfoPlugin, error)
	// RevokeWorkInfoPlugin は承認を取り消す。無ければ domain.ErrNotFound を返す。
	RevokeWorkInfoPlugin(ctx context.Context, id int64, now time.Time) (domain.WorkInfoPlugin, error)
	// SetWorkInfoPluginValues は設定の値を書く。取引の中で読み直し、ApproveWorkInfoPlugin と同じ
	// 誤りに加えて、宣言に無いキーか長すぎる値なら domain.ErrWorkInfoPluginSettingInvalid を返す。
	SetWorkInfoPluginValues(ctx context.Context, id int64, values map[string]*string, now time.Time) (domain.WorkInfoPlugin, error)
	// DeleteWorkInfoPlugin はプラグインとその設定の値を消す。無ければ domain.ErrNotFound を返す。
	DeleteWorkInfoPlugin(ctx context.Context, id int64) error
	// SaveWorkInfoPluginRun はプラグインの直近の実行 run を、前の実行に替えて書く。プラグインの行が無いか
	// 世代が run.Generation でなければ何も書かない。
	SaveWorkInfoPluginRun(ctx context.Context, run domain.WorkInfoPluginRun) error
	// WorkInfoPluginRun はプラグイン id の直近の実行と、それが残っているかを返す。プラグインが無ければ
	// domain.ErrNotFound を返す。
	WorkInfoPluginRun(ctx context.Context, id int64) (domain.WorkInfoPluginRun, bool, error)
}

// WorkInfoPluginsOptions は WorkInfoPlugins の依存である。
type WorkInfoPluginsOptions struct {
	Store   WorkInfoPluginStore
	Runtime WorkInfoPluginRuntime
	// Videos は取得の前に動画があるかを確かめる読み出しである。
	Videos WorkInfoVideos
	// Applier は取得の結果を動画に書く保存先である。
	Applier FetchedWorkInfoApplier
	// Matcher は取得した人物・団体を既存の実体と照合する読み出しである。
	Matcher WorkInfoCreditMatcher
	// Now は今の時刻を返す。nil なら time.Now。
	Now func() time.Time
}

// WorkInfoPlugins は作品情報プラグインの登録を扱うユースケースである。
type WorkInfoPlugins struct {
	store   WorkInfoPluginStore
	runtime WorkInfoPluginRuntime
	videos  WorkInfoVideos
	applier FetchedWorkInfoApplier
	matcher WorkInfoCreditMatcher
	now     func() time.Time
	fetches *workInfoFetches
}

// NewWorkInfoPlugins は WorkInfoPlugins を作る。
func NewWorkInfoPlugins(opts WorkInfoPluginsOptions) *WorkInfoPlugins {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &WorkInfoPlugins{
		store: opts.Store, runtime: opts.Runtime, videos: opts.Videos, applier: opts.Applier, matcher: opts.Matcher, now: now,
		fetches: newWorkInfoFetches(),
	}
}

// ListWorkInfoPlugins はすべてのプラグインを、秘密の設定の値を落とした形で返す。
func (p *WorkInfoPlugins) ListWorkInfoPlugins(ctx context.Context) ([]domain.WorkInfoPluginEntry, error) {
	plugins, err := p.store.ListWorkInfoPlugins(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]domain.WorkInfoPluginEntry, 0, len(plugins))
	for _, plugin := range plugins {
		entries = append(entries, plugin.Entry())
	}
	return entries, nil
}

// UploadWorkInfoPlugin はファイル名 fileName のモジュール module を検査して保存し、保存したプラグインと、
// 新しい行か（偽なら同じ名前の行を差し替えた）を返す。使えないモジュールも問題を付けて保存する。
// モジュールが domain.WorkInfoPluginModuleMaxBytes を超えれば domain.ErrWorkInfoPluginTooLarge を返し、
// 何も書かない。
func (p *WorkInfoPlugins) UploadWorkInfoPlugin(
	ctx context.Context, fileName string, module []byte,
) (domain.WorkInfoPluginEntry, bool, error) {
	if len(module) > domain.WorkInfoPluginModuleMaxBytes {
		return domain.WorkInfoPluginEntry{}, false, domain.ErrWorkInfoPluginTooLarge
	}
	manifest, problem, err := p.runtime.Inspect(ctx, module)
	if err != nil {
		return domain.WorkInfoPluginEntry{}, false, err
	}
	if fileName == "" {
		fileName = workInfoPluginDefaultFileName
	}
	sum := sha256.Sum256(module)
	saved, replaced, err := p.store.SaveWorkInfoPlugin(ctx, domain.WorkInfoPluginUpload{
		FileName: fileName,
		Module:   module,
		Digest:   hex.EncodeToString(sum[:]),
		Manifest: manifest,
		Problem:  problem,
	}, p.now())
	if err != nil {
		return domain.WorkInfoPluginEntry{}, false, err
	}
	return saved.Entry(), !replaced, nil
}

// ApproveWorkInfoPlugin は、所有者が一覧で見たダイジェスト digest のモジュールを承認する
// （research.md R-7）。無ければ domain.ErrNotFound、問題があれば domain.ErrWorkInfoPluginUnusable、
// 今のモジュールのダイジェストと違えば domain.ErrWorkInfoPluginChanged を返す。保存先は取引の中で
// 同じことを確かめ直す。
func (p *WorkInfoPlugins) ApproveWorkInfoPlugin(ctx context.Context, id int64, digest string) (domain.WorkInfoPluginEntry, error) {
	plugin, err := p.store.WorkInfoPlugin(ctx, id)
	if err != nil {
		return domain.WorkInfoPluginEntry{}, err
	}
	switch {
	case plugin.Problem != "":
		return domain.WorkInfoPluginEntry{}, domain.ErrWorkInfoPluginUnusable
	case plugin.Digest != digest:
		return domain.WorkInfoPluginEntry{}, domain.ErrWorkInfoPluginChanged
	}
	return entryOf(p.store.ApproveWorkInfoPlugin(ctx, id, digest, p.now()))
}

// RevokeWorkInfoPluginApproval はプラグイン id の承認を取り消す。無ければ domain.ErrNotFound を返す。
func (p *WorkInfoPlugins) RevokeWorkInfoPluginApproval(ctx context.Context, id int64) (domain.WorkInfoPluginEntry, error) {
	return entryOf(p.store.RevokeWorkInfoPlugin(ctx, id, p.now()))
}

// SetWorkInfoPluginSettings はプラグイン id の設定の値を書く。nil か空の値はそのキーの値を消し、
// values に無いキーの値はそのまま残す。承認は変わらない（research.md R-13）。無ければ
// domain.ErrNotFound、問題があれば domain.ErrWorkInfoPluginUnusable、マニフェストが宣言しないキーか
// domain.WorkInfoPluginSettingValueMaxRunes 文字を超える値があれば domain.ErrWorkInfoPluginSettingInvalid
// を返し、何も書かない。
func (p *WorkInfoPlugins) SetWorkInfoPluginSettings(
	ctx context.Context, id int64, values map[string]*string,
) (domain.WorkInfoPluginEntry, error) {
	plugin, err := p.store.WorkInfoPlugin(ctx, id)
	if err != nil {
		return domain.WorkInfoPluginEntry{}, err
	}
	if plugin.Problem != "" {
		return domain.WorkInfoPluginEntry{}, domain.ErrWorkInfoPluginUnusable
	}
	if err := domain.CheckWorkInfoPluginValues(plugin.Manifest.Settings, values); err != nil {
		return domain.WorkInfoPluginEntry{}, err
	}
	return entryOf(p.store.SetWorkInfoPluginValues(ctx, id, values, p.now()))
}

// DeleteWorkInfoPlugin はプラグイン id とその設定の値を消す。無ければ domain.ErrNotFound を返す。
func (p *WorkInfoPlugins) DeleteWorkInfoPlugin(ctx context.Context, id int64) error {
	return p.store.DeleteWorkInfoPlugin(ctx, id)
}

// WorkInfoPluginLastRun はプラグイン id の直近の実行の標準エラー（秘密の値を伏せたもの）と、それが
// 残っているかを返す。差し替えたプラグインは、差し替えの後の実行だけが残る。無ければ domain.ErrNotFound を返す
// （specs/056-plugin-tooling/contracts/screen-api.md）。
func (p *WorkInfoPlugins) WorkInfoPluginLastRun(ctx context.Context, id int64) (domain.WorkInfoPluginRun, bool, error) {
	return p.store.WorkInfoPluginRun(ctx, id)
}

// recordRun は、プラグイン plugin の 1 回の実行のログを保存する関数を返す。ログの標準エラーのうち、その実行が
// 受け取った秘密の値 secrets を伏せてから書く。実行の枠を持っている間に呼ばれるので、行は実行が終わった順に
// 書かれる。呼び出し元の ctx が終わっても書く（specs/056-plugin-tooling/research.md R-6）。
func (p *WorkInfoPlugins) recordRun(
	ctx context.Context, plugin domain.WorkInfoPlugin, secrets []string,
) func(domain.WorkInfoRunLog) error {
	return func(log domain.WorkInfoRunLog) error {
		return p.store.SaveWorkInfoPluginRun(context.WithoutCancel(ctx), domain.WorkInfoPluginRun{
			PluginID: plugin.ID, Generation: plugin.Generation, RanAt: p.now(),
			WorkInfoRunLog: domain.MaskWorkInfoRunLog(log, secrets),
		})
	}
}

func entryOf(plugin domain.WorkInfoPlugin, err error) (domain.WorkInfoPluginEntry, error) {
	if err != nil {
		return domain.WorkInfoPluginEntry{}, err
	}
	return plugin.Entry(), nil
}
