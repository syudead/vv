package domain

import "testing"

func TestCleanVersionTitleDropsEncodeWordsButKeepsEpisodes(t *testing.T) {
	if a, b := CleanVersionTitle("Movie [1080p]"), CleanVersionTitle("movie.720p.x265"); a != b || a == "" {
		t.Errorf("同じ作品の版 = %q, %q, want 同じ", a, b)
	}
	if a, b := CleanVersionTitle("Show 01 (BD)"), CleanVersionTitle("Show 02 (BD)"); a == b {
		t.Errorf("別の話数 = %q, %q, want 違う", a, b)
	}
}

func TestLeanVersionFacts(t *testing.T) {
	for _, c := range []struct {
		name string
		a, b VersionFacts
		want VersionLeaning
	}{
		{"作品コードが一致", VersionFacts{WorkCode: "abc-1"}, VersionFacts{WorkCode: "abc-1"}, VersionLeansStronglySame},
		{"同じ出どころの外部 ID が違う",
			VersionFacts{ExternalIDs: []ExternalID{{Source: "db", Value: "1"}}},
			VersionFacts{ExternalIDs: []ExternalID{{Source: "db", Value: "2"}}}, VersionLeansStronglyDifferent},
		{"強い手がかりが食い違う",
			VersionFacts{WorkCode: "abc-1", RecordedAt: "2024-01-01"},
			VersionFacts{WorkCode: "abc-1", RecordedAt: "2025-01-01"}, VersionLeansNowhere},
		{"ファイル名だけ一致", VersionFacts{FileTitle: "Movie 1080p"}, VersionFacts{FileTitle: "movie_720p"}, VersionLeansSame},
		{"音声の言語だけ違う",
			VersionFacts{AudioLanguages: []string{"jpn"}}, VersionFacts{AudioLanguages: []string{"eng"}}, VersionLeansNowhere},
	} {
		if got := LeanVersionFacts(CompareVersionFacts(c.a, c.b)); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSettleVersionVerdict(t *testing.T) {
	unsure, low := 0.55, 0.3
	for _, c := range []struct {
		name        string
		verdict     VersionVerdict
		leaning     VersionLeaning
		probability *float64
		want        VersionVerdict
		byFacts     bool
	}{
		{"コマは同じ、事実は別物", VersionSame, VersionLeansStronglyDifferent, nil, VersionUndecided, true},
		{"決めかねた、事実は同じ", VersionUndecided, VersionLeansStronglySame, &unsure, VersionSame, true},
		{"接続待ち、事実は別物", VersionNoConnection, VersionLeansStronglyDifferent, nil, VersionDifferent, true},
		{"ファイル名だけ一致で AI が同じ寄り", VersionUndecided, VersionLeansSame, &unsure, VersionSame, true},
		{"ファイル名だけ一致で AI が別物寄り", VersionUndecided, VersionLeansSame, &low, VersionUndecided, false},
		{"事実なし", VersionDifferent, VersionLeansNowhere, &low, VersionDifferent, false},
	} {
		got, byFacts := SettleVersionVerdict(c.verdict, c.leaning, c.probability)
		if got != c.want || byFacts != c.byFacts {
			t.Errorf("%s: %v（%v）, want %v（%v）", c.name, got, byFacts, c.want, c.byFacts)
		}
	}
}
