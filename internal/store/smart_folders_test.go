package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func createSmartFolder(t *testing.T, db *DB, name string, parent int64) domain.SmartFolder {
	t.Helper()
	folder, err := db.SmartFolders().CreateSmartFolder(context.Background(),
		domain.SmartFolderCreate{Name: name, Rule: domain.SmartRule{Q: name}, ParentID: parent})
	if err != nil {
		t.Fatalf("CreateSmartFolder(%q) error = %v", name, err)
	}
	return folder
}

// smartFolderRows は smart_folders の行を id の順に "id:parent:name:owner_kept" で返す。
func smartFolderRows(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.sql.Query(`select id || ':' || coalesce(parent_id, 0) || ':' || name || ':' || owner_kept
		from smart_folders order by id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCreateSmartFolderNestsUnderItsParent(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	at := time.UnixMilli(1_700_000_000_000)
	smartFolderNow = func() time.Time { return at }
	t.Cleanup(func() { smartFolderNow = groupNow })

	root := createSmartFolder(t, db, "Root", 0)
	if root.ParentID != 0 || root.Sort != domain.SortAddedDesc || !root.OwnerKept ||
		root.CreatedAt != at.UnixMilli() || root.UpdatedAt != at.UnixMilli() {
		t.Fatalf("root = %+v", root)
	}
	five := 5
	child, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{
		Name: "  Cats  ", Rule: domain.SmartRule{Watch: domain.WatchUnwatched, AddedWithin: &five},
		Sort: domain.SortTitleAsc, ParentID: root.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.SmartFolder{
		ID: child.ID, ParentID: root.ID, Name: "Cats", Rule: domain.SmartRule{Watch: domain.WatchUnwatched, AddedWithin: &five},
		Sort: domain.SortTitleAsc, OwnerKept: true, CreatedAt: at.UnixMilli(), UpdatedAt: at.UnixMilli(),
	}
	if !reflect.DeepEqual(child, want) {
		t.Fatalf("child = %+v, want %+v", child, want)
	}
	var titleKey string
	if err := db.sql.QueryRow(`select title_key from smart_folders where id = ?`, child.ID).Scan(&titleKey); err != nil {
		t.Fatal(err)
	}
	if titleKey != domain.NaturalSortKey("Cats") {
		t.Fatalf("title_key = %q", titleKey)
	}
}

func TestCreateSmartFolderRefusesInvalidInput(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	cases := map[string]struct {
		create domain.SmartFolderCreate
		want   error
	}{
		"random sort":    {domain.SmartFolderCreate{Name: "A", Sort: domain.SortRandom}, domain.ErrInvalidSmartRule},
		"invalid rule":   {domain.SmartFolderCreate{Name: "A", Rule: domain.SmartRule{Watch: domain.WatchAll}}, domain.ErrInvalidSmartRule},
		"missing parent": {domain.SmartFolderCreate{Name: "A", ParentID: 99}, domain.ErrSmartFolderNotFound},
	}
	for name, c := range cases {
		if _, err := db.SmartFolders().CreateSmartFolder(ctx, c.create); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v, want %v", name, err, c.want)
		}
	}
	var nameErr *domain.InvalidGroupNameError
	if _, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{Name: "   "}); !errors.As(err, &nameErr) {
		t.Errorf("empty name error = %v", err)
	}
	if rows := smartFolderRows(t, db); len(rows) != 0 {
		t.Fatalf("rows = %v, want none", rows)
	}
}

func TestMoveSmartFolderUnderItsDescendantWritesNothing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a := createSmartFolder(t, db, "A", 0)
	b := createSmartFolder(t, db, "B", a.ID)
	c := createSmartFolder(t, db, "C", b.ID)
	before := smartFolderRows(t, db)

	name := "Renamed"
	for _, parent := range []int64{a.ID, b.ID, c.ID} {
		_, err := db.SmartFolders().ChangeSmartFolder(ctx, a.ID, domain.SmartFolderChange{Name: &name, ParentSet: true, ParentID: parent})
		if !errors.Is(err, domain.ErrSmartFolderCycle) {
			t.Fatalf("move A under %d error = %v, want ErrSmartFolderCycle", parent, err)
		}
	}
	if _, err := db.SmartFolders().ChangeSmartFolder(ctx, a.ID, domain.SmartFolderChange{ParentSet: true, ParentID: 99}); !errors.Is(err, domain.ErrSmartFolderNotFound) {
		t.Fatalf("move under a missing folder error = %v", err)
	}
	if _, err := db.SmartFolders().ChangeSmartFolder(ctx, 99, domain.SmartFolderChange{Name: &name}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("change a missing folder error = %v", err)
	}
	if after := smartFolderRows(t, db); !reflect.DeepEqual(after, before) {
		t.Fatalf("rows = %v, want %v", after, before)
	}

	moved, err := db.SmartFolders().ChangeSmartFolder(ctx, c.ID, domain.SmartFolderChange{ParentSet: true, ParentID: a.ID})
	if err != nil || moved.ParentID != a.ID {
		t.Fatalf("move C under A = %+v, %v", moved, err)
	}
	root, err := db.SmartFolders().ChangeSmartFolder(ctx, b.ID, domain.SmartFolderChange{ParentSet: true})
	if err != nil || root.ParentID != 0 {
		t.Fatalf("move B to the root = %+v, %v", root, err)
	}
}

func TestChangeSmartFolderSetsEveryMemberAndOwnerKept(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	folder := createSmartFolder(t, db, "A", 0)
	if _, err := db.sql.Exec(`update smart_folders set owner_kept = 0 where id = ?`, folder.ID); err != nil {
		t.Fatal(err)
	}
	later := time.UnixMilli(folder.UpdatedAt + 5000)
	smartFolderNow = func() time.Time { return later }
	t.Cleanup(func() { smartFolderNow = groupNow })

	name, sort, rule := "B", domain.SortDurationDesc, domain.SmartRule{Favorite: true}
	changed, err := db.SmartFolders().ChangeSmartFolder(ctx, folder.ID, domain.SmartFolderChange{Name: &name, Rule: &rule, Sort: &sort})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Name != "B" || changed.Sort != sort || !reflect.DeepEqual(changed.Rule, rule) ||
		!changed.OwnerKept || changed.UpdatedAt != later.UnixMilli() || changed.CreatedAt != folder.CreatedAt {
		t.Fatalf("changed = %+v", changed)
	}
	random := domain.SortRandom
	if _, err := db.SmartFolders().ChangeSmartFolder(ctx, folder.ID, domain.SmartFolderChange{Sort: &random}); !errors.Is(err, domain.ErrInvalidSmartRule) {
		t.Fatalf("random sort error = %v", err)
	}
}

func TestEverySmartFolderWriteSetsOwnerKept(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	parent := createSmartFolder(t, db, "Parent", 0)
	folder := createSmartFolder(t, db, "A", 0)
	name, sort, rule := "B", domain.SortTitleDesc, domain.SmartRule{Playable: true}
	for _, change := range []domain.SmartFolderChange{
		{Name: &name}, {Rule: &rule}, {Sort: &sort}, {ParentSet: true, ParentID: parent.ID}, {ParentSet: true},
	} {
		if _, err := db.sql.Exec(`update smart_folders set owner_kept = 0`); err != nil {
			t.Fatal(err)
		}
		changed, err := db.SmartFolders().ChangeSmartFolder(ctx, folder.ID, change)
		if err != nil || !changed.OwnerKept {
			t.Fatalf("ChangeSmartFolder(%+v) = %+v, %v", change, changed, err)
		}
	}
	var kept int
	if err := db.sql.QueryRow(`select owner_kept from smart_folders where id = ?`, parent.ID).Scan(&kept); err != nil || kept != 0 {
		t.Fatalf("an unchanged folder's owner_kept = %d, %v", kept, err)
	}
}

func TestDeleteSmartFolderDeletesItsDescendants(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a := createSmartFolder(t, db, "A", 0)
	b := createSmartFolder(t, db, "B", a.ID)
	c := createSmartFolder(t, db, "C", b.ID)
	other := createSmartFolder(t, db, "Other", 0)
	if _, err := db.sql.Exec(`insert into smart_folder_favorites (smart_folder_id, favorited_at) values (?, 1), (?, 1)`, c.ID, other.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.SmartFolders().DeleteSmartFolder(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if rows := smartFolderRows(t, db); !reflect.DeepEqual(rows, []string{smartFolderRow(other)}) {
		t.Fatalf("rows = %v", rows)
	}
	var favorites int
	if err := db.sql.QueryRow(`select count(*) from smart_folder_favorites`).Scan(&favorites); err != nil || favorites != 1 {
		t.Fatalf("favorites = %d, %v", favorites, err)
	}
	if err := db.SmartFolders().DeleteSmartFolder(ctx, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete again error = %v", err)
	}
}

func smartFolderRow(folder domain.SmartFolder) string {
	kept := 0
	if folder.OwnerKept {
		kept = 1
	}
	return fmt.Sprintf("%d:%d:%s:%d", folder.ID, folder.ParentID, folder.Name, kept)
}

func TestRuleNamingADeletedTagIsStoredAndReportedMissing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	cat, err := db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}
	dog, err := db.Tags().CreateTag(ctx, "犬")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{
		Name: "Animals", Rule: domain.SmartRule{Tags: []int64{dog.ID}, Person: 404},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Tags().DeleteTag(ctx, cat.ID); err != nil {
		t.Fatal(err)
	}

	rule := domain.SmartRule{Tags: []int64{cat.ID, dog.ID}, TagKind: 7}
	folder, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{Name: "Cats", Rule: rule, ParentID: parent.ID})
	if err != nil {
		t.Fatalf("a rule naming a deleted tag error = %v", err)
	}
	if !reflect.DeepEqual(folder.Rule, rule) {
		t.Fatalf("rule = %+v, want %+v", folder.Rule, rule)
	}
	missing, err := db.SmartFolders().MissingRefs(ctx, folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SmartRuleRef{{Kind: domain.SmartRefPerson, ID: 404}, {Kind: domain.SmartRefTag, ID: cat.ID}}
	if !reflect.DeepEqual(missing, want) {
		t.Fatalf("MissingRefs() = %+v, want %+v", missing, want)
	}
	if missing, err := db.SmartFolders().MissingRefs(ctx, parent.ID); err != nil || len(missing) != 1 {
		t.Fatalf("parent MissingRefs() = %+v, %v", missing, err)
	}
	if _, err := db.SmartFolders().MissingRefs(ctx, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing folder error = %v", err)
	}
}
