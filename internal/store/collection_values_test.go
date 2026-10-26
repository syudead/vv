package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A page must use committed collection values and decode only the requested covers.
func TestLibraryPageUsesStoredValuesAndBoundedPreviews(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := make([]int64, domain.MaxFolderPreviews+1)
	for i := range ids {
		ids[i] = looseVideo(t, db, fmt.Sprintf("cover%d", i))
	}
	groupID := createGroup(t, db, "A group", domain.GroupAdd{VideoIDs: ids})
	folderID := newSmartFolder(t, db, "Z folder", domain.SmartRule{}, domain.SortAddedDesc, 0)
	execSQL(t, db, `update videos set thumbnail_state = 'done'`)
	// Width is irrelevant to paging and the fifth video is beyond the cover boundary.
	if _, err := db.sql.Exec(`update videos set width = 'must not decode' where id = ?`, ids[len(ids)-1]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Library().GetVideo(ctx, domain.AudienceOwner, ids[len(ids)-1]); err == nil {
		t.Fatal("fixture must reject full detail decoding")
	}
	refreshCollections(t, db)
	for _, table := range []string{"group_values", "group_members"} {
		execSQL(t, db, `create trigger forbid_`+table+` before insert on `+table+` begin select raise(abort, 'page recomputed collection values'); end`)
	}
	// Smart folders lead the library, so the group follows on the second page.
	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].SmartFolder == nil || page.Items[0].SmartFolder.ID != folderID {
		t.Fatalf("first page = %+v", page)
	}
	page, err = db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Group == nil || page.Items[0].Group.ID != groupID || page.NextCursor != "" {
		t.Fatalf("second page = %+v", page)
	}
	group := page.Items[0].Group
	if group.Card == nil || len(group.Members) != 0 || !slices.Equal(group.Card.VideoIDs, ids) {
		t.Fatalf("group card = %+v", group)
	}
	if got := memberIDs(domain.LibraryGroup{Members: group.Card.Previews}); !slices.Equal(got, ids[:domain.MaxFolderPreviews]) {
		t.Fatalf("previews = %v", got)
	}
}

func TestCollectionRefreshFailureRollsBackAndCanRetry(t *testing.T) {
	for _, table := range []string{"group_values"} {
		t.Run(table, func(t *testing.T) {
			db := migratedDB(t)
			ctx := context.Background()
			video := looseVideo(t, db, "one")
			group := createGroup(t, db, "group", domain.GroupAdd{VideoIDs: []int64{video}})
			refreshCollections(t, db)
			// Model a write made while the application was stopped, awaiting startup recovery.
			execSQL(t, db, `update videos set duration_ms = 2500`)
			execSQL(t, db, `create trigger reject_refresh before insert on `+table+`
				when new.audience = 0 begin select raise(abort, 'refresh rejected'); end`)
			if err := db.RefreshCollectionValues(ctx); err == nil || !strings.Contains(err.Error(), "refresh rejected") {
				t.Fatalf("refresh error = %v", err)
			}
			assertStoredDuration(t, db, group, sql.NullInt64{})
			var pending bool
			if err := db.sql.QueryRow(`select exists(select 1 from collection_changes)`).Scan(&pending); err != nil || !pending {
				t.Fatalf("recovery lost pending changes: %v, %v", pending, err)
			}
			execSQL(t, db, `drop trigger reject_refresh`)
			if err := db.RefreshCollectionValues(ctx); err != nil {
				t.Fatal(err)
			}
			assertStoredDuration(t, db, group, sql.NullInt64{Int64: 2500, Valid: true})
			if err := db.sql.QueryRow(`select exists(select 1 from collection_changes)`).Scan(&pending); err != nil || pending {
				t.Fatalf("successful recovery left pending changes: %v, %v", pending, err)
			}
		})
	}
}

