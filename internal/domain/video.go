package domain

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

var ErrUnprocessableMedia = errors.New("video cannot be transcoded live")

var ErrSeekFrameUnavailable = errors.New("cannot extract frames for the seek preview")

type ProbeState string

const (
	ProbeStatePending ProbeState = "pending"
	ProbeStateDone    ProbeState = "done"
	ProbeStateFailed  ProbeState = "failed"
)

type ThumbnailState string

const (
	ThumbnailStatePending ThumbnailState = "pending"
	ThumbnailStateDone    ThumbnailState = "done"
	ThumbnailStateFailed  ThumbnailState = "failed"
)

// PreviewState is the lifecycle of the content-keyed hover preview asset.
type PreviewState string

const (
	PreviewStatePending PreviewState = "pending"
	PreviewStateDone    PreviewState = "done"
	PreviewStateFailed  PreviewState = "failed"
)

type UnplayableReason string

const (
	ReasonContainer  UnplayableReason = "container"
	ReasonVideoCodec UnplayableReason = "video_codec"
	ReasonAudioCodec UnplayableReason = "audio_codec"
)

// Use allowlists to avoid promising playback for unknown formats; HEVC and MKV remain excluded despite support in some browsers.
var (
	allowedContainers = map[string]struct{}{
		"mp4": {}, "m4v": {}, "webm": {},
	}
	allowedVideoCodecs = map[string]struct{}{
		"h264": {}, "vp8": {}, "vp9": {}, "av1": {},
	}
	// An absent audio stream is allowed.
	allowedAudioCodecs = map[string]struct{}{
		"aac": {}, "mp3": {}, "opus": {}, "vorbis": {},
	}
)

// Probe contains ffprobe facts; domain rules interpret them while external execution remains in internal/media.
type Probe struct {
	// Unknown duration is zero here and stored as null, preserving the distinction from a known zero duration.
	DurationMs int64
	// Dimensions reflect rotation, or zero when unknown.
	Width  int
	Height int
	// DisplayAspectRatio includes rotation and pixel aspect, or zero when unknown.
	DisplayAspectRatio float64
	VideoCodec         string
	AudioCodec         string
	// FormatName is diagnostic only because ffprobe groups MOV/MP4 and related formats together.
	FormatName string
	// Transcode is nil without a non-attached video stream with valid dimensions and is then not persisted.
	Transcode *TranscodeProbe
	// Source is the pre-probe file stamp used to validate persisted transcode metadata.
	Source FileStamp
	// Keyframes is an optional bounded sample for live-transcode seek decisions.
	Keyframes *KeyframeProbe
	// Import probes always populate Media from the same ffprobe output.
	Media *MediaInfo
}

// Reason is empty exactly when Playable is true.
type Playability struct {
	Playable bool
	Reason   UnplayableReason
}

// EvaluatePlayability reports the first failed check in container/video/audio order; import persists the result for later listing.
func EvaluatePlayability(container string, probe Probe) Playability {
	if _, ok := allowedContainers[normalizeCodecName(container)]; !ok {
		return Playability{Reason: ReasonContainer}
	}
	if _, ok := allowedVideoCodecs[normalizeCodecName(probe.VideoCodec)]; !ok {
		return Playability{Reason: ReasonVideoCodec}
	}
	if audio := normalizeCodecName(probe.AudioCodec); audio != "" {
		if _, ok := allowedAudioCodecs[audio]; !ok {
			return Playability{Reason: ReasonAudioCodec}
		}
	}
	return Playability{Playable: true}
}

// Use the extension because ffprobe format_name conflates MOV and MP4; delivery Content-Type uses the same source.
func ContainerFromPath(path string) string {
	return normalizeCodecName(strings.TrimPrefix(filepath.Ext(path), "."))
}

