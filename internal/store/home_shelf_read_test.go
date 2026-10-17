package store

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// homeShelfReadFixture は itemsFixture（グループ show・pair、グループに入らない動画）に、ホームの棚の
// 読み出しが分ける場合を足す。手で作った入れ子のグループ nest（pair を子に持つ）、2 つの根に入る動画
// （ep2 は show と other に入る）、集まり（x と y）、タグ、お気に入り、公開の動画である。
func homeShelfReadFixture(t *testing.T) (*DB, map[string]int64, map[string]int64) {
	t.Helper()
	db, ids := itemsFixture(t)
	ctx := context.Background()
	id := func(name string) int64 { return ids[fixturePath("/media/"+name)] }
	tags := map[string]int64{}
	for _, name := range []string{"cat", "dog"} {
		tag, err := db.Tags().CreateTag(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		tags[name] = tag.ID
	}
	// cat は show の一部と solo に、dog は show と pair の全部に付く。
	for _, name := range []string{"show/ep10.mp4", "show/ep1.mp4", "solo.mp4"} {
		attachTag(t, db, "key"+fixturePath("/media/"+name), tags["cat"])
	}
	for _, name := range []string{"show/ep10.mp4", "show/ep1.mp4", "show/ep2.mp4", "pair/p1.mp4", "pair/p2.mp4"} {
		attachTag(t, db, "key"+fixturePath("/media/"+name), tags["dog"])
	}
	if _, err := db.Groups().CreateGroup(ctx, "nest", domain.GroupAdd{VideoIDs: []int64{id("mixed/sub/y.mp4")},
		GroupIDs: []int64{groupIDOf(t, db, fixturePath("/media/pair"))}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Groups().CreateGroup(ctx, "other", domain.GroupAdd{VideoIDs: []int64{id("show/ep2.mp4"), id("solo.mp4")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Versions().Bundle(ctx, []int64{id("mixed/x.mp4"), id("mixed/sub/y.mp4")}, id("mixed/x.mp4")); err != nil {
		t.Fatal(err)
	}
	for name, at := range map[string]int64{"solo.mp4": 30, "mixed/x.mp4": 10, "show/ep1.mp4": 50} {
		if _, err := db.sql.Exec(`insert into video_favorites (content_key, favorited_at) values (?, ?)`,
			"key"+fixturePath("/media/"+name), at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.sql.Exec(`insert into group_favorites (group_id, favorited_at) values (?, 30)`,
		groupIDOf(t, db, fixturePath("/media/pair"))); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"show/ep10.mp4", "show/ep1.mp4", "show/ep2.mp4", "pair/p1.mp4", "solo.mp4", "mixed/x.mp4"} {
		if _, err := db.sql.Exec(`insert into public_videos (content_key, published_at) values (?, 1)`,
			"key"+fixturePath("/media/"+name)); err != nil {
			t.Fatal(err)
		}
	}
	return db, ids, tags
}

// homeShelfReadShelves は、並び順ごとのタグと視聴状態の棚、検索式（1〜2 文字は instr、3 文字以上は
// 全文索引）、再生可否、お気に入りのみ、長さ、追加日、親の条件を重ねる入れ子のフォルダの棚を作って
// ホームに並べ、その id を返す。
func homeShelfReadShelves(t *testing.T, db *DB, tags map[string]int64) []int64 {
	t.Helper()
	ctx := context.Background()
	var ids []int64
	create := func(name string, rule domain.SmartRule, sort domain.VideoSort, parent int64) int64 {
		t.Helper()
		folder, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{Name: name, Rule: rule, Sort: sort, ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, folder.ID)
		return folder.ID
	}
	for _, sort := range allSorts {
		if sort == domain.SortRandom {
			// スマートフォルダの並び順にシャッフルは無い。
			continue
		}
		create("cat "+string(sort), domain.SmartRule{Tags: []int64{tags["cat"]}}, sort, 0)
		create("progress "+string(sort), domain.SmartRule{Watch: domain.WatchInProgress}, sort, 0)
	}
	create("ep", domain.SmartRule{Q: "ep"}, domain.SortTitleAsc, 0)
	create("show", domain.SmartRule{Q: "show"}, domain.SortAddedDesc, 0)
	create("playable", domain.SmartRule{Playable: true}, domain.SortDurationDesc, 0)
	create("favorite", domain.SmartRule{Favorite: true}, domain.SortFavoritedDesc, 0)
	short := 2
	create("short", domain.SmartRule{MaxDuration: &short}, domain.SortSizeAsc, 0)
	day := 1
	create("today", domain.SmartRule{AddedWithin: &day}, domain.SortModifiedDesc, 0)
	dog := create("dog", domain.SmartRule{Tags: []int64{tags["dog"]}}, domain.SortAddedAsc, 0)
	create("dog watched", domain.SmartRule{Watch: domain.WatchWatched}, domain.SortCreatedDesc, dog)
	if err := db.Home().SetShelves(ctx, ids); err != nil {
		t.Fatal(err)
	}
	return ids
}

// itemIDs は項目の鍵（itemKey）の並びである。
func itemIDs(items []domain.LibraryItem) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, itemKey(item))
	}
	return out
}

// ホームの棚の読み出し（一時表から読む home_shelf_read.go）は、どの棚でも、ライブラリの項目の問い合わせ
// （libraryItemsCTE）の最初のページと同じ項目・同じ並び・同じ数を返す。所有者とゲストの両方で、並び順の
// すべてと、グループの一部だけが当たる棚、入れ子のグループ、2 つの根に入る動画、集まり、検索式、
// 再生可否、お気に入りのみ、入れ子のフォルダの棚、決まった棚（最近追加・棚にない動画・続きから見る）を
// 確かめる。
func TestHomeShelfReadMatchesTheLibraryItemsRead(t *testing.T) {
	db, _, tags := homeShelfReadFixture(t)
	listNow = func() time.Time { return fixedTime.Add(time.Hour) }
	t.Cleanup(func() { listNow = time.Now })
	ids := homeShelfReadShelves(t, db, tags)
	ctx := context.Background()
	keys := []domain.HomeShelfKey{{Kind: domain.HomeShelfRecent}, {Kind: domain.HomeShelfUnshelved}, {Kind: domain.HomeShelfContinue}}
	for _, id := range ids {
		keys = append(keys, domain.FolderShelfKey(id))
	}
	for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
		page, err := db.Home().ShelfItems(ctx, audience, keys)
		if err != nil {
			t.Fatal(err)
		}
		got := map[domain.HomeShelfKey]domain.HomeShelfItems{}
		for _, shelf := range page.Shelves {
			got[shelf.Key] = shelf
		}
		tx, err := db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		now := listNow().Unix()
		checked := 0
		for _, key := range keys {
			spec, found, err := homeShelfItemsSpec(ctx, tx, audience, key, now)
			if err != nil {
				t.Fatal(err)
			}
			if !found || spec == nil {
				if _, ok := got[key]; ok && !found {
					t.Errorf("%s %v: 棚でない鍵が返った", audience, key)
				}
				continue
			}
			if err := fillShelved(ctx, tx, spec); err != nil {
				t.Fatal(err)
			}
			want, err := listLibraryPageTx(ctx, tx, *spec)
			if err != nil {
				t.Fatal(err)
			}
			shelf, ok := got[key]
			if !ok {
				if want.Total != 0 || audience.IsOwner() {
					t.Errorf("%s %v: 棚が無い（ライブラリの項目は %d 件）", audience, key, want.Total)
				}
				continue
			}
			if shelf.Total != want.Total || !slices.Equal(itemIDs(shelf.Items), itemIDs(want.Items)) {
				t.Errorf("%s %v (%s): items = %v total = %d, want %v total = %d", audience, key, spec.sort,
					itemIDs(shelf.Items), shelf.Total, itemIDs(want.Items), want.Total)
			}
			if want.Total > 0 {
				checked++
			}
		}
		_ = tx.Rollback()
		if checked < 20 {
			t.Errorf("%s: 項目のある棚が %d しか無い", audience, checked)
		}
	}
}

// 読み出しの一時表は取引とともに消え、接続に残らない。
func TestHomeShelfReadLeavesNoTempTable(t *testing.T) {
	db, _, tags := homeShelfReadFixture(t)
	homeShelfReadShelves(t, db, tags)
	db.read.SetMaxOpenConns(1)
	ctx := context.Background()
	if _, err := db.Home().ShelfItems(ctx, domain.AudienceOwner, []domain.HomeShelfKey{{Kind: domain.HomeShelfUnshelved}, {Kind: domain.HomeShelfRecent}}); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.read.QueryRowContext(ctx, `select count(*) from sqlite_temp_master`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("一時表が %d 残っている", left)
	}
}

// 無いフォルダの鍵だけを読むときは一時表を作らず、空の応答を返す。
func TestHomeShelfReadWithNoShelfBuildsNothing(t *testing.T) {
	db, _, _ := homeShelfReadFixture(t)
	page, err := db.Home().ShelfItems(context.Background(), domain.AudienceOwner, []domain.HomeShelfKey{domain.FolderShelfKey(999)})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Shelves) != 0 {
		t.Errorf("shelves = %+v", page.Shelves)
	}
}
