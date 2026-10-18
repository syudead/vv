package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Place one video directly under each folder so automatic grouping does not hide it.
func looseVideo(t *testing.T, db *DB, key string) int64 {
	t.Helper()
	return upsertFolderVideo(t, db, fixturePath("/media/"+key+"/"+key+".mp4"), key)
}

func createGroup(t *testing.T, db *DB, name string, add domain.GroupAdd) int64 {
	t.Helper()
	group, err := db.Groups().CreateGroup(context.Background(), name, add)
	if err != nil {
		t.Fatalf("CreateGroup(%q) error = %v", name, err)
	}
	return group.ID
}

// Also assert that stored positions are contiguous from zero.
func groupEntries(t *testing.T, db *DB, id int64) []string {
	t.Helper()
	rows, err := db.sql.Query(`select position, coalesce('v:' || content_key, 'g:' || child_id) from group_items
		where group_id = ? order by position`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var position int
		var entry string
		if err := rows.Scan(&position, &entry); err != nil {
			t.Fatal(err)
		}
		if position != len(out) {
			t.Fatalf("group %d position %d at index %d", id, position, len(out))
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertEntries(t *testing.T, db *DB, id int64, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := groupEntries(t, db, id); !slices.Equal(got, want) {
		t.Fatalf("group %d items = %v, want %v", id, got, want)
	}
}

// Snapshot all group tables and overrides to detect partial writes.
func groupTablesSnapshot(t *testing.T, db *DB) string {
	t.Helper()
	var b strings.Builder
	for _, query := range []string{
		`select id || '|' || name || '|' || coalesce(origin_folder, '') || '|' || updated_at from groups order by id`,
		`select id || '|' || group_id || '|' || coalesce(content_key, '') || '|' || coalesce(child_id, '') || '|' ||
			position || '|' || from_folder from group_items order by id`,
		`select group_id || '|' || content_key from group_exclusions order by group_id, content_key`,
		`select path || '|' || mode from folder_group_overrides order by path`,
	} {
		rows, err := queryStringsForTest(db, query)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(&b, rows)
	}
	return b.String()
}

func groupItemID(t *testing.T, db *DB, groupID int64, entry string) int64 {
	t.Helper()
	var id int64
	if err := db.sql.QueryRow(`select id from group_items where group_id = ?
		and coalesce('v:' || content_key, 'g:' || child_id) = ?`, groupID, entry).Scan(&id); err != nil {
		t.Fatalf("item %s of group %d: %v", entry, groupID, err)
	}
	return id
}

func groupExists(t *testing.T, db *DB, id int64) bool {
	t.Helper()
	var exists bool
	if err := db.sql.QueryRow(`select exists (select 1 from groups where id = ?)`, id).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	return exists
}

func TestCreateAndAddGroupItems(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b, c := looseVideo(t, db, "ka"), looseVideo(t, db, "kb"), looseVideo(t, db, "kc")
	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{c}})

	group, err := db.Groups().CreateGroup(ctx, "  Mine  ", domain.GroupAdd{VideoIDs: []int64{b, a, b}, GroupIDs: []int64{child}})
	if err != nil {
		t.Fatal(err)
	}
	if group.Name != "Mine" || group.OriginFolder != "" {
		t.Fatalf("group = %+v", group)
	}
	assertEntries(t, db, group.ID, "v:kb", "v:ka", fmt.Sprintf("g:%d", child))

	other := createGroup(t, db, "Other", domain.GroupAdd{VideoIDs: []int64{a}})
	assertEntries(t, db, other, "v:ka")
	assertEntries(t, db, group.ID, "v:kb", "v:ka", fmt.Sprintf("g:%d", child))

	if _, err := db.Groups().AddGroupItems(ctx, other, domain.GroupAdd{VideoIDs: []int64{a, c}, GroupIDs: []int64{child}}); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, other, "v:ka", "v:kc", fmt.Sprintf("g:%d", child))

	d, e := looseVideo(t, db, "kd"), looseVideo(t, db, "ke")
	bundle(t, db, d, d, e)
	keys, err := queryStringsForTest(db, `select user_key from video_bundles`)
	if err != nil || len(keys) != 1 {
		t.Fatalf("bundles = %v, %v", keys, err)
	}
	if _, err := db.Groups().AddGroupItems(ctx, other, domain.GroupAdd{VideoIDs: []int64{d, e}}); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, other, "v:ka", "v:kc", fmt.Sprintf("g:%d", child), "v:"+keys[0])
}

