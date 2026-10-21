package domain

import (
	"regexp"
	"slices"
	"strings"
)

// Stored facts beside the frames tell two encodes of one video apart from two different videos; see
// docs/design-docs/duplicate-judging.md.

// VersionFacts are one side's stored facts; empty fields are unknown.
type VersionFacts struct {
	FileTitle string
	// WorkCode is folded with FoldForMatch.
	WorkCode       string
	ExternalIDs    []ExternalID
	EmbeddedTitle  string
	RecordedAt     string
	Program        string
	AudioLanguages []string
}

type VersionFactKind string

const (
	VersionFactWorkCode      VersionFactKind = "work_code"
	VersionFactExternalID    VersionFactKind = "external_id"
	VersionFactRecordedAt    VersionFactKind = "recorded_at"
	VersionFactTitle         VersionFactKind = "title"
	VersionFactEmbeddedTitle VersionFactKind = "embedded_title"
	VersionFactProgram       VersionFactKind = "program"
	VersionFactAudioLanguage VersionFactKind = "audio_language"
)

// VersionFactMatch says what one fact says about the pair.
type VersionFactMatch string

const (
	VersionFactSame      VersionFactMatch = "same"
	VersionFactDifferent VersionFactMatch = "different"
)

// VersionFactSignal is one fact both sides have; Strong facts can decide a pair on their own.
type VersionFactSignal struct {
	Kind   VersionFactKind  `json:"kind"`
	Match  VersionFactMatch `json:"match"`
	Strong bool             `json:"strong,omitempty"`
	A      string           `json:"a"`
	B      string           `json:"b"`
}

// VersionLeaning is what the facts together say.
type VersionLeaning int

const (
	VersionLeansNowhere VersionLeaning = iota
	// VersionLeansSame has only a weak same signal, a cleaned title match.
	VersionLeansSame
	VersionLeansStronglySame
	VersionLeansStronglyDifferent
)

// CompareVersionFacts lists the facts both sides have; an audio language difference is a note, since a dub is
// still a version of the same video.
func CompareVersionFacts(a, b VersionFacts) []VersionFactSignal {
	var signals []VersionFactSignal
	add := func(kind VersionFactKind, strong bool, x, y string, equal bool) {
		if x == "" || y == "" {
			return
		}
		match := VersionFactDifferent
		if equal {
			match = VersionFactSame
		}
		signals = append(signals, VersionFactSignal{Kind: kind, Match: match, Strong: strong, A: x, B: y})
	}
	add(VersionFactWorkCode, true, a.WorkCode, b.WorkCode, a.WorkCode == b.WorkCode)
	signals = append(signals, externalIDSignals(a.ExternalIDs, b.ExternalIDs)...)
	// Many encoders rewrite creation_time to the encode time, so only an equal recording time counts.
	recordedEqual := a.RecordedAt == b.RecordedAt
	add(VersionFactRecordedAt, recordedEqual, a.RecordedAt, b.RecordedAt, recordedEqual)
	titleA, titleB := CleanVersionTitle(a.FileTitle), CleanVersionTitle(b.FileTitle)
	if titleA != "" && titleB != "" {
		add(VersionFactTitle, false, a.FileTitle, b.FileTitle, titleA == titleB)
	}
	add(VersionFactEmbeddedTitle, false, a.EmbeddedTitle, b.EmbeddedTitle,
		FoldForMatch(a.EmbeddedTitle) == FoldForMatch(b.EmbeddedTitle))
	add(VersionFactProgram, false, a.Program, b.Program, a.Program == b.Program)
	languagesA, languagesB := languageList(a.AudioLanguages), languageList(b.AudioLanguages)
	add(VersionFactAudioLanguage, false, languagesA, languagesB, languagesA == languagesB)
	return signals
}

