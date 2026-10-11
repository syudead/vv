package store

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// プレイリスト（specs/044-playlists/data-model.md「Store operations」「Succession and bundling」、
// research.md R-1〜R-5・R-11）。

func createPlaylist(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	playlist, err := db.Playlists().CreatePlaylist(context.Background(), name)
	if err != nil {
		t.Fatalf("プレイリスト %q を作れない: %v", name, err)
	}
	return playlist.ID
}

func addToPlaylist(t *testing.T, db *DB, id int64, add domain.PlaylistAdd) domain.PlaylistAdded {
	t.Helper()
	added, err := db.Playlists().AddPlaylistItems(context.Background(), id, add)
	if err != nil {
		t.Fatalf("プレイリスト %d に足せない (%+v): %v", id, add, err)
	}
	return added
}

func addVideos(t *testing.T, db *DB, id int64, videoIDs ...int64) domain.PlaylistAdded {
	t.Helper()
	return addToPlaylist(t, db, id, domain.PlaylistAdd{VideoIDs: videoIDs})
}

func readPlaylist(t *testing.T, db *DB, id int64) (domain.Playlist, []domain.PlaylistItem) {
	t.Helper()
	playlist, items, roots, err := db.Playlists().Playlist(context.Background(), id)
	if err != nil {
		t.Fatalf("プレイリスト %d を読めない: %v", id, err)
	}
	if playlist.ItemCount != len(items) {
		t.Errorf("ItemCount = %d, 見せる項目 = %d", playlist.ItemCount, len(items))
	}
	// 登録フォルダは項目と同じスナップショットから読むので、見せるグループはどれもその下にある。
	for _, item := range items {
		if item.Group != nil {
			if _, ok := domain.LocateFolder(roots, item.Group.Path); !ok {
				t.Errorf("グループ %s が返した登録フォルダ %+v の下に無い", item.Group.Path, roots)
			}
		}
	}
	return playlist, items
}

// playlistEntries はプレイリストの見せる項目を、動画なら "v:<id>"、グループなら "g:<パス>" で返す。
func playlistEntries(t *testing.T, db *DB, id int64) []string {
	t.Helper()
	_, items := readPlaylist(t, db, id)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Video != nil {
			out = append(out, "v:"+itoa(item.Video.ID))
		} else {
			out = append(out, "g:"+item.Group.Path)
		}
	}
	return out
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}

func videoEntries(ids ...int64) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, "v:"+itoa(id))
	}
	return out
}

