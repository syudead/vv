package store

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type itemFile struct {
	path     string
	added    int // Minutes.
	mtime    int // Minutes.
	size     int64
	duration *int64
	progress *domain.Progress
	played   int64  // Playback save time in Unix seconds; used only with progress.
	created  *int64 // Creation time in minutes; nil falls back to modification time.
}

// Include tied values to exercise ID tie breakers across groups and standalone videos.
var itemFiles = []itemFile{
	{path: fixturePath("/media/show/ep10.mp4"), added: 9, mtime: 1, size: 100, duration: ptr(1000),
		progress: &domain.Progress{PositionMs: 500}, played: 300, created: ptr(7)},
	{path: fixturePath("/media/show/ep1.mp4"), added: 2, mtime: 4, size: 200, duration: ptr(2000),
		progress: &domain.Progress{PositionMs: 2000, Completed: true}, played: 100},
	{path: fixturePath("/media/show/ep2.mp4"), added: 3, mtime: 2, size: 300, duration: nil,
		progress: &domain.Progress{PositionMs: 0}, played: 200},
	{path: fixturePath("/media/pair/p1.mp4"), added: 1, mtime: 8, size: 50, duration: nil, created: ptr(0)},
	{path: fixturePath("/media/pair/p2.mp4"), added: 5, mtime: 3, size: 50, duration: nil},
	{path: fixturePath("/media/solo.mp4"), added: 9, mtime: 8, size: 600, duration: ptr(3000),
		progress: &domain.Progress{PositionMs: 3000, Completed: true}, played: 300, created: ptr(2)},
	{path: fixturePath("/media/mixed/x.mp4"), added: 4, mtime: 8, size: 100, duration: ptr(3000), created: ptr(2)},
	{path: fixturePath("/media/mixed/sub/y.mp4"), added: 6, mtime: 5, size: 700, duration: ptr(100),
		progress: &domain.Progress{PositionMs: 50}, played: 50},
}

func itemsFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	ids := map[string]int64{}
	for _, file := range itemFiles {
		key := "key" + file.path
		var created time.Time
		if file.created != nil {
			created = fixedTime.Add(time.Duration(*file.created) * time.Minute)
		}
		got, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: file.path, Title: titleOf(file.path), ContentKey: key, SizeBytes: file.size,
			MTime: fixedTime.Add(time.Duration(file.mtime) * time.Minute), FileCreatedAt: created,
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
	return strings.TrimSuffix(filepath.Base(path), ".mp4")
}

// Videos use even keys, groups odd keys, and smart folders negative keys.
func itemKey(item domain.LibraryItem) int64 {
	if item.SmartFolder != nil {
		return -item.SmartFolder.ID
	}
	if item.Group != nil {
		return item.Group.ID*2 + 1
	}
	return item.Video.ID * 2
}

