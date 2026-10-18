package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const MaxHomeShelfKeys = 8

const HomeShelfItemLimit = 24

var ErrInvalidHomeShelfKey = errors.New("invalid home shelf key")

const homeShelfFolderPrefix = "folder:"

// HomeShelfKey identifies a saved smart-folder shelf.
type HomeShelfKey struct {
	FolderID int64
}

func FolderShelfKey(id int64) HomeShelfKey {
	return HomeShelfKey{FolderID: id}
}

func (k HomeShelfKey) String() string {
	return homeShelfFolderPrefix + strconv.FormatInt(k.FolderID, 10)
}

// ParseHomeShelfKey accepts only canonical positive folder IDs.
func ParseHomeShelfKey(value string) (HomeShelfKey, error) {
	if rest, ok := strings.CutPrefix(value, homeShelfFolderPrefix); ok {
		id, err := strconv.ParseInt(rest, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != rest {
			return HomeShelfKey{}, fmt.Errorf("%w: %q", ErrInvalidHomeShelfKey, value)
		}
		return FolderShelfKey(id), nil
	}
	return HomeShelfKey{}, fmt.Errorf("%w: %q", ErrInvalidHomeShelfKey, value)
}

// ParseHomeShelfKeys validates the batch size and deduplicates keys in order.
func ParseHomeShelfKeys(values []string) ([]HomeShelfKey, error) {
	if len(values) == 0 || len(values) > MaxHomeShelfKeys {
		return nil, fmt.Errorf("%w: %d keys, want 1 to %d", ErrInvalidHomeShelfKey, len(values), MaxHomeShelfKeys)
	}
	keys := make([]HomeShelfKey, 0, len(values))
	seen := make(map[HomeShelfKey]bool, len(values))
	for _, value := range values {
		key, err := ParseHomeShelfKey(value)
		if err != nil {
			return nil, err
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys, nil
}

type HomeShelf struct {
	Key  HomeShelfKey
	Name string
}

func FolderShelf(folder SmartFolder) HomeShelf {
	return HomeShelf{Key: FolderShelfKey(folder.ID), Name: folder.Name}
}

type Home struct {
	Shelves  []HomeShelf
	NoVideos bool
}

type HomeShelfItems struct {
	Key   HomeShelfKey
	Items []LibraryItem

	// Total is capped at HomeShelfItemLimit + 1.
	Total int
}

type HomeShelfPage struct {
	Shelves []HomeShelfItems
	Roots   []MediaFolder
}