func normalizeCodecName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Unknown numeric values stay nil so listings distinguish unknown values from zero.
type Video struct {
	ID        int64
	Path      string
	Title     string
	SizeBytes int64
	MTime     time.Time
	AddedAt   time.Time
	// IndexedAt changes during background processing and is never exposed through the API.
	IndexedAt time.Time
	// EditedAt backs API updatedAt and is coalesced from video_edits.edited_at and videos.added_at, independently of IndexedAt.
	EditedAt time.Time
	// FileCreatedAt falls back to MTime when creation time is unavailable.
	FileCreatedAt time.Time

	// ContentKey preserves identity across moves/renames and keys progress and thumbnail names.
	ContentKey string
	// UserKey is the bundle's user_key for members, otherwise ContentKey; it is empty without ContentKey.
	UserKey string
	// Versions is populated only by detail reads for bundle members, never listings.
	Versions *VideoVersionsRef
	// WorkInfo is populated by detail/external reads and work-info write results, not listings.
	WorkInfo *WorkInfo
	// MediaInfo is populated only by single-video detail reads with a current-version stored row.
	MediaInfo *MediaInfo

	DurationMs         *int64
	Width              *int
	Height             *int
	DisplayAspectRatio *float64

	Container  string
	VideoCodec string
	AudioCodec string

	Playable         bool
	UnplayableReason UnplayableReason

	ProbeState ProbeState
	ProbeError string
	// Older failed rows may have no ProbeErrorCode.
	ProbeErrorCode ProbeErrorCode
	ThumbnailState ThumbnailState
	// SeekThumbnailState is the persisted state before artifact-existence verification.
	SeekThumbnailState SeekThumbnailState
	PreviewState       PreviewState

	// Public is always false without ContentKey.
	Public bool

	// Favorite uses the user-data key and is always false without ContentKey.
	Favorite bool

	// FileTitle comes from the representative location; Title prefers DisplayName when set.
	FileTitle           string
	DisplayName         string
	ThumbnailPositionMs *int64
	// ThumbnailRevision versions URLs only and is not a response field; zero means unset.
	ThumbnailRevision int64
	// A nonzero ThumbnailImageRevision replaces the position revision in thumbnail URLs.
	ThumbnailImageRevision int64
	ThumbnailImageOrigin   ThumbnailImageOrigin
	// Builds count forced rebuilds of each artifact and version its URLs only; zero means never rebuilt.
	Builds ArtifactBuilds
}

// ArtifactBuilds counts the forced rebuilds of a video's thumbnail, seek sprite and hover preview.
type ArtifactBuilds struct {
	Thumbnail     int64
	SeekThumbnail int64
	Preview       int64
}

// PlayableInBrowser requires completed probing even when Playable is set.
func (v Video) PlayableInBrowser() bool {
	return v.Playable && v.ProbeState == ProbeStateDone
}

func (v Video) HasThumbnail() bool {
	return v.ThumbnailState == ThumbnailStateDone
}

// VideoFile contains scan facts only, excluding ffprobe metadata.
type VideoFile struct {
	Path       string
	Title      string
	ContentKey string
	SizeBytes  int64
	MTime      time.Time
	// Zero FileCreatedAt means unavailable.
	FileCreatedAt time.Time
	Container     string
	// AddedAt applies only to new videos and defaults to import time when zero.
	AddedAt time.Time
	// Decider records the scan initiator for location-only imports; empty means rule.
	Decider ActivityDecider
}

type IndexedVideo struct {
	ID              int64
	ContentKey      string
	LocationID      int64
	LocationVersion int64
	SizeBytes       int64
	MTime           time.Time
	// Zero FileCreatedAt maps to null.
	FileCreatedAt      time.Time
	ProbeState         ProbeState
	ThumbnailState     ThumbnailState
	SeekThumbnailState SeekThumbnailState
	PreviewState       PreviewState
	// FingerprintMissing schedules rebuilding a missing/current-version fingerprint for an already-complete sprite.
	FingerprintMissing bool
}

// MediaFolder is one independently managed scan root.
type MediaFolder struct {
	ID        int64
	Path      string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// VideoLocation is one current filesystem location for a logical video.
type VideoLocation struct {
	ID        int64
	VideoID   int64
	Path      string
	Version   int64
	Title     string
	SizeBytes int64
	MTime     time.Time
	// Zero FileCreatedAt maps to null.
	FileCreatedAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UpsertOutcome string

const (
	OutcomeAdded     UpsertOutcome = "added"
	OutcomeUpdated   UpsertOutcome = "updated"
	OutcomeMoved     UpsertOutcome = "moved"
	OutcomeUnchanged UpsertOutcome = "unchanged"
)

type UpsertResult struct {
	ID                 int64
	Outcome            UpsertOutcome
	NeedsProbe         bool
	NeedsThumbnail     bool
	NeedsSeekThumbnail bool
	NeedsPreview       bool
}

// Seek-thumbnail URL and state are exposed only when HasSeekThumbnail is true.
func (v Video) HasSeekThumbnail() bool {
	return v.ProbeState == ProbeStateDone && v.DurationMs != nil &&
		*v.DurationMs > 0 && v.VideoCodec != "" && v.ContentKey != ""
}

type SeekThumbnailState string

const (
	SeekThumbnailPending SeekThumbnailState = "pending"
	SeekThumbnailDone    SeekThumbnailState = "done"
	SeekThumbnailFailed  SeekThumbnailState = "failed"
)

// PreviewAvailable reflects current file existence; missing artifacts queued for regeneration change PreviewState to pending.
type VideoView struct {
	Video            Video
	PreviewAvailable bool
}

type DeletedVideo struct {
	ID         int64
	ContentKey string
}

// ErrPreviewStale means nothing was published and the job may safely retry after source changes.
var ErrPreviewStale = errors.New("preview source identity changed")

// The one-second minimum provides useful short-video previews; long videos spread frames, and persisted layout intervals keep older sprites readable.
const SeekThumbnailInterval = 1 * time.Second
