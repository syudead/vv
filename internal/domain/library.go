package domain

import (
	"errors"
	"time"
)

const (
	DefaultLimit = 60
	MaxLimit     = 200
)

var ErrNotFound = errors.New("not found")

var (
	ErrNoMediaFolders         = errors.New("no media folders are configured")
	ErrScanRunning            = errors.New("a scan is in progress")
	ErrFolderConflict         = errors.New("media folders overlap or contain each other")
	ErrVersionConflict        = errors.New("media folders were changed by another operation")
	ErrInvalidMediaFolder     = errors.New("cannot be added as a media folder")
	ErrUnsupportedMediaFolder = errors.New("unsupported media folder")
)

var ErrProbeNotFailed = errors.New("the video has not failed to probe")

// Reject invalid cursors rather than restarting pagination, which would loop the same infinite-scroll results.
var ErrInvalidCursor = errors.New("invalid cursor")

type VideoSort string

const (
	SortAddedAsc     VideoSort = "addedAsc"
	SortAddedDesc    VideoSort = "addedDesc"
	SortModifiedAsc  VideoSort = "modifiedAsc"
	SortModifiedDesc VideoSort = "modifiedDesc"
	// Creation-time sorting falls back to modification time when creation time is unavailable.
	SortCreatedAsc  VideoSort = "createdAsc"
	SortCreatedDesc VideoSort = "createdDesc"
	SortTitleAsc    VideoSort = "titleAsc"
	SortTitleDesc   VideoSort = "titleDesc"
	// Videos with unknown duration sort last.
	SortDurationAsc VideoSort = "durationAsc"
	// Videos with unknown duration sort last.
	SortDurationDesc VideoSort = "durationDesc"
	SortSizeAsc      VideoSort = "sizeAsc"
	SortSizeDesc     VideoSort = "sizeDesc"
	// Videos without playback history sort last.
	SortPlayedAsc VideoSort = "playedAsc"
	// Videos without playback history sort last.
	SortPlayedDesc VideoSort = "playedDesc"
	// Non-favorite items sort last.
	SortFavoritedAsc VideoSort = "favoritedAsc"
	// Non-favorite items sort last.
	SortFavoritedDesc VideoSort = "favoritedDesc"
	SortRandom        VideoSort = "random"
)

func (s VideoSort) Valid() bool {
	switch s {
	case SortAddedAsc, SortAddedDesc, SortModifiedAsc, SortModifiedDesc,
		SortCreatedAsc, SortCreatedDesc, SortTitleAsc, SortTitleDesc, SortDurationAsc, SortDurationDesc,
		SortSizeAsc, SortSizeDesc, SortPlayedAsc, SortPlayedDesc, SortFavoritedAsc, SortFavoritedDesc, SortRandom:
		return true
	default:
		return false
	}
}

const MaxShuffleSeed = 2147483647

// ShuffleKey is stable across pages and membership changes for a given seed and ID; callers break collisions by ID.
func ShuffleKey(seed, id int64) int64 {
	z := uint64(seed)*0x9e3779b97f4a7c15 + uint64(id)
	z += 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	// Clear the sign bit to keep signed 64-bit values nonnegative.
	return int64(z >> 1)
}

type VideoQuery struct {
	// Query uses ParseSearchQuery syntax; no remaining terms means no filtering.
	Query string
	// An empty Watch means WatchAll.
	Watch        WatchFilter
	PlayableOnly bool
	Sort         VideoSort
	// Seed is used only by SortRandom and ranges from zero to MaxShuffleSeed.
	Seed   int64
	Cursor string
	Limit  int
	// TagIDs uses AND matching; missing IDs are excluded from filters and returned in MissingTagIDs (docs/design-docs/catalog-organization.md).
	TagIDs       []int64
	FavoriteOnly bool
	// Person matches any role; zero disables filtering and nonexistent IDs match nothing.
	Person       int64
	Organization int64
	// TagKindID matches manual or folder-derived tags; zero disables filtering and nonexistent IDs match nothing.
	TagKindID int64
	// AddedWithinDays is inclusive; zero disables filtering.
	AddedWithinDays int
	// Duration bounds are inclusive seconds; nil leaves a side unbounded and any bound excludes unknown durations.
	MinDurationSec *int
	MaxDurationSec *int
	// Unshelved は、ホームのどのフォルダの棚の実効の規則（祖先の条件を含む経路の条件）にも一致しない
	// 動画だけにする。棚との対応は保存せず、読むたびに求める
	// （docs/design-docs/home.md）。ライブラリの項目の一覧（GET /api/library）だけが読む。
	Unshelved bool
}

