package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func groupIDOf(t *testing.T, db *DB, dir string) int64 {
	t.Helper()
	var id int64
	if err := db.sql.QueryRow(`select id from groups where origin_folder = ?`, domain.FolderKey(dir)).Scan(&id); err != nil {
		t.Fatalf("%s の自動グループ: %v", dir, err)
	}
	return id
}

// String items denote video user keys; int64 items denote child-group IDs.
func insertGroup(t *testing.T, db *DB, name string, items ...any) int64 {
	t.Helper()
	result, err := db.sql.Exec(`insert into groups (name, title_key, created_at, updated_at) values (?, ?, 0, 0)`,
		name, domain.NaturalSortKey(name))
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	for position, item := range items {
		var key, child any
		switch v := item.(type) {
		case string:
			key = v
		case int64:
			child = v
		default:
			t.Fatalf("項目の型 %T", item)
		}
		if _, err := db.sql.Exec(`insert into group_items (group_id, content_key, child_id, position) values (?, ?, ?, ?)`,
			id, key, child, position); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func itemKeyOf(path string) string {
	return "key" + fixturePath(path)
}

func ownerLibraryNames(t *testing.T, db *DB, q domain.VideoQuery) []string {
	t.Helper()
	names := itemNames(libraryPages(t, db, domain.AudienceOwner, q))
	slices.Sort(names)
	return names
}

func groupItemNames(items []domain.GroupItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Group != nil {
			out = append(out, "group:"+item.Group.Name)
		} else {
			out = append(out, item.Video.Title)
		}
	}
	return out
}

func TestListLibraryListsHandMadeGroupAsOneItem(t *testing.T) {
	db, ids := itemsFixture(t)
	mix := insertGroup(t, db, "mix",
		itemKeyOf("/media/mixed/sub/y.mp4"), itemKeyOf("/media/solo.mp4"), itemKeyOf("/media/mixed/x.mp4"))

	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: 1})
	names := itemNames(items)
	slices.Sort(names)
	if want := []string{"group:mix", "group:pair", "group:show"}; !slices.Equal(names, want) {
		t.Fatalf("項目 = %v, want %v", names, want)
	}
	group := findGroup(t, items, "mix")
	want := []int64{ids[fixturePath("/media/mixed/sub/y.mp4")], ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]}
	if group.ID != mix || group.Path != "" || !slices.Equal(memberIDs(group), want) {
		t.Errorf("mix = id %d, path %q, members %v; want id %d, no path, members %v", group.ID, group.Path, memberIDs(group), mix, want)
	}
}

