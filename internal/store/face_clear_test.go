package store

import (
	"context"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Clearing deletes every person, named or provisional, with their credits and face data, and a later analysis
// of the same video makes its people afresh instead of carrying the old appearances over.
func TestClearPeopleDeletesEveryPersonAndAnalysesTheVideosAfresh(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run("enabled="+strconv.FormatBool(enabled), func(t *testing.T) {
			f := newLinkedFixture(t)
			ctx := context.Background()
			if err := writeSetting(ctx, f.db.sql, faceEnabledKey, strconv.FormatBool(enabled)); err != nil {
				t.Fatal(err)
			}
			generation := countRows(t, f.db, `select coalesce((select cast(value as integer) from settings
				where key = 'face.generation'), 0)`)

			cleared, err := f.db.Faces().ClearPeople(ctx)
			want := domain.PeopleCleared{People: 4}
			if enabled {
				want.Queued = 1
			}
			if err != nil || cleared != want {
				t.Fatalf("ClearPeople = %+v, %v; want %+v", cleared, err, want)
			}
			for _, table := range []string{"people", "face_appearances", "video_work_people", "face_candidates",
				"person_face_vectors"} {
				if n := countRows(t, f.db, `select count(*) from `+table); n != 0 {
					t.Errorf("%s has %d rows, want none", table, n)
				}
			}
			if got := countRows(t, f.db, `select cast(value as integer) from settings where key = 'face.generation'`); got != generation+1 {
				t.Errorf("face.generation = %d, want %d", got, generation+1)
			}
			if got := countRows(t, f.db, `select count(*) from jobs where kind = 'face' and state = 'queued'`); got != int64(want.Queued) {
				t.Errorf("queued rows = %d, want %d", got, want.Queued)
			}

			f.analyse(t, facePerson(0.25, 0, faceBasis(0), "crop-again"))
			appearances := faceAppearances(t, f.db)
			if len(appearances) != 1 || appearances[0].personID == 0 || appearances[0].dropped {
				t.Fatalf("appearances after a new analysis = %+v, want one new person", appearances)
			}
		})
	}
}
