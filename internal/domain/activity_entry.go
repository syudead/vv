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

// アクティビティの記録（specs/055-activity/data-model.md）。取り込み・自動タグ付け・重複の判定・作品情報の
// 取得・失敗した走査と、動画のタグと集まりを変える所有者の操作を、変える内容と同じ取引で 1 件ずつ残す。
// 取り込み中の今の処理（ScanActivity）とは別物で、名前も「activity entry」として分ける（research.md R-1）。

// ActivityKind は記録の種類である。値は activity_entries.kind 列に対応する。
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
	// ActivityUndo は自動の判断の取り消しである。feature は取り消した記録のものを写す。
	ActivityUndo ActivityKind = "undo"
)

// ActivityFeature は種類のまとまりで、画面の機能の絞り込みの単位である
// （specs/055-activity/contracts/screen-api.md「Features」）。
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

// ActivityFeatures は機能の値を、画面の絞り込みに並べる順で持つ。
var ActivityFeatures = []ActivityFeature{
	ActivityFeatureImport, ActivityFeatureScan, ActivityFeatureDuplicateCheck, ActivityFeatureAutoTagging,
	ActivityFeatureWorkInfo, ActivityFeatureTags, ActivityFeatureVersions, ActivityFeatureMediaFolders,
	ActivityFeatureOrganize,
}

// activityKindFeatures は undo 以外の種類の機能である。
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

// Feature は種類の機能を返す。undo の機能は取り消した記録で決まるので、ここでは決まらず ok は偽である。
func (k ActivityKind) Feature() (ActivityFeature, bool) {
	feature, ok := activityKindFeatures[k]
	return feature, ok
}

// ParseActivityFeature は絞り込みの値を読む。知らない値は ErrInvalidActivityFilter を返す。
func ParseActivityFeature(value string) (ActivityFeature, error) {
	feature := ActivityFeature(value)
	if !slices.Contains(ActivityFeatures, feature) {
		return "", fmt.Errorf("%w: feature %s", ErrInvalidActivityFilter, strconv.Quote(value))
	}
	return feature, nil
}

// ActivitySubjectKind は記録の対象の種類である（research.md R-10）。
type ActivitySubjectKind string

const (
	ActivitySubjectVideo  ActivitySubjectKind = "video"
	ActivitySubjectPair   ActivitySubjectKind = "pair"
	ActivitySubjectScan   ActivitySubjectKind = "scan"
	ActivitySubjectFolder ActivitySubjectKind = "folder"
)

// ActivityStatus は記録の状態である（research.md R-4）。
type ActivityStatus string

const (
	ActivityRunning   ActivityStatus = "running"
	ActivityAttention ActivityStatus = "attention"
	ActivityDone      ActivityStatus = "done"
)

// ActivityStatuses は状態の値を、画面に並べる順で持つ。
var ActivityStatuses = []ActivityStatus{ActivityRunning, ActivityAttention, ActivityDone}

// ParseActivityStatus は絞り込みの値を読む。知らない値は ErrInvalidActivityFilter を返す。
func ParseActivityStatus(value string) (ActivityStatus, error) {
	status := ActivityStatus(value)
	if !slices.Contains(ActivityStatuses, status) {
		return "", fmt.Errorf("%w: status %s", ErrInvalidActivityFilter, strconv.Quote(value))
	}
	return status, nil
}

// ActivityReason は running と attention の記録の理由である（data-model.md「Reasons」）。
type ActivityReason string

const (
	// ActivityWaiting はワーカーが取り出す前である。
	ActivityWaiting ActivityReason = "waiting"
	// ActivityProcessing はワーカーが取り出した後である。
	ActivityProcessing ActivityReason = "processing"
	ActivityFailed     ActivityReason = "failed"
	// ActivityInterrupted は起動時に processing のまま残っていた記録である。
	ActivityInterrupted  ActivityReason = "interrupted"
	ActivityNoConnection ActivityReason = "no_connection"
	ActivityUndecided    ActivityReason = "undecided"
)

// ActivityOutcome は done の記録の結果である（data-model.md「Outcomes」）。
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

// ActivityDecider は記録した判断をしたもの、無ければ処理を始めたものである（research.md R-5）。
type ActivityDecider string

const (
	ActivityByRule  ActivityDecider = "rule"
	ActivityByAI    ActivityDecider = "ai"
	ActivityByOwner ActivityDecider = "owner"
)