func TestGroupNamesAreValidated(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id := createGroup(t, db, "Name", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ka")}})
	before := groupTablesSnapshot(t, db)
	for _, name := range []string{" \t ", " " + strings.Repeat("a", 101) + " "} {
		if _, err := db.Groups().CreateGroup(ctx, name, domain.GroupAdd{}); !errors.Is(err, domain.ErrInvalidGroupName) {
			t.Errorf("CreateGroup(%q) error = %v", name, err)
		}
		if _, err := db.Groups().RenameGroup(ctx, id, name); !errors.Is(err, domain.ErrInvalidGroupName) {
			t.Errorf("RenameGroup(%q) error = %v", name, err)
		}
	}
	if after := groupTablesSnapshot(t, db); after != before {
		t.Fatalf("invalid names wrote:\n%s\nwant\n%s", after, before)
	}

	renamed, err := db.Groups().RenameGroup(ctx, id, "  "+strings.Repeat("b", 100)+" ")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != strings.Repeat("b", 100) {
		t.Fatalf("renamed = %+v", renamed)
	}
	if rows := readGroups(t, db); rows[0].Name != renamed.Name {
		t.Fatalf("groups = %+v", rows)
	}
	if _, err := db.Groups().RenameGroup(ctx, 999, "x"); !errors.Is(err, domain.ErrGroupNotFound) {
		t.Fatalf("RenameGroup(missing) error = %v", err)
	}
}

func TestAddGroupItemsRefusesCycles(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	v := looseVideo(t, db, "kv")
	a := createGroup(t, db, "A", domain.GroupAdd{VideoIDs: []int64{v}})
	b := createGroup(t, db, "B", domain.GroupAdd{GroupIDs: []int64{a}})
	top := createGroup(t, db, "Top", domain.GroupAdd{GroupIDs: []int64{b}})
	before := groupTablesSnapshot(t, db)
	for name, c := range map[string]struct{ parent, child int64 }{
		"B を A に":   {parent: a, child: b},
		"Top を A に": {parent: a, child: top},
		"A を A に":   {parent: a, child: a},
	} {
		_, err := db.Groups().AddGroupItems(ctx, c.parent, domain.GroupAdd{VideoIDs: []int64{v}, GroupIDs: []int64{c.child}})
		if !errors.Is(err, domain.ErrGroupCycle) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
	if after := groupTablesSnapshot(t, db); after != before {
		t.Fatalf("cycles wrote:\n%s\nwant\n%s", after, before)
	}
}

func TestGroupEditsFailWholeOnMissingTargets(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	v, w := looseVideo(t, db, "kv"), looseVideo(t, db, "kw")
	id := createGroup(t, db, "G", domain.GroupAdd{VideoIDs: []int64{v}})
	item := groupItemID(t, db, id, "v:kv")
	before := groupTablesSnapshot(t, db)
	missing := int64(9999)
	groups := db.Groups()
	for name, err := range map[string]error{
		"作成: 無い動画": func() error {
			_, err := groups.CreateGroup(ctx, "New", domain.GroupAdd{VideoIDs: []int64{w, missing}})
			return err
		}(),
		"作成: 無いグループ": func() error {
			_, err := groups.CreateGroup(ctx, "New", domain.GroupAdd{GroupIDs: []int64{missing}})
			return err
		}(),
		"追加: 無い動画": func() error {
			_, err := groups.AddGroupItems(ctx, id, domain.GroupAdd{VideoIDs: []int64{w, missing}})
			return err
		}(),
		"追加: 無いグループ": func() error {
			_, err := groups.AddGroupItems(ctx, id, domain.GroupAdd{VideoIDs: []int64{w}, GroupIDs: []int64{missing}})
			return err
		}(),
		"除去: 無い項目": func() error { _, _, err := groups.RemoveGroupItems(ctx, id, []int64{item, missing}); return err }(),
		"移動: 無い項目": groups.MoveGroupItem(ctx, id, missing, nil),
		"移動: 無い目印": groups.MoveGroupItem(ctx, id, item, &missing),
	} {
		if !errors.Is(err, domain.ErrGroupTargetNotFound) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
	for name, err := range map[string]error{
		"追加": func() error {
			_, err := groups.AddGroupItems(ctx, missing, domain.GroupAdd{VideoIDs: []int64{w}})
			return err
		}(),
		"除去": func() error { _, _, err := groups.RemoveGroupItems(ctx, missing, []int64{item}); return err }(),
		"移動": groups.MoveGroupItem(ctx, missing, item, nil),
		"解散": groups.DissolveGroup(ctx, missing),
		"タグ": func() error { _, err := groups.TagGroup(ctx, missing); return err }(),
	} {
		if !errors.Is(err, domain.ErrGroupNotFound) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
	if after := groupTablesSnapshot(t, db); after != before {
		t.Fatalf("missing targets wrote:\n%s\nwant\n%s", after, before)
	}
}

func TestMoveAndRemoveGroupItems(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b, c := looseVideo(t, db, "ka"), looseVideo(t, db, "kb"), looseVideo(t, db, "kc")
	id := createGroup(t, db, "G", domain.GroupAdd{VideoIDs: []int64{a, b, c}})
	ia, ib, ic := groupItemID(t, db, id, "v:ka"), groupItemID(t, db, id, "v:kb"), groupItemID(t, db, id, "v:kc")

	if err := db.Groups().MoveGroupItem(ctx, id, ia, &ic); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, id, "v:kb", "v:kc", "v:ka")
	if err := db.Groups().MoveGroupItem(ctx, id, ic, nil); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, id, "v:kc", "v:kb", "v:ka")
	if err := db.Groups().MoveGroupItem(ctx, id, ib, &ib); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, id, "v:kc", "v:kb", "v:ka")

	if _, _, err := db.Groups().RemoveGroupItems(ctx, id, []int64{ic, ia}); err != nil {
		t.Fatal(err)
	}
	assertEntries(t, db, id, "v:kb")
	if got := groupExclusions(t, db, id); len(got) != 0 {
		t.Fatalf("hand-made group exclusions = %v", got)
	}
}

func TestRemovingFolderMemberWritesExclusion(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	k1 := upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	id := readGroups(t, db)[0].ID
	origin := domain.FolderKey(fixturePath("/media/Show"))

	if _, _, err := db.Groups().RemoveGroupItems(ctx, id, []int64{groupItemID(t, db, id, "v:k1")}); err != nil {
		t.Fatal(err)
	}
	if got := groupExclusions(t, db, id); !slices.Equal(got, []string{"k1"}) {
		t.Fatalf("exclusions = %v", got)
	}
	rebuildIndex(t, db)
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: origin, Items: folderItems("k2")}})

	if _, err := db.Groups().AddGroupItems(ctx, id, domain.GroupAdd{VideoIDs: []int64{k1}}); err != nil {
		t.Fatal(err)
	}
	if got := groupExclusions(t, db, id); len(got) != 0 {
		t.Fatalf("exclusions after adding back = %v", got)
	}
	rebuildIndex(t, db)
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: origin, Items: []groupItem{{Key: "k2", Folder: true}, {Key: "k1"}}}})
}

