package store

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// itemFile はライブラリの項目の検証用の所在1件である。nil は値が無いことを表す。
type itemFile struct {
	path     string
	added    int // 分
	mtime    int // 分
	size     int64
	duration *int64
	progress *domain.Progress
	played   int64 // playback_progress.updated_at（Unix 秒）。progress があるときだけ使う
}

// itemFiles は、グループ show（3本）と pair（2本）、グループにならない動画3本
// （登録フォルダ直下の solo、子フォルダを持つ mixed の x、1本だけの mixed/sub の y）
// である。値の同じ項目を混ぜ、id での決着も確かめる。
var itemFiles = []itemFile{
	{path: "/media/show/ep10.mp4", added: 9, mtime: 1, size: 100, duration: ptr(1000),
		progress: &domain.Progress{PositionMs: 500}, played: 300},
	{path: "/media/show/ep1.mp4", added: 2, mtime: 4, size: 200, duration: ptr(2000),
		progress: &domain.Progress{PositionMs: 2000, Completed: true}, played: 100},
	{path: "/media/show/ep2.mp4", added: 3, mtime: 2, size: 300, duration: nil,
		progress: &domain.Progress{PositionMs: 0}, played: 200},
	{path: "/media/pair/p1.mp4", added: 1, mtime: 8, size: 50, duration: nil},
	{path: "/media/pair/p2.mp4", added: 5, mtime: 3, size: 50, duration: nil},
	{path: "/media/solo.mp4", added: 9, mtime: 8, size: 600, duration: ptr(3000),
		progress: &domain.Progress{PositionMs: 3000, Completed: true}, played: 300},
	{path: "/media/mixed/x.mp4", added: 4, mtime: 8, size: 100, duration: ptr(3000)},
	{path: "/media/mixed/sub/y.mp4", added: 6, mtime: 5, size: 700, duration: ptr(100),
		progress: &domain.Progress{PositionMs: 50}, played: 50},
}

// itemsFixture は itemFiles を取り込んで索引を作り、パスごとの動画の id を返す。
func itemsFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	ids := map[string]int64{}
	for _, file := range itemFiles {
		key := "key" + file.path
		got, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: file.path, Title: titleOf(file.path), ContentKey: key, SizeBytes: file.size,
			MTime:   fixedTime.Add(time.Duration(file.mtime) * time.Minute),
			AddedAt: fixedTime.Add(time.Duration(file.added) * time.Minute), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids[file.path] = got.ID
		if file.duration != nil {
			if err := db.Ingest().ApplyProbe(ctx, got.ID, domain.Probe{DurationMs: *file.duration, VideoCodec: "h264"},
				domain.Playability{Playable: true}); err != nil {
				t.Fatal(err)
			}
		}
		if file.progress != nil {
			if _, err := db.sql.Exec(`insert into playback_progress (content_key, position_ms, duration_ms, completed, updated_at)
				values (?, ?, ?, ?, ?)`, key, file.progress.PositionMs, file.progress.DurationMs,
				boolToInt(file.progress.Completed), file.played); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	return db, ids
}

func titleOf(path string) string {
	base := path[len("/media/"):]
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '/' {
			base = base[i+1:]
			break
		}
	}
	return base[:len(base)-len(".mp4")]
}

// itemKey は項目の keyset の id（動画の id か、グループの最初のメンバーの id）である。
func itemKey(item domain.LibraryItem) int64 {
	if item.Group != nil {
		return item.Group.Members[0].ID
	}
	return item.Video.ID
}

// libraryPages は limit 件ずつカーソルで最後まで読み、出た項目を順に返す。どの
// ページの total も同じで、項目の数と一致することを確かめる。
func libraryPages(t *testing.T, db *DB, audience domain.Audience, q domain.VideoQuery) []domain.LibraryItem {
	t.Helper()
	var out []domain.LibraryItem
	for range 50 {
		page, err := db.Library().ListLibrary(context.Background(), audience, q)
		if err != nil {
			t.Fatalf("%+v: %v", q, err)
		}
		out = append(out, page.Items...)
		// グループのフォルダは、同じスナップショットの登録フォルダの下にある。
		for _, item := range page.Items {
			if item.Group == nil {
				continue
			}
			if _, ok := domain.LocateFolder(page.Roots, item.Group.Path); !ok {
				t.Errorf("%+v: グループ %s が登録フォルダ %+v の下に無い", q, item.Group.Path, page.Roots)
			}
		}
		if page.NextCursor == "" {
			if page.Total != len(out) {
				t.Errorf("%+v: total = %d, items = %d", q, page.Total, len(out))
			}
			return out
		}
		q.Cursor = page.NextCursor
	}
	t.Fatalf("%+v: ページが終わらない", q)
	return nil
}

