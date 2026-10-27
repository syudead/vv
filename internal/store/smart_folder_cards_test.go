package store

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func smartFolderCards(t *testing.T, db *DB, audience domain.Audience, ids ...int64) []domain.SmartFolderCard {
	t.Helper()
	cards, err := db.SmartFolders().SmartFolderCards(context.Background(), audience, ids)
	if err != nil {
		t.Fatalf("SmartFolderCards(%v) error = %v", ids, err)
	}
	return cards
}

func TestSmartFolderCardsReadValuesOverMatchingVideos(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x, y := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")], ids[fixturePath("/media/mixed/sub/y.mp4")]
	cat := attachNamedTag(t, db, "猫", solo, x, y)
	if _, err := db.sql.Exec(`update videos set thumbnail_state = 'done' where id in (?, ?)`, solo, x); err != nil {
		t.Fatal(err)
	}
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortAddedDesc, 0)
	empty := newSmartFolder(t, db, "空", domain.SmartRule{Q: "どれにも当たらない"}, domain.SortAddedDesc, 0)
	favoriteSmartFolder(t, db, folder, 5)

	cards := smartFolderCards(t, db, domain.AudienceOwner, empty, 999, folder, empty)
	if len(cards) != 2 || cards[0].ID != empty || cards[1].ID != folder {
		t.Fatalf("cards = %+v, want [空 猫]", cards)
	}
	card := cards[1]
	if card.SmartFolderSummary != (domain.SmartFolderSummary{ID: folder, Name: "猫", Favorite: true}) {
		t.Errorf("要約 = %+v", card.SmartFolderSummary)
	}
	if card.VideoCount != 3 || card.SizeBytes != 1400 || card.DurationMs == nil || *card.DurationMs != 6100 {
		t.Errorf("本数・大きさ・長さ = %d %d %v", card.VideoCount, card.SizeBytes, card.DurationMs)
	}
	if !card.AddedAt.Equal(fixedTime.Add(9*time.Minute)) || card.LastPlayedAt == nil || card.LastPlayedAt.Unix() != 300 {
		t.Errorf("追加・再生 = %v %v", card.AddedAt, card.LastPlayedAt)
	}
	if card.WatchState != domain.WatchStateInProgress || card.WatchedCount != 1 || card.OpenVideoID != y {
		t.Errorf("視聴 = %s %d, 開く = %d, want inProgress 1 %d", card.WatchState, card.WatchedCount, card.OpenVideoID, y)
	}
	if got := videoIDsOf(card.Previews); !slices.Equal(got, []int64{solo, x}) {
		t.Errorf("絵柄 = %v, want [%d %d]", got, solo, x)
	}
	if len(card.Tags) != 1 || card.Tags[0].ID != cat || card.Tags[0].Name != "猫" || !card.Tags[0].Manual || card.Tags[0].FromFolder {
		t.Errorf("タグ = %+v", card.Tags)
	}

	blank := cards[0]
	if blank.VideoCount != 0 || blank.OpenVideoID != 0 || !blank.AddedAt.IsZero() || blank.DurationMs != nil ||
		blank.LastPlayedAt != nil || len(blank.Previews) != 0 || len(blank.Tags) != 0 || blank.WatchState != domain.WatchStateUnwatched {
		t.Errorf("空のカード = %+v", blank)
	}
}

// Guests do not see smart folders, so they get no cards; their library and folder list leave folders out too.
func TestSmartFolderCardsLeaveOutEveryFolderForGuests(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	if _, err := db.Visibility().SetVideosPublic(context.Background(), []int64{solo}, true); err != nil {
		t.Fatal(err)
	}
	cat := attachNamedTag(t, db, "猫", solo, x)
	shown := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)

	if cards := smartFolderCards(t, db, domain.AudienceGuest, shown); len(cards) != 0 {
		t.Fatalf("ゲストのカード = %+v, want none", cards)
	}
	if got := smartFolderCards(t, db, domain.AudienceOwner, shown); len(got) != 1 || got[0].VideoCount != 2 {
		t.Errorf("所有者のカード = %+v", got)
	}
}

