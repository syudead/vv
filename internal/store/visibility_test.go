package store

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 公開フラグと、見る人ごとの読み出し（specs/016-single-account-auth/data-model.md
// §1・§3・§5）。

var (
	guest = domain.AudienceGuest
	owner = domain.AudienceOwner
)

// setPublic は公開フラグを切り替え、反映した本数を返す。
func setPublic(t *testing.T, db *DB, public bool, ids ...int64) int {
	t.Helper()
	applied, err := db.Visibility().SetVideosPublic(context.Background(), ids, public)
	if err != nil {
		t.Fatalf("公開フラグを切り替えられない: %v", err)
	}
	return applied
}

// visibilityFixture は公開と非公開の動画を混ぜた索引を作る。
//
//	/media/pub/a.mp4       公開
//	/media/pub/b.mp4       非公開
//	/media/pub/deep/c.mp4  公開
//	/media/pub/deep/d.mp4  非公開
//	/media/private/e.mp4   非公開（このフォルダには非公開の動画しか無い）
//	/media/f.mp4           公開
//	/media/g.mp4           非公開
func visibilityFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	files := []domain.VideoFile{
		listingFile("/media/pub/a.mp4", "a", "key-a", 1),
		listingFile("/media/pub/b.mp4", "b", "key-b", 2),
		listingFile("/media/pub/deep/c.mp4", "c", "key-c", 3),
		listingFile("/media/pub/deep/d.mp4", "d", "key-d", 4),
		listingFile("/media/private/e.mp4", "e", "key-e", 5),
		listingFile("/media/f.mp4", "f", "key-f", 6),
		listingFile("/media/g.mp4", "g", "key-g", 7),
	}
	for index := range files {
		files[index].MTime = fixedTime.Add(time.Duration(len(files)-index) * time.Second)
	}
	ids := upsertAll(t, db, files...)
	if applied := setPublic(t, db, true, ids["/media/pub/a.mp4"], ids["/media/pub/deep/c.mp4"], ids["/media/f.mp4"]); applied != 3 {
		t.Fatalf("applied = %d, want 3", applied)
	}
	return db, ids
}

func listTitles(t *testing.T, db *DB, audience domain.Audience, q domain.VideoQuery) ([]string, int) {
	t.Helper()
	q.Limit = domain.MaxLimit
	page, err := db.Library().ListVideos(context.Background(), audience, q)
	if err != nil {
		t.Fatalf("一覧に失敗した (%+v): %v", q, err)
	}
	titles := titlesOf(page)
	slices.Sort(titles)
	return titles, page.Total
}

