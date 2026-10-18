package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func merge(t *testing.T, db *DB, into int64, items ...domain.OrganizeRef) []byte {
	t.Helper()
	image, err := db.Organize().Merge(context.Background(), items, into, 0)
	if err != nil {
		t.Fatalf("Merge(%v, %d) error = %v", items, into, err)
	}
	return image
}

func undo(t *testing.T, db *DB, image []byte) {
	t.Helper()
	if err := db.Organize().Undo(context.Background(), image); err != nil {
		t.Fatalf("Undo error = %v", err)
	}
}

func playlistWith(t *testing.T, db *DB, name string, add domain.PlaylistAdd) int64 {
	t.Helper()
	ctx := context.Background()
	playlist, err := db.Playlists().CreatePlaylist(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playlists().AddPlaylistItems(ctx, playlist.ID, add); err != nil {
		t.Fatal(err)
	}
	return playlist.ID
}

func playlistEntryIDs(t *testing.T, db *DB, id int64) []string {
	t.Helper()
	rows, err := queryStringsForTest(db, `select id || '=' || coalesce('v:' || content_key, 'g:' || group_id)
		from playlist_items where playlist_id = ? order by position`, id)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func groupFavorite(t *testing.T, db *DB, id int64) bool {
	t.Helper()
	var favorite bool
	if err := db.sql.QueryRow(`select exists (select 1 from group_favorites where group_id = ?)`, id).Scan(&favorite); err != nil {
		t.Fatal(err)
	}
	return favorite
}

// Exclude timestamps, which undo deliberately advances.
func mergeSnapshot(t *testing.T, db *DB) string {
	t.Helper()
	var b strings.Builder
	for _, query := range []string{
		`select id || '|' || name || '|' || coalesce(origin_folder, '-') from groups order by id`,
		`select id || '|' || group_id || '|' || coalesce(content_key, '') || '|' || coalesce(child_id, '') || '|' ||
			position || '|' || from_folder from group_items order by id`,
		`select group_id || '|' || content_key from group_exclusions order by group_id, content_key`,
		`select group_id || '|' || favorited_at from group_favorites order by group_id`,
		`select id || '|' || playlist_id || '|' || coalesce(content_key, '') || '|' || coalesce(group_id, '') || '|' ||
			position || '|' || added_at from playlist_items order by id`,
		`select path || '|' || mode from folder_group_overrides order by path`,
	} {
		rows, err := queryStringsForTest(db, query)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(&b, rows)
	}
	return b.String()
}

func TestOrganizeMergeHandsOverAndUndoes(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ya, yb, xa, xb := looseVideo(t, db, "ya"), looseVideo(t, db, "yb"), looseVideo(t, db, "xa"), looseVideo(t, db, "xb")
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{ya, yb}})
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{xa, yb, xb}})
	q := createGroup(t, db, "Q", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "q1")}, GroupIDs: []int64{x}})
	if _, err := db.Groups().AddGroupItems(ctx, q, domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "q2")}}); err != nil {
		t.Fatal(err)
	}
	setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{x}, Favorite: true})
	playlist := playlistWith(t, db, "P", domain.PlaylistAdd{VideoIDs: []int64{ya}, GroupIDs: []int64{x}})
	playlistBefore := playlistEntryIDs(t, db, playlist)
	qItem := groupItemID(t, db, q, fmt.Sprintf("g:%d", x))
	before := mergeSnapshot(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	image := merge(t, db, y, groupRef(x))

	assertEntries(t, db, y, "v:ya", "v:yb", "v:xa", "v:xb")
	if groupExists(t, db, x) {
		t.Fatal("the merged group still exists")
	}
	if !groupFavorite(t, db, y) {
		t.Fatal("the target did not become a favorite")
	}
	assertEntries(t, db, q, "v:q1", fmt.Sprintf("g:%d", y), "v:q2")
	if got := groupItemID(t, db, q, fmt.Sprintf("g:%d", y)); got != qItem {
		t.Fatalf("parent item id = %d, want %d (replaced in place)", got, qItem)
	}
	wantPlaylist := []string{playlistBefore[0], strings.Replace(playlistBefore[1], fmt.Sprintf("g:%d", x), fmt.Sprintf("g:%d", y), 1)}
	if got := playlistEntryIDs(t, db, playlist); !slices.Equal(got, wantPlaylist) {
		t.Fatalf("playlist after merge = %v, want %v", got, wantPlaylist)
	}
	want := []domain.Event{domain.GroupsChanged{GroupIDs: normalizeIDs([]int64{x, y, q})}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("merge events = %+v, want %+v", recorder.events, want)
	}

	undo(t, db, image)
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
	if groupFavorite(t, db, y) || !groupFavorite(t, db, x) {
		t.Fatal("undo did not put the favorite back on the merged group")
	}
}

