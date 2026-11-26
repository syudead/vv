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
	if _, err := db.Tags().AddSynonym(ctx, tagID, "トラベル", nil); err != nil {
		t.Fatal(err)
	}
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
		{"promote synonym", func() error { _, err := db.Tags().PromoteSynonym(ctx, tagID, "トラベル"); return err },
			committed(`select count(*) from tag_names where tag_id = ? and name = 'トラベル' and canonical = 1`, tagID)},
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

// Deleting or merging away a fileless video changes the videos of the tags it had, and only those.
func TestVirtualVideoRemovalPublishesTagsChangedWhenTagged(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	fileID := upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1))[fixturePath("/media/clip.mp4")]
	untagged := createVirtual(t, db, "Plain", fixedTime)
	deleted := createVirtual(t, db, "Doomed", fixedTime)
	merged := createVirtual(t, db, "Moonlight", fixedTime)
	attachTags(t, db, deleted.ID, "piano")
	attachTags(t, db, merged.ID, "night")

	for _, removal := range []struct {
		name   string
		remove func() error
		want   int
	}{
		{"delete untagged", func() error { return db.VirtualVideos().Delete(ctx, untagged.ID, untagged.UserKey) }, 0},
		{"delete tagged", func() error { return db.VirtualVideos().Delete(ctx, deleted.ID, deleted.UserKey) }, 1},
		{"merge tagged", func() error {
			_, err := db.VirtualVideos().Merge(ctx, merged.ID, merged.UserKey, fileID, "key-clip")
			return err
		}, 1},
	} {
		recorder := &eventRecorder{}
		db.PublishTo(recorder)
		if err := removal.remove(); err != nil {
			t.Fatalf("%s: %v", removal.name, err)
		}
		if got := tagsChangedCount(recorder.events); got != removal.want {
			t.Errorf("%s: TagsChanged %d times, want %d", removal.name, got, removal.want)
		}
	}
}
