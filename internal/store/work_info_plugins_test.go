package store

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの行と設定の値の規則（specs/049-work-info-plugins/data-model.md の Rules）。

var pluginNow = time.Unix(1_800_000_000, 0)

func pluginUpload(name, digest string, keys ...string) domain.WorkInfoPluginUpload {
	settings := make([]domain.WorkInfoPluginSetting, 0, len(keys))
	for _, key := range keys {
		settings = append(settings, domain.WorkInfoPluginSetting{Key: key, Label: key, Secret: key == "apiKey"})
	}
	return domain.WorkInfoPluginUpload{
		FileName: name + ".wasm",
		Module:   []byte("module " + digest),
		Digest:   digest,
		Manifest: domain.WorkInfoPluginManifest{
			Contract: 1, Name: name, Version: "1.0", Description: "d",
			Operations: []domain.WorkInfoOperation{domain.WorkInfoSearch, domain.WorkInfoFetch},
			Settings:   settings,
		},
	}
}

func brokenUpload(fileName, digest string, problem domain.WorkInfoPluginProblem) domain.WorkInfoPluginUpload {
	return domain.WorkInfoPluginUpload{FileName: fileName, Module: []byte("x"), Digest: digest, Problem: problem}
}

func savePlugin(t *testing.T, s *WorkInfoPluginStore, upload domain.WorkInfoPluginUpload) (domain.WorkInfoPlugin, bool) {
	t.Helper()
	plugin, replaced, err := s.SaveWorkInfoPlugin(context.Background(), upload, pluginNow)
	if err != nil {
		t.Fatalf("SaveWorkInfoPlugin(%s): %v", upload.FileName, err)
	}
	return plugin, replaced
}

func setPluginValues(t *testing.T, s *WorkInfoPluginStore, id int64, values map[string]*string) domain.WorkInfoPlugin {
	t.Helper()
	plugin, err := s.SetWorkInfoPluginValues(context.Background(), id, values, pluginNow)
	if err != nil {
		t.Fatalf("SetWorkInfoPluginValues: %v", err)
	}
	return plugin
}

func TestWorkInfoPluginUploadApproveAndReplace(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()

	added, replaced := savePlugin(t, s, pluginUpload("example", "d1"))
	if replaced || added.State() != domain.WorkInfoPluginUnapproved || added.Manifest.Name != "example" ||
		len(added.Manifest.Operations) != 2 {
		t.Fatalf("added = %+v, replaced %v; want a new unapproved plugin", added, replaced)
	}
	approved, err := s.ApproveWorkInfoPlugin(ctx, added.ID, "d1", pluginNow)
	if err != nil || approved.State() != domain.WorkInfoPluginApproved {
		t.Fatalf("Approve = %+v, %v; want approved", approved, err)
	}

	again, replaced := savePlugin(t, s, pluginUpload("example", "d2"))
	if !replaced || again.ID != added.ID || again.Digest != "d2" || again.State() != domain.WorkInfoPluginUnapproved {
		t.Fatalf("replacement = %+v, replaced %v; want the same row unapproved with the new digest", again, replaced)
	}
	if _, err := s.ApproveWorkInfoPlugin(ctx, added.ID, "d1", pluginNow); !errors.Is(err, domain.ErrWorkInfoPluginChanged) {
		t.Fatalf("Approve with the old digest err = %v, want ErrWorkInfoPluginChanged", err)
	}
	if list, err := s.ListWorkInfoPlugins(ctx); err != nil || len(list) != 1 {
		t.Fatalf("List = %d plugins, %v; want the one replaced row", len(list), err)
	}
}

