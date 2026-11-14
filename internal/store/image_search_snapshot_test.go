package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// The snapshot reads only the listed folders' indexed videos, gives a bundle member its representative's listed id,
// and counts every listed file video without a readable current index, members included.
func TestImageSearchSnapshotReadsListedIndexesUnderTheAudience(t *testing.T) {
	f := switchedOffFolderFixture(t)
	db := f.db
	ctx := context.Background()
	on := func(name string) string { return fixturePath("/media/on/" + name + ".mp4") }
	ids := upsertAll(t, db, listingFile(on("rep"), "rep", "key-rep", 3), listingFile(on("member"), "member", "key-member", 4),
		listingFile(on("stale"), "stale", "key-stale", 5), listingFile(on("none"), "none", "key-none", 6),
		listingFile(on("bad"), "bad", "key-bad", 7))
	rep, member := ids[on("rep")], ids[on("member")]
	bundle(t, db, rep, rep, member)
	entries := domain.SearchIndex{Entries: []domain.SearchIndexEntry{{TimeMs: 0, Hash: 7}, {TimeMs: 2000, Hash: 9}}}.Encode()
	for key, row := range map[string]struct {
		version int
		entries []byte
	}{
		"key-on": {domain.SearchIndexVersion, entries}, "key-off": {domain.SearchIndexVersion, entries},
		"key-rep": {domain.SearchIndexVersion, entries}, "key-member": {domain.SearchIndexVersion, entries},
		"key-stale": {domain.SearchIndexVersion + 1, entries}, "key-bad": {domain.SearchIndexVersion, []byte{1, 2, 3}},
	} {
		if _, err := db.sql.Exec(`insert into video_search_index (content_key, version, entries, updated_at)
			values (?, ?, ?, 0)`, key, row.version, row.entries); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, err := db.Library().ImageSearchSnapshot(ctx, domain.AudienceOwner)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[int64]int64{}
	for _, c := range snapshot.Candidates {
		listed[c.Content.VideoID] = c.ListedID
		if len(c.Content.Frames) != 2 || c.Content.Frames[1] != (domain.ImageSearchFrame{TimeMs: 2000, Hash: 9}) {
			t.Fatalf("frames of %d = %+v", c.Content.VideoID, c.Content.Frames)
		}
	}
	if want := map[int64]int64{f.shown: f.shown, rep: rep, member: rep}; !reflect.DeepEqual(listed, want) {
		t.Fatalf("listed ids = %v, want %v", listed, want)
	}
	if snapshot.UnindexedCount != 3 {
		t.Fatalf("unindexed = %d, want stale, none and bad", snapshot.UnindexedCount)
	}
}
