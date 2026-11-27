package store

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Distinct counts per filter (1, 2, 3, all below the 4 tags) expose a swapped or unfiltered tally.
func TestTagTalliesMatchListTotals(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	genre := presetKindID(t, db, "Genre")
	ids := createTagsNamed(t, db, "draft", "plain used", "plain idle", "genre idle")
	if _, err := db.sql.Exec(`update tags set tentative = 1 where id = ?`, ids["draft"]); err != nil {
		t.Fatal(err)
	}
	setTagKindForTest(t, db, ids["draft"], genre)
	setTagKindForTest(t, db, ids["genre idle"], genre)
	keys := addLibraryVideos(t, db, 1)
	attachTag(t, db, keys[0], ids["plain used"])

	got, err := db.Tags().TagTallies(ctx, owner)
	if err != nil {
		t.Fatalf("TagTallies() error = %v", err)
	}
	if want := (domain.TagTallies{Tentative: 1, NoKind: 2, Unused: 3}); got != want {
		t.Fatalf("TagTallies() = %+v, want %+v", got, want)
	}
	for _, tc := range []struct {
		query domain.TagListQuery
		count int
	}{
		{domain.TagListQuery{TentativeOnly: true}, got.Tentative},
		{domain.TagListQuery{Kind: domain.TagKindFilter{None: true}}, got.NoKind},
		{domain.TagListQuery{UnusedOnly: true}, got.Unused},
	} {
		page, err := db.Tags().ListTags(ctx, owner, tc.query)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != tc.count {
			t.Errorf("%+v: ListTags Total = %d, tally = %d", tc.query, page.Total, tc.count)
		}
	}
}