func TestListSmartFoldersByNameWithParents(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	if _, err := db.Visibility().SetVideosPublic(context.Background(), []int64{solo}, true); err != nil {
		t.Fatal(err)
	}
	cat := attachNamedTag(t, db, "猫", solo, x)
	private := attachNamedTag(t, db, "非公開", x)
	parent := newSmartFolder(t, db, "b 猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	child := newSmartFolder(t, db, "a 子", domain.SmartRule{}, domain.SortTitleAsc, parent)
	hidden := newSmartFolder(t, db, "c 非公開", domain.SmartRule{Tags: []int64{private}}, domain.SortTitleAsc, 0)
	favoriteSmartFolder(t, db, hidden, 5)

	owner, err := db.SmartFolders().ListSmartFolders(context.Background(), domain.AudienceOwner)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SmartFolderSummary{
		{ID: child, ParentID: parent, Name: "a 子"},
		{ID: parent, Name: "b 猫"},
		{ID: hidden, Name: "c 非公開", Favorite: true},
	}
	if !slices.Equal(owner, want) {
		t.Errorf("所有者の一覧 = %+v, want %+v", owner, want)
	}
	guest, err := db.SmartFolders().ListSmartFolders(context.Background(), domain.AudienceGuest)
	if err != nil {
		t.Fatal(err)
	}
	if len(guest) != 0 {
		t.Errorf("ゲストの一覧 = %+v, want none", guest)
	}
}

func TestSmartFolderCardsOfAFolderWithAMissingTag(t *testing.T) {
	db, ids := itemsFixture(t)
	cat := attachNamedTag(t, db, "猫", ids[fixturePath("/media/solo.mp4")])
	parent := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	child := newSmartFolder(t, db, "全部", domain.SmartRule{}, domain.SortTitleAsc, parent)
	if err := db.Tags().DeleteTag(context.Background(), cat); err != nil {
		t.Fatal(err)
	}
	cards := smartFolderCards(t, db, domain.AudienceOwner, child)
	if len(cards) != 1 || cards[0].VideoCount != 0 || cards[0].ParentID != parent {
		t.Errorf("cards = %+v, want 値の無い 全部", cards)
	}
	if got := smartFolderCards(t, db, domain.AudienceGuest, child); len(got) != 0 {
		t.Errorf("ゲストのカード = %+v, want 空", got)
	}
}

func TestSmartFolderCardsApplyFavoriteRules(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x, ep2, p2 := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")],
		ids[fixturePath("/media/show/ep2.mp4")], ids[fixturePath("/media/pair/p2.mp4")]
	cat := attachNamedTag(t, db, "猫", solo, x, ep2, p2)
	favoriteVideos(t, db, true, x, p2)
	all := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	favorite := newSmartFolder(t, db, "お気に入りの猫", domain.SmartRule{Tags: []int64{cat}, Favorite: true}, domain.SortTitleAsc, 0)

	cards := smartFolderCards(t, db, domain.AudienceOwner, all, favorite)
	if len(cards) != 2 {
		t.Fatalf("cards = %+v", cards)
	}
	for i, want := range []struct {
		count int
		open  int64
		size  int64
	}{
		{4, ep2, 1050},
		{2, p2, 150},
	} {
		card := cards[i]
		if card.VideoCount != want.count || card.OpenVideoID != want.open || card.SizeBytes != want.size {
			t.Errorf("%s のカード = 本数 %d・開く %d・大きさ %d, want %d・%d・%d",
				card.Name, card.VideoCount, card.OpenVideoID, card.SizeBytes, want.count, want.open, want.size)
		}
	}
}

// Folder tags come only from indexed representatives, so nonrepresentative folder names cannot contribute tags.
func TestSmartFolderCardTagsOfABundleMatchTheLibraryItem(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile(fixturePath("/media/X/alpha.mp4"), "alpha", "key-a", 1, 0),
		sampleFile(fixturePath("/media/Y/bravo.mp4"), "bravo", "key-b", 1, 0),
	)
	a, b := ids[fixturePath("/media/X/alpha.mp4")], ids[fixturePath("/media/Y/bravo.mp4")]
	x := createTagForTest(t, db, "X")
	createTagForTest(t, db, "Y")
	versions := bundle(t, db, a, a, b)
	folder := newSmartFolder(t, db, "alpha", domain.SmartRule{Q: "alpha"}, domain.SortAddedDesc, 0)

	cards := smartFolderCards(t, db, domain.AudienceOwner, folder)
	library := videoTagsOf(t, db, versions.Items[0].UserKey)
	if len(cards) != 1 || cards[0].VideoCount != 1 || cards[0].OpenVideoID != a {
		t.Fatalf("cards = %+v, want 代表 A の 1 本", cards)
	}
	if !reflect.DeepEqual(cards[0].Tags, library) || len(library) != 1 || library[0].ID != x || !library[0].FromFolder {
		t.Errorf("カードのタグ = %+v, ライブラリのタグ = %+v, want どちらも X（フォルダ由来）だけ", cards[0].Tags, library)
	}
}

// Many search folders on one screen must not exceed SQLite's limits on one statement.
func TestSmartFolderCardsOfManySearchFolders(t *testing.T) {
	db, _ := itemsFixture(t)
	var ids []int64
	for i := range 40 {
		ids = append(ids, newSmartFolder(t, db, fmt.Sprintf("f%d", i), domain.SmartRule{Q: fmt.Sprintf("ep%d", i)}, domain.SortTitleAsc, 0))
	}
	cards, err := db.SmartFolders().SmartFolderCards(context.Background(), domain.AudienceOwner, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != len(ids) || cards[2].ID != ids[2] || cards[2].VideoCount != 1 {
		t.Fatalf("cards = %d, third = %+v; want %d cards and ep2 in f2", len(cards), cards[2], len(ids))
	}
}
