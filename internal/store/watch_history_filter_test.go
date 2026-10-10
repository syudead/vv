package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 視聴履歴の絞り込み・検索・日付（specs/043-watch-history/data-model.md の Rules の revision、
// research.md R-8〜R-11）。

// setProgress は鍵 key に再生位置を書く。completed なら視聴済み。
func setProgress(t *testing.T, db *DB, key string, positionMs int64, completed bool) {
	t.Helper()
	if _, err := db.Playback().SaveProgress(context.Background(), key,
		domain.Progress{PositionMs: positionMs, DurationMs: 100_000, Completed: completed}, nil); err != nil {
		t.Fatal(err)
	}
}

// fillTitleKeys は title_key の無い行（insertHistory が入れた行）に起動時の埋め直しを掛ける。
func fillTitleKeys(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.Playback().RefreshWatchHistoryTitleKeys(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// queryHistory は query を満たす件を 1 ページ（200 件まで）読み、id を新しい順に返す。
func queryHistory(t *testing.T, db *DB, query domain.WatchHistoryQuery) []int64 {
	t.Helper()
	page, err := db.Playback().ListWatchHistory(context.Background(), domain.AudienceOwner, query, "", 200)
	if err != nil {
		t.Fatal(err)
	}
	return historyIDs(page)
}

func filterQuery(filter domain.WatchHistoryFilter) domain.WatchHistoryQuery {
	return domain.WatchHistoryQuery{Filter: filter}
}

func searchQuery(text string) domain.WatchHistoryQuery {
	return domain.WatchHistoryQuery{Search: domain.ParseSearchQuery(text)}
}

// 視聴済み・途中・未視聴の動画が 1 本ずつと動画の無い件が 1 件あるとき、inProgress は途中の動画の件、
// watched は視聴済みの動画の件、all は全件を返す。同じ途中の動画の件が 2 件あれば両方出る（R-9）。
func TestListWatchHistoryFiltersByWatchState(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile(fixturePath("/media/w.mp4"), "w", "key-w", 1),
		listingFile(fixturePath("/media/i.mp4"), "i", "key-i", 2),
		listingFile(fixturePath("/media/u.mp4"), "u", "key-u", 3),
	)
	setProgress(t, db, "key-w", 100_000, true)
	setProgress(t, db, "key-i", 30_000, false)
	// 動画の無い件は再生位置があっても「すべて」にだけ出る。
	setProgress(t, db, "key-gone", 30_000, false)
	watched := insertHistory(t, db, "key-w", "w", 1000)
	inProgress := insertHistory(t, db, "key-i", "i", 2000)
	unwatched := insertHistory(t, db, "key-u", "u", 3000)
	gone := insertHistory(t, db, "key-gone", "gone", 4000)

	for filter, want := range map[domain.WatchHistoryFilter][]int64{
		domain.WatchHistoryAll:        {gone, unwatched, inProgress, watched},
		"":                            {gone, unwatched, inProgress, watched},
		domain.WatchHistoryInProgress: {inProgress},
		domain.WatchHistoryWatched:    {watched},
	} {
		if got := queryHistory(t, db, filterQuery(filter)); !slices.Equal(got, want) {
			t.Errorf("watch=%q = %v, want %v", filter, got, want)
		}
	}

	again := insertHistory(t, db, "key-i", "i", 5000)
	if got, want := queryHistory(t, db, filterQuery(domain.WatchHistoryInProgress)), []int64{again, inProgress}; !slices.Equal(got, want) {
		t.Errorf("途中の動画の件が 2 件のとき inProgress = %v, want %v", got, want)
	}
}

// 束ねた動画のメンバーの件は、そのメンバーの content_key ではなく束の共有の進捗で分類される。
func TestListWatchHistoryFilterUsesBundleProgress(t *testing.T) {
	db := migratedDB(t)
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2),
	)
	a, b := ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")]
	if _, err := db.Versions().Bundle(context.Background(), []int64{a, b}, a); err != nil {
		t.Fatal(err)
	}
	var bundleKey string
	if err := db.sql.QueryRow(`select user_key from video_bundles`).Scan(&bundleKey); err != nil {
		t.Fatal(err)
	}
	setProgress(t, db, bundleKey, 100_000, true)
	member := insertHistory(t, db, "key-b", "b", 1000)

	if got := queryHistory(t, db, filterQuery(domain.WatchHistoryWatched)); !slices.Equal(got, []int64{member}) {
		t.Errorf("watched = %v, want [%d]", got, member)
	}
	if got := queryHistory(t, db, filterQuery(domain.WatchHistoryInProgress)); len(got) != 0 {
		t.Errorf("inProgress = %v, want 空", got)
	}
}

