package store

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// queryPlan は問い合わせの実行計画（EXPLAIN QUERY PLAN の detail）を行ごとに返す。
func queryPlan(t *testing.T, db *DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(), `explain query plan `+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan
}

// TestVersionCandidatesUseTheContentKeyIndex は、候補の組の動画を内容の識別子の索引で引き、
// videos を全件走査しないことを確かめる（issue 674: 走査すると動画の本数の2乗で時間が増える）。
func TestVersionCandidatesUseTheContentKeyIndex(t *testing.T) {
	db := migratedDB(t)
	plan := queryPlan(t, db, `select count(*) `+candidatePairsFrom+candidatePairsCondition())
	for _, step := range plan {
		if strings.HasPrefix(step, "SCAN va") || strings.HasPrefix(step, "SCAN vb") {
			t.Errorf("plan scans the videos: %q\n%s", step, strings.Join(plan, "\n"))
		}
	}
}

// TestFolderLocationsSearchThePathRange は、フォルダの配下の所在をパスの範囲で引き、
// video_locations を全件走査しないことを確かめる（issue 674）。
func TestFolderLocationsSearchThePathRange(t *testing.T) {
	db := migratedDB(t)
	inFolder, args := folderPrefixCondition("l", folderPrefix(fixturePath("/media/show")))
	plan := queryPlan(t, db, `select l.path from video_locations l join videos on videos.id = l.video_id
		where `+inFolder+` and `+visibleLocationCondition("l", domain.AudienceOwner), args...)
	index := "sqlite_autoindex_video_locations_1"
	if runtime.GOOS == "windows" {
		index = "video_locations_lower_path_idx"
	}
	found := false
	for _, step := range plan {
		if strings.HasPrefix(step, "SCAN l") {
			t.Errorf("plan scans the locations: %q\n%s", step, strings.Join(plan, "\n"))
		}
		if strings.HasPrefix(step, "SEARCH l USING") && strings.Contains(step, index) {
			found = true
		}
	}
	if !found {
		t.Errorf("plan does not search %s:\n%s", index, strings.Join(plan, "\n"))
	}
}
