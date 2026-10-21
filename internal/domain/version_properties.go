package domain

import (
	"regexp"
	"strconv"
	"strings"
)

// Properties read from the files themselves at import add evidence beside the frames; they only ever say same, since
// a version may differ in every other property. See docs/design-docs/duplicate-judging.md.

// VersionProperties are one side's stored properties; empty fields are unknown.
type VersionProperties struct {
	DurationMs    int64
	Chapters      []MediaChapter
	EmbeddedTitle string
	RecordedAt    string
}

type VersionPropertyKind string

const (
	VersionPropertyChapters      VersionPropertyKind = "chapters"
	VersionPropertyRecordedAt    VersionPropertyKind = "recorded_at"
	VersionPropertyEmbeddedTitle VersionPropertyKind = "embedded_title"
	VersionPropertyDuration      VersionPropertyKind = "duration"
)

// VersionPropertySignal compares one property both sides have. Strong signals decide a pair on their own; weak ones
// only tip an unsure classifier answer, and Counts is false for a property that is shown only.
type VersionPropertySignal struct {
	Kind   VersionPropertyKind `json:"kind"`
	Same   bool                `json:"same"`
	Strong bool                `json:"strong,omitempty"`
	Counts bool                `json:"counts,omitempty"`
	A      string              `json:"a"`
	B      string              `json:"b"`
}

// VersionLeaning is what the properties together say.
type VersionLeaning int

const (
	VersionLeansNowhere VersionLeaning = iota
	VersionLeansSame
	VersionLeansStronglySame
)

const (
	// Chapter lists need this many entries before they say anything.
	versionMinChapters = 3
	// Chapter starts within this many milliseconds are the same chapter.
	versionChapterToleranceMs = 1000
)

var (
	// Titles an encoder writes on its own say nothing about the content.
	genericVersionTitle = regexp.MustCompile(`^(?:untitled|no title|video|movie|title|clip|\d+)$`)
	versionTitleDigits  = regexp.MustCompile(`[0-9\s]+`)
)

// CompareVersionProperties lists the properties both sides have.
func CompareVersionProperties(a, b VersionProperties) []VersionPropertySignal {
	var signals []VersionPropertySignal
	if signal, ok := chapterSignal(a.Chapters, b.Chapters); ok {
		signals = append(signals, signal)
	}
	if a.RecordedAt != "" && b.RecordedAt != "" {
		// Many encoders rewrite creation_time to the encode time, so only an equal time says anything.
		same := a.RecordedAt == b.RecordedAt
		signals = append(signals, VersionPropertySignal{Kind: VersionPropertyRecordedAt, Same: same, Strong: same,
			Counts: same, A: a.RecordedAt, B: b.RecordedAt})
	}
	titleA, titleB := meaningfulTitle(a.EmbeddedTitle), meaningfulTitle(b.EmbeddedTitle)
	if titleA != "" && titleB != "" {
		same := titleA == titleB
		signals = append(signals, VersionPropertySignal{Kind: VersionPropertyEmbeddedTitle, Same: same, Counts: same,
			A: a.EmbeddedTitle, B: b.EmbeddedTitle})
	}
	if a.DurationMs > 0 && b.DurationMs > 0 {
		// Candidates already have close durations, so the difference is shown but never counted.
		signals = append(signals, VersionPropertySignal{Kind: VersionPropertyDuration,
			Same: abs64(a.DurationMs-b.DurationMs) <= versionChapterToleranceMs,
			A:    strconv.FormatInt(a.DurationMs, 10), B: strconv.FormatInt(b.DurationMs, 10)})
	}
	return signals
}

// Equal chapter starts with a meaningful equal title are strong; equal starts alone are weak, since a tool may place
// chapters every few minutes in any video.
func chapterSignal(a, b []MediaChapter) (VersionPropertySignal, bool) {
	if len(a) < versionMinChapters || len(b) < versionMinChapters {
		return VersionPropertySignal{}, false
	}
	signal := VersionPropertySignal{Kind: VersionPropertyChapters,
		A: strconv.Itoa(len(a)), B: strconv.Itoa(len(b))}
	if len(a) != len(b) {
		return signal, true
	}
	titled := false
	for i := range a {
		if abs64(a[i].StartMs-b[i].StartMs) > versionChapterToleranceMs {
			return signal, true
		}
		title := meaningfulTitle(a[i].Title)
		if title != "" && title == meaningfulTitle(b[i].Title) {
			titled = true
		}
	}
	signal.Same, signal.Counts, signal.Strong = true, true, titled && !numberedChapters(a)
	return signal, true
}

// numberedChapters reports titles that differ only by their numbers, as a chapter tool writes them in any language.
func numberedChapters(chapters []MediaChapter) bool {
	first := versionTitleDigits.ReplaceAllString(FoldForMatch(chapters[0].Title), "")
	for _, chapter := range chapters[1:] {
		if versionTitleDigits.ReplaceAllString(FoldForMatch(chapter.Title), "") != first {
			return false
		}
	}
	return true
}

func meaningfulTitle(title string) string {
	folded := strings.TrimSpace(FoldForMatch(title))
	if genericVersionTitle.MatchString(folded) {
		return ""
	}
	return folded
}

// LeanVersionProperties is strongly same with any counted strong signal, same with a counted weak one.
func LeanVersionProperties(signals []VersionPropertySignal) VersionLeaning {
	leaning := VersionLeansNowhere
	for _, signal := range signals {
		switch {
		case signal.Counts && signal.Strong:
			return VersionLeansStronglySame
		case signal.Counts:
			leaning = VersionLeansSame
		}
	}
	return leaning
}

// SettleVersionVerdict lets a strong same decide a pair the frames and the classifier left open, holds a different
// answer it contradicts for the owner, and lets a weak same decide an open pair the classifier leaned same on;
// byProperties reports that the properties changed the verdict.
func SettleVersionVerdict(
	verdict VersionVerdict, leaning VersionLeaning, probability *float64,
) (settled VersionVerdict, byProperties bool) {
	switch verdict {
	case VersionDifferent:
		if leaning == VersionLeansStronglySame {
			return VersionUndecided, true
		}
	case VersionUndecided, VersionNoConnection, VersionFailed:
		switch {
		case leaning == VersionLeansStronglySame:
			return VersionSame, true
		case leaning == VersionLeansSame && probability != nil && *probability >= 0.5:
			return VersionSame, true
		}
	}
	return verdict, false
}