// ActivityDeciders は判断したものの値である。
var ActivityDeciders = []ActivityDecider{ActivityByRule, ActivityByAI, ActivityByOwner}

// ParseActivityDecider は絞り込みの値を読む。知らない値は ErrInvalidActivityFilter を返す。
func ParseActivityDecider(value string) (ActivityDecider, error) {
	decider := ActivityDecider(value)
	if !slices.Contains(ActivityDeciders, decider) {
		return "", fmt.Errorf("%w: decider %s", ErrInvalidActivityFilter, strconv.Quote(value))
	}
	return decider, nil
}

// ErrInvalidActivityFilter は一覧の絞り込みの値が知らないものであることを表す。
var ErrInvalidActivityFilter = errors.New("invalid activity filter")

// ErrActivityNotRetryable は記録がやり直しを出さないか、その対象がもう無いことを表す。
var ErrActivityNotRetryable = errors.New("the activity entry cannot be retried")

// ErrActivityNotUndoable は記録か付けたタグが取り消しを出さないか、もう取り消したことを表す。
var ErrActivityNotUndoable = errors.New("the activity entry cannot be undone")

// ActivitySubject は記録の対象である。組なら KeyB と LabelB も入り、Key < KeyB に並べる。Label は書いた
// 時点の表示名で、対象が消えても残る（research.md R-10）。
type ActivitySubject struct {
	Kind   ActivitySubjectKind
	Key    string
	KeyB   string
	Label  string
	LabelB string
}

// SameAs は 2 つの対象が同じ鍵を指すかを返す。表示名は比べない。
func (s ActivitySubject) SameAs(other ActivitySubject) bool {
	return s.Kind == other.Kind && s.Key == other.Key && s.KeyB == other.KeyB
}

// ActivityEntry は記録の 1 件である。0 と空は列の null に当たる。
type ActivityEntry struct {
	ID      int64
	Kind    ActivityKind
	Feature ActivityFeature
	Subject ActivitySubject
	Status  ActivityStatus
	Reason  ActivityReason
	Outcome ActivityOutcome
	// Detail は失敗の文、所有者の操作が変えたタグ、メディアフォルダの登録の解除で外れた動画の数などである。
	Detail string
	// Stage は失敗した import の、諦めた段階である。
	Stage JobKind
	// UndoesID は undo の記録が取り消した記録である。
	UndoesID    int64
	Decider     ActivityDecider
	Probability *float64
	// UndoKeys は自動で束ねた組の、加わった側のメンバーの鍵と元の代表の鍵の JSON である（research.md R-7）。
	UndoKeys string
	// UndoneBy はこの記録を取り消した所有者の記録である。
	UndoneBy  int64
	CreatedAt time.Time
	UpdatedAt time.Time

	// ここから下は一覧の読み出しが添える。書き込みでは使わない。

	// Tags は auto_tag の記録が付けたタグである。
	Tags []ActivityEntryTag
	// Undoes は undo の記録が取り消した記録の要約である。
	Undoes *ActivityUndone
	// VideoIDs は対象の鍵ごとの、見る人が開ける今の動画である。無い鍵は 0。
	VideoIDs [2]int64
	// Candidate は組の候補が重複の候補の画面にまだあるかである。
	Candidate bool
	// RetryAt は失敗した version_judgement が次に試す時刻である。
	RetryAt time.Time
	// Progress は waiting を過ぎた running の import の進み具合である。
	Progress *ActivityImportProgress
}

// ActivityEntryTag は auto_tag の記録が付けたタグの 1 つである（data-model.md「activity_entry_tags」）。
type ActivityEntryTag struct {
	TagID       int64
	Name        string
	Suggested   bool
	Probability float64
	// UndoneBy はこのタグを取り消した undo の記録である。
	UndoneBy int64
}

// ActivityUndone は undo の記録が取り消した記録の要約である。
type ActivityUndone struct {
	Kind    ActivityKind
	Outcome ActivityOutcome
	// TagName はタグごとの取り消しのときのタグの名前である。
	TagName string
}

// ActivityImportProgress は running の import の記録の、今の段階と済んだ段階の数である。Total は 5。
type ActivityImportProgress struct {
	Stage JobKind
	Done  int
	Total int
}

// automatic は判断したものが自動（rule か ai）かを返す。取り消しを出すのは自動の判断だけである。
func (d ActivityDecider) automatic() bool {
	return d == ActivityByRule || d == ActivityByAI
}