func itemNames(items []domain.LibraryItem) []string {
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

func findGroup(t *testing.T, items []domain.LibraryItem, name string) domain.LibraryGroup {
	t.Helper()
	for _, item := range items {
		if item.Group != nil && item.Group.Name == name {
			return *item.Group
		}
	}
	t.Fatalf("グループ %s が無い: %v", name, itemNames(items))
	return domain.LibraryGroup{}
}

func memberIDs(group domain.LibraryGroup) []int64 {
	ids := make([]int64, 0, len(group.Members))
	for _, member := range group.Members {
		ids = append(ids, member.ID)
	}
	return ids
}

// 受け入れ条件 1・2: グループは1件の項目になり、名前・本数・合計の長さ・大きさ・
// 追加日時・最初のメンバー（cover）を全メンバーから作る。
func TestListLibraryGroupsFoldersIntoOneItem(t *testing.T) {
	db, ids := itemsFixture(t)
	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})
	names := itemNames(items)
	slices.Sort(names)
	if want := []string{"group:pair", "group:show", "solo", "x", "y"}; !slices.Equal(names, want) {
		t.Fatalf("項目 = %v, want %v", names, want)
	}

	show := findGroup(t, items, "show")
	if want := []int64{ids["/media/show/ep1.mp4"], ids["/media/show/ep2.mp4"], ids["/media/show/ep10.mp4"]}; !slices.Equal(memberIDs(show), want) {
		t.Errorf("show のメンバー = %v, want %v（自然順）", memberIDs(show), want)
	}
	if show.Path != "/media/show" {
		t.Errorf("show のパス = %q", show.Path)
	}
	if show.DurationMs == nil || *show.DurationMs != 3000 {
		t.Errorf("show の長さ = %v, want 3000（分かっているメンバーの合計）", show.DurationMs)
	}
	if show.SizeBytes != 600 {
		t.Errorf("show の大きさ = %d, want 600", show.SizeBytes)
	}
	if want := fixedTime.Add(9 * time.Minute); !show.AddedAt.Equal(want) {
		t.Errorf("show の追加日時 = %v, want %v", show.AddedAt, want)
	}
	if show.LastPlayedAt == nil || show.LastPlayedAt.Unix() != 300 {
		t.Errorf("show の最後に再生した時刻 = %v, want 300", show.LastPlayedAt)
	}
	if pair := findGroup(t, items, "pair"); pair.DurationMs != nil || pair.LastPlayedAt != nil {
		t.Errorf("pair の長さ・再生 = %v・%v, want どちらも無い", pair.DurationMs, pair.LastPlayedAt)
	}
}

// 受け入れ条件 11: 1本のメンバーだけが検索語に当たるグループも出て、値は全メンバーから作る。
// 条件はメンバー単位で、1本の動画が全条件を満たす必要がある（要件 19）。
func TestListLibraryFiltersPerMember(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()

	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "ep10", Limit: domain.MaxLimit})
	if names := itemNames(items); !slices.Equal(names, []string{"group:show"}) {
		t.Fatalf("ep10 の検索 = %v, want [group:show]", names)
	}
	if got := len(items[0].Group.Members); got != 3 {
		t.Errorf("当たったグループの本数 = %d, want 3（全メンバー）", got)
	}

	// 再生可否は ep2 だけが満たさない。グループには ep1・ep10 が当たる。
	items = libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{PlayableOnly: true, Limit: domain.MaxLimit})
	names := itemNames(items)
	slices.Sort(names)
	if want := []string{"group:show", "solo", "x", "y"}; !slices.Equal(names, want) {
		t.Errorf("再生できるものだけ = %v, want %v", names, want)
	}

	// タグの AND は1本の動画に求める。ep1 に a、ep2 に b を付けると、a と b の両方では
	// show は出ない。
	tagA, err := db.Tags().CreateTag(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	tagB, err := db.Tags().CreateTag(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids["/media/show/ep1.mp4"]}, tagA.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids["/media/show/ep2.mp4"]}, tagB.ID); err != nil {
		t.Fatal(err)
	}
	if names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tagA.ID}})); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("タグ a = %v, want [group:show]", names)
	}
	if names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tagA.ID, tagB.ID}})); len(names) != 0 {
		t.Errorf("タグ a と b = %v, want 空（1本で両方を満たすメンバーが無い）", names)
	}
}

