package domain

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"
)

// Activity entries record durable changes in the same transaction; ScanActivity reports transient import progress.

// ActivityKind values correspond to activity_entries.kind.
type ActivityKind string

const (
	ActivityImport             ActivityKind = "import"
	ActivityScan               ActivityKind = "scan"
	ActivityAutoTag            ActivityKind = "auto_tag"
	ActivityVersionJudgement   ActivityKind = "version_judgement"
	ActivityWorkInfoFetch      ActivityKind = "work_info_fetch"
	ActivityBundle             ActivityKind = "bundle"
	ActivityUnbundle           ActivityKind = "unbundle"
	ActivityMakeRepresentative ActivityKind = "make_representative"
	ActivityDismiss            ActivityKind = "dismiss"
	ActivityTagAttach          ActivityKind = "tag_attach"
	ActivityTagDetach          ActivityKind = "tag_detach"
	ActivityTagKeep            ActivityKind = "tag_keep"
	ActivityTagReject          ActivityKind = "tag_reject"
	ActivityWorkInfoApply      ActivityKind = "work_info_apply"
	ActivityMediaFolderRemove  ActivityKind = "media_folder_remove"
	ActivityOrganizeUndo       ActivityKind = "organize_undo"
	// ActivityUndo inherits the feature of the entry it reverses.
	ActivityUndo ActivityKind = "undo"
)

type ActivityFeature string

const (
	ActivityFeatureImport         ActivityFeature = "import"
	ActivityFeatureScan           ActivityFeature = "scan"
	ActivityFeatureDuplicateCheck ActivityFeature = "duplicate_check"
	ActivityFeatureAutoTagging    ActivityFeature = "auto_tagging"
	ActivityFeatureWorkInfo       ActivityFeature = "work_info"
	ActivityFeatureTags           ActivityFeature = "tags"
	ActivityFeatureVersions       ActivityFeature = "versions"
	ActivityFeatureMediaFolders   ActivityFeature = "media_folders"
	ActivityFeatureOrganize       ActivityFeature = "organize"
)

// ActivityFeatures follows the UI filter order.
var ActivityFeatures = []ActivityFeature{
	ActivityFeatureImport, ActivityFeatureScan, ActivityFeatureDuplicateCheck, ActivityFeatureAutoTagging,
	ActivityFeatureWorkInfo, ActivityFeatureTags, ActivityFeatureVersions, ActivityFeatureMediaFolders,
	ActivityFeatureOrganize,
}

var activityKindFeatures = map[ActivityKind]ActivityFeature{
	ActivityImport:             ActivityFeatureImport,
	ActivityScan:               ActivityFeatureScan,
	ActivityVersionJudgement:   ActivityFeatureDuplicateCheck,
	ActivityAutoTag:            ActivityFeatureAutoTagging,
	ActivityWorkInfoFetch:      ActivityFeatureWorkInfo,
	ActivityWorkInfoApply:      ActivityFeatureWorkInfo,
	ActivityTagAttach:          ActivityFeatureTags,
	ActivityTagDetach:          ActivityFeatureTags,
	ActivityTagKeep:            ActivityFeatureTags,
	ActivityTagReject:          ActivityFeatureTags,
	ActivityBundle:             ActivityFeatureVersions,
	ActivityUnbundle:           ActivityFeatureVersions,
	ActivityMakeRepresentative: ActivityFeatureVersions,
	ActivityDismiss:            ActivityFeatureVersions,
	ActivityMediaFolderRemove:  ActivityFeatureMediaFolders,
	ActivityOrganizeUndo:       ActivityFeatureOrganize,
}

// Feature returns false for undo entries, whose feature comes from the reversed entry.
func (k ActivityKind) Feature() (ActivityFeature, bool) {
	feature, ok := activityKindFeatures[k]
	return feature, ok
}

func ParseActivityFeature(value string) (ActivityFeature, error) {
	feature := ActivityFeature(value)
	if !slices.Contains(ActivityFeatures, feature) {
		return "", fmt.Errorf("%w: feature %s", ErrInvalidActivityFilter, strconv.Quote(value))
	}
	return feature, nil
}

type ActivitySubjectKind string

const (
	ActivitySubjectVideo  ActivitySubjectKind = "video"
	ActivitySubjectPair   ActivitySubjectKind = "pair"
	ActivitySubjectScan   ActivitySubjectKind = "scan"
	ActivitySubjectFolder ActivitySubjectKind = "folder"
)

type ActivityStatus string