// Verify totals agree across pages and with the final item count.
func libraryPages(t *testing.T, db *DB, audience domain.Audience, q domain.VideoQuery) []domain.LibraryItem {
	t.Helper()
	var out []domain.LibraryItem
	for range 50 {
		page, err := db.Library().ListLibrary(context.Background(), audience, q)
		if err != nil {
			t.Fatalf("%+v: %v", q, err)
		}
		assertPageSize(t, len(page.Items), q.Limit, page.NextCursor)
		out = append(out, page.Items...)
		for _, item := range page.Items {
			if item.Group == nil || item.Group.Path == "" {
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
		switch {
		case item.SmartFolder != nil:
			out = append(out, "folder:"+item.SmartFolder.Name)
		case item.Group != nil:
			out = append(out, "group:"+item.Group.Name)
		default:
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
	if group.Card != nil {
		return group.Card.VideoIDs
	}
	ids := make([]int64, 0, len(group.Members))
	for _, member := range group.Members {
		ids = append(ids, member.ID)
	}
	return ids
}

func TestListLibraryGroupsFoldersIntoOneItem(t *testing.T) {
	db, ids := itemsFixture(t)
	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})
	names := itemNames(items)
	slices.Sort(names)
	if want := []string{"group:pair", "group:show", "solo", "x", "y"}; !slices.Equal(names, want) {
		t.Fatalf("項目 = %v, want %v", names, want)
	}

	show := findGroup(t, items, "show")
	if want := []int64{ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep2.mp4")], ids[fixturePath("/media/show/ep10.mp4")]}; !slices.Equal(memberIDs(show), want) {
		t.Errorf("show のメンバー = %v, want %v（自然順）", memberIDs(show), want)
	}
	if show.Path != fixturePath("/media/show") {
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

func TestListLibraryShowsPartiallyMatchedMembersAsVideos(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()

	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Query: "ep10", Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if names := itemNames(page.Items); !slices.Equal(names, []string{"ep10"}) || page.Total != 1 {
		t.Fatalf("ep10 の検索 = %v（total %d）, want [ep10]（total 1）", names, page.Total)
	}
	if page.Items[0].Video.ID != ids[fixturePath("/media/show/ep10.mp4")] {
		t.Errorf("ep10 の検索の動画 = %d, want ep10", page.Items[0].Video.ID)
	}

	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "show", Limit: domain.MaxLimit})
	if names := itemNames(items); !slices.Equal(names, []string{"group:show"}) {
		t.Fatalf("show の検索 = %v, want [group:show]", names)
	}
	if got := len(memberIDs(*items[0].Group)); got != 3 {
		t.Errorf("show の検索のグループの本数 = %d, want 3（全メンバー）", got)
	}

	// Apply playability after group formation so one unplayable member does not split the group.
	items = libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{PlayableOnly: true, Limit: domain.MaxLimit})
	names := itemNames(items)
	slices.Sort(names)
	if want := []string{"group:show", "solo", "x", "y"}; !slices.Equal(names, want) {
		t.Errorf("再生できるものだけ = %v, want %v", names, want)
	}
	if names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "ep2", PlayableOnly: true})); len(names) != 0 {
		t.Errorf("ep2 の検索で再生できるものだけ = %v, want 空", names)
	}
}

func TestListLibraryFiltersTagsPerMember(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	ep1, ep2, ep10 := ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep2.mp4")], ids[fixturePath("/media/show/ep10.mp4")]

	two, err := db.Tags().CreateTag(ctx, "two")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ep2, ep10}, two.ID); err != nil {
		t.Fatal(err)
	}
	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{two.ID}, Limit: domain.MaxLimit})
	names := itemNames(items)
	slices.Sort(names)
	if want := []string{"ep10", "ep2"}; !slices.Equal(names, want) {
		t.Errorf("タグ two = %v, want %v", names, want)
	}
	if names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{two.ID}, Watch: domain.WatchUnwatched})); !slices.Equal(names, []string{"ep2"}) {
		t.Errorf("タグ two の未視聴 = %v, want [ep2]", names)
	}
	selection, _, err := db.Library().LibraryIDs(ctx, domain.VideoQuery{TagIDs: []int64{two.ID}, Watch: domain.WatchUnwatched})
	if err != nil {
		t.Fatal(err)
	}
	got := selection.AllVideoIDs()
	if !slices.Equal(got, []int64{ep2}) {
		t.Errorf("タグ two の未視聴の ids = %v, want [%d]", got, ep2)
	}

	folder, err := db.Tags().CreateTag(ctx, "show")
	if err != nil {
		t.Fatal(err)
	}
	if names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{folder.ID}})); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("タグ show = %v, want [group:show]", names)
	}
	names = itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{folder.ID, two.ID}}))
	slices.Sort(names)
	if want := []string{"ep10", "ep2"}; !slices.Equal(names, want) {
		t.Errorf("タグ show と two = %v, want %v", names, want)
	}

	tagA, err := db.Tags().CreateTag(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ep1}, tagA.ID); err != nil {
		t.Fatal(err)
	}
	if names := itemNames(libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tagA.ID, two.ID}})); len(names) != 0 {
		t.Errorf("タグ a と two = %v, want 空（1本で両方を満たすメンバーが無い）", names)
	}
}

