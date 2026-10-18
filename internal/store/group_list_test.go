package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func listGroupNames(t *testing.T, db *DB, audience domain.Audience, query string, limit int) []string {
	t.Helper()
	var names []string
	cursor := ""
	for range 100 {
		page, err := db.Library().ListGroups(context.Background(), audience, query, cursor, limit)
		if err != nil {
			t.Fatal(err)
		}
		if limit > 0 && len(page.Groups) > limit {
			t.Fatalf("1 ページ %d 件, want %d 件以下", len(page.Groups), limit)
		}
		for _, group := range page.Groups {
			names = append(names, group.Name)
		}
		if page.NextCursor == "" {
			return names
		}
		cursor = page.NextCursor
	}
	t.Fatal("カーソルが尽きない")
	return nil
}

func TestListGroupsPagesInNameOrderSkippingHiddenGroups(t *testing.T) {
	db, _ := itemsFixture(t)
	inner := insertGroup(t, db, "Mix 2", itemKeyOf("/media/solo.mp4"))
	insertGroup(t, db, "Mix 10", inner)
	for index := range 5 {
		insertGroup(t, db, "Mix 1 hidden "+strconv.Itoa(index), "key-of-no-video")
	}
	insertGroup(t, db, "Mix empty")
	insertGroup(t, db, "Mix 3", itemKeyOf("/media/mixed/x.mp4"))

	want := []string{"Mix 2", "Mix 3", "Mix 10", "pair", "show"}
	for _, limit := range []int{1, 2, 0, domain.GroupListMaxLimit + 1} {
		if got := listGroupNames(t, db, domain.AudienceOwner, "", limit); !slices.Equal(got, want) {
			t.Errorf("limit %d の一覧 = %v, want %v", limit, got, want)
		}
	}
}

func TestListGroupsFiltersByNameIgnoringCase(t *testing.T) {
	db, _ := itemsFixture(t)
	insertGroup(t, db, "Summer Trip", itemKeyOf("/media/solo.mp4"))
	insertGroup(t, db, "ΟΣ", itemKeyOf("/media/mixed/x.mp4"))
	insertGroup(t, db, "trip hidden", "key-of-no-video")

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"  TRIP ", []string{"Summer Trip"}},
		{"ος", []string{"ΟΣ"}},
		{"σ", []string{"ΟΣ"}},
		{"   ", []string{"pair", "show", "Summer Trip", "ΟΣ"}},
		{"nothing", nil},
	} {
		if got := listGroupNames(t, db, domain.AudienceOwner, tc.query, 1); !slices.Equal(got, tc.want) {
			t.Errorf("q %q の一覧 = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestListGroupsForGuestReadsPublicVideosOnly(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	insertGroup(t, db, "public", itemKeyOf("/media/solo.mp4"), itemKeyOf("/media/mixed/x.mp4"))
	insertGroup(t, db, "private", itemKeyOf("/media/mixed/x.mp4"))
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{ids[fixturePath("/media/solo.mp4")]}, true); err != nil {
		t.Fatal(err)
	}

	page, err := db.Library().ListGroups(ctx, domain.AudienceGuest, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Groups) != 1 || page.Groups[0].Name != "public" || page.NextCursor != "" {
		t.Fatalf("ゲストの一覧 = %+v, want [public]", page.Groups)
	}
	if got := memberIDs(page.Groups[0]); !slices.Equal(got, []int64{ids[fixturePath("/media/solo.mp4")]}) {
		t.Errorf("ゲストの public のメンバー = %v", got)
	}
}

func TestListGroupsRejectsUnreadableCursor(t *testing.T) {
	db, _ := itemsFixture(t)
	for _, bad := range []string{
		"not-a-cursor",
		packCursor(groupItemsCursorKind, "1", "a"),
		packCursor(groupListCursorKind, "x", "a"),
		packCursor(groupListCursorKind, "0", "a"),
		packCursor(groupListCursorKind, "1"),
	} {
		if _, err := db.Library().ListGroups(context.Background(), domain.AudienceOwner, "", bad, 1); !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("カーソル %q = %v, want ErrInvalidCursor", bad, err)
		}
	}
}
