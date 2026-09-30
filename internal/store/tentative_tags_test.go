package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 仮のタグと却下した名前（specs/031-tentative-tags/data-model.md §3・§5）。

// tagTentative は保存されたタグの tentative を読む。
func tagTentative(t *testing.T, db *DB, id int64) bool {
	t.Helper()
	var tentative bool
	if err := db.sql.QueryRow(`select tentative from tags where id = ?`, id).Scan(&tentative); err != nil {
		t.Fatalf("タグ %d の tentative を読めない: %v", id, err)
	}
	return tentative
}

// tagIDByName は名前（元の名前かシノニム）のタグの id を返す。無ければ 0。
func tagIDByName(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	lookup, found, err := lookupTagName(context.Background(), db.sql, name)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		return 0
	}
	return lookup.tagID
}

func rejectedNames(t *testing.T, db *DB) []string {
	t.Helper()
	names, err := db.Tags().ListRejectedTagNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func applyTentative(t *testing.T, db *DB, refs []domain.VideoRef, action domain.VideoTagsAction, names ...string) domain.VideoTagsOutcome {
	t.Helper()
	outcome, err := db.Tags().ApplyVideoTags(context.Background(), refs, action, names, true)
	if err != nil {
		t.Fatalf("ApplyVideoTags(tentative, %s, %v): %v", action, names, err)
	}
	return outcome
}

// rejectName は name の仮のタグを key-a に付けてから却下し、却下した名前にする。
func rejectName(t *testing.T, db *DB, name string) {
	t.Helper()
	applyTentative(t, db, []domain.VideoRef{{ContentKey: "key-a"}}, domain.VideoTagsAdd, name)
	id := tagIDByName(t, db, name)
	if got, err := db.Tags().RejectTag(context.Background(), id); err != nil || got != name {
		t.Fatalf("RejectTag(%s) = %q, %v", name, got, err)
	}
}

// tentativeFixture は key-a・key-b の 2 本の動画があるデータベースを返す。
func tentativeFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 1),
	)
	return db, ids
}

var bothVideos = []domain.VideoRef{{ContentKey: "key-a"}, {ContentKey: "key-b"}}

// 受け入れ条件 1・3、Edge Case「同じ新しい名前が複数の動画」: tentative の add は無い名前を
// 仮のタグとして 1 つだけ作って全対象に付け、既存の確定したタグの名前・シノニムの状態は変えない。
func TestApplyVideoTagsTentativeCreatesOneTentativeTag(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	cat, err := db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, cat.ID, "ねこ", nil); err != nil {
		t.Fatal(err)
	}

	outcome := applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬", "ねこ", " 犬 ")
	if outcome.SkippedNames == nil || len(outcome.SkippedNames) != 0 {
		t.Errorf("SkippedNames = %#v, want 空の配列", outcome.SkippedNames)
	}
	if n := countTags(t, db); n != 2 {
		t.Fatalf("タグ %d 個, want 2（犬を 1 つだけ作る）", n)
	}
	dog := tagIDByName(t, db, "犬")
	if !tagTentative(t, db, dog) {
		t.Error("犬が仮のタグではない")
	}
	if tagTentative(t, db, cat.ID) {
		t.Error("既存の確定したタグ 猫 が仮になった")
	}
	for i, item := range outcome.Items {
		if names := tagNames(item.Tags); !reflect.DeepEqual(names, []string{"犬:m", "猫:m"}) {
			t.Errorf("item %d: tags = %v", i, names)
		}
		for _, tag := range item.Tags {
			if tag.Tentative != (tag.ID == dog) {
				t.Errorf("item %d: %s の Tentative = %v", i, tag.Name, tag.Tentative)
			}
		}
	}

	// 既存の仮のタグの名前を tentative でも手でも付けても、状態を変えない。
	applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬")
	if _, err := applyVideoTags(ctx, db, bothVideos, domain.VideoTagsAdd, []string{"犬"}); err != nil {
		t.Fatal(err)
	}
	if !tagTentative(t, db, dog) || countTags(t, db) != 2 {
		t.Error("既存の仮のタグへの付与で状態が変わった")
	}
}

