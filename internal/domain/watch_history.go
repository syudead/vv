package domain

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Watch history records each playback using the played version's content_key.

var ErrInvalidPlaybackID = errors.New("invalid playback id")

const playbackIDLength = 36

// PlaybackID is prevalidated; empty ContentKey suppresses history, and Title snapshots the effective title.
type Play struct {
	PlaybackID string
	ContentKey string
	Title      string
}

// Video is populated only when currently accessible; otherwise retain the historical Title alone.
type WatchHistoryEntry struct {
	ID       int64
	PlayedAt time.Time
	Title    string
	Video    *Video
}

type WatchHistoryPage struct {
	Items      []WatchHistoryEntry
	NextCursor string
}

// WatchHistoryCursor stores (played_at Unix milliseconds, ID); descending pages continue with smaller pairs.
type WatchHistoryCursor struct {
	PlayedAtMs int64 `json:"p"`
	ID         int64 `json:"i"`
}

func EncodeWatchHistoryCursor(cursor WatchHistoryCursor) string {
	body, _ := json.Marshal(cursor) // Marshaling a struct containing only integers cannot fail.
	return base64.RawURLEncoding.EncodeToString(body)
}

// DecodeWatchHistoryCursor rejects malformed, missing, or nonpositive IDs.
func DecodeWatchHistoryCursor(cursor string) (WatchHistoryCursor, error) {
	body, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return WatchHistoryCursor{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	var fields struct {
		PlayedAtMs *int64 `json:"p"`
		ID         *int64 `json:"i"`
	}
	if err := json.Unmarshal(body, &fields); err != nil || fields.PlayedAtMs == nil || fields.ID == nil || *fields.ID < 1 {
		return WatchHistoryCursor{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	return WatchHistoryCursor{PlayedAtMs: *fields.PlayedAtMs, ID: *fields.ID}, nil
}

// ValidatePlaybackID checks UUID string shape only, without enforcing version or variant bits.
func ValidatePlaybackID(id string) error {
	if len(id) != playbackIDLength {
		return ErrInvalidPlaybackID
	}
	for i := range len(id) {
		c := id[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return ErrInvalidPlaybackID
			}
		default:
			if !isHexDigit(c) {
				return ErrInvalidPlaybackID
			}
		}
	}
	return nil
}

func isHexDigit(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

var ErrInvalidWatchHistoryFilter = errors.New("invalid watch history filter")

var ErrInvalidWatchHistoryPeriod = errors.New("invalid watch history period")

// WatchHistoryFilter uses current watch state and excludes unwatched because every history entry represents playback.
type WatchHistoryFilter string

const (
	// Only WatchHistoryAll includes entries without a current video.
	WatchHistoryAll        WatchHistoryFilter = "all"
	WatchHistoryInProgress WatchHistoryFilter = "inProgress"
	WatchHistoryWatched    WatchHistoryFilter = "watched"
)

func ParseWatchHistoryFilter(value string) (WatchHistoryFilter, error) {
	switch filter := WatchHistoryFilter(value); filter {
	case WatchHistoryAll, WatchHistoryInProgress, WatchHistoryWatched:
		return filter, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidWatchHistoryFilter, strconv.Quote(value))
	}
}

// Before is exclusive and ignored by date listings; empty Filter/Search applies no corresponding restriction.
type WatchHistoryQuery struct {
	Filter WatchHistoryFilter
	Search SearchExpr
	Before time.Time
}

// Day zero denotes a month.
type WatchHistoryPeriod struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseWatchHistoryPeriod requires zero-padded YYYY-MM or a real YYYY-MM-DD date.
func ParseWatchHistoryPeriod(value string) (WatchHistoryPeriod, error) {
	invalid := fmt.Errorf("%w: %s", ErrInvalidWatchHistoryPeriod, strconv.Quote(value))
	switch len(value) {
	case len("2006-01-02"):
		day, err := time.Parse("2006-01-02", value)
		if err != nil {
			return WatchHistoryPeriod{}, invalid
		}
		return WatchHistoryPeriod{Year: day.Year(), Month: day.Month(), Day: day.Day()}, nil
	case len("2006-01"):
		month, err := time.Parse("2006-01", value)
		if err != nil {
			return WatchHistoryPeriod{}, invalid
		}
		return WatchHistoryPeriod{Year: month.Year(), Month: month.Month()}, nil
	default:
		return WatchHistoryPeriod{}, invalid
	}
}

// End uses the local calendar; if midnight is skipped by DST, return the next day's first real instant so the previous day's last hour is retained.
func (p WatchHistoryPeriod) End(loc *time.Location) time.Time {
	year, month, day := p.Year, p.Month, p.Day+1
	if p.Day == 0 {
		month, day = p.Month+1, 1
	}
	end := time.Date(year, month, day, 0, 0, 0, 0, loc)
	next := time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
	if y, m, d := end.Date(); y != next.Year() || m != next.Month() || d != next.Day() {
		_, end = end.ZoneBounds()
	}
	return end
}
