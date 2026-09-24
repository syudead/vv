package store

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// allSorts は contracts/list-api.md §3 の 13 通りの並び順である。
var allSorts = []domain.VideoSort{
	domain.SortAddedAsc, domain.SortAddedDesc, domain.SortModifiedAsc, domain.SortModifiedDesc,
	domain.SortTitleAsc, domain.SortTitleDesc, domain.SortDurationAsc, domain.SortDurationDesc,
	domain.SortSizeAsc, domain.SortSizeDesc, domain.SortPlayedAsc, domain.SortPlayedDesc, domain.SortRandom,
}

// sortRow は並べ替えの検証用の動画1件である。値の同じ行を混ぜ、id での決着も
// 確かめる。nil は値が無いことを表す。
type sortRow struct {
	title    string
	added    int // 分
	mtime    int // 分
	size     int64
	duration *int64
	played   *int64 // playback_progress.updated_at（Unix 秒）
}

func ptr(v int64) *int64 { return &v }

var sortRows = []sortRow{
	{"10話", 3, 5, 300, nil, nil},
	{"2話", 1, 2, 100, ptr(5000), ptr(100)},
	{"b", 5, 2, 300, ptr(3000), nil},
	{"A", 2, 9, 200, nil, ptr(300)},
	{"1話", 4, 1, 500, ptr(5000), ptr(200)},
	{"2話", 3, 7, 100, ptr(1000), ptr(100)},
	{"ア", 0, 5, 400, nil, nil},
}

