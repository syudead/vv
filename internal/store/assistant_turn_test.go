package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func startTurn(t *testing.T, db *DB, text string) domain.AssistantTurn {
	t.Helper()
	turn, err := db.Assistant().CreateTurn(context.Background(), domain.AssistantTurnInput{
		Text: text, Context: domain.AssistantTurnContext{Route: domain.AssistantRouteLibrary, SelectedVideoIDs: []int64{7}},
	}, assistantNow)
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

func TestAssistantTurnRunsOneAtATimeAndFinishesOnce(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	first := startTurn(t, db, "猫の動画は何本?")
	if first.Status != domain.AssistantTurnRunning || !slices.Equal(first.Context.SelectedVideoIDs, []int64{7}) {
		t.Fatalf("turn = %+v", first)
	}
	busy, err := db.Assistant().CreateTurn(ctx, domain.AssistantTurnInput{Text: "もう一つ"}, assistantNow)
	if !errors.Is(err, domain.ErrAssistantBusy) || busy.ID != first.ID {
		t.Fatalf("second turn = %+v, %v; want busy with the first", busy, err)
	}

	first.Status, first.Answer = domain.AssistantTurnDone, "12 本です。"
	if err := db.Assistant().FinishTurn(ctx, first, assistantNow); err != nil {
		t.Fatal(err)
	}
	first.Status = domain.AssistantTurnStopped
	if err := db.Assistant().FinishTurn(ctx, first, assistantNow); !errors.Is(err, domain.ErrAssistantTurnNotRunning) {
		t.Fatalf("second finish error = %v", err)
	}
	stored, err := db.Assistant().Turn(ctx, first.ID)
	if err != nil || stored.Status != domain.AssistantTurnDone || stored.Answer != "12 本です。" || stored.FinishedAt.IsZero() {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	startTurn(t, db, "次の依頼")
}

func TestAssistantTurnProgressPublishesEachCommit(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	turn := startTurn(t, db, "猫の動画は何本?")
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	item := domain.AssistantProgressItem{Tool: "listVideos", Kind: domain.AssistantProgressRead, Labels: []domain.AssistantLabel{}, At: assistantNow}
	for n := 1; n <= 2; n++ {
		if err := db.Assistant().RecordTurnProgress(ctx, turn.ID, slices.Repeat([]domain.AssistantProgressItem{item}, n)); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.events) != 2 || recorder.events[1] != (domain.AssistantChanged{}) {
		t.Fatalf("events = %v, want one per progress commit", recorder.events)
	}
	stored, err := db.Assistant().Turn(ctx, turn.ID)
	if err != nil || len(stored.Progress) != 2 || stored.Progress[1].Tool != "listVideos" {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
}

func TestRecoverAssistantTurnsInterruptsTheRunningTurn(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	turn := startTurn(t, db, "猫の動画は何本?")

	if n, err := db.Assistant().RecoverTurns(ctx, assistantNow); err != nil || n != 1 {
		t.Fatalf("RecoverTurns() = %d, %v", n, err)
	}
	stored, err := db.Assistant().Turn(ctx, turn.ID)
	if err != nil || stored.Status != domain.AssistantTurnInterrupted {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	if _, err := db.Assistant().RunningTurn(ctx); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("RunningTurn() error = %v", err)
	}
}

func TestAssistantTurnsExpireAndLeaveTheActiveList(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	old := startTurn(t, db, "古い依頼")
	old.Status = domain.AssistantTurnDone
	if err := db.Assistant().FinishTurn(ctx, old, assistantNow.Add(-2*domain.AssistantActiveTurnWindow)); err != nil {
		t.Fatal(err)
	}
	running := startTurn(t, db, "今の依頼")

	active, err := db.Assistant().ActiveTurns(ctx, assistantNow.Add(-domain.AssistantActiveTurnWindow))
	if err != nil || len(active) != 1 || active[0].ID != running.ID {
		t.Fatalf("active = %+v, %v", active, err)
	}
	if n, err := db.Assistant().ExpireTurns(ctx, assistantNow.Add(-domain.AssistantActiveTurnWindow)); err != nil || n != 1 {
		t.Fatalf("ExpireTurns() = %d, %v", n, err)
	}
	if _, err := db.Assistant().Turn(ctx, old.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expired turn error = %v", err)
	}
}

func TestAssistantNamesInFindsFoldedNamesOfThreeOrMoreCharacters(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	for _, name := range []string{"カラオケ", "Live", "猫"} {
		if _, err := db.Tags().CreateTag(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	createPerson(t, db, "山田花子", "Hanako Yamada")

	got, err := db.Assistant().NamesIn(ctx, "からおけ と LIVE と 猫 の動画、hanako yamada が出ています")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"hanako yamada", "live", "カラオケ"}; !slices.Equal(got, want) {
		t.Fatalf("NamesIn() = %v, want %v", got, want)
	}
}