func TestOrganizeMergeRemovesWhereTargetIsThere(t *testing.T) {
	db := migratedDB(t)
	ya, xa := looseVideo(t, db, "ya"), looseVideo(t, db, "xa")
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{xa}})
	emptied := createGroup(t, db, "Emptied", domain.GroupAdd{GroupIDs: []int64{x}})
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{ya}, GroupIDs: []int64{x, emptied}})
	both := createGroup(t, db, "Both", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "b1")}, GroupIDs: []int64{x, y}})
	playlist := playlistWith(t, db, "P", domain.PlaylistAdd{GroupIDs: []int64{y, x}})
	playlistBefore := playlistEntryIDs(t, db, playlist)
	before := mergeSnapshot(t, db)

	image := merge(t, db, y, groupRef(x))

	assertEntries(t, db, y, "v:ya", "v:xa")
	if groupExists(t, db, emptied) {
		t.Fatal("the parent emptied by the merge was kept")
	}
	assertEntries(t, db, both, "v:b1", fmt.Sprintf("g:%d", y))
	if got := playlistEntryIDs(t, db, playlist); !slices.Equal(got, playlistBefore[:1]) {
		t.Fatalf("playlist after merge = %v, want %v", got, playlistBefore[:1])
	}

	undo(t, db, image)
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
}

func TestOrganizeMergeMixedSelection(t *testing.T) {
	db := migratedDB(t)
	ya, v, xa := looseVideo(t, db, "ya"), looseVideo(t, db, "v"), looseVideo(t, db, "xa")
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{ya}})
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{xa, v}})
	before := mergeSnapshot(t, db)

	image := merge(t, db, y, videoRef(v), groupRef(x), videoRef(v))
	assertEntries(t, db, y, "v:ya", "v:v", "v:xa")
	if groupExists(t, db, x) {
		t.Fatal("the merged group still exists")
	}

	undo(t, db, image)
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
}

func TestOrganizeMergeAutomaticGroupWritesUngroup(t *testing.T) {
	db := migratedDB(t)
	show := fixturePath("/media/Show")
	upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	auto := readGroups(t, db)[0].ID
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ya")}})
	before := mergeSnapshot(t, db)

	image := merge(t, db, y, groupRef(auto))
	assertEntries(t, db, y, "v:ya", "v:k1", "v:k2")
	if grouping := folderGroupingOf(t, db, show); grouping.Mode != domain.FolderGroupUngroup || grouping.GroupID != 0 {
		t.Fatalf("grouping after merge = %+v", grouping)
	}
	rebuildIndex(t, db)
	if groupExists(t, db, auto) || len(readGroups(t, db)) != 1 {
		t.Fatalf("groups after the rebuild = %+v", readGroups(t, db))
	}

	undo(t, db, image)
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
	if grouping := folderGroupingOf(t, db, show); grouping.Mode != "" || grouping.GroupID != auto {
		t.Fatalf("grouping after undo = %+v", grouping)
	}
}

