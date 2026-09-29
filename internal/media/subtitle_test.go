package media

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
	xunicode "golang.org/x/text/encoding/unicode"

	"github.com/syudead/vv/internal/domain"
)

const japaneseSRT = "1\r\n00:00:01,000 --> 00:00:02,500\r\nこんにちは、世界\r\n\r\n2\r\n00:00:03,000 --> 00:00:04,000\r\n字幕の表示を確かめる。\r\n二行目\r\n"

const japaneseVTT = "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\nこんにちは、世界\n\n00:00:03.000 --> 00:00:04.000\n字幕の表示を確かめる。\n二行目\n"

func convert(t *testing.T, src []byte, format domain.SubtitleFormat, offsetMs int64) string {
	t.Helper()
	out, err := NewSubtitleConverter().Convert(src, format, offsetMs)
	if err != nil {
		t.Fatalf("Convert(%q) = %v", src, err)
	}
	return string(out)
}

func encodeWith(t *testing.T, text string, encode func(string) (string, error)) []byte {
	t.Helper()
	encoded, err := encode(text)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(encoded)
}

func TestConvertDetectsCharacterEncodings(t *testing.T) {
	utf16LE := xunicode.UTF16(xunicode.LittleEndian, xunicode.UseBOM).NewEncoder().String
	utf16BE := xunicode.UTF16(xunicode.BigEndian, xunicode.UseBOM).NewEncoder().String
	shiftJIS := japanese.ShiftJIS.NewEncoder().String
	inputs := map[string][]byte{
		"UTF-8":        []byte(japaneseSRT),
		"UTF-8 BOM":    append([]byte{0xEF, 0xBB, 0xBF}, japaneseSRT...),
		"UTF-16LE BOM": encodeWith(t, japaneseSRT, utf16LE),
		"UTF-16BE BOM": encodeWith(t, japaneseSRT, utf16BE),
		"Shift_JIS":    encodeWith(t, japaneseSRT, shiftJIS),
	}
	for name, src := range inputs {
		if got := convert(t, src, domain.SubtitleFormatSRT, 0); got != japaneseVTT {
			t.Errorf("%s: got %q, want %q", name, got, japaneseVTT)
		}
	}
}

func TestConvertTellsShiftJISFromUTF8(t *testing.T) {
	// E0 A1 A1 は UTF-8 としても妥当（U+0861）だが、字幕の文字体系に入らないので
	// Shift_JIS として読む。
	ambiguous := []byte("1\n00:00:01,000 --> 00:00:02,000\n\xE0\xA1\xA1\n")
	if got, want := convert(t, ambiguous, domain.SubtitleFormatSRT, 0), "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n爍｡\n"; got != want {
		t.Errorf("ambiguous Shift_JIS: got %q, want %q", got, want)
	}
	korean := []byte("1\n00:00:01,000 --> 00:00:02,000\n안녕하세요, 세계\n")
	if got, want := convert(t, korean, domain.SubtitleFormatSRT, 0), "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n안녕하세요, 세계\n"; got != want {
		t.Errorf("Korean UTF-8: got %q, want %q", got, want)
	}
	// 一覧に無い文字体系でも、Shift_JIS として読めない UTF-8 は UTF-8 のまま読む。
	georgian := []byte("1\n00:00:01,000 --> 00:00:02,000\nგამარჯობა\n")
	if got, want := convert(t, georgian, domain.SubtitleFormatSRT, 0), "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nგამარჯობა\n"; got != want {
		t.Errorf("Georgian UTF-8: got %q, want %q", got, want)
	}
	// F0 40 は Shift_JIS の復号器が U+FFFD を出す列で、UTF-8 としても妥当でない。
	broken := []byte("1\n00:00:01,000 --> 00:00:02,000\n\xF0\x40\n")
	if _, err := NewSubtitleConverter().Convert(broken, domain.SubtitleFormatSRT, 0); !errors.Is(err, domain.ErrSubtitleUnreadable) {
		t.Errorf("undecodable: err = %v, want ErrSubtitleUnreadable", err)
	}
	// 0x80 は Shift_JIS の復号器が U+0080 に通す。
	c1 := []byte("1\n00:00:01,000 --> 00:00:02,000\n\x80\n")
	if _, err := NewSubtitleConverter().Convert(c1, domain.SubtitleFormatSRT, 0); !errors.Is(err, domain.ErrSubtitleUnreadable) {
		t.Errorf("C1 control: err = %v, want ErrSubtitleUnreadable", err)
	}
}

