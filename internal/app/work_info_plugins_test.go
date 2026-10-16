package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの登録の判断（specs/049-work-info-plugins/data-model.md の Rules、research.md R-7・R-13）。
// 保存の規則（差し替え・値の引き継ぎ・削除の連鎖）は internal/store のテストが本物の SQLite で確かめる。

// fakePluginRuntime はモジュールのバイト列をマニフェストの名前として読む検査である。"broken" は
// not_wasm にする。
type fakePluginRuntime struct {
	settings []domain.WorkInfoPluginSetting
}

func (r *fakePluginRuntime) Inspect(_ context.Context, module []byte) (domain.WorkInfoPluginManifest, domain.WorkInfoPluginProblem, error) {
	if string(module) == "broken" {
		return domain.WorkInfoPluginManifest{}, domain.WorkInfoPluginNotWasm, nil
	}
	return domain.WorkInfoPluginManifest{
		Contract: 1, Name: string(module), Version: "1",
		Operations: []domain.WorkInfoOperation{domain.WorkInfoFetch}, Settings: r.settings,
	}, "", nil
}

func (r *fakePluginRuntime) Run(context.Context, string, []byte, domain.WorkInfoRunRequest, domain.WorkInfoRunNetwork) (domain.WorkInfoRunResult, error) {
	return domain.WorkInfoRunResult{}, errors.New("not used")
}

// fakePluginStore は行を 1 つ持つ保存先で、書き込みの呼び出しを数える。
type fakePluginStore struct {
	plugin  *domain.WorkInfoPlugin
	uploads []domain.WorkInfoPluginUpload
	writes  int
}

func (s *fakePluginStore) ListWorkInfoPlugins(context.Context) ([]domain.WorkInfoPlugin, error) {
	if s.plugin == nil {
		return nil, nil
	}
	return []domain.WorkInfoPlugin{*s.plugin}, nil
}

func (s *fakePluginStore) WorkInfoPlugin(_ context.Context, id int64) (domain.WorkInfoPlugin, error) {
	if s.plugin == nil || s.plugin.ID != id {
		return domain.WorkInfoPlugin{}, domain.ErrNotFound
	}
	return *s.plugin, nil
}

func (s *fakePluginStore) WorkInfoPluginModule(ctx context.Context, id int64) (domain.WorkInfoPlugin, []byte, error) {
	plugin, err := s.WorkInfoPlugin(ctx, id)
	return plugin, nil, err
}

func (s *fakePluginStore) SaveWorkInfoPlugin(_ context.Context, upload domain.WorkInfoPluginUpload, now time.Time) (domain.WorkInfoPlugin, bool, error) {
	s.uploads = append(s.uploads, upload)
	replaced := s.plugin != nil && upload.Manifest.Name != "" && s.plugin.Manifest.Name == upload.Manifest.Name
	values := map[string]string{}
	if replaced {
		values = s.plugin.Values
	}
	s.plugin = &domain.WorkInfoPlugin{
		ID: 1, FileName: upload.FileName, Manifest: upload.Manifest, Digest: upload.Digest,
		Problem: upload.Problem, UpdatedAt: now, Values: values,
	}
	return *s.plugin, replaced, nil
}

func (s *fakePluginStore) ApproveWorkInfoPlugin(_ context.Context, _ int64, digest string, _ time.Time) (domain.WorkInfoPlugin, error) {
	s.writes++
	s.plugin.ApprovedDigest = digest
	return *s.plugin, nil
}

func (s *fakePluginStore) RevokeWorkInfoPlugin(ctx context.Context, id int64, _ time.Time) (domain.WorkInfoPlugin, error) {
	s.writes++
	if _, err := s.WorkInfoPlugin(ctx, id); err != nil {
		return domain.WorkInfoPlugin{}, err
	}
	s.plugin.ApprovedDigest = ""
	return *s.plugin, nil
}

