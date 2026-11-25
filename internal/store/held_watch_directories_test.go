package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// The rows round-trip with their recursive flag, a directory held twice is one row, and a release deletes only the
// directories it names. A write that changes the rows tells the background status; one that changes none does not.
func TestHeldWatchDirectoriesRoundTripAndRelease(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scans := db.Scans()
	a := domain.DirtyDirectory{Path: fixturePath("/media/a"), Recursive: false}
	aTree := domain.DirtyDirectory{Path: fixturePath("/media/a"), Recursive: true}
	b := domain.DirtyDirectory{Path: fixturePath("/media/b"), Recursive: false}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if err := scans.HoldWatchDirectories(ctx, []domain.DirtyDirectory{b, a}); err != nil {
		t.Fatal(err)
	}
	if err := scans.HoldWatchDirectories(ctx, []domain.DirtyDirectory{a, aTree}); err != nil {
		t.Fatal(err)
	}
	got, err := scans.HeldWatchDirectories(ctx)
	if want := []domain.DirtyDirectory{a, aTree, b}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("held = %v, %v; want %v", got, err, want)
	}

	if err := scans.ReleaseWatchDirectories(ctx, []domain.DirtyDirectory{a, b}); err != nil {
		t.Fatal(err)
	}
	got, err = scans.HeldWatchDirectories(ctx)
	if want := []domain.DirtyDirectory{aTree}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("held after the release = %v, %v; want %v", got, err, want)
	}
	if err := scans.HoldWatchDirectories(ctx, []domain.DirtyDirectory{aTree}); err != nil {
		t.Fatal(err)
	}
	if err := scans.ReleaseWatchDirectories(ctx, []domain.DirtyDirectory{b}); err != nil {
		t.Fatal(err)
	}
	changed := slices.Repeat([]domain.Event{domain.BackgroundChanged{}}, 3)
	if !slices.Equal(recorder.events, changed) {
		t.Fatalf("events = %v, want %v", recorder.events, changed)
	}
}
