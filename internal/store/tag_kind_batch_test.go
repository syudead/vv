package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func presetKindID(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	kinds, err := db.Tags().ListTagKinds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range kinds {
		if kind.Name == name {
			return kind.ID
		}
	}
	t.Fatalf("種類 %q が無い", name)
	return 0
}

func setKindRequest(kindID int64) domain.TagBatchRequest {
	return domain.TagBatchRequest{Action: domain.TagBatchSetKind, KindID: kindID}
}

func TestBatchTagsSetKindGivesEveryTagTheKind(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	genre := presetKindID(t, db, "Genre")
	ids := make([]int64, 10)
	for i := range ids {
		ids[i] = createTagForTest(t, db, fmt.Sprintf("tag %d", i))
	}

	outcome, err := db.Tags().BatchTags(ctx, setKindRequest(genre), ids)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(outcome.AppliedIDs, ids) || len(outcome.NotFoundIDs) != 0 || len(outcome.NotApplicableIDs) != 0 {
		t.Fatalf("outcome = %+v", outcome)
	}
	for _, id := range ids {
		if kind := kindOf(t, db, id); kind == nil || kind.ID != genre || kind.Name != "Genre" {
			t.Errorf("tag %d kind = %+v, want Genre", id, kind)
		}
	}
	assertTagInvariantsNow(t, db)
}

func TestBatchTagsKindKeepsTentativeTagsTentative(t *testing.T) {
	db, ids := batchFixture(t)
	ctx := context.Background()
	work := presetKindID(t, db, "Work")
	tentative := []int64{ids["t1"], ids["t2"]}

	if _, err := db.Tags().BatchTags(ctx, setKindRequest(work), tentative); err != nil {
		t.Fatal(err)
	}
	for _, id := range tentative {
		if !tagTentative(t, db, id) {
			t.Errorf("setKind confirmed tentative tag %d", id)
		}
		if kind := kindOf(t, db, id); kind == nil || kind.ID != work {
			t.Errorf("tag %d kind = %+v, want Work", id, kind)
		}
	}

	outcome, err := db.Tags().BatchTags(ctx, domain.TagBatchRequest{Action: domain.TagBatchClearKind}, tentative)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(outcome.AppliedIDs, tentative) {
		t.Fatalf("clearKind outcome = %+v", outcome)
	}
	for _, id := range tentative {
		if !tagTentative(t, db, id) {
			t.Errorf("clearKind confirmed tentative tag %d", id)
		}
		if kind := kindOf(t, db, id); kind != nil {
			t.Errorf("tag %d kind = %+v after clearKind, want none", id, kind)
		}
	}
	assertTagInvariantsNow(t, db)
}

func TestBatchTagsKindReportsTagsAlreadyInState(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	genre := presetKindID(t, db, "Genre")
	place := presetKindID(t, db, "Place")
	already := createTagForTest(t, db, "already genre")
	other := createTagForTest(t, db, "place tag")
	plain := createTagForTest(t, db, "plain")
	setTagKindForTest(t, db, already, genre)
	setTagKindForTest(t, db, other, place)
	const missing = 999999

	outcome, err := db.Tags().BatchTags(ctx, setKindRequest(genre), []int64{already, other, missing, plain, already})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(outcome.AppliedIDs, []int64{other, plain}) ||
		!slices.Equal(outcome.NotFoundIDs, []int64{missing}) ||
		!slices.Equal(outcome.NotApplicableIDs, []int64{already}) {
		t.Fatalf("setKind outcome = %+v", outcome)
	}

	setTagKindForTest(t, db, plain, 0)
	outcome, err = db.Tags().BatchTags(ctx, domain.TagBatchRequest{Action: domain.TagBatchClearKind}, []int64{plain, already})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(outcome.AppliedIDs, []int64{already}) || !slices.Equal(outcome.NotApplicableIDs, []int64{plain}) {
		t.Fatalf("clearKind outcome = %+v", outcome)
	}
}