// 検索は動画のある件をファイルの題名と表示名で、動画の無い件を題名の写しで見つけ、相対パスやタグ名に
// しか無い語、動画のある件の題名の写しにしか無い語では見つけない。除外と OR はライブラリと同じ（R-10）。
func TestListWatchHistorySearchesTitles(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/folderword/sun.mp4"), "旅行 Sunrise", "key-sun", 1),
		listingFile(fixturePath("/media/moon.mp4"), "plain", "key-moon", 2),
	)
	sun, moon := ids[fixturePath("/media/folderword/sun.mp4")], ids[fixturePath("/media/moon.mp4")]
	if _, err := db.Overrides().SetDisplayName(ctx, moon, "表示名 Moonlight"); err != nil {
		t.Fatal(err)
	}
	tag, err := db.Tags().CreateTag(ctx, "tagword")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{sun}, tag.ID); err != nil {
		t.Fatal(err)
	}
	sunEntry := insertHistory(t, db, "key-sun", "oldname", 1000)
	moonEntry := insertHistory(t, db, "key-moon", "plain", 2000)
	gone := insertHistory(t, db, "key-gone", "Snapshot ＳＴＡＲＬＩＧＨＴ", 3000)
	fillTitleKeys(t, db)

	for text, want := range map[string][]int64{
		"sunrise":              {sunEntry},
		"旅行":                   {sunEntry},
		"moonlight":            {moonEntry},
		"plain":                {moonEntry},
		"starlight":            {gone},
		"folderword":           nil,
		"sun.mp4":              nil,
		"tagword":              nil,
		"oldname":              nil,
		"-sunrise":             {gone, moonEntry},
		"sunrise OR moonlight": {moonEntry, sunEntry},
		"sunrise | starlight":  {gone, sunEntry},
		"sunrise moonlight":    nil,
		"":                     {gone, moonEntry, sunEntry},
	} {
		if got := queryHistory(t, db, searchQuery(text)); !slices.Equal(got, want) {
			t.Errorf("query=%q = %v, want %v", text, got, want)
		}
	}
}

// フレーズは改行を含む題名を、動画がライブラリにある間も離れた後も見つける（R-10）。
func TestListWatchHistoryPhraseMatchesTitleWithNewline(t *testing.T) {
	db := migratedDB(t)
	id := upsertOne(t, db, listingFile(fixturePath("/media/summer.mp4"), "Summer\nTrip", "key-summer", 1))
	savePlay(t, db, "key-summer", 1000, &domain.Play{PlaybackID: testPlaybackA, ContentKey: "key-summer", Title: "Summer\nTrip"})
	entries := queryHistory(t, db, domain.WatchHistoryQuery{})
	if len(entries) != 1 {
		t.Fatalf("履歴 = %v, want 1 件", entries)
	}

	if got := queryHistory(t, db, searchQuery(`"Summer Trip"`)); !slices.Equal(got, entries) {
		t.Errorf("ライブラリにある間 = %v, want %v", got, entries)
	}
	if err := db.ScanIndex().DeleteVideos(context.Background(), []int64{id}); err != nil {
		t.Fatal(err)
	}
	page, err := db.Playback().ListWatchHistory(context.Background(), domain.AudienceOwner, searchQuery(`"Summer Trip"`), "", 60)
	if err != nil {
		t.Fatal(err)
	}
	if got := historyIDs(page); !slices.Equal(got, entries) || page.Items[0].Video != nil {
		t.Errorf("ライブラリを離れた後 = %+v, want %v で動画なし", page.Items, entries)
	}
}

