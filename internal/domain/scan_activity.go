package domain

import "path/filepath"

type ScanActivityKind string

const (
	ActivityRegistering   ScanActivityKind = "registering"
	ActivityProbe         ScanActivityKind = "probe"
	ActivityThumbnail     ScanActivityKind = "thumbnail"
	ActivitySeekThumbnail ScanActivityKind = "seekThumbnail"
	ActivityPreview       ScanActivityKind = "preview"
	ActivityFingerprint   ScanActivityKind = "fingerprint"
)

func ActivityKindOf(kind JobKind) (ScanActivityKind, bool) {
	switch kind {
	case JobProbe:
		return ActivityProbe, true
	case JobThumbnail:
		return ActivityThumbnail, true
	case JobSeekThumbnail:
		return ActivitySeekThumbnail, true
	case JobPreview:
		return ActivityPreview, true
	case JobFingerprint:
		return ActivityFingerprint, true
	}
	return "", false
}

// ScanActivity is held only in internal/app memory; empty Kind means idle.
type ScanActivity struct {
	Kind    ScanActivityKind
	VideoID int64
	Path    string
	// Folder is valid only when Located and is resolved against current roots at read time.
	Folder   VideoFolder
	RootName string
	Located  bool
}

func (a ScanActivity) Active() bool {
	return a.Kind != ""
}

func (a ScanActivity) FileName() string {
	return filepath.Base(a.Path)
}

func (a ScanActivity) Locate(roots []MediaFolder) ScanActivity {
	folder, ok := LocateVideoFolder(roots, a.Path)
	if !ok {
		return a
	}
	a.Folder, a.Located = folder, true
	for _, root := range roots {
		if root.ID == folder.RootID {
			a.RootName = FolderName(root.Path, "")
			break
		}
	}
	return a
}
