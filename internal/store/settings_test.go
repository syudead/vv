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

// LAN からの接続の許可は、行が無ければ偽で、保存した値が読める
// （specs/037-windows-app/research.md R-14）。
func TestLANAccessRoundTrip(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	settings := db.Settings()

	allowed, err := settings.LANAccess(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("行が無いのに許可になっている")
	}

	for _, want := range []bool{true, false, true} {
		if err := settings.SaveLANAccess(ctx, want); err != nil {
			t.Fatal(err)
		}
		got, err := settings.LANAccess(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("保存した %v を読むと %v", want, got)
		}
	}

	var value string
	if err := db.sql.QueryRowContext(ctx, `select value from settings where key = 'desktop.lan_access'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "true" {
		t.Fatalf("保存値 = %q, want true", value)
	}
}

// 知らない値は許可として読まない（許可していないのに LAN で待ち受けない）。
func TestLANAccessTreatsUnknownValueAsDenied(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.sql.ExecContext(ctx,
		`insert into settings (key, value, updated_at) values ('desktop.lan_access', 'yes', 0)`); err != nil {
		t.Fatal(err)
	}
	allowed, err := db.Settings().LANAccess(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("知らない値を許可として読んだ")
	}
}
