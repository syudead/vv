package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A bundle's versions share one code and are filled once, through the first version requested; a video without a
// code is counted but not returned.
func TestCodedVideosReturnsOneCodedVideoPerUserKey(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	if _, err := db.sql.Exec(`insert into video_work_info (content_key, code)
		select user_key, 'AB-1' from video_bundles union all select 'key-d', 'D-1'`); err != nil {
		t.Fatal(err)
	}
	coded, keys, err := db.Overrides().CodedVideos(context.Background(), []int64{ids["b"], ids["c"], ids["a"], ids["d"]})
	if err != nil {
		t.Fatalf("CodedVideos: %v", err)
	}
	want := []domain.CodedVideo{{VideoID: ids["b"], ContentKey: "key-b", Code: "AB-1"}, {VideoID: ids["d"], ContentKey: "key-d", Code: "D-1"}}
	if !slices.Equal(coded, want) || keys != 3 {
		t.Fatalf("CodedVideos = %+v, %d keys; want %+v, 3 keys", coded, keys, want)
	}
}