func TestListLibraryListsTopGroupsAndGroupShowsChildGroup(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	a := insertGroup(t, db, "A", itemKeyOf("/media/mixed/x.mp4"), itemKeyOf("/media/solo.mp4"))
	b := insertGroup(t, db, "B", itemKeyOf("/media/mixed/sub/y.mp4"), a, itemKeyOf("/media/solo.mp4"))

	if names, want := ownerLibraryNames(t, db, domain.VideoQuery{}), []string{"group:B", "group:pair", "group:show"}; !slices.Equal(names, want) {
		t.Fatalf("項目 = %v, want %v", names, want)
	}
	page, err := db.Library().Group(ctx, domain.AudienceOwner, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if names := groupItemNames(page.Items); !slices.Equal(names, []string{"y", "group:A", "solo"}) {
		t.Errorf("B の項目 = %v, want [y group:A solo]", names)
	}
	want := []int64{ids[fixturePath("/media/mixed/sub/y.mp4")], ids[fixturePath("/media/mixed/x.mp4")], ids[fixturePath("/media/solo.mp4")]}
	if !slices.Equal(memberIDs(page.Group), want) {
		t.Errorf("B の再生順 = %v, want %v（solo は最初の 1 回だけ）", memberIDs(page.Group), want)
	}
	if child := page.Items[1].Group; child == nil || child.ID != a || len(child.Members) != 2 {
		t.Errorf("B の中の A = %+v", page.Items[1])
	}
}

func TestListLibraryMatchesOverDescendants(t *testing.T) {
	db, _ := itemsFixture(t)
	a := insertGroup(t, db, "A", itemKeyOf("/media/mixed/x.mp4"))
	insertGroup(t, db, "B", a, itemKeyOf("/media/mixed/sub/y.mp4"))
	insertGroup(t, db, "C", itemKeyOf("/media/solo.mp4"), itemKeyOf("/media/pair/p1.mp4"))
	insertGroup(t, db, "D", itemKeyOf("/media/solo.mp4"), itemKeyOf("/media/pair/p2.mp4"))

	if names := ownerLibraryNames(t, db, domain.VideoQuery{Query: "mixed"}); !slices.Equal(names, []string{"group:B"}) {
		t.Errorf("mixed の検索 = %v, want [group:B]", names)
	}
	if names := ownerLibraryNames(t, db, domain.VideoQuery{Query: "y"}); !slices.Equal(names, []string{"y"}) {
		t.Errorf("y の検索 = %v, want [y]", names)
	}
	if names := ownerLibraryNames(t, db, domain.VideoQuery{Query: "solo"}); !slices.Equal(names, []string{"solo"}) {
		t.Errorf("solo の検索 = %v, want [solo]（1 回だけ）", names)
	}
}

// A partial group can expose a standalone video even when a different whole-group card includes it.
func TestListLibraryKeepsPartialMatchBesideWholeGroup(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	insertGroup(t, db, "C", itemKeyOf("/media/solo.mp4"))
	insertGroup(t, db, "D", itemKeyOf("/media/solo.mp4"), itemKeyOf("/media/pair/p2.mp4"))

	q := domain.VideoQuery{Query: "solo"}
	if names := ownerLibraryNames(t, db, q); !slices.Equal(names, []string{"group:C", "solo"}) {
		t.Errorf("solo の検索 = %v, want [group:C solo]", names)
	}
	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, q)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Errorf("solo の検索の件数 = %d, want 2", page.Total)
	}
	selection, _, err := db.Library().LibraryIDs(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{ids[fixturePath("/media/solo.mp4")]}; !slices.Equal(selection.VideoIDs, want) || len(selection.Groups) != 1 {
		t.Errorf("solo の検索の id = videos %v, groups %+v; want videos %v and 1 group", selection.VideoIDs, selection.Groups, want)
	}
}

func TestGroupReadsDescendantsAtAnyDepth(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	top := insertGroup(t, db, "deep", itemKeyOf("/media/solo.mp4"))
	for range 80 {
		top = insertGroup(t, db, "deep", top)
	}
	page, err := db.Library().Group(ctx, domain.AudienceOwner, top, nil)
	if err != nil {
		t.Fatalf("81 段のグループ: %v", err)
	}
	if want := []int64{ids[fixturePath("/media/solo.mp4")]}; !slices.Equal(memberIDs(page.Group), want) {
		t.Errorf("81 段のグループの再生順 = %v, want %v", memberIDs(page.Group), want)
	}

}

// Forty diamond levels encode 2^40 paths; verify bounded traversal preserves the earliest depth-first order.
func TestGroupReadsSharedChildGroupsOnce(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	left := insertGroup(t, db, "left", itemKeyOf("/media/solo.mp4"))
	right := insertGroup(t, db, "right", itemKeyOf("/media/mixed/x.mp4"))
	for range 40 {
		left, right = insertGroup(t, db, "left", left, right), insertGroup(t, db, "right", right, left)
	}
	top := insertGroup(t, db, "top", left, right)
	page, err := db.Library().Group(ctx, domain.AudienceOwner, top, nil)
	if err != nil {
		t.Fatalf("ダイヤモンド状の 40 段のグループ: %v", err)
	}
	want := []int64{ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]}
	if got := memberIDs(page.Group); !slices.Equal(got, want) {
		t.Errorf("再生順 = %v, want %v", got, want)
	}
	names := ownerLibraryNames(t, db, domain.VideoQuery{})
	if !slices.Contains(names, "group:top") {
		t.Errorf("ライブラリのいちばん上 = %v, want group:top を含む", names)
	}
}

