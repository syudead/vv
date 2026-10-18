package domain

import "time"

// LibraryItem contains exactly one of Video, Group, or SmartFolder; SmartFolder is a summary without card details.
type LibraryItem struct {
	Video       *Video
	Group       *LibraryGroup
	SmartFolder *SmartFolderSummary
}

type LibraryPage struct {
	Items         []LibraryItem
	Total         int
	NextCursor    string
	Limit         int
	MissingTagIDs []int64
	// Roots shares the items' read snapshot and supplies response VideoFolder values.
	Roots []MediaFolder
}

// LibrarySelection is unpaginated and separates video IDs from group IDs and their playback-order members (docs/design-docs/catalog-organization.md).
type LibrarySelection struct {
	VideoIDs []int64
	Groups   []LibraryGroupSelection
	// Smart-folder videos are excluded from AllVideoIDs.
	SmartFolderIDs []int64
}

type LibraryGroupSelection struct {
	ID       int64
	VideoIDs []int64
}

func (s LibrarySelection) AllVideoIDs() []int64 {
	ids := append([]int64{}, s.VideoIDs...)
	for _, group := range s.Groups {
		ids = append(ids, group.VideoIDs...)
	}
	return ids
}

// LibraryGroup aggregates all visible descendants in playback order regardless of filters.
type LibraryGroup struct {
	ID int64
	// Path stores the automatic group's FolderKey; response folders are resolved only under registered roots.
	Path string
	Name string
	// Members expands child groups depth-first, keeping each visible video once.
	Members      []Video
	WatchState   WatchState
	WatchedCount int
	OpenVideoID  int64
	// DurationMs is nil when every member has unknown duration.
	DurationMs   *int64
	SizeBytes    int64
	AddedAt      time.Time
	LastPlayedAt *time.Time
	// Favorite belongs to the group independently of member favorites and is populated by storage.
	Favorite bool
}

// progress must align with members; guests receive all-nil progress and therefore open the first member.
func NewLibraryGroup(id int64, path, name string, members []Video, progress []*Progress) LibraryGroup {
	group := LibraryGroup{ID: id, Path: path, Name: name, Members: members}
	group.WatchState, group.WatchedCount = GroupWatch(progress)
	if index := GroupOpenIndex(progress); index < len(members) {
		group.OpenVideoID = members[index].ID
	}
	for i, member := range members {
		group.SizeBytes += member.SizeBytes
		if member.DurationMs != nil {
			total := *member.DurationMs
			if group.DurationMs != nil {
				total += *group.DurationMs
			}
			group.DurationMs = &total
		}
		if member.AddedAt.After(group.AddedAt) {
			group.AddedAt = member.AddedAt
		}
		if i < len(progress) && progress[i] != nil {
			played := progress[i].UpdatedAt
			if group.LastPlayedAt == nil || played.After(*group.LastPlayedAt) {
				group.LastPlayedAt = &played
			}
		}
	}
	return group
}

func memberStarted(progress *Progress) bool {
	return ClassifyWatch(progress) != WatchStateUnwatched
}

// GroupWatch must stay consistent with the watch-state SQL in internal/store's libraryItemsCTE.
func GroupWatch(progress []*Progress) (WatchState, int) {
	started, watched := 0, 0
	for _, p := range progress {
		if memberStarted(p) {
			started++
		}
		if ClassifyWatch(p) == WatchStateWatched {
			watched++
		}
	}
	return WatchStateOf(len(progress), started, watched), watched
}

func WatchStateOf(count, started, watched int) WatchState {
	switch {
	case started == 0:
		return WatchStateUnwatched
	case watched == count:
		return WatchStateWatched
	default:
		return WatchStateInProgress
	}
}

// GroupOpenIndex chooses the first in-progress member, then first unfinished member, then zero (also for empty groups).
func GroupOpenIndex(progress []*Progress) int {
	for i, p := range progress {
		if ClassifyWatch(p) == WatchStateInProgress {
			return i
		}
	}
	for i, p := range progress {
		if ClassifyWatch(p) != WatchStateWatched {
			return i
		}
	}
	return 0
}