// ゲストの一覧・件数・検索には公開の動画だけが現れ、所有者には全件が現れる。
// Video.Public は公開フラグを写す。
func TestListVideosByAudience(t *testing.T) {
	db, _ := visibilityFixture(t)
	ctx := context.Background()

	titles, total := listTitles(t, db, guest, domain.VideoQuery{})
	if want := []string{"a", "c", "f"}; !slices.Equal(titles, want) || total != len(want) {
		t.Errorf("ゲスト: titles = %v total = %d, want %v", titles, total, want)
	}
	titles, total = listTitles(t, db, owner, domain.VideoQuery{})
	if want := []string{"a", "b", "c", "d", "e", "f", "g"}; !slices.Equal(titles, want) || total != len(want) {
		t.Errorf("所有者: titles = %v total = %d, want %v", titles, total, want)
	}

	// 検索（所在のパスに当たる語）も公開の動画だけに当たる。
	titles, total = listTitles(t, db, guest, domain.VideoQuery{Query: "pub"})
	if want := []string{"a", "c"}; !slices.Equal(titles, want) || total != len(want) {
		t.Errorf("ゲストの検索: titles = %v total = %d, want %v", titles, total, want)
	}
	titles, _ = listTitles(t, db, owner, domain.VideoQuery{Query: "pub"})
	if want := []string{"a", "b", "c", "d"}; !slices.Equal(titles, want) {
		t.Errorf("所有者の検索: titles = %v, want %v", titles, want)
	}

	for audience, want := range map[domain.Audience]int{guest: 3, owner: 7} {
		count, err := db.Library().CountVideos(ctx, audience, "")
		if err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Errorf("%s: CountVideos = %d, want %d", audience, count, want)
		}
	}

	page, err := db.Library().ListVideos(ctx, owner, domain.VideoQuery{Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	for _, video := range page.Items {
		if want := video.Title == "a" || video.Title == "c" || video.Title == "f"; video.Public != want {
			t.Errorf("%s: Public = %v, want %v", video.Title, video.Public, want)
		}
	}
}

// ゲストが使える並べ替えのすべてで、ページをまたいで公開の動画だけが1度ずつ現れる。
func TestListVideosGuestPagesEverySort(t *testing.T) {
	db, ids := visibilityFixture(t)
	public := []int64{ids["/media/pub/a.mp4"], ids["/media/pub/deep/c.mp4"], ids["/media/f.mp4"]}
	slices.Sort(public)

	sorts := slices.Collect(maps.Keys(listOrders))
	slices.Sort(sorts)
	for _, sort := range sorts {
		q := domain.VideoQuery{Sort: sort, Seed: 7, Limit: 1}
		if guest.CheckVideoQuery(q) != nil {
			continue
		}
		var got []int64
		for range 10 {
			page, err := db.Library().ListVideos(context.Background(), guest, q)
			if err != nil {
				t.Fatalf("%s: %v", sort, err)
			}
			if page.Total != len(public) {
				t.Errorf("%s: total = %d, want %d", sort, page.Total, len(public))
			}
			for _, item := range page.Items {
				got = append(got, item.ID)
			}
			if page.NextCursor == "" {
				break
			}
			q.Cursor = page.NextCursor
		}
		slices.Sort(got)
		if !slices.Equal(got, public) {
			t.Errorf("%s: ids = %v, want %v", sort, got, public)
		}
	}
}

// フォルダの一覧と件数、フォルダの動画には、ゲストには公開の動画から導いたものだけが
// 現れる。非公開の動画だけを含むフォルダはゲストに現れない。
func TestFoldersByAudience(t *testing.T) {
	db, _ := visibilityFixture(t)
	ctx := context.Background()
	root := domain.MediaFolder{ID: 1, Path: "/media"}

	summarize := func(audience domain.Audience, rel string) domain.FolderListing {
		t.Helper()
		locations, err := db.Library().FolderLocations(ctx, audience, domain.FolderDir(root.Path, rel))
		if err != nil {
			t.Fatal(err)
		}
		listing, _ := domain.SummarizeFolder(root, rel, locations)
		return listing
	}
	folderCounts := func(listing domain.FolderListing) map[string]int {
		out := map[string]int{}
		for _, folder := range listing.Folders {
			out[folder.Name] = folder.VideoCount
		}
		return out
	}

	// VideoCount は直下の動画の件数である。
	guestRoot, ownerRoot := summarize(guest, ""), summarize(owner, "")
	if got, want := folderCounts(guestRoot), map[string]int{"pub": 1}; !maps.Equal(got, want) {
		t.Errorf("ゲストのフォルダ = %v, want %v", got, want)
	}
	if guestRoot.Folder.VideoCount != 1 || guestRoot.Folder.FolderCount != 1 {
		t.Errorf("ゲストの登録フォルダ = %+v, want 動画1・フォルダ1", guestRoot.Folder)
	}
	if got, want := folderCounts(ownerRoot), map[string]int{"pub": 2, "private": 1}; !maps.Equal(got, want) {
		t.Errorf("所有者のフォルダ = %v, want %v", got, want)
	}
	if ownerRoot.Folder.VideoCount != 2 || ownerRoot.Folder.FolderCount != 2 {
		t.Errorf("所有者の登録フォルダ = %+v, want 動画2・フォルダ2", ownerRoot.Folder)
	}
	if got, want := folderCounts(summarize(guest, "pub")), map[string]int{"deep": 1}; !maps.Equal(got, want) {
		t.Errorf("ゲストの pub の下 = %v, want %v", got, want)
	}

	for _, tc := range []struct {
		audience domain.Audience
		dir      string
		want     bool
	}{
		{guest, "/media/private", false},
		{owner, "/media/private", true},
		{guest, "/media/pub/deep", true},
	} {
		found, err := db.Library().HasFolderLocations(ctx, tc.audience, tc.dir)
		if err != nil {
			t.Fatal(err)
		}
		if found != tc.want {
			t.Errorf("%s %s: HasFolderLocations = %v, want %v", tc.audience, tc.dir, found, tc.want)
		}
	}

	for _, tc := range []struct {
		audience domain.Audience
		scope    domain.FolderScope
		want     []string
	}{
		{guest, domain.FolderScopeDirect, []string{"a"}},
		{guest, domain.FolderScopeSubtree, []string{"a", "c"}},
		{owner, domain.FolderScopeDirect, []string{"a", "b"}},
		{owner, domain.FolderScopeSubtree, []string{"a", "b", "c", "d"}},
	} {
		page, err := db.Library().ListFolderVideos(ctx, tc.audience,
			domain.FolderVideoQuery{Dir: "/media/pub", Scope: tc.scope, Limit: domain.MaxLimit})
		if err != nil {
			t.Fatal(err)
		}
		titles := titlesOf(page)
		slices.Sort(titles)
		if !slices.Equal(titles, tc.want) || page.Total != len(tc.want) {
			t.Errorf("%s %s: titles = %v total = %d, want %v", tc.audience, tc.scope, titles, page.Total, tc.want)
		}
	}
}

// 関連動画の読み出しと1本の読み出しは、ゲストには公開の動画だけを返す。
func TestRelatedAndGetVideoByAudience(t *testing.T) {
	db, ids := visibilityFixture(t)
	ctx := context.Background()
	lib := db.Library()

	siblings, err := lib.DirectVideoPaths(ctx, guest, "/media/pub")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := siblingIDs(siblings), []int64{ids["/media/pub/a.mp4"]}; !slices.Equal(got, want) {
		t.Errorf("ゲストの同じフォルダ = %v, want %v", got, want)
	}
	siblings, err = lib.DirectVideoPaths(ctx, owner, "/media/pub")
	if err != nil {
		t.Fatal(err)
	}
	if len(siblings) != 2 {
		t.Errorf("所有者の同じフォルダ = %+v, want 2件", siblings)
	}

	self, err := lib.GetVideo(ctx, owner, ids["/media/f.mp4"])
	if err != nil {
		t.Fatal(err)
	}
	for audience, want := range map[domain.Audience][]int64{
		guest: {ids["/media/pub/a.mp4"], ids["/media/pub/deep/c.mp4"]},
		owner: {ids["/media/pub/a.mp4"], ids["/media/pub/b.mp4"], ids["/media/pub/deep/c.mp4"],
			ids["/media/pub/deep/d.mp4"], ids["/media/private/e.mp4"], ids["/media/g.mp4"]},
	} {
		neighbors, err := lib.VideosAddedNear(ctx, audience, self.ID, self.AddedAt, 10)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]int64, 0, len(neighbors))
		for _, neighbor := range neighbors {
			got = append(got, neighbor.VideoID)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: VideosAddedNear = %v, want %v", audience, got, want)
		}
	}

	all := slices.Collect(maps.Values(ids))
	slices.Sort(all)
	for audience, want := range map[domain.Audience]int{guest: 3, owner: 7} {
		videos, err := lib.VideosByIDs(ctx, audience, all)
		if err != nil {
			t.Fatal(err)
		}
		if len(videos) != want {
			t.Errorf("%s: VideosByIDs = %d件, want %d", audience, len(videos), want)
		}
		for _, video := range videos {
			if audience == guest && !video.Public {
				t.Errorf("ゲストに非公開の動画 %s が返った", video.Title)
			}
		}
	}

	video, err := lib.GetVideo(ctx, guest, ids["/media/pub/a.mp4"])
	if err != nil {
		t.Fatal(err)
	}
	if !video.Public || video.Path != "/media/pub/a.mp4" {
		t.Errorf("ゲストの GetVideo = %+v", video)
	}
	if _, err := lib.GetVideo(ctx, guest, ids["/media/pub/b.mp4"]); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ゲストの非公開の GetVideo: err = %v, want ErrNotFound", err)
	}
	video, err = lib.GetVideo(ctx, owner, ids["/media/pub/b.mp4"])
	if err != nil {
		t.Fatal(err)
	}
	if video.Public {
		t.Error("非公開の動画の Public が true")
	}
}

