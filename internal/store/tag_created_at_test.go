package store

import (
	"context"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// タグの作った日（specs/036-tag-admin-scale/data-model.md §1・§2、research.md R-8）。

// TestCreateTagReturnsCreatedAtMatchingListTags は、CreateTag の戻り値に作った時刻（秒）が
// 載り、続く ListTags の同じタグと等しいことを確かめる。
func TestCreateTagReturnsCreatedAtMatchingListTags(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	before := time.Now().Unix()
	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	after := time.Now().Unix()
	if tag.CreatedAt.IsZero() {
		t.Fatal("CreatedAt がゼロ時刻")
	}
	if got := tag.CreatedAt.Unix(); got < before || got > after {
		t.Errorf("CreatedAt = %d, want %d〜%d", got, before, after)
	}
	var stored int64
	if err := db.sql.QueryRow(`select created_at from tags where id = ?`, tag.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if tag.CreatedAt.Unix() != stored {
		t.Errorf("CreatedAt = %d, tags.created_at = %d", tag.CreatedAt.Unix(), stored)
	}

	tagsPage, err := db.Tags().ListTags(ctx, domain.TagListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	tags := tagsPage.Items
	if len(tags) != 1 || !tags[0].CreatedAt.Equal(tag.CreatedAt) {
		t.Errorf("ListTags = %#v, want CreatedAt %v", tags, tag.CreatedAt)
	}
}

// TestTagOperationsReturnCreatedAt は、ListTags の全件と、改名・確定・シノニム・統合の
// 戻り値に、保存した tags.created_at がそのまま載ることを確かめる。統合は MergeTags の
// 統合先。
func TestTagOperationsReturnCreatedAt(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	names := []string{"旅行", "猫", "犬", "仮"}
	ids := make(map[string]int64, len(names))
	for i, name := range names {
		tag, err := db.Tags().CreateTag(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = tag.ID
		// 区別できるよう、作った時刻をタグごとに別の値にする。
		if _, err := db.sql.Exec(`update tags set created_at = ? where id = ?`, 1_700_000_000+int64(i)*86400, tag.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.sql.Exec(`update tags set tentative = 1 where id = ?`, ids["仮"]); err != nil {
		t.Fatal(err)
	}
	want := func(name string) time.Time {
		var sec int64
		if err := db.sql.QueryRow(`select created_at from tags where id = ?`, ids[name]).Scan(&sec); err != nil {
			t.Fatal(err)
		}
		return time.Unix(sec, 0)
	}

	tagsPage, err := db.Tags().ListTags(ctx, domain.TagListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	tags := tagsPage.Items
	if len(tags) != len(names) {
		t.Fatalf("ListTags = %d 件, want %d", len(tags), len(names))
	}
	for _, tag := range tags {
		if !tag.CreatedAt.Equal(want(tag.Name)) {
			t.Errorf("ListTags の %s: CreatedAt = %v, want %v", tag.Name, tag.CreatedAt, want(tag.Name))
		}
	}

	check := func(label, name string, got time.Time) {
		t.Helper()
		if !got.Equal(want(name)) {
			t.Errorf("%s: CreatedAt = %v, want %v", label, got, want(name))
		}
	}

	renamed, err := db.Tags().RenameTag(ctx, ids["旅行"], "旅")
	if err != nil {
		t.Fatal(err)
	}
	check("RenameTag", "旅行", renamed.CreatedAt)

	confirmed, err := db.Tags().ConfirmTag(ctx, ids["仮"])
	if err != nil {
		t.Fatal(err)
	}
	check("ConfirmTag", "仮", confirmed.CreatedAt)

	withSynonym, err := db.Tags().AddSynonym(ctx, ids["猫"], "ねこ", nil)
	if err != nil {
		t.Fatal(err)
	}
	check("AddSynonym", "猫", withSynonym.CreatedAt)

	merged, err := db.Tags().MergeTags(ctx, ids["猫"], []int64{ids["犬"]})
	if err != nil {
		t.Fatal(err)
	}
	check("MergeTags", "猫", merged.Tag.CreatedAt)
}