func (s *fakePluginStore) SetWorkInfoPluginValues(_ context.Context, _ int64, values map[string]*string, _ time.Time) (domain.WorkInfoPlugin, error) {
	s.writes++
	for key, value := range values {
		s.plugin.Values[key] = *value
	}
	return *s.plugin, nil
}

func (s *fakePluginStore) DeleteWorkInfoPlugin(ctx context.Context, id int64) error {
	s.writes++
	if _, err := s.WorkInfoPlugin(ctx, id); err != nil {
		return err
	}
	s.plugin = nil
	return nil
}

func newTestWorkInfoPlugins(settings ...domain.WorkInfoPluginSetting) (*WorkInfoPlugins, *fakePluginStore) {
	store := &fakePluginStore{}
	plugins := NewWorkInfoPlugins(WorkInfoPluginsOptions{
		Store: store, Runtime: &fakePluginRuntime{settings: settings},
		Now: func() time.Time { return time.Unix(1_800_000_000, 0) },
	})
	return plugins, store
}

func digestOf(module string) string {
	sum := sha256.Sum256([]byte(module))
	return hex.EncodeToString(sum[:])
}

func TestUploadWorkInfoPluginStoresTheInspection(t *testing.T) {
	ctx := context.Background()
	plugins, _ := newTestWorkInfoPlugins()
	entry, created, err := plugins.UploadWorkInfoPlugin(ctx, "", []byte("example"))
	if err != nil || !created {
		t.Fatalf("Upload = %+v, created %v, %v", entry, created, err)
	}
	if entry.State != domain.WorkInfoPluginUnapproved || entry.Name != "example" || entry.Digest != digestOf("example") ||
		entry.FileName != workInfoPluginDefaultFileName {
		t.Fatalf("entry = %+v; want unapproved with the module's digest and the default file name", entry)
	}
	again, created, err := plugins.UploadWorkInfoPlugin(ctx, "new.wasm", []byte("example"))
	if err != nil || created || again.FileName != "new.wasm" {
		t.Fatalf("second upload = %+v, created %v, %v; want a replacement", again, created, err)
	}
	broken, created, err := plugins.UploadWorkInfoPlugin(ctx, "b.wasm", []byte("broken"))
	if err != nil || !created || broken.State != domain.WorkInfoPluginInvalid || broken.Problem != domain.WorkInfoPluginNotWasm ||
		len(broken.Operations) != 0 {
		t.Fatalf("broken = %+v, created %v, %v; want a stored invalid plugin", broken, created, err)
	}
}

func TestUploadWorkInfoPluginOverLimitWritesNothing(t *testing.T) {
	plugins, store := newTestWorkInfoPlugins()
	_, _, err := plugins.UploadWorkInfoPlugin(context.Background(), "big.wasm", make([]byte, domain.WorkInfoPluginModuleMaxBytes+1))
	if !errors.Is(err, domain.ErrWorkInfoPluginTooLarge) || len(store.uploads) != 0 {
		t.Fatalf("err = %v, uploads %d; want ErrWorkInfoPluginTooLarge and nothing stored", err, len(store.uploads))
	}
}

func TestApproveWorkInfoPluginChecksTheDigestAndProblem(t *testing.T) {
	ctx := context.Background()
	plugins, store := newTestWorkInfoPlugins()
	if _, err := plugins.ApproveWorkInfoPlugin(ctx, 1, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("approve of a missing plugin err = %v", err)
	}
	listed, _, _ := plugins.UploadWorkInfoPlugin(ctx, "a.wasm", []byte("example"))
	approved, err := plugins.ApproveWorkInfoPlugin(ctx, listed.ID, listed.Digest)
	if err != nil || approved.State != domain.WorkInfoPluginApproved {
		t.Fatalf("approve = %+v, %v; want approved", approved, err)
	}
	// 一覧を見たあとに差し替えられたモジュールは、見たダイジェストでは承認できない。
	store.plugin.Digest = "replaced"
	writes := store.writes
	if _, err := plugins.ApproveWorkInfoPlugin(ctx, listed.ID, listed.Digest); !errors.Is(err, domain.ErrWorkInfoPluginChanged) {
		t.Fatalf("approve with the old digest err = %v, want ErrWorkInfoPluginChanged", err)
	}
	broken, _, _ := plugins.UploadWorkInfoPlugin(ctx, "b.wasm", []byte("broken"))
	if _, err := plugins.ApproveWorkInfoPlugin(ctx, broken.ID, broken.Digest); !errors.Is(err, domain.ErrWorkInfoPluginUnusable) {
		t.Fatalf("approve of an invalid plugin err = %v, want ErrWorkInfoPluginUnusable", err)
	}
	if store.writes != writes {
		t.Fatal("a refused approval reached the store")
	}
}

