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

	for _, write := range []struct {
		name  string
		apply func() error
	}{
		{"rename", func() error { _, err := db.Tags().RenameTag(ctx, tagID, "旅"); return err }},
		{"attach", func() error { _, _, err := db.Tags().AttachTagByID(ctx, []int64{videoID}, tagID); return err }},
		{"set kind", func() error {
			_, err := db.Tags().BatchTags(ctx, domain.TagBatchRequest{Action: domain.TagBatchSetKind, KindID: genre},
				[]int64{tagID})
			return err
		}},
	} {
		recorder := &eventRecorder{}
		db.PublishTo(recorder)
		if err := write.apply(); err != nil {
			t.Fatalf("%s: %v", write.name, err)
		}
		if got := tagsChangedCount(recorder.events); got != 1 {
			t.Errorf("%s: TagsChanged %d times, want 1: %v", write.name, got, recorder.events)
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
