package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestSmartFolderSearchNarrowsSavedMembershipAndPages(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b, outside := looseVideo(t, db, "clip-a"), looseVideo(t, db, "clip-b"), looseVideo(t, db, "clip-outside")
	tag := attachNamedTag(t, db, "private-label", a, b)
	parent := newSmartFolder(t, db, "parent", domain.SmartRule{Tags: []int64{tag}}, domain.SortTitleAsc, 0)
	folder := newSmartFolder(t, db, "folder", domain.SmartRule{Q: "clip"}, domain.SortTitleAsc, parent)
	newSmartFolder(t, db, "child", domain.SmartRule{}, domain.SortTitleAsc, folder)
	var got []int64
	cursor := ""
	for range 2 {
		page, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, folder, cursor, 1, domain.VideoQuery{Query: `"clip" -outside`})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || page.Items[0].Video == nil {
			t.Fatalf("page items = %+v", page.Items)
		}
		got = append(got, page.Items[0].Video.ID)
		if len(page.SmartFolder.Members) != 2 || page.Folder.Rule.Q != "clip" {
			t.Fatalf("search changed collection = %+v", page)
		}
		cursor = page.NextCursor
	}
	if !slices.Equal(got, []int64{a, b}) || cursor != "" {
		t.Fatalf("paged IDs = %v, cursor %q", got, cursor)
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{a, outside}, true); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"clip-outside", "private-label"} {
		page, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceGuest, folder, "", 1, domain.VideoQuery{Query: query})
		if err != nil {
			t.Fatalf("empty guest search must remain accessible: %v", err)
		}
		if len(page.Items) != 0 || len(page.SmartFolder.Members) != 1 || page.NextCursor != "" {
			t.Fatalf("guest search %q = %+v", query, page)
		}
	}
	original, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, folder, "", 10, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if names := smartItemNames(original.Items); !slices.Equal(names, []string{"folder:child", "clip-a", "clip-b"}) {
		t.Fatalf("cleared search = %v", names)
	}
}

func TestSmartFolderSearchNarrowsGroupAndKeepsSummary(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := looseVideo(t, db, "watched"), looseVideo(t, db, "new")
	createGroup(t, db, "mixed", domain.GroupAdd{VideoIDs: []int64{a, b}})
	if _, err := db.Playback().SaveProgress(ctx, "watched", domain.Progress{Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	folder := newSmartFolder(t, db, "all", domain.SmartRule{}, domain.SortTitleAsc, 0)
	page, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, folder, "", 10, domain.VideoQuery{Query: "watched"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Video == nil || page.Items[0].Video.ID != a {
		t.Fatalf("search should narrow the mixed group's current members: %+v", page.Items)
	}
	if page.SmartFolder.WatchState != domain.WatchStateInProgress {
		t.Fatalf("summary changed: %s", page.SmartFolder.WatchState)
	}
}