func TestBatchTagsSetKindWithMissingKindChangesNothing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	genre := presetKindID(t, db, "Genre")
	kept := createTagForTest(t, db, "kept genre")
	plain := createTagForTest(t, db, "plain")
	setTagKindForTest(t, db, kept, genre)

	_, err := db.Tags().BatchTags(ctx, setKindRequest(999), []int64{kept, plain})
	if !errors.Is(err, domain.ErrTagKindNotFound) {
		t.Fatalf("err = %v, want ErrTagKindNotFound", err)
	}
	if kind := kindOf(t, db, kept); kind == nil || kind.ID != genre {
		t.Errorf("kept kind = %+v, want Genre", kind)
	}
	if kind := kindOf(t, db, plain); kind != nil {
		t.Errorf("plain kind = %+v, want none", kind)
	}
}

func TestBatchTagsRejectsKindIDMismatch(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id := createTagForTest(t, db, "x")
	for _, req := range []domain.TagBatchRequest{
		{Action: domain.TagBatchSetKind},
		{Action: domain.TagBatchClearKind, KindID: 1},
		{Action: domain.TagBatchDelete, KindID: 1},
	} {
		if _, err := db.Tags().BatchTags(ctx, req, []int64{id}); err == nil {
			t.Errorf("BatchTags(%+v) succeeded", req)
		}
	}
	if !tagExists(t, db, id) {
		t.Error("a refused batch deleted the tag")
	}
}

func listedTagNames(page domain.TagPage) []string {
	names := make([]string, 0, len(page.Items))
	for _, tag := range page.Items {
		names = append(names, tag.Name)
	}
	return names
}

func TestListTagsFiltersByKind(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	genre := presetKindID(t, db, "Genre")
	work := presetKindID(t, db, "Work")
	action := createTagForTest(t, db, "action")
	setTagKindForTest(t, db, action, genre)
	setTagKindForTest(t, db, createTagForTest(t, db, "adventure"), genre)
	setTagKindForTest(t, db, createTagForTest(t, db, "series"), work)
	createTagForTest(t, db, "plain")

	cases := []struct {
		query     domain.TagListQuery
		want      []string
		wantTotal int
	}{
		{domain.TagListQuery{Kind: domain.TagKindFilter{ID: genre}}, []string{"action", "adventure"}, 2},
		{domain.TagListQuery{Kind: domain.TagKindFilter{ID: genre}, Search: "act"}, []string{"action"}, 1},
		{domain.TagListQuery{Kind: domain.TagKindFilter{ID: genre}, Limit: 1}, []string{"action"}, 2},
		{domain.TagListQuery{Kind: domain.TagKindFilter{None: true}}, []string{"plain"}, 1},
		{domain.TagListQuery{Kind: domain.TagKindFilter{ID: 999}}, []string{}, 0},
		{domain.TagListQuery{}, []string{"action", "adventure", "plain", "series"}, 4},
	}
	for _, tc := range cases {
		page, err := db.Tags().ListTags(ctx, tc.query)
		if err != nil {
			t.Fatal(err)
		}
		if got := listedTagNames(page); !slices.Equal(got, tc.want) {
			t.Errorf("%+v: names = %v, want %v", tc.query, got, tc.want)
		}
		if page.Total != tc.wantTotal || page.TotalAll != 4 {
			t.Errorf("%+v: total = %d totalAll = %d, want %d and 4", tc.query, page.Total, page.TotalAll, tc.wantTotal)
		}
	}

	for _, tc := range []struct {
		kind domain.TagKindFilter
		want bool
	}{{domain.TagKindFilter{ID: genre}, true}, {domain.TagKindFilter{None: true}, false}, {domain.TagKindFilter{ID: work}, false}} {
		page, err := db.Tags().ListTags(ctx, domain.TagListQuery{Search: "action", Kind: tc.kind, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if got := page.Exact != nil && page.Exact.ID == action; got != tc.want {
			t.Errorf("kind %+v: exact = %+v, want found=%v", tc.kind, page.Exact, tc.want)
		}
	}
}