func TestSetWorkInfoPluginSettingsChecksDeclarationsAndHidesSecrets(t *testing.T) {
	ctx := context.Background()
	plugins, store := newTestWorkInfoPlugins(
		domain.WorkInfoPluginSetting{Key: "apiKey", Label: "API key", Secret: true, Required: true},
		domain.WorkInfoPluginSetting{Key: "lang", Label: "Language"},
	)
	listed, _, _ := plugins.UploadWorkInfoPlugin(ctx, "a.wasm", []byte("example"))
	if _, err := plugins.ApproveWorkInfoPlugin(ctx, listed.ID, listed.Digest); err != nil {
		t.Fatal(err)
	}
	entry, err := plugins.SetWorkInfoPluginSettings(ctx, listed.ID, map[string]*string{"apiKey": new("secret-value"), "lang": new("ja")})
	if err != nil {
		t.Fatal(err)
	}
	if entry.State != domain.WorkInfoPluginApproved {
		t.Fatalf("state after writing values = %s, want approved", entry.State)
	}
	secret, lang := entry.Settings[0], entry.Settings[1]
	if !secret.Set || secret.Value != "" || !lang.Set || lang.Value != "ja" {
		t.Fatalf("settings = %+v; want the secret only marked set", entry.Settings)
	}
	listedAll, err := plugins.ListWorkInfoPlugins(ctx)
	if err != nil || listedAll[0].Settings[0].Value != "" {
		t.Fatalf("list = %+v, %v; want no secret value", listedAll, err)
	}

	writes := store.writes
	for name, values := range map[string]map[string]*string{
		"undeclared key": {"other": new("x")},
		"too long value": {"lang": new(string(make([]rune, domain.WorkInfoPluginSettingValueMaxRunes+1)))},
	} {
		if _, err := plugins.SetWorkInfoPluginSettings(ctx, listed.ID, values); !errors.Is(err, domain.ErrWorkInfoPluginSettingInvalid) {
			t.Errorf("%s: err = %v, want ErrWorkInfoPluginSettingInvalid", name, err)
		}
	}
	if store.writes != writes {
		t.Fatal("a refused setting reached the store")
	}
}

func TestRevokeAndDeleteWorkInfoPlugin(t *testing.T) {
	ctx := context.Background()
	plugins, _ := newTestWorkInfoPlugins()
	listed, _, _ := plugins.UploadWorkInfoPlugin(ctx, "a.wasm", []byte("example"))
	if _, err := plugins.ApproveWorkInfoPlugin(ctx, listed.ID, listed.Digest); err != nil {
		t.Fatal(err)
	}
	revoked, err := plugins.RevokeWorkInfoPluginApproval(ctx, listed.ID)
	if err != nil || revoked.State != domain.WorkInfoPluginUnapproved {
		t.Fatalf("revoke = %+v, %v", revoked, err)
	}
	if err := plugins.DeleteWorkInfoPlugin(ctx, listed.ID); err != nil {
		t.Fatal(err)
	}
	if err := plugins.DeleteWorkInfoPlugin(ctx, listed.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete err = %v, want ErrNotFound", err)
	}
}