const (
	ActivityRunning   ActivityStatus = "running"
	ActivityAttention ActivityStatus = "attention"
	ActivityDone      ActivityStatus = "done"
)

// ActivityStatuses follows the UI display order.
var ActivityStatuses = []ActivityStatus{ActivityRunning, ActivityAttention, ActivityDone}

func ParseActivityStatus(value string) (ActivityStatus, error) {
	status := ActivityStatus(value)
	if !slices.Contains(ActivityStatuses, status) {
		return "", fmt.Errorf("%w: status %s", ErrInvalidActivityFilter, strconv.Quote(value))
	}
	return status, nil
}

type ActivityReason string

const (
	ActivityWaiting    ActivityReason = "waiting"
	ActivityProcessing ActivityReason = "processing"
	ActivityFailed     ActivityReason = "failed"
	// ActivityInterrupted marks processing entries left behind at startup.
	ActivityInterrupted  ActivityReason = "interrupted"
	ActivityNoConnection ActivityReason = "no_connection"
	ActivityUndecided    ActivityReason = "undecided"
)

type ActivityOutcome string

const (
	ActivityCompleted       ActivityOutcome = "completed"
	ActivitySame            ActivityOutcome = "same"
	ActivityDifferent       ActivityOutcome = "different"
	ActivityOutcomeFailed   ActivityOutcome = "failed"
	ActivitySuperseded      ActivityOutcome = "superseded"
	ActivityDecidedByOwner  ActivityOutcome = "decided_by_owner"
	ActivityGone            ActivityOutcome = "gone"
	ActivityAlreadyReverted ActivityOutcome = "already_reverted"
)

// ActivityDecider identifies the decision maker, or the initiator when no decision exists.
type ActivityDecider string

const (
	ActivityByRule  ActivityDecider = "rule"
	ActivityByAI    ActivityDecider = "ai"
	ActivityByOwner ActivityDecider = "owner"
)

var ActivityDeciders = []ActivityDecider{ActivityByRule, ActivityByAI, ActivityByOwner}

func ParseActivityDecider(value string) (ActivityDecider, error) {
	decider := ActivityDecider(value)
	if !slices.Contains(ActivityDeciders, decider) {
		return "", fmt.Errorf("%w: decider %s", ErrInvalidActivityFilter, strconv.Quote(value))
	}
	return decider, nil
}

var ErrInvalidActivityFilter = errors.New("invalid activity filter")

// ErrActivityNotRetryable also covers targets that no longer exist.
var ErrActivityNotRetryable = errors.New("the activity entry cannot be retried")

// ErrActivityNotUndoable also covers entries or tags already undone.
var ErrActivityNotUndoable = errors.New("the activity entry cannot be undone")

// ActivitySubject orders pairs by Key < KeyB; labels are snapshots retained after deletion.
type ActivitySubject struct {
	Kind   ActivitySubjectKind
	Key    string
	KeyB   string
	Label  string
	LabelB string
}

// SameAs compares keys, ignoring display labels.
func (s ActivitySubject) SameAs(other ActivitySubject) bool {
	return s.Kind == other.Kind && s.Key == other.Key && s.KeyB == other.KeyB
}

// ActivityEntry maps zero and empty values to SQL null.
type ActivityEntry struct {
	ID          int64
	Kind        ActivityKind
	Feature     ActivityFeature
	Subject     ActivitySubject
	Status      ActivityStatus
	Reason      ActivityReason
	Outcome     ActivityOutcome
	Detail      string
	Stage       JobKind
	UndoesID    int64
	Decider     ActivityDecider
	Probability *float64
	// UndoKeys stores JSON containing the joined members' keys and their former representative.
	UndoKeys  string
	UndoneBy  int64
	CreatedAt time.Time
	UpdatedAt time.Time

	// The following fields are populated by list reads and ignored on writes.

	Tags   []ActivityEntryTag
	Undoes *ActivityUndone
	// VideoIDs maps subject keys to currently accessible videos; missing keys map to zero.
	VideoIDs  [2]int64
	Candidate bool
	// RetryAt is the next retry time for a failed version_judgement.
	RetryAt time.Time
	// Progress describes a running import after the waiting stage.
	Progress *ActivityImportProgress
}

type ActivityEntryTag struct {
	TagID       int64
	Name        string
	Suggested   bool
	Probability float64
	UndoneBy    int64
}

type ActivityUndone struct {
	Kind    ActivityKind
	Outcome ActivityOutcome
	TagName string
}

// ActivityImportProgress counts completed stages out of five.
type ActivityImportProgress struct {
	Stage JobKind
	Done  int
	Total int
}

