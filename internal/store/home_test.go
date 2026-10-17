package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// homeFixture は、タグ名 names ごとに 1 本の動画にそのタグを付けたデータベースと、名前からタグの id を
// 返す。
func homeFixture(t *testing.T, names ...string) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	tags := map[string]int64{}
	for i, name := range names {
		tag, err := db.Tags().CreateTag(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		tags[name] = tag.ID
		key := fmt.Sprintf("key-%d", i)
		upsertAll(t, db, listingFile(fixturePath(fmt.Sprintf("/media/%d.mp4", i)), name, key, i))
		attachTag(t, db, key, tag.ID)
	}
	return db, tags
}

// homeRows はホームの棚を並びの順に "name:owner_kept" で返す。
func homeRows(t *testing.T, db *DB) []string {
	t.Helper()
	shelves, err := db.Home().Shelves(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, shelf := range shelves {
		out = append(out, fmt.Sprintf("%s:%t", shelf.Name, shelf.OwnerKept))
	}
	return out
}

func homeChanges(recorder *eventRecorder) int {
	count := 0
	for _, event := range recorder.events {
		if _, ok := event.(domain.HomeChanged); ok {
			count++
		}
	}
	return count
}

func shelfByName(t *testing.T, db *DB, name string) domain.SmartFolder {
	t.Helper()
	shelves, err := db.Home().Shelves(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, shelf := range shelves {
		if shelf.Name == name {
			return shelf
		}
	}
	t.Fatalf("no shelf %q in %v", name, homeRows(t, db))
	return domain.SmartFolder{}
}

// 棚を並べ替えると、動かした棚の owner_kept が 1 になる（research.md R-7）。
func TestOrderingShelvesKeepsTheMovedShelf(t *testing.T) {
	db, tags := homeFixture(t, "Cats", "Dogs", "Birds")
	ctx := context.Background()
	seedLegacyShelves(t, db,
		tagShelf("Cats", tags["Cats"]), tagShelf("Dogs", tags["Dogs"]), tagShelf("Birds", tags["Birds"]))
	cats, dogs, birds := shelfByName(t, db, "Cats"), shelfByName(t, db, "Dogs"), shelfByName(t, db, "Birds")

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	if err := db.Home().SetShelves(ctx, []int64{birds.ID, cats.ID, dogs.ID}); err != nil {
		t.Fatal(err)
	}
	if got, want := homeRows(t, db), []string{"Birds:true", "Cats:false", "Dogs:false"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shelves = %v, want %v", got, want)
	}
	if got := homeChanges(recorder); got != 1 {
		t.Fatalf("HomeChanged published %d times, want 1", got)
	}

	if err := db.Home().SetShelves(ctx, []int64{birds.ID, cats.ID}); err != nil {
		t.Fatal(err)
	}
	removed, err := db.SmartFolders().SmartFolder(ctx, dogs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if removed.HomePosition != 0 || !removed.OwnerKept {
		t.Fatalf("removed shelf = %+v, want off the home and owner-kept", removed)
	}
	if err := db.Home().SetShelves(ctx, []int64{birds.ID, cats.ID}); err != nil {
		t.Fatal(err)
	}
	if got := homeChanges(recorder); got != 2 {
		t.Fatalf("HomeChanged published %d times, want 2 (none for an unchanged order)", got)
	}
}

// HomeChanged は確定後にだけ出て、巻き戻した書き込みでは出ない。
func TestHomeChangedIsNotPublishedAfterARollback(t *testing.T) {
	db, tags := homeFixture(t, "Cats", "Dogs")
	ctx := context.Background()
	seedLegacyShelves(t, db, tagShelf("Cats", tags["Cats"]))
	cats := shelfByName(t, db, "Cats")
	before := smartFolderRows(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if err := db.Home().SetShelves(ctx, []int64{cats.ID, 9999}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetShelves error = %v, want ErrNotFound", err)
	}
	if err := db.Home().SetShelves(ctx, []int64{cats.ID, cats.ID}); !errors.Is(err, domain.ErrInvalidHomeShelves) {
		t.Fatalf("SetShelves error = %v, want ErrInvalidHomeShelves", err)
	}
	if got := smartFolderRows(t, db); !reflect.DeepEqual(got, before) {
		t.Fatalf("folders = %v, want %v", got, before)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("events = %+v, want none", recorder.events)
	}
}

// seedLegacyShelves represents shelves created by the retired organizer.
func seedLegacyShelves(t *testing.T, db *DB, shelves ...domain.SmartFolder) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for i, shelf := range shelves {
		encoded, err := checkSmartFolder(&shelf)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := insertSmartFolder(ctx, tx, shelf, encoded, false, i+1); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func tagShelf(name string, tagID int64) domain.SmartFolder {
	return domain.SmartFolder{Name: name, Rule: domain.SmartRule{Tags: []int64{tagID}}, Sort: domain.SortAddedDesc}
}
