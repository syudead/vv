package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Move（specs/050-drag-organize/data-model.md「Operations」「Undo image」）。

func videoRef(id int64) domain.OrganizeRef {
	return domain.OrganizeRef{Kind: domain.OrganizeVideo, ID: id}
}
func groupRef(id int64) domain.OrganizeRef {
	return domain.OrganizeRef{Kind: domain.OrganizeGroup, ID: id}
}

// organizeSnapshot はグループの表、お気に入り、プレイリストの項目、フォルダの例外を、更新日時を
// 除いて並べる。取り消しで元に戻ったか、拒まれた操作が何も書かなかったかを確かめる。
func organizeSnapshot(t *testing.T, db *DB) []string {
	t.Helper()
	var out []string
	for _, query := range []string{
		`select id || '|' || name || '|' || coalesce(origin_folder, '') from groups order by id`,
		`select id || '|' || group_id || '|' || coalesce(content_key, '') || '|' || coalesce(child_id, '') || '|' ||
			position || '|' || from_folder from group_items order by id`,
		`select group_id || '|' || content_key from group_exclusions order by group_id, content_key`,
		`select group_id || '|' || favorited_at from group_favorites order by group_id`,
		`select id || '|' || playlist_id || '|' || coalesce(content_key, '') || '|' || coalesce(group_id, '') || '|' ||
			position from playlist_items order by id`,
		`select path || '|' || mode from folder_group_overrides order by path`,
	} {
		rows, err := queryStringsForTest(db, query)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, rows...)
		out = append(out, "--")
	}
	return out
}

func moveCards(t *testing.T, db *DB, m domain.OrganizeMove) []byte {
	t.Helper()
	image, err := db.Organize().Move(context.Background(), m)
	if err != nil {
		t.Fatalf("Move(%+v) error = %v", m, err)
	}
	return image
}

func undoImageForTest(t *testing.T, db *DB, image []byte) {
	t.Helper()
	if err := db.Organize().Undo(context.Background(), image); err != nil {
		t.Fatalf("Undo error = %v", err)
	}
}

func assertSnapshot(t *testing.T, db *DB, label string, want []string) {
	t.Helper()
	if got := organizeSnapshot(t, db); !slices.Equal(got, want) {
		t.Fatalf("%s:\n%v\nwant\n%v", label, got, want)
	}
}

// A から B へ動かした動画は B の末尾にあって A に無く（受け入れ条件 6）、知らせは両方を名指す。
// 取り消すと A と B は同じ項目 id のまま元に戻る。
func TestOrganizeMoveBetweenGroups(t *testing.T) {
	db := migratedDB(t)
	k1, k2, k3 := looseVideo(t, db, "k1"), looseVideo(t, db, "k2"), looseVideo(t, db, "k3")
	a := createGroup(t, db, "A", domain.GroupAdd{VideoIDs: []int64{k1, k2}})
	b := createGroup(t, db, "B", domain.GroupAdd{VideoIDs: []int64{k3}})
	before := organizeSnapshot(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	image := moveCards(t, db, domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k1)}, From: a, To: b})
	assertEntries(t, db, a, "v:k2")
	assertEntries(t, db, b, "v:k3", "v:k1")
	want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{a, b}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("events = %+v, want %+v", recorder.events, want)
	}

	undoImageForTest(t, db, image)
	assertSnapshot(t, db, "after undo", before)
}

// ライブラリから入れた動画とグループの 3 枚は、画面の順のまま末尾に入る（受け入れ条件 7）。既に
// あるものは飛ばす。
func TestOrganizeMoveKeepsScreenOrder(t *testing.T) {
	db := migratedDB(t)
	k1, k2, k3 := looseVideo(t, db, "k1"), looseVideo(t, db, "k2"), looseVideo(t, db, "k3")
	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{k3}})
	to := createGroup(t, db, "To", domain.GroupAdd{VideoIDs: []int64{k1}})
	before := organizeSnapshot(t, db)

	image := moveCards(t, db, domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k3), groupRef(child), videoRef(k1), videoRef(k2)}, To: to})
	assertEntries(t, db, to, "v:k1", "v:k3", "g:"+itoa(child), "v:k2")
	undoImageForTest(t, db, image)
	assertSnapshot(t, db, "after undo", before)
}

// to に既にある動画は足さず、from から外すだけになる。
func TestOrganizeMoveSkipsItemsAlreadyInTarget(t *testing.T) {
	db := migratedDB(t)
	k1, k2 := looseVideo(t, db, "k1"), looseVideo(t, db, "k2")
	from := createGroup(t, db, "From", domain.GroupAdd{VideoIDs: []int64{k1, k2}})
	to := createGroup(t, db, "To", domain.GroupAdd{VideoIDs: []int64{k2, k1}})

	moveCards(t, db, domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k1)}, From: from, To: to})
	assertEntries(t, db, from, "v:k2")
	assertEntries(t, db, to, "v:k2", "v:k1")
}

