package store

import (
	"context"
	"database/sql"
	"fmt"
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
	// 視聴状態だけの棚は視聴状態の表から引く（watchDriven）。
	create("unwatched", domain.SmartRule{Watch: domain.WatchUnwatched}, domain.SortAddedDesc, 0)
	create("watched", domain.SmartRule{Watch: domain.WatchWatched}, domain.SortModifiedDesc, 0)
	create("unwatched created", domain.SmartRule{Watch: domain.WatchUnwatched}, domain.SortCreatedAsc, 0)
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

// itemCards は項目ごとのカードの値（グループは名前・フォルダ・本数・視聴・開くメンバー・合計・日時・
// お気に入り・絵柄のサムネイル、動画は id と見せる所在のパスと題名）を比べられる文字列にする。
func itemCards(items []domain.LibraryItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Group == nil {
			out = append(out, fmt.Sprintf("video %d %q %q", item.Video.ID, item.Video.Path, item.Video.Title))
			continue
		}
		g := item.Group
		duration, played := int64(-1), int64(-1)
		if g.DurationMs != nil {
			duration = *g.DurationMs
		}
		if g.LastPlayedAt != nil {
			played = g.LastPlayedAt.Unix()
		}
		previews := make([]int64, 0, len(g.Previews))
		for _, preview := range g.Previews {
			previews = append(previews, preview.VideoID)
		}
		out = append(out, fmt.Sprintf("group %d %q %q count=%d watch=%s/%d open=%d duration=%d size=%d added=%d played=%d fav=%v previews=%v",
			g.ID, g.Name, g.Path, g.VideoCount, g.WatchState, g.WatchedCount, g.OpenVideoID, duration, g.SizeBytes, g.AddedAt.Unix(), played, g.Favorite, previews))
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
	keys := []domain.HomeShelfKey{{Kind: domain.HomeShelfRecent}, {Kind: domain.HomeShelfUnshelved}, {Kind: domain.HomeShelfContinue}}
	for _, id := range ids {
		keys = append(keys, domain.FolderShelfKey(id))
	}
	for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
		if checked := requireShelvesMatchLibrary(t, db, audience, keys); checked < 20 {
			t.Errorf("%s: 項目のある棚が %d しか無い", audience, checked)
		}
	}
}

// 検索式の棚の動画の項目は、ライブラリと同じく検索式に当たる所在（そのパスの最小）を見せる。動画の先頭の
// 所在が当たらず 2 つめの所在だけが当たるときも、先頭の所在を見せない（PR #1348 のレビュー）。
func TestHomeShelfReadShowsTheLocationTheSearchMatches(t *testing.T) {
	db, _, _ := homeShelfReadFixture(t)
	listNow = func() time.Time { return fixedTime.Add(time.Hour) }
	t.Cleanup(func() { listNow = time.Now })
	ctx := context.Background()
	if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: fixturePath("/media/zz-unique.mp4"), Title: "zz-unique", ContentKey: "key" + fixturePath("/media/solo.mp4"),
		SizeBytes: 7, MTime: fixedTime.Add(-time.Hour), AddedAt: fixedTime, Container: "mp4",
	}); err != nil {
		t.Fatal(err)
	}
	var keys []domain.HomeShelfKey
	var shelves []int64
	for _, sort := range []domain.VideoSort{domain.SortTitleAsc, domain.SortModifiedDesc, domain.SortAddedDesc, domain.SortPlayedDesc} {
		folder, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{Name: "unique " + string(sort),
			Rule: domain.SmartRule{Q: "unique"}, Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		shelves = append(shelves, folder.ID)
		keys = append(keys, domain.FolderShelfKey(folder.ID))
	}
	if err := db.Home().SetShelves(ctx, shelves); err != nil {
		t.Fatal(err)
	}
	for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
		if checked := requireShelvesMatchLibrary(t, db, audience, keys); checked != len(keys) {
			t.Errorf("%s: 項目のある棚が %d、want %d", audience, checked, len(keys))
		}
	}
}