// ListsSmartFolders allows only unfiltered or favorite-only listings; sort order does not count as a filter.
func (q VideoQuery) ListsSmartFolders() bool {
	return ParseSearchQuery(q.Query).Empty() && (q.Watch == "" || q.Watch == WatchAll) && !q.PlayableOnly &&
		len(q.TagIDs) == 0 && q.Person == 0 && q.Organization == 0 && q.TagKindID == 0 &&
		q.AddedWithinDays == 0 && q.MinDurationSec == nil && q.MaxDurationSec == nil && !q.Unshelved
}

const MaxAddedWithinDays = 36500

const MaxDurationBoundSec = 360000

var (
	ErrInvalidAddedWithin   = errors.New("added within is out of range")
	ErrInvalidDurationRange = errors.New("duration range is invalid")
)

// CheckAddedAndLength shares limits between library queries and smart-folder conditions; nil means unspecified.
func CheckAddedAndLength(addedWithin, minSec, maxSec *int) error {
	if addedWithin != nil && (*addedWithin < 1 || *addedWithin > MaxAddedWithinDays) {
		return ErrInvalidAddedWithin
	}
	for _, bound := range []*int{minSec, maxSec} {
		if bound != nil && (*bound < 0 || *bound > MaxDurationBoundSec) {
			return ErrInvalidDurationRange
		}
	}
	if minSec != nil && maxSec != nil && *minSec > *maxSec {
		return ErrInvalidDurationRange
	}
	return nil
}

type VideoPage struct {
	Items []Video
	// Total is independent of pagination.
	Total      int
	NextCursor string
	// Limit reports the effective, potentially clamped page size.
	Limit         int
	MissingTagIDs []int64
}

type ScanState string

const (
	// Only one ScanRunning may exist at a time.
	ScanRunning ScanState = "running"
	// ScanDone may still contain individual file failures.
	ScanDone   ScanState = "done"
	ScanFailed ScanState = "failed"
)

type Scan struct {
	ID    int64
	State ScanState
	// Origin defaults to manual for rows predating the field.
	Origin     ScanOrigin
	StartedAt  time.Time
	FinishedAt time.Time
	Total      int
	Completed  int
	Failed     int
	// Error excludes individual file failures.
	Error string
	// ErrorCode may be empty for older failed rows.
	ErrorCode ScanErrorCode
	// ErrorPath is absolute or empty when no location applies.
	ErrorPath     string
	Videos        int
	SettledVideos int
	// SettledAt is zero until all target videos settle.
	SettledAt      time.Time
	IssuesRevision int64
	// Import is populated by internal/app, not storage.
	Import ImportProgress
	// Issues is populated by internal/app, not storage.
	Issues ScanIssueCounts
	// Activity is populated by internal/app from memory, not storage.
	Activity ScanActivity
}

func (s Scan) Tally(unregisteredIssues, failedIssues int) ImportTally {
	return ImportTally{
		State:              s.State,
		ScanTotal:          s.Total,
		ScanCompleted:      s.Completed,
		ScanFailed:         s.Failed,
		Videos:             s.Videos,
		SettledVideos:      s.SettledVideos,
		UnregisteredIssues: unregisteredIssues,
		FailedIssues:       failedIssues,
	}
}

type ScanProgress struct {
	Total     int
	Completed int
	Failed    int
}