// A shared source with different values marks different works only when no shared source agrees.
func externalIDSignals(a, b []ExternalID) []VersionFactSignal {
	var same, different []VersionFactSignal
	for _, x := range a {
		for _, y := range b {
			if x.Source != y.Source {
				continue
			}
			signal := VersionFactSignal{Kind: VersionFactExternalID, Strong: true,
				A: x.Source + ":" + x.Value, B: y.Source + ":" + y.Value}
			if x.Value == y.Value {
				signal.Match = VersionFactSame
				same = append(same, signal)
			} else {
				signal.Match = VersionFactDifferent
				different = append(different, signal)
			}
		}
	}
	if len(same) > 0 {
		return same[:1]
	}
	if len(different) > 0 {
		return different[:1]
	}
	return nil
}

func languageList(languages []string) string {
	list := slices.Clone(languages)
	slices.Sort(list)
	return strings.Join(slices.Compact(list), ",")
}

// LeanVersionFacts lets strong facts decide only when they agree, and a weak title match lean same.
func LeanVersionFacts(signals []VersionFactSignal) VersionLeaning {
	var same, different, weakSame bool
	for _, signal := range signals {
		switch {
		case signal.Strong && signal.Match == VersionFactSame:
			same = true
		case signal.Strong && signal.Match == VersionFactDifferent:
			different = true
		case signal.Kind == VersionFactTitle && signal.Match == VersionFactSame:
			weakSame = true
		}
	}
	switch {
	case same && different:
		return VersionLeansNowhere
	case same:
		return VersionLeansStronglySame
	case different:
		return VersionLeansStronglyDifferent
	case weakSame:
		return VersionLeansSame
	default:
		return VersionLeansNowhere
	}
}

var (
	versionTitleBrackets = regexp.MustCompile(`[\[(（【〔{][^\])）】〕}]*[\])）】〕}]`)
	// Words that name an encode rather than the video: resolution, codec, source, audio and quality tags.
	versionTitleEncodeWords = regexp.MustCompile(`\b(?:\d{3,4}p|[248]k|uhd|fhd|hd|sd|x26[45]|h\.?26[45]|hevc|avc|av1|` +
		`vp9|aac|ac3|eac3|dts|flac|opus|mp3|truehd|atmos|blu-?ray|bd(?:rip|mv)?|dvd(?:rip)?|web-?(?:dl|rip)?|` +
		`remux|hdr10?\+?|dv|sdr|10bit|8bit|\d{2,3}fps|proper|repack|encode|re-?encoded?|copy|backup)\b`)
	versionTitleSeparators = regexp.MustCompile(`[\s._\-–—~・]+`)
)

// CleanVersionTitle folds a file title and drops bracketed parts, encode words and separators, so "Movie [1080p]"
// and "movie.720p.x265" compare equal while episode numbers stay.
func CleanVersionTitle(title string) string {
	folded := FoldForMatch(title)
	folded = versionTitleBrackets.ReplaceAllString(folded, " ")
	// Underscores are word characters to \b, so "movie_720p" would hide the resolution word.
	folded = strings.ReplaceAll(folded, "_", " ")
	folded = versionTitleEncodeWords.ReplaceAllString(folded, " ")
	folded = versionTitleSeparators.ReplaceAllString(folded, " ")
	return strings.Join(strings.Fields(folded), "")
}

// SettleVersionVerdict lets agreeing strong facts decide a pair the frames and the classifier left open, and holds a
// frames-or-classifier verdict that strong facts contradict for the owner; byFacts reports that the facts changed it.
// A weak title match decides only an open pair the classifier leaned same on.
func SettleVersionVerdict(
	verdict VersionVerdict, leaning VersionLeaning, probability *float64,
) (settled VersionVerdict, byFacts bool) {
	switch verdict {
	case VersionSame:
		if leaning == VersionLeansStronglyDifferent {
			return VersionUndecided, true
		}
	case VersionDifferent:
		if leaning == VersionLeansStronglySame {
			return VersionUndecided, true
		}
	case VersionUndecided, VersionNoConnection, VersionFailed:
		switch {
		case leaning == VersionLeansStronglySame:
			return VersionSame, true
		case leaning == VersionLeansStronglyDifferent:
			return VersionDifferent, true
		case leaning == VersionLeansSame && probability != nil && *probability >= 0.5:
			return VersionSame, true
		}
	}
	return verdict, false
}
