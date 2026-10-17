package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 追加日と長さによる一覧の絞り込み。

// addedLengthFile は追加日と長さの絞り込みの検証用の動画1本である。duration が nil なら長さが分からない。
type addedLengthFile struct {
	name     string
	addedAgo time.Duration
	duration *int64
	tagged   bool
}

// addedLengthFiles の len600 は 600.999 秒で、秒未満を切り捨てると 600 秒になる。
var addedLengthFiles = []addedLengthFile{
	{name: "added6days", addedAgo: 6 * 24 * time.Hour, duration: ptr(30_000), tagged: true},
	{name: "added8days", addedAgo: 8 * 24 * time.Hour, duration: ptr(30_000), tagged: true},
	{name: "len59", addedAgo: time.Hour, duration: ptr(59_999), tagged: true},
	{name: "len60", addedAgo: time.Hour, duration: ptr(60_000), tagged: true},
	{name: "len600", addedAgo: 30 * 24 * time.Hour, duration: ptr(600_999), tagged: true},
	{name: "len601", addedAgo: time.Hour, duration: ptr(601_000), tagged: true},
	{name: "lenUnknown", addedAgo: time.Hour, duration: nil, tagged: true},
	{name: "len300untagged", addedAgo: time.Hour, duration: ptr(300_000)},
}

// addedLengthFixture は addedLengthFiles を登録フォルダの直下に取り込み、tagged の動画にタグを付けて
// そのタグの id を返す。追加日はいまの時刻から数える。
func addedLengthFixture(t *testing.T) (*DB, int64) {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	tagID := createTagsNamed(t, db, "猫")["猫"]
	now := time.Now()
	for _, file := range addedLengthFiles {
		path := fixturePath("/media/" + file.name + ".mp4")
		key := "key" + path
		got, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: path, Title: file.name, ContentKey: key, SizeBytes: 100,
			MTime: fixedTime, AddedAt: now.Add(-file.addedAgo), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		if file.duration != nil {
			if err := db.Ingest().ApplyProbe(ctx, got.ID, domain.Probe{DurationMs: *file.duration, VideoCodec: "h264"},
				domain.Playability{Playable: true}); err != nil {
				t.Fatal(err)
			}
		}
		if file.tagged {
			attachTag(t, db, key, tagID)
		}
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	return db, tagID
}

func seconds(v int) *int { return &v }

// addedWithin はその日数前以後に追加した動画、minDuration と maxDuration は秒未満を切り捨てた長さが
// 両端を含む範囲にある動画だけにし、長さの分からない動画は当てない。どちらもタグと AND になる。
// ListLibrary・ListVideos・LibraryIDs が同じ動画を返す。
func TestListFiltersByAddedWithinAndLength(t *testing.T) {
	db, tagID := addedLengthFixture(t)

	cases := []struct {
		name string
		q    domain.VideoQuery
		want []string
	}{
		{"追加日", domain.VideoQuery{AddedWithinDays: 7},
			[]string{"added6days", "len300untagged", "len59", "len60", "len601", "lenUnknown"}},
		{"長さの範囲", domain.VideoQuery{MinDurationSec: seconds(60), MaxDurationSec: seconds(600)},
			[]string{"len300untagged", "len60", "len600"}},
		{"下限だけでも長さの分からない動画は当たらない", domain.VideoQuery{MinDurationSec: seconds(0)},
			[]string{"added6days", "added8days", "len300untagged", "len59", "len60", "len600", "len601"}},
		{"追加日とタグの AND", domain.VideoQuery{AddedWithinDays: 7, TagIDs: []int64{tagID}},
			[]string{"added6days", "len59", "len60", "len601", "lenUnknown"}},
		{"長さとタグの AND", domain.VideoQuery{MinDurationSec: seconds(60), MaxDurationSec: seconds(600), TagIDs: []int64{tagID}},
			[]string{"len60", "len600"}},
		{"追加日と長さの AND", domain.VideoQuery{AddedWithinDays: 7, MinDurationSec: seconds(60), MaxDurationSec: seconds(600)},
			[]string{"len300untagged", "len60"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sortedLibraryNames(t, db, tc.q); !slices.Equal(got, tc.want) {
				t.Errorf("ListLibrary = %v, want %v", got, tc.want)
			}

			q := tc.q
			q.Limit = 50
			page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, q)
			if err != nil {
				t.Fatal(err)
			}
			listed := make([]string, 0, len(page.Items))
			for _, video := range page.Items {
				listed = append(listed, video.Title)
			}
			slices.Sort(listed)
			if !slices.Equal(listed, tc.want) || page.Total != len(tc.want) {
				t.Errorf("ListVideos = %v（total %d）, want %v", listed, page.Total, tc.want)
			}

			selection, _, err := db.Library().LibraryIDs(context.Background(), tc.q)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(selection.AllVideoIDs()); got != len(tc.want) {
				t.Errorf("LibraryIDs = %d 本, want %d 本", got, len(tc.want))
			}
		})
	}
}

// addedWithin の境界は一覧を読み始めた時刻で一度だけ決まる。ちょうどその日数前に追加した動画は
// 当たり、その1秒前に追加した動画は当たらず、ListVideos の件数とページが同じ基準で数える
// （SQL の unixepoch() を文ごとに評価すると、件数とページの間に秒が進んで食い違う）。
func TestAddedWithinCutoffIsFixedPerRead(t *testing.T) {
	now := fixedTime
	previous := listNow
	listNow = func() time.Time { return now }
	t.Cleanup(func() { listNow = previous })

	db := migratedDB(t)
	ctx := context.Background()
	boundary := now.Add(-7 * 24 * time.Hour)
	for name, addedAt := range map[string]time.Time{
		"onBoundary": boundary, "beforeBoundary": boundary.Add(-time.Second),
	} {
		path := fixturePath("/media/" + name + ".mp4")
		if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: path, Title: name, ContentKey: "key" + path, SizeBytes: 100,
			MTime: fixedTime, AddedAt: addedAt, Container: "mp4",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}

	q := domain.VideoQuery{AddedWithinDays: 7}
	if got := sortedLibraryNames(t, db, q); !slices.Equal(got, []string{"onBoundary"}) {
		t.Errorf("ListLibrary = %v, want [onBoundary]", got)
	}
	q.Limit = 50
	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "onBoundary" || page.Total != 1 {
		t.Errorf("ListVideos = %d 本（total %d）, want onBoundary だけ", len(page.Items), page.Total)
	}
}
