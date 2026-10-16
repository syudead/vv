package domain

import (
	"strings"
	"testing"
)

func TestClassifyHDR(t *testing.T) {
	tests := []struct {
		name          string
		colorTransfer string
		codecTag      string
		dolbyVision   bool
		want          HDRKind
	}{
		{"PQ は HDR10", "smpte2084", "hvc1", false, HDRKindHDR10},
		{"HLG", "arib-std-b67", "", false, HDRKindHLG},
		{"DOVI の構成記録は PQ より先", "smpte2084", "hvc1", true, HDRKindDolbyVision},
		{"Dolby Vision の codec tag", "", "dvh1", false, HDRKindDolbyVision},
		{"BT.709 は SDR", "bt709", "avc1", false, HDRKindSDR},
		{"色の情報が無ければ SDR", "", "", false, HDRKindSDR},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyHDR(tc.colorTransfer, tc.codecTag, tc.dolbyVision); got != tc.want {
				t.Errorf("ClassifyHDR = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInterlacedFromFieldOrder(t *testing.T) {
	for _, order := range []string{"tt", "bb", "tb", "bt"} {
		if got := InterlacedFromFieldOrder(order); got == nil || !*got {
			t.Errorf("%s: got %v, want interlaced", order, got)
		}
	}
	if got := InterlacedFromFieldOrder("progressive"); got == nil || *got {
		t.Errorf("progressive: got %v, want false", got)
	}
	for _, order := range []string{"unknown", ""} {
		if got := InterlacedFromFieldOrder(order); got != nil {
			t.Errorf("%q: got %v, want nil", order, *got)
		}
	}
}

func TestVideoBitDepth(t *testing.T) {
	tests := []struct {
		raw    int
		format string
		want   int
	}{
		{8, "yuv420p10le", 8},
		{0, "yuv420p10le", 10},
		{0, "p010le", 10},
		{0, "yuv444p12be", 12},
		{0, "gray10le", 10},
		{0, "yuv420p", 0},
		{0, "rgb48le", 0},
		{0, "", 0},
	}
	for _, tc := range tests {
		if got := VideoBitDepth(tc.raw, tc.format); got != tc.want {
			t.Errorf("VideoBitDepth(%d, %q) = %d, want %d", tc.raw, tc.format, got, tc.want)
		}
	}
}

func TestCleanMediaText(t *testing.T) {
	if got := CleanMediaText("  題\x00名\x1b\n二行目\t \xff "); got != "題名\n二行目" {
		t.Errorf("got %q", got)
	}
	if got := CleanMediaText(" \x01 "); got != "" {
		t.Errorf("制御文字だけの値 = %q, want empty", got)
	}
	long := strings.Repeat("あ", MediaTextMaxRunes+10)
	if got := CleanMediaText(long); len([]rune(got)) != MediaTextMaxRunes {
		t.Errorf("len = %d, want %d", len([]rune(got)), MediaTextMaxRunes)
	}
	if got := CleanMediaLanguage("und"); got != "" {
		t.Errorf("und = %q", got)
	}
	if got := CleanMediaLanguage(" jpn "); got != "jpn" {
		t.Errorf("jpn = %q", got)
	}
}

func TestMediaDevice(t *testing.T) {
	if got := MediaDevice([2]string{"Apple", "iPhone 15 Pro"}, [2]string{"Google", "Pixel"}); got != "Apple iPhone 15 Pro" {
		t.Errorf("got %q", got)
	}
	if got := MediaDevice([2]string{"", " "}, [2]string{"Google", "Pixel 8"}); got != "Google Pixel 8" {
		t.Errorf("got %q", got)
	}
	if got := MediaDevice([2]string{"", ""}); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestParseRecordedAt(t *testing.T) {
	tests := []struct {
		name       string
		candidates []string
		want       string
	}{
		{"RFC 3339 のオフセットを保つ", []string{"2024-05-01T12:34:56+09:00"}, "2024-05-01T12:34:56+09:00"},
		{"Apple のコロンの無いオフセット", []string{"2024-05-01T12:34:56+0900"}, "2024-05-01T12:34:56+09:00"},
		{"小数秒つきの Z", []string{"2024-05-01T03:34:56.000000Z"}, "2024-05-01T03:34:56Z"},
		{"オフセットの無い値は UTC", []string{"2024-05-01T12:34:56"}, "2024-05-01T12:34:56Z"},
		{"空白区切り", []string{"2024-05-01 12:34:56"}, "2024-05-01T12:34:56Z"},
		{"読めない値は次の候補", []string{"yesterday", "2024-05-01 12:34:56"}, "2024-05-01T12:34:56Z"},
		{"どれも読めない", []string{"2024", ""}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseRecordedAt(tc.candidates...); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseISO6709(t *testing.T) {
	got := ParseISO6709("+35.6586+139.7454+040.123/")
	if got == nil || got.Latitude != 35.6586 || got.Longitude != 139.7454 || got.Altitude == nil || *got.Altitude != 40.123 {
		t.Fatalf("高度つき = %+v", got)
	}
	got = ParseISO6709("-33.8688+151.2093/")
	if got == nil || got.Latitude != -33.8688 || got.Longitude != 151.2093 || got.Altitude != nil {
		t.Fatalf("高度なし = %+v", got)
	}
	// 度分（4 桁と 5 桁）。
	got = ParseISO6709("+3530.0+13945.0/")
	if got == nil || got.Latitude != 35.5 || got.Longitude != 139.75 {
		t.Fatalf("度分 = %+v", got)
	}
	// 度分秒（6 桁と 7 桁）。
	got = ParseISO6709("+353000+1394500/")
	if got == nil || got.Latitude != 35.5 || got.Longitude != 139.75 {
		t.Fatalf("度分秒 = %+v", got)
	}
	for _, bad := range []string{"+95.0+139.0/", "+35.0+190.0/", "+3.5+139.0/", "35.0,139.0", ""} {
		if got := ParseISO6709(bad); got != nil {
			t.Errorf("%q: got %+v, want nil", bad, got)
		}
	}
	if got := ParseISO6709("bogus", "+35.0+139.0/"); got == nil || got.Latitude != 35 {
		t.Errorf("次の候補 = %+v", got)
	}
}

func TestNormalizeChapters(t *testing.T) {
	chapters := []MediaChapter{
		{StartMs: 60_000, EndMs: 130_000, Title: " 後半 "},
		{StartMs: 0, EndMs: 60_000, Title: "前半"},
		{StartMs: 120_000, EndMs: 150_000, Title: "尺の後"},
		{StartMs: 30_000, EndMs: 20_000},
	}
	got := NormalizeChapters(chapters, 120_000)
	want := []MediaChapter{
		{StartMs: 0, EndMs: 60_000, Title: "前半"},
		{StartMs: 30_000, EndMs: 30_000},
		{StartMs: 60_000, EndMs: 120_000, Title: "後半"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// 1 つだけでも残る。
	if got := NormalizeChapters([]MediaChapter{{StartMs: 0, EndMs: 10_000}}, 10_000); len(got) != 1 {
		t.Errorf("1 つ = %+v", got)
	}
	// 重なりはそのまま。
	overlapping := NormalizeChapters([]MediaChapter{{StartMs: 0, EndMs: 8_000}, {StartMs: 5_000, EndMs: 9_000}}, 10_000)
	if len(overlapping) != 2 || overlapping[0].EndMs != 8_000 {
		t.Errorf("重なり = %+v", overlapping)
	}

	many := make([]MediaChapter, MediaChaptersMaxCount+5)
	for i := range many {
		many[i] = MediaChapter{StartMs: int64(i), EndMs: int64(i + 1)}
	}
	if got := NormalizeChapters(many, 1_000_000); len(got) != MediaChaptersMaxCount {
		t.Errorf("len = %d, want %d", len(got), MediaChaptersMaxCount)
	}
}

func TestLimitMediaTracks(t *testing.T) {
	tracks := make([]MediaAudioTrack, MediaTracksMaxCount+1)
	if got := LimitMediaTracks(tracks); len(got) != MediaTracksMaxCount {
		t.Errorf("len = %d", len(got))
	}
}
