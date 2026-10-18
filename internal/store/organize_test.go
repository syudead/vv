package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func fiveItemGroup(t *testing.T, db *DB) (int64, []int64) {
	t.Helper()
	var videos []int64
	for _, key := range []string{"k1", "k2", "k3", "k4", "k5"} {
		videos = append(videos, looseVideo(t, db, key))
	}
	id := createGroup(t, db, "Five", domain.GroupAdd{VideoIDs: videos})
	var items []int64
	for _, key := range []string{"k1", "k2", "k3", "k4", "k5"} {
		items = append(items, groupItemID(t, db, id, "v:"+key))
	}
	return id, items
}

func groupItemIDsInOrder(t *testing.T, db *DB, id int64) []int64 {
	t.Helper()
	ids, err := queryStringsForTest(db, `select id from group_items where group_id = ? order by position`, id)
	if err != nil {
		t.Fatal(err)
	}
	return parseIDsForTest(t, ids)
}

func parseIDsForTest(t *testing.T, values []string) []int64 {
	t.Helper()
	out := make([]int64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

func TestOrganizeReorderAndUndo(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id, items := fiveItemGroup(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	organize := db.Organize()

	image, err := organize.Reorder(ctx, id, []int64{items[4], items[1], items[3]}, &items[0])
	if err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, id, "v:k1", "v:k5", "v:k2", "v:k4", "v:k3")
	want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{id}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("reorder events = %+v, want %+v", recorder.events, want)
	}
	recorder.events = nil

	if err := organize.Undo(ctx, image); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, id, "v:k1", "v:k2", "v:k3", "v:k4", "v:k5")
	if got := groupItemIDsInOrder(t, db, id); !slices.Equal(got, items) {
		t.Fatalf("item ids after undo = %v, want %v", got, items)
	}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("undo events = %+v, want %+v", recorder.events, want)
	}
}

func TestOrganizeReorderToTopNotifiesAncestors(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id, items := fiveItemGroup(t, db)
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{id}})
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if _, err := db.Organize().Reorder(ctx, id, []int64{items[2], items[2]}, nil); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, id, "v:k3", "v:k1", "v:k2", "v:k4", "v:k5")
	want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{id, parent}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("events = %+v, want %+v", recorder.events, want)
	}
}

func TestOrganizeUndoConflictsAfterAnotherChange(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id, items := fiveItemGroup(t, db)
	organize := db.Organize()

	first, err := organize.Reorder(ctx, id, []int64{items[4]}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := organize.Reorder(ctx, id, []int64{items[0]}, &items[3])
	if err != nil {
		t.Fatal(err)
	}
	before := groupTablesSnapshot(t, db)
	if err := organize.Undo(ctx, first); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo(first) error = %v, want ErrUndoConflict", err)
	}
	if after := groupTablesSnapshot(t, db); after != before {
		t.Fatalf("conflicting undo wrote:\n%s\nwant\n%s", after, before)
	}

	if err := organize.Undo(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := organize.Undo(ctx, first); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, id, "v:k1", "v:k2", "v:k3", "v:k4", "v:k5")
}

func TestOrganizeUndoConflictsAfterRemovalOrDissolve(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id, items := fiveItemGroup(t, db)
	organize := db.Organize()

	image, err := organize.Reorder(ctx, id, []int64{items[1]}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Groups().RemoveGroupItems(ctx, id, []int64{items[4]}); err != nil {
		t.Fatal(err)
	}
	if err := organize.Undo(ctx, image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo after removal error = %v", err)
	}
	if err := db.Groups().DissolveGroup(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := organize.Undo(ctx, image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo after dissolve error = %v", err)
	}
	if groupExists(t, db, id) {
		t.Fatal("undo brought the dissolved group back")
	}
}

func TestOrganizeReorderRefusesMissingTargets(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id, items := fiveItemGroup(t, db)
	other := createGroup(t, db, "Other", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "kx")}})
	otherItem := groupItemID(t, db, other, "v:kx")
	missing := int64(999999)
	before := groupTablesSnapshot(t, db)
	for name, c := range map[string]struct {
		group int64
		items []int64
		after *int64
	}{
		"無いグループ":      {group: missing, items: []int64{items[0]}},
		"ほかのグループの項目":  {group: id, items: []int64{items[0], otherItem}},
		"ほかのグループの目印":  {group: id, items: []int64{items[0]}, after: &otherItem},
		"無い目印":        {group: id, items: []int64{items[0]}, after: &missing},
		"ドラッグした項目が目印": {group: id, items: []int64{items[0], items[1]}, after: &items[1]},
	} {
		if _, err := db.Organize().Reorder(ctx, c.group, c.items, c.after); !errors.Is(err, domain.ErrGroupTargetNotFound) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
	if after := groupTablesSnapshot(t, db); after != before {
		t.Fatalf("refused reorder wrote:\n%s\nwant\n%s", after, before)
	}
}

func TestUndoImageRefusesUnknownTables(t *testing.T) {
	for _, data := range []string{
		`{"scopes":[{"table":"videos","column":"id","key":{"i":1}}]}`,
		`{"scopes":[{"table":"group_items","column":"position","key":{"i":1}}]}`,
		`{"scopes":[{"table":"group_items","column":"group_id","key":{"i":1},"columns":["id; drop table groups"]}]}`,
		`not json`,
	} {
		if _, err := decodeUndoImage([]byte(data)); err == nil {
			t.Errorf("decodeUndoImage(%s) accepted", data)
		}
	}
}
