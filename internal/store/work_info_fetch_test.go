package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの取得の結果の反映（specs/049-work-info-plugins/data-model.md の Rules、research.md R-8・R-9）。

// approvedPlugin は名前 name・ダイジェスト digest のプラグインを保存して承認する。
func approvedPlugin(t *testing.T, db *DB, name, digest string) domain.WorkInfoPlugin {
	t.Helper()
	added, _ := savePlugin(t, db.WorkInfoPlugins(), pluginUpload(name, digest))
	plugin, err := db.WorkInfoPlugins().ApproveWorkInfoPlugin(context.Background(), added.ID, digest, pluginNow)
	if err != nil {
		t.Fatalf("ApproveWorkInfoPlugin: %v", err)
	}
	return plugin
}

func rejectTagName(t *testing.T, db *DB, name string) {
	t.Helper()
	if _, err := db.sql.Exec(`insert into rejected_tag_names (name, created_at) values (?, 0)`, name); err != nil {
		t.Fatal(err)
	}
}

func displayNameOf(t *testing.T, db *DB, id int64) string {
	t.Helper()
	video, err := db.Library().GetVideo(context.Background(), domain.AudienceOwner, id)
	if err != nil {
		t.Fatal(err)
	}
	return video.DisplayName
}

func allTagCount(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	if err := db.sql.QueryRow(`select count(*) from tags`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// 題名とタグを選ぶと、表示名が変わり、タグが仮のタグとして付き、却下した名前は付かずに返る。選んだ動画の
// ほかは変わらない。
func TestApplyFetchedWorkInfoWritesTitleAndTentativeTags(t *testing.T) {
	ctx := context.Background()
	db, ids := overrideFixture(t)
	plugin := approvedPlugin(t, db, "example", "d1")
	rejectTagName(t, db, "spoiler")
	title := "  Fetched title  "

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	applied, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: ids[fixturePath("/media/alpha.mp4")], PluginID: plugin.ID, Digest: "d1", Title: &title,
		Tags: []string{"drama", "spoiler", "comedy"},
	})
	if err != nil {
		t.Fatalf("ApplyFetchedWorkInfo: %v", err)
	}
	video, rejected := applied.Video, applied.RejectedTags
	if video.DisplayName != "Fetched title" || video.Title != "Fetched title" {
		t.Fatalf("video = %q / %q; want the trimmed fetched title", video.DisplayName, video.Title)
	}
	if !slices.Equal(rejected, []string{"spoiler"}) {
		t.Fatalf("rejected = %v; want [spoiler]", rejected)
	}
	got := manualTagNames(t, db, "key-alpha")
	slices.Sort(got)
	if !slices.Equal(got, []string{"comedy", "drama"}) {
		t.Fatalf("tags = %v; want comedy and drama", got)
	}
	for _, name := range []string{"comedy", "drama"} {
		if !tagTentative(t, db, tagIDByName(t, db, name)) {
			t.Fatalf("%s is not tentative", name)
		}
	}
	if tagIDByName(t, db, "spoiler") != 0 {
		t.Fatal("the rejected name became a tag")
	}
	if displayNameOf(t, db, ids[fixturePath("/media/beta.mp4")]) != "" || len(manualTagNames(t, db, "key-beta")) != 0 {
		t.Fatal("another video changed")
	}
	if !slices.Contains(recorder.events, domain.Event(domain.VideoOverrideChanged{VideoID: ids[fixturePath("/media/alpha.mp4")]})) {
		t.Fatalf("events = %v; want VideoOverrideChanged for the video", recorder.events)
	}
}

