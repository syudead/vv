package domain

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
)

// Automatic tagging scores existing tags through ClassifierRequest and never creates tags; see docs/design-docs/auto-tagging.md.

const (
	// The default threshold favors avoiding false positives.
	DefaultAutoTagThreshold = 0.8
	// Scores between the tentative and confirmed thresholds require owner confirmation.
	DefaultAutoTagTentativeThreshold = 0.5
)

var ErrInvalidAutoTagSettings = errors.New("invalid auto-tagging settings")

var ErrAutoTaggingOff = errors.New("auto-tagging is turned off")

// AutoTagSettings excludes provider connection settings, which belong to the provider adapter.
type AutoTagSettings struct {
	// Disabled tagging neither starts nor dequeues work and never contacts the classifier; disabled by default.
	Enabled bool
	// AfterImport applies after seek-preview generation and only while Enabled.
	AfterImport bool
	// Threshold must be in (0, 1].
	Threshold float64
	// TentativeThreshold must be in (0, Threshold]; equality disables tentative assignments.
	TentativeThreshold float64
}

// DefaultAutoTagSettings disables tagging and automatic classification.
func DefaultAutoTagSettings() AutoTagSettings {
	return AutoTagSettings{
		Threshold:          DefaultAutoTagThreshold,
		TentativeThreshold: DefaultAutoTagTentativeThreshold,
	}
}

func ValidateAutoTagSettings(s AutoTagSettings) (AutoTagSettings, error) {
	if math.IsNaN(s.Threshold) || s.Threshold <= 0 || s.Threshold > 1 {
		return AutoTagSettings{}, fmt.Errorf("%w: the threshold must be greater than 0 and at most 1", ErrInvalidAutoTagSettings)
	}
	if math.IsNaN(s.TentativeThreshold) || s.TentativeThreshold <= 0 || s.TentativeThreshold > s.Threshold {
		return AutoTagSettings{}, fmt.Errorf("%w: the tentative threshold must be greater than 0 and at most the threshold", ErrInvalidAutoTagSettings)
	}
	return s, nil
}

type AutoTagScope string

const (
	// AutoTagScopeMissing includes unclassified and failed videos.
	AutoTagScopeMissing AutoTagScope = "missing"
	AutoTagScopeAll     AutoTagScope = "all"
)

func (s AutoTagScope) Valid() bool {
	return s == AutoTagScopeMissing || s == AutoTagScopeAll
}

type AutoTagQueueMode int

const (
	// AutoTagQueueIfNew avoids reassigning owner-removed tags after import.
	AutoTagQueueIfNew AutoTagQueueMode = iota
	AutoTagQueueUnlessDone
	// AutoTagQueueAgain excludes jobs already running.
	AutoTagQueueAgain
)

func (s AutoTagScope) QueueMode() AutoTagQueueMode {
	if s == AutoTagScopeAll {
		return AutoTagQueueAgain
	}
	return AutoTagQueueUnlessDone
}

type AutoTagCounts = QueueCounts

type AutoTagStatus struct {
	Settings AutoTagSettings
	Counts   AutoTagCounts
}

// AutoTagCandidate excludes tentative tags and tags already confirmed or rejected for this video.
type AutoTagCandidate struct {
	ID   int64
	Name string
	// Synonyms are sorted by name and included in the classifier description.
	Synonyms []string
}

// SortAutoTagCandidates uses the same natural name order as SortTagRefs.
func SortAutoTagCandidates(candidates []AutoTagCandidate) {
	slices.SortStableFunc(candidates, func(a, b AutoTagCandidate) int {
		return compareTagRefs(TagRef{ID: a.ID, Name: a.Name}, TagRef{ID: b.ID, Name: b.Name})
	})
}

// Seven spread frames plus the thumbnail fit Ollama's eight-image limit; frames are at most 320px on the long edge.
const AutoTagFrameCount = 7

