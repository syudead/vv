package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func groupName(t *testing.T, db *DB, id int64) string {
	t.Helper()
	var name string
	if err := db.sql.QueryRow(`select name from groups where id = ?`, id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func groupOp(t *testing.T, db *DB, videoIDs []int64, target int64, from *int64) (int64, []byte) {
	t.Helper()
	id, image, err := db.Organize().Group(context.Background(), videoIDs, target, from)
	if err != nil {
		t.Fatalf("Group error = %v", err)
	}
	return id, image
}

func TestOrganizeGroupMakesAGroupOfTargetThenDragged(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := looseVideo(t, db, "ka"), looseVideo(t, db, "kb")
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	id, image := groupOp(t, db, []int64{a}, b, nil)
	assertEntries(t, db, id, "v:kb", "v:ka")
	if got := groupName(t, db, id); got != "kb" {
		t.Fatalf("name = %q, want the target's title", got)
	}
	want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{id}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("events = %+v, want %+v", recorder.events, want)
	}

	if err := db.Organize().Undo(ctx, image); err != nil {
		t.Fatal(err)
	}
	if groupExists(t, db, id) {
		t.Fatal("undo kept the new group")
	}
	if got, err := queryStringsForTest(db, `select id from group_items`); err != nil || len(got) != 0 {
		t.Fatalf("group items after undo = %v (%v)", got, err)
	}
}

func TestOrganizeGroupInsideFromTakesTheTargetsPlace(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	from, items := fiveItemGroup(t, db)
	before := groupTablesSnapshot(t, db)
	k2, k4 := videoIDForKey(t, db, "k2"), videoIDForKey(t, db, "k4")

	id, image := groupOp(t, db, []int64{k4}, k2, &from)
	assertEntries(t, db, from, "v:k1", "g:"+strconv.FormatInt(id, 10), "v:k3", "v:k5")
	assertEntries(t, db, id, "v:k2", "v:k4")

	if err := db.Organize().Undo(ctx, image); err != nil {
		t.Fatal(err)
	}
	if groupExists(t, db, id) {
		t.Fatal("undo kept the new group")
	}
	if got := groupItemIDsInOrder(t, db, from); !slices.Equal(got, items) {
		t.Fatalf("item ids after undo = %v, want %v", got, items)
	}
	if after := withoutUpdatedAt(groupTablesSnapshot(t, db)); after != withoutUpdatedAt(before) {
		t.Fatalf("undo left:\n%s\nwant\n%s", after, before)
	}
}

func TestOrganizeGroupInsideAnAutomaticGroupWritesExclusions(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	k1 := upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	k2 := upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	upsertFolderVideo(t, db, fixturePath("/media/Show/3.mp4"), "k3")
	rebuildIndex(t, db)
	show := readGroups(t, db)[0].ID

	id, image := groupOp(t, db, []int64{k2}, k1, &show)
	if got := groupExclusions(t, db, show); !slices.Equal(got, []string{"k1", "k2"}) {
		t.Fatalf("exclusions = %v", got)
	}
	rebuildIndex(t, db)
	assertEntries(t, db, show, "g:"+strconv.FormatInt(id, 10), "v:k3")

	if err := db.Organize().Undo(ctx, image); err != nil {
		t.Fatal(err)
	}
	if got := groupExclusions(t, db, show); len(got) != 0 {
		t.Fatalf("exclusions after undo = %v", got)
	}
	assertEntries(t, db, show, "v:k1", "v:k2", "v:k3")
}

func TestOrganizeGroupCutsALongTitle(t *testing.T) {
	db := migratedDB(t)
	a, b := looseVideo(t, db, "ka"), looseVideo(t, db, "kb")
	title := strings.Repeat("あ", 120)
	if _, err := db.Overrides().SetDisplayName(context.Background(), b, title); err != nil {
		t.Fatal(err)
	}
	id, _ := groupOp(t, db, []int64{a}, b, nil)
	if got := groupName(t, db, id); got != strings.Repeat("あ", 100) {
		t.Fatalf("name = %q (%d runes)", got, len([]rune(got)))
	}
}

func TestOrganizeGroupRefusesMissingTargets(t *testing.T) {
	db := migratedDB(t)
	from, _ := fiveItemGroup(t, db)
	k1 := videoIDForKey(t, db, "k1")
	outside := looseVideo(t, db, "kx")
	missing := int64(999999)
	before := groupTablesSnapshot(t, db)
	for name, c := range map[string]struct {
		videos []int64
		target int64
		from   *int64
	}{
		"無いドラッグした動画":    {videos: []int64{missing}, target: k1},
		"無い落とした先":       {videos: []int64{k1}, target: missing},
		"無い from":       {videos: []int64{outside}, target: k1, from: &missing},
		"from に無い動画":    {videos: []int64{outside}, target: k1, from: &from},
		"from に無い落とした先": {videos: []int64{k1}, target: outside, from: &from},
	} {
		if _, _, err := db.Organize().Group(context.Background(), c.videos, c.target, c.from); !errors.Is(err, domain.ErrGroupTargetNotFound) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
	if after := groupTablesSnapshot(t, db); after != before {
		t.Fatalf("refused group wrote:\n%s\nwant\n%s", after, before)
	}
}

func TestOrganizeGroupUndoConflictsAfterLaterChanges(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, db *DB, id int64){
		"お気に入り": func(t *testing.T, db *DB, id int64) {
			t.Helper()
			if _, err := db.Favorites().SetFavorites(context.Background(), domain.FavoriteChange{GroupIDs: []int64{id}, Favorite: true}); err != nil {
				t.Fatal(err)
			}
		},
		"プレイリスト": func(t *testing.T, db *DB, id int64) {
			t.Helper()
			playlist, err := db.Playlists().CreatePlaylist(context.Background(), "List")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Playlists().AddPlaylistItems(context.Background(), playlist.ID, domain.PlaylistAdd{GroupIDs: []int64{id}}); err != nil {
				t.Fatal(err)
			}
		},
		"別のグループ": func(t *testing.T, db *DB, id int64) {
			t.Helper()
			createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{id}})
		},
		"トークンなしの名前変更": func(t *testing.T, db *DB, id int64) {
			t.Helper()
			if _, err := db.Groups().RenameGroup(context.Background(), id, "Renamed"); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			db := migratedDB(t)
			a, b := looseVideo(t, db, "ka"), looseVideo(t, db, "kb")
			id, image := groupOp(t, db, []int64{a}, b, nil)
			change(t, db, id)
			before := groupTablesSnapshot(t, db)
			if err := db.Organize().Undo(context.Background(), image); !errors.Is(err, domain.ErrUndoConflict) {
				t.Fatalf("Undo error = %v, want ErrUndoConflict", err)
			}
			if after := groupTablesSnapshot(t, db); after != before {
				t.Fatalf("conflicting undo wrote:\n%s\nwant\n%s", after, before)
			}
			if !groupExists(t, db, id) {
				t.Fatal("conflicting undo deleted the group")
			}
		})
	}
}

