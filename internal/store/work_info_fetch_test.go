package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

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

// Revoking and reapproving the same digest must invalidate the fetched generation; recheck it inside the transaction.
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

// Fetched tentative tags must not promote automatic suggestions.
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

// A virtual video takes the fetched title as its own title, never as a display-name override, and the other fields as
// work information under its key; a stale plugin leaves it unchanged.
func TestApplyFetchedWorkInfoToVirtualVideo(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	virtual := createVirtual(t, db, "Before", fixedTime)
	plugin := approvedPlugin(t, db, "example", "d1")
	title := "  Fetched title  "
	apply := domain.FetchedWorkInfoApply{
		VideoID: virtual.ID, PluginID: plugin.ID, Digest: "d1", Generation: plugin.Generation, Title: &title,
		Details: domain.FetchedWorkInfo{
			Description: "Fetched notes", Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2024-03-01"}},
			URLs: []string{"https://example.com/works/1"},
		},
		DetailFields: []domain.WorkInfoField{
			domain.WorkInfoFieldDescription, domain.DateField(domain.DateRelease), domain.WorkInfoFieldURLs,
		},
	}
	stored := func() (string, domain.WorkInfo, int) {
		t.Helper()
		var stored string
		var overrides int
		if err := db.sql.QueryRow(`select title from virtual_videos where user_key = ?`, virtual.UserKey).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if err := db.sql.QueryRow(`select count(*) from video_overrides where content_key = ?`, virtual.UserKey).Scan(&overrides); err != nil {
			t.Fatal(err)
		}
		infos, err := db.WorkInfo().ByUserKeys(ctx, []string{virtual.UserKey})
		if err != nil {
			t.Fatal(err)
		}
		return stored, infos[virtual.UserKey], overrides
	}

	if _, err := db.WorkInfoPlugins().RevokeWorkInfoPlugin(ctx, plugin.ID, pluginNow); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, apply); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("revoked plugin: err = %v, want ErrWorkInfoFetchStale", err)
	}
	if got, info, overrides := stored(); got != "Before" || info.Description != "" || len(info.URLs) != 0 || overrides != 0 {
		t.Fatalf("after a stale apply: title %q, work info %+v, %d overrides; want nothing written", got, info, overrides)
	}

	plugin = approvedPlugin(t, db, "example", "d1")
	apply.Generation = plugin.Generation
	applied, err := db.Overrides().ApplyFetchedWorkInfo(ctx, apply)
	if err != nil {
		t.Fatalf("ApplyFetchedWorkInfo: %v", err)
	}
	got, info, overrides := stored()
	if got != "Fetched title" || applied.Video.Title != "Fetched title" || overrides != 0 {
		t.Fatalf("title %q (video %q), %d overrides; want the trimmed title in virtual_videos alone",
			got, applied.Video.Title, overrides)
	}
	if info.Description != "Fetched notes" || !slices.Equal(info.URLs, apply.Details.URLs) ||
		!slices.Equal(info.Dates, apply.Details.Dates) {
		t.Fatalf("work info = %+v; want the fetched description, release date and link", info)
	}
}

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