// 差し替えは、ダイジェストが承認したときと同じでも承認前に戻す。同じバイト列の再アップロードと、
// 問題のある差し替えを挟んで承認したバイト列へ戻した差し替えの両方で確かめる。
func TestWorkInfoPluginReplacementWithTheApprovedDigestIsUnapproved(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()
	added, _ := savePlugin(t, s, pluginUpload("example", "d1"))
	approve := func() {
		t.Helper()
		if _, err := s.ApproveWorkInfoPlugin(ctx, added.ID, "d1", pluginNow); err != nil {
			t.Fatalf("Approve: %v", err)
		}
	}

	approve()
	same, replaced := savePlugin(t, s, pluginUpload("example", "d1"))
	if !replaced || same.ID != added.ID || same.State() != domain.WorkInfoPluginUnapproved {
		t.Fatalf("same bytes again = %+v, replaced %v; want the same row unapproved", same, replaced)
	}

	approve()
	broken := brokenUpload("example.wasm", "b1", domain.WorkInfoPluginImportUnsupported)
	broken.Manifest = domain.WorkInfoPluginManifest{Contract: 1, Name: "example", Version: "2.0"}
	invalid, replaced := savePlugin(t, s, broken)
	if !replaced || invalid.ID != added.ID || invalid.State() != domain.WorkInfoPluginInvalid {
		t.Fatalf("broken replacement = %+v, replaced %v; want the same row invalid", invalid, replaced)
	}
	back, replaced := savePlugin(t, s, pluginUpload("example", "d1"))
	if !replaced || back.ID != added.ID || back.State() != domain.WorkInfoPluginUnapproved {
		t.Fatalf("approved bytes back = %+v, replaced %v; want the same row unapproved", back, replaced)
	}
}