// 受け入れ条件 2: tentative が偽なら確定したタグとして作る。
func TestApplyVideoTagsWithoutTentativeCreatesConfirmedTag(t *testing.T) {
	db, _ := tentativeFixture(t)
	outcome, err := db.Tags().ApplyVideoTags(context.Background(), bothVideos, domain.VideoTagsReplace, []string{"鳥"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if tagTentative(t, db, tagIDByName(t, db, "鳥")) {
		t.Error("tentative が偽なのに仮のタグとして作った")
	}
	if len(outcome.SkippedNames) != 0 || len(outcome.Items) != 2 {
		t.Errorf("outcome = %+v", outcome)
	}
}

// 受け入れ条件 8・9、Edge Case「すべて却下した名前」「replace と tentative」: 却下したタグは付与ごと
// 消えて名前が一覧に入り、その名前は tentative の付与で飛ばされる。
func TestRejectTagAndSkipRejectedNames(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬")
	dog := tagIDByName(t, db, "犬")

	name, err := db.Tags().RejectTag(ctx, dog)
	if err != nil || name != "犬" {
		t.Fatalf("RejectTag = %q, %v", name, err)
	}
	if countTags(t, db) != 0 {
		t.Error("却下したタグが残っている")
	}
	var assignments int
	if err := db.sql.QueryRow(`select count(*) from video_tags`).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if assignments != 0 {
		t.Errorf("却下したタグの付与が %d 件残っている", assignments)
	}
	if got := rejectedNames(t, db); !reflect.DeepEqual(got, []string{"犬"}) {
		t.Errorf("却下した名前 = %v", got)
	}
	if _, err := db.Tags().RejectTag(ctx, dog); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("無いタグの却下: err = %v", err)
	}

	// add: 却下した名前だけを飛ばし、残りは付く。
	outcome := applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬", "鳥", " 犬 ")
	if !reflect.DeepEqual(outcome.SkippedNames, []string{"犬"}) {
		t.Errorf("SkippedNames = %v, want [犬]", outcome.SkippedNames)
	}
	for i, item := range outcome.Items {
		if names := tagNames(item.Tags); !reflect.DeepEqual(names, []string{"鳥:m"}) {
			t.Errorf("add item %d: tags = %v", i, names)
		}
	}
	if tagIDByName(t, db, "犬") != 0 {
		t.Error("却下した名前のタグが作られた")
	}

	// replace: 飛ばした名前は置き換え後の集合に入らない。
	outcome = applyTentative(t, db, bothVideos, domain.VideoTagsReplace, "猫", "犬")
	if !reflect.DeepEqual(outcome.SkippedNames, []string{"犬"}) {
		t.Errorf("replace SkippedNames = %v", outcome.SkippedNames)
	}
	for i, item := range outcome.Items {
		if names := tagNames(item.Tags); !reflect.DeepEqual(names, []string{"猫:m"}) {
			t.Errorf("replace item %d: tags = %v", i, names)
		}
	}

	// すべてが却下した名前なら空で置き換える。
	outcome = applyTentative(t, db, bothVideos, domain.VideoTagsReplace, "犬")
	for i, item := range outcome.Items {
		if len(item.Tags) != 0 {
			t.Errorf("all-rejected replace item %d: tags = %v", i, tagNames(item.Tags))
		}
	}

	// remove は tentative を読まず、無い名前を何もしない。
	outcome = applyTentative(t, db, bothVideos, domain.VideoTagsRemove, "犬", "無い名前")
	if len(outcome.SkippedNames) != 0 {
		t.Errorf("remove SkippedNames = %v", outcome.SkippedNames)
	}
}

// 受け入れ条件 8: 確定したタグの却下は ErrTagNotTentative で何も変えない。
func TestRejectConfirmedTagChangesNothing(t *testing.T) {
	db, ids := tentativeFixture(t)
	ctx := context.Background()
	tag, err := db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids[fixturePath("/media/a.mp4")]}, tag.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().RejectTag(ctx, tag.ID); !errors.Is(err, domain.ErrTagNotTentative) {
		t.Fatalf("err = %v, want ErrTagNotTentative", err)
	}
	got, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].VideoCount != 1 || len(rejectedNames(t, db)) != 0 {
		t.Errorf("確定したタグの却下で変わった: %+v, 却下した名前 %v", got, rejectedNames(t, db))
	}
}

