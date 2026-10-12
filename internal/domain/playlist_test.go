package domain

import (
	"errors"
	"testing"
)

// プレイリストの一覧のカーソル（specs/044-playlists/contracts/screen-api.md「GET /api/playlists」）。

// カーソルは書いたものを読み戻せ、読めないものは ErrInvalidCursor になる。
func TestPlaylistCursorRoundTrip(t *testing.T) {
	want := PlaylistCursor{UpdatedAtMs: 1_760_000_000_123, ID: 42}
	got, err := DecodePlaylistCursor(EncodePlaylistCursor(want))
	if err != nil || got != want {
		t.Fatalf("読み戻し = %+v, %v, want %+v", got, err, want)
	}
	for _, cursor := range []string{
		"", "!!", "bm90LWpzb24", // "not-json"
		EncodePlaylistCursor(PlaylistCursor{UpdatedAtMs: 1, ID: 0}),
		"eyJ1IjoxfQ", // {"u":1}
	} {
		if _, err := DecodePlaylistCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("DecodePlaylistCursor(%q) = %v, want ErrInvalidCursor", cursor, err)
		}
	}
}