func TestOrganizeMergeRefusals(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	v := looseVideo(t, db, "v")
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{v}})
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{y}})
	grand := createGroup(t, db, "Grand", domain.GroupAdd{GroupIDs: []int64{parent}})
	other := createGroup(t, db, "Other", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "o")}})
	before := mergeSnapshot(t, db)
	missing := int64(999999)
	for name, c := range map[string]struct {
		into  int64
		items []domain.OrganizeRef
		want  error
	}{
		"自分自身":   {into: y, items: []domain.OrganizeRef{groupRef(other), groupRef(y)}, want: domain.ErrGroupCycle},
		"親":      {into: y, items: []domain.OrganizeRef{groupRef(parent)}, want: domain.ErrGroupCycle},
		"祖父":     {into: y, items: []domain.OrganizeRef{groupRef(other), groupRef(grand)}, want: domain.ErrGroupCycle},
		"無いマージ先": {into: missing, items: []domain.OrganizeRef{groupRef(other)}, want: domain.ErrGroupTargetNotFound},
		"無いグループ": {into: y, items: []domain.OrganizeRef{groupRef(other), groupRef(missing)}, want: domain.ErrGroupTargetNotFound},
		"無い動画":   {into: y, items: []domain.OrganizeRef{groupRef(other), videoRef(missing)}, want: domain.ErrGroupTargetNotFound},
	} {
		if _, err := db.Organize().Merge(ctx, c.items, c.into, 0); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v, want %v", name, err, c.want)
		}
	}
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("refused merge wrote:\n%s\nwant\n%s", after, before)
	}
}

func TestOrganizeMergeUndoConflicts(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ya")}})
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "xa")}})
	image := merge(t, db, y, groupRef(x))
	if _, err := db.Groups().AddGroupItems(ctx, y, domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "late")}}); err != nil {
		t.Fatal(err)
	}
	before := mergeSnapshot(t, db)
	if err := db.Organize().Undo(ctx, image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo error = %v, want ErrUndoConflict", err)
	}
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("conflicting undo wrote:\n%s\nwant\n%s", after, before)
	}
}

func TestOrganizeMergeUndoConflictsOnTargetRename(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ya")}})
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "xa")}})
	image := merge(t, db, y, groupRef(x))
	if _, err := db.Groups().RenameGroup(ctx, y, "Renamed"); err != nil {
		t.Fatal(err)
	}
	before := mergeSnapshot(t, db)
	if err := db.Organize().Undo(ctx, image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo error = %v, want ErrUndoConflict", err)
	}
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("conflicting undo wrote:\n%s\nwant\n%s", after, before)
	}
}

func TestOrganizeMergeKeepsAnEmptiedTarget(t *testing.T) {
	for name, nested := range map[string]bool{"Y が X だけを持つ": false, "Y が X だけを持つ子だけを持つ": true} {
		t.Run(name, func(t *testing.T) {
			db := migratedDB(t)
			x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "lone")}})
			holder := x
			if nested {
				holder = createGroup(t, db, "P", domain.GroupAdd{GroupIDs: []int64{x}})
			}
			y := createGroup(t, db, "Y", domain.GroupAdd{GroupIDs: []int64{holder}})
			// Delete fixture rows directly because normal item removal would dissolve the empty group.
			if _, err := db.sql.Exec(`delete from group_items where group_id = ?`, x); err != nil {
				t.Fatal(err)
			}
			before := mergeSnapshot(t, db)

			image := merge(t, db, y, groupRef(x))
			if !groupExists(t, db, y) {
				t.Fatal("the merge dissolved its target")
			}
			if groupExists(t, db, x) || (nested && groupExists(t, db, holder)) {
				t.Fatal("the merged group or the emptied parent was kept")
			}
			assertEntries(t, db, y)

			undo(t, db, image)
			if after := mergeSnapshot(t, db); after != before {
				t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
			}
		})
	}
}