func TestWorkInfoPluginInvalidIsListedAndCannotBeApproved(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()
	broken, _ := savePlugin(t, s, brokenUpload("broken.wasm", "b1", domain.WorkInfoPluginNotWasm))
	another, replaced := savePlugin(t, s, brokenUpload("broken.wasm", "b2", domain.WorkInfoPluginNotWasm))
	if replaced || another.ID == broken.ID {
		t.Fatal("an upload without a readable name replaced another row")
	}
	list, err := s.ListWorkInfoPlugins(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if list[0].State() != domain.WorkInfoPluginInvalid || list[0].Problem != domain.WorkInfoPluginNotWasm ||
		list[0].Manifest.Name != "" {
		t.Fatalf("listed = %+v; want invalid with its problem", list[0])
	}
	if _, err := s.ApproveWorkInfoPlugin(ctx, broken.ID, "b1", pluginNow); !errors.Is(err, domain.ErrWorkInfoPluginUnusable) {
		t.Fatalf("Approve err = %v, want ErrWorkInfoPluginUnusable", err)
	}
	if _, err := s.SetWorkInfoPluginValues(ctx, broken.ID, map[string]*string{}, pluginNow); !errors.Is(err, domain.ErrWorkInfoPluginUnusable) {
		t.Fatalf("SetValues err = %v, want ErrWorkInfoPluginUnusable", err)
	}
}

func TestWorkInfoPluginListOrder(t *testing.T) {
	s := migratedDB(t).WorkInfoPlugins()
	savePlugin(t, s, brokenUpload("a.wasm", "x", domain.WorkInfoPluginNotWasm))
	savePlugin(t, s, pluginUpload("zeta", "z"))
	savePlugin(t, s, pluginUpload("alpha", "a"))
	list, err := s.ListWorkInfoPlugins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range list {
		got = append(got, p.Manifest.Name+"/"+p.FileName)
	}
	want := []string{"alpha/alpha.wasm", "zeta/zeta.wasm", "/a.wasm"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestWorkInfoPluginRevokeAndDelete(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()
	added, _ := savePlugin(t, s, pluginUpload("example", "d1", "apiKey"))
	if _, err := s.ApproveWorkInfoPlugin(ctx, added.ID, "d1", pluginNow); err != nil {
		t.Fatal(err)
	}
	revoked, err := s.RevokeWorkInfoPlugin(ctx, added.ID, pluginNow)
	if err != nil || revoked.State() != domain.WorkInfoPluginUnapproved || revoked.ApprovedDigest != "" {
		t.Fatalf("Revoke = %+v, %v", revoked, err)
	}
	setPluginValues(t, s, added.ID, map[string]*string{"apiKey": new("secret")})
	if err := s.DeleteWorkInfoPlugin(ctx, added.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WorkInfoPlugin(ctx, added.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("read after delete err = %v, want ErrNotFound", err)
	}
	var values int
	if err := s.sql.QueryRow(`select count(*) from work_info_plugin_settings`).Scan(&values); err != nil || values != 0 {
		t.Fatalf("setting rows after delete = %d, %v; want 0", values, err)
	}
	if err := s.DeleteWorkInfoPlugin(ctx, added.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete err = %v, want ErrNotFound", err)
	}
	if _, err := s.RevokeWorkInfoPlugin(ctx, added.ID, pluginNow); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoke of a deleted plugin err = %v, want ErrNotFound", err)
	}
}

func TestWorkInfoPluginValuesKeepApproval(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()
	added, _ := savePlugin(t, s, pluginUpload("example", "d1", "apiKey", "lang"))
	approved, err := s.ApproveWorkInfoPlugin(ctx, added.ID, "d1", pluginNow)
	if err != nil {
		t.Fatal(err)
	}
	later := pluginNow.Add(time.Hour)
	written, err := s.SetWorkInfoPluginValues(ctx, added.ID, map[string]*string{"apiKey": new("k"), "lang": new("ja")}, later)
	if err != nil {
		t.Fatal(err)
	}
	if written.State() != domain.WorkInfoPluginApproved || !written.UpdatedAt.Equal(approved.UpdatedAt) {
		t.Fatalf("after writing values = %+v; want still approved with the same updatedAt", written)
	}
	if !maps.Equal(written.Values, map[string]string{"apiKey": "k", "lang": "ja"}) {
		t.Fatalf("values = %v", written.Values)
	}
	// null と空の値は消し、省いたキーは残す。
	cleared := setPluginValues(t, s, added.ID, map[string]*string{"apiKey": nil})
	if !maps.Equal(cleared.Values, map[string]string{"lang": "ja"}) {
		t.Fatalf("values after null = %v", cleared.Values)
	}
	cleared = setPluginValues(t, s, added.ID, map[string]*string{"lang": new("")})
	if len(cleared.Values) != 0 {
		t.Fatalf("values after an empty value = %v", cleared.Values)
	}
	if _, err := s.SetWorkInfoPluginValues(ctx, added.ID, map[string]*string{"other": new("x")}, pluginNow); !errors.Is(err, domain.ErrWorkInfoPluginSettingInvalid) {
		t.Fatalf("undeclared key err = %v, want ErrWorkInfoPluginSettingInvalid", err)
	}
}

func TestWorkInfoPluginReplacementKeepsDeclaredValues(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()
	added, _ := savePlugin(t, s, pluginUpload("example", "d1", "apiKey", "lang"))
	setPluginValues(t, s, added.ID, map[string]*string{"apiKey": new("k"), "lang": new("ja")})

	replaced, _ := savePlugin(t, s, pluginUpload("example", "d2", "apiKey", "region"))
	if !maps.Equal(replaced.Values, map[string]string{"apiKey": "k"}) {
		t.Fatalf("values after replacement = %v; want only the still-declared key", replaced.Values)
	}

	// 問題のある差し替えは宣言を読めていないので、値を全部残す。
	broken := brokenUpload("example.wasm", "d3", domain.WorkInfoPluginImportUnsupported)
	broken.Manifest.Name = "example"
	invalid, wasReplaced := savePlugin(t, s, broken)
	if !wasReplaced || invalid.State() != domain.WorkInfoPluginInvalid || len(invalid.Manifest.Settings) != 0 ||
		len(invalid.Manifest.Operations) != 0 {
		t.Fatalf("broken replacement = %+v, replaced %v", invalid, wasReplaced)
	}
	if !maps.Equal(invalid.Values, map[string]string{"apiKey": "k"}) {
		t.Fatalf("values after a broken replacement = %v; want all kept", invalid.Values)
	}
	var module []byte
	if err := s.sql.QueryRowContext(ctx, `select module from work_info_plugins where id = ?`, added.ID).Scan(&module); err != nil || module != nil {
		t.Fatalf("module of the broken replacement = %v, %v; want null", module, err)
	}
}
