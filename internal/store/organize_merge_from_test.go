package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func mergeFrom(t *testing.T, db *DB, into, from int64, items ...domain.OrganizeRef) []byte {
	t.Helper()
	image, err := db.Organize().Merge(context.Background(), items, into, from)
	if err != nil {
		t.Fatalf("Merge(%v, %d, from %d) error = %v", items, into, from, err)
	}
	return image
}

func TestOrganizeMergeFromLeavesTheOpenGroupAndUndoes(t *testing.T) {
	db := migratedDB(t)
	v, ya, xa, xb := looseVideo(t, db, "v"), looseVideo(t, db, "ya"), looseVideo(t, db, "xa"), looseVideo(t, db, "xb")
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{xa, xb}})
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{ya}})
	g := createGroup(t, db, "G", domain.GroupAdd{VideoIDs: []int64{v}, GroupIDs: []int64{x, y}})
	setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{x}, Favorite: true})
	playlist := playlistWith(t, db, "P", domain.PlaylistAdd{GroupIDs: []int64{x}})
	playlistBefore := playlistEntryIDs(t, db, playlist)
	assertEntries(t, db, g, "v:v", fmt.Sprintf("g:%d", x), fmt.Sprintf("g:%d", y))
	before := mergeSnapshot(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	image := mergeFrom(t, db, y, g, videoRef(v), groupRef(x))

	assertEntries(t, db, g, fmt.Sprintf("g:%d", y))
	assertEntries(t, db, y, "v:ya", "v:v", "v:xa", "v:xb")
	if groupExists(t, db, x) {
		t.Fatal("the merged group still exists")
	}
	want := []domain.Event{domain.GroupsChanged{GroupIDs: normalizeIDs([]int64{g, x, y})}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("merge events = %+v, want %+v", recorder.events, want)
	}

	undo(t, db, image)
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
	if !groupFavorite(t, db, x) || groupFavorite(t, db, y) {
		t.Fatal("undo did not put the favorite back on the merged group")
	}
	if got := playlistEntryIDs(t, db, playlist); !slices.Equal(got, playlistBefore) {
		t.Fatalf("playlist after undo = %v, want %v", got, playlistBefore)
	}
}

func TestOrganizeMergeFromRefusesCardsOutsideIt(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	v, outside := looseVideo(t, db, "v"), looseVideo(t, db, "outside")
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "xa")}})
	other := createGroup(t, db, "Other", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "oa")}})
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ya")}})
	g := createGroup(t, db, "G", domain.GroupAdd{VideoIDs: []int64{v}, GroupIDs: []int64{x, y}})
	before := mergeSnapshot(t, db)
	for name, c := range map[string]struct {
		from  int64
		items []domain.OrganizeRef
	}{
		"from にない動画":   {from: g, items: []domain.OrganizeRef{videoRef(v), videoRef(outside), groupRef(x)}},
		"from にないグループ": {from: g, items: []domain.OrganizeRef{groupRef(x), groupRef(other)}},
		"無い from":      {from: 999999, items: []domain.OrganizeRef{groupRef(x)}},
	} {
		if _, err := db.Organize().Merge(ctx, c.items, y, c.from); !errors.Is(err, domain.ErrGroupTargetNotFound) {
			t.Errorf("%s: error = %v, want ErrGroupTargetNotFound", name, err)
		}
	}
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("refused merge wrote:\n%s\nwant\n%s", after, before)
	}
}

func TestOrganizeMergeFromAnAutomaticGroupWritesExclusions(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	k1 := upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	show := readGroups(t, db)[0].ID
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "xa")}})
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ya")}})
	if _, err := db.Groups().AddGroupItems(ctx, show, domain.GroupAdd{GroupIDs: []int64{x, y}}); err != nil {
		t.Fatal(err)
	}
	before := mergeSnapshot(t, db)

	image := mergeFrom(t, db, y, show, videoRef(k1), groupRef(x))
	if got := groupExclusions(t, db, show); !slices.Equal(got, []string{"k1"}) {
		t.Fatalf("exclusions = %v", got)
	}
	rebuildIndex(t, db)
	assertEntries(t, db, show, "v:k2", fmt.Sprintf("g:%d", y))
	assertEntries(t, db, y, "v:ya", "v:k1", "v:xa")

	undo(t, db, image)
	if after := mergeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
}
