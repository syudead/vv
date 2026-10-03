package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 却下した名前のページ読み（specs/036-tag-admin-scale/data-model.md §2）。

// 036 #727: 却下した名前 250 個を limit 100 で 3 ページ読むと、名前の自然順で重複も抜けも無く、
// Total は 250、3 ページ目の NextCursor は空。外したあとの Total は減る。
func TestListRejectedTagNamesPages(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()

	want := make([]string, 0, 250)
	for i := 1; i <= 250; i++ {
		want = append(want, fmt.Sprintf("name %d", i))
	}
	// 挿入の順に依らないことを見るため、逆順に作る。
	created := slices.Clone(want)
	slices.Reverse(created)
	applyTentative(t, db, []domain.VideoRef{{ContentKey: "key-a"}}, domain.VideoTagsAdd, created...)
	ids := make([]int64, 0, len(created))
	for _, name := range created {
		ids = append(ids, tagIDByName(t, db, name))
	}
	if _, err := db.Tags().BatchTags(ctx, domain.TagBatchReject, ids); err != nil {
		t.Fatal(err)
	}

	var got []string
	cursor := ""
	for page := 1; page <= 3; page++ {
		result, err := db.Tags().ListRejectedTagNames(ctx, cursor, 100)
		if err != nil {
			t.Fatalf("ページ %d: %v", page, err)
		}
		if result.Total != 250 {
			t.Errorf("ページ %d: Total = %d, want 250", page, result.Total)
		}
		wantLen := 100
		if page == 3 {
			wantLen = 50
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

	if err := db.Tags().ForgetRejectedTagName(ctx, "name 2"); err != nil {
		t.Fatal(err)
	}
	result, err := db.Tags().ListRejectedTagNames(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 249 || slices.Contains(result.Items, "name 2") {
		t.Errorf("外したあと: Total = %d, name 2 を含む = %v", result.Total, slices.Contains(result.Items, "name 2"))
	}
}

// sort_key が同じ名前は name のバイト順で並び、ページの境目でも抜けない。解釈できないカーソルや
// ほかの一覧のカーソルは ErrInvalidCursor になる。
func TestListRejectedTagNamesTiesAndInvalidCursor(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	// 照合形が同じ（全角と半角・大文字と小文字、先頭の 0）で sort_key が同じになる組。
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