func TestRemovingTheLastItemDissolvesTheGroup(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := looseVideo(t, db, "ka"), looseVideo(t, db, "kb")
	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{a, b}})
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{child}, VideoIDs: []int64{a}})
	top := createGroup(t, db, "Top", domain.GroupAdd{GroupIDs: []int64{parent}})
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	group, dissolved, err := db.Groups().RemoveGroupItems(ctx, child, []int64{groupItemID(t, db, child, "v:ka")})
	if err != nil || dissolved || group.ID != child {
		t.Fatalf("removing one of two: group = %+v, dissolved = %v, err = %v", group, dissolved, err)
	}
	recorder.events = nil

	group, dissolved, err = db.Groups().RemoveGroupItems(ctx, child, []int64{groupItemID(t, db, child, "v:kb")})
	if err != nil || !dissolved || group.ID != 0 {
		t.Fatalf("removing the last item: group = %+v, dissolved = %v, err = %v", group, dissolved, err)
	}
	if groupExists(t, db, child) {
		t.Fatal("the emptied group still exists")
	}
	assertEntries(t, db, parent, "v:ka")
	want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{child, parent, top}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("events = %+v, want %+v", recorder.events, want)
	}
	if _, err := db.Library().GetVideo(ctx, domain.AudienceOwner, b); err != nil {
		t.Fatalf("video of the dissolved group: %v", err)
	}

	show := fixturePath("/media/Show")
	upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	auto := readGroups(t, db)[len(readGroups(t, db))-1].ID
	if _, dissolved, err := db.Groups().RemoveGroupItems(ctx, auto,
		[]int64{groupItemID(t, db, auto, "v:k1"), groupItemID(t, db, auto, "v:k2")}); err != nil || !dissolved {
		t.Fatalf("emptying the automatic group: dissolved = %v, err = %v", dissolved, err)
	}
	if grouping := folderGroupingOf(t, db, show); grouping.Mode != domain.FolderGroupUngroup || grouping.GroupID != 0 {
		t.Fatalf("grouping after emptying = %+v", grouping)
	}
	rebuildIndex(t, db)
	if groupExists(t, db, auto) {
		t.Fatal("the rebuild recreated the emptied automatic group")
	}
}