// ゲストの検索はタグの名前とシノニムに照合しない。所有者は今どおり照合する。
func TestGuestSearchIgnoresTagNames(t *testing.T) {
	db, _ := visibilityFixture(t)
	ctx := context.Background()
	tag, err := db.Tags().CreateTag(ctx, "花火大会")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "夏祭り", nil); err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-a", tag.ID)

	for _, query := range []string{"花火大会", "夏祭り", "花火"} {
		if titles, total := listTitles(t, db, guest, domain.VideoQuery{Query: query}); len(titles) != 0 || total != 0 {
			t.Errorf("ゲストの検索 %q = %v (total %d), want 0件", query, titles, total)
		}
		if titles, _ := listTitles(t, db, owner, domain.VideoQuery{Query: query}); !slices.Equal(titles, []string{"a"}) {
			t.Errorf("所有者の検索 %q = %v, want [a]", query, titles)
		}
	}
	// 除外語もタグの名前を見ない。
	if titles, _ := listTitles(t, db, guest, domain.VideoQuery{Query: "-花火大会"}); !slices.Equal(titles, []string{"a", "c", "f"}) {
		t.Errorf("ゲストの除外 = %v, want [a c f]", titles)
	}
}

// 公開は content_key に結ぶので、所在を移しても、同じ内容の別の所在を足しても
// 公開のままで、最後の所在が消えるとゲストに現れない。
func TestPublicFollowsContentKey(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile("/media/old/movie.mp4", "movie", "key-m", 1))
	id := ids["/media/old/movie.mp4"]
	setPublic(t, db, true, id)

	// 所在を移す: 新しい所在を足して、古い所在を消す。
	moved := upsertAll(t, db, listingFile("/media/new/movie.mp4", "movie", "key-m", 1))
	if moved["/media/new/movie.mp4"] != id {
		t.Fatalf("移した所在が同じ動画にならない: %d != %d", moved["/media/new/movie.mp4"], id)
	}
	removeLocation(t, db, id, "/media/old/movie.mp4")
	video, err := db.Library().GetVideo(ctx, guest, id)
	if err != nil {
		t.Fatalf("移した後にゲストが読めない: %v", err)
	}
	if !video.Public || video.Path != "/media/new/movie.mp4" {
		t.Errorf("移した後 = %+v", video)
	}

	// 同じ内容の別の所在を足しても公開のまま、1本として数える。
	upsertAll(t, db, listingFile("/media/copy/movie.mp4", "movie", "key-m", 1))
	if titles, total := listTitles(t, db, guest, domain.VideoQuery{}); !slices.Equal(titles, []string{"movie"}) || total != 1 {
		t.Errorf("別の所在を足した後 = %v (total %d)", titles, total)
	}

	// 最後の所在が消えると、ゲストに現れない。
	removeLocation(t, db, id, "/media/copy/movie.mp4")
	removeLocation(t, db, id, "/media/new/movie.mp4")
	if titles, total := listTitles(t, db, guest, domain.VideoQuery{}); len(titles) != 0 || total != 0 {
		t.Errorf("最後の所在が消えた後 = %v (total %d)", titles, total)
	}
	if _, err := db.Library().GetVideo(ctx, guest, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("最後の所在が消えた後の GetVideo: err = %v", err)
	}

	// 同じ内容を取り込み直すと、公開のまま戻る（公開フラグは失われない）。
	back := upsertAll(t, db, listingFile("/media/again/movie.mp4", "movie", "key-m", 1))
	video, err = db.Library().GetVideo(ctx, guest, back["/media/again/movie.mp4"])
	if err != nil {
		t.Fatalf("取り込み直した後にゲストが読めない: %v", err)
	}
	if !video.Public {
		t.Error("取り込み直した後の Public が false")
	}
}

