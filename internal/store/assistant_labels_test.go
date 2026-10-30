package store

import (
	"context"
	"maps"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Schema rows label activity entries and face appearances, which the card names and the stale check re-reads.
func TestAssistantEntityNamesReadActivityEntriesAndFaceAppearances(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	execSQL(t, db, `insert into video_overrides (content_key, display_name, updated_at) values ('k1', 'Clip A', 1)`)
	execSQL(t, db, `insert into activity_entries (id, kind, feature, subject_kind, subject_key, subject_label, status,
		reason, decider, created_at, updated_at)
		values (7, 'import', 'import', 'video', 'k1', 'a.mp4', 'done', null, 'rule', 1, 1)`)
	execSQL(t, db, `insert into face_appearances (id, content_key, recognizer, dropped, tracks, good_seconds, track_count)
		values (8, 'k1', 'sface_int8', 0, '[]', 10, 2)`)
	const appearance = 8

	for kind, want := range map[domain.AssistantLabelKind]map[int64]string{
		domain.AssistantLabelActivity:       {7: "a.mp4"},
		domain.AssistantLabelFaceAppearance: {appearance: "Clip A"},
	} {
		got, err := db.Assistant().EntityNames(ctx, kind, []int64{7, appearance, 999})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s names = %v, want %v", kind, got, want)
		}
	}
}