func TestDissolveGroupKeepsChildrenAndLeavesParents(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := looseVideo(t, db, "ka"), looseVideo(t, db, "kb")
	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{a}})
	middle := createGroup(t, db, "Middle", domain.GroupAdd{VideoIDs: []int64{b}, GroupIDs: []int64{child}})
	parent := createGroup(t, db, "Parent", domain.GroupAdd{VideoIDs: []int64{a}, GroupIDs: []int64{middle}})
	other := createGroup(t, db, "Other", domain.GroupAdd{GroupIDs: []int64{middle}, VideoIDs: []int64{b}})
	assertEntries(t, db, other, "v:kb", fmt.Sprintf("g:%d", middle))

	if err := db.Groups().DissolveGroup(ctx, middle); err != nil {
		t.Fatal(err)
	}
	if groupExists(t, db, middle) {
		t.Fatal("dissolved group still exists")
	}
	assertEntries(t, db, child, "v:ka")
	assertEntries(t, db, parent, "v:ka")
	assertEntries(t, db, other, "v:kb")
	if _, err := db.Library().GetVideo(ctx, domain.AudienceOwner, b); err != nil {
		t.Fatalf("video of the dissolved group: %v", err)
	}
}

func TestDissolveAutomaticGroupWritesUngroup(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	show := fixturePath("/media/Show")
	upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	id := readGroups(t, db)[0].ID
	loose := looseVideo(t, db, "ka")
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{id}, VideoIDs: []int64{loose}})

	if err := db.Groups().DissolveGroup(ctx, id); err != nil {
		t.Fatal(err)
	}
	grouping := folderGroupingOf(t, db, show)
	if grouping.Mode != domain.FolderGroupUngroup || grouping.GroupID != 0 {
		t.Fatalf("grouping after dissolve = %+v", grouping)
	}
	rebuildIndex(t, db)
	assertEntries(t, db, parent, "v:ka")

	if _, err := db.FolderGroups().SetFolderGrouping(ctx, show, ""); err != nil {
		t.Fatal(err)
	}
	rows := readGroups(t, db)
	if len(rows) != 2 || rows[1].ID == id || rows[1].Origin != domain.FolderKey(show) {
		t.Fatalf("groups after auto = %+v", rows)
	}
	recreated := rows[1].ID

	if _, err := db.FolderGroups().SetFolderGrouping(ctx, show, domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	if groupExists(t, db, recreated) {
		t.Fatal("ungroup kept the folder's group")
	}
}

