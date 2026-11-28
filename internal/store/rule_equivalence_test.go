package store

import (
	"context"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// These tests evaluate each SQL predicate that restates a domain rule over the same inputs as the Go function, so
// a change to one side fails here until the other matches (ARCHITECTURE.md, "The domain decides, the store enforces").

func queryBool(t *testing.T, db *DB, query string, args ...any) bool {
	t.Helper()
	var got bool
	if err := db.sql.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("%v\n%s", err, query)
	}
	return got
}

type claimInputs struct {
	registered bool
	probe      domain.ProbeState
	// thumbnail is the other video's thumbnail job: "none", "queued", "running", or "parked" (queued under an
	// unavailable folder, which is not claimable).
	thumbnail string
	seek      domain.SeekThumbnailState
}

func setClaimInputs(t *testing.T, db *DB, videoID int64, in claimInputs) {
	t.Helper()
	path := fixturePath("/outside/a.mp4")
	if in.registered {
		path = fixturePath("/media/a.mp4")
	}
	execSQL(t, db, `update video_locations set path = ? where video_id = ?`, path, videoID)
	execSQL(t, db, `update videos set probe_state = ?, seek_thumbnail_state = ? where id = ?`,
		string(in.probe), string(in.seek), videoID)
	state := map[string]string{"none": "failed", "queued": "queued", "running": "running", "parked": "queued"}[in.thumbnail]
	execSQL(t, db, `update jobs set state = ? where kind = ?`, state, string(domain.JobThumbnail))
}

func TestClaimConditionSQLMatchesDomain(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	other, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/b.mp4"), "b", "key-b", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJob(ctx, domain.JobThumbnail, other.ID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	var folderID int64
	if err := db.sql.QueryRow(`select id from media_folders`).Scan(&folderID); err != nil {
		t.Fatal(err)
	}
	kinds := []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobFingerprint}
	probes := []domain.ProbeState{domain.ProbeStatePending, domain.ProbeStateDone, domain.ProbeStateFailed}
	seeks := []domain.SeekThumbnailState{domain.SeekThumbnailPending, domain.SeekThumbnailDone, domain.SeekThumbnailFailed}
	for _, registered := range []bool{false, true} {
		for _, probe := range probes {
			for _, thumbnail := range []string{"none", "queued", "running", "parked"} {
				for _, seek := range seeks {
					in := claimInputs{registered: registered, probe: probe, thumbnail: thumbnail, seek: seek}
					setClaimInputs(t, db, videoID, in)
					var unavailable []int64
					if thumbnail == "parked" {
						unavailable = []int64{folderID}
					}
					claimable := thumbnail == "queued" || thumbnail == "running"
					for _, kind := range kinds {
						c := domain.ClaimConditionFor(kind)
						got := queryBool(t, db, `select exists (select 1 from (select ? as video_id) j where 1 = 1`+
							claimConditionSQL(c, "j", unavailable)+`)`, videoID)
						if want := c.Allows(registered, probe, claimable, seek); got != want {
							t.Errorf("%s %+v: SQL = %v, domain = %v", kind, in, got, want)
						}
					}
				}
			}
		}
	}
}

// progressRow is a playback_progress row as a left join yields it; nil is the missing row.
func progressRow(p *domain.Progress) []any {
	if p == nil {
		return []any{nil, nil, nil}
	}
	return []any{"key", p.Completed, p.PositionMs}
}

var watchCases = []*domain.Progress{
	nil,
	{PositionMs: 0},
	{PositionMs: 1},
	{PositionMs: 0, Completed: true},
	{PositionMs: 5000, Completed: true},
}

func TestWatchStateSQLMatchesClassifyWatch(t *testing.T) {
	db := migratedDB(t)
	from := ` from (select ? as content_key, ? as completed, ? as position_ms) p`
	filters := map[domain.WatchHistoryFilter]domain.WatchState{
		domain.WatchHistoryInProgress: domain.WatchStateInProgress,
		domain.WatchHistoryWatched:    domain.WatchStateWatched,
	}
	for _, p := range watchCases {
		want := domain.ClassifyWatch(p)
		var got string
		if err := db.sql.QueryRow(`select `+videoWatchStateExpr+from, progressRow(p)...).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if domain.WatchState(got) != want {
			t.Errorf("videoWatchStateExpr(%+v) = %s, ClassifyWatch = %s", p, got, want)
		}
		for filter, state := range filters {
			matched := queryBool(t, db, `select coalesce((`+watchHistoryStateCondition(filter)+`), 0)`+from, progressRow(p)...)
			if matched != (want == state) {
				t.Errorf("watchHistoryStateCondition(%s) on %+v = %v, ClassifyWatch = %s", filter, p, matched, want)
			}
		}
	}
}

