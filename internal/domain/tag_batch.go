package domain

// The 20,000-ID limit fits the body cap and a single json_each argument, matching video batch operations.
const MaxTagBatch = 20000

type TagBatchAction string

const (
	TagBatchConfirm TagBatchAction = "confirm"
	// TagBatchReject deletes tentative tags and records their canonical names as rejected.
	TagBatchReject TagBatchAction = "reject"
	TagBatchDelete TagBatchAction = "delete"
	// Changing kind does not confirm tentative tags.
	TagBatchSetKind TagBatchAction = "setKind"
	// Clearing kind does not confirm tentative tags.
	TagBatchClearKind TagBatchAction = "clearKind"
)

func (a TagBatchAction) Valid() bool {
	switch a {
	case TagBatchConfirm, TagBatchReject, TagBatchDelete, TagBatchSetKind, TagBatchClearKind:
		return true
	}
	return false
}

type TagBatchRequest struct {
	Action TagBatchAction
	KindID int64
}

type TagBatchTarget struct {
	Tentative bool
	KindID    int64
}

// TagBatchApplies mirrors row actions: confirm/reject tentative tags, delete confirmed tags, and change kinds regardless of tentative state.
func TagBatchApplies(req TagBatchRequest, target TagBatchTarget) bool {
	switch req.Action {
	case TagBatchConfirm, TagBatchReject:
		return target.Tentative
	case TagBatchDelete:
		return !target.Tentative
	case TagBatchSetKind:
		return target.KindID != req.KindID
	case TagBatchClearKind:
		return target.KindID != 0
	}
	return false
}

// Outcome arrays are disjoint, follow deduplicated input order, and use empty slices rather than nil.
type TagBatchOutcome struct {
	AppliedIDs       []int64
	NotFoundIDs      []int64
	NotApplicableIDs []int64
}

type TagImpactAction string

const (
	TagImpactReject TagImpactAction = "reject"
	TagImpactDelete TagImpactAction = "delete"
	TagImpactMerge  TagImpactAction = "merge"
)

func (a TagImpactAction) Valid() bool {
	switch a {
	case TagImpactReject, TagImpactDelete, TagImpactMerge:
		return true
	}
	return false
}

// TagImpactApplies mirrors batch applicability so confirmation counts match actual work; merges accept both tentative and confirmed tags.
func TagImpactApplies(action TagImpactAction, tentative bool) bool {
	switch action {
	case TagImpactReject:
		return TagBatchApplies(TagBatchRequest{Action: TagBatchReject}, TagBatchTarget{Tentative: tentative})
	case TagImpactDelete:
		return TagBatchApplies(TagBatchRequest{Action: TagBatchDelete}, TagBatchTarget{Tentative: tentative})
	case TagImpactMerge:
		return true
	}
	return false
}

type TagImpact struct {
	TagCount   int
	VideoCount int
}

type TagMergeOutcome struct {
	Tag Tag
	// NotFoundIDs follows deduplicated source order and uses an empty slice when none are missing.
	NotFoundIDs []int64
}