// The root automatic folder key is empty, which must remain distinct from a manual group's NULL.
func TestDissolveRootFolderGroupWritesUngroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("登録フォルダ `/` は Unix だけのもの")
	}
	db := migratedDB(t)
	id := createGroup(t, db, "root", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ka")}})
	if _, err := db.sql.Exec(`update groups set origin_folder = ? where id = ?`, domain.FolderKey("/"), id); err != nil {
		t.Fatal(err)
	}

	if err := db.Groups().DissolveGroup(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	overrides, err := queryStringsForTest(db, `select path || '|' || mode from folder_group_overrides order by path`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"|" + string(domain.FolderGroupUngroup)}; !slices.Equal(overrides, want) {
		t.Fatalf("overrides = %q, want %q", overrides, want)
	}
}

func folderGroupingOf(t *testing.T, db *DB, path string) domain.FolderGrouping {
	t.Helper()
	groupings, err := db.FolderGroups().FolderGroupings(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	return groupings[0]
}

func TestTagGroupAttachesToDescendantsAndDissolves(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b, c := looseVideo(t, db, "ka"), looseVideo(t, db, "kb"), looseVideo(t, db, "kc")
	grandchild := createGroup(t, db, "Grandchild", domain.GroupAdd{VideoIDs: []int64{c}})
	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{b}, GroupIDs: []int64{grandchild}})
	id := createGroup(t, db, " Trip ", domain.GroupAdd{VideoIDs: []int64{a}, GroupIDs: []int64{child}})

	result, err := db.Groups().TagGroup(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tag.Name != "Trip" || !result.Created {
		t.Fatalf("result = %+v", result)
	}
	keys, err := queryStringsForTest(db, `select content_key from video_tags where tag_id = ? and tentative = 0 order by content_key`, result.Tag.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keys, []string{"ka", "kb", "kc"}) {
		t.Fatalf("tagged keys = %v", keys)
	}
	if groupExists(t, db, id) || !groupExists(t, db, child) || !groupExists(t, db, grandchild) {
		t.Fatal("only the tagged group should be dissolved")
	}

	again := createGroup(t, db, "Trip", domain.GroupAdd{VideoIDs: []int64{a}})
	result2, err := db.Groups().TagGroup(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	if result2.Created || result2.Tag.ID != result.Tag.ID {
		t.Fatalf("second tag = %+v, first %+v", result2, result)
	}
}

func TestGroupEditsPublishGroupsChanged(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := looseVideo(t, db, "ka"), looseVideo(t, db, "kb")
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	groups := db.Groups()
	expect := func(name string, ids ...int64) {
		t.Helper()
		want := []domain.Event{domain.GroupsChanged{GroupIDs: ids}}
		if ids == nil {
			want = nil
		}
		if !reflect.DeepEqual(recorder.events, want) {
			t.Fatalf("%s: events = %+v, want %+v", name, recorder.events, want)
		}
		recorder.events = nil
	}

	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{a}})
	expect("create", child)
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{child}, VideoIDs: []int64{b}})
	expect("create parent", parent)
	top := createGroup(t, db, "Top", domain.GroupAdd{GroupIDs: []int64{parent}})
	expect("create top", top)

	if _, err := groups.RenameGroup(ctx, child, "Renamed"); err != nil {
		t.Fatal(err)
	}
	expect("rename", child, parent, top)
	if _, err := groups.AddGroupItems(ctx, child, domain.GroupAdd{VideoIDs: []int64{b}}); err != nil {
		t.Fatal(err)
	}
	expect("add", child, parent, top)
	if _, err := groups.AddGroupItems(ctx, child, domain.GroupAdd{VideoIDs: []int64{b}}); err != nil {
		t.Fatal(err)
	}
	expect("add nothing new")
	if err := groups.MoveGroupItem(ctx, child, groupItemID(t, db, child, "v:kb"), nil); err != nil {
		t.Fatal(err)
	}
	expect("move", child, parent, top)
	if _, _, err := groups.RemoveGroupItems(ctx, child, []int64{groupItemID(t, db, child, "v:kb")}); err != nil {
		t.Fatal(err)
	}
	expect("remove", child, parent, top)
	before := groupTablesSnapshot(t, db)
	if _, _, err := groups.RemoveGroupItems(ctx, child, nil); err != nil {
		t.Fatal(err)
	}
	expect("remove nothing")
	if after := groupTablesSnapshot(t, db); after != before {
		t.Fatalf("an empty removal wrote:\n%s\nwant\n%s", after, before)
	}
	if _, err := groups.AddGroupItems(ctx, child, domain.GroupAdd{GroupIDs: []int64{top}}); !errors.Is(err, domain.ErrGroupCycle) {
		t.Fatal(err)
	}
	expect("failed add")
	if err := groups.DissolveGroup(ctx, child); err != nil {
		t.Fatal(err)
	}
	expect("dissolve", child, parent, top)
	if _, err := groups.TagGroup(ctx, parent); err != nil {
		t.Fatal(err)
	}
	if want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{parent, top}}, domain.ActivityChanged{}}; !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("tag: events = %+v, want %+v", recorder.events, want)
	}
	if groupExists(t, db, top) {
		t.Fatal("the parent emptied by turning its only child into a tag still exists")
	}
}

