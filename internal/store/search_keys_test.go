package store

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// locationKeys は path の所在の search_key・title_key・search_version を返す。
func locationKeys(t *testing.T, db *DB, path string) (searchKey, titleKey string, version int) {
	t.Helper()
	if err := db.sql.QueryRow(
		`select search_key, title_key, search_version from video_locations where path = ?`, path,
	).Scan(&searchKey, &titleKey, &version); err != nil {
		t.Fatalf("所在 %s の鍵を読めない: %v", path, err)
	}
	return searchKey, titleKey, version
}

// 00006 の状態で所在を入れた DB を移行して開くと、再取り込みなしで全所在の鍵が
// 埋まる（親 Issue の受け入れ条件 8）。
func TestRefreshSearchKeysFillsExistingLibraryAfterMigration(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values ('/media', 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	// バッチの境目を跨ぐよう、searchKeyBatchSize より多く入れる。
	const extra = searchKeyBatchSize + 1
	locations := []struct{ path, title string }{
		{"/media/旅/ＡＢＣ１２３.mp4", "ＡＢＣ１２３"},
		{"/other/outside.mp4", "outside"},
	}
	for i := range extra {
		locations = append(locations, struct{ path, title string }{
			fmt.Sprintf("/media/bulk/clip%d.mp4", i), fmt.Sprintf("clip%d", i),
		})
	}
	for i, location := range locations {
		res, err := db.sql.Exec(`insert into videos(content_key, container) values (?, 'mp4')`, fmt.Sprintf("key-%d", i))
		if err != nil {
			t.Fatal(err)
		}
		videoID, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.sql.Exec(`insert into video_locations
			(video_id, path, title, size_bytes, mtime, created_at, updated_at)
			values (?, ?, ?, 1, 1, 1, 1)`, videoID, location.path, location.title); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if key, _, version := locationKeys(t, db, "/media/旅/ＡＢＣ１２３.mp4"); key != "" || version != 0 {
		t.Fatalf("移行直後の鍵 = %q (版 %d), want 空（版 0）", key, version)
	}

	refreshed, err := db.Library().RefreshSearchKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed != len(locations) {
		t.Errorf("作り直した件数 = %d, want %d", refreshed, len(locations))
	}
	var stale int
	if err := db.sql.QueryRow(`select count(*) from video_locations where search_version < ?`, domain.SearchKeyVersion).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Errorf("古い版のまま残った所在 = %d, want 0", stale)
	}

	key, titleKey, _ := locationKeys(t, db, "/media/旅/ＡＢＣ１２３.mp4")
	if want := "abc123\n旅/abc123.mp4"; key != want {
		t.Errorf("search_key = %q, want %q", key, want)
	}
	if want := domain.NaturalSortKey("ＡＢＣ１２３"); titleKey != want {
		t.Errorf("title_key = %q, want %q", titleKey, want)
	}
	if key, titleKey, version := locationKeys(t, db, "/other/outside.mp4"); key != "" || titleKey != domain.NaturalSortKey("outside") || version != domain.SearchKeyVersion {
		t.Errorf("登録フォルダ外の鍵 = %q・%q（版 %d）, want 空・題名の鍵（現在の版）", key, titleKey, version)
	}

	// 埋め直した鍵が索引に届いている。
	if got := searchTitles(t, db, "abc123"); len(got) != 1 || got[0] != "ＡＢＣ１２３" {
		t.Errorf("検索 abc123 = %v, want [ＡＢＣ１２３]", got)
	}

	// 版は行ごとなので、2度目は何も作り直さない。途中で止まった分だけが残る。
	if refreshed, err := db.Library().RefreshSearchKeys(ctx); err != nil || refreshed != 0 {
		t.Errorf("2度目の埋め直し = %d (err=%v), want 0", refreshed, err)
	}
	if _, err := db.sql.Exec(`update video_locations set search_version = 0 where path like '/media/bulk/%' and id % 2 = 0`); err != nil {
		t.Fatal(err)
	}
	var staleRows int
	if err := db.sql.QueryRow(`select count(*) from video_locations where search_version < ?`, domain.SearchKeyVersion).Scan(&staleRows); err != nil {
		t.Fatal(err)
	}
	if staleRows == 0 {
		t.Fatal("版の古い所在を作れていない")
	}
	if refreshed, err := db.Library().RefreshSearchKeys(ctx); err != nil || refreshed != staleRows {
		t.Errorf("続きからの埋め直し = %d (err=%v), want %d", refreshed, err, staleRows)
	}
}

// 登録フォルダ自身のパスは search_key に入らない（要件 8）。
func TestSearchKeyExcludesRegisteredFolderPath(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/旅行/movie.mp4", "movie", "key-1", 1, 0)); err != nil {
		t.Fatal(err)
	}

	key, _, version := locationKeys(t, db, "/media/旅行/movie.mp4")
	if want := "movie\n旅行/movie.mp4"; key != want {
		t.Errorf("search_key = %q, want %q", key, want)
	}
	if version != domain.SearchKeyVersion {
		t.Errorf("search_version = %d, want %d", version, domain.SearchKeyVersion)
	}
	for _, query := range []string{"media", "/media", "dia"} {
		if got := searchTitles(t, db, query); len(got) != 0 {
			t.Errorf("検索 %q = %v, want 登録フォルダのパスには当たらない", query, got)
		}
	}
	// 登録フォルダより下のディレクトリ名と拡張子には当たる。
	for _, query := range []string{"旅行", "movie.mp4", ".mp4"} {
		if got := searchTitles(t, db, query); len(got) != 1 {
			t.Errorf("検索 %q = %v, want [movie]", query, got)
		}
	}
}

