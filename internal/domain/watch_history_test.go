package domain

import (
	"errors"
	"testing"
	"time"
)

// 視聴の識別子は RFC 4122 の文字列の形（36 文字、8-4-4-4-12 の 16 進）だけを受け付ける。
func TestValidatePlaybackID(t *testing.T) {
	for id, valid := range map[string]bool{
		"0f8fad5b-d9cb-469f-a165-70867728950e":  true,
		"0F8FAD5B-D9CB-469F-A165-70867728950E":  true,
		"00000000-0000-0000-0000-000000000000":  true,
		"":                                      false,
		"0f8fad5b-d9cb-469f-a165-70867728950":   false,
		"0f8fad5b-d9cb-469f-a165-70867728950e0": false,
		"0f8fad5bd-9cb-469f-a165-70867728950e":  false,
		"0f8fad5b-d9cb-469f-a165-70867728950g":  false,
		"{f8fad5b-d9cb-469f-a165-70867728950}":  false,
		"0f8fad5b_d9cb_469f_a165_70867728950e":  false,
	} {
		err := ValidatePlaybackID(id)
		if valid && err != nil {
			t.Errorf("ValidatePlaybackID(%q) = %v, want nil", id, err)
		}
		if !valid && !errors.Is(err, ErrInvalidPlaybackID) {
			t.Errorf("ValidatePlaybackID(%q) = %v, want ErrInvalidPlaybackID", id, err)
		}
	}
}

// 視聴履歴のカーソルは書いたものを読み戻せ、読めないものは ErrInvalidCursor になる。
func TestWatchHistoryCursorRoundTrip(t *testing.T) {
	want := WatchHistoryCursor{PlayedAtMs: 1_760_000_000_123, ID: 42}
	got, err := DecodeWatchHistoryCursor(EncodeWatchHistoryCursor(want))
	if err != nil || got != want {
		t.Fatalf("読み戻し = %+v, %v, want %+v", got, err, want)
	}
	for _, cursor := range []string{
		"", "!!", "bm90LWpzb24", // "not-json"
		EncodeWatchHistoryCursor(WatchHistoryCursor{PlayedAtMs: 1, ID: 0}),
		"eyJwIjoxfQ", // {"p":1}
	} {
		if _, err := DecodeWatchHistoryCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("DecodeWatchHistoryCursor(%q) = %v, want ErrInvalidCursor", cursor, err)
		}
	}
}

// 絞り込みは 3 つの値だけを受け付け、一覧の同じ状態の絞り込みに写る。
func TestParseWatchHistoryFilter(t *testing.T) {
	for value, want := range map[string]WatchFilter{
		"all": WatchAll, "inProgress": WatchInProgress, "watched": WatchWatched,
	} {
		filter, err := ParseWatchHistoryFilter(value)
		if err != nil || filter.WatchFilter() != want {
			t.Errorf("ParseWatchHistoryFilter(%q) = %q, %v, want %q", value, filter, err, want)
		}
	}
	for _, value := range []string{"", "unwatched", "Watched", "in_progress"} {
		if _, err := ParseWatchHistoryFilter(value); !errors.Is(err, ErrInvalidWatchHistoryFilter) {
			t.Errorf("ParseWatchHistoryFilter(%q) = %v, want ErrInvalidWatchHistoryFilter", value, err)
		}
	}
}

// 日付への移動は YYYY-MM-DD と YYYY-MM だけを読み、End はその地域の次の日か次の月の 0 時になる。
func TestWatchHistoryPeriod(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	for value, want := range map[string]time.Time{
		"2026-09-15": time.Date(2026, 9, 16, 0, 0, 0, 0, tokyo),
		"2026-09":    time.Date(2026, 10, 1, 0, 0, 0, 0, tokyo),
		"2026-12":    time.Date(2027, 1, 1, 0, 0, 0, 0, tokyo),
		"2026-12-31": time.Date(2027, 1, 1, 0, 0, 0, 0, tokyo),
	} {
		period, err := ParseWatchHistoryPeriod(value)
		if err != nil {
			t.Errorf("ParseWatchHistoryPeriod(%q) = %v", value, err)
			continue
		}
		if got := period.End(tokyo); !got.Equal(want) {
			t.Errorf("%q の End = %v, want %v", value, got, want)
		}
	}
	for _, value := range []string{
		"", "2026-9", "2026-9-15", "2026-09-5", "2026-13", "2026-02-30", "2026/09", "2026-09-15T00:00",
	} {
		if _, err := ParseWatchHistoryPeriod(value); !errors.Is(err, ErrInvalidWatchHistoryPeriod) {
			t.Errorf("ParseWatchHistoryPeriod(%q) = %v, want ErrInvalidWatchHistoryPeriod", value, err)
		}
	}
}