// itemSortValue は項目の並べ替えの値を、ListLibrary が返した項目から Go で作る
// （data-model.md §5 の 3）。played は動画の id から再生の時刻を引く。
func itemSortValue(sort domain.VideoSort, seed int64, item domain.LibraryItem, played map[int64]int64) (isNull bool, num int64, text string) {
	nullable := func(v *int64) (bool, int64, string) {
		if v == nil {
			return true, 0, ""
		}
		return false, *v, ""
	}
	if group := item.Group; group != nil {
		switch sort {
		case domain.SortAddedAsc, domain.SortAddedDesc:
			return false, group.AddedAt.Unix(), ""
		case domain.SortModifiedAsc, domain.SortModifiedDesc:
			var latest int64
			for _, member := range group.Members {
				latest = max(latest, member.MTime.Unix())
			}
			return false, latest, ""
		case domain.SortTitleAsc, domain.SortTitleDesc:
			return false, 0, domain.NaturalSortKey(group.Name)
		case domain.SortDurationAsc, domain.SortDurationDesc:
			return nullable(group.DurationMs)
		case domain.SortSizeAsc, domain.SortSizeDesc:
			return false, group.SizeBytes, ""
		case domain.SortPlayedAsc, domain.SortPlayedDesc:
			if group.LastPlayedAt == nil {
				return true, 0, ""
			}
			return false, group.LastPlayedAt.Unix(), ""
		}
		return false, domain.ShuffleKey(seed, itemKey(item)), ""
	}
	video := item.Video
	switch sort {
	case domain.SortAddedAsc, domain.SortAddedDesc:
		return false, video.AddedAt.Unix(), ""
	case domain.SortModifiedAsc, domain.SortModifiedDesc:
		return false, video.MTime.Unix(), ""
	case domain.SortTitleAsc, domain.SortTitleDesc:
		return false, 0, domain.NaturalSortKey(video.Title)
	case domain.SortDurationAsc, domain.SortDurationDesc:
		return nullable(video.DurationMs)
	case domain.SortSizeAsc, domain.SortSizeDesc:
		return false, video.SizeBytes, ""
	case domain.SortPlayedAsc, domain.SortPlayedDesc:
		if value, ok := played[video.ID]; ok {
			return false, value, ""
		}
		return true, 0, ""
	}
	return false, domain.ShuffleKey(seed, video.ID), ""
}

// 受け入れ条件 10: 13 の並び順すべてで、項目が要件 18 の値で並び、ページをまたいで
// 重複と抜けが無く、total が項目の数と一致する。
func TestListLibraryAllSortsPageInExpectedOrder(t *testing.T) {
	db, ids := itemsFixture(t)
	played := map[int64]int64{}
	for _, file := range itemFiles {
		if file.progress != nil {
			played[ids[file.path]] = file.played
		}
	}
	all := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})

	for _, sort := range allSorts {
		const seed = 42
		want := slices.Clone(all)
		desc := sort != domain.SortRandom && sort[len(sort)-4:] == "Desc"
		slices.SortFunc(want, func(a, b domain.LibraryItem) int {
			aNull, aNum, aText := itemSortValue(sort, seed, a, played)
			bNull, bNum, bText := itemSortValue(sort, seed, b, played)
			if aNull != bNull {
				if aNull {
					return 1
				}
				return -1
			}
			c := cmp.Or(cmp.Compare(aNum, bNum), cmp.Compare(aText, bText), cmp.Compare(itemKey(a), itemKey(b)))
			if desc {
				return -c
			}
			return c
		})
		for _, limit := range []int{1, 2, 3} {
			got := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: sort, Seed: seed, Limit: limit})
			if !slices.Equal(itemNames(got), itemNames(want)) {
				t.Errorf("%s limit=%d = %v, want %v", sort, limit, itemNames(got), itemNames(want))
			}
		}
	}
}