// 取り込みで題名が変わると、鍵も作り直される。
func TestUpsertVideoRefreshesSearchKey(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mp4", "古い題名", "key-1", 1, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mp4", "新しい題名", "key-1", 2, 0)); err != nil {
		t.Fatal(err)
	}
	key, titleKey, _ := locationKeys(t, db, "/media/a.mp4")
	if want := domain.FoldForMatch("新しい題名") + "\na.mp4"; key != want {
		t.Errorf("search_key = %q, want %q", key, want)
	}
	if titleKey != domain.NaturalSortKey("新しい題名") {
		t.Errorf("title_key = %q", titleKey)
	}
	if got := searchTitles(t, db, "古い題名"); len(got) != 0 {
		t.Errorf("古い題名で %v が当たった。索引に古い鍵が残っている", got)
	}
}

// メディアフォルダを追加・差し替えると、新しい登録の下の所在の鍵が、新しい
// 相対パスで作り直される。
func TestMediaFolderChangesRebuildSearchKeys(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	addedRoot := filepath.Join(root, "added")
	wideRoot := filepath.Join(root, "wide")
	for _, path := range []string{oldRoot, addedRoot, wideRoot} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldFolder, err := db.Settings().AddMediaFolder(ctx, oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	inOld := filepath.Join(oldRoot, "clip.mp4")
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(inOld, "clip", "key-old", 1, 0)); err != nil {
		t.Fatal(err)
	}
	if key, _, _ := locationKeys(t, db, inOld); key != "clip\nclip.mp4" {
		t.Errorf("登録フォルダ直下の search_key = %q", key)
	}

	// 登録の無いまま残った所在。鍵は空になる。
	inAdded := filepath.Join(addedRoot, "season", "episode.mp4")
	inWide := filepath.Join(wideRoot, "deep", "season", "finale.mp4")
	for i, path := range []string{inAdded, inWide} {
		if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(path, strings.TrimSuffix(filepath.Base(path), ".mp4"), fmt.Sprintf("key-%d", i), 1, 0)); err != nil {
			t.Fatal(err)
		}
		if key, _, _ := locationKeys(t, db, path); key != "" {
			t.Fatalf("登録フォルダ外の search_key = %q, want 空", key)
		}
	}

	// 追加: 新しい登録の下の所在が、登録からの相対パスで作り直される。
	if _, err := db.Settings().AddMediaFolder(ctx, addedRoot); err != nil {
		t.Fatal(err)
	}
	if key, _, _ := locationKeys(t, db, inAdded); key != "episode\nseason/episode.mp4" {
		t.Errorf("追加後の search_key = %q", key)
	}
	if key, _, _ := locationKeys(t, db, inWide); key != "" {
		t.Errorf("追加した登録の外の search_key = %q, want 空のまま", key)
	}

	// 差し替え: 新しい登録の下の所在が、新しい登録からの相対パスで作り直される。
	if _, err := db.Settings().ReplaceMediaFolder(ctx, oldFolder.ID, oldFolder.Version, wideRoot); err != nil {
		t.Fatal(err)
	}
	key, _, _ := locationKeys(t, db, inWide)
	if want := "finale\ndeep/season/finale.mp4"; key != want {
		t.Errorf("差し替え後の search_key = %q, want %q", key, want)
	}
	if strings.Contains(key, filepath.ToSlash(root)) {
		t.Errorf("search_key に登録フォルダのパスが入った: %q", key)
	}
	if got := searchTitles(t, db, "deep/season"); len(got) != 1 || got[0] != "finale" {
		t.Errorf("検索 deep/season = %v, want [finale]", got)
	}
}