// SpreadFrameIndexes selects ascending interval midpoints, or every frame when frameCount <= n.
func SpreadFrameIndexes(frameCount, n int) []int {
	if frameCount <= 0 || n <= 0 {
		return nil
	}
	if frameCount <= n {
		indexes := make([]int, frameCount)
		for i := range indexes {
			indexes[i] = i
		}
		return indexes
	}
	indexes := make([]int, n)
	for i := range indexes {
		indexes[i] = (2*i + 1) * frameCount / (2 * n)
	}
	return indexes
}

type AutoTagVideo struct {
	Video Video
	// MediaFolder is the deepest registered folder containing Video.Path.
	MediaFolder string
}

type AutoTagSubject struct {
	// Title uses the display-name override when present.
	Title string
	// FileName includes the extension.
	FileName string
	// Folder is relative to the registered folder with / separators; omit ancestor paths because they are environment-specific.
	Folder string
	// Images contains the thumbnail followed by spread preview frames as JPEGs; empty means text-only classification.
	Images [][]byte
}

func (s AutoTagSubject) ClassifierInput() ClassifierInput {
	return ClassifierInput{
		Facts:  map[string]string{"title": s.Title, "file_name": s.FileName, "folder": s.Folder},
		Images: s.Images,
	}
}

// NewAutoTagSubject uses OS path rules and suppresses absolute paths if the video is outside mediaFolder.
func NewAutoTagSubject(title, mediaFolder, path string) AutoTagSubject {
	subject := AutoTagSubject{Title: title, FileName: filepath.Base(path)}
	if !PathInsideRoot(mediaFolder, path) {
		return subject
	}
	rel, err := filepath.Rel(filepath.Clean(mediaFolder), filepath.Clean(path))
	if err != nil {
		return subject
	}
	if dir := filepath.Dir(rel); dir != "." {
		subject.Folder = filepath.ToSlash(dir)
	}
	return subject
}

func AutoTagQuestionKey(tagID int64) string {
	return fmt.Sprintf("tag_%d", tagID)
}

func AutoTagQuestionFor(candidate AutoTagCandidate) ClassifierQuestion {
	var b strings.Builder
	// Ask whether a tag applies anywhere in the video; whole-video phrasing lowers scores for partial matches (docs/design-docs/auto-tagging.md).
	fmt.Fprintf(&b, "Does %q appear in or apply to any part of this video?", candidate.Name)
	if len(candidate.Synonyms) > 0 {
		quoted := make([]string, len(candidate.Synonyms))
		for i, synonym := range candidate.Synonyms {
			quoted[i] = fmt.Sprintf("%q", synonym)
		}
		fmt.Fprintf(&b, " The tag is also known as %s.", strings.Join(quoted, ", "))
	}
	b.WriteString(" The images are the thumbnail and frames taken from across the video; a tag applies if it fits any of them, or the title, file name or folder.")
	return ClassifierQuestion{Key: AutoTagQuestionKey(candidate.ID), Text: b.String()}
}

type AutoTagSelection struct {
	Confirmed []int64
	Tentative []int64
	// Probabilities includes only selected tags and is retained in the activity log.
	Probabilities map[int64]float64
}

// SelectAutoTags preserves candidate order and skips unanswered candidates.
func SelectAutoTags(candidates []AutoTagCandidate, probabilities map[string]float64, settings AutoTagSettings) AutoTagSelection {
	var selection AutoTagSelection
	keep := func(id int64, p float64) {
		if selection.Probabilities == nil {
			selection.Probabilities = map[int64]float64{}
		}
		selection.Probabilities[id] = p
	}
	for _, candidate := range candidates {
		p, ok := probabilities[AutoTagQuestionKey(candidate.ID)]
		switch {
		case !ok || math.IsNaN(p):
		case p >= settings.Threshold:
			selection.Confirmed = append(selection.Confirmed, candidate.ID)
			keep(candidate.ID, p)
		case p >= settings.TentativeThreshold:
			selection.Tentative = append(selection.Tentative, candidate.ID)
			keep(candidate.ID, p)
		}
	}
	return selection
}

// AutoTagApplied signals completed classification even when no tags were assigned, prompting UI refreshes.
type AutoTagApplied struct {
	VideoIDs []int64
}

func (AutoTagApplied) event() {}