// Derive expected sort values from returned items, using fixture playback, creation and favorite times independently of SQL.
func itemSortValue(sort domain.VideoSort, seed int64, item domain.LibraryItem, played, created, modified, favorited map[int64]int64, groupFavorited map[string]int64) (isNull bool, num int64, text string) {
	lookup := func(value int64, ok bool) (bool, int64, string) {
		if !ok {
			return true, 0, ""
		}
		return false, value, ""
	}
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
			for _, id := range memberIDs(*group) {
				latest = max(latest, modified[id])
			}
			return false, latest, ""
		case domain.SortCreatedAsc, domain.SortCreatedDesc:
			var latest int64
			for _, id := range memberIDs(*group) {
				latest = max(latest, created[id])
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
		case domain.SortFavoritedAsc, domain.SortFavoritedDesc:
			value, ok := groupFavorited[group.Path]
			return lookup(value, ok)
		}
		return false, domain.ShuffleKey(seed, itemKey(item)), ""
	}
	video := item.Video
	switch sort {
	case domain.SortAddedAsc, domain.SortAddedDesc:
		return false, video.AddedAt.Unix(), ""
	case domain.SortModifiedAsc, domain.SortModifiedDesc:
		return false, video.MTime.Unix(), ""
	case domain.SortCreatedAsc, domain.SortCreatedDesc:
		return false, created[video.ID], ""
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
	case domain.SortFavoritedAsc, domain.SortFavoritedDesc:
		value, ok := favorited[video.ID]
		return lookup(value, ok)
	}
	return false, domain.ShuffleKey(seed, itemKey(item)), ""
}

func TestListLibraryAllSortsPageInExpectedOrder(t *testing.T) {
	db, ids := itemsFixture(t)
	played := map[int64]int64{}
	created := map[int64]int64{}
	modified := map[int64]int64{}
	for _, file := range itemFiles {
		if file.progress != nil {
			played[ids[file.path]] = file.played
		}
		modified[ids[file.path]] = fixedTime.Add(time.Duration(file.mtime) * time.Minute).Unix()
		minutes := int64(file.mtime)
		if file.created != nil {
			minutes = *file.created
		}
		created[ids[file.path]] = fixedTime.Add(time.Duration(minutes) * time.Minute).Unix()
	}
	favorited := map[int64]int64{
		ids[fixturePath("/media/solo.mp4")]:     30,
		ids[fixturePath("/media/mixed/x.mp4")]:  10,
		ids[fixturePath("/media/show/ep1.mp4")]: 50,
	}
	groupFavorited := map[string]int64{fixturePath("/media/pair"): 30}
	for id, at := range favorited {
		if _, err := db.sql.Exec(`insert into video_favorites (content_key, favorited_at)
			select content_key, ? from videos where id = ?`, at, id); err != nil {
			t.Fatal(err)
		}
	}
	for path, at := range groupFavorited {
		if _, err := db.sql.Exec(`insert into group_favorites (group_id, favorited_at) values (?, ?)`, groupIDOf(t, db, path), at); err != nil {
			t.Fatal(err)
		}
	}
	all := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})

	for _, tc := range pagingCases(allSorts, 2, 3) {
		sort := tc.value
		const seed = 42
		want := slices.Clone(all)
		desc := sort != domain.SortRandom && sort[len(sort)-4:] == "Desc"
		slices.SortFunc(want, func(a, b domain.LibraryItem) int {
			aNull, aNum, aText := itemSortValue(sort, seed, a, played, created, modified, favorited, groupFavorited)
			bNull, bNum, bText := itemSortValue(sort, seed, b, played, created, modified, favorited, groupFavorited)
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
		got := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Sort: sort, Seed: seed, Limit: tc.limit})
		if !slices.Equal(itemNames(got), itemNames(want)) {
			t.Errorf("%s limit=%d = %v, want %v", sort, tc.limit, itemNames(got), itemNames(want))
		}
	}
}

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

