package store

import (
	"context"
	"testing"
)

func TestTranscodeEncoderChoiceRoundTrip(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	settings := db.Settings()

	value, found, err := settings.TranscodeEncoderChoice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if found || value != "" {
		t.Fatalf("before saving = (%q, %v), want not found", value, found)
	}

	if err := settings.SaveTranscodeEncoderChoice(ctx, "nvenc"); err != nil {
		t.Fatal(err)
	}
	value, found, err = settings.TranscodeEncoderChoice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !found || value != "nvenc" {
		t.Fatalf("after saving = (%q, %v), want (nvenc, true)", value, found)
	}

	// 解釈は store では行わないので、知らない文字列もそのまま戻る。
	if err := settings.SaveTranscodeEncoderChoice(ctx, "future_encoder"); err != nil {
		t.Fatal(err)
	}
	if err := settings.SaveTranscodeEncoderChoice(ctx, "auto"); err != nil {
		t.Fatal(err)
	}
	value, found, err = settings.TranscodeEncoderChoice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !found || value != "auto" {
		t.Fatalf("after saving twice = (%q, %v), want (auto, true)", value, found)
	}

	var rows int
	if err := db.sql.QueryRowContext(ctx, `select count(*) from settings`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("settings rows = %d, want 1", rows)
	}
}

func TestTranscodeEncoderChoiceKeepsUnknownValue(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if err := db.Settings().SaveTranscodeEncoderChoice(ctx, "future_encoder"); err != nil {
		t.Fatal(err)
	}
	value, found, err := db.Settings().TranscodeEncoderChoice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !found || value != "future_encoder" {
		t.Fatalf("stored = (%q, %v), want (future_encoder, true)", value, found)
	}
}