// 別の並び順のカーソルは誤りになる（013 と同じカーソルの形）。
func TestListLibraryRejectsCursorOfOtherSort(t *testing.T) {
	db, _ := itemsFixture(t)
	ctx := context.Background()
	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortSizeAsc, Cursor: page.NextCursor, Limit: 1})
	if !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("err = %v, want ErrInvalidCursor", err)
	}
}

// 受け入れ条件 12・14: グループの視聴状態・見終えた本数・開くメンバーと、視聴状態の
// 絞り込み。SQL の視聴状態（絞り込み）は domain.GroupWatch・ClassifyWatch と同じ結果になる。
func TestListLibraryWatchStateMatchesDomain(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})
	show := findGroup(t, items, "show")
	if show.WatchState != domain.WatchStateInProgress || show.WatchedCount != 1 {
		t.Errorf("show = %s・%d, want inProgress・1", show.WatchState, show.WatchedCount)
	}
	if show.OpenVideoID != ids["/media/show/ep10.mp4"] {
		t.Errorf("show の開くメンバー = %d, want ep10（途中まで見た最初のメンバー）", show.OpenVideoID)
	}
	if pair := findGroup(t, items, "pair"); pair.WatchState != domain.WatchStateUnwatched || pair.OpenVideoID != ids["/media/pair/p1.mp4"] {
		t.Errorf("pair = %s・開く %d, want unwatched・p1", pair.WatchState, pair.OpenVideoID)
	}

	// 全部を完了にすると watched になり、開くのは最初のメンバーになる。
	for _, name := range []string{"ep2", "ep10"} {
		if _, err := db.sql.Exec(`update playback_progress set completed = 1 where content_key = ?`, "key/media/show/"+name+".mp4"); err != nil {
			t.Fatal(err)
		}
	}
	for _, filter := range []domain.WatchFilter{domain.WatchUnwatched, domain.WatchInProgress, domain.WatchWatched} {
		got := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Watch: filter, Limit: 2})
		for _, item := range got {
			var state domain.WatchState
			if item.Group != nil {
				state = item.Group.WatchState
				if filter == domain.WatchWatched && item.Group.OpenVideoID != item.Group.Members[0].ID {
					t.Errorf("完了した %s の開くメンバー = %d, want 最初", item.Group.Name, item.Group.OpenVideoID)
				}
			} else {
				var progress *domain.Progress
				if p, err := db.Playback().Progress(ctx, item.Video.ContentKey); err == nil {
					progress = &p
				}
				state = domain.ClassifyWatch(progress)
			}
			if !filter.Matches(state) {
				t.Errorf("%s の一覧に %v（%s）が出た", filter, itemNames([]domain.LibraryItem{item}), state)
			}
		}
		// 絞り込みの結果と全体を視聴状態で分けた結果が一致する。
		all := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})
		want := 0
		for _, item := range all {
			var state domain.WatchState
			if item.Group != nil {
				state, _ = domain.GroupWatch(groupProgress(t, db, *item.Group))
			} else {
				var progress *domain.Progress
				if p, err := db.Playback().Progress(ctx, item.Video.ContentKey); err == nil {
					progress = &p
				}
				state = domain.ClassifyWatch(progress)
			}
			if filter.Matches(state) {
				want++
			}
		}
		if len(got) != want {
			t.Errorf("%s = %v（%d 件）, want %d 件", filter, itemNames(got), len(got), want)
		}
	}
}

func groupProgress(t *testing.T, db *DB, group domain.LibraryGroup) []*domain.Progress {
	t.Helper()
	out := make([]*domain.Progress, 0, len(group.Members))
	for _, member := range group.Members {
		if p, err := db.Playback().Progress(context.Background(), member.ContentKey); err == nil {
			out = append(out, &p)
		} else {
			out = append(out, nil)
		}
	}
	return out
}