// 受け入れ条件 7: 確定しても付いている動画は変わらない。
func TestConfirmTagKeepsAssignments(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬")
	dog := tagIDByName(t, db, "犬")

	for round := range 2 {
		tag, err := db.Tags().ConfirmTag(ctx, dog)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if tag.Tentative || tag.Name != "犬" || tag.VideoCount != 2 {
			t.Errorf("round %d: tag = %+v", round, tag)
		}
	}
	if tagTentative(t, db, dog) {
		t.Error("確定したのに仮のまま")
	}
	tags, err := db.Tags().TagsByContentKeys(ctx, []string{"key-a", "key-b"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"key-a", "key-b"} {
		if names := tagNames(tags[key]); !reflect.DeepEqual(names, []string{"犬:m"}) {
			t.Errorf("%s: tags = %v", key, names)
		}
	}
	if _, err := db.Tags().ConfirmTag(ctx, 9999); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("無いタグの確定: err = %v", err)
	}
}

// 受け入れ条件 11: 仮のタグの改名・シノニム登録・統合先で確定し、同じ名前への改名では変わらない。
func TestManualEditsConfirmTentativeTag(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	tags := db.Tags()
	applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬", "猫", "鳥", "魚", "虫")
	id := func(name string) int64 { return tagIDByName(t, db, name) }

	// 同じ名前への改名は何も変えない。
	if tag, err := tags.RenameTag(ctx, id("犬"), " 犬 "); err != nil || !tag.Tentative {
		t.Fatalf("同じ名前への改名 = %+v, %v", tag, err)
	}
	if tag, err := tags.RenameTag(ctx, id("犬"), "いぬ"); err != nil || tag.Tentative {
		t.Fatalf("改名 = %+v, %v", tag, err)
	}

	if tag, err := tags.AddSynonym(ctx, id("猫"), "ねこ", nil); err != nil || tag.Tentative {
		t.Fatalf("シノニム登録 = %+v, %v", tag, err)
	}

	// 承諾した統合を伴うシノニム登録: 先（鳥）が確定し、元（魚）は消える。
	fish := id("魚")
	if tag, err := tags.AddSynonym(ctx, id("鳥"), "魚", &fish); err != nil || tag.Tentative {
		t.Fatalf("統合を伴うシノニム登録 = %+v, %v", tag, err)
	}

	// 統合先（虫）が確定する。元に確定したタグを使っても仮のタグを使っても同じ。
	confirmed, err := tags.CreateTag(ctx, "蜂")
	if err != nil {
		t.Fatal(err)
	}
	if tag, err := tags.MergeTag(ctx, id("虫"), confirmed.ID); err != nil || tag.Tentative {
		t.Fatalf("統合 = %+v, %v", tag, err)
	}

	list, err := tags.ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range list {
		if tag.Tentative {
			t.Errorf("%s が仮のまま", tag.Name)
		}
	}
}

// 受け入れ条件 14: 却下した名前を手で使うと、却下した名前の一覧から消える。
func TestManualUseForgetsRejectedName(t *testing.T) {
	db, ids := tentativeFixture(t)
	ctx := context.Background()
	tags := db.Tags()
	videoA := []int64{ids[fixturePath("/media/a.mp4")]}
	base, err := tags.CreateTag(ctx, "基")
	if err != nil {
		t.Fatal(err)
	}
	other, err := tags.CreateTag(ctx, "別")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		use  func(name string) error
	}{
		{"CreateTag", func(name string) error { _, err := tags.CreateTag(ctx, name); return err }},
		{"RenameTag", func(name string) error { _, err := tags.RenameTag(ctx, other.ID, name); return err }},
		{"AddSynonym", func(name string) error { _, err := tags.AddSynonym(ctx, base.ID, name, nil); return err }},
		{"AttachTagByName", func(name string) error { _, _, err := tags.AttachTagByName(ctx, videoA, name); return err }},
		{"ApplyVideoTags", func(name string) error {
			_, err := tags.ApplyVideoTags(ctx, []domain.VideoRef{{ContentKey: "key-b"}}, domain.VideoTagsAdd, []string{name}, false)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := "却下-" + tc.name
			rejectName(t, db, name)
			if err := tc.use(name); err != nil {
				t.Fatal(err)
			}
			for _, got := range rejectedNames(t, db) {
				if got == name {
					t.Errorf("%s のあとも %s が却下した名前に残っている", tc.name, name)
				}
			}
			id := tagIDByName(t, db, name)
			if id == 0 || tagTentative(t, db, id) {
				t.Errorf("%s: 手で使った名前のタグ %d が確定していない", tc.name, id)
			}
		})
	}
}

