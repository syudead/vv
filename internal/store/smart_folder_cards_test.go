package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// スマートフォルダのカードの読み出し（specs/052-smart-folders/data-model.md「Cards read」、
// research.md R-14）。

func smartFolderCards(t *testing.T, db *DB, audience domain.Audience, ids ...int64) []domain.SmartFolderCard {
	t.Helper()
	cards, err := db.SmartFolders().SmartFolderCards(context.Background(), audience, ids)
	if err != nil {
		t.Fatalf("SmartFolderCards(%v) error = %v", ids, err)
	}
	return cards
}

// カードの値は一致する動画の本数・合計・最大・視聴状態と、フォルダの並べ順の先頭からの絵柄と開く
// 動画である。カードは求めた順に返り、無い id は飛ばす。
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
	// 追加の降順は solo(9)・y(6)・x(4)。見ている途中は y、見終えたのは solo。
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

// 視聴状態の条件では、項目にならない混ざったグループの一致する動画も数える。show は ep2 だけが未視聴で、
// フォルダのページ（再生順）は空だが、カードは ep2 の 1 本である。条件と並べ順が同じフォルダも、それぞれ
// 自分の要約で同じ値を返す。
func TestSmartFolderCardsCountMatchingVideosOfMixedGroups(t *testing.T) {
	db, ids := itemsFixture(t)
	ep2 := ids[fixturePath("/media/show/ep2.mp4")]
	rule := domain.SmartRule{Q: "ep", Watch: domain.WatchUnwatched}
	folder := newSmartFolder(t, db, "未視聴のep", rule, domain.SortTitleAsc, 0)
	twin := newSmartFolder(t, db, "同じ", rule, domain.SortTitleAsc, 0)

	page, err := db.SmartFolders().SmartFolderContents(context.Background(), domain.AudienceOwner, folder, "", 0)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("ページ = %v, %v, want 空", smartItemNames(page.Items), err)
	}
	cards := smartFolderCards(t, db, domain.AudienceOwner, folder, twin)
	if len(cards) != 2 {
		t.Fatalf("cards = %+v", cards)
	}
	for i, want := range []string{"未視聴のep", "同じ"} {
		card := cards[i]
		if card.Name != want || card.VideoCount != 1 || card.OpenVideoID != ep2 || card.WatchState != domain.WatchStateUnwatched {
			t.Errorf("%s のカード = %+v, want 1 本で ep2 を開く", want, card)
		}
	}
}

// ゲストには、再生順の空のフォルダのカードを返さない。見せるフォルダのカードは公開の動画だけから作り、
// タグを載せない。
func TestSmartFolderCardsLeaveOutFoldersHiddenFromGuests(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	if _, err := db.Visibility().SetVideosPublic(context.Background(), []int64{solo}, true); err != nil {
		t.Fatal(err)
	}
	cat := attachNamedTag(t, db, "猫", solo, x)
	private := attachNamedTag(t, db, "非公開", x)
	shown := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	hidden := newSmartFolder(t, db, "非公開", domain.SmartRule{Tags: []int64{private}}, domain.SortTitleAsc, 0)

	cards := smartFolderCards(t, db, domain.AudienceGuest, hidden, shown)
	if len(cards) != 1 || cards[0].ID != shown {
		t.Fatalf("ゲストのカード = %+v, want [猫]", cards)
	}
	if cards[0].VideoCount != 1 || cards[0].OpenVideoID != solo || len(cards[0].Tags) != 0 {
		t.Errorf("ゲストの 猫 のカード = %+v", cards[0])
	}
	if got := smartFolderCards(t, db, domain.AudienceOwner, hidden, shown); len(got) != 2 || got[1].VideoCount != 2 {
		t.Errorf("所有者のカード = %+v", got)
	}
}

// フォルダの一覧はどの深さのものも名前の順に親の id とともに返り、ゲストには再生順の空のフォルダが
// 入らない（GET /api/smart-folders、research.md R-8）。
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
	if !slices.Equal(guest, want[:2]) {
		t.Errorf("ゲストの一覧 = %+v, want %+v", guest, want[:2])
	}
}

// 経路の条件が無い実体を名指すフォルダは、所有者には値の無いカードで返る（research.md R-9）。
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
