package store

import (
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// conditionVideo is one library video with the values the duration, added-day and watch filters read.
type conditionVideo struct {
	name       string
	durationMs *int64
	addedAt    time.Time
	// progress is nil for a video without a playback position row.
	progress *domain.Progress
}

func conditionFixture(t *testing.T, videos []conditionVideo) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	ids := map[string]int64{}
	for _, v := range videos {
		key := "k-" + v.name
		id := upsertFolderVideo(t, db, fixturePath("/media/"+v.name+".mp4"), key)
		ids[v.name] = id
		added := v.addedAt
		if added.IsZero() {
			added = time.Date(2025, 6, 1, 12, 0, 0, 0, time.Local)
		}
		execSQL(t, db, `update videos set duration_ms = ?, added_at = ? where id = ?`, v.durationMs, added.Unix(), id)
		if v.progress != nil {
			execSQL(t, db, `insert into playback_progress (content_key, position_ms, duration_ms, completed, updated_at)
				values (?, ?, 0, ?, 1)`, key, v.progress.PositionMs, boolToInt(v.progress.Completed))
		}
	}
	rebuildIndexForTest(t, db)
	return db, ids
}

// assertConditionMatches checks ListVideos, ListLibrary and a smart folder holding the same conditions return the same videos.
func assertConditionMatches(t *testing.T, db *DB, ids map[string]int64, c domain.VideoConditions, want ...string) {
	t.Helper()
	wantIDs := []int64{}
	for _, name := range want {
		wantIDs = append(wantIDs, ids[name])
	}
	slices.Sort(wantIDs)
	q := domain.VideoQuery{Limit: 50, VideoConditions: c}
	if got := listIDs(t, db, domain.AudienceOwner, q); !slices.Equal(got, wantIDs) {
		t.Errorf("%+v: ListVideos = %v, want %v", c, got, wantIDs)
	}
	if library := itemVideoIDs(libraryPages(t, db, domain.AudienceOwner, q)); !slices.Equal(library, wantIDs) {
		t.Errorf("%+v: ListLibrary = %v, want %v", c, library, wantIDs)
	}
	folder := newSmartFolder(t, db, "f", domain.SmartRule{VideoConditions: c}, domain.SortAddedDesc, 0)
	_, items := smartFolderPages(t, db, domain.AudienceOwner, folder, domain.MaxLimit)
	if inFolder := itemVideoIDs(items); !slices.Equal(inFolder, wantIDs) {
		t.Errorf("%+v: smart folder = %v, want %v", c, inFolder, wantIDs)
	}
}

// itemVideoIDs returns the sorted IDs of video items; the fixtures hold no groups.
func itemVideoIDs(items []domain.LibraryItem) []int64 {
	ids := []int64{}
	for _, item := range items {
		if item.Video != nil {
			ids = append(ids, item.Video.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

func TestDurationConditionsIncludeBothBoundsAndSkipUnknown(t *testing.T) {
	ms := func(v int64) *int64 { return &v }
	db, ids := conditionFixture(t, []conditionVideo{
		{name: "unknown"},
		{name: "below", durationMs: ms(599_999)},
		{name: "min", durationMs: ms(600_000)},
		{name: "max", durationMs: ms(1_800_000)},
		{name: "above", durationMs: ms(1_800_001)},
	})
	assertConditionMatches(t, db, ids, domain.VideoConditions{MinDurationMs: 600_000, MaxDurationMs: 1_800_000}, "min", "max")
	assertConditionMatches(t, db, ids, domain.VideoConditions{MinDurationMs: 600_000}, "min", "max", "above")
	assertConditionMatches(t, db, ids, domain.VideoConditions{MaxDurationMs: 1_800_000}, "below", "min", "max")
}

func TestAddedDayConditionsUseLocalDayBounds(t *testing.T) {
	from := time.Date(2025, 3, 10, 0, 0, 0, 0, time.Local)
	to := time.Date(2025, 3, 12, 0, 0, 0, 0, time.Local)
	db, ids := conditionFixture(t, []conditionVideo{
		{name: "before", addedAt: from.Add(-time.Second)},
		{name: "first", addedAt: from},
		{name: "last", addedAt: to.AddDate(0, 0, 1).Add(-time.Second)},
		{name: "after", addedAt: to.AddDate(0, 0, 1)},
	})
	assertConditionMatches(t, db, ids, domain.VideoConditions{AddedFrom: "2025-03-10", AddedTo: "2025-03-12"}, "first", "last")
	assertConditionMatches(t, db, ids, domain.VideoConditions{AddedFrom: "2025-03-10"}, "first", "last", "after")
	assertConditionMatches(t, db, ids, domain.VideoConditions{AddedTo: "2025-03-12"}, "before", "first", "last")
}

func TestWatchStateConditionsFollowPositionRows(t *testing.T) {
	db, ids := conditionFixture(t, []conditionVideo{
		{name: "none"},
		{name: "zero", progress: &domain.Progress{PositionMs: 0}},
		{name: "partway", progress: &domain.Progress{PositionMs: 5_000}},
		{name: "done", progress: &domain.Progress{PositionMs: 5_000, Completed: true}},
	})
	assertConditionMatches(t, db, ids, domain.VideoConditions{WatchState: domain.WatchStateUnwatched}, "none", "zero")
	assertConditionMatches(t, db, ids, domain.VideoConditions{WatchState: domain.WatchStateInProgress}, "partway")
	assertConditionMatches(t, db, ids, domain.VideoConditions{WatchState: domain.WatchStateWatched}, "done")
}