// 受け入れ条件 14: グループのタグ化も手での作成で、却下した名前から外す。
func TestTagFolderGroupForgetsRejectedName(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	upsertFolderVideo(t, db, fixturePath("/media/Show/a.mp4"), "key-a")
	upsertFolderVideo(t, db, fixturePath("/media/Show/b.mp4"), "key-b")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	rejectName(t, db, "Show")

	result, err := db.FolderGroups().TagFolderGroup(ctx, fixturePath("/media/Show"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Tag.Tentative || tagTentative(t, db, result.Tag.ID) {
		t.Errorf("result = %+v", result)
	}
	if got := rejectedNames(t, db); len(got) != 0 {
		t.Errorf("却下した名前 = %v, want 空", got)
	}
}

// 受け入れ条件 13、Edge Case: 却下した名前を外すと再び仮のタグとして作られ、無い名前を外しても
// 誤りにならない。
func TestForgetRejectedTagName(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	rejectName(t, db, "犬")
	rejectName(t, db, "猫10")
	rejectName(t, db, "猫9")
	if got := rejectedNames(t, db); !reflect.DeepEqual(got, []string{"犬", "猫9", "猫10"}) {
		t.Errorf("却下した名前 = %v, want 自然順", got)
	}

	for _, name := range []string{" 犬 ", "無い名前", "\t"} {
		if err := db.Tags().ForgetRejectedTagName(ctx, name); err != nil {
			t.Fatalf("ForgetRejectedTagName(%q): %v", name, err)
		}
	}
	outcome := applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬")
	if len(outcome.SkippedNames) != 0 {
		t.Errorf("外した名前が飛ばされた: %v", outcome.SkippedNames)
	}
	if id := tagIDByName(t, db, "犬"); id == 0 || !tagTentative(t, db, id) {
		t.Error("外した名前が仮のタグとして作られない")
	}
}

// 読み出しに仮かどうかが載る。
func TestTagReadsCarryTentative(t *testing.T) {
	db, ids := tentativeFixture(t)
	ctx := context.Background()
	tags := db.Tags()
	applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬")
	dog := tagIDByName(t, db, "犬")
	videoIDs := []int64{ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")]}

	list, err := tags.ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Tentative {
		t.Errorf("ListTags = %+v", list)
	}
	byKey, err := tags.TagsByContentKeys(ctx, []string{"key-a"})
	if err != nil {
		t.Fatal(err)
	}
	if got := byKey["key-a"]; len(got) != 1 || !got[0].Tentative {
		t.Errorf("TagsByContentKeys = %+v", got)
	}
	summary, err := tags.Summary(ctx, videoIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Items) != 1 || !summary.Items[0].Tag.Tentative {
		t.Errorf("Summary = %+v", summary)
	}
	if ref, _, err := tags.AttachTagByID(ctx, videoIDs, dog); err != nil || !ref.Tentative {
		t.Errorf("AttachTagByID = %+v, %v", ref, err)
	}
	if ref, _, err := tags.AttachTagByName(ctx, videoIDs, "犬"); err != nil || !ref.Tentative {
		t.Errorf("AttachTagByName = %+v, %v", ref, err)
	}
	if ref, _, err := tags.DetachTag(ctx, videoIDs, dog); err != nil || !ref.Tentative {
		t.Errorf("DetachTag = %+v, %v", ref, err)
	}
	if !tagTentative(t, db, dog) {
		t.Error("手での付け外しで仮でなくなった")
	}
}

// Edge Case「既存データの移行」: 移行のあと既存のタグはすべて確定で、却下した名前は空。Down は
// 表と列を落とす。
func TestTentativeTagsMigration(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	downTo(t, db, 21)
	if _, ok := tableColumns(t, db, "tags")["tentative"]; ok {
		t.Fatal("Down のあとも tags.tentative が残っている")
	}
	var tables int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master where name = 'rejected_tag_names'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("Down のあとも rejected_tag_names が残っている")
	}
	for i, name := range []string{"猫", "犬"} {
		if _, err := db.sql.Exec(`insert into tags (id, created_at) values (?, 1)`, i+1); err != nil {
			t.Fatal(err)
		}
		if _, err := db.sql.Exec(`insert into tag_names (name, tag_id, canonical) values (?, ?, 1)`, name, i+1); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	list, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("ListTags = %+v", list)
	}
	for _, tag := range list {
		if tag.Tentative {
			t.Errorf("移行前のタグ %s が仮になった", tag.Name)
		}
	}
	if got := rejectedNames(t, db); len(got) != 0 {
		t.Errorf("却下した名前 = %v, want 空", got)
	}
}