func TestConvertNormalizesSRT(t *testing.T) {
	tests := []struct {
		name string
		srt  string
		want string
	}{
		{
			name: "comma and dot fractions",
			srt:  "1\n00:00:01,000 --> 00:00:02,000\none\n\n2\n00:00:03.000 --> 00:00:04.000\ntwo\n",
			want: "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\none\n\n00:00:03.000 --> 00:00:04.000\ntwo\n",
		},
		{
			name: "cue without a number line",
			srt:  "00:00:01,000 --> 00:00:02,000\none\n\n2\n00:00:03,000 --> 00:00:04,000\ntwo\n",
			want: "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\none\n\n00:00:03.000 --> 00:00:04.000\ntwo\n",
		},
		{
			name: "CRLF and CR line endings",
			srt:  "1\r\n00:00:01,000 --> 00:00:02,000\r\none\r\n\r\n2\r00:00:03,000 --> 00:00:04,000\rtwo\r",
			want: "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\none\n\n00:00:03.000 --> 00:00:04.000\ntwo\n",
		},
		{
			name: "short timestamps",
			srt:  "1\n0:01:02,5 --> 0:1:3,25\none\n\n2\n1:02:03 --> 01:02:04.1\ntwo\n",
			want: "WEBVTT\n\n00:01:02.500 --> 00:01:03.250\none\n\n01:02:03.000 --> 01:02:04.100\ntwo\n",
		},
		{
			name: "tags pass through",
			srt:  "1\n00:00:01,000 --> 00:00:02,000\n{\\an8}<i>italic</i> and <b>bold</b>\n<font color=\"#ff0000\">red</font>\n",
			want: "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n{\\an8}<i>italic</i> and <b>bold</b>\n<font color=\"#ff0000\">red</font>\n",
		},
		{
			name: "cues with unreadable timing are dropped",
			srt:  "1\n00:00:01,000 -> 00:00:02,000\nbroken\n\n2\n00:00:03,000 --> 00:00:04,000\ntwo\n\n3\nnot a time\nthree\n\n4\n00:61:00,000 --> 00:62:00,000\nfour\n",
			want: "WEBVTT\n\n00:00:03.000 --> 00:00:04.000\ntwo\n",
		},
		{
			name: "SRT coordinates are dropped",
			srt:  "1\n00:00:01,000 --> 00:00:02,000  X1:10 X2:20 Y1:30 Y2:40\none\n",
			want: "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\none\n",
		},
		{
			name: "overlapping, unordered and long cues are kept",
			srt:  "1\n10:00:00,000 --> 10:00:05,000\nlate\n\n2\n00:00:05,000 --> 00:00:09,000\nfirst\n\n3\n00:00:06,000 --> 00:00:07,000\noverlap\n",
			want: "WEBVTT\n\n10:00:00.000 --> 10:00:05.000\nlate\n\n00:00:05.000 --> 00:00:09.000\nfirst\n\n00:00:06.000 --> 00:00:07.000\noverlap\n",
		},
	}
	for _, tt := range tests {
		if got := convert(t, []byte(tt.srt), domain.SubtitleFormatSRT, 0); got != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.name, got, tt.want)
		}
	}
}

