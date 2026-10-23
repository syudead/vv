package domain

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// PlaylistNameMaxLength counts code points, matching SQLite length() and the table constraint.
const PlaylistNameMaxLength = 100

// Covers is populated only by ListPlaylists: up to MaxFolderPreviews ready thumbnails in item order, taking one ready member per group.
type Playlist struct {
	ID        int64
	Name      string
	ItemCount int
	UpdatedAt time.Time
	Covers    []FolderPreview
}

// PlaylistPage orders by (updated_at desc, id desc); clients must restart pagination when Version changes because edits can move items ahead of the cursor.
type PlaylistPage struct {
	Items      []Playlist
	Total      int
	NextCursor string
	Version    string
}

// PlaylistCursor stores Unix milliseconds and ID; descending pagination continues with smaller pairs.
type PlaylistCursor struct {
	UpdatedAtMs int64 `json:"u"`
	ID          int64 `json:"i"`
}

func EncodePlaylistCursor(cursor PlaylistCursor) string {
	body, _ := json.Marshal(cursor) // Marshaling a struct containing only integers cannot fail.
	return base64.RawURLEncoding.EncodeToString(body)
}

// DecodePlaylistCursor rejects malformed, missing, or nonpositive IDs.
func DecodePlaylistCursor(cursor string) (PlaylistCursor, error) {
	body, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return PlaylistCursor{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	var fields struct {
		UpdatedAtMs *int64 `json:"u"`
		ID          *int64 `json:"i"`
	}
	if err := json.Unmarshal(body, &fields); err != nil || fields.UpdatedAtMs == nil || fields.ID == nil || *fields.ID < 1 {
		return PlaylistCursor{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	return PlaylistCursor{UpdatedAtMs: *fields.UpdatedAtMs, ID: *fields.ID}, nil
}

// Visible PlaylistItems contain exactly one owner-view Video, Group, or SmartFolder; hidden playback-order entries contain none.
type PlaylistItem struct {
	ID          int64
	Video       *Video
	Group       *LibraryGroup
	SmartFolder *LibrarySmartFolder
}

// PlaylistAdd appends videos, groups, then smart folders.
type PlaylistAdd struct {
	VideoIDs       []int64
	GroupIDs       []int64
	SmartFolderIDs []int64
}

// PlaylistAdded excludes nonexistent targets from both Added and Existing.
type PlaylistAdded struct {
	Added    int
	Existing int
}

var ErrPlaylistNotFound = fmt.Errorf("playlist %w", ErrNotFound)

var ErrPlaylistItemNotFound = fmt.Errorf("playlist item %w", ErrNotFound)

var ErrInvalidPlaylistName = errors.New("invalid playlist name")

type PlaylistNameProblem int

const (
	PlaylistNameEmpty PlaylistNameProblem = iota + 1
	PlaylistNameTooLong
)

type InvalidPlaylistNameError struct {
	Problem PlaylistNameProblem
}

func (e *InvalidPlaylistNameError) Error() string {
	switch e.Problem {
	case PlaylistNameEmpty:
		return "playlist name is empty"
	case PlaylistNameTooLong:
		return fmt.Sprintf("playlist name must be at most %d characters", PlaylistNameMaxLength)
	default:
		return ErrInvalidPlaylistName.Error()
	}
}

func (e *InvalidPlaylistNameError) Unwrap() error { return ErrInvalidPlaylistName }

// ValidatePlaylistName trims Unicode whitespace, permits duplicate names, and rejects empty or overlong code-point counts.
func ValidatePlaylistName(input string) (string, error) {
	trimmed, problem := checkName(input, PlaylistNameMaxLength)
	switch problem {
	case nameEmpty:
		return "", &InvalidPlaylistNameError{Problem: PlaylistNameEmpty}
	case nameTooLong:
		return "", &InvalidPlaylistNameError{Problem: PlaylistNameTooLong}
	}
	return trimmed, nil
}

type PlaylistStep struct {
	ItemID        int64
	Video         Video
	SmartFolderID int64
}

// Position is one-based among visible items, or zero for a hidden current item; neighbor steps point to each visible item's starting video.
// Next, the continuous-playback step, passes over virtual videos; the manual PreviousItem and NextItem do not.
type PlaylistPlayback struct {
	Playlist Playlist
	ItemID   int64
	Position int
	Count    int
	// PlayableCount counts visible items whose starting video has a file.
	PlayableCount int
	Next          *PlaylistStep
	PreviousItem  *PlaylistStep
	NextItem      *PlaylistStep
}

type PlaylistCurrent struct {
	ItemID  int64
	VideoID int64
}

func (i PlaylistItem) Shown() bool {
	return i.Video != nil || i.Group != nil || i.SmartFolder != nil
}

func (i PlaylistItem) playOrder() *LibraryGroup {
	switch {
	case i.Group != nil:
		return i.Group
	case i.SmartFolder != nil:
		return &i.SmartFolder.LibraryGroup
	}
	return nil
}

// start uses GroupOpenIndex, falling back to the first member; no playable video yields nil.
func (i PlaylistItem) start() *PlaylistStep {
	if i.Video != nil {
		return i.step(*i.Video)
	}
	group := i.playOrder()
	if group == nil || len(group.Members) == 0 {
		return nil
	}
	member := group.Members[0]
	for _, candidate := range group.Members {
		if candidate.ID == group.OpenVideoID {
			member = candidate
			break
		}
	}
	return i.step(member)
}

// playableStart is the item's starting step when that video has a file.
func (i PlaylistItem) playableStart() *PlaylistStep {
	if step := i.start(); step != nil && !step.Video.Virtual() {
		return step
	}
	return nil
}

func (i PlaylistItem) step(video Video) *PlaylistStep {
	step := &PlaylistStep{ItemID: i.ID, Video: video}
	if i.SmartFolder != nil {
		step.SmartFolderID = i.SmartFolder.ID
	}
	return step
}

// PlaylistPlaybackOf advances through current group/folder members including watched ones, then visible items; hidden current items retain their position for navigation, missing ItemID yields ErrPlaylistItemNotFound, and callers fill Playlist.
func PlaylistPlaybackOf(items []PlaylistItem, current PlaylistCurrent) (PlaylistPlayback, error) {
	at := -1
	for i, item := range items {
		if item.ID == current.ItemID {
			at = i
			break
		}
	}
	if at < 0 {
		return PlaylistPlayback{}, ErrPlaylistItemNotFound
	}

	playback := PlaylistPlayback{ItemID: current.ItemID}
	for i, item := range items {
		if !item.Shown() {
			continue
		}
		playback.Count++
		if i == at {
			playback.Position = playback.Count
		}
		if item.playableStart() != nil {
			playback.PlayableCount++
		}
	}
	for i := at - 1; i >= 0 && playback.PreviousItem == nil; i-- {
		if items[i].Shown() {
			playback.PreviousItem = items[i].start()
		}
	}
	for i := at + 1; i < len(items) && playback.NextItem == nil; i++ {
		if items[i].Shown() {
			playback.NextItem = items[i].start()
		}
	}
	playback.Next = nextPlayable(items, at, current.VideoID)
	return playback, nil
}

// nextPlayable continues with the current group's later members, then the following items' starts, passing over virtual videos.
func nextPlayable(items []PlaylistItem, at int, videoID int64) *PlaylistStep {
	if group := items[at].playOrder(); group != nil {
		for j, member := range group.Members {
			if member.ID != videoID {
				continue
			}
			for _, later := range group.Members[j+1:] {
				if !later.Virtual() {
					return items[at].step(later)
				}
			}
			break
		}
	}
	for _, item := range items[at+1:] {
		if step := item.playableStart(); step != nil {
			return step
		}
	}
	return nil
}
