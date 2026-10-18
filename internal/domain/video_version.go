package domain

import (
	"cmp"
	"errors"
	"slices"
)

// Version bundles own their user-data key for tags, progress, and visibility; storage exposes it as Video.UserKey.

var ErrNotBundled = errors.New("the video is not bundled")

var ErrRepresentativeNotSelected = errors.New("the representative is not among the selected videos")

var ErrTooFewVersions = errors.New("at least two videos are needed to bundle")

type VideoVersionsRef struct {
	// Count includes only members with locations visible to the audience.
	Count            int
	RepresentativeID int64
}

// VideoVersions places the representative first, then natural title and ID; an unbundled video returns itself alone.
type VideoVersions struct {
	RepresentativeID int64
	Items            []Video
}

func SortVideoVersions(items []Video, representativeID int64) {
	slices.SortStableFunc(items, func(a, b Video) int {
		switch {
		case a.ID == b.ID:
			return 0
		case a.ID == representativeID:
			return -1
		case b.ID == representativeID:
			return 1
		}
		if order := CompareNatural(a.Title, b.Title); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

type VideoBundleChanged struct {
	VideoIDs []int64
}

func (VideoBundleChanged) event() {}

const successionToleranceMinMs = 1000

// DurationsMatch permits max(1 second, 0.5% of the longer duration), rejecting unknown/nonpositive durations; succession and fingerprint candidates share this rule.
func DurationsMatch(a, b int64) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= max(successionToleranceMinMs, max(a, b)*5/1000)
}

const MaxVersionCandidates = 200

// Videos is ID-ordered; Distance is the median fingerprint Hamming distance.
type VersionCandidate struct {
	Videos   [2]Video
	Distance int
}

type VersionCandidatePage struct {
	Items []VersionCandidate
	Total int
}
