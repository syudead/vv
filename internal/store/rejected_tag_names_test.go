package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Rejected names retain natural order across full and partial pages; forgetting one reduces the total.
func TestListRejectedTagNamesPages(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()

	want := []string{"name 1", "name 2", "name 3", "name 10", "name 11"}
	// Insert in reverse order to prove output does not depend on insertion.
	created := slices.Clone(want)
	slices.Reverse(created)
	applyTentative(t, db, []domain.VideoRef{{ContentKey: "key-a"}}, domain.VideoTagsAdd, created...)
	ids := make([]int64, 0, len(created))
	for _, name := range created {
		ids = append(ids, tagIDByName(t, db, name))
	}
	if _, err := db.Tags().BatchTags(ctx, domain.TagBatchRequest{Action: domain.TagBatchReject}, ids); err != nil {
		t.Fatal(err)
	}

	var got []string
	cursor := ""
	for page := 1; page <= 3; page++ {
		result, err := db.Tags().ListRejectedTagNames(ctx, cursor, 2)
		if err != nil {
			t.Fatalf("ページ %d: %v", page, err)
		}
		if result.Total != 5 {
			t.Errorf("ページ %d: Total = %d, want 5", page, result.Total)
		}
		wantLen := 2
		if page == 3 {
			wantLen = 1
		}
		if len(result.Items) != wantLen {
			t.Errorf("ページ %d: %d 件, want %d", page, len(result.Items), wantLen)
		}
		if page < 3 && result.NextCursor == "" {
			t.Fatalf("ページ %d: NextCursor が空", page)
		}
		if page == 3 && result.NextCursor != "" {
			t.Errorf("3 ページ目の NextCursor = %q, want 空", result.NextCursor)
		}
		got = append(got, result.Items...)
		cursor = result.NextCursor
	}
	if !slices.Equal(got, want) {
		t.Errorf("3 ページを繋いだ名前が自然順で重複も抜けも無い形でない: 先頭 %v", got[:min(len(got), 12)])
	}

	if removed, err := db.Tags().ForgetRejectedTagName(ctx, "name 2"); err != nil || !removed {
		t.Fatalf("ForgetRejectedTagName = %v, %v, want true", removed, err)
	}
	result, err := db.Tags().ListRejectedTagNames(ctx, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 4 || slices.Contains(result.Items, "name 2") {
		t.Errorf("外したあと: Total = %d, name 2 を含む = %v", result.Total, slices.Contains(result.Items, "name 2"))
	}
}

func TestListRejectedTagNamesTiesAndInvalidCursor(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	// Use spellings that normalize to the same natural-sort key.
	for _, name := range []string{"ＡＢＣ", "abc", "ABC", "x01", "x1"} {
		rejectName(t, db, name)
	}
	want := []string{"ABC", "abc", "ＡＢＣ", "x01", "x1"}

	var got []string
	cursor := ""
	for range len(want) + 1 {
		result, err := db.Tags().ListRejectedTagNames(ctx, cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, result.Items...)
		if cursor = result.NextCursor; cursor == "" {
			break
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("1 件ずつ読んだ名前 = %v, want %v", got, want)
	}

	invalid := []string{
		"!!",
		encodeCursorFields(cursorFields{sort: "name", value: "a"}),
		encodeCursorFields(cursorFields{sort: rejectedTagNameCursorSort, value: "no separator"}),
	}
	for _, cursor := range invalid {
		if _, err := db.Tags().ListRejectedTagNames(ctx, cursor, 10); !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("カーソル %q: err = %v, want ErrInvalidCursor", cursor, err)
		}
	}
}

// Deferred read snapshots must succeed while another connection holds the write slot.
func TestListRejectedTagNamesDoesNotWaitForTheWriter(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	rejectName(t, db, "held")

	writer, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("書き込みトランザクションを開始できない: %v", err)
	}
	defer func() { _ = writer.Rollback() }()

	done := make(chan error, 1)
	go func() {
		_, err := db.Tags().ListRejectedTagNames(ctx, "", 100)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("書き込み中の却下した名前の読み出しに失敗した: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("却下した名前の読み出しが書き込みの枠を待った")
	}
}