func TestGroupWithoutVisibleVideoIsNotShown(t *testing.T) {
	db, _ := itemsFixture(t)
	ctx := context.Background()
	gone := insertGroup(t, db, "gone", "key-of-no-video")
	empty := insertGroup(t, db, "empty")

	if names := ownerLibraryNames(t, db, domain.VideoQuery{}); slices.Contains(names, "group:gone") || slices.Contains(names, "group:empty") {
		t.Errorf("項目 = %v に見える動画の無いグループがある", names)
	}
	for _, id := range []int64{gone, empty, 9999} {
		if _, err := db.Library().Group(ctx, domain.AudienceOwner, id, nil); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("グループ %d = %v, want ErrNotFound", id, err)
		}
		if _, err := db.Library().GroupItems(ctx, domain.AudienceOwner, id, "", 0); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("グループ %d の項目 = %v, want ErrNotFound", id, err)
		}
	}
}

func TestGroupForGuestReadsPublicVideosOnly(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	hidden := insertGroup(t, db, "hidden", itemKeyOf("/media/mixed/x.mp4"))
	outer := insertGroup(t, db, "outer", hidden, itemKeyOf("/media/solo.mp4"), itemKeyOf("/media/mixed/sub/y.mp4"))
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{ids[fixturePath("/media/solo.mp4")]}, true); err != nil {
		t.Fatal(err)
	}

	items := libraryPages(t, db, domain.AudienceGuest, domain.VideoQuery{})
	if names := itemNames(items); !slices.Equal(names, []string{"group:outer"}) {
		t.Fatalf("ゲストの項目 = %v, want [group:outer]", names)
	}
	page, err := db.Library().Group(ctx, domain.AudienceGuest, outer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if names := groupItemNames(page.Items); !slices.Equal(names, []string{"solo"}) {
		t.Errorf("ゲストの outer の項目 = %v, want [solo]", names)
	}
	if !slices.Equal(memberIDs(page.Group), []int64{ids[fixturePath("/media/solo.mp4")]}) {
		t.Errorf("ゲストの outer の再生順 = %v", memberIDs(page.Group))
	}
	if _, err := db.Library().Group(ctx, domain.AudienceGuest, hidden, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ゲストの hidden = %v, want ErrNotFound", err)
	}
}

func TestGroupTrailIsCutAtABrokenPair(t *testing.T) {
	db, _ := itemsFixture(t)
	ctx := context.Background()
	a := insertGroup(t, db, "A", itemKeyOf("/media/solo.mp4"))
	b := insertGroup(t, db, "B", a)
	c := insertGroup(t, db, "C", b)
	other := insertGroup(t, db, "other", itemKeyOf("/media/mixed/x.mp4"))

	for _, tc := range []struct {
		trail []int64
		want  []domain.GroupRef
	}{
		{nil, []domain.GroupRef{}},
		{[]int64{c, b}, []domain.GroupRef{{ID: c, Name: "C"}, {ID: b, Name: "B"}}},
		{[]int64{other, b}, []domain.GroupRef{{ID: b, Name: "B"}}},
		{[]int64{c, other}, []domain.GroupRef{}},
		{[]int64{9999, c, b}, []domain.GroupRef{{ID: c, Name: "C"}, {ID: b, Name: "B"}}},
	} {
		page, err := db.Library().Group(ctx, domain.AudienceOwner, a, tc.trail)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(page.Trail, tc.want) {
			t.Errorf("trail %v = %+v, want %+v", tc.trail, page.Trail, tc.want)
		}
	}
}

func TestGroupItemsPages(t *testing.T) {
	db, _ := itemsFixture(t)
	ctx := context.Background()
	g := insertGroup(t, db, "g", itemKeyOf("/media/solo.mp4"), "key-of-no-video", itemKeyOf("/media/mixed/x.mp4"), itemKeyOf("/media/mixed/sub/y.mp4"))

	var names []string
	cursor := ""
	for range 10 {
		page, err := db.Library().GroupItems(ctx, domain.AudienceOwner, g, cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, groupItemNames(page.Items)...)
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if !slices.Equal(names, []string{"solo", "x", "y"}) {
		t.Errorf("項目 = %v, want [solo x y]", names)
	}
	for _, bad := range []string{"not-a-cursor", packCursor("other", "1"), packCursor(groupItemsCursorKind, "x")} {
		if _, err := db.Library().GroupItems(ctx, domain.AudienceOwner, g, bad, 1); !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("カーソル %q = %v, want ErrInvalidCursor", bad, err)
		}
	}
}
