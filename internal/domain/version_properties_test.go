package domain

import "testing"

func chapters(titles ...string) []MediaChapter {
	var list []MediaChapter
	for i, title := range titles {
		list = append(list, MediaChapter{StartMs: int64(i) * 300_000, Title: title})
	}
	return list
}

func TestLeanVersionProperties(t *testing.T) {
	for _, c := range []struct {
		name string
		a, b VersionProperties
		want VersionLeaning
	}{
		{"名前つきチャプターが同じ時刻", VersionProperties{Chapters: chapters("Opening", "Part A", "Ending")},
			VersionProperties{Chapters: chapters("Opening", "Part A", "Ending")}, VersionLeansStronglySame},
		{"番号だけのチャプターが同じ時刻", VersionProperties{Chapters: chapters("Chapter 1", "Chapter 2", "Chapter 3")},
			VersionProperties{Chapters: chapters("1", "2", "3")}, VersionLeansSame},
		{"番号違いだけの名前", VersionProperties{Chapters: chapters("チャプター 1", "チャプター 2", "チャプター 3")},
			VersionProperties{Chapters: chapters("チャプター 1", "チャプター 2", "チャプター 3")}, VersionLeansSame},
		{"チャプターの数が違う", VersionProperties{Chapters: chapters("A", "B", "C")},
			VersionProperties{Chapters: chapters("A", "B", "C", "D")}, VersionLeansNowhere},
		{"録画日時が一致", VersionProperties{RecordedAt: "2024-01-01T00:00:00Z"},
			VersionProperties{RecordedAt: "2024-01-01T00:00:00Z"}, VersionLeansStronglySame},
		// Re-encoding often rewrites creation_time, so a difference says nothing.
		{"録画日時が違う", VersionProperties{RecordedAt: "2024-01-01T00:00:00Z"},
			VersionProperties{RecordedAt: "2025-01-01T00:00:00Z"}, VersionLeansNowhere},
		{"埋め込みタイトルが一致", VersionProperties{EmbeddedTitle: "Summer Trip"},
			VersionProperties{EmbeddedTitle: "summer trip"}, VersionLeansSame},
		{"決まり文句のタイトル", VersionProperties{EmbeddedTitle: "Untitled"},
			VersionProperties{EmbeddedTitle: "Untitled"}, VersionLeansNowhere},
		{"長さだけ", VersionProperties{DurationMs: 60_000}, VersionProperties{DurationMs: 60_400}, VersionLeansNowhere},
	} {
		if got := LeanVersionProperties(CompareVersionProperties(c.a, c.b)); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSettleVersionVerdict(t *testing.T) {
	unsure, low := 0.55, 0.3
	for _, c := range []struct {
		name         string
		verdict      VersionVerdict
		leaning      VersionLeaning
		probability  *float64
		want         VersionVerdict
		byProperties bool
	}{
		{"AI は別物、情報は強く同じ", VersionDifferent, VersionLeansStronglySame, &low, VersionUndecided, true},
		{"決めかねた、情報は強く同じ", VersionUndecided, VersionLeansStronglySame, &unsure, VersionSame, true},
		{"接続待ち、情報は強く同じ", VersionNoConnection, VersionLeansStronglySame, nil, VersionSame, true},
		{"弱い同じで AI が同じ寄り", VersionUndecided, VersionLeansSame, &unsure, VersionSame, true},
		{"弱い同じで AI が別物寄り", VersionUndecided, VersionLeansSame, &low, VersionUndecided, false},
		{"コマで同じ", VersionSame, VersionLeansNowhere, nil, VersionSame, false},
	} {
		got, byProperties := SettleVersionVerdict(c.verdict, c.leaning, c.probability)
		if got != c.want || byProperties != c.byProperties {
			t.Errorf("%s: %v（%v）, want %v（%v）", c.name, got, byProperties, c.want, c.byProperties)
		}
	}
}