// removeLocation は動画の所在を1つ消す。最後の所在なら動画も消える。
func removeLocation(t *testing.T, db *DB, videoID int64, path string) {
	t.Helper()
	locations, err := db.Library().VideoLocations(context.Background(), videoID)
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range locations {
		if location.Path == path {
			if err := db.ScanIndex().DeleteVideoLocations(context.Background(), []int64{location.ID}); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("所在 %s が無い", path)
}

// 空の content_key の動画は、public_videos に空の行があってもゲストに現れず、
// 切り替えでも数えない。
func TestEmptyContentKeyIsNeverPublic(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile("/media/legacy.mp4", "legacy", "key-l", 1))
	id := ids["/media/legacy.mp4"]
	if _, err := db.sql.Exec(`update videos set content_key = '' where id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if applied := setPublic(t, db, true, id); applied != 0 {
		t.Errorf("空の content_key を数えた: applied = %d", applied)
	}
	if _, err := db.sql.Exec(`insert into public_videos (content_key, published_at) values ('', 1)`); err != nil {
		t.Fatal(err)
	}

	if titles, total := listTitles(t, db, guest, domain.VideoQuery{}); len(titles) != 0 || total != 0 {
		t.Errorf("ゲストの一覧 = %v (total %d), want 0件", titles, total)
	}
	if _, err := db.Library().GetVideo(ctx, guest, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ゲストの GetVideo: err = %v, want ErrNotFound", err)
	}
	if found, err := db.Library().HasFolderLocations(ctx, guest, "/media"); err != nil || found {
		t.Errorf("ゲストの HasFolderLocations = %v, %v", found, err)
	}
	video, err := db.Library().GetVideo(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if video.Public {
		t.Error("空の content_key の動画の Public が true")
	}
}

// 切り替えはライブラリに無い id を数えず、重複した id は1つとして扱い、既に同じ
// 状態の動画も誤りにしない。
func TestSetVideosPublicCountsLibraryVideos(t *testing.T) {
	db, ids := visibilityFixture(t)
	a, b := ids["/media/pub/a.mp4"], ids["/media/pub/b.mp4"]

	if applied := setPublic(t, db, true, a, b, b, 9999); applied != 2 {
		t.Errorf("公開: applied = %d, want 2", applied)
	}
	if titles, _ := listTitles(t, db, guest, domain.VideoQuery{}); !slices.Equal(titles, []string{"a", "b", "c", "f"}) {
		t.Errorf("公開の後 = %v", titles)
	}
	if applied := setPublic(t, db, false, b, 9999); applied != 1 {
		t.Errorf("非公開: applied = %d, want 1", applied)
	}
	if applied := setPublic(t, db, false, b); applied != 1 {
		t.Errorf("もう一度非公開: applied = %d, want 1", applied)
	}
	if applied := setPublic(t, db, true); applied != 0 {
		t.Errorf("空: applied = %d, want 0", applied)
	}
	if titles, _ := listTitles(t, db, guest, domain.VideoQuery{}); !slices.Equal(titles, []string{"a", "c", "f"}) {
		t.Errorf("非公開の後 = %v", titles)
	}

	// 公開の時刻は、最初に公開したときのまま変えない。
	var publishedAt int64
	if _, err := db.sql.Exec(`update public_videos set published_at = 1 where content_key = 'key-a'`); err != nil {
		t.Fatal(err)
	}
	setPublic(t, db, true, a)
	if err := db.sql.QueryRow(`select published_at from public_videos where content_key = 'key-a'`).Scan(&publishedAt); err != nil {
		t.Fatal(err)
	}
	if publishedAt != 1 {
		t.Errorf("published_at = %d, want 1", publishedAt)
	}
}

// 途中で失敗したら、1つも反映しない。
func TestSetVideosPublicIsAllOrNothing(t *testing.T) {
	db, ids := visibilityFixture(t)
	if _, err := db.sql.Exec(`create trigger fail_public before insert on public_videos
		when new.content_key = 'key-d' begin select raise(abort, 'boom'); end`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Visibility().SetVideosPublic(context.Background(),
		[]int64{ids["/media/pub/b.mp4"], ids["/media/pub/deep/d.mp4"], ids["/media/g.mp4"]}, true)
	if err == nil {
		t.Fatal("失敗が返らない")
	}
	if titles, _ := listTitles(t, db, guest, domain.VideoQuery{}); !slices.Equal(titles, []string{"a", "c", "f"}) {
		t.Errorf("失敗の後 = %v, want 変化なし", titles)
	}

	if _, err := db.sql.Exec(`drop trigger fail_public; create trigger fail_private before delete on public_videos
		when old.content_key = 'key-f' begin select raise(abort, 'boom'); end`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Visibility().SetVideosPublic(context.Background(),
		[]int64{ids["/media/pub/a.mp4"], ids["/media/f.mp4"]}, false)
	if err == nil {
		t.Fatal("失敗が返らない")
	}
	if titles, _ := listTitles(t, db, guest, domain.VideoQuery{}); !slices.Equal(titles, []string{"a", "c", "f"}) {
		t.Errorf("失敗の後 = %v, want 変化なし", titles)
	}
}

// 公開フラグを読めないときは、公開とみなさずに誤りを返す。
func TestGuestReadsFailWhenVisibilityCannotBeRead(t *testing.T) {
	db, ids := visibilityFixture(t)
	ctx := context.Background()
	if _, err := db.sql.Exec(`drop table public_videos`); err != nil {
		t.Fatal(err)
	}
	lib := db.Library()
	if _, err := lib.ListVideos(ctx, guest, domain.VideoQuery{}); err == nil {
		t.Error("ListVideos が成功した")
	}
	if _, err := lib.ListFolderVideos(ctx, guest, domain.FolderVideoQuery{Dir: "/media/pub"}); err == nil {
		t.Error("ListFolderVideos が成功した")
	}
	if _, err := lib.GetVideo(ctx, guest, ids["/media/pub/a.mp4"]); err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetVideo: err = %v, want ErrNotFound 以外の誤り", err)
	}
	if _, err := lib.FolderLocations(ctx, guest, "/media"); err == nil {
		t.Error("FolderLocations が成功した")
	}
	if _, err := lib.HasFolderLocations(ctx, guest, "/media"); err == nil {
		t.Error("HasFolderLocations が成功した")
	}
	if _, err := lib.DirectVideoPaths(ctx, guest, "/media/pub"); err == nil {
		t.Error("DirectVideoPaths が成功した")
	}
	if _, err := lib.VideosAddedNear(ctx, guest, ids["/media/f.mp4"], fixedTime, 5); err == nil {
		t.Error("VideosAddedNear が成功した")
	}
	if _, err := lib.VideosByIDs(ctx, guest, []int64{ids["/media/pub/a.mp4"]}); err == nil {
		t.Error("VideosByIDs が成功した")
	}
}

// 00010 の Up は public_videos を作り、Down は落とす。
func TestPublicVideosMigrationDown(t *testing.T) {
	db := migratedDB(t)
	tableCount := func() int {
		t.Helper()
		var count int
		if err := db.sql.QueryRow(`select count(*) from sqlite_master where type = 'table' and name = 'public_videos'`).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if tableCount() != 1 {
		t.Fatal("public_videos が無い")
	}
	downTo(t, db, 9)
	if tableCount() != 0 {
		t.Error("Down の後に public_videos が残っている")
	}
}
