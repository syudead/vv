package store

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// newCastFixture analyses a video whose one main person is new, so it becomes the video's provisional performer.
func newCastFixture(t *testing.T) (faceFixture, int64) {
	t.Helper()
	f := newFaceFixture(t)
	f.analyse(t, facePerson(0.5, 0, faceBasis(2), "crop"))
	return f, faceAppearances(t, f.db)[0].personID
}

// checkCastMerge asserts the face_merge of the provisional person into the person and its effect on the video.
func checkCastMerge(t *testing.T, f faceFixture, provisional, person int64) {
	t.Helper()
	entries := entriesOfKind(t, f.db, domain.ActivityFaceMerge)
	if len(entries) != 1 || entries[0].Subject.Key != strconv.FormatInt(person, 10) ||
		entries[0].Decider != domain.ActivityByRule {
		t.Fatalf("face_merge entries = %+v, want one by rule for person %d", entries, person)
	}
	if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{person}) {
		t.Fatalf("performers = %v, want only %d", got, person)
	}
	if got := faceAppearances(t, f.db); len(got) != 1 || got[0].personID != person {
		t.Fatalf("appearances = %+v, want the provisional person's moved to %d", got, provisional)
	}
}

func TestMergeByCastMergesTheProvisionalPersonIntoTheFetchedPerformer(t *testing.T) {
	f, provisional := newCastFixture(t)
	applied := applyDetails(t, f.db, f.videoID,
		domain.FetchedWorkInfo{People: []domain.FetchedPerson{fetchedPerformer("Actor")}}, performerField)

	merged, err := f.db.Faces().MergeByCast(context.Background(), f.videoID)
	if err != nil || !merged {
		t.Fatalf("MergeByCast = %v, %v; want a merge", merged, err)
	}
	checkCastMerge(t, f, provisional, applied.PersonIDs[0])
}

func TestMergeByCastMergesNothingWithoutAOneToOnePair(t *testing.T) {
	for _, tc := range []struct {
		name string
		// cast sets the video's performers besides the provisional person.
		cast func(t *testing.T, f faceFixture, provisional int64)
	}{
		{"two performers without a face", func(t *testing.T, f faceFixture, _ int64) {
			t.Helper()
			applyDetails(t, f.db, f.videoID, domain.FetchedWorkInfo{People: []domain.FetchedPerson{
				fetchedPerformer("First"), fetchedPerformer("Second"),
			}}, performerField)
		}},
		{"a dismissed pair", func(t *testing.T, f faceFixture, provisional int64) {
			t.Helper()
			named := createPerson(t, f.db, "Named").ID
			if _, err := f.db.sql.Exec(`insert into face_candidates (provisional_id, person_id, similarity, dismissed,
				created_at) values (?, ?, 0.3, 1, 0)`, provisional, named); err != nil {
				t.Fatal(err)
			}
			info := domain.WorkInfo{People: []domain.WorkPerson{performer(provisional), performer(named)}}
			if _, err := f.db.WorkInfo().Set(context.Background(), f.videoID, info); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, provisional := newCastFixture(t)
			tc.cast(t, f, provisional)

			merged, err := f.db.Faces().MergeByCast(context.Background(), f.videoID)
			if err != nil || merged {
				t.Fatalf("MergeByCast = %v, %v; want no merge", merged, err)
			}
			if entries := entriesOfKind(t, f.db, domain.ActivityFaceMerge); len(entries) != 0 {
				t.Fatalf("face_merge entries = %+v, want none", entries)
			}
			if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Contains(got, provisional) {
				t.Fatalf("performers = %v, want the provisional person %d kept", got, provisional)
			}
		})
	}
}

func TestStoreFaceAnalysisMergesByCastWhenTheAnalysisEnds(t *testing.T) {
	f := newFaceFixture(t)
	named := createPerson(t, f.db, "Named").ID
	info := domain.WorkInfo{People: []domain.WorkPerson{performer(named)}}
	if _, err := f.db.WorkInfo().Set(context.Background(), f.videoID, info); err != nil {
		t.Fatal(err)
	}

	f.analyse(t, facePerson(0.5, 0, faceBasis(2), "crop"))
	links := faceLinksOf(t, f.db, entriesOfKind(t, f.db, domain.ActivityFaceAnalysis)[0].ID)
	if len(links) != 1 || !links[0].created {
		t.Fatalf("face links = %+v, want one created provisional person", links)
	}
	checkCastMerge(t, f, links[0].personID, named)
}