// グループを自分の子孫へ、または自分自身へ動かすのは ErrGroupCycle で、何も書かない。無い
// グループ・動画や from に無いカードは ErrGroupTargetNotFound、from と to がどちらも無いか同じなら
// 誤りで、どれも何も書かない。
func TestOrganizeMoveRefusals(t *testing.T) {
	db := migratedDB(t)
	k1, k2 := looseVideo(t, db, "k1"), looseVideo(t, db, "k2")
	inner := createGroup(t, db, "Inner", domain.GroupAdd{VideoIDs: []int64{k1}})
	outer := createGroup(t, db, "Outer", domain.GroupAdd{GroupIDs: []int64{inner}})
	top := createGroup(t, db, "Top", domain.GroupAdd{GroupIDs: []int64{outer}, VideoIDs: []int64{k2}})
	missing := int64(999999)
	before := organizeSnapshot(t, db)
	for _, tc := range []struct {
		name string
		move domain.OrganizeMove
		want error
	}{
		{"子孫へ", domain.OrganizeMove{Items: []domain.OrganizeRef{groupRef(outer)}, From: top, To: inner}, domain.ErrGroupCycle},
		{"自分へ", domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k2), groupRef(outer)}, From: top, To: outer}, domain.ErrGroupCycle},
		{"無い to", domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k2)}, From: top, To: missing}, domain.ErrGroupTargetNotFound},
		{"無い from", domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k2)}, From: missing, To: inner}, domain.ErrGroupTargetNotFound},
		{"無い動画", domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(missing)}, To: inner}, domain.ErrGroupTargetNotFound},
		{"無いグループ", domain.OrganizeMove{Items: []domain.OrganizeRef{groupRef(missing)}, To: inner}, domain.ErrGroupTargetNotFound},
		{"from に無い", domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k2), videoRef(k1)}, From: top, To: inner}, domain.ErrGroupTargetNotFound},
	} {
		if _, err := db.Organize().Move(context.Background(), tc.move); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
	for _, m := range []domain.OrganizeMove{
		{Items: []domain.OrganizeRef{videoRef(k2)}},
		{Items: []domain.OrganizeRef{videoRef(k2)}, From: top, To: top},
	} {
		if _, err := db.Organize().Move(context.Background(), m); err == nil {
			t.Errorf("Move(%+v) succeeded", m)
		}
	}
	assertSnapshot(t, db, "refused moves wrote", before)
}

// A の最後の項目を外へ出すと A は解散し、空になった親も解散する。取り消すと A と親は id、
// お気に入り、プレイリストの項目ごと戻る。
func TestOrganizeMoveOutDissolvesAndUndoRestores(t *testing.T) {
	db := migratedDB(t)
	k1, k2 := looseVideo(t, db, "k1"), looseVideo(t, db, "k2")
	a := createGroup(t, db, "A", domain.GroupAdd{VideoIDs: []int64{k1}})
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{a}})
	setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{a, parent}, Favorite: true})
	playlist := createPlaylist(t, db, "List")
	addToPlaylist(t, db, playlist, domain.PlaylistAdd{GroupIDs: []int64{a}})
	addVideos(t, db, playlist, k2)
	before := organizeSnapshot(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	image := moveCards(t, db, domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k1)}, From: a})
	if groupExists(t, db, a) || groupExists(t, db, parent) {
		t.Fatal("the emptied groups still exist")
	}
	if got, want := playlistEntries(t, db, playlist), videoEntries(k2); !slices.Equal(got, want) {
		t.Fatalf("playlist after the move = %v, want %v", got, want)
	}
	want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{a, parent}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("events = %+v, want %+v", recorder.events, want)
	}
	recorder.events = nil

	undoImageForTest(t, db, image)
	assertSnapshot(t, db, "after undo", before)
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("undo events = %+v, want %+v", recorder.events, want)
	}
}

// 移動の後で戻すグループを別のグループに入れたなら、取り消しは ErrUndoConflict で何も書かない。
func TestOrganizeMoveUndoConflicts(t *testing.T) {
	db := migratedDB(t)
	k1, k2 := looseVideo(t, db, "k1"), looseVideo(t, db, "k2")
	from := createGroup(t, db, "From", domain.GroupAdd{VideoIDs: []int64{k1, k2}})
	to := createGroup(t, db, "To", domain.GroupAdd{VideoIDs: []int64{k2}})
	image := moveCards(t, db, domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k1)}, From: from, To: to})
	createGroup(t, db, "Later", domain.GroupAdd{GroupIDs: []int64{to}})

	before := organizeSnapshot(t, db)
	if err := db.Organize().Undo(context.Background(), image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo error = %v, want ErrUndoConflict", err)
	}
	assertSnapshot(t, db, "conflicting undo wrote", before)
}

// 自動グループのフォルダメンバーを外すと除外が書かれ、取り消すと除外が消えて同じ項目が戻る。
// 最後のメンバーまで外すと解散して ungroup が書かれ、取り消すと例外が消えてグループが同じ id で
// 戻り、作り直しでもそのまま残る。
func TestOrganizeMoveFolderMemberAndUndo(t *testing.T) {
	db := migratedDB(t)
	k1 := upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	k2 := upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	auto := readGroups(t, db)[0].ID
	before := organizeSnapshot(t, db)

	image := moveCards(t, db, domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k1)}, From: auto})
	if got := groupExclusions(t, db, auto); !slices.Equal(got, []string{"k1"}) {
		t.Fatalf("exclusions = %v", got)
	}
	undoImageForTest(t, db, image)
	assertSnapshot(t, db, "after undo", before)

	image = moveCards(t, db, domain.OrganizeMove{Items: []domain.OrganizeRef{videoRef(k2), videoRef(k1)}, From: auto})
	if groupExists(t, db, auto) {
		t.Fatal("the emptied automatic group still exists")
	}
	if grouping := folderGroupingOf(t, db, fixturePath("/media/Show")); grouping.Mode != domain.FolderGroupUngroup {
		t.Fatalf("grouping after emptying = %+v", grouping)
	}
	undoImageForTest(t, db, image)
	assertSnapshot(t, db, "after undoing the dissolve", before)
	rebuildIndex(t, db)
	assertSnapshot(t, db, "after the rebuild", before)
}