func TestConvertRejectsUnreadableSubtitles(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		format domain.SubtitleFormat
	}{
		{"empty SRT", "", domain.SubtitleFormatSRT},
		{"empty VTT", "", domain.SubtitleFormatVTT},
		{"SRT without cues", "just some text\nwith no timing\n", domain.SubtitleFormatSRT},
		{"SRT with only broken cues", "1\n00:00:01 -> 00:00:02\nx\n", domain.SubtitleFormatSRT},
		{"VTT without header", "00:00:01.000 --> 00:00:02.000\none\n", domain.SubtitleFormatVTT},
		{"VTT with a wrong header", "WEBVTTX\n\n00:00:01.000 --> 00:00:02.000\none\n", domain.SubtitleFormatVTT},
		{"unknown format", "WEBVTT\n", domain.SubtitleFormat("ass")},
	}
	for _, tt := range tests {
		if _, err := NewSubtitleConverter().Convert([]byte(tt.src), tt.format, 0); !errors.Is(err, domain.ErrSubtitleUnreadable) {
			t.Errorf("%s: err = %v, want ErrSubtitleUnreadable", tt.name, err)
		}
	}
}

func TestConvertPassesWebVTTThrough(t *testing.T) {
	vtt := "WEBVTT - title\r\nKind: captions\r\n\r\nNOTE a comment\r\n\r\nSTYLE\r\n::cue { color: yellow }\r\n\r\nintro\r\n00:01.000 --> 00:02.000 align:start line:10%\r\n<v Speaker>hello</v>\r\n"
	want := "WEBVTT - title\nKind: captions\n\nNOTE a comment\n\nSTYLE\n::cue { color: yellow }\n\nintro\n00:01.000 --> 00:02.000 align:start line:10%\n<v Speaker>hello</v>\n"
	if got := convert(t, []byte(vtt), domain.SubtitleFormatVTT, 0); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	bom := append([]byte{0xEF, 0xBB, 0xBF}, "WEBVTT\n"...)
	if got := convert(t, bom, domain.SubtitleFormatVTT, 0); got != "WEBVTT\n" {
		t.Errorf("BOM: got %q", got)
	}
}

func TestConvertShiftsCuesByOffset(t *testing.T) {
	srt := "1\n00:00:00,000 --> 00:00:05,000\ngone\n\n2\n00:00:03,000 --> 00:00:10,000\nclipped\n\n3\n00:00:10,000 --> 00:00:12,000\nshifted\n\n4\n00:00:06,000 --> 00:00:08,000\nends at zero\n"
	want := "WEBVTT\n\n00:00:00.000 --> 00:00:02.000\nclipped\n\n00:00:02.000 --> 00:00:04.000\nshifted\n"
	if got := convert(t, []byte(srt), domain.SubtitleFormatSRT, 8000); got != want {
		t.Errorf("SRT:\n got %q\nwant %q", got, want)
	}

	vtt := "WEBVTT\n\nNOTE kept\n\nSTYLE\n::cue { color: yellow }\n\ngone\n00:00:00.000 --> 00:00:05.000\ngone\n\nclipped\n00:00:03.000 --> 00:00:10.000 align:start\nclipped\n\n00:10.000 --> 00:12.000\nshifted\n"
	wantVTT := "WEBVTT\n\nNOTE kept\n\nSTYLE\n::cue { color: yellow }\n\nclipped\n00:00:00.000 --> 00:00:02.000 align:start\nclipped\n\n00:00:02.000 --> 00:00:04.000\nshifted\n"
	if got := convert(t, []byte(vtt), domain.SubtitleFormatVTT, 8000); got != wantVTT {
		t.Errorf("VTT:\n got %q\nwant %q", got, wantVTT)
	}

	// どの cue も消えるほどずらしても、読めない字幕ではない。
	if got := convert(t, []byte(srt), domain.SubtitleFormatSRT, 60_000); got != "WEBVTT\n" {
		t.Errorf("all cues shifted out: got %q", got)
	}
	if !strings.HasPrefix(convert(t, []byte(srt), domain.SubtitleFormatSRT, 0), "WEBVTT\n\n00:00:00.000 --> 00:00:05.000\ngone\n") {
		t.Error("offset 0 must keep every cue")
	}
}
