package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func tagsChangedCount(events []domain.Event) int {
	n := 0
	for _, event := range events {
		if _, ok := event.(domain.TagsChanged); ok {
			n++
		}
	}
	return n
}

// Each committed tag-list write publishes TagsChanged once; a write that rolls back publishes nothing.
func TestTagWritesPublishTagsChangedAfterCommit(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tagID := createTagForTest(t, db, "旅行")
	createTagForTest(t, db, "海")
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))
	videoID := ids[fixturePath("/media/a.mp4")]
	genre := presetKindID(t, db, "Genre")

	// committed reads through the read pool, a separate connection that sees only committed rows.
	committed := func(query string, args ...any) func() bool {
		return func() bool {
			var n int
			if err := db.read.QueryRow(query, args...).Scan(&n); err != nil {
				t.Error(err)
			}
			return n == 1
		}
	}
	for _, write := range []struct {
		name    string
		apply   func() error
		visible func() bool
	}{
		{"rename", func() error { _, err := db.Tags().RenameTag(ctx, tagID, "旅"); return err },
			committed(`select count(*) from tag_names where tag_id = ? and name = '旅' and canonical = 1`, tagID)},
		{"attach", func() error { _, _, err := db.Tags().AttachTagByID(ctx, []int64{videoID}, tagID); return err },
			committed(`select count(*) from video_tags where tag_id = ?`, tagID)},
		{"set kind", func() error {
			_, err := db.Tags().BatchTags(ctx, domain.TagBatchRequest{Action: domain.TagBatchSetKind, KindID: genre},
				[]int64{tagID})
			return err
		}, committed(`select count(*) from tags where id = ? and kind_id = ?`, tagID, genre)},
	} {
		published := 0
		db.PublishTo(publisherFunc(func(events ...domain.Event) {
			n := tagsChangedCount(events)
			if n > 0 && !write.visible() {
				t.Errorf("%s: TagsChanged published before the write was committed", write.name)
			}
			published += n
		}))
		if err := write.apply(); err != nil {
			t.Fatalf("%s: %v", write.name, err)
		}
		if published != 1 {
			t.Errorf("%s: TagsChanged %d times, want 1", write.name, published)
		}
	}

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	var conflict *domain.TagNameConflict
	if _, err := db.Tags().RenameTag(ctx, tagID, "海"); !errors.As(err, &conflict) {
		t.Fatalf("RenameTag to a used name = %v, want *domain.TagNameConflict", err)
	}
	if len(recorder.events) != 0 {
		t.Errorf("rolled-back rename published %v", recorder.events)
	}
}
