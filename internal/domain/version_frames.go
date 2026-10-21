package domain

// Matching frame hashes bypass the model; differing runs provide side-by-side frames for model judgement.

const (
	VersionJudgeQuestionKey = "same_picture"
	// The question asks about the content, so a version the owner would not tell apart counts as the same video.
	VersionJudgeQuestion = "Each image puts two frames side by side, one from each of two video files, taken at " +
		"matching moments of the content. In every image, do the left half and the right half show the same scene " +
		"of the same video? Ignore differences in sharpness, resolution, compression, colour, cropping or black " +
		"borders, watermarks, channel or site logos, burned-in subtitles or captions, and a timing difference of a " +
		"few seconds."
	VersionSameProbability      = 0.8
	VersionDifferentProbability = 0.2
	VersionJudgeMaxImages       = 4
	// At most 1/20 (5%) differing frames may still count as the same video.
	versionFramesDifferingDivisor = 20
)

type VersionFrameOutcome int

const (
	// VersionFramesSame permits at most 5% nonconsecutive differences without model judgement.
	VersionFramesSame VersionFrameOutcome = iota + 1
	VersionFramesDiffer
	VersionFramesIncomparable
)

func (o VersionFrameOutcome) String() string {
	switch o {
	case VersionFramesSame:
		return "frames_match"
	case VersionFramesDiffer:
		return "frames_differ"
	case VersionFramesIncomparable:
		return "incomparable"
	default:
		return "unknown"
	}
}

// Shown contains at most VersionJudgeMaxImages pairs in timestamp order, only for VersionFramesDiffer.
// Run holds the first and last pair of the longest differing run when it has two or more pairs.
type VersionFrameRule struct {
	Outcome    VersionFrameOutcome
	Compared   int
	Differing  int
	LongestRun int
	Run        [2]FramePair
	Shown      []FramePair
}

// JudgeVersionFrames tolerates at most 5% isolated differences; otherwise it samples the longest differing run (earliest tie), or all isolated differences, evenly up to the image limit.
func JudgeVersionFrames(a, b Fingerprint) VersionFrameRule {
	pairs, ok := ComparableFramePairs(a, b)
	if !ok {
		return VersionFrameRule{Outcome: VersionFramesIncomparable}
	}
	rule := VersionFrameRule{Compared: len(pairs)}
	var differing, longest, current []FramePair
	for _, pair := range pairs {
		if pair.Distance <= FingerprintMatchMaxDistance {
			current = nil
			continue
		}
		differing = append(differing, pair)
		if len(current) > 0 && !framePairsAdjacent(a, b, current[len(current)-1], pair) {
			// Skipped flat or unmatched frames break differing runs.
			current = nil
		}
		current = append(current, pair)
		if len(current) > len(longest) {
			longest = current
		}
	}
	rule.Differing = len(differing)
	if len(longest) >= 2 {
		rule.LongestRun = len(longest)
		rule.Run = [2]FramePair{longest[0], longest[len(longest)-1]}
	}
	if len(longest) < 2 && rule.Differing*versionFramesDifferingDivisor <= rule.Compared {
		rule.Outcome = VersionFramesSame
		return rule
	}
	rule.Outcome = VersionFramesDiffer
	shown := longest
	if len(shown) < 2 {
		shown = differing
	}
	for _, index := range SpreadFrameIndexes(len(shown), VersionJudgeMaxImages) {
		rule.Shown = append(rule.Shown, shown[index])
	}
	return rule
}

// framePairsAdjacent checks both timelines against the coarser interval so differing-rate inputs remain symmetric and skipped frames break adjacency.
func framePairsAdjacent(a, b Fingerprint, p, q FramePair) bool {
	step := max(a.IntervalMs, b.IntervalMs)
	return int64(q.A-p.A)*a.IntervalMs <= step && int64(q.B-p.B)*b.IntervalMs <= step
}

func VersionVerdictFor(probability float64) VersionVerdict {
	switch {
	case probability >= VersionSameProbability:
		return VersionSame
	case probability <= VersionDifferentProbability:
		return VersionDifferent
	default:
		return VersionUndecided
	}
}