// requireShelvesMatchLibrary は、見る人 audience の棚 keys の読み出しが、どれもライブラリの項目の問い合わせの
// 最初のページと同じ項目・並び・数・カードの値を返すことを確かめ、項目のある棚の数を返す。
func requireShelvesMatchLibrary(t *testing.T, db *DB, audience domain.Audience, keys []domain.HomeShelfKey) int {
	t.Helper()
	ctx := context.Background()
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
	defer func() { _ = tx.Rollback() }()
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
		if shelf.Total != min(want.Total, domain.HomeShelfItemLimit+1) || !slices.Equal(itemIDs(shelf.Items), itemIDs(want.Items)) {
			t.Errorf("%s %v (%s): items = %v total = %d, want %v total = %d", audience, key, spec.sort,
				itemIDs(shelf.Items), shelf.Total, itemIDs(want.Items), want.Total)
		}
		// グループの項目はカードの値だけで作る（loadGroupCards）が、値はメンバーから作るライブラリの項目と同じ。
		if got, wantCards := itemCards(shelf.Items), itemCards(want.Items); !slices.Equal(got, wantCards) {
			t.Errorf("%s %v (%s): cards = %v, want %v", audience, key, spec.sort, got, wantCards)
		}
		if want.Total > 0 {
			checked++
		}
	}
	return checked
}

// 無いフォルダの鍵だけを読むときは空の応答を返す。
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

// 長さの分からない動画（duration が NULL）を持つグループに長さの条件を掛けると、その動画は当たらないので
// グループは丸ごと当たらず、ライブラリと同じく当たった動画が 1 本ずつ並ぶ（条件が NULL のメンバーを
// 「当たらない」と見る。PR #1348 のレビュー）。
func TestHomeShelfReadTreatsANullRuleResultAsNoMatch(t *testing.T) {
	db, ids, _ := homeShelfReadFixture(t)
	ctx := context.Background()
	long := 3000
	folder, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{Name: "long", Rule: domain.SmartRule{MaxDuration: &long}, Sort: domain.SortAddedDesc})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Home().SetShelves(ctx, []int64{folder.ID}); err != nil {
		t.Fatal(err)
	}
	page, err := db.Home().ShelfItems(ctx, domain.AudienceOwner, []domain.HomeShelfKey{domain.FolderShelfKey(folder.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Shelves) != 1 {
		t.Fatalf("shelves = %+v", page.Shelves)
	}
	// show は ep2 の長さが分からないので丸ごと当たらず、ep10 と ep1 が動画で出る。
	got := itemIDs(page.Shelves[0].Items)
	for _, name := range []string{"show/ep10.mp4", "show/ep1.mp4"} {
		if !slices.Contains(got, ids[fixturePath("/media/"+name)]*2) {
			t.Errorf("%s が動画の項目に無い: %v", name, got)
		}
	}
	if slices.Contains(got, groupIDOf(t, db, fixturePath("/media/show"))*2+1) {
		t.Errorf("show がグループの項目で出た: %v", got)
	}
}

// 1 本の動画が多くの根に入っていても（まとまりの行数を超える）、その動画の行は 1 つのまとまりで読み切り、
// ライブラリと同じ項目を返す（PR #1348 のレビュー）。
func TestHomeShelfReadReadsAVideoInManyGroupsWhole(t *testing.T) {
	db, ids, _ := homeShelfReadFixture(t)
	ctx := context.Background()
	solo := ids[fixturePath("/media/solo.mp4")]
	for i := range homeShelfTopBatch + 5 {
		if _, err := db.Groups().CreateGroup(ctx, fmt.Sprintf("many-%d", i), domain.GroupAdd{VideoIDs: []int64{solo, ids[fixturePath("/media/mixed/x.mp4")]}}); err != nil {
			t.Fatal(err)
		}
	}
	folder, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{Name: "all", Rule: domain.SmartRule{Playable: true}, Sort: domain.SortAddedDesc})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Home().SetShelves(ctx, []int64{folder.ID}); err != nil {
		t.Fatal(err)
	}
	page, err := db.Home().ShelfItems(ctx, domain.AudienceOwner, []domain.HomeShelfKey{domain.FolderShelfKey(folder.ID)})
	if err != nil || len(page.Shelves) != 1 {
		t.Fatalf("shelves = %+v, %v", page.Shelves, err)
	}
	tx, err := db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	spec, _, err := homeShelfItemsSpec(ctx, tx, domain.AudienceOwner, domain.FolderShelfKey(folder.ID), listNow().Unix())
	if err != nil {
		t.Fatal(err)
	}
	want, err := listLibraryPageTx(ctx, tx, *spec)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(itemIDs(page.Shelves[0].Items), itemIDs(want.Items)) {
		t.Errorf("items = %v, want %v", itemIDs(page.Shelves[0].Items), itemIDs(want.Items))
	}
}