func TestOrganizeRenameUnderTheToken(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	from, items := fiveItemGroup(t, db)
	k2, k4 := videoIDForKey(t, db, "k2"), videoIDForKey(t, db, "k4")
	id, image := groupOp(t, db, []int64{k4}, k2, &from)

	group, renamed, err := db.Organize().RenameGroup(ctx, id, "  Renamed  ", image)
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != id || group.Name != "Renamed" || renamed == nil {
		t.Fatalf("group = %+v, image = %d bytes", group, len(renamed))
	}
	if err := db.Organize().Undo(ctx, image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo of the old image error = %v", err)
	}
	if err := db.Organize().Undo(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if groupExists(t, db, id) {
		t.Fatal("undo kept the renamed group")
	}
	if got := groupItemIDsInOrder(t, db, from); !slices.Equal(got, items) {
		t.Fatalf("item ids after undo = %v, want %v", got, items)
	}

	id, image = groupOp(t, db, []int64{k4}, k2, &from)
	if _, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{GroupIDs: []int64{id}, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	group, renamed, err = db.Organize().RenameGroup(ctx, id, "Stale", image)
	if err != nil || group.Name != "Stale" || renamed != nil {
		t.Fatalf("stale rename = %+v, %d bytes, %v", group, len(renamed), err)
	}
	group, renamed, err = db.Organize().RenameGroup(ctx, id, "Plain", nil)
	if err != nil || group.Name != "Plain" || renamed != nil {
		t.Fatalf("plain rename = %+v, %d bytes, %v", group, len(renamed), err)
	}
	if _, _, err := db.Organize().RenameGroup(ctx, 999999, "Missing", nil); !errors.Is(err, domain.ErrGroupNotFound) {
		t.Fatalf("missing group error = %v", err)
	}
	var invalid *domain.InvalidGroupNameError
	if _, _, err := db.Organize().RenameGroup(ctx, id, " ", image); !errors.As(err, &invalid) {
		t.Fatalf("empty name error = %v", err)
	}
}

func videoIDForKey(t *testing.T, db *DB, key string) int64 {
	t.Helper()
	var id int64
	if err := db.sql.QueryRow(`select id from videos where content_key = ?`, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Ignore timestamps because undo deliberately advances them while restoring all other state.
func withoutUpdatedAt(snapshot string) string {
	lines := strings.Split(snapshot, "\n")
	for i, line := range lines {
		fields := strings.Split(line, "|")
		if len(fields) == 4 {
			lines[i] = strings.Join(fields[:3], "|")
		}
	}
	return strings.Join(lines, "\n")
}