// Only automatic decisions (rule or AI) can be undone.
func (d ActivityDecider) automatic() bool {
	return d == ActivityByRule || d == ActivityByAI
}

var activityRetryKinds = []ActivityKind{ActivityImport, ActivityScan, ActivityAutoTag, ActivityVersionJudgement}

// RetryOffered accepts failed or interrupted import, scan, auto_tag, and version_judgement entries requiring attention.
func (e ActivityEntry) RetryOffered() bool {
	if e.Status != ActivityAttention || (e.Reason != ActivityFailed && e.Reason != ActivityInterrupted) {
		return false
	}
	return slices.Contains(activityRetryKinds, e.Kind)
}

// UndoOffered accepts automatic same/different judgements not already undone; auto_tag uses TagUndoOffered per tag.
func (e ActivityEntry) UndoOffered() bool {
	if e.UndoneBy != 0 || !e.Decider.automatic() {
		return false
	}
	switch e.Kind {
	case ActivityVersionJudgement:
		return e.Status == ActivityDone && (e.Outcome == ActivitySame || e.Outcome == ActivityDifferent)
	case ActivityAutoTag:
		return slices.ContainsFunc(e.Tags, func(tag ActivityEntryTag) bool { return e.TagUndoOffered(tag) })
	default:
		return false
	}
}

// TagUndoOffered accepts automatic tag assignments not already undone.
func (e ActivityEntry) TagUndoOffered(tag ActivityEntryTag) bool {
	return e.Kind == ActivityAutoTag && e.Decider.automatic() && tag.UndoneBy == 0
}

// Supersedes closes attention entries with the same kind and subject, except version_judgement, which retains one entry per pair.
func (e ActivityEntry) Supersedes(older ActivityEntry) bool {
	return e.Kind != ActivityVersionJudgement && older.Status == ActivityAttention &&
		older.Kind == e.Kind && older.Subject.SameAs(e.Subject)
}

type ActivityState struct {
	Status  ActivityStatus
	Reason  ActivityReason
	Outcome ActivityOutcome
}

// ImportActivityState prioritizes queued/running work, then failed work, then completion; running work is waiting until any job has been claimed.
func ImportActivityState(jobs []JobState) ActivityState {
	pending := slices.ContainsFunc(jobs, func(state JobState) bool { return state == JobQueued || state == JobRunning })
	if pending {
		if slices.ContainsFunc(jobs, func(state JobState) bool { return state != JobQueued }) {
			return ActivityState{Status: ActivityRunning, Reason: ActivityProcessing}
		}
		return ActivityState{Status: ActivityRunning, Reason: ActivityWaiting}
	}
	if slices.Contains(jobs, JobFailed) {
		return ActivityState{Status: ActivityAttention, Reason: ActivityFailed}
	}
	return ActivityState{Status: ActivityDone, Outcome: ActivityCompleted}
}

// ActivityChanged is published after the transaction commits and becomes a payload-free UI notification.
type ActivityChanged struct{}

func (ActivityChanged) event() {}

// ActivityQuery's zero value applies no filters.
type ActivityQuery struct {
	Status   ActivityStatus
	Features []ActivityFeature
	Decider  ActivityDecider
	Since    time.Time
	// Search matches subject labels case-insensitively.
	Search string
}

// MaxActivitySearchLength counts Unicode code points.
const MaxActivitySearchLength = 200

type ActivityCounts struct {
	Running   int
	Attention int
	Done      int
}

// ActivityPage counts ignore Status, feature counts ignore Features, and NextCursor is set only when more entries exist.
type ActivityPage struct {
	Items      []ActivityEntry
	NextCursor string
	Counts     ActivityCounts
	Features   map[ActivityFeature]int
}

// EncodeActivityCursor encodes the last ID; descending pagination continues with smaller IDs.
func EncodeActivityCursor(lastID int64) string {
	body, _ := json.Marshal(struct {
		ID int64 `json:"i"`
	}{lastID}) // Marshaling a struct containing only an integer cannot fail.
	return base64.RawURLEncoding.EncodeToString(body)
}

// DecodeActivityCursor rejects malformed, missing, or nonpositive IDs with ErrInvalidCursor.
func DecodeActivityCursor(cursor string) (int64, error) {
	body, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	var fields struct {
		ID *int64 `json:"i"`
	}
	if err := json.Unmarshal(body, &fields); err != nil || fields.ID == nil || *fields.ID < 1 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	return *fields.ID, nil
}
