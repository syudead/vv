package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 複数の統合元を受ける統合（specs/036-tag-admin-scale/data-model.md §2・§3）。

// countingTx は取引に流れた文を数える。mergeTagsInto の文の数と、統合先の tagByID が何回
// 走ったか（tagByID だけが読む `select created_at from tags where id = ?` の回数）を見る。
type countingTx struct {
	tx         *sql.Tx
	execs      int
	tagByIDRun int
}

func (c *countingTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	c.execs++
	return c.tx.ExecContext(ctx, query, args...)
}

func (c *countingTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.tx.QueryContext(ctx, query, args...)
}

func (c *countingTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if strings.Contains(query, "select created_at from tags where id = ?") {
		c.tagByIDRun++
	}
	return c.tx.QueryRowContext(ctx, query, args...)
}

func tagNamesOf(t *testing.T, db *DB, tagID int64) map[string]bool {
	t.Helper()
	rows, err := db.sql.Query(`select name, canonical from tag_names where tag_id = ?`, tagID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	names := map[string]bool{}
	for rows.Next() {
		var name string
		var canonical bool
		if err := rows.Scan(&name, &canonical); err != nil {
			t.Fatal(err)
		}
		names[name] = canonical
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func assignmentCount(t *testing.T, db *DB, contentKey string, tagID int64) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_tags where content_key = ? and tag_id = ?`, contentKey, tagID).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestMergeTagsMovesSeveralSourcesAndSkipsMissingOnes(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tags := db.Tags()

	create := func(name string) int64 {
		tag, err := tags.CreateTag(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		return tag.ID
	}
	target, a, b := create("T"), create("A"), create("B")
	if _, err := tags.AddSynonym(ctx, a, "Aの別名", nil); err != nil {
		t.Fatal(err)
	}
	// 統合先を仮のタグにして、統合で確定になることを見る。
	if _, err := db.sql.Exec(`update tags set tentative = 1 where id = ?`, target); err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-1", a)
	attachTag(t, db, "key-2", a)
	attachTag(t, db, "key-2", b)
	attachTag(t, db, "key-2", target)
	attachTag(t, db, "key-3", b)
	const missing = int64(999_999)

	outcome, err := tags.MergeTags(ctx, target, []int64{a, missing, b, a, target})
	if err != nil {
		t.Fatalf("MergeTags() error = %v", err)
	}
	if want := []int64{missing}; !reflect.DeepEqual(outcome.NotFoundIDs, want) {
		t.Errorf("NotFoundIDs = %v, want %v", outcome.NotFoundIDs, want)
	}
	if outcome.Tag.ID != target || outcome.Tag.Name != "T" || outcome.Tag.Tentative {
		t.Errorf("Tag = %+v, want 確定した T", outcome.Tag)
	}
	if want := []string{"A", "Aの別名", "B"}; !reflect.DeepEqual(outcome.Tag.Synonyms, want) {
		t.Errorf("Synonyms = %v, want %v", outcome.Tag.Synonyms, want)
	}

	for _, key := range []string{"key-1", "key-2", "key-3"} {
		if got := assignmentCount(t, db, key, target); got != 1 {
			t.Errorf("%s の T の付与数 = %d, want 1", key, got)
		}
	}
	for _, id := range []int64{a, b} {
		if tagExists(t, db, id) {
			t.Errorf("統合元 %d が残っている", id)
		}
	}
	want := map[string]bool{"T": true, "A": false, "Aの別名": false, "B": false}
	if got := tagNamesOf(t, db, target); !reflect.DeepEqual(got, want) {
		t.Errorf("T の名前 = %v, want %v", got, want)
	}
	assertTagInvariantsNow(t, db)
}

func TestMergeTagsReportsMissingTarget(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	source, err := db.Tags().CreateTag(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().MergeTags(ctx, 999_999, []int64{source.ID}); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("MergeTags(無い統合先) error = %v, want ErrTagNotFound", err)
	}
	if !tagExists(t, db, source.ID) {
		t.Error("統合先が無いのに統合元が消えた")
	}
}

func TestMergeTagsWithOnlyMissingSourcesKeepsTarget(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target, err := db.Tags().CreateTag(ctx, "T")
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := db.Tags().MergeTags(ctx, target.ID, []int64{999_998, 999_999})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(outcome.Tag, target) {
		t.Errorf("Tag = %+v, want 変わらない %+v", outcome.Tag, target)
	}
	if want := []int64{999_998, 999_999}; !reflect.DeepEqual(outcome.NotFoundIDs, want) {
		t.Errorf("NotFoundIDs = %v, want %v", outcome.NotFoundIDs, want)
	}
}

// insertMergeSources は統合元のタグ n 個を直接作り、それぞれ 1 本の付与（content_key は
// keys 種類を巡回）とシノニム 1 つを持たせる。作った id を返す。
func insertMergeSources(t *testing.T, db *DB, n, keys int) []int64 {
	t.Helper()
	tx, err := db.sql.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	ids := make([]int64, 0, n)
	for i := range n {
		res, err := tx.Exec(`insert into tags (created_at, tentative) values (1, ?)`, i%2)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []struct {
			name      string
			canonical int
		}{{fmt.Sprintf("s%d", i), 1}, {fmt.Sprintf("s%d-別名", i), 0}} {
			if _, err := tx.Exec(`insert into tag_names (name, tag_id, canonical, search_key, search_version)
				values (?, ?, ?, ?, ?)`, name.name, id, name.canonical,
				domain.FoldForMatch(name.name), domain.SearchKeyVersion); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.Exec(`insert into video_tags (content_key, tag_id, created_at) values (?, ?, 1)`,
			fmt.Sprintf("key-%d", i%keys), id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// mergeCounted は mergeTagsInTx を数える取引で走らせて確定し、結果と数えた値を返す。
func mergeCounted(t *testing.T, db *DB, targetID int64, sourceIDs []int64) (domain.TagMergeOutcome, *countingTx) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	counted := &countingTx{tx: tx}
	outcome, err := mergeTagsInTx(ctx, counted, targetID, sourceIDs)
	if err != nil {
		t.Fatalf("mergeTagsInTx() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return outcome, counted
}

func TestMergeTagsHandlesMaxTagBatchSourcesInFixedStatements(t *testing.T) {
	const keys = 50
	ctx := context.Background()

	// 統合元 1 個のときの文の数を基準にとる。
	small := migratedDB(t)
	smallTarget, err := small.Tags().CreateTag(ctx, "T")
	if err != nil {
		t.Fatal(err)
	}
	_, baseline := mergeCounted(t, small, smallTarget.ID, insertMergeSources(t, small, 1, keys))

	db := migratedDB(t)
	target, err := db.Tags().CreateTag(ctx, "T")
	if err != nil {
		t.Fatal(err)
	}
	sources := insertMergeSources(t, db, domain.MaxTagBatch, keys)
	outcome, counted := mergeCounted(t, db, target.ID, sources)

	if counted.execs != baseline.execs {
		t.Errorf("統合元 %d 個の文の数 = %d, 1 個のとき = %d（統合元の数によってはいけない）",
			domain.MaxTagBatch, counted.execs, baseline.execs)
	}
	if counted.tagByIDRun != 1 {
		t.Errorf("統合先の tagByID = %d 回, want 1", counted.tagByIDRun)
	}
	if len(outcome.NotFoundIDs) != 0 || outcome.Tag.Tentative {
		t.Errorf("outcome = NotFoundIDs %v, Tentative %v", outcome.NotFoundIDs, outcome.Tag.Tentative)
	}
	if got, want := len(outcome.Tag.Synonyms), 2*domain.MaxTagBatch; got != want {
		t.Errorf("統合先のシノニムの数 = %d, want %d", got, want)
	}

	var remaining, assignments, names int
	if err := db.sql.QueryRow(`select count(*) from tags`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow(`select count(*) from video_tags where tag_id = ?`, target.ID).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow(`select count(*) from tag_names where tag_id = ?`, target.ID).Scan(&names); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 || assignments != keys || names != 1+2*domain.MaxTagBatch {
		t.Errorf("残ったタグ %d・統合先の付与 %d・名前 %d, want 1・%d・%d",
			remaining, assignments, names, keys, 1+2*domain.MaxTagBatch)
	}
	assertTagInvariantsNow(t, db)
}
