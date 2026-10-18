package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Groups have stable IDs and store videos by user-data key; automatic groups synchronize only folder-derived members (PlanFolderGroupSync).

// GroupNameMaxLength counts code points, matching SQLite length() and the table constraint.
const GroupNameMaxLength = 100

type Group struct {
	ID   int64
	Name string
	// OriginFolder is also empty for the automatic / group; use Automatic to distinguish manual groups.
	OriginFolder string
	Automatic    bool
	// UpdatedAt uses Unix milliseconds.
	UpdatedAt int64
	// Favorite persists even without visible videos.
	Favorite bool
}

type GroupRef struct {
	ID   int64
	Name string
}

const (
	GroupItemsDefaultLimit = 100
	GroupItemsMaxLimit     = 200
)

const (
	GroupListDefaultLimit = 100
	GroupListMaxLimit     = 200
)

type GroupListPage struct {
	// Groups includes all depths in natural-name order, breaking ties by ID.
	Groups     []LibraryGroup
	NextCursor string
	// Roots shares the groups' read snapshot.
	Roots []MediaFolder
}

// GroupItem holds exactly one of Video and Group.
type GroupItem struct {
	// ID identifies group_items, not the video or group.
	ID    int64
	Video *Video
	Group *LibraryGroup
}

type GroupPage struct {
	Group LibraryGroup
	// Items omits inaccessible videos and child groups without visible videos.
	Items []GroupItem
	// Trail is the contiguous parent-child prefix, ordered outermost first and ending at this group's parent.
	Trail      []GroupRef
	NextCursor string
	// Roots shares the items' read snapshot.
	Roots []MediaFolder
}

// GroupsChanged is emitted after commit with sorted, unique changed-group and ancestor IDs.
type GroupsChanged struct {
	GroupIDs []int64
}

func (GroupsChanged) event() {}

// ValidateGroupName wraps ErrInvalidGroupName in InvalidGroupNameError with a specific reason.
var ErrInvalidGroupName = errors.New("invalid group name")

type GroupNameProblem int

const (
	GroupNameEmpty GroupNameProblem = iota + 1
	GroupNameTooLong
)

type InvalidGroupNameError struct {
	Problem GroupNameProblem
}

func (e *InvalidGroupNameError) Error() string {
	switch e.Problem {
	case GroupNameEmpty:
		return "group name is empty"
	case GroupNameTooLong:
		return fmt.Sprintf("group name must be at most %d characters", GroupNameMaxLength)
	default:
		return ErrInvalidGroupName.Error()
	}
}

func (e *InvalidGroupNameError) Unwrap() error { return ErrInvalidGroupName }

// ValidateGroupName trims Unicode whitespace and rejects empty or overlong code-point counts; duplicate names are allowed.
func ValidateGroupName(input string) (string, error) {
	trimmed, problem := checkName(input, GroupNameMaxLength)
	switch problem {
	case nameEmpty:
		return "", &InvalidGroupNameError{Problem: GroupNameEmpty}
	case nameTooLong:
		return "", &InvalidGroupNameError{Problem: GroupNameTooLong}
	}
	return trimmed, nil
}

type nameProblem int

const (
	nameOK nameProblem = iota
	nameEmpty
	nameTooLong
)

// checkName is shared by playlist and group names.
func checkName(input string, maxLength int) (string, nameProblem) {
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", nameEmpty
	}
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", nameTooLong
	}
	return trimmed, nameOK
}

// cutGroupName truncates initial automatic names by code points.
func cutGroupName(name string) string {
	if utf8.RuneCountInString(name) <= GroupNameMaxLength {
		return name
	}
	runes := []rune(name)
	return string(runes[:GroupNameMaxLength])
}

type FolderSyncFolder struct {
	Key     string
	Name    string
	Grouped bool
	// ContentKeys contains unique direct-child user-data keys in natural filename order.
	ContentKeys []string
}

type AutoGroup struct {
	ID           int64
	OriginFolder string
	// Items excludes child groups, which synchronization never changes.
	Items      []AutoGroupItem
	Exclusions []string
}

type AutoGroupItem struct {
	ContentKey string
	FromFolder bool
}

type FolderGroupCreate struct {
	OriginFolder string
	Name         string
	// ContentKeys preserves item order; every inserted row has from_folder = 1.
	ContentKeys []string
}

type FolderGroupChange struct {
	GroupID int64
	Remove  []string
	Append  []string
}

type FolderGroupSync struct {
	// Create is ordered by folder key.
	Create []FolderGroupCreate
	// Change includes only changed groups, ordered by ID.
	Change []FolderGroupChange
}

// PlanFolderGroupSync creates eligible groups and synchronizes folder-derived members while honoring exclusions; it preserves manual members, names, order, and existing groups even when no longer eligible.
func PlanFolderGroupSync(folders []FolderSyncFolder, groups []AutoGroup, overrides map[string]FolderGroupMode) FolderGroupSync {
	byKey := make(map[string]FolderSyncFolder, len(folders))
	for _, folder := range folders {
		byKey[folder.Key] = folder
	}
	plan := FolderGroupSync{Create: []FolderGroupCreate{}, Change: []FolderGroupChange{}}
	origins := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		origins[group.OriginFolder] = struct{}{}
		if change, changed := syncAutoGroup(group, byKey[group.OriginFolder].ContentKeys); changed {
			plan.Change = append(plan.Change, change)
		}
	}
	for _, folder := range folders {
		if _, exists := origins[folder.Key]; exists || !folder.Grouped || overrides[folder.Key] == FolderGroupUngroup {
			continue
		}
		plan.Create = append(plan.Create, FolderGroupCreate{
			OriginFolder: folder.Key, Name: cutGroupName(folder.Name), ContentKeys: folder.ContentKeys,
		})
	}
	slices.SortFunc(plan.Create, func(a, b FolderGroupCreate) int { return strings.Compare(a.OriginFolder, b.OriginFolder) })
	slices.SortFunc(plan.Change, func(a, b FolderGroupChange) int { return cmp.Compare(a.GroupID, b.GroupID) })
	return plan
}

func syncAutoGroup(group AutoGroup, direct []string) (FolderGroupChange, bool) {
	directSet := make(map[string]struct{}, len(direct))
	for _, key := range direct {
		directSet[key] = struct{}{}
	}
	known := make(map[string]struct{}, len(group.Items)+len(group.Exclusions))
	change := FolderGroupChange{GroupID: group.ID}
	for _, item := range group.Items {
		known[item.ContentKey] = struct{}{}
		if _, still := directSet[item.ContentKey]; item.FromFolder && !still {
			change.Remove = append(change.Remove, item.ContentKey)
		}
	}
	for _, key := range group.Exclusions {
		known[key] = struct{}{}
	}
	for _, key := range direct {
		if _, held := known[key]; !held {
			change.Append = append(change.Append, key)
		}
	}
	return change, len(change.Remove) > 0 || len(change.Append) > 0
}