// 検索語と search_key に同じ照合形を掛けるので、全角半角・大文字小文字・かなの
// 違いを問わず当たる。
func TestSearchFoldsQueryAndKey(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	for i, title := range []string{"ＡＢＣ１２３", "たびにっき", "100% 満足", "100 点"} {
		if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/"+title+".mp4", title, fmt.Sprintf("key-%d", i), int64(i+1), 0)); err != nil {
			t.Fatal(err)
		}
	}

	for query, want := range map[string]string{
		"abc123": "ＡＢＣ１２３",
		"ABC":    "ＡＢＣ１２３",
		"タビ":     "たびにっき", // 2文字は instr 経路
		"タビニッキ":  "たびにっき", // 3文字以上は MATCH 経路
		"ﾀﾋﾞ":    "たびにっき", // 半角カナも NFKC で畳まれる
		"100%":   "100% 満足",
		"0%":     "100% 満足", // % は字面として扱う
	} {
		got := searchTitles(t, db, query)
		if len(got) != 1 || got[0] != want {
			t.Errorf("検索 %q = %v, want [%s]", query, got, want)
		}
	}
	if got := searchTitles(t, db, "_"); len(got) != 0 {
		t.Errorf("検索 _ = %v, want 0件（_ は字面として扱う）", got)
	}
}

