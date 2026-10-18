package main

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/store"
)

// Exercise the startup/store boundary without starting workers or an HTTP listener.
func TestLibraryInformationStartupRejectsFailedCollectionRecovery(t *testing.T) {
	dir := t.TempDir()
	db, err := store.OpenMigratedForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	ctx := context.Background()
	if err := refreshLibraryInformation(ctx, db, logger); err != nil {
		t.Fatal(err)
	}
	handle, err := sql.Open("sqlite", store.DatabasePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Close() }()
	if _, err := handle.Exec(`insert into collection_changes values ('group', 0);
		create trigger reject_recovery before delete on collection_changes
		begin select raise(abort, 'recovery rejected'); end`); err != nil {
		t.Fatal(err)
	}
	err = refreshLibraryInformation(ctx, db, logger)
	if err == nil || !strings.Contains(err.Error(), "cannot update collection information") || !strings.Contains(err.Error(), "recovery rejected") {
		t.Fatalf("startup error = %v", err)
	}
	if _, err := handle.Exec(`drop trigger reject_recovery`); err != nil {
		t.Fatal(err)
	}
	if err := refreshLibraryInformation(ctx, db, logger); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if count := countRows(t, dir, "collection_changes"); count != 0 {
		t.Fatalf("pending after retry = %d", count)
	}
}