// 絞り込みと検索を合わせると、両方が通す件だけが残る。
func TestListWatchHistoryCombinesFilterAndSearch(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile(fixturePath("/media/sun.mp4"), "sunrise", "key-sun", 1),
		listingFile(fixturePath("/media/moon.mp4"), "moonlight", "key-moon", 2),
		listingFile(fixturePath("/media/star.mp4"), "starlight", "key-star", 3),
	)
	setProgress(t, db, "key-sun", 30_000, false)
	setProgress(t, db, "key-moon", 100_000, true)
	setProgress(t, db, "key-star", 30_000, false)
	sun := insertHistory(t, db, "key-sun", "sunrise", 1000)
	insertHistory(t, db, "key-moon", "moonlight", 2000)
	insertHistory(t, db, "key-star", "starlight", 3000)

	query := domain.WatchHistoryQuery{Filter: domain.WatchHistoryInProgress, Search: domain.ParseSearchQuery("sunrise OR moonlight")}
	if got := queryHistory(t, db, query); !slices.Equal(got, []int64{sun}) {
		t.Errorf("inProgress と検索 = %v, want [%d]", got, sun)
	}
}

// Before はそれより前の件だけを残し、カーソルはその中を続く。2026-09 は 9 月の件から始まって 8 月に
// 届き、2026-09-15 はその日から始まる。
func TestListWatchHistoryBefore(t *testing.T) {
	db := migratedDB(t)
	august := insertHistory(t, db, "key-a", "a", time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC).UnixMilli())
	early := insertHistory(t, db, "key-b", "b", time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC).UnixMilli())
	late := insertHistory(t, db, "key-c", "c", time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC).UnixMilli())
	insertHistory(t, db, "key-d", "d", time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC).UnixMilli())

	before := func(date string) domain.WatchHistoryQuery {
		t.Helper()
		period, err := domain.ParseWatchHistoryPeriod(date)
		if err != nil {
			t.Fatal(err)
		}
		return domain.WatchHistoryQuery{Before: period.End(time.UTC)}
	}
	if got, want := queryHistory(t, db, before("2026-09-15")), []int64{early, august}; !slices.Equal(got, want) {
		t.Errorf("2026-09-15 = %v, want %v", got, want)
	}
	query := before("2026-09")
	first, err := db.Playback().ListWatchHistory(context.Background(), domain.AudienceOwner, query, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := historyIDs(first); !slices.Equal(got, []int64{late, early}) || first.NextCursor == "" {
		t.Fatalf("1 ページ目 = %v（cursor %q）, want [%d %d]", got, first.NextCursor, late, early)
	}
	second, err := db.Playback().ListWatchHistory(context.Background(), domain.AudienceOwner, query, first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := historyIDs(second); !slices.Equal(got, []int64{august}) || second.NextCursor != "" {
		t.Errorf("2 ページ目 = %v（cursor %q）, want [%d]", got, second.NextCursor, august)
	}
}

// 日の一覧は件をその地域の暦の日に置き、新しい順に重複なく返し、絞り込みと検索に従う（R-11）。
func TestListWatchHistoryDays(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile(fixturePath("/media/sun.mp4"), "sunrise", "key-sun", 1),
		listingFile(fixturePath("/media/moon.mp4"), "moonlight", "key-moon", 2),
	)
	setProgress(t, db, "key-sun", 30_000, false)
	setProgress(t, db, "key-moon", 100_000, true)
	insertHistory(t, db, "key-sun", "sunrise", time.Date(2026, 9, 14, 23, 50, 0, 0, time.UTC).UnixMilli())
	insertHistory(t, db, "key-sun", "sunrise", time.Date(2026, 9, 15, 0, 10, 0, 0, time.UTC).UnixMilli())
	insertHistory(t, db, "key-moon", "moonlight", time.Date(2026, 9, 15, 15, 30, 0, 0, time.UTC).UnixMilli())

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		loc   *time.Location
		query domain.WatchHistoryQuery
		want  []string
	}{
		{"UTC", time.UTC, domain.WatchHistoryQuery{}, []string{"2026-09-15", "2026-09-14"}},
		{"Tokyo", tokyo, domain.WatchHistoryQuery{}, []string{"2026-09-16", "2026-09-15"}},
		{"Los Angeles", losAngeles, domain.WatchHistoryQuery{}, []string{"2026-09-15", "2026-09-14"}},
		{"Tokyo inProgress", tokyo, filterQuery(domain.WatchHistoryInProgress), []string{"2026-09-15"}},
		{"Los Angeles moonlight", losAngeles, searchQuery("moonlight"), []string{"2026-09-15"}},
		{"no match", tokyo, searchQuery("nothing"), []string{}},
		{"Before is ignored", tokyo, domain.WatchHistoryQuery{Before: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, []string{"2026-09-16", "2026-09-15"}},
	}
	for _, c := range cases {
		got, err := db.Playback().ListWatchHistoryDays(context.Background(), domain.AudienceOwner, c.query, c.loc)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, c.want) || got == nil {
			t.Errorf("%s: days = %#v, want %#v", c.name, got, c.want)
		}
	}
}