func TestDissolveCascadesToEmptiedParents(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := looseVideo(t, db, "ka"), looseVideo(t, db, "kb")
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	expectDissolved := func(name string, dissolve func() error, gone []int64, kept []int64, events ...int64) {
		t.Helper()
		recorder.events = nil
		if err := dissolve(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, id := range gone {
			if groupExists(t, db, id) {
				t.Fatalf("%s: group %d still exists", name, id)
			}
		}
		for _, id := range kept {
			if !groupExists(t, db, id) {
				t.Fatalf("%s: group %d was dissolved", name, id)
			}
		}
		want := []domain.Event{domain.GroupsChanged{GroupIDs: events}}
		if !reflect.DeepEqual(recorder.events, want) {
			t.Fatalf("%s: events = %+v, want %+v", name, recorder.events, want)
		}
	}

	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{a}})
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{child}})
	expectDissolved("single parent", func() error { return db.Groups().DissolveGroup(ctx, child) },
		[]int64{child, parent}, nil, child, parent)

	child = createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{a}})
	middle := createGroup(t, db, "Middle", domain.GroupAdd{GroupIDs: []int64{child}})
	upper := createGroup(t, db, "Upper", domain.GroupAdd{GroupIDs: []int64{middle}})
	top := createGroup(t, db, "Top", domain.GroupAdd{GroupIDs: []int64{upper}, VideoIDs: []int64{b}})
	expectDissolved("chain of two", func() error { return db.Groups().DissolveGroup(ctx, child) },
		[]int64{child, middle, upper}, []int64{top}, child, middle, upper, top)
	assertEntries(t, db, top, "v:kb")

	child = createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{a}})
	sibling := createGroup(t, db, "Sibling", domain.GroupAdd{GroupIDs: []int64{child}, VideoIDs: []int64{b}})
	lonely := createGroup(t, db, "Lonely", domain.GroupAdd{GroupIDs: []int64{child}})
	expectDissolved("remove the last item", func() error {
		_, _, err := db.Groups().RemoveGroupItems(ctx, child, []int64{groupItemID(t, db, child, "v:ka")})
		return err
	}, []int64{child, lonely}, []int64{sibling}, child, sibling, lonely)
	assertEntries(t, db, sibling, "v:kb")
}

// Empty automatic ancestors must use normal dissolution so their ungroup overrides prevent recreation.
func TestDissolveCascadeToAutomaticParentWritesUngroup(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	show := fixturePath("/media/Show")
	upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	auto := readGroups(t, db)[0].ID
	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ka")}})
	if _, err := db.Groups().AddGroupItems(ctx, auto, domain.GroupAdd{GroupIDs: []int64{child}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Groups().RemoveGroupItems(ctx, auto,
		[]int64{groupItemID(t, db, auto, "v:k1"), groupItemID(t, db, auto, "v:k2")}); err != nil {
		t.Fatal(err)
	}

	if err := db.Groups().DissolveGroup(ctx, child); err != nil {
		t.Fatal(err)
	}
	if groupExists(t, db, auto) {
		t.Fatal("the emptied automatic parent still exists")
	}
	if grouping := folderGroupingOf(t, db, show); grouping.Mode != domain.FolderGroupUngroup || grouping.GroupID != 0 {
		t.Fatalf("grouping after the cascade = %+v", grouping)
	}
	rebuildIndex(t, db)
	if len(readGroups(t, db)) != 0 {
		t.Fatalf("groups after the rebuild = %+v", readGroups(t, db))
	}
}
