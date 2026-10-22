package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
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

func smartFolderRows(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.sql.Query(`select id || ':' || coalesce(parent_id, 0) || ':' || name
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
	if root.ParentID != 0 || root.Sort != domain.SortAddedDesc ||
		root.CreatedAt != at.UnixMilli() || root.UpdatedAt != at.UnixMilli() {
		t.Fatalf("root = %+v", root)
	}
	child, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{
		Name: "  Cats  ", Rule: domain.SmartRule{Q: "cat", Favorite: true},
		Sort: domain.SortTitleAsc, ParentID: root.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.SmartFolder{
		ID: child.ID, ParentID: root.ID, Name: "Cats", Rule: domain.SmartRule{Q: "cat", Favorite: true},
		Sort: domain.SortTitleAsc, CreatedAt: at.UnixMilli(), UpdatedAt: at.UnixMilli(),
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
		"invalid rule":   {domain.SmartFolderCreate{Name: "A", Rule: domain.SmartRule{Person: -1}}, domain.ErrInvalidSmartRule},
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

func TestChangeSmartFolderSetsEveryMember(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	folder := createSmartFolder(t, db, "A", 0)
	later := time.UnixMilli(folder.UpdatedAt + 5000)
	smartFolderNow = func() time.Time { return later }
	t.Cleanup(func() { smartFolderNow = groupNow })

	name, sort, rule := "B", domain.SortDurationDesc, domain.SmartRule{Favorite: true}
	changed, err := db.SmartFolders().ChangeSmartFolder(ctx, folder.ID, domain.SmartFolderChange{Name: &name, Rule: &rule, Sort: &sort})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Name != "B" || changed.Sort != sort || !reflect.DeepEqual(changed.Rule, rule) ||
		changed.UpdatedAt != later.UnixMilli() || changed.CreatedAt != folder.CreatedAt {
		t.Fatalf("changed = %+v", changed)
	}
	random := domain.SortRandom
	if _, err := db.SmartFolders().ChangeSmartFolder(ctx, folder.ID, domain.SmartFolderChange{Sort: &random}); !errors.Is(err, domain.ErrInvalidSmartRule) {
		t.Fatalf("random sort error = %v", err)
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

func TestDeleteSmartFolderRenumbersItsPlaylists(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a := createSmartFolder(t, db, "A", 0)
	b := createSmartFolder(t, db, "B", a.ID)
	execSQL(t, db, `insert into playlists (id, name, created_at, updated_at) values (1, 'P', 1, 1), (2, 'Q', 1, 1)`)
	execSQL(t, db, `insert into playlist_items (playlist_id, content_key, smart_folder_id, position, added_at) values
		(1, 'key-a', null, 0, 1), (1, null, ?, 1, 1), (1, 'key-b', null, 2, 1), (1, null, ?, 3, 1),
		(2, 'key-a', null, 0, 1)`, a.ID, b.ID)

	if err := db.SmartFolders().DeleteSmartFolder(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	items, err := queryStringsForTest(db, `select playlist_id || ':' || content_key || ':' || position
		from playlist_items order by playlist_id, position`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1:key-a:0", "1:key-b:1", "2:key-a:0"}; !reflect.DeepEqual(items, want) {
		t.Fatalf("playlist_items = %v, want %v", items, want)
	}
	updated, err := queryStringsForTest(db, `select id || ':' || (updated_at > 1) from playlists order by id`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1:1", "2:0"}; !reflect.DeepEqual(updated, want) {
		t.Fatalf("playlists updated = %v, want %v", updated, want)
	}
}

func smartFolderRow(folder domain.SmartFolder) string {
	return fmt.Sprintf("%d:%d:%s", folder.ID, folder.ParentID, folder.Name)
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
	want := []domain.SmartRuleRef{
		{Kind: domain.SmartRefPerson, ID: 404}, {Kind: domain.SmartRefTag, ID: cat.ID}, {Kind: domain.SmartRefTagKind, ID: 7},
	}
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

// Stored rules lose the removed filter keys, which ParseSmartRule would otherwise reject, and keep the rest.
func TestRemovedListFiltersMigrationStripsStoredRules(t *testing.T) {
	ctx := context.Background()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	files, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 75); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `insert into smart_folders (id, name, title_key, rule, sort, created_at, updated_at) values
		(1, 'kept', 'kept', '{"q":"cat","watch":"unwatched","playable":true,"addedWithin":7,"minDuration":60,"maxDuration":600}', 'titleAsc', 1, 1),
		(2, 'emptied', 'emptied', '{"watch":"watched"}', 'titleAsc', 1, 1)`)
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[int64]string{1: `{"q":"cat"}`, 2: `{}`} {
		var rule string
		if err := db.sql.QueryRow(`select rule from smart_folders where id = ?`, id).Scan(&rule); err != nil {
			t.Fatal(err)
		}
		if rule != want {
			t.Errorf("rule of folder %d = %s, want %s", id, rule, want)
		}
	}
}
