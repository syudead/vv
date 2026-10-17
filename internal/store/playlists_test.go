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

// プレイリスト。

func createPlaylist(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	playlist, err := db.Playlists().CreatePlaylist(context.Background(), name)
	if err != nil {
		t.Fatalf("プレイリスト %q を作れない: %v", name, err)
	}
	return playlist.ID
}

// listPlaylists は一覧の全部を、最大の件数のページで NextCursor が尽きるまで辿って読む。
func listPlaylists(ctx context.Context, db *DB) ([]domain.Playlist, error) {
	var items []domain.Playlist
	cursor := ""
	for {
		page, err := db.Playlists().ListPlaylists(ctx, cursor, domain.MaxLimit)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
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
		if item.Group != nil && item.Group.Path != "" {
			if _, ok := domain.LocateFolder(roots, item.Group.Path); !ok {
				t.Errorf("グループ %s が返した登録フォルダ %+v の下に無い", item.Group.Path, roots)
			}
		}
	}
	return playlist, items
}

// playlistEntries はプレイリストの見せる項目を、動画なら "v:<id>"、自動グループなら "g:<パス>"、
// 手で作ったグループなら "g:#<id>" で返す。
func playlistEntries(t *testing.T, db *DB, id int64) []string {
	t.Helper()
	_, items := readPlaylist(t, db, id)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Video != nil {
			out = append(out, "v:"+itoa(item.Video.ID))
		} else if item.Group.Path != "" {
			out = append(out, "g:"+item.Group.Path)
		} else {
			out = append(out, "g:#"+itoa(item.Group.ID))
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

// 動画とグループは要求の順（動画、次にグループ）で末尾に並び、ライブラリに無い動画の id と無い
// グループの id はどちらの数にも入らない。グループは所有者の値で読む。
func TestPlaylistAppendsInRequestOrder(t *testing.T) {
	db, ids := itemsFixture(t)
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	show, pair := fixturePath("/media/show"), fixturePath("/media/pair")
	id := createPlaylist(t, db, "混ぜる")
	addVideos(t, db, id, solo)

	added := addToPlaylist(t, db, id, domain.PlaylistAdd{
		VideoIDs: []int64{x, 99_999, solo, x},
		GroupIDs: []int64{groupIDOf(t, db, pair), 99_999, groupIDOf(t, db, show), groupIDOf(t, db, pair)},
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
	addToPlaylist(t, db, id, domain.PlaylistAdd{VideoIDs: []int64{solo}, GroupIDs: []int64{groupIDOf(t, db, show)}})
	_, items := readPlaylist(t, db, id)
	var members []int64
	for _, video := range items[1].Group.Members {
		members = append(members, video.ID)
	}

	if err := db.ScanIndex().DeleteVideos(ctx, members); err != nil {
		t.Fatal(err)
	}
	var groups int
	if err := db.sql.QueryRow(`select count(*) from groups where origin_folder = ?`, domain.FolderKey(show)).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if groups != 1 {
		t.Fatalf("索引の作り直し前のグループの行 = %d, want 1", groups)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(solo)) {
		t.Errorf("メンバーが消えたあと = %v", got)
	}
	playlists, err := listPlaylists(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(playlists) != 1 || playlists[0].ItemCount != 1 {
		t.Errorf("一覧 = %+v, want 見せる項目 1", playlists)
	}
}

// 動画の場所がなくなった項目は読み出しから外れ、中身を再スキャンすると元の位置で読める。グループの
// 項目は、フォルダのグループ化を解くとグループの解散で消え、自動に戻して新しいグループができても
// 戻らない（R-4、docs/design-docs/catalog-organization.md）。
func TestPlaylistHidesItemsUntilTheyResolveAgain(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	soloPath := fixturePath("/media/solo.mp4")
	solo, x := ids[soloPath], ids[fixturePath("/media/mixed/x.mp4")]
	show := fixturePath("/media/show")
	id := createPlaylist(t, db, "戻る")
	addToPlaylist(t, db, id, domain.PlaylistAdd{VideoIDs: []int64{x, solo}, GroupIDs: []int64{groupIDOf(t, db, show)}})
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
	playlists, err := listPlaylists(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(playlists) != 1 || playlists[0].ItemCount != 3 {
		t.Errorf("一覧 = %+v, want 見せる項目 3", playlists)
	}
	if err := db.FolderGroups().ClearOverride(ctx, show); err != nil {
		t.Fatal(err)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(x, back, y)) {
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
		playlists, err := listPlaylists(ctx, db)
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

// 一覧は cursor の次から limit 件を返し、続きがあるときだけ NextCursor を持つ。同じ更新日時の
// プレイリストもページの境で重ならず漏れず、Total はどのページでも全部の数で、Covers は返した
// ページの分だけ埋まる。読めない cursor は ErrInvalidCursor。
func TestListPlaylistsPaged(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	clock := time.UnixMilli(1_000_000)
	previous := playlistNow
	playlistNow = func() time.Time { return clock }
	t.Cleanup(func() { playlistNow = previous })

	// 同じ時刻に 3 つ、1 秒後に 2 つ。並びは [e d c b a]。
	a, b, c := createPlaylist(t, db, "a"), createPlaylist(t, db, "b"), createPlaylist(t, db, "c")
	clock = clock.Add(time.Second)
	d, e := createPlaylist(t, db, "d"), createPlaylist(t, db, "e")
	addVideos(t, db, e, ids["a"])

	var got []int64
	cursor := ""
	for pages := 0; ; pages++ {
		page, err := db.Playlists().ListPlaylists(ctx, cursor, 2)
		if err != nil {
			t.Fatalf("ページ %d: %v", pages, err)
		}
		if page.Total != 5 {
			t.Errorf("ページ %d の Total = %d, want 5", pages, page.Total)
		}
		for _, playlist := range page.Items {
			if playlist.Covers == nil {
				t.Errorf("プレイリスト %d の Covers が nil", playlist.ID)
			}
			got = append(got, playlist.ID)
		}
		if page.NextCursor == "" {
			if len(page.Items) != 1 {
				t.Errorf("最後のページ = %d 件, want 1", len(page.Items))
			}
			break
		}
		if len(page.Items) != 2 {
			t.Errorf("ページ %d = %d 件, want 2", pages, len(page.Items))
		}
		cursor = page.NextCursor
		if pages > 5 {
			t.Fatal("ページが終わらない")
		}
	}
	if want := []int64{e, d, c, b, a}; !slices.Equal(got, want) {
		t.Errorf("ページを結んだ並び = %v, want %v", got, want)
	}

	whole, err := db.Playlists().ListPlaylists(ctx, "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if whole.NextCursor != "" || len(whole.Items) != 5 {
		t.Errorf("ちょうど全件のページ = %d 件, NextCursor %q", len(whole.Items), whole.NextCursor)
	}
	if _, err := db.Playlists().ListPlaylists(ctx, "not-a-cursor", 2); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("読めないカーソル = %v, want ErrInvalidCursor", err)
	}
}

// 一覧の Version はどのページでも同じで、プレイリストを作る・名前か項目を変える・消すたびに
// 変わる。同じミリ秒に別のプレイリストを変えても変わる。何も変わらない操作（既にある動画を
// 足す）では変わらない。ページを読んでいる間に変わったプレイリストはカーソルの次からは返らないので、
// クライアントはこれを比べて最初のページから読み直す。
func TestListPlaylistsVersion(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	clock := time.UnixMilli(1_000_000)
	previous := playlistNow
	playlistNow = func() time.Time { return clock }
	t.Cleanup(func() { playlistNow = previous })

	version := func(cursor string) string {
		t.Helper()
		page, err := db.Playlists().ListPlaylists(ctx, cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		return page.Version
	}
	empty := version("")
	first, second := createPlaylist(t, db, "1"), createPlaylist(t, db, "2")
	created := version("")
	if created == empty {
		t.Errorf("作ったあとも版が同じ: %q", created)
	}
	firstPage, err := db.Playlists().ListPlaylists(ctx, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := version(firstPage.NextCursor); got != created {
		t.Errorf("2 ページ目の版 = %q, 1 ページ目 = %q", got, created)
	}

	// 同じミリ秒に、既に最大の更新日時を持つものとは別のプレイリストを変える。
	addVideos(t, db, first, ids["a"])
	added := version("")
	if added == created {
		t.Errorf("同じミリ秒に項目を足したあとも版が同じ: %q", added)
	}
	addVideos(t, db, first, ids["a"])
	if got := version(""); got != added {
		t.Errorf("既にある動画を足しただけで版が変わった: %q -> %q", added, got)
	}
	if _, err := db.Playlists().RenamePlaylist(ctx, second, "二"); err != nil {
		t.Fatal(err)
	}
	renamed := version("")
	if renamed == added {
		t.Errorf("改名したあとも版が同じ: %q", renamed)
	}
	if err := db.Playlists().DeletePlaylist(ctx, first); err != nil {
		t.Fatal(err)
	}
	if got := version(""); got == renamed {
		t.Errorf("消したあとも版が同じ: %q", got)
	}
}

// 同じミリ秒の変更でも、時計が戻っても、名前か項目を変えるたびに更新日時は前より進む。
// 再生の応答とプレイリストの応答は更新日時が同じときだけ同じ並びとして使われる。
func TestPlaylistUpdatedAtAdvancesOnEveryChange(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	clock := time.UnixMilli(1_000_000)
	previous := playlistNow
	playlistNow = func() time.Time { return clock }
	t.Cleanup(func() { playlistNow = previous })

	id := createPlaylist(t, db, "朝")
	addVideos(t, db, id, ids["a"], ids["b"])
	last, items := readPlaylist(t, db, id)
	advanced := func(what string) {
		t.Helper()
		now, _ := readPlaylist(t, db, id)
		if !now.UpdatedAt.After(last.UpdatedAt) {
			t.Errorf("%s: 更新日時 %v が前の %v より進まない", what, now.UpdatedAt, last.UpdatedAt)
		}
		last = now
	}
	if err := db.Playlists().MovePlaylistItem(ctx, id, items[1].ID, nil); err != nil {
		t.Fatal(err)
	}
	advanced("同じミリ秒の移動")
	if err := db.Playlists().MovePlaylistItem(ctx, id, items[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	advanced("同じミリ秒の 2 度目の移動")
	clock = clock.Add(-time.Hour)
	if _, err := db.Playlists().RenamePlaylist(ctx, id, "昼"); err != nil {
		t.Fatal(err)
	}
	advanced("時計が戻った後の改名")
	if err := db.Playlists().RemovePlaylistItem(ctx, id, items[0].ID); err != nil {
		t.Fatal(err)
	}
	advanced("時計が戻った後に外す")
}

// 束ねて項目が 1 つにまとまったプレイリストは更新日時が進み、鍵を付け替えただけのプレイリストは
// 並びが変わらないので進まない（R-2）。
func TestBundleAdvancesUpdatedAtOnlyWhereItemsMerge(t *testing.T) {
	db, ids := versionFixture(t)
	clock := time.UnixMilli(1_000_000)
	previous := playlistNow
	playlistNow = func() time.Time { return clock }
	t.Cleanup(func() { playlistNow = previous })

	merged, moved := createPlaylist(t, db, "両方"), createPlaylist(t, db, "代表以外")
	addVideos(t, db, merged, ids["a"], ids["b"])
	addVideos(t, db, moved, ids["b"])
	beforeMerged, _ := readPlaylist(t, db, merged)
	beforeMoved, _ := readPlaylist(t, db, moved)

	bundle(t, db, ids["a"], ids["a"], ids["b"])

	if after, _ := readPlaylist(t, db, merged); !after.UpdatedAt.After(beforeMerged.UpdatedAt) {
		t.Errorf("まとまったプレイリストの更新日時 = %v, want %v より後", after.UpdatedAt, beforeMerged.UpdatedAt)
	}
	if after, _ := readPlaylist(t, db, moved); !after.UpdatedAt.Equal(beforeMoved.UpdatedAt) {
		t.Errorf("付け替えただけのプレイリストの更新日時 = %v, want %v のまま", after.UpdatedAt, beforeMoved.UpdatedAt)
	}
}

// 後継で隠れていた項目が見えるようになったプレイリストは更新日時が進み、項目の鍵が変わらない
// プレイリストは進まない。
func TestSuccessionAdvancesUpdatedAtWhenAnItemReappears(t *testing.T) {
	db, a := taggedVideoFixture(t)
	clock := time.UnixMilli(1_000_000)
	previous := playlistNow
	playlistNow = func() time.Time { return clock }
	t.Cleanup(func() { playlistNow = previous })
	d := upsertOne(t, db, listingFile(fixturePath("/media/d.mp4"), "d", "key-d", 4))
	carried, untouched := createPlaylist(t, db, "前の中身"), createPlaylist(t, db, "別の動画")
	addVideos(t, db, carried, a, d)
	addVideos(t, db, untouched, d)

	scan := startTestScan(t, db)
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	finishTestScan(t, db, scan, domain.ScanDone)
	if got := playlistEntries(t, db, carried); !slices.Equal(got, videoEntries(d)) {
		t.Fatalf("後継の前 = %v, want 前の中身の項目は隠れる", got)
	}
	beforeCarried, _ := readPlaylist(t, db, carried)
	beforeUntouched, _ := readPlaylist(t, db, untouched)

	probeDuration(t, db, b, 100_000)

	afterCarried, _ := readPlaylist(t, db, carried)
	if got := playlistEntries(t, db, carried); !slices.Equal(got, videoEntries(b, d)) {
		t.Fatalf("後継の後 = %v", got)
	}
	if !afterCarried.UpdatedAt.After(beforeCarried.UpdatedAt) {
		t.Errorf("項目が見えたプレイリストの更新日時 = %v, want %v より後", afterCarried.UpdatedAt, beforeCarried.UpdatedAt)
	}
	if after, _ := readPlaylist(t, db, untouched); !after.UpdatedAt.Equal(beforeUntouched.UpdatedAt) {
		t.Errorf("項目の変わらないプレイリストの更新日時 = %v, want %v のまま", after.UpdatedAt, beforeUntouched.UpdatedAt)
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

// 一覧の Covers は、見せる項目を位置の順にたどり、動画の項目はその動画、グループの項目は並びの順で
// 最初のサムネイル生成済みのメンバーを 1 枚ずつ、サムネイル生成済みのものだけ最大 4 件持つ。
// 見せなくなった項目は飛ばし、空のプレイリストは空の配列になる。
func TestListPlaylistsCovers(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	path := func(rel string) string { return fixturePath("/media/" + rel) }
	show := path("show")
	ep2, ep10 := ids[path("show/ep2.mp4")], ids[path("show/ep10.mp4")]
	p2, x, solo, y := ids[path("pair/p2.mp4")], ids[path("mixed/x.mp4")], ids[path("solo.mp4")], ids[path("mixed/sub/y.mp4")]
	for _, id := range []int64{ep2, ep10, x, solo, y} {
		if err := db.Ingest().SetThumbnailState(ctx, id, domain.ThumbnailStateDone); err != nil {
			t.Fatal(err)
		}
	}
	position := int64(1500)
	if _, err := db.Overrides().SetThumbnailPosition(ctx, solo, &position); err != nil {
		t.Fatal(err)
	}

	empty := createPlaylist(t, db, "空")
	id := createPlaylist(t, db, "絵柄")
	// p2 にはサムネイルが無いので飛ばす。show はサムネイル生成済みのメンバーのうち並びの先頭。
	addToPlaylist(t, db, id, domain.PlaylistAdd{VideoIDs: []int64{p2}, GroupIDs: []int64{groupIDOf(t, db, show)}})
	addVideos(t, db, id, x, solo, y)

	_, items := readPlaylist(t, db, id)
	var member int64
	for _, video := range items[1].Group.Members {
		if video.HasThumbnail() {
			member = video.ID
			break
		}
	}
	if member == 0 {
		t.Fatal("グループにサムネイル生成済みのメンバーが無い")
	}

	covers := func() map[int64][]int64 {
		t.Helper()
		playlists, err := listPlaylists(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64][]int64{}
		for _, playlist := range playlists {
			if playlist.Covers == nil {
				t.Errorf("プレイリスト %d の Covers が nil", playlist.ID)
			}
			videoIDs := []int64{}
			for _, cover := range playlist.Covers {
				videoIDs = append(videoIDs, cover.VideoID)
				if cover.ContentKey == "" {
					t.Errorf("動画 %d の content_key が空", cover.VideoID)
				}
				if cover.VideoID == solo && (cover.ThumbnailPositionMs == nil || *cover.ThumbnailPositionMs != position || cover.ThumbnailRevision == 0) {
					t.Errorf("solo の位置と改版 = %v・%d", cover.ThumbnailPositionMs, cover.ThumbnailRevision)
				}
			}
			out[playlist.ID] = videoIDs
		}
		return out
	}
	want := map[int64][]int64{empty: {}, id: {member, x, solo, y}}
	if got := covers(); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("Covers = %v, want %v", got, want)
	}

	// 動画の場所が無くなった項目は飛ばし、次の項目が繰り上がる。4 件を超える分は入らない。
	if err := db.ScanIndex().DeleteVideos(ctx, []int64{x}); err != nil {
		t.Fatal(err)
	}
	want[id] = []int64{member, solo, y}
	if got := covers(); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("x が消えたあとの Covers = %v, want %v", got, want)
	}
	addVideos(t, db, id, ep10, ep2)
	want[id] = []int64{member, solo, y, ep10}
	if got := covers(); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("5 件目を足したあとの Covers = %v, want %v", got, want)
	}

	// 一覧以外の読み出しは Covers を持たない。
	playlist, _ := readPlaylist(t, db, id)
	if playlist.Covers != nil {
		t.Errorf("Playlist の Covers = %v, want nil", playlist.Covers)
	}
}
