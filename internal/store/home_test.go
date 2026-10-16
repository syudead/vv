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

func tagShelf(name string, tagID int64) domain.ProposedShelf {
	return domain.ProposedShelf{Name: name, Rule: domain.SmartRule{Tags: []int64{tagID}}}
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

func applyProposal(t *testing.T, db *DB, run domain.HomeOrganizeRun, proposal ...domain.ProposedShelf) domain.HomeApplied {
	t.Helper()
	applied, err := db.Home().ApplyProposal(context.Background(), run, proposal)
	if err != nil {
		t.Fatalf("ApplyProposal(%s) error = %v", run, err)
	}
	return applied
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

// rebuild は、案が条件を繰り返さなかった整理役のフォルダを消し、繰り返したフォルダを同じ id と
// お気に入りのまま案の名前で残す（research.md R-8）。
func TestRebuildKeepsRepeatedOrganizerFoldersAndDeletesTheRest(t *testing.T) {
	db, tags := homeFixture(t, "Cats", "Dogs", "Birds")
	applied := applyProposal(t, db, domain.HomeRebuild, tagShelf("Cats", tags["Cats"]), tagShelf("Dogs", tags["Dogs"]))
	if applied != (domain.HomeApplied{Added: 2, Changed: true}) {
		t.Fatalf("first rebuild = %+v", applied)
	}
	cats := shelfByName(t, db, "Cats")
	dogs := shelfByName(t, db, "Dogs")
	if _, err := db.sql.Exec(`insert into smart_folder_favorites (smart_folder_id, favorited_at) values (?, 1)`, cats.ID); err != nil {
		t.Fatal(err)
	}

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	applied = applyProposal(t, db, domain.HomeRebuild, tagShelf("Birds", tags["Birds"]), tagShelf("Kittens", tags["Cats"]))
	if applied != (domain.HomeApplied{Added: 1, Removed: 1, Changed: true}) {
		t.Fatalf("second rebuild = %+v", applied)
	}
	if got, want := homeRows(t, db), []string{"Birds:false", "Kittens:false"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shelves = %v, want %v", got, want)
	}
	kittens := shelfByName(t, db, "Kittens")
	if kittens.ID != cats.ID || kittens.OwnerKept {
		t.Fatalf("kept folder = %+v, want id %d with owner_kept 0", kittens, cats.ID)
	}
	var favorite bool
	if err := db.sql.QueryRow(`select exists (select 1 from smart_folder_favorites where smart_folder_id = ?)`, cats.ID).
		Scan(&favorite); err != nil || !favorite {
		t.Fatalf("favorite = %t, %v; want kept", favorite, err)
	}
	if _, err := db.SmartFolders().SmartFolder(context.Background(), dogs.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Dogs folder error = %v, want ErrNotFound", err)
	}
	if got := homeChanges(recorder); got != 1 {
		t.Fatalf("HomeChanged published %d times, want 1", got)
	}
}

// rebuild は owner-kept の棚と同じ条件の案を捨て、owner-kept のフォルダをそのまま残す。
func TestRebuildLeavesOwnerKeptShelvesAlone(t *testing.T) {
	db, tags := homeFixture(t, "Cats", "Dogs")
	ctx := context.Background()
	applyProposal(t, db, domain.HomeRebuild, tagShelf("Cats", tags["Cats"]))
	cats := shelfByName(t, db, "Cats")
	name := "My cats"
	if _, err := db.SmartFolders().ChangeSmartFolder(ctx, cats.ID, domain.SmartFolderChange{Name: &name}); err != nil {
		t.Fatal(err)
	}
	before, err := db.SmartFolders().SmartFolder(ctx, cats.ID)
	if err != nil {
		t.Fatal(err)
	}

	applied := applyProposal(t, db, domain.HomeRebuild, tagShelf("Cats again", tags["Cats"]), tagShelf("Dogs", tags["Dogs"]))
	if applied != (domain.HomeApplied{Added: 1, Changed: true}) {
		t.Fatalf("rebuild = %+v", applied)
	}
	if got, want := homeRows(t, db), []string{"My cats:true", "Dogs:false"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shelves = %v, want %v", got, want)
	}
	after, err := db.SmartFolders().SmartFolder(ctx, cats.ID)
	if err != nil {
		t.Fatal(err)
	}
	before.HomePosition = after.HomePosition
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("owner-kept folder = %+v, want %+v", after, before)
	}
}

// extend は最後の棚の後に棚を足し、今ある棚の名前も条件も変えない。
func TestExtendAddsAfterTheLastShelfAndRenamesNothing(t *testing.T) {
	db, tags := homeFixture(t, "Cats", "Dogs", "Birds")
	applyProposal(t, db, domain.HomeRebuild, tagShelf("Cats", tags["Cats"]))
	cats := shelfByName(t, db, "Cats")

	applied := applyProposal(t, db, domain.HomeExtend,
		tagShelf("Kittens", tags["Cats"]), tagShelf("Dogs", tags["Dogs"]), tagShelf("Birds", tags["Birds"]))
	if applied != (domain.HomeApplied{Added: 2, Changed: true}) {
		t.Fatalf("extend = %+v", applied)
	}
	if got, want := homeRows(t, db), []string{"Cats:false", "Dogs:false", "Birds:false"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shelves = %v, want %v", got, want)
	}
	after := shelfByName(t, db, "Cats")
	if after.ID != cats.ID || !reflect.DeepEqual(after.Rule, cats.Rule) || after.HomePosition != cats.HomePosition {
		t.Fatalf("existing shelf = %+v, want %+v", after, cats)
	}
	if shelfByName(t, db, "Dogs").HomePosition <= cats.HomePosition {
		t.Fatal("the added shelf is not after the last shelf")
	}

	if applied := applyProposal(t, db, domain.HomeExtend, tagShelf("Dogs", tags["Dogs"])); applied.Changed {
		t.Fatalf("extend of a repeated rule = %+v, want unchanged", applied)
	}
}

// どの動画にも一致しない案と規則に合わない案は捨てる。
func TestProposalsThatMatchNoVideoOrBreakTheRulesAreDropped(t *testing.T) {
	db, tags := homeFixture(t, "Cats")
	ctx := context.Background()
	empty, err := db.Tags().CreateTag(ctx, "Empty")
	if err != nil {
		t.Fatal(err)
	}
	two := domain.ProposedShelf{Name: "Two", Rule: domain.SmartRule{Tags: []int64{tags["Cats"], empty.ID}}}
	query := domain.ProposedShelf{Name: "Query", Rule: domain.SmartRule{Q: "Cats"}}
	for _, run := range []domain.HomeOrganizeRun{domain.HomeRebuild, domain.HomeExtend} {
		applied := applyProposal(t, db, run, tagShelf("Empty", empty.ID), two, query, tagShelf("", tags["Cats"]),
			tagShelf("Missing", 9999), domain.ProposedShelf{Name: "Nobody", Rule: domain.SmartRule{Person: 9999}})
		if applied.Changed {
			t.Fatalf("%s = %+v, want unchanged", run, applied)
		}
	}
	if rows := smartFolderRows(t, db); len(rows) != 0 {
		t.Fatalf("folders = %v, want none", rows)
	}
}

// 棚を並べ替えると、動かした棚の owner_kept が 1 になる（research.md R-7）。
func TestOrderingShelvesKeepsTheMovedShelf(t *testing.T) {
	db, tags := homeFixture(t, "Cats", "Dogs", "Birds")
	ctx := context.Background()
	applyProposal(t, db, domain.HomeRebuild,
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
	applyProposal(t, db, domain.HomeRebuild, tagShelf("Cats", tags["Cats"]))
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
	if _, err := db.sql.Exec(`create trigger refuse_delete before delete on smart_folders
		begin select raise(abort, 'refused'); end`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Home().ApplyProposal(ctx, domain.HomeRebuild, []domain.ProposedShelf{tagShelf("Dogs", tags["Dogs"])}); err == nil {
		t.Fatal("ApplyProposal error = nil, want the refused delete")
	}
	if got := smartFolderRows(t, db); !reflect.DeepEqual(got, before) {
		t.Fatalf("folders = %v, want %v", got, before)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("events = %+v, want none", recorder.events)
	}
}

// 整理役の settings のキーは書いた値を読み戻し、消せば無いものになる。
func TestOrganizerSettingsRoundTrip(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	home := db.Home()
	if _, found, err := home.PendingOrganize(ctx); err != nil || found {
		t.Fatalf("pending = %t, %v; want none", found, err)
	}
	if err := home.SavePendingOrganize(ctx, domain.HomeExtend); err != nil {
		t.Fatal(err)
	}
	if run, found, err := home.PendingOrganize(ctx); err != nil || !found || run != domain.HomeExtend {
		t.Fatalf("pending = %q, %t, %v", run, found, err)
	}
	if err := home.ClearPendingOrganize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := home.PendingOrganize(ctx); found {
		t.Fatal("pending survived the clear")
	}
	if err := home.SavePendingOrganize(ctx, "later"); err == nil {
		t.Fatal("SavePendingOrganize accepted an unknown run")
	}

	last := domain.HomeOrganizeLast{Run: domain.HomeRebuild, Outcome: domain.HomeOrganizeDone, FinishedAt: 1_700_000_000_000, Added: 3, Removed: 1}
	if err := home.SaveLastOrganize(ctx, last); err != nil {
		t.Fatal(err)
	}
	if got, found, err := home.LastOrganize(ctx); err != nil || !found || got != last {
		t.Fatalf("last = %+v, %t, %v; want %+v", got, found, err, last)
	}
	var raw string
	if err := db.sql.QueryRow(`select value from settings where key = 'home.organize.last'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if want := `{"run":"rebuild","outcome":"done","finishedAt":1700000000000,"added":3,"removed":1}`; raw != want {
		t.Fatalf("stored = %s, want %s", raw, want)
	}
}
