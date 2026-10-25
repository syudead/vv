package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A bundle's versions share one code and are filled once, through the first version requested.
func TestWorkInfoFillTargetsReturnsOneVideoPerUserKey(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	if _, err := db.sql.Exec(`insert into video_work_info (content_key, code) select user_key, 'AB-1' from video_bundles`); err != nil {
		t.Fatal(err)
	}
	targets, err := db.Overrides().WorkInfoFillTargets(context.Background(), []int64{ids["b"], ids["c"], ids["a"]})
	if err != nil {
		t.Fatalf("WorkInfoFillTargets: %v", err)
	}
	want := []domain.WorkInfoFillTarget{
		{VideoID: ids["b"], ContentKey: "key-b", Code: "AB-1"}, {VideoID: ids["c"], ContentKey: "key-c"},
	}
	if !slices.Equal(targets, want) {
		t.Fatalf("WorkInfoFillTargets = %+v; want %+v", targets, want)
	}
}