// Compare SQL watch-state filtering with the domain's group/video classification.
func TestListLibraryWatchStateMatchesDomain(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	items := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})
	show := findGroup(t, items, "show")
	if show.WatchState != domain.WatchStateInProgress || show.WatchedCount != 1 {
		t.Errorf("show = %s・%d, want inProgress・1", show.WatchState, show.WatchedCount)
	}
	if show.OpenVideoID != ids[fixturePath("/media/show/ep10.mp4")] {
		t.Errorf("show の開くメンバー = %d, want ep10（途中まで見た最初のメンバー）", show.OpenVideoID)
	}
	if pair := findGroup(t, items, "pair"); pair.WatchState != domain.WatchStateUnwatched || pair.OpenVideoID != ids[fixturePath("/media/pair/p1.mp4")] {
		t.Errorf("pair = %s・開く %d, want unwatched・p1", pair.WatchState, pair.OpenVideoID)
	}

	for _, name := range []string{"ep2", "ep10"} {
		if _, err := db.sql.Exec(`update playback_progress set completed = 1 where content_key = ?`, "key"+fixturePath("/media/show/"+name+".mp4")); err != nil {
			t.Fatal(err)
		}
	}
	for _, filter := range []domain.WatchFilter{domain.WatchUnwatched, domain.WatchInProgress, domain.WatchWatched} {
		got := libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Watch: filter, Limit: 2})
		for _, item := range got {
			var state domain.WatchState
			if item.Group != nil {
				state = item.Group.WatchState
				if filter == domain.WatchWatched && item.Group.OpenVideoID != memberIDs(*item.Group)[0] {
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
	out := make([]*domain.Progress, 0, len(memberIDs(group)))
	for _, id := range memberIDs(group) {
		var key string
		if err := db.sql.QueryRow(`select content_key from videos where id = ?`, id).Scan(&key); err != nil {
			t.Fatal(err)
		}
		if p, err := db.Playback().Progress(context.Background(), key); err == nil {
			out = append(out, &p)
		} else {
			out = append(out, nil)
		}
	}
	return out
}

func TestLibraryIDsFollowLibraryItems(t *testing.T) {
	db, ids := itemsFixture(t)
	ep10 := ids[fixturePath("/media/show/ep10.mp4")]
	selection, missing, err := db.Library().LibraryIDs(context.Background(), domain.VideoQuery{Query: "ep10 OR solo", TagIDs: []int64{999}})
	if err != nil {
		t.Fatal(err)
	}
	got := selection.AllVideoIDs()
	if !slices.Equal(missing, []int64{999}) {
		t.Errorf("missing = %v", missing)
	}
	slices.Sort(got)
	want := []int64{ep10, ids[fixturePath("/media/solo.mp4")]}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}

	selection, _, err = db.Library().LibraryIDs(context.Background(), domain.VideoQuery{Query: "ep10"})
	if err != nil {
		t.Fatal(err)
	}
	got = selection.AllVideoIDs()
	if !slices.Equal(got, []int64{ep10}) {
		t.Errorf("ep10 の ids = %v, want [%d]", got, ep10)
	}

	selection, _, err = db.Library().LibraryIDs(context.Background(), domain.VideoQuery{Query: "show"})
	if err != nil {
		t.Fatal(err)
	}
	got = selection.AllVideoIDs()
	slices.Sort(got)
	want = []int64{ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep2.mp4")], ep10}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("show の ids = %v, want %v", got, want)
	}

	selection, _, err = db.Library().LibraryIDs(context.Background(), domain.VideoQuery{Watch: domain.WatchUnwatched})
	if err != nil {
		t.Fatal(err)
	}
	got = selection.AllVideoIDs()
	slices.Sort(got)
	want = []int64{ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")], ids[fixturePath("/media/mixed/x.mp4")]}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("未視聴の ids = %v, want %v", got, want)
	}
}

func TestListLibraryForGuestCountsPublicMembers(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	public := []int64{ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep10.mp4")], ids[fixturePath("/media/pair/p2.mp4")], ids[fixturePath("/media/solo.mp4")]}
	if _, err := db.Visibility().SetVideosPublic(ctx, public, true); err != nil {
		t.Fatal(err)
	}

	items := libraryPages(t, db, domain.AudienceGuest, domain.VideoQuery{Limit: 1})
	names := itemNames(items)
	slices.Sort(names)
	if want := []string{"group:pair", "group:show", "solo"}; !slices.Equal(names, want) {
		t.Fatalf("ゲストの項目 = %v, want %v", names, want)
	}
	if pair := findGroup(t, items, "pair"); !slices.Equal(memberIDs(pair), []int64{ids[fixturePath("/media/pair/p2.mp4")]}) {
		t.Errorf("ゲストの pair のメンバー = %v, want [p2]", memberIDs(pair))
	}
	show := findGroup(t, items, "show")
	if want := []int64{ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/show/ep10.mp4")]}; !slices.Equal(memberIDs(show), want) {
		t.Errorf("ゲストの show のメンバー = %v, want %v", memberIDs(show), want)
	}
	if show.SizeBytes != 300 || show.DurationMs == nil || *show.DurationMs != 3000 {
		t.Errorf("ゲストの show の大きさ・長さ = %d・%v, want 300・3000", show.SizeBytes, show.DurationMs)
	}
	if show.LastPlayedAt != nil || show.WatchedCount != 0 || show.OpenVideoID != ids[fixturePath("/media/show/ep1.mp4")] {
		t.Errorf("ゲストの show に再生の記録が出た: %+v", show)
	}
	if names := itemNames(libraryPages(t, db, domain.AudienceGuest, domain.VideoQuery{Query: "ep10"})); !slices.Equal(names, []string{"ep10"}) {
		t.Errorf("ゲストの ep10 の検索 = %v, want [ep10]", names)
	}
	items = libraryPages(t, db, domain.AudienceGuest, domain.VideoQuery{Query: "show"})
	if names := itemNames(items); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("ゲストの show の検索 = %v, want [group:show]", names)
	} else if got := len(memberIDs(*items[0].Group)); got != 2 {
		t.Errorf("ゲストの show の検索のグループの本数 = %d, want 2", got)
	}
	if names := itemNames(libraryPages(t, db, domain.AudienceGuest, domain.VideoQuery{Query: "ep2"})); len(names) != 0 {
		t.Errorf("ゲストの ep2 の検索 = %v, want 空", names)
	}

	pair, err := db.Library().Group(ctx, domain.AudienceGuest, groupIDOf(t, db, fixturePath("/media/pair")), nil, "")
	if err != nil || len(pair.Group.Members) != 1 {
		t.Errorf("ゲストの pair（公開1本）= %+v, %v", pair.Group, err)
	}
	group, err := db.Library().Group(ctx, domain.AudienceGuest, groupIDOf(t, db, fixturePath("/media/show")), nil, "")
	if err != nil || len(group.Group.Members) != 2 || len(group.Items) != 2 {
		t.Errorf("ゲストの show = %+v, %v", group, err)
	}
}

// An empty page cannot carry a window count and must still return the full filtered total.
func TestListLibraryCountsWithoutItemsAfterTheCursor(t *testing.T) {
	db, _ := itemsFixture(t)
	ctx := context.Background()
	first, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortSizeDesc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemNames(first.Items); !slices.Equal(got, []string{"y"}) || first.NextCursor == "" {
		t.Fatalf("first page = %v (cursor %q), want [y] with a cursor", got, first.NextCursor)
	}
	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{
		Sort: domain.SortSizeDesc, Query: "y", Cursor: first.NextCursor, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Total != 1 || page.NextCursor != "" {
		t.Errorf("page = %d items, total %d, cursor %q; want 0 items, total 1, no cursor",
			len(page.Items), page.Total, page.NextCursor)
	}
}
