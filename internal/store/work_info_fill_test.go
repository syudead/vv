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

// A fill names a file video that has no display name, and keeps a display name the owner set.
func TestFillNamesOnlyAFileVideoWithoutADisplayName(t *testing.T) {
	ctx := context.Background()
	db, ids := overrideFixture(t)
	alpha, beta := ids[fixturePath("/media/alpha.mp4")], ids[fixturePath("/media/beta.mp4")]
	if _, err := db.Overrides().SetDisplayName(ctx, beta, "Owner's name"); err != nil {
		t.Fatal(err)
	}
	plugin := approvedPlugin(t, db, "example", "d1")
	for id, key := range map[int64]string{alpha: "key-alpha", beta: "key-beta"} {
		title := "Fetched title"
		if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
			VideoID: id, ContentKey: key, PluginID: plugin.ID, Digest: "d1", Generation: plugin.Generation,
			Title: &title, FillEmpty: true,
		}); err != nil {
			t.Fatalf("fill %s: %v", key, err)
		}
	}
	if got := displayNameOf(t, db, alpha); got != "Fetched title" {
		t.Errorf("alpha = %q; want the fetched title", got)
	}
	if got := displayNameOf(t, db, beta); got != "Owner's name" {
		t.Errorf("beta = %q; want the owner's name kept", got)
	}
}