// 束のメンバーにタグを反映すると、タグは束の鍵に付くので、束の全メンバーに動画の知らせが届く。
// 題名だけなら反映先だけである。
func TestApplyFetchedWorkInfoNotifiesEveryBundleMemberOfTags(t *testing.T) {
	ctx := context.Background()
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	plugin := approvedPlugin(t, db, "example", "d1")

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: ids["b"], PluginID: plugin.ID, Digest: "d1", Tags: []string{"drama"},
	}); err != nil {
		t.Fatalf("ApplyFetchedWorkInfo: %v", err)
	}
	// 適用の記録の知らせ（specs/055-activity/research.md R-6）が確定の直後に先に届く。
	want := []domain.Event{domain.ActivityChanged{}, domain.VideoOverrideChanged{VideoID: ids["b"]},
		domain.VideoOverrideChanged{VideoID: ids["a"]}}
	if !slices.Equal(recorder.events, want) {
		t.Fatalf("events = %v; want %v", recorder.events, want)
	}

	recorder.events = nil
	title := "Fetched"
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: ids["b"], PluginID: plugin.ID, Digest: "d1", Title: &title,
	}); err != nil {
		t.Fatalf("ApplyFetchedWorkInfo: %v", err)
	}
	if want := []domain.Event{domain.ActivityChanged{}, domain.VideoOverrideChanged{VideoID: ids["b"]}}; !slices.Equal(recorder.events, want) {
		t.Fatalf("title-only events = %v; want %v", recorder.events, want)
	}
}

// タグだけを選ぶと表示名は変わらない。
func TestApplyFetchedWorkInfoTagsOnlyKeepsTheDisplayName(t *testing.T) {
	ctx := context.Background()
	db, ids := overrideFixture(t)
	plugin := approvedPlugin(t, db, "example", "d1")
	setDisplayName(t, db, ids[fixturePath("/media/alpha.mp4")], "Mine")
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: ids[fixturePath("/media/alpha.mp4")], PluginID: plugin.ID, Digest: "d1", Tags: []string{"drama"},
	}); err != nil {
		t.Fatal(err)
	}
	if displayNameOf(t, db, ids[fixturePath("/media/alpha.mp4")]) != "Mine" || !slices.Equal(manualTagNames(t, db, "key-alpha"), []string{"drama"}) {
		t.Fatal("tags-only apply changed the display name or did not attach the tag")
	}
}

// app の確認の後で反映の取引の前にプラグインが差し替えられたか、承認を取り消されたか、消されると、
// 取引の中の読み直しで work_info_fetch_stale になり、何も書かない（R-8）。同じダイジェストに戻して再承認しても、
// 行の世代が変わるので古い結果のままである。
func TestApplyFetchedWorkInfoRechecksThePluginInsideTheTransaction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, db *DB, id int64)
	}{
		{"replaced", func(t *testing.T, db *DB, _ int64) {
			t.Helper()
			savePlugin(t, db.WorkInfoPlugins(), pluginUpload("example", "d2"))
		}},
		{"replaced and approved", func(t *testing.T, db *DB, id int64) {
			t.Helper()
			savePlugin(t, db.WorkInfoPlugins(), pluginUpload("example", "d2"))
			if _, err := db.WorkInfoPlugins().ApproveWorkInfoPlugin(context.Background(), id, "d2", pluginNow); err != nil {
				t.Fatal(err)
			}
		}},
		{"replaced with the same bytes and approved again", func(t *testing.T, db *DB, id int64) {
			t.Helper()
			savePlugin(t, db.WorkInfoPlugins(), pluginUpload("example", "d1"))
			if _, err := db.WorkInfoPlugins().ApproveWorkInfoPlugin(context.Background(), id, "d1", pluginNow); err != nil {
				t.Fatal(err)
			}
		}},
		{"revoked and approved again", func(t *testing.T, db *DB, id int64) {
			t.Helper()
			if _, err := db.WorkInfoPlugins().RevokeWorkInfoPlugin(context.Background(), id, pluginNow); err != nil {
				t.Fatal(err)
			}
			if _, err := db.WorkInfoPlugins().ApproveWorkInfoPlugin(context.Background(), id, "d1", pluginNow); err != nil {
				t.Fatal(err)
			}
		}},
		{"revoked", func(t *testing.T, db *DB, id int64) {
			t.Helper()
			if _, err := db.WorkInfoPlugins().RevokeWorkInfoPlugin(context.Background(), id, pluginNow); err != nil {
				t.Fatal(err)
			}
		}},
		{"deleted", func(t *testing.T, db *DB, id int64) {
			t.Helper()
			if err := db.WorkInfoPlugins().DeleteWorkInfoPlugin(context.Background(), id); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, ids := overrideFixture(t)
			plugin := approvedPlugin(t, db, "example", "d1")
			tc.change(t, db, plugin.ID)
			tags := allTagCount(t, db)
			title := "Fetched"
			_, err := db.Overrides().ApplyFetchedWorkInfo(context.Background(), domain.FetchedWorkInfoApply{
				VideoID: ids[fixturePath("/media/alpha.mp4")], PluginID: plugin.ID, Digest: "d1", Generation: plugin.Generation,
				Title: &title, Tags: []string{"drama"},
			})
			if !errors.Is(err, domain.ErrWorkInfoFetchStale) {
				t.Fatalf("err = %v; want ErrWorkInfoFetchStale", err)
			}
			if displayNameOf(t, db, ids[fixturePath("/media/alpha.mp4")]) != "" || len(manualTagNames(t, db, "key-alpha")) != 0 || allTagCount(t, db) != tags {
				t.Fatal("a stale apply wrote something")
			}
		})
	}
}