// sortFixture は sortRows を取り込み、行の順に動画の id を返す。
func sortFixture(t *testing.T) (*DB, []int64) {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	ids := make([]int64, 0, len(sortRows))
	for i, row := range sortRows {
		key := fmt.Sprintf("key-%d", i)
		got, err := db.UpsertVideo(ctx, domain.VideoFile{
			Path: fmt.Sprintf("/media/%d-%s.mp4", i, row.title), Title: row.title, ContentKey: key,
			SizeBytes: row.size, MTime: fixedTime.Add(time.Duration(row.mtime) * time.Minute),
			AddedAt: fixedTime.Add(time.Duration(row.added) * time.Minute), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, got.ID)
		if row.duration != nil {
			if err := db.ApplyProbe(ctx, got.ID, domain.Probe{DurationMs: *row.duration, VideoCodec: "h264"}, domain.Playability{Playable: true}); err != nil {
				t.Fatal(err)
			}
		}
		if row.played != nil {
			if _, err := db.SaveProgress(ctx, key, domain.Progress{PositionMs: 1000}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.SQL().Exec(`update playback_progress set updated_at = ? where content_key = ?`, *row.played, key); err != nil {
				t.Fatal(err)
			}
		}
	}
	return db, ids
}

// expectedOrder は contracts/list-api.md §3 の定義どおりに並べた id を返す。
func expectedOrder(sort domain.VideoSort, seed int64, ids []int64) []int64 {
	type entry struct {
		id     int64
		isNull bool
		num    int64
		text   string
	}
	entries := make([]entry, len(ids))
	for i, row := range sortRows {
		e := entry{id: ids[i]}
		nullable := func(v *int64) {
			if v == nil {
				e.isNull = true
			} else {
				e.num = *v
			}
		}
		switch sort {
		case domain.SortAddedAsc, domain.SortAddedDesc:
			e.num = int64(row.added)
		case domain.SortModifiedAsc, domain.SortModifiedDesc:
			e.num = int64(row.mtime)
		case domain.SortTitleAsc, domain.SortTitleDesc:
			e.text = domain.NaturalSortKey(row.title)
		case domain.SortDurationAsc, domain.SortDurationDesc:
			nullable(row.duration)
		case domain.SortSizeAsc, domain.SortSizeDesc:
			e.num = row.size
		case domain.SortPlayedAsc, domain.SortPlayedDesc:
			nullable(row.played)
		case domain.SortRandom:
			e.num = domain.ShuffleKey(seed, ids[i])
		}
		entries[i] = e
	}
	desc := sort != domain.SortRandom && sort[len(sort)-4:] == "Desc"
	slices.SortFunc(entries, func(a, b entry) int {
		// 値の無い行は向きに関係なく末尾。
		if a.isNull != b.isNull {
			if a.isNull {
				return 1
			}
			return -1
		}
		c := cmp.Or(cmp.Compare(a.num, b.num), cmp.Compare(a.text, b.text), cmp.Compare(a.id, b.id))
		if desc {
			return -c
		}
		return c
	})
	out := make([]int64, len(entries))
	for i, e := range entries {
		out[i] = e.id
	}
	return out
}

// pageIDs は limit 件ずつカーソルで最後まで読み、出た id を順に返す。
func pageIDs(t *testing.T, db *DB, q domain.VideoQuery) []int64 {
	t.Helper()
	var out []int64
	for range 50 {
		page, err := db.ListVideos(context.Background(), q)
		if err != nil {
			t.Fatalf("%+v: %v", q, err)
		}
		for _, item := range page.Items {
			out = append(out, item.ID)
		}
		if page.NextCursor == "" {
			return out
		}
		q.Cursor = page.NextCursor
	}
	t.Fatalf("%+v: ページが終わらない", q)
	return nil
}

// 13 の並び順すべてで、ページをまたいで全件が1度ずつ、期待する順で返る。
func TestListVideosAllSortsPageInExpectedOrder(t *testing.T) {
	db, ids := sortFixture(t)
	for _, sort := range allSorts {
		for _, limit := range []int{1, 2, 3} {
			got := pageIDs(t, db, domain.VideoQuery{Sort: sort, Seed: 42, Limit: limit})
			if want := expectedOrder(sort, 42, ids); !slices.Equal(got, want) {
				t.Errorf("%s limit=%d = %v, want %v", sort, limit, got, want)
			}
		}
	}
}

// フォルダの一覧も同じ並び順と seed で並ぶ。
func TestListFolderVideosSortsWithSeed(t *testing.T) {
	db, ids := sortFixture(t)
	for _, sort := range []domain.VideoSort{domain.SortRandom, domain.SortDurationDesc} {
		var got []int64
		cursor := ""
		for range 20 {
			page, err := db.ListFolderVideos(context.Background(), domain.FolderVideoQuery{
				Dir: "/media", Sort: sort, Seed: 9, Limit: 2, Cursor: cursor,
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range page.Items {
				got = append(got, item.ID)
			}
			if cursor = page.NextCursor; cursor == "" {
				break
			}
		}
		if want := expectedOrder(sort, 9, ids); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", sort, got, want)
		}
	}
}

// titleAsc は自然順で、2話 が 10話 より前に来る（親 Issue #195 の受け入れ条件 13）。
func TestListVideosTitleAscIsNatural(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile("/media/10話.mp4", "10話", "key-10", 0),
		listingFile("/media/2話.mp4", "2話", "key-2", 1),
		listingFile("/media/1話.mp4", "1話", "key-1", 2),
	)
	page, err := db.ListVideos(context.Background(), domain.VideoQuery{Sort: domain.SortTitleAsc})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := titlesOf(page), []string{"1話", "2話", "10話"}; !slices.Equal(got, want) {
		t.Errorf("titleAsc = %q, want %q", got, want)
	}
}

// 長さの無い動画と再生の記録の無い動画は、昇順でも降順でも末尾に来る
// （受け入れ条件 14）。
func TestListVideosMissingValuesComeLast(t *testing.T) {
	db, ids := sortFixture(t)
	for _, tc := range []struct {
		sorts   []domain.VideoSort
		missing func(sortRow) bool
	}{
		{[]domain.VideoSort{domain.SortDurationAsc, domain.SortDurationDesc}, func(r sortRow) bool { return r.duration == nil }},
		{[]domain.VideoSort{domain.SortPlayedAsc, domain.SortPlayedDesc}, func(r sortRow) bool { return r.played == nil }},
	} {
		missing := map[int64]bool{}
		for i, row := range sortRows {
			if tc.missing(row) {
				missing[ids[i]] = true
			}
		}
		for _, sort := range tc.sorts {
			got := pageIDs(t, db, domain.VideoQuery{Sort: sort, Limit: 2})
			tail := got[len(got)-len(missing):]
			for _, id := range tail {
				if !missing[id] {
					t.Errorf("%s: 末尾 %v に値のある動画 %d が混じった", sort, tail, id)
				}
			}
		}
	}
}

// random は同じ seed なら同じ並び、別の seed なら別の並びになる。
func TestListVideosRandomDependsOnlyOnSeed(t *testing.T) {
	db, _ := sortFixture(t)
	first := pageIDs(t, db, domain.VideoQuery{Sort: domain.SortRandom, Seed: 1, Limit: 3})
	if again := pageIDs(t, db, domain.VideoQuery{Sort: domain.SortRandom, Seed: 1, Limit: 2}); !slices.Equal(first, again) {
		t.Errorf("同じ seed で並びが変わった: %v, %v", first, again)
	}
	differs := false
	for seed := int64(2); seed < 6; seed++ {
		if !slices.Equal(first, pageIDs(t, db, domain.VideoQuery{Sort: domain.SortRandom, Seed: seed, Limit: 3})) {
			differs = true
		}
	}
	if !differs {
		t.Errorf("seed を変えても並びが変わらない: %v", first)
	}
}

// random で読み進める途中で別の動画に所在を足しても、同じ動画が2度出ない
// （受け入れ条件 15、Edge Case「ランダムと取り込み」）。題名が変わる所在
// （パスが前に来る）を足しても、並べ替えの値は seed と id だけで決まる。
func TestListVideosRandomSurvivesAddedLocations(t *testing.T) {
	db, ids := sortFixture(t)
	ctx := context.Background()
	q := domain.VideoQuery{Sort: domain.SortRandom, Seed: 5, Limit: 2}
	var seen []int64
	for pageIndex := range 20 {
		page, err := db.ListVideos(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			seen = append(seen, item.ID)
		}
		if pageIndex == 0 {
			// まだ出ていない動画すべてに、パスが前に来る所在を足す。
			for i := range sortRows {
				if !slices.Contains(seen, ids[i]) {
					upsertAll(t, db, listingFile(fmt.Sprintf("/media/0-extra-%d.mp4", i), "extra", fmt.Sprintf("key-%d", i), 0))
				}
			}
		}
		if page.NextCursor == "" {
			break
		}
		q.Cursor = page.NextCursor
	}
	got := slices.Clone(seen)
	slices.Sort(got)
	want := slices.Clone(ids)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("出た id = %v, want 全件が1度ずつ %v", seen, ids)
	}
}

// 別の並び順・別の seed で作ったカーソルは ErrInvalidCursor になる
// （contracts/list-api.md §5）。
func TestListVideosRejectsCursorOfOtherSortOrSeed(t *testing.T) {
	db, _ := sortFixture(t)
	ctx := context.Background()
	cursorOf := func(q domain.VideoQuery) string {
		t.Helper()
		q.Limit = 1
		page, err := db.ListVideos(ctx, q)
		if err != nil || page.NextCursor == "" {
			t.Fatalf("%+v: cursor = %q, err = %v", q, page.NextCursor, err)
		}
		return page.NextCursor
	}

	added := cursorOf(domain.VideoQuery{Sort: domain.SortAddedDesc})
	random := cursorOf(domain.VideoQuery{Sort: domain.SortRandom, Seed: 1})
	for _, tc := range []struct {
		name string
		q    domain.VideoQuery
	}{
		{"addedDesc のカーソルを addedAsc に", domain.VideoQuery{Sort: domain.SortAddedAsc, Cursor: added}},
		{"addedDesc のカーソルを random に", domain.VideoQuery{Sort: domain.SortRandom, Seed: 1, Cursor: added}},
		{"random のカーソルを addedDesc に", domain.VideoQuery{Sort: domain.SortAddedDesc, Cursor: random}},
		{"random のカーソルを別の seed に", domain.VideoQuery{Sort: domain.SortRandom, Seed: 2, Cursor: random}},
		{"値の無いカーソルを値の必ずある並びに", domain.VideoQuery{Sort: domain.SortSizeAsc,
			Cursor: base64.RawURLEncoding.EncodeToString([]byte("sizeAsc\x1f\x1f1\x1f1\x1f"))}},
		{"数でない値", domain.VideoQuery{Sort: domain.SortSizeAsc,
			Cursor: base64.RawURLEncoding.EncodeToString([]byte("sizeAsc\x1f\x1f0\x1f1\x1fabc"))}},
	} {
		if _, err := db.ListVideos(ctx, tc.q); !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("%s: err = %v, want domain.ErrInvalidCursor", tc.name, err)
		}
	}

	// 同じ並び順・同じ seed なら続きが取れる。
	if _, err := db.ListVideos(ctx, domain.VideoQuery{Sort: domain.SortRandom, Seed: 1, Cursor: random}); err != nil {
		t.Errorf("同じ seed のカーソルで失敗した: %v", err)
	}
}

// 既知の並び順（VideoSort.Valid）はすべて listOrders に定義がある。定義が無いと
// 空の式で SQL が壊れるので、2つの一覧を揃えて保つ。
func TestListOrdersCoverEveryValidSort(t *testing.T) {
	for _, sort := range allSorts {
		if !sort.Valid() {
			t.Errorf("%s が Valid ではない", sort)
		}
		if _, ok := listOrders[sort]; !ok {
			t.Errorf("%s の並べ替えの定義が listOrders に無い", sort)
		}
	}
	if len(listOrders) != len(allSorts) {
		t.Errorf("listOrders = %d 件, want %d 件", len(listOrders), len(allSorts))
	}
}