// activityRetryKinds はやり直しを出す種類である（data-model.md「Rules」）。
var activityRetryKinds = []ActivityKind{ActivityImport, ActivityScan, ActivityAutoTag, ActivityVersionJudgement}

// RetryOffered は記録がやり直しを出すかを返す。attention で理由が failed か interrupted の、import・scan・
// auto_tag・version_judgement の記録だけである。
func (e ActivityEntry) RetryOffered() bool {
	if e.Status != ActivityAttention || (e.Reason != ActivityFailed && e.Reason != ActivityInterrupted) {
		return false
	}
	return slices.Contains(activityRetryKinds, e.Kind)
}

// UndoOffered は記録そのものが取り消しを出すかを返す。rule か ai が決めて、まだ取り消していない
// version_judgement の same と different である。auto_tag はタグごとに TagUndoOffered で決める。
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

// TagUndoOffered は auto_tag の記録が付けたタグ tag が取り消しを出すかを返す。rule か ai が決めて、まだ
// 取り消していないタグである。
func (e ActivityEntry) TagUndoOffered(tag ActivityEntryTag) bool {
	return e.Kind == ActivityAutoTag && e.Decider.automatic() && tag.UndoneBy == 0
}

// Supersedes は新しい記録 e が、古い記録 older を superseded で閉じるかを返す。同じ種類と対象の attention の
// 記録だけを閉じる（research.md R-4）。version_judgement は組ごとに 1 件を保つので、新しい記録で閉じない。
// done で書く import は所在を足しただけで段階を走らせていないので、前の失敗を閉じない。
func (e ActivityEntry) Supersedes(older ActivityEntry) bool {
	if e.Kind == ActivityImport && e.Status == ActivityDone {
		return false
	}
	return e.Kind != ActivityVersionJudgement && older.Status == ActivityAttention &&
		older.Kind == e.Kind && older.Subject.SameAs(e.Subject)
}

// ActivityState は記録の状態と、その理由か結果である。
type ActivityState struct {
	Status  ActivityStatus
	Reason  ActivityReason
	Outcome ActivityOutcome
}

// ImportActivityState は import の記録の状態を、その記録に属する仕事の状態から決める（data-model.md
// 「Rules」）。待っているか走っている仕事があれば running で、まだどの仕事も取り出されていなければ理由は
// waiting、そうでなければ processing。諦めた仕事（failed）があれば attention/failed、無ければ done/completed。
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

// ActivityChanged は記録が書かれたか変わったことを表す。記録を書いた取引の確定後に発行し、画面へは中身の
// 無い activity の知らせになる（research.md R-8）。
type ActivityChanged struct{}

func (ActivityChanged) event() {}

// ActivityQuery は一覧の絞り込みである。ゼロ値は絞り込まない。
type ActivityQuery struct {
	Status   ActivityStatus
	Features []ActivityFeature
	Decider  ActivityDecider
	// Since はこの時刻以降に作った記録だけにする。
	Since time.Time
	// Search は対象の表示名に含まれる文字列で、大文字と小文字を区別しない。
	Search string
}

// MaxActivitySearchLength は Search の文字数の上限である。
const MaxActivitySearchLength = 200

// ActivityCounts は状態ごとの記録の数である。
type ActivityCounts struct {
	Running   int
	Attention int
	Done      int
}

// ActivityPage は一覧の 1 ページである。Counts は Status 以外の、Features は Features 以外の絞り込みに従う
// （contracts/screen-api.md「GET /api/activity」）。NextCursor は続きがあるときだけ入る。
type ActivityPage struct {
	Items      []ActivityEntry
	NextCursor string
	Counts     ActivityCounts
	Features   map[ActivityFeature]int
}

// EncodeActivityCursor はページの最後の記録の id を不透明なカーソルにする。並びは id の降順なので、次の
// ページはこの id より小さい記録から始まる（research.md R-9）。
func EncodeActivityCursor(lastID int64) string {
	body, _ := json.Marshal(struct {
		ID int64 `json:"i"`
	}{lastID}) // 整数 1 つの構造体の Marshal は失敗しない。
	return base64.RawURLEncoding.EncodeToString(body)
}

// DecodeActivityCursor は EncodeActivityCursor の逆である。読めないもの、欄の欠けたもの、id が 1 未満の
// ものは ErrInvalidCursor を返す。
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
