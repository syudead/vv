package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Test run-log orchestration; store tests verify replacement and generation checks against SQLite.

func TestRunSavesTheMaskedLogWithTheGenerationThatRan(t *testing.T) {
	e := newFetchEnv()
	e.store.plugins[1].Generation = 3
	e.runtime.stderr = "using key-one for ja; key-one again"
	if _, err := e.plugins.SearchWorkInfo(context.Background(), 1, "work"); err != nil {
		t.Fatal(err)
	}
	want := domain.WorkInfoPluginRun{
		PluginID: 1, Generation: 3, RanAt: e.now,
		WorkInfoRunLog: domain.WorkInfoRunLog{
			Operation: domain.WorkInfoSearch, Stderr: "using [secret] for ja; [secret] again",
		},
	}
	if len(e.store.runs) != 1 || e.store.runs[0] != want {
		t.Fatalf("runs = %+v, want [%+v]", e.store.runs, want)
	}

	run, ok, err := e.plugins.WorkInfoPluginLastRun(context.Background(), 1)
	if err != nil || !ok || run != want {
		t.Fatalf("WorkInfoPluginLastRun = %+v, %v, %v; want %+v", run, ok, err, want)
	}
}

func TestRunSavesTheLogOfAFailedRun(t *testing.T) {
	e := newFetchEnv()
	e.runtime.stderr = "panic: key-one"
	e.runtime.truncated = true
	e.runtime.err = &domain.WorkInfoRunError{Failure: domain.WorkInfoRunTimedOut}
	_, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "W-1")
	var runErr *domain.WorkInfoRunError
	if !errors.As(err, &runErr) {
		t.Fatalf("FetchWorkInfo err = %v, want the run failure", err)
	}
	want := domain.WorkInfoRunLog{
		Operation: domain.WorkInfoFetch, Failure: domain.WorkInfoRunTimedOut, Stderr: "panic: [secret]", Truncated: true,
	}
	if len(e.store.runs) != 1 || e.store.runs[0].WorkInfoRunLog != want {
		t.Fatalf("runs = %+v, want one with %+v", e.store.runs, want)
	}
}

// Persist runs in completion order, replacing the previous log.
func TestRunLastRunIsTheNewest(t *testing.T) {
	e := newFetchEnv()
	e.runtime.stderr = "first"
	if _, err := e.plugins.SearchWorkInfo(context.Background(), 1, "work"); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(time.Minute)
	e.runtime.stderr = "second"
	if _, err := e.plugins.SearchWorkInfo(context.Background(), 1, "work"); err != nil {
		t.Fatal(err)
	}
	run, ok, err := e.plugins.WorkInfoPluginLastRun(context.Background(), 1)
	if err != nil || !ok || run.Stderr != "second" || !run.RanAt.Equal(e.now) {
		t.Fatalf("WorkInfoPluginLastRun = %+v, %v, %v; want the second run", run, ok, err)
	}
}

// Preflight rejections never execute the module or record a log.
func TestRunRefusedBeforeTheModuleRunsSavesNoLog(t *testing.T) {
	e := newFetchEnv()
	e.store.plugins[1].ApprovedDigest = ""
	if _, err := e.plugins.SearchWorkInfo(context.Background(), 1, "work"); !errors.Is(err, domain.ErrWorkInfoPluginNotApproved) {
		t.Fatalf("err = %v", err)
	}
	if len(e.store.runs) != 0 {
		t.Fatalf("runs = %+v, want none", e.store.runs)
	}
}