// Bulk writes such as job results only mark collections; a refresh that would fail must not block them.
func TestCollectionRefreshFailureDoesNotBlockSourceWrites(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	video := looseVideo(t, db, "one")
	group := createGroup(t, db, "group", domain.GroupAdd{VideoIDs: []int64{video}})
	refreshCollections(t, db)
	execSQL(t, db, `create trigger reject_refresh before insert on group_values
		begin select raise(abort, 'refresh rejected'); end`)
	probe := domain.Probe{DurationMs: 2500}
	if err := db.Ingest().ApplyProbe(ctx, video, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatalf("write error = %v", err)
	}
	var duration sql.NullInt64
	if err := db.sql.QueryRow(`select duration_ms from videos where id = ?`, video).Scan(&duration); err != nil || duration.Int64 != 2500 {
		t.Fatalf("source write = %v, %v", duration, err)
	}
	assertStoredDuration(t, db, group, sql.NullInt64{})
}

func assertStoredDuration(t *testing.T, db *DB, group int64, want sql.NullInt64) {
	t.Helper()
	var got sql.NullInt64
	if err := db.sql.QueryRow(`select duration_ms from group_values where group_id = ? and audience = 1`, group).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("group_values duration = %v, want %v", got, want)
	}
}

// A refresh brings stored values up to every change committed before it.
func TestCollectionValuesFollowCommittedChanges(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	one, two := looseVideo(t, db, "one"), looseVideo(t, db, "two")
	groupID := createGroup(t, db, "group", domain.GroupAdd{VideoIDs: []int64{one, two}})
	if err := db.Ingest().ApplyProbe(ctx, one, domain.Probe{DurationMs: 2000}, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{one}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, "one", domain.Progress{Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	refreshCollections(t, db)
	var count, duration, watched, open int64
	if err := db.sql.QueryRow(`select video_count, duration_ms, watched, open_video_id from group_values where group_id = ? and audience = 1`, groupID).Scan(&count, &duration, &watched, &open); err != nil {
		t.Fatal(err)
	}
	if count != 2 || duration != 2000 || watched != 1 || open != two {
		t.Fatalf("owner group = %d, %d, %d, %d", count, duration, watched, open)
	}
	if err := db.sql.QueryRow(`select video_count, watched from group_values where group_id = ? and audience = 0`, groupID).Scan(&count, &watched); err != nil {
		t.Fatal(err)
	}
	if count != 1 || watched != 0 {
		t.Fatalf("guest group = %d, %d", count, watched)
	}
	if err := db.ScanIndex().DeleteVideos(ctx, []int64{one}); err != nil {
		t.Fatal(err)
	}
	refreshCollections(t, db)
	if err := db.sql.QueryRow(`select video_count from group_values where group_id = ? and audience = 1`, groupID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count after deleting a member = %d, %v", count, err)
	}
	var pending int
	if err := db.sql.QueryRow(`select count(*) from collection_changes`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("pending changes after refresh = %d, %v", pending, err)
	}
}

// A video change refreshes the groups that contain it, with their ancestors, and leaves other groups alone.
func TestCollectionRefreshTouchesOnlyGroupsOfTheChangedVideo(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	one, two := looseVideo(t, db, "one"), looseVideo(t, db, "two")
	child := createGroup(t, db, "child", domain.GroupAdd{VideoIDs: []int64{one}})
	parent := createGroup(t, db, "parent", domain.GroupAdd{GroupIDs: []int64{child}})
	other := createGroup(t, db, "other", domain.GroupAdd{VideoIDs: []int64{two}})
	refreshCollections(t, db)
	execSQL(t, db, fmt.Sprintf(`create trigger reject_other before insert on group_values when new.group_id = %d
		begin select raise(abort, 'unrelated group refreshed'); end`, other))

	probe := domain.Probe{DurationMs: 2500}
	if err := db.Ingest().ApplyProbe(ctx, one, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	refreshCollections(t, db)
	for _, id := range []int64{child, parent} {
		assertStoredDuration(t, db, id, sql.NullInt64{Int64: 2500, Valid: true})
	}
}

func refreshCollections(t *testing.T, db *DB) {
	t.Helper()
	if err := db.RefreshCollectionValues(context.Background()); err != nil {
		t.Fatal(err)
	}
}