// 失敗した反映は表示名もタグも残さない。動画が無ければ not_found、題名が規則に合わなければその誤りである。
func TestApplyFetchedWorkInfoFailureWritesNeitherField(t *testing.T) {
	ctx := context.Background()
	db, ids := overrideFixture(t)
	plugin := approvedPlugin(t, db, "example", "d1")
	tags := allTagCount(t, db)

	title := "Fetched"
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: 9999, PluginID: plugin.ID, Digest: "d1", Title: &title, Tags: []string{"drama"},
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing video: err = %v; want ErrNotFound", err)
	}
	bad := "bad\ttitle"
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: ids[fixturePath("/media/alpha.mp4")], PluginID: plugin.ID, Digest: "d1", Title: &bad, Tags: []string{"drama"},
	}); !errors.Is(err, domain.ErrInvalidDisplayName) {
		t.Fatalf("bad title: err = %v; want ErrInvalidDisplayName", err)
	}
	if displayNameOf(t, db, ids[fixturePath("/media/alpha.mp4")]) != "" || len(manualTagNames(t, db, "key-alpha")) != 0 || allTagCount(t, db) != tags {
		t.Fatal("a failed apply wrote something")
	}
}

// 自動タグ付けが提案した付与と同じタグを取り込んでも、提案は確定せずに残る。取り込みは仮の付け方
// だからである（Issue #968 の要件 6、docs/design-docs/auto-tagging.md）。
func TestApplyFetchedWorkInfoKeepsASuggestedTagSuggested(t *testing.T) {
	ctx := context.Background()
	db, ids := overrideFixture(t)
	plugin := approvedPlugin(t, db, "example", "d1")
	tagID := createTagsNamed(t, db, "drama")["drama"]
	if _, err := db.sql.Exec(`insert into video_tags (content_key, tag_id, created_at, tentative) values (?, ?, 0, 1)`,
		"key-alpha", tagID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: ids[fixturePath("/media/alpha.mp4")], PluginID: plugin.ID, Digest: "d1", Tags: []string{"drama", "comedy"},
	}); err != nil {
		t.Fatalf("ApplyFetchedWorkInfo: %v", err)
	}
	if got := tentativeOf(t, db, "key-alpha", tagID); got != 1 {
		t.Fatalf("suggested drama tentative = %d; want 1 (still a suggestion)", got)
	}
	if got := tentativeOf(t, db, "key-alpha", tagIDByName(t, db, "comedy")); got != 0 {
		t.Fatalf("comedy assignment tentative = %d; want 0", got)
	}
}

// 行とモジュールは同じ読み出しで返り、問題のある行のモジュールは無い。
func TestWorkInfoPluginModuleReturnsTheRowAndBytes(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()
	added, _ := savePlugin(t, s, pluginUpload("example", "d1", "lang"))
	plugin, module, err := s.WorkInfoPluginModule(ctx, added.ID)
	if err != nil || plugin.Digest != "d1" || string(module) != "module d1" {
		t.Fatalf("WorkInfoPluginModule = %+v, %q, %v", plugin, module, err)
	}
	broken, _ := savePlugin(t, s, brokenUpload("b.wasm", "d9", domain.WorkInfoPluginNotWasm))
	if _, module, err := s.WorkInfoPluginModule(ctx, broken.ID); err != nil || module != nil {
		t.Fatalf("broken module = %q, %v; want nil", module, err)
	}
	if _, _, err := s.WorkInfoPluginModule(ctx, 9999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: err = %v; want ErrNotFound", err)
	}
}