// freshClosure は group_closure を再帰で求め直したもの（root_id, content_key, sort_key）である。
func freshClosure(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.sql.Query(`with ` + groupDescendantsCTEFrom(`select id from groups`, `gi.content_key`, ``) + `
		select root_id, video_id, sort_key from gdesc order by root_id, video_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var root int64
		var key, sortKey string
		if err := rows.Scan(&root, &key, &sortKey); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%d %s %s", root, key, sortKey))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// storedClosure は group_closure の行である。
func storedClosure(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.sql.Query(`select root_id, content_key, sort_key from group_closure order by root_id, content_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var root int64
		var key, sortKey string
		if err := rows.Scan(&root, &key, &sortKey); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%d %s %s", root, key, sortKey))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// requireClosureCurrent は group_closure が再帰で求め直したものと等しく、印が無いことを確かめる。
func requireClosureCurrent(t *testing.T, db *DB, step string) {
	t.Helper()
	if got, want := storedClosure(t, db), freshClosure(t, db); !slices.Equal(got, want) {
		t.Errorf("%s: group_closure = %v, want %v", step, got, want)
	}
	dirty, err := groupClosureDirty(context.Background(), db.sql)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Errorf("%s: group_closure に印が残っている", step)
	}
}

// グループの構造を変えるどの書き込みも、同じ取引で group_closure を作り直す: 取り込みのフォルダの自動
// グループ、グループの作成・項目の追加と削除・解散、整理の結合、集まり（鍵の付け替え）。
func TestGroupClosureFollowsEveryWriter(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	requireClosureCurrent(t, db, "フォルダの自動グループ")
	id := func(name string) int64 { return ids[fixturePath("/media/"+name)] }
	group, err := db.Groups().CreateGroup(ctx, "nest", domain.GroupAdd{VideoIDs: []int64{id("mixed/sub/y.mp4")},
		GroupIDs: []int64{groupIDOf(t, db, fixturePath("/media/pair"))}})
	if err != nil {
		t.Fatal(err)
	}
	requireClosureCurrent(t, db, "グループの作成")
	if _, err := db.Groups().AddGroupItems(ctx, group.ID, domain.GroupAdd{VideoIDs: []int64{id("solo.mp4")}}); err != nil {
		t.Fatal(err)
	}
	requireClosureCurrent(t, db, "項目の追加")
	var itemID int64
	if err := db.sql.QueryRow(`select id from group_items where group_id = ? and content_key is not null order by position limit 1`, group.ID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Groups().RemoveGroupItems(ctx, group.ID, []int64{itemID}); err != nil {
		t.Fatal(err)
	}
	requireClosureCurrent(t, db, "項目の削除")
	other, err := db.Groups().CreateGroup(ctx, "other", domain.GroupAdd{VideoIDs: []int64{id("show/ep2.mp4"), id("solo.mp4")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Organize().Merge(ctx, []domain.OrganizeRef{{Kind: domain.OrganizeGroup, ID: other.ID}}, group.ID, 0); err != nil {
		t.Logf("merge: %v", err)
	}
	requireClosureCurrent(t, db, "整理の結合")
	if err := db.Groups().DissolveGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	requireClosureCurrent(t, db, "解散")
	if _, err := db.Versions().Bundle(ctx, []int64{id("show/ep10.mp4"), id("show/ep1.mp4")}, id("show/ep10.mp4")); err != nil {
		t.Fatal(err)
	}
	requireClosureCurrent(t, db, "集まり")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	requireClosureCurrent(t, db, "フォルダの自動グループの再同期")
}

// 動画・所在・再生の記録・公開の書き込みは引き金で根の値を消し、確定のあとの維持が埋め直した値は、所有者と
// ゲストのどちらも子孫から集め直したものと等しい。棚への一致（home_shelved_videos）も求め直したものと等しい。
func TestGroupRootValuesFollowVideoWrites(t *testing.T) {
	db, ids, tags := homeShelfReadFixture(t)
	homeShelfReadShelves(t, db, tags)
	ctx := context.Background()
	check := func(step string) {
		t.Helper()
		if err := db.maintainHomeIndexes(ctx); err != nil {
			t.Fatal(err)
		}
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		roots, err := queryInt64s(ctx, tx, `select id from groups order by id`)
		if err != nil {
			t.Fatal(err)
		}
		for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
			stored, err := readStoredRootValues(ctx, tx, audience, roots)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := readRootValues(ctx, tx, audience, roots, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, root := range roots {
				got, want := stored[root], fresh[root]
				want.titleKey, want.favoritedAt = "", sql.NullInt64{}
				if got.members == 0 && want.members == 0 {
					continue
				}
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("%s (%s): root %d の値 = %+v, want %+v", step, audience, root, got, want)
				}
			}
		}
		// 棚への一致と視聴状態: 表と、規則（視聴状態）を全動画に掛け直したものが等しい。
		userKey := userKeyExpr("videos")
		fresh, err := queryStrings(ctx, tx, `select distinct `+userKey+` || ' ' || `+videoWatchStateSQL(userKey)+` from videos where videos.content_key <> '' order by 1`)
		if err != nil {
			t.Fatal(err)
		}
		states, err := queryStrings(ctx, tx, `select content_key || ' ' || state from video_watch_states order by 1`)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(states, fresh) {
			t.Errorf("%s: 視聴状態 = %v, want %v", step, states, fresh)
		}
		if _, err := tx.ExecContext(ctx, `update home_shelved_state set all_dirty = 1`); err != nil {
			t.Fatal(err)
		}
		before, err := queryStrings(ctx, tx, `select content_key from home_shelved_videos order by content_key`)
		if err != nil {
			t.Fatal(err)
		}
		if err := refillShelvedVideos(ctx, tx); err != nil {
			t.Fatal(err)
		}
		after, err := queryStrings(ctx, tx, `select content_key from home_shelved_videos order by content_key`)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(before, after) {
			t.Errorf("%s: 棚への一致 = %v, want %v", step, before, after)
		}
		// 検索の当たり: 表と、全文索引から読み直したものが等しい。
		shelves, err := homeShelfSpecs(ctx, tx, domain.AudienceOwner, listNow().Unix())
		if err != nil {
			t.Fatal(err)
		}
		searched := 0
		for _, shelf := range shelves {
			if !shelf.hasSearch() {
				continue
			}
			searched++
			query, args := searchVideosSelect(shelf.scope, shelf.searchExprs())
			want, err := queryInt64s(ctx, tx, `select video_id from (`+query+`) order by 1`, args...)
			if err != nil {
				t.Fatal(err)
			}
			got, err := queryInt64s(ctx, tx, `select video_id from home_search_hits where search = ? order by 1`, searchIdentity(shelf))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s: 検索の当たり %v = %v, want %v", step, shelf.searchExprs(), got, want)
			}
		}
		if searched == 0 {
			t.Errorf("%s: 検索式の棚が無い", step)
		}
	}
	check("最初の読み出し")
	db.homeShelves.dropFor([]domain.Event{domain.HomeChanged{}})
	if _, err := db.sql.Exec(`update playback_progress set position_ms = 10, completed = 0 where content_key = ?`,
		"key"+fixturePath("/media/show/ep1.mp4")); err != nil {
		t.Fatal(err)
	}
	check("再生の記録")
	db.homeShelves.dropFor([]domain.Event{domain.HomeChanged{}})
	if err := db.Ingest().ApplyProbe(ctx, ids[fixturePath("/media/show/ep2.mp4")], domain.Probe{DurationMs: 5000, VideoCodec: "h264"},
		domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}
	check("長さの確定")
	if _, err := db.sql.Exec(`insert into public_videos (content_key, published_at) values (?, 1)`, "key"+fixturePath("/media/pair/p2.mp4")); err != nil {
		t.Fatal(err)
	}
	check("公開")
	tags2, err := db.Tags().CreateTag(ctx, "bird")
	if err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key"+fixturePath("/media/mixed/x.mp4"), tags2.ID)
	check("タグ")
	db.homeShelves.dropFor([]domain.Event{domain.HomeChanged{}})
	if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{Path: fixturePath("/media/pair/p3.mp4"), Title: "p3", ContentKey: "key-p3",
		SizeBytes: 9, MTime: fixedTime, AddedAt: fixedTime.Add(time.Hour), Container: "mp4"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	check("取り込み")
}

// 追加日からの日数の棚（今日）への一致は時刻で変わるので表に入れず、読み出しが行ごとに掛ける: 棚にない
// 動画の棚は、書き込みが無くても日数が過ぎた動画を出す（R-11）。
func TestHomeShelfReadUnshelvedFollowsTheClock(t *testing.T) {
	db, _, _ := homeShelfReadFixture(t)
	ctx := context.Background()
	// 取り込みの動画はどれも基準の時刻の直後に追加したものである。
	listNow = func() time.Time { return fixedTime.Add(time.Hour) }
	t.Cleanup(func() { listNow = time.Now })
	day := 1
	today, err := db.SmartFolders().CreateSmartFolder(ctx, domain.SmartFolderCreate{Name: "today", Rule: domain.SmartRule{AddedWithin: &day}, Sort: domain.SortAddedDesc})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Home().SetShelves(ctx, []int64{today.ID}); err != nil {
		t.Fatal(err)
	}
	keys := []domain.HomeShelfKey{{Kind: domain.HomeShelfUnshelved}, domain.FolderShelfKey(today.ID)}
	read := func() (unshelved, shelf []int64) {
		t.Helper()
		db.homeShelves.dropFor([]domain.Event{domain.HomeChanged{}})
		page, err := db.Home().ShelfItems(ctx, domain.AudienceOwner, keys)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Shelves) != 2 {
			t.Fatalf("shelves = %+v", page.Shelves)
		}
		return itemIDs(page.Shelves[0].Items), itemIDs(page.Shelves[1].Items)
	}
	pending, err := shelvedMarksPending(ctx, db.sql)
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("棚への一致の印が残っている")
	}
	unshelved, today0 := read()
	if len(unshelved) != 0 || len(today0) == 0 {
		t.Errorf("今日: unshelved = %v, today = %v", unshelved, today0)
	}
	listNow = func() time.Time { return fixedTime.Add(3 * 24 * time.Hour) }
	if unshelved, shelf := read(); !slices.Equal(unshelved, today0) || len(shelf) != 0 {
		t.Errorf("3 日後: unshelved = %v, today = %v, want unshelved = %v", unshelved, shelf, today0)
	}
}