// 移行の前に書かれた行は起動時の埋め直しの後に title_key を持ち、新しい件は書いた時点で持つ。
// 埋め直しは null の行だけを変える。
func TestWatchHistoryTitleKeys(t *testing.T) {
	db, provider := migrateTo(t, 34)
	ctx := context.Background()
	execSQL(t, db, `insert into watch_history (content_key, title, played_at) values ('key-a', 'Ｓｕｍｍｅｒ' || char(10) || 'Trip', 1)`)
	execSQL(t, db, `insert into watch_history (content_key, title, played_at) values ('key-b', '', 2)`)
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	titleKeys := func() []string {
		t.Helper()
		rows, err := db.sql.Query(`select coalesce(title_key, '<null>') from watch_history order by id`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var out []string
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				t.Fatal(err)
			}
			out = append(out, key)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got, want := titleKeys(), []string{"<null>", "<null>"}; !slices.Equal(got, want) {
		t.Fatalf("移行の直後 = %v, want %v", got, want)
	}

	filled, err := db.Playback().RefreshWatchHistoryTitleKeys(ctx)
	if err != nil || filled != 2 {
		t.Fatalf("埋め直し = %d, %v, want 2", filled, err)
	}
	execSQL(t, db, `update watch_history set title_key = 'kept' where content_key = 'key-b'`)
	if filled, err := db.Playback().RefreshWatchHistoryTitleKeys(ctx); err != nil || filled != 0 {
		t.Errorf("2 回目の埋め直し = %d, %v, want 0", filled, err)
	}
	savePlay(t, db, "key-c", 1000, &domain.Play{PlaybackID: testPlaybackA, ContentKey: "key-c", Title: "Ｎｅｗ\nＴｉｔｌｅ"})
	if got, want := titleKeys(), []string{"summer trip", "kept", "new title"}; !slices.Equal(got, want) {
		t.Errorf("title_key = %v, want %v", got, want)
	}
}

// 埋め直しは searchKeyBatchSize を超える行も全部埋める。
func TestRefreshWatchHistoryTitleKeysCrossesBatches(t *testing.T) {
	db, provider := migrateTo(t, 34)
	ctx := context.Background()
	execSQL(t, db, `with recursive n(i) as (select 1 union all select i + 1 from n where i < ?)
		insert into watch_history (content_key, title, played_at) select 'key', 'Title', i from n`, searchKeyBatchSize+1)
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	filled, err := db.Playback().RefreshWatchHistoryTitleKeys(ctx)
	if err != nil || filled != searchKeyBatchSize+1 {
		t.Fatalf("埋め直し = %d, %v, want %d", filled, err, searchKeyBatchSize+1)
	}
	var missing int
	if err := db.sql.QueryRow(`select count(*) from watch_history where title_key is null or title_key <> 'title'`).Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing != 0 {
		t.Errorf("埋まっていない行 = %d", missing)
	}
}
