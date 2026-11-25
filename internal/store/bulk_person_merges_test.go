package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func videoPerformers(t *testing.T, db *DB, videoID int64) []int64 {
	t.Helper()
	info, _ := workInfoOf(t, db, videoID)
	return performerIDs(&info)
}

// The merge reaches the source's videos outside the selection, skips sources gone or named, and can be undone.
func TestApplyVideoEditMergesProvisionalPeopleIntoTheAddedPerson(t *testing.T) {
	f := newMergeFixture(t, false)
	ctx := context.Background()
	named := createPerson(t, f.db, "Named").ID

	result, err := f.db.VideoEdits().Apply(ctx, []int64{f.videoA}, domain.VideoEdit{
		Credits: []domain.CreditEdit{{Kind: domain.CreditPerson, Role: string(domain.PersonPerformer), Add: []int64{f.target}}},
		Merges:  []domain.PersonMerge{{Into: f.target, SourceIDs: []int64{f.provisional, named, 999_999}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Merged) != 1 || result.Merged[0].PersonID != f.provisional || result.Merged[0].Into != f.target ||
		result.Merged[0].UndoActivityID == 0 || !reflect.DeepEqual(result.SkippedPersonIDs, []int64{named, 999_999}) ||
		result.Applied != 1 {
		t.Fatalf("result = %+v, want the provisional person merged with an undo, the others skipped, 1 applied", result)
	}
	for _, video := range []int64{f.videoA, f.videoB} {
		if got := videoPerformers(t, f.db, video); !reflect.DeepEqual(got, []int64{f.target}) {
			t.Errorf("performers of video %d = %v, want only the target", video, got)
		}
	}

	f.undo(t, domain.ActivityEntry{ID: result.Merged[0].UndoActivityID})
	if got := videoPerformers(t, f.db, f.videoB); !reflect.DeepEqual(got, []int64{f.provisional}) {
		t.Errorf("performers of b after undo = %v, want the provisional person back", got)
	}
}

func TestApplyVideoEditWithAMissingMergeTargetWritesNothing(t *testing.T) {
	f := newMergeFixture(t, false)

	_, err := f.db.VideoEdits().Apply(context.Background(), []int64{f.videoA}, domain.VideoEdit{
		Credits: []domain.CreditEdit{{Kind: domain.CreditPerson, Role: string(domain.PersonPerformer), Add: []int64{f.target}}},
		Merges:  []domain.PersonMerge{{Into: 999_999, SourceIDs: []int64{f.provisional}}},
	})
	var missing *domain.CreditNotFoundError
	if !errors.As(err, &missing) || missing.Field != "merges[0].into" {
		t.Fatalf("err = %v, want the person missing at merges[0].into", err)
	}
	if got := videoPerformers(t, f.db, f.videoA); !reflect.DeepEqual(got, []int64{f.provisional}) {
		t.Errorf("performers of a = %v, want the provisional person alone", got)
	}
}

// a holds the provisional person alone, b shares it with a second one; only the added person whose face reaches the
// merge threshold is similar.
func TestProvisionalMatchesTellAloneCoPerformersAndSimilarFaces(t *testing.T) {
	f := newMergeFixture(t, false)
	ctx := context.Background()
	second := createProvisionalPerson(t, f.db)
	info := domain.WorkInfo{People: []domain.WorkPerson{performer(f.provisional), performer(second)}}
	if _, err := f.db.WorkInfo().Set(ctx, f.videoB, info); err != nil {
		t.Fatal(err)
	}
	setFaceVector(t, f.db, f.target, faceNear(2, 4, 0.9))

	got, err := f.db.VideoEdits().ProvisionalMatches(ctx, []int64{f.videoA, f.videoB}, []int64{f.near, f.target})
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ProvisionalMatch{
		{PersonID: f.provisional, Alone: true, CoPerformers: []int64{second}, SimilarTo: []int64{f.target}},
		{PersonID: second, Alone: false, CoPerformers: []int64{f.provisional}, SimilarTo: []int64{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("matches = %+v, want %+v", got, want)
	}

	// The editor lists the unnamed performers too; a person is never similar to themself.
	got, err = f.db.VideoEdits().ProvisionalMatches(ctx, []int64{f.videoA, f.videoB}, []int64{f.target, f.provisional, second})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("matches with the unnamed performers listed = %+v, want %+v", got, want)
	}
}