// 「すべて選択」の id は、当たった動画と当たったグループの全メンバーである（要件 21）。
func TestLibraryIDsIncludeAllMembersOfMatchedGroups(t *testing.T) {
	db, ids := itemsFixture(t)
	got, missing, err := db.Library().LibraryIDs(context.Background(), domain.VideoQuery{Query: "ep10 OR solo", TagIDs: []int64{999}})
	if err != nil {
		t.Fatal(err)
	}
	// 存在しないタグは条件から落ちる。
	if !slices.Equal(missing, []int64{999}) {
		t.Errorf("missing = %v", missing)
	}
	slices.Sort(got)
	want := []int64{ids["/media/show/ep1.mp4"], ids["/media/show/ep2.mp4"], ids["/media/show/ep10.mp4"], ids["/media/solo.mp4"]}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}

	got, _, err = db.Library().LibraryIDs(context.Background(), domain.VideoQuery{Watch: domain.WatchUnwatched})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	want = []int64{ids["/media/pair/p1.mp4"], ids["/media/pair/p2.mp4"], ids["/media/mixed/x.mp4"]}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("未視聴の ids = %v, want %v", got, want)
	}
}

// ゲストには公開のメンバーだけで数えた項目を返す。公開のメンバーが1本のグループは
// 動画の項目になり、0本のグループは出ない（data-model.md §7）。
func TestListLibraryForGuestCountsPublicMembers(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	public := []int64{ids["/media/show/ep1.mp4"], ids["/media/show/ep10.mp4"], ids["/media/pair/p2.mp4"], ids["/media/solo.mp4"]}
	if _, err := db.Visibility().SetVideosPublic(ctx, public, true); err != nil {
		t.Fatal(err)
	}

	items := libraryPages(t, db, domain.AudienceGuest, domain.VideoQuery{Limit: 1})
	names := itemNames(items)
	slices.Sort(names)
	if want := []string{"group:show", "p2", "solo"}; !slices.Equal(names, want) {
		t.Fatalf("ゲストの項目 = %v, want %v", names, want)
	}
	show := findGroup(t, items, "show")
	if want := []int64{ids["/media/show/ep1.mp4"], ids["/media/show/ep10.mp4"]}; !slices.Equal(memberIDs(show), want) {
		t.Errorf("ゲストの show のメンバー = %v, want %v", memberIDs(show), want)
	}
	if show.SizeBytes != 300 || show.DurationMs == nil || *show.DurationMs != 3000 {
		t.Errorf("ゲストの show の大きさ・長さ = %d・%v, want 300・3000", show.SizeBytes, show.DurationMs)
	}
	if show.LastPlayedAt != nil || show.WatchedCount != 0 || show.OpenVideoID != ids["/media/show/ep1.mp4"] {
		t.Errorf("ゲストの show に再生の記録が出た: %+v", show)
	}
	// ゲストの検索は非公開のメンバー（ep2）に当たらない。
	if names := itemNames(libraryPages(t, db, domain.AudienceGuest, domain.VideoQuery{Query: "ep2"})); len(names) != 0 {
		t.Errorf("ゲストの ep2 の検索 = %v, want 空", names)
	}

	if _, err := db.Library().FolderGroup(ctx, domain.AudienceGuest, "/media/pair"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ゲストの pair（公開1本）= %v, want ErrNotFound", err)
	}
	group, err := db.Library().FolderGroup(ctx, domain.AudienceGuest, "/media/show")
	if err != nil || len(group.Members) != 2 {
		t.Errorf("ゲストの show = %+v, %v", group, err)
	}
}

// グループ1件は絞り込みに関係なく全メンバーから作り、グループでないフォルダは
// ErrNotFound である。
func TestFolderGroup(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	group, err := db.Library().FolderGroup(ctx, domain.AudienceOwner, "/media/show/")
	if err != nil {
		t.Fatal(err)
	}
	if group.Name != "show" || len(group.Members) != 3 || group.OpenVideoID != ids["/media/show/ep10.mp4"] {
		t.Errorf("show = %+v", group)
	}
	for _, dir := range []string{"/media/mixed", "/media/mixed/sub", "/media", "/media/none"} {
		if _, err := db.Library().FolderGroup(ctx, domain.AudienceOwner, dir); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s = %v, want ErrNotFound", dir, err)
		}
	}

	// 「まとめを解除」するとグループでなくなり、メンバーは動画の項目に戻る。
	if err := db.FolderGroups().SetOverride(ctx, "/media/show", domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Library().FolderGroup(ctx, domain.AudienceOwner, "/media/show"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("解除した show = %v, want ErrNotFound", err)
	}
	names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit}))
	slices.Sort(names)
	if want := []string{"ep1", "ep10", "ep2", "group:pair", "solo", "x", "y"}; !slices.Equal(names, want) {
		t.Errorf("解除後の項目 = %v, want %v", names, want)
	}
}
