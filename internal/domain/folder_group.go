package domain

import (
	"cmp"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Bump FolderIndexVersion when BuildFolderIndex or FolderKey rules change; startup rebuilds mismatched persisted indexes.
const FolderIndexVersion = 3

type FolderGroupMode string

const (
	FolderGroupUngroup FolderGroupMode = "ungroup"
	// FolderGroupDirect includes roots and folders with child directories.
	FolderGroupDirect FolderGroupMode = "group_direct"
)

func (m FolderGroupMode) Valid() bool {
	return m == FolderGroupUngroup || m == FolderGroupDirect
}

type FolderGrouping struct {
	// An empty Mode selects automatic grouping.
	Mode FolderGroupMode
	// GroupID is zero without an automatic group for this folder.
	GroupID int64
}

type FolderIndexLocation struct {
	VideoID int64
	Path    string
}

type FolderGroupAssignment struct {
	Key string
	// Path uses the first location's spelling, or the registered path for a root.
	Path     string
	Name     string
	TitleKey string
	VideoIDs []int64
}

type FolderIndex struct {
	// Groups is key-sorted; each video belongs to at most one group.
	Groups []FolderGroupAssignment
	// FolderNames contains normalized, unique, sorted ancestor names; videos without names are omitted.
	FolderNames map[int64][]string
	// Folders includes ungrouped folders with direct locations, key-sorted for PlanFolderGroupSync.
	Folders []FolderVideos
}

type FolderVideos struct {
	Key     string
	Name    string
	Grouped bool
	// VideoIDs includes each directly located video once, in natural filename order, regardless of representative location.
	VideoIDs []int64
}

// FolderKey strips trailing separators and normalizes Windows separators and ASCII case consistently with LocateVideoFolder.
func FolderKey(path string) string {
	return folderKeyFor(path, runtime.GOOS == "windows", filepath.Separator)
}

func folderKeyFor(path string, windows bool, separator rune) string {
	key := strings.TrimRight(path, registrationSeparatorsFor(windows, separator))
	if windows {
		key = LowerASCII(strings.ReplaceAll(key, "/", `\`))
	}
	return key
}

// BuildFolderIndex accepts unordered locations and ignores those outside registered roots.
func BuildFolderIndex(roots []MediaFolder, locations []FolderIndexLocation, overrides map[string]FolderGroupMode) FolderIndex {
	return buildFolderIndexFor(roots, locations, overrides, runtime.GOOS == "windows", filepath.Separator)
}

type folderIndexLocation struct {
	videoID int64
	path    string
	root    MediaFolder
	// dirs spans the registered root (depth zero) through the location's parent.
	dirs     []string
	segments []string
}

type folderState struct {
	path        string
	root        MediaFolder
	rel         string
	hasChild    bool
	direct      []siblingEntry
	located     []siblingEntry
	locatedSeen map[int64]struct{}
}

func buildFolderIndexFor(roots []MediaFolder, locations []FolderIndexLocation, overrides map[string]FolderGroupMode, windows bool, separator rune) FolderIndex {
	separators := registrationSeparatorsFor(windows, separator)
	rootKeys := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		rootKeys[folderKeyFor(root.Path, windows, separator)] = struct{}{}
	}

	placed := make([]folderIndexLocation, 0, len(locations))
	for _, location := range locations {
		if entry, ok := placeLocation(roots, location, separators, windows); ok {
			placed = append(placed, entry)
		}
	}
	// Byte-sorted paths choose each video's representative and each folder's spelling deterministically.
	slices.SortFunc(placed, func(a, b folderIndexLocation) int {
		if order := strings.Compare(a.path, b.path); order != 0 {
			return order
		}
		return cmp.Compare(a.videoID, b.videoID)
	})

	folders := map[string]*folderState{}
	folderOf := func(entry folderIndexLocation, depth int) *folderState {
		key := folderKeyFor(entry.dirs[depth], windows, separator)
		state := folders[key]
		if state == nil {
			state = &folderState{
				path: entry.dirs[depth],
				root: entry.root,
				rel:  strings.Join(entry.segments[:depth], "/"),
			}
			folders[key] = state
		}
		return state
	}

	represented := map[int64]struct{}{}
	names := map[int64][]string{}
	for _, entry := range placed {
		parent := len(entry.dirs) - 1
		for depth := range parent {
			folderOf(entry, depth).hasChild = true
		}
		parentState := folderOf(entry, parent)
		fileName := entry.segments[len(entry.segments)-1]
		if _, seen := parentState.locatedSeen[entry.videoID]; !seen {
			if parentState.locatedSeen == nil {
				parentState.locatedSeen = map[int64]struct{}{}
			}
			parentState.locatedSeen[entry.videoID] = struct{}{}
			parentState.located = append(parentState.located, siblingEntry{id: entry.videoID, name: fileName})
		}
		if _, seen := represented[entry.videoID]; !seen {
			represented[entry.videoID] = struct{}{}
			parentState.direct = append(parentState.direct, siblingEntry{id: entry.videoID, name: fileName})
		}
		// Ancestor names come from all locations, not just representatives.
		for _, segment := range entry.segments[:len(entry.segments)-1] {
			name, err := NormalizeTagName(segment)
			if err != nil {
				continue
			}
			if !slices.Contains(names[entry.videoID], name) {
				names[entry.videoID] = append(names[entry.videoID], name)
			}
		}
	}

	index := FolderIndex{Groups: []FolderGroupAssignment{}, FolderNames: make(map[int64][]string, len(names)), Folders: []FolderVideos{}}
	for key, state := range folders {
		grouped := groupsFolder(key, state, overrides, rootKeys)
		name := FolderName(state.root.Path, state.rel)
		if len(state.located) > 0 {
			index.Folders = append(index.Folders, FolderVideos{Key: key, Name: name, Grouped: grouped, VideoIDs: sortedSiblingIDs(state.located)})
		}
		if !grouped {
			continue
		}
		index.Groups = append(index.Groups, FolderGroupAssignment{
			Key: key, Path: state.path, Name: name, TitleKey: NaturalSortKey(name), VideoIDs: sortedSiblingIDs(state.direct),
		})
	}
	slices.SortFunc(index.Groups, func(a, b FolderGroupAssignment) int { return strings.Compare(a.Key, b.Key) })
	slices.SortFunc(index.Folders, func(a, b FolderVideos) int { return strings.Compare(a.Key, b.Key) })
	for videoID, list := range names {
		slices.Sort(list)
		index.FolderNames[videoID] = list
	}
	return index
}

func groupsFolder(key string, state *folderState, overrides map[string]FolderGroupMode, rootKeys map[string]struct{}) bool {
	if len(state.direct) < 2 {
		return false
	}
	mode, overridden := overrides[key]
	if overridden {
		return mode == FolderGroupDirect
	}
	_, isRoot := rootKeys[key]
	return !state.hasChild && !isRoot
}

func placeLocation(roots []MediaFolder, location FolderIndexLocation, separators string, windows bool) (folderIndexLocation, bool) {
	for _, root := range roots {
		rest, ok := pathBelowRoot(root.Path, location.Path, separators, windows)
		if !ok {
			continue
		}
		offset := len(location.Path) - len(rest)
		entry := folderIndexLocation{videoID: location.VideoID, path: location.Path, root: root, dirs: []string{root.Path}}
		start, previousEnd := 0, 0
		for i := 0; i <= len(rest); i++ {
			if i < len(rest) && !strings.ContainsRune(separators, rune(rest[i])) {
				continue
			}
			if i > start {
				if len(entry.segments) > 0 {
					entry.dirs = append(entry.dirs, location.Path[:offset+previousEnd])
				}
				entry.segments = append(entry.segments, rest[start:i])
				previousEnd = i
			}
			start = i + 1
		}
		if len(entry.segments) == 0 {
			// A location pointing to the root itself is not a file.
			return folderIndexLocation{}, false
		}
		return entry, true
	}
	return folderIndexLocation{}, false
}

func sortedSiblingIDs(entries []siblingEntry) []int64 {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, compareSiblings)
	ids := make([]int64, 0, len(sorted))
	for _, entry := range sorted {
		ids = append(ids, entry.id)
	}
	return ids
}

type siblingEntry struct {
	id   int64
	name string
}

// compareSiblings breaks natural filename ties by bytes, then video ID.
func compareSiblings(a, b siblingEntry) int {
	if order := CompareNatural(a.name, b.name); order != 0 {
		return order
	}
	if order := strings.Compare(a.name, b.name); order != 0 {
		return order
	}
	return cmp.Compare(a.id, b.id)
}
