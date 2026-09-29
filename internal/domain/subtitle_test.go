package domain

import (
	"slices"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func sidecarEntries(names ...string) []SidecarEntry {
	entries := make([]SidecarEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, SidecarEntry{Name: name, Size: 100})
	}
	return entries
}

func TestSubtitleSidecarsMatchesNames(t *testing.T) {
	tests := []struct {
		video string
		entry string
		want  []SubtitleSidecar
	}{
		{"movie.mp4", "movie.srt", []SubtitleSidecar{{File: "movie.srt", Label: "", Format: SubtitleFormatSRT}}},
		{"movie.mp4", "Movie.SRT", []SubtitleSidecar{{File: "Movie.SRT", Label: "", Format: SubtitleFormatSRT}}},
		{"movie.mp4", "movie.ja.srt", []SubtitleSidecar{{File: "movie.ja.srt", Label: "ja", Format: SubtitleFormatSRT}}},
		{"movie.mp4", "movie.en.forced.vtt", []SubtitleSidecar{{File: "movie.en.forced.vtt", Label: "en.forced", Format: SubtitleFormatVTT}}},
		{"movie.mp4", "MOVIE.JA.Vtt", []SubtitleSidecar{{File: "MOVIE.JA.Vtt", Label: "JA", Format: SubtitleFormatVTT}}},
		{"my.movie.2024.mp4", "my.movie.2024.ja.srt", []SubtitleSidecar{{File: "my.movie.2024.ja.srt", Label: "ja", Format: SubtitleFormatSRT}}},
		// 動画と字幕で正規化の形が違っても（macOS の NFD）照合する。File は元の名前のまま。
		{norm.NFD.String("café.mp4"), norm.NFC.String("café.fr.srt"), []SubtitleSidecar{{File: norm.NFC.String("café.fr.srt"), Label: "fr", Format: SubtitleFormatSRT}}},
		{"movie.mp4", "movie2.srt", nil},
		{"movie.mp4", "movie.txt", nil},
		{"movie.mp4", "other.srt", nil},
		{"movie.mp4", "movie..srt", nil},
		{"movie.mp4", "movie.mp4", nil},
		{"movie.mp4", "srt", nil},
		{"my.movie.2024.mp4", "my.movie.srt", nil},
	}
	for _, tt := range tests {
		got := SubtitleSidecars(tt.video, sidecarEntries(tt.entry))
		if !slices.Equal(got, tt.want) {
			t.Errorf("SubtitleSidecars(%q, %q) = %+v, want %+v", tt.video, tt.entry, got, tt.want)
		}
	}
}

func TestSubtitleSidecarsDedupesAndOrders(t *testing.T) {
	got := SubtitleSidecars("movie.mp4", sidecarEntries(
		"movie.ja.srt", "movie.en.srt", "movie.JA.vtt", "movie.10.srt", "movie.2.srt",
		"Movie.srt", "movie.srt", "movie.fr.srt", "movie.FR.srt", "notes.txt",
	))
	want := []SubtitleSidecar{
		{File: "Movie.srt", Label: "", Format: SubtitleFormatSRT},
		{File: "movie.2.srt", Label: "2", Format: SubtitleFormatSRT},
		{File: "movie.10.srt", Label: "10", Format: SubtitleFormatSRT},
		{File: "movie.en.srt", Label: "en", Format: SubtitleFormatSRT},
		{File: "movie.FR.srt", Label: "FR", Format: SubtitleFormatSRT},
		{File: "movie.JA.vtt", Label: "JA", Format: SubtitleFormatVTT},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("SubtitleSidecars = %+v\nwant %+v", got, want)
	}
}

func TestSubtitleSidecarsSkipsFilesOverTheLimit(t *testing.T) {
	got := SubtitleSidecars("movie.mp4", []SidecarEntry{
		{Name: "movie.srt", Size: SubtitleFileLimit},
		{Name: "movie.ja.vtt", Size: SubtitleFileLimit + 1},
		{Name: "movie.ja.srt", Size: 10},
		{Name: "movie.en.srt", Size: SubtitleFileLimit + 1},
	})
	want := []SubtitleSidecar{
		{File: "movie.srt", Label: "", Format: SubtitleFormatSRT},
		// 上限を超える .vtt は候補にならないので、同じラベルの .srt を隠さない。
		{File: "movie.ja.srt", Label: "ja", Format: SubtitleFormatSRT},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("SubtitleSidecars = %+v, want %+v", got, want)
	}
	if SubtitleFileLimit != 4*1024*1024 {
		t.Fatalf("SubtitleFileLimit = %d, want 4 MiB", SubtitleFileLimit)
	}
}
