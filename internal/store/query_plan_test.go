package store

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

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

// A full video scan per candidate produces quadratic work; require content-key index lookups.
func TestVersionCandidatesUseTheContentKeyIndex(t *testing.T) {
	db := migratedDB(t)
	plan := queryPlan(t, db, `select count(*) `+candidatePairsFrom+candidatePairsCondition())
	for _, step := range plan {
		if strings.HasPrefix(step, "SCAN va") || strings.HasPrefix(step, "SCAN vb") {
			t.Errorf("plan scans the videos: %q\n%s", step, strings.Join(plan, "\n"))
		}
	}
}

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

// A large folder stops at the newest matches of the added-at index; a small one reads only its path range, so an
// empty or sparse root never walks the whole library.
func TestRecentFolderPreviewsPlans(t *testing.T) {
	db := migratedDB(t)
	walk, args := recentFolderPreviewsQuery(domain.AudienceOwner, fixturePath("/media"), false)
	plan := queryPlan(t, db, walk, args...)
	// Only the bundle representative subquery may sort; the outer query reads the index in order.
	if len(plan) == 0 || plan[0] != "SCAN videos USING INDEX videos_added_at_desc_idx" {
		t.Errorf("large folder plan does not walk videos_added_at_desc_idx first:\n%s", strings.Join(plan, "\n"))
	}

	byRange, args := recentFolderPreviewsQuery(domain.AudienceOwner, fixturePath("/media"), true)
	plan = queryPlan(t, db, byRange, args...)
	if len(plan) == 0 || plan[0] != "SEARCH videos USING INTEGER PRIMARY KEY (rowid=?)" {
		t.Errorf("small folder plan does not look up the range's videos by id:\n%s", strings.Join(plan, "\n"))
	}
	for _, step := range plan {
		if strings.HasPrefix(step, "SCAN videos") || strings.HasPrefix(step, "SCAN l") {
			t.Errorf("small folder plan scans a whole table: %q\n%s", step, strings.Join(plan, "\n"))
		}
	}
}
