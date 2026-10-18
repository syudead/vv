package store

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func sortKeyRow(t *testing.T, db *DB, table, name string) (sortKey string, searchVersion int64) {
	t.Helper()
	if err := db.sql.QueryRow(`select sort_key, search_version from `+table+` where name = ?`, name).
		Scan(&sortKey, &searchVersion); err != nil {
		t.Fatalf("%s の行を読めない (%s): %v", table, name, err)
	}
	return sortKey, searchVersion
}

func assertSortKeyWritten(t *testing.T, db *DB, table, name, when string) {
	t.Helper()
	key, version := sortKeyRow(t, db, table, name)
	if want := domain.NaturalSortKey(name); key != want || version != domain.SearchKeyVersion {
		t.Errorf("%sの %s %q: sort_key/version = %q/%d, want %q/%d",
			when, table, name, key, version, want, domain.SearchKeyVersion)
	}
}

func TestTagSortKeysMigrationAndRefresh(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	downTo(t, db, 29)
	if _, ok := tableColumns(t, db, "tag_names")["sort_key"]; ok {
		t.Fatal("Down のあとも tag_names.sort_key が残っている")
	}
	rejectedColumns := tableColumns(t, db, "rejected_tag_names")
	if _, ok := rejectedColumns["sort_key"]; ok {
		t.Fatal("Down のあとも rejected_tag_names.sort_key が残っている")
	}
	if _, ok := rejectedColumns["search_version"]; ok {
		t.Fatal("Down のあとも rejected_tag_names.search_version が残っている")
	}

	// Even current search keys become stale when the new sort-key column needs backfill.
	for i, name := range []string{"tag10", "tag2"} {
		if _, err := db.sql.Exec(`insert into tags (id, created_at) values (?, 1)`, i+1); err != nil {
			t.Fatal(err)
		}
		if _, err := db.sql.Exec(
			`insert into tag_names (name, tag_id, canonical, search_key, search_version) values (?, ?, 1, ?, ?)`,
			name, i+1, domain.FoldForMatch(name), domain.SearchKeyVersion,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.sql.Exec(`insert into tag_names (name, tag_id, canonical, search_key, search_version) values ('ｔａｇ1', 1, 0, '', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into rejected_tag_names (name, created_at) values ('犬3', 1), ('犬20', 1)`); err != nil {
		t.Fatal(err)
	}

	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for table, names := range map[string][]string{
		"tag_names":          {"tag10", "tag2", "ｔａｇ1"},
		"rejected_tag_names": {"犬3", "犬20"},
	} {
		for _, name := range names {
			if key, version := sortKeyRow(t, db, table, name); key != "" || version != 0 {
				t.Errorf("移行直後の %s %q: sort_key/version = %q/%d, want 空/0", table, name, key, version)
			}
		}
	}

	refreshed, err := db.Tags().RefreshSearchKeys(ctx)
	if err != nil {
		t.Fatalf("RefreshSearchKeys() error = %v", err)
	}
	if refreshed != 5 {
		t.Errorf("refreshed = %d, want 5（tag_names 3 行と rejected_tag_names 2 行）", refreshed)
	}
	for _, name := range []string{"tag10", "tag2", "ｔａｇ1"} {
		assertSortKeyWritten(t, db, "tag_names", name, "埋め直し後")
		key, _ := tagNameRow(t, db, name)
		if want := domain.FoldForMatch(name); key != want {
			t.Errorf("埋め直し後の %q の search_key = %q, want %q", name, key, want)
		}
	}
	for _, name := range []string{"犬3", "犬20"} {
		assertSortKeyWritten(t, db, "rejected_tag_names", name, "埋め直し後")
	}

	if k2, _ := sortKeyRow(t, db, "tag_names", "tag2"); k2 >= domain.NaturalSortKey("tag10") {
		t.Errorf("tag2 の鍵 %q が tag10 の鍵より前でない", k2)
	}

	again, err := db.Tags().RefreshSearchKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Errorf("2 回目の refreshed = %d, want 0", again)
	}
	assertTagInvariantsNow(t, db)
}

func TestTagSortKeysAreWrittenByEveryNameWriter(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "Anime 10")
	if err != nil {
		t.Fatal(err)
	}
	assertSortKeyWritten(t, db, "tag_names", "Anime 10", "作成後")

	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "アニメ 2", nil); err != nil {
		t.Fatal(err)
	}
	assertSortKeyWritten(t, db, "tag_names", "アニメ 2", "シノニム登録後")

	if _, err := db.Tags().RenameTag(ctx, tag.ID, "Anime 3"); err != nil {
		t.Fatal(err)
	}
	assertSortKeyWritten(t, db, "tag_names", "Anime 3", "改名後")

	applyTentative(t, db, []domain.VideoRef{{ContentKey: "key-a"}}, domain.VideoTagsAdd, "犬 1", "猫 12", "鳥 7")
	for _, name := range []string{"犬 1", "猫 12", "鳥 7"} {
		assertSortKeyWritten(t, db, "tag_names", name, "付与での作成後")
	}

	if _, err := db.Tags().RejectTag(ctx, tagIDByName(t, db, "犬 1")); err != nil {
		t.Fatal(err)
	}
	assertSortKeyWritten(t, db, "rejected_tag_names", "犬 1", "却下後")

	outcome, err := db.Tags().BatchTags(ctx, domain.TagBatchRequest{Action: domain.TagBatchReject},
		[]int64{tagIDByName(t, db, "猫 12"), tagIDByName(t, db, "鳥 7"), tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.AppliedIDs) != 2 {
		t.Fatalf("AppliedIDs = %v, want 2 件", outcome.AppliedIDs)
	}
	for _, name := range []string{"猫 12", "鳥 7"} {
		assertSortKeyWritten(t, db, "rejected_tag_names", name, "まとめての却下後")
	}
	assertTagInvariantsNow(t, db)
}

func TestMergeTagsKeepsSortKeys(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	target, err := db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}
	var sources []int64
	for _, name := range []string{"cat 2", "cat 10"} {
		source, err := db.Tags().CreateTag(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source.ID)
	}
	if _, err := db.Tags().AddSynonym(ctx, sources[0], "ねこ", nil); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	for _, name := range []string{"猫", "cat 2", "cat 10", "ねこ"} {
		before[name], _ = sortKeyRow(t, db, "tag_names", name)
	}

	if _, err := db.Tags().MergeTags(ctx, target.ID, sources); err != nil {
		t.Fatal(err)
	}
	for name, want := range before {
		key, _ := sortKeyRow(t, db, "tag_names", name)
		if key != want {
			t.Errorf("統合後の %q の sort_key = %q, want %q（変わらない）", name, key, want)
		}
		if id := tagIDByName(t, db, name); id != target.ID {
			t.Errorf("統合後の %q の tag_id = %d, want %d", name, id, target.ID)
		}
	}
	assertTagInvariantsNow(t, db)
}