// Down で videos_fts と3つのトリガが戻り、足した列と索引が消える。
func TestLocationSearchMigrationDownRestoresVideosFTS(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/夏休みの旅行.mp4", "夏休みの旅行", "key-1", 1, 0)); err != nil {
		t.Fatal(err)
	}

	// 00008（タグ）を先に戻してから、検査対象の00007を戻す。
	if err := Down(ctx, db); err != nil {
		t.Fatalf("tags Down に失敗した: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down に失敗した: %v", err)
	}

	for name, want := range map[string]int{
		"videos_fts": 1, "video_locations_ai": 1, "video_locations_ad": 1, "video_locations_au": 1,
		"location_search_fts": 0, "location_search_fts_ai": 0, "location_search_fts_ad": 0, "location_search_fts_au": 0,
	} {
		var count int
		if err := db.sql.QueryRow(`select count(*) from sqlite_master where name = ?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Errorf("%s の数 = %d, want %d", name, count, want)
		}
	}
	columns := tableColumns(t, db, "video_locations")
	for _, name := range []string{"search_key", "title_key", "search_version"} {
		if _, ok := columns[name]; ok {
			t.Errorf("video_locations に %s 列が残っている", name)
		}
	}

	// rebuild で既存の所在が索引に戻り、トリガが以後の書き込みを写す。
	var count int
	if err := db.sql.QueryRow(`select count(*) from videos_fts where videos_fts match '夏休み'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Down 後の MATCH '夏休み' = %d, want 1", count)
	}
	if _, err := db.sql.Exec(`update video_locations set title = '花火大会'`); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow(`select count(*) from videos_fts where videos_fts match '花火大'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Down 後の更新が videos_fts に届かない: %d 件", count)
	}

	// もう一度上げても壊れない。
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatalf("Down の後の再適用に失敗した: %v", err)
	}
	if _, err := db.Library().RefreshSearchKeys(ctx); err != nil {
		t.Fatal(err)
	}
	if got := searchTitles(t, db, "花火大会"); len(got) != 1 {
		t.Errorf("再適用後の検索 = %v, want [花火大会]", got)
	}
}

// 一覧の登録判定と鍵の登録判定は同じ規則に従う。`\` を区切りとして扱うのは
// Windows だけで、ほかの OS では `\` はファイル名の一部である。
func TestSearchKeyFollowsListRegistrationRule(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("`\\` is a path separator on this OS")
	}
	db := migratedDB(t)
	ctx := context.Background()
	// 登録フォルダ /media の直後の `\` は区切りではないので、この所在は登録の外である。
	outside := `/media\film.mp4`
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(outside, "film", "key-outside", 1, 0)); err != nil {
		t.Fatal(err)
	}
	if key, _, _ := locationKeys(t, db, outside); key != "" {
		t.Errorf("登録の外の search_key = %q, want 空", key)
	}
	if got := searchTitles(t, db, ""); len(got) != 0 {
		t.Errorf("一覧 = %v, want 登録の外の所在は出ない", got)
	}

	// 名前が `\` で終わる登録フォルダでは、`\` を削らずにその下だけを登録とみなす。
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`, `/elsewhere\`); err != nil {
		t.Fatal(err)
	}
	inside := `/elsewhere\/clip.mp4`
	sibling := `/elsewhere\clip2.mp4`
	for i, path := range []string{inside, sibling} {
		if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(path, "clip"+strconv.Itoa(i), "key-x"+strconv.Itoa(i), 1, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if key, _, _ := locationKeys(t, db, inside); key != "clip0\nclip.mp4" {
		t.Errorf("登録の下の search_key = %q, want %q", key, "clip0\nclip.mp4")
	}
	if key, _, _ := locationKeys(t, db, sibling); key != "" {
		t.Errorf("隣のフォルダの search_key = %q, want 空", key)
	}
	if got := searchTitles(t, db, "clip"); len(got) != 1 || got[0] != "clip0" {
		t.Errorf("検索 clip = %v, want [clip0]", got)
	}
}

// 改行を含む題名も、改行を空白に読み替えた語で見つかる。
func TestSearchFindsTitleContainingNewline(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mp4", "abc\ndef", "key-newline", 1, 0)); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"abc\ndef", "abc def"} {
		if got := searchTitles(t, db, query); len(got) != 1 {
			t.Errorf("検索 %q = %v, want 1件", query, got)
		}
	}
}

// 題名と相対パスの境目（改行）をまたぐ検索語には当たらない。
func TestSearchDoesNotMatchAcrossTitleAndPath(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/def.mp4", "abc", "key-boundary", 1, 0)); err != nil {
		t.Fatal(err)
	}
	// 3文字以上（MATCH）と1〜2文字（instr）の両方の経路を調べる。
	// 空白（改行を含む）は語の区切りなので、改行はフレーズの中にだけ現れる。
	for _, query := range []string{"\"bc\nde\"", "\"c\nd\""} {
		if got := searchTitles(t, db, query); len(got) != 0 {
			t.Errorf("検索 %q = %v, want 境目をまたいで当たらない", query, got)
		}
	}
}
