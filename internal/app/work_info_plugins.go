package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Runtime inspection and store persistence handle plugin registration and settings. Public entries omit secret values through domain.WorkInfoPlugin.Entry.

// Use a fallback filename so plugins without readable names remain identifiable.
const workInfoPluginDefaultFileName = "plugin.wasm"

// WorkInfoPluginStore persists plugin rows and settings; implemented by store.WorkInfoPluginStore.
type WorkInfoPluginStore interface {
	// ListWorkInfoPlugins orders by name, then filename.
	ListWorkInfoPlugins(ctx context.Context) ([]domain.WorkInfoPlugin, error)
	// WorkInfoPlugin returns ErrNotFound when absent.
	WorkInfoPlugin(ctx context.Context, id int64) (domain.WorkInfoPlugin, error)
	// WorkInfoPluginModule returns the row and module bytes, or ErrNotFound.
	WorkInfoPluginModule(ctx context.Context, id int64) (domain.WorkInfoPlugin, []byte, error)
	// SaveWorkInfoPlugin returns the saved row and whether it replaced a same-name row.
	SaveWorkInfoPlugin(ctx context.Context, upload domain.WorkInfoPluginUpload, now time.Time) (domain.WorkInfoPlugin, bool, error)
	// ApproveWorkInfoPlugin rechecks the row transactionally: absent returns ErrNotFound, unusable ErrWorkInfoPluginUnusable, digest mismatch ErrWorkInfoPluginChanged.
	ApproveWorkInfoPlugin(ctx context.Context, id int64, digest string, now time.Time) (domain.WorkInfoPlugin, error)
	// RevokeWorkInfoPlugin returns ErrNotFound when absent.
	RevokeWorkInfoPlugin(ctx context.Context, id int64, now time.Time) (domain.WorkInfoPlugin, error)
	// SetWorkInfoPluginValues revalidates transactionally like approval; unknown keys or oversized values additionally return ErrWorkInfoPluginSettingInvalid.
	SetWorkInfoPluginValues(ctx context.Context, id int64, values map[string]*string, now time.Time) (domain.WorkInfoPlugin, error)
	// DeleteWorkInfoPlugin removes the plugin and settings, or returns ErrNotFound.
	DeleteWorkInfoPlugin(ctx context.Context, id int64) error
	// SaveWorkInfoPluginRun replaces the latest log only if the row exists and its generation matches run.Generation.
	SaveWorkInfoPluginRun(ctx context.Context, run domain.WorkInfoPluginRun) error
	// WorkInfoPluginRun returns the latest run and its presence, or ErrNotFound for a missing plugin.
	WorkInfoPluginRun(ctx context.Context, id int64) (domain.WorkInfoPluginRun, bool, error)
}

type WorkInfoPluginsOptions struct {
	Store   WorkInfoPluginStore
	Runtime WorkInfoPluginRuntime
	// Check video existence before fetching.
	Videos WorkInfoVideos

	Applier FetchedWorkInfoApplier

	Matcher WorkInfoCreditMatcher
	// Read network settings before each run; nil means direct access.
	Network WorkInfoRunNetworkSource

	Covers WorkInfoCoverApplier
	// PersonImages gives linked people without a face their fetched images; nil drops them.
	PersonImages WorkInfoPersonImageFiller
	// People serves the person page's fetches; nil leaves them unconfigured.
	People PersonPluginPeople
	// Now defaults to time.Now.
	Now func() time.Time
}

// WorkInfoRunNetworkSource supplies per-run networking; implemented by WorkInfoPluginProxy.
type WorkInfoRunNetworkSource interface {
	RunNetwork(ctx context.Context) (domain.WorkInfoRunNetwork, error)
}

// WorkInfoPlugins manages plugin registration.
type WorkInfoPlugins struct {
	store   WorkInfoPluginStore
	runtime WorkInfoPluginRuntime
	videos  WorkInfoVideos
	applier FetchedWorkInfoApplier
	matcher WorkInfoCreditMatcher
	network WorkInfoRunNetworkSource
	covers  WorkInfoCoverApplier
	// personImages may be nil.
	personImages WorkInfoPersonImageFiller
	people       PersonPluginPeople
	now          func() time.Time
	fetches      *workInfoFetches
}

// NewWorkInfoPlugins creates the plugin use case.
func NewWorkInfoPlugins(opts WorkInfoPluginsOptions) *WorkInfoPlugins {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &WorkInfoPlugins{
		store: opts.Store, runtime: opts.Runtime, videos: opts.Videos, applier: opts.Applier, matcher: opts.Matcher,
		network: opts.Network, covers: opts.Covers, personImages: opts.PersonImages, people: opts.People,
		now: now, fetches: newWorkInfoFetches(),
	}
}

// ListWorkInfoPlugins omits secret setting values.
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

// UploadWorkInfoPlugin inspects and saves modules, retaining unusability issues; the boolean indicates a new row rather than same-name replacement. Oversized modules return ErrWorkInfoPluginTooLarge without writes.
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

// ApproveWorkInfoPlugin approves the observed digest, returning ErrNotFound, ErrWorkInfoPluginUnusable or ErrWorkInfoPluginChanged as applicable. Storage repeats validation within its transaction.
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

// RevokeWorkInfoPluginApproval returns ErrNotFound when absent.
func (p *WorkInfoPlugins) RevokeWorkInfoPluginApproval(ctx context.Context, id int64) (domain.WorkInfoPluginEntry, error) {
	return entryOf(p.store.RevokeWorkInfoPlugin(ctx, id, p.now()))
}

// SetWorkInfoPluginSettings clears nil/empty values, preserves omitted keys and leaves approval unchanged. Missing/unusable plugins or undeclared/oversized values return ErrNotFound, ErrWorkInfoPluginUnusable or ErrWorkInfoPluginSettingInvalid without writes.
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

// DeleteWorkInfoPlugin removes the plugin and settings, or returns ErrNotFound.
func (p *WorkInfoPlugins) DeleteWorkInfoPlugin(ctx context.Context, id int64) error {
	return p.store.DeleteWorkInfoPlugin(ctx, id)
}

// WorkInfoPluginLastRun returns retained stderr with secrets redacted; replacement discards earlier-module logs. Missing plugins return ErrNotFound.
func (p *WorkInfoPlugins) WorkInfoPluginLastRun(ctx context.Context, id int64) (domain.WorkInfoPluginRun, bool, error) {
	return p.store.WorkInfoPluginRun(ctx, id)
}

// Redact the execution's secret values before persisting logs, even after caller cancellation. The runtime holds its slot through recording so logs follow completion order.
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
