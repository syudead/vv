package domain

import (
	"cmp"
	"slices"
	"time"
)

// PeerFolder is one media folder as a read node lists it to its reader, in that node's own id.
type PeerFolder struct {
	ID      int64
	Path    string
	Version int64
}

// PeerMediaFolder is the reading node's snapshot of one of a link's media folders with the owner's switch, which
// applies on the reading node only.
type PeerMediaFolder struct {
	// FolderID is the folder's id on the link.
	FolderID    int64
	Path        string
	Version     int64
	Enabled     bool
	RefreshedAt time.Time
}

// PeerLinkFolders is the snapshot of a link's media folders with the link's connection state, so the folders are
// shown while the link is unreachable.
type PeerLinkFolders struct {
	State   PeerConnection
	Reason  PeerUnreachableReason
	Folders []PeerMediaFolder
}

// ReconcilePeerFolders returns the snapshot after the link listed listed: a folder the link no longer lists is
// dropped, a folder whose path or version differs is replaced with its switch on, since its id may now name
// another folder, and a new folder is added on. Every row is stamped now; changed ignores the stamps. Ids below 1
// and repeated ids after the first are ignored.
func ReconcilePeerFolders(current []PeerMediaFolder, listed []PeerFolder, now time.Time) (next []PeerMediaFolder, changed bool) {
	kept := make(map[int64]PeerMediaFolder, len(current))
	for _, row := range current {
		kept[row.FolderID] = row
	}
	next = make([]PeerMediaFolder, 0, len(listed))
	seen := make(map[int64]bool, len(listed))
	for _, folder := range listed {
		if folder.ID < 1 || seen[folder.ID] {
			continue
		}
		seen[folder.ID] = true
		row := PeerMediaFolder{FolderID: folder.ID, Path: folder.Path, Version: folder.Version, Enabled: true, RefreshedAt: now}
		if old, ok := kept[folder.ID]; ok && old.Path == folder.Path && old.Version == folder.Version {
			row.Enabled = old.Enabled
		} else {
			changed = true
		}
		next = append(next, row)
	}
	slices.SortFunc(next, func(a, b PeerMediaFolder) int { return cmp.Compare(a.FolderID, b.FolderID) })
	return next, changed || len(next) != len(current)
}

// SwitchedOffFolders returns the link's own ids of the rows switched off, in id order.
func SwitchedOffFolders(rows []PeerMediaFolder) []int64 {
	var off []int64
	for _, row := range rows {
		if !row.Enabled {
			off = append(off, row.FolderID)
		}
	}
	slices.Sort(off)
	return off
}