func TestGroupWatchCountsSQLMatchesGroupWatch(t *testing.T) {
	db := migratedDB(t)
	for _, members := range [][]*domain.Progress{
		{},
		{nil, watchCases[1]},
		{watchCases[2], nil},
		{watchCases[3], watchCases[4]},
		{watchCases[4], nil},
		{watchCases[4], watchCases[2]},
	} {
		// Each member contributes its own left-joined progress row, coalesced as groupValuesSelect does.
		rows := []string{`select null as content_key, null as completed, null as position_ms where 0`}
		args := []any{}
		for _, p := range members {
			rows = append(rows, `select ?, ?, ?`)
			args = append(args, progressRow(p)...)
		}
		query := `select count(*), ` + groupWatchCountsSQL + ` from (select ` + ownerMemberProgressColumns +
			` from (` + strings.Join(rows, ` union all `) + `) p) m`
		var count, watched, started int
		if err := db.sql.QueryRow(query, args...).Scan(&count, &watched, &started); err != nil {
			t.Fatalf("%v\n%s", err, query)
		}
		wantState, wantWatched := domain.GroupWatch(members)
		if got := domain.WatchStateOf(count, started, watched); got != wantState || watched != wantWatched {
			t.Errorf("members %v: SQL = %s/%d, GroupWatch = %s/%d", members, got, watched, wantState, wantWatched)
		}
	}
}

// Media folder and location paths are stored cleaned, so the cases are clean paths.
func TestInMediaFolderConditionMatchesPathWithinRoot(t *testing.T) {
	db := migratedDB(t)
	roots := []string{"/", "/media", "/media/sub"}
	paths := []string{
		"/", "/media", "/media/a.mp4", "/media/sub/b.mp4", "/media-other/a.mp4", "/mediaa", "/med", "/Media/a.mp4",
		"/media\\a.mp4", "/other/media/a.mp4",
	}
	matched := 0
	for _, root := range roots {
		for _, path := range paths {
			root, path := fixturePath(root), fixturePath(path)
			got := queryBool(t, db, `select `+inMediaFolderCondition(`?2`)+` from (select ?1 as path) mf`, root, path)
			if want := domain.PathWithinRoot(root, path); got != want {
				t.Errorf("root %q, path %q: SQL = %v, PathWithinRoot = %v", root, path, got, want)
			}
			if got {
				matched++
			}
		}
	}
	if matched == 0 || matched == len(roots)*len(paths) {
		t.Fatalf("cases must include both outcomes, matched %d", matched)
	}
}

func TestScopedLocationConditionMatchesListsFolder(t *testing.T) {
	db := migratedDB(t)
	execSQL(t, db, `insert into media_folders(path, version, enabled, created_at, updated_at) values (?, 1, 0, 1, 1)`,
		fixturePath("/off"))
	folders, err := db.Settings().ListMediaFolders(context.Background())
	if err != nil || len(folders) != 2 {
		t.Fatalf("folders = %+v, %v", folders, err)
	}
	audiences := []domain.Audience{
		domain.AudienceGuest,
		domain.AudienceOwner,
		domain.AudienceGuest.WithExcludedFolders(nil),
		domain.AudienceOwner.WithExcludedFolders([]int64{folders[0].ID}),
		domain.AudienceGuest.WithExcludedFolders([]int64{folders[1].ID}),
	}
	for _, path := range []string{"/media/a.mp4", "/off/a.mp4", "/elsewhere/a.mp4"} {
		path = fixturePath(path)
		for _, audience := range audiences {
			want := false
			for _, folder := range folders {
				want = want || (domain.PathWithinRoot(folder.Path, path) && audience.ListsFolder(folder))
			}
			got := queryBool(t, db, `select `+scopedLocationCondition("l", audience)+` from (select ? as path) l`, path)
			if got != want {
				t.Errorf("%s %+v: SQL = %v, ListsFolder = %v", path, audience, got, want)
			}
		}
	}
}
