package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"testing"
	"time"

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
	for _, table := range []string{"group_values", "group_members", "smart_folder_values"} {
		execSQL(t, db, `create trigger forbid_`+table+` before insert on `+table+` begin select raise(abort, 'page recomputed collection values'); end`)
	}
	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].Group == nil || page.Items[0].Group.ID != groupID {
		t.Fatalf("first page = %+v", page)
	}
	group := page.Items[0].Group
	if group.Card == nil || len(group.Members) != 0 || !slices.Equal(group.Card.VideoIDs, ids) {
		t.Fatalf("group card = %+v", group)
	}
	if got := memberIDs(domain.LibraryGroup{Members: group.Card.Previews}); !slices.Equal(got, ids[:domain.MaxFolderPreviews]) {
		t.Fatalf("previews = %v", got)
	}
	page, err = db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].SmartFolder == nil || page.Items[0].SmartFolder.ID != folderID || page.NextCursor != "" {
		t.Fatalf("second page = %+v", page)
	}
}

// Writes publish current collection values before any list request can refresh them.
func TestCollectionValuesFollowCommittedChanges(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	one, two := looseVideo(t, db, "one"), looseVideo(t, db, "two")
	groupID := createGroup(t, db, "group", domain.GroupAdd{VideoIDs: []int64{one, two}})
	folderID := newSmartFolder(t, db, "watched", domain.SmartRule{Watch: domain.WatchWatched}, domain.SortAddedDesc, 0)
	if err := db.Ingest().ApplyProbe(ctx, one, domain.Probe{DurationMs: 2000}, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{one}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, "one", domain.Progress{Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
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
	if err := db.sql.QueryRow(`select duration_ms from smart_folder_values where folder_id = ? and audience = 1`, folderID).Scan(&duration); err != nil || duration != 2000 {
		t.Fatalf("smart folder duration = %d, %v", duration, err)
	}
	if err := db.ScanIndex().DeleteVideos(ctx, []int64{one}); err != nil {
		t.Fatal(err)
	}
	var empty sql.NullInt64
	if err := db.sql.QueryRow(`select duration_ms from smart_folder_values where folder_id = ? and audience = 1`, folderID).Scan(&empty); err != nil || empty.Valid {
		t.Fatalf("deleted member duration = %v, %v", empty, err)
	}
	if _, err := db.SmartFolders().ChangeSmartFolder(ctx, folderID, domain.SmartFolderChange{Rule: &domain.SmartRule{Q: "two"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow(`select size_bytes from smart_folder_values where folder_id = ? and audience = 1`, folderID).Scan(&duration); err != nil || duration != 10 {
		t.Fatalf("changed rule size = %d, %v", duration, err)
	}
	var pending int
	if err := db.sql.QueryRow(`select count(*) from collection_changes`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("pending changes after commit = %d, %v", pending, err)
	}
}

func TestStoredSmartFolderAgeBoundary(t *testing.T) {
	now := fixedTime
	previous := listNow
	listNow = func() time.Time { return now }
	t.Cleanup(func() { listNow = previous })
	db := migratedDB(t)
	ctx := context.Background()
	file := sampleFile(fixturePath("/media/recent.mp4"), "recent", "recent", 10, 0)
	file.AddedAt = now.Add(-24 * time.Hour)
	video, err := db.ScanIndex().UpsertVideo(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{video.ID}, true); err != nil {
		t.Fatal(err)
	}
	folder := newSmartFolder(t, db, "recent", domain.SmartRule{AddedWithin: new(1)}, domain.SortAddedDesc, 0)
	for _, expired := range []bool{false, true} {
		if expired {
			now = now.Add(time.Second)
		}
		page, err := db.Library().ListLibrary(ctx, domain.AudienceGuest, domain.VideoQuery{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		found := slices.ContainsFunc(page.Items, func(item domain.LibraryItem) bool { return item.SmartFolder != nil && item.SmartFolder.ID == folder })
		if found == expired {
			t.Fatalf("folder present = %v, expired = %v", found, expired)
		}
	}
}

// Group watch state can match without an individually matching member; its age boundary still expires.
func TestStoredSmartFolderExpiresGroupOnlyMatch(t *testing.T) {
	now := fixedTime
	previous := listNow
	listNow = func() time.Time { return now }
	t.Cleanup(func() { listNow = previous })
	db := migratedDB(t)
	ctx := context.Background()
	var ids []int64
	for _, key := range []string{"watched", "unwatched"} {
		file := sampleFile(fixturePath("/media/"+key+".mp4"), key, key, 10, 0)
		file.AddedAt = now.Add(-24 * time.Hour)
		video, err := db.ScanIndex().UpsertVideo(ctx, file)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, video.ID)
	}
	createGroup(t, db, "mixed", domain.GroupAdd{VideoIDs: ids})
	if _, err := db.Visibility().SetVideosPublic(ctx, ids, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, "watched", domain.Progress{Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	folder := newSmartFolder(t, db, "recent progress", domain.SmartRule{AddedWithin: new(1), Watch: domain.WatchInProgress}, domain.SortAddedDesc, 0)
	var visible bool
	var expiry sql.NullInt64
	if err := db.sql.QueryRow(`select visible, expires_at from smart_folder_values where folder_id = ? and audience = 0`, folder).Scan(&visible, &expiry); err != nil {
		t.Fatal(err)
	}
	if !visible || !expiry.Valid || expiry.Int64 != now.Unix()+1 {
		t.Fatalf("group-only folder = visible %v, expiry %v", visible, expiry)
	}
	now = now.Add(time.Second)
	page, err := db.Library().ListLibrary(ctx, domain.AudienceGuest, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(page.Items, func(item domain.LibraryItem) bool { return item.SmartFolder != nil && item.SmartFolder.ID == folder }) {
		t.Fatal("expired group-only folder remains")
	}
}