// playlistItemIDs はプレイリストの見せる項目の id を位置の順に返す。
func playlistItemIDs(t *testing.T, db *DB, id int64) []int64 {
	t.Helper()
	_, items := readPlaylist(t, db, id)
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func playlistRowsWithKey(t *testing.T, db *DB, key string) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from playlist_items where content_key = ?`, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// 1 本の動画を 2 つのプレイリストに足すと両方で読め、同じ動画をもう一度足すと existing に
// 数えられて項目は 1 つのまま（受け入れ条件 1・2）。
func TestPlaylistAddsAVideoToTwoPlaylists(t *testing.T) {
	db, ids := itemsFixture(t)
	solo := ids[fixturePath("/media/solo.mp4")]
	first, second := createPlaylist(t, db, "朝"), createPlaylist(t, db, "夜")

	for _, id := range []int64{first, second} {
		if added := addVideos(t, db, id, solo); added != (domain.PlaylistAdded{Added: 1}) {
			t.Fatalf("added = %+v", added)
		}
		if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(solo)) {
			t.Errorf("プレイリスト %d = %v", id, got)
		}
	}

	if added := addVideos(t, db, first, solo); added != (domain.PlaylistAdded{Existing: 1}) {
		t.Errorf("もう一度足した added = %+v, want existing 1", added)
	}
	if got := playlistEntries(t, db, first); len(got) != 1 {
		t.Errorf("もう一度足したあと = %v", got)
	}
}

// 動画とグループは要求の順（動画、次にフォルダ）で末尾に並び、ライブラリに無い id とグループで
// ないフォルダはどちらの数にも入らない。グループは所有者の値で読む。
func TestPlaylistAppendsInRequestOrder(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	show, pair := fixturePath("/media/show"), fixturePath("/media/pair")
	id := createPlaylist(t, db, "混ぜる")
	addVideos(t, db, id, solo)

	added := addToPlaylist(t, db, id, domain.PlaylistAdd{
		VideoIDs:    []int64{x, 99_999, solo, x},
		FolderPaths: []string{pair, fixturePath("/media/mixed"), show, pair},
	})
	if added != (domain.PlaylistAdded{Added: 3, Existing: 1}) {
		t.Errorf("added = %+v, want added 3, existing 1", added)
	}
	want := []string{"v:" + itoa(solo), "v:" + itoa(x), "g:" + pair, "g:" + show}
	if got := playlistEntries(t, db, id); !slices.Equal(got, want) {
		t.Errorf("項目 = %v, want %v", got, want)
	}

	_, items := readPlaylist(t, db, id)
	group := items[3].Group
	if len(group.Members) != 3 || group.WatchedCount != 1 || group.OpenVideoID == 0 {
		t.Errorf("グループ = %d 本, 視聴済み %d, 開く %d", len(group.Members), group.WatchedCount, group.OpenVideoID)
	}
}

// 先頭へ、別の項目の後ろへ、末尾への移動が、その順で読み戻せる。項目かプレイリストが無ければ
// 誤りになる。
func TestPlaylistMovesItems(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	id := createPlaylist(t, db, "並べ替え")
	addVideos(t, db, id, ids["a"], ids["b"], ids["c"], ids["d"])
	items := playlistItemIDs(t, db, id)
	a, b, c, d := items[0], items[1], items[2], items[3]
	move := func(item int64, after *int64) {
		t.Helper()
		if err := db.Playlists().MovePlaylistItem(ctx, id, item, after); err != nil {
			t.Fatalf("動かせない: %v", err)
		}
	}

	move(d, nil)
	if got := playlistItemIDs(t, db, id); !slices.Equal(got, []int64{d, a, b, c}) {
		t.Errorf("先頭へ = %v", got)
	}
	move(a, &b)
	if got := playlistItemIDs(t, db, id); !slices.Equal(got, []int64{d, b, a, c}) {
		t.Errorf("後ろへ = %v", got)
	}
	move(d, &c)
	if got := playlistItemIDs(t, db, id); !slices.Equal(got, []int64{b, a, c, d}) {
		t.Errorf("末尾へ = %v", got)
	}

	if err := db.Playlists().MovePlaylistItem(ctx, id, 99_999, nil); !errors.Is(err, domain.ErrPlaylistItemNotFound) {
		t.Errorf("無い項目 = %v", err)
	}
	missing := int64(99_999)
	if err := db.Playlists().MovePlaylistItem(ctx, id, a, &missing); !errors.Is(err, domain.ErrPlaylistItemNotFound) {
		t.Errorf("無い afterItemId = %v", err)
	}
	if err := db.Playlists().MovePlaylistItem(ctx, 99_999, a, nil); !errors.Is(err, domain.ErrPlaylistNotFound) {
		t.Errorf("無いプレイリスト = %v", err)
	}
	other := createPlaylist(t, db, "別")
	if err := db.Playlists().MovePlaylistItem(ctx, other, a, nil); !errors.Is(err, domain.ErrPlaylistItemNotFound) {
		t.Errorf("別のプレイリストの項目 = %v", err)
	}
}

// 項目を外すと残りが詰め直され、お気に入りと再生位置は変わらない（受け入れ条件 4）。プレイリストを
// 消すと項目も消える。
func TestPlaylistRemovesItemsAndPlaylists(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	y := ids[fixturePath("/media/mixed/sub/y.mp4")]
	favoriteVideos(t, db, true, solo)
	id := createPlaylist(t, db, "外す")
	addVideos(t, db, id, x, solo, y)
	favorites, progress := favoriteRowCount(t, db, "video_favorites"), favoriteRowCount(t, db, "playback_progress")

	items := playlistItemIDs(t, db, id)
	if err := db.Playlists().RemovePlaylistItem(ctx, id, items[1]); err != nil {
		t.Fatal(err)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(x, y)) {
		t.Errorf("外したあと = %v", got)
	}
	if favoriteRowCount(t, db, "video_favorites") != favorites || favoriteRowCount(t, db, "playback_progress") != progress {
		t.Errorf("お気に入りか再生位置が変わった")
	}
	if !ownerVideo(t, db, solo).Favorite {
		t.Errorf("外した動画のお気に入りが外れた")
	}
	if err := db.Playlists().RemovePlaylistItem(ctx, id, items[1]); !errors.Is(err, domain.ErrPlaylistItemNotFound) {
		t.Errorf("外した項目をもう一度 = %v", err)
	}

	if err := db.Playlists().DeletePlaylist(ctx, id); err != nil {
		t.Fatal(err)
	}
	if n := favoriteRowCount(t, db, "playlist_items"); n != 0 {
		t.Errorf("消したプレイリストの項目が %d 行残った", n)
	}
	if err := db.Playlists().DeletePlaylist(ctx, id); !errors.Is(err, domain.ErrPlaylistNotFound) {
		t.Errorf("消したプレイリストをもう一度 = %v", err)
	}
	if _, _, _, err := db.Playlists().Playlist(ctx, id); !errors.Is(err, domain.ErrPlaylistNotFound) {
		t.Errorf("消したプレイリストを読む = %v", err)
	}
}

// 並べ替えて項目 id と位置の順が食い違ったあとに外しても、残りは今の並びのまま 0 から重ならずに
// 詰め直される。
func TestPlaylistRemovesAfterAMove(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	id := createPlaylist(t, db, "並べ替えて外す")
	addVideos(t, db, id, ids["a"], ids["b"], ids["c"])
	items := playlistItemIDs(t, db, id)
	a, b, c := items[0], items[1], items[2]
	if err := db.Playlists().MovePlaylistItem(ctx, id, a, &c); err != nil {
		t.Fatal(err)
	}
	if err := db.Playlists().RemovePlaylistItem(ctx, id, b); err != nil {
		t.Fatal(err)
	}
	if got := playlistItemIDs(t, db, id); !slices.Equal(got, []int64{c, a}) {
		t.Errorf("外したあと = %v, want %v", got, []int64{c, a})
	}
	rows, err := db.sql.Query(`select id, position from playlist_items where playlist_id = ? order by id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	positions := map[int64]int{}
	for rows.Next() {
		var itemID int64
		var position int
		if err := rows.Scan(&itemID, &position); err != nil {
			t.Fatal(err)
		}
		positions[itemID] = position
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := map[int64]int{c: 0, a: 1}; !maps.Equal(positions, want) {
		t.Errorf("位置 = %v, want %v", positions, want)
	}
}

// グループの全メンバーが消え、フォルダの索引を作り直すまでグループの行だけが残っても、その項目は
// 一覧の件数にも Playlist の項目にも入らない。
func TestPlaylistHidesAGroupWithoutMembers(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo, show := ids[fixturePath("/media/solo.mp4")], fixturePath("/media/show")
	id := createPlaylist(t, db, "空のグループ")
	addToPlaylist(t, db, id, domain.PlaylistAdd{VideoIDs: []int64{solo}, FolderPaths: []string{show}})
	_, items := readPlaylist(t, db, id)
	var members []int64
	for _, video := range items[1].Group.Members {
		members = append(members, video.ID)
	}

	if err := db.ScanIndex().DeleteVideos(ctx, members); err != nil {
		t.Fatal(err)
	}
	var groups int
	if err := db.sql.QueryRow(`select count(*) from folder_groups where path_key = ?`, domain.FolderKey(show)).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if groups != 1 {
		t.Fatalf("索引の作り直し前のグループの行 = %d, want 1", groups)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(solo)) {
		t.Errorf("メンバーが消えたあと = %v", got)
	}
	playlists, err := db.Playlists().ListPlaylists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(playlists) != 1 || playlists[0].ItemCount != 1 {
		t.Errorf("一覧 = %+v, want 見せる項目 1", playlists)
	}
}

// 動画の場所がなくなった項目は読み出しから外れ、中身を再スキャンすると元の位置で読める。グループの
// 項目は、フォルダのグループ化を解くと外れ、自動に戻すと戻る（R-4、Edge Case）。
func TestPlaylistHidesItemsUntilTheyResolveAgain(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	soloPath := fixturePath("/media/solo.mp4")
	solo, x := ids[soloPath], ids[fixturePath("/media/mixed/x.mp4")]
	show := fixturePath("/media/show")
	id := createPlaylist(t, db, "戻る")
	addToPlaylist(t, db, id, domain.PlaylistAdd{VideoIDs: []int64{x, solo}, FolderPaths: []string{show}})
	addToPlaylist(t, db, id, domain.PlaylistAdd{VideoIDs: []int64{ids[fixturePath("/media/mixed/sub/y.mp4")]}})
	y := ids[fixturePath("/media/mixed/sub/y.mp4")]

	if err := db.ScanIndex().DeleteVideos(ctx, []int64{solo}); err != nil {
		t.Fatal(err)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, []string{"v:" + itoa(x), "g:" + show, "v:" + itoa(y)}) {
		t.Errorf("場所が無くなったあと = %v", got)
	}
	back := upsertOne(t, db, listingFile(soloPath, "solo", "key"+soloPath, 0))
	if got := playlistEntries(t, db, id); !slices.Equal(got, []string{"v:" + itoa(x), "v:" + itoa(back), "g:" + show, "v:" + itoa(y)}) {
		t.Errorf("再スキャンのあと = %v", got)
	}

	if err := db.FolderGroups().SetOverride(ctx, show, domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(x, back, y)) {
		t.Errorf("グループ化を解いたあと = %v", got)
	}
	playlists, err := db.Playlists().ListPlaylists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(playlists) != 1 || playlists[0].ItemCount != 3 {
		t.Errorf("一覧 = %+v, want 見せる項目 3", playlists)
	}
	if err := db.FolderGroups().ClearOverride(ctx, show); err != nil {
		t.Fatal(err)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, []string{"v:" + itoa(x), "v:" + itoa(back), "g:" + show, "v:" + itoa(y)}) {
		t.Errorf("自動に戻したあと = %v", got)
	}
}

// 同じパスの後継で項目が付け替わり、両方の鍵を持つプレイリストでは前の位置の方が残る（R-2）。
func TestSuccessionCarriesPlaylistItems(t *testing.T) {
	db, a := taggedVideoFixture(t)
	d := upsertOne(t, db, listingFile(fixturePath("/media/d.mp4"), "d", "key-d", 4))
	only, both := createPlaylist(t, db, "前だけ"), createPlaylist(t, db, "両方")
	addVideos(t, db, only, a)
	addVideos(t, db, both, a, d)

	scan := startTestScan(t, db)
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	addVideos(t, db, both, b)
	finishTestScan(t, db, scan, domain.ScanDone)
	probeDuration(t, db, b, 100_000)

	if got := playlistEntries(t, db, only); !slices.Equal(got, videoEntries(b)) {
		t.Errorf("前だけのプレイリスト = %v", got)
	}
	if got := playlistEntries(t, db, both); !slices.Equal(got, videoEntries(b, d)) {
		t.Errorf("両方のプレイリスト = %v, want 前の位置に新しい中身", got)
	}
	if n := playlistRowsWithKey(t, db, "key-a"); n != 0 {
		t.Errorf("前の鍵の項目が %d 行残った", n)
	}
}

// プレイリストに入っている 2 つのメンバーを束ねると前の位置に束の項目が 1 つ残り、代表でない
// メンバーだけが入っているときはその項目が束の鍵で残る（R-2）。
func TestBundleMergesPlaylistItems(t *testing.T) {
	db, ids := versionFixture(t)
	both, other := createPlaylist(t, db, "両方"), createPlaylist(t, db, "代表以外")
	addVideos(t, db, both, ids["c"], ids["b"], ids["d"], ids["a"])
	addVideos(t, db, other, ids["b"])

	bundle(t, db, ids["a"], ids["a"], ids["b"])
	bundleKey := ownerVideo(t, db, ids["a"]).UserKey

	if got := playlistEntries(t, db, both); !slices.Equal(got, videoEntries(ids["c"], ids["a"], ids["d"])) {
		t.Errorf("両方のプレイリスト = %v, want 束（代表 a）が b の位置に", got)
	}
	if got := playlistEntries(t, db, other); !slices.Equal(got, videoEntries(ids["a"])) {
		t.Errorf("代表以外のプレイリスト = %v", got)
	}
	if n := playlistRowsWithKey(t, db, bundleKey); n != 2 {
		t.Errorf("束の鍵の項目 = %d 行, want 2", n)
	}
	for _, key := range []string{"key-a", "key-b"} {
		if n := playlistRowsWithKey(t, db, key); n != 0 {
			t.Errorf("メンバーの鍵 %s の項目が %d 行残った", key, n)
		}
	}

	// 束同士を束ねると、吸い込んだ束の鍵の項目も新しい束へ移る。
	bundle(t, db, ids["c"], ids["c"], ids["a"])
	if got := playlistEntries(t, db, both); !slices.Equal(got, videoEntries(ids["c"], ids["d"])) {
		t.Errorf("束同士を束ねたあと = %v", got)
	}
	if n := playlistRowsWithKey(t, db, bundleKey); n != 0 {
		t.Errorf("吸い込んだ束の鍵の項目が %d 行残った", n)
	}
}

// 他のメンバーが残るときにメンバーを外すと項目は束に残り、外したメンバーには項目が無い。束を
// 解消すると、項目は残ったメンバーに移る（R-2）。
func TestUnbundleKeepsPlaylistItemsWithTheBundle(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	id := createPlaylist(t, db, "束")
	bundle(t, db, ids["a"], ids["a"], ids["b"], ids["c"])
	bundleKey := ownerVideo(t, db, ids["a"]).UserKey
	addVideos(t, db, id, ids["d"], ids["c"])

	if _, err := db.Versions().Unbundle(ctx, ids["c"]); err != nil {
		t.Fatal(err)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(ids["d"], ids["a"])) {
		t.Errorf("c を外したあと = %v", got)
	}
	if playlistRowsWithKey(t, db, bundleKey) != 1 || playlistRowsWithKey(t, db, "key-c") != 0 {
		t.Errorf("項目が束に残らない")
	}

	if _, err := db.Versions().Unbundle(ctx, ids["b"]); err != nil {
		t.Fatal(err)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(ids["d"], ids["a"])) {
		t.Errorf("束を解消したあと = %v", got)
	}
	if playlistRowsWithKey(t, db, "key-a") != 1 || playlistRowsWithKey(t, db, bundleKey) != 0 {
		t.Errorf("項目が残ったメンバーに移らない")
	}
}

// トリム後 0 文字と 101 文字の名前は拒否され、日本語 100 文字は受け付け、同じ名前の 2 つの
// プレイリストを作れる（R-11）。改名も同じ規則で、無いプレイリストは誤りになる。
func TestPlaylistNames(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	for name, input := range map[string]string{"空白だけ": " 　\t", "101 文字": strings.Repeat("あ", 101)} {
		if _, err := db.Playlists().CreatePlaylist(ctx, input); !errors.Is(err, domain.ErrInvalidPlaylistName) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	long := strings.Repeat("あ", 100)
	created, err := db.Playlists().CreatePlaylist(ctx, "  "+long+"  ")
	if err != nil || created.Name != long {
		t.Fatalf("日本語 100 文字 = %+v, %v", created, err)
	}
	first, second := createPlaylist(t, db, "同じ"), createPlaylist(t, db, "同じ")
	if first == second {
		t.Errorf("同じ名前で同じ id")
	}

	renamed, err := db.Playlists().RenamePlaylist(ctx, first, " 新しい ")
	if err != nil || renamed.Name != "新しい" || renamed.ID != first {
		t.Errorf("改名 = %+v, %v", renamed, err)
	}
	if _, err := db.Playlists().RenamePlaylist(ctx, first, ""); !errors.Is(err, domain.ErrInvalidPlaylistName) {
		t.Errorf("空の名前への改名 = %v", err)
	}
	if _, err := db.Playlists().RenamePlaylist(ctx, 99_999, "x"); !errors.Is(err, domain.ErrPlaylistNotFound) {
		t.Errorf("無いプレイリストの改名 = %v", err)
	}
	if _, err := db.Playlists().AddPlaylistItems(ctx, 99_999, domain.PlaylistAdd{}); !errors.Is(err, domain.ErrPlaylistNotFound) {
		t.Errorf("無いプレイリストへの追加 = %v", err)
	}
}

// 一覧は更新日時の新しい順、同じなら id の大きい順で、名前か項目の変更で更新日時が進む。
func TestListPlaylistsByUpdatedAt(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	clock := time.UnixMilli(1_000_000)
	previous := playlistNow
	playlistNow = func() time.Time { return clock }
	t.Cleanup(func() { playlistNow = previous })

	first, second, third := createPlaylist(t, db, "1"), createPlaylist(t, db, "2"), createPlaylist(t, db, "3")
	order := func() []int64 {
		t.Helper()
		playlists, err := db.Playlists().ListPlaylists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]int64, 0, len(playlists))
		for _, playlist := range playlists {
			out = append(out, playlist.ID)
		}
		return out
	}
	if got := order(); !slices.Equal(got, []int64{third, second, first}) {
		t.Errorf("同じ時刻 = %v", got)
	}
	clock = clock.Add(time.Second)
	addVideos(t, db, first, ids["a"])
	if got := order(); !slices.Equal(got, []int64{first, third, second}) {
		t.Errorf("足したあと = %v", got)
	}
	clock = clock.Add(time.Second)
	addVideos(t, db, second, 99_999)
	if got := order(); !slices.Equal(got, []int64{first, third, second}) {
		t.Errorf("何も足さなかったのに進んだ = %v", got)
	}
	if _, err := db.Playlists().RenamePlaylist(ctx, second, "二"); err != nil {
		t.Fatal(err)
	}
	if got := order(); !slices.Equal(got, []int64{second, first, third}) {
		t.Errorf("改名したあと = %v", got)
	}
}

// 移行は空の 2 表を作り、Down は表を落とす。
func TestPlaylistsMigration(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsy)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 38); err != nil {
		t.Fatal(err)
	}
	countTables := func() int {
		t.Helper()
		var tables int
		if err := db.sql.QueryRow(`select count(*) from sqlite_master
			where type = 'table' and name in ('playlists', 'playlist_items')`).Scan(&tables); err != nil {
			t.Fatal(err)
		}
		return tables
	}
	if countTables() != 2 {
		t.Fatalf("移行のあとの表が足りない")
	}
	assertPlaylistInvariant(t, db)
	if _, err := provider.DownTo(ctx, 37); err != nil {
		t.Fatal(err)
	}
	if n := countTables(); n != 0 {
		t.Errorf("Down のあとも %d 表が残った", n)
	}
}
