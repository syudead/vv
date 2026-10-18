package domain

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
)

type ScanIssueKind string

const (
	IssueUnreadable              ScanIssueKind = "unreadable"
	IssueChangedDuringImport     ScanIssueKind = "changed_during_import"
	IssueRegisterFailed          ScanIssueKind = "register_failed"
	IssueProbeFailed             ScanIssueKind = "probe_failed"
	IssueThumbnailFailed         ScanIssueKind = "thumbnail_failed"
	IssueSeekThumbnailFailed     ScanIssueKind = "seek_thumbnail_failed"
	IssuePreviewFailed           ScanIssueKind = "preview_failed"
	IssueFingerprintFailed       ScanIssueKind = "fingerprint_failed"
	IssueThumbnailFirstFrame     ScanIssueKind = "thumbnail_first_frame"
	IssueSeekThumbnailFullDecode ScanIssueKind = "seek_thumbnail_full_decode"
)

// Kinds are ordered by severity for aggregated issues.
var scanIssueKinds = []ScanIssueKind{
	IssueUnreadable, IssueChangedDuringImport, IssueRegisterFailed,
	IssueProbeFailed, IssueThumbnailFailed, IssueSeekThumbnailFailed, IssuePreviewFailed,
	IssueFingerprintFailed, IssueThumbnailFirstFrame, IssueSeekThumbnailFullDecode,
}

func ScanIssueKinds() []ScanIssueKind {
	return slices.Clone(scanIssueKinds)
}

func (k ScanIssueKind) Valid() bool {
	return slices.Contains(scanIssueKinds, k)
}

func (k ScanIssueKind) Severity() ScanIssueSeverity {
	switch k {
	case IssueThumbnailFirstFrame, IssueSeekThumbnailFullDecode:
		return IssueSubstituted
	default:
		return IssueFailed
	}
}

// FromScan failures outside the target-video set count as unregistered files in both total and settled counts.
func (k ScanIssueKind) FromScan() bool {
	return k == IssueUnreadable || k == IssueChangedDuringImport || k == IssueRegisterFailed
}

func FailedIssueKind(kind JobKind) (ScanIssueKind, bool) {
	switch kind {
	case JobProbe:
		return IssueProbeFailed, true
	case JobThumbnail:
		return IssueThumbnailFailed, true
	case JobSeekThumbnail:
		return IssueSeekThumbnailFailed, true
	case JobPreview:
		return IssuePreviewFailed, true
	case JobFingerprint:
		return IssueFingerprintFailed, true
	}
	return "", false
}

func SubstitutedIssueKind(kind JobKind) (ScanIssueKind, bool) {
	switch kind {
	case JobThumbnail:
		return IssueThumbnailFirstFrame, true
	case JobSeekThumbnail:
		return IssueSeekThumbnailFullDecode, true
	}
	return "", false
}

type Substitution int

const (
	// SubstitutionUnknown preserves existing issue rows when reusing an artifact whose fallback status is unknown.
	SubstitutionUnknown Substitution = iota
	// SubstitutionNone removes any prior substitution issue.
	SubstitutionNone
	// SubstitutionUsed inserts a substitution issue.
	SubstitutionUsed
)

func SubstitutionOf(substituted bool) Substitution {
	if substituted {
		return SubstitutionUsed
	}
	return SubstitutionNone
}

type ScanIssueSeverity string

const (
	IssueFailed      ScanIssueSeverity = "failed"
	IssueSubstituted ScanIssueSeverity = "substituted"
)

type ScanFileIssue struct {
	VideoID int64
	Path    string
	Kind    ScanIssueKind
}

type ScanIssueRecord struct {
	VideoID int64
	// Path is the current registered representative for videos, otherwise the event-time path; videos without registered locations have an empty path.
	Path     string
	Kinds    []ScanIssueKind
	InImport bool
}

type ScanIssue struct {
	VideoID  int64
	Severity ScanIssueSeverity
	// Kinds is nonempty and severity-ordered.
	Kinds    []ScanIssueKind
	FileName string
	Folder   VideoFolder
	RootName string
	// Path is the final ordering tie-breaker and identifies unregistered files.
	Path         string
	Unregistered bool
}

type ScanIssueCounts struct {
	Failed      int
	Substituted int
	// Unregistered contributes to both total and settled counts.
	Unregistered int
}

// ScanIssues excludes paths outside registered roots and sorts by failure severity, filename, then folder.
func ScanIssues(roots []MediaFolder, records []ScanIssueRecord) []ScanIssue {
	out := make([]ScanIssue, 0, len(records))
	for _, record := range records {
		if record.Path == "" || len(record.Kinds) == 0 {
			continue
		}
		folder, ok := LocateVideoFolder(roots, record.Path)
		if !ok {
			continue
		}
		issue := ScanIssue{
			VideoID:  record.VideoID,
			Severity: IssueSubstituted,
			Kinds:    sortedIssueKinds(record.Kinds),
			FileName: filepath.Base(record.Path),
			Folder:   folder,
			Path:     record.Path,
		}
		for _, root := range roots {
			if root.ID == folder.RootID {
				issue.RootName = FolderName(root.Path, "")
				break
			}
		}
		for _, kind := range issue.Kinds {
			if kind.Severity() == IssueFailed {
				issue.Severity = IssueFailed
			}
			if kind.FromScan() && !record.InImport {
				issue.Unregistered = true
			}
		}
		out = append(out, issue)
	}
	slices.SortFunc(out, func(a, b ScanIssue) int {
		return compareIssueKeys(a.sortKey(), b.sortKey())
	})
	return out
}

func sortedIssueKinds(kinds []ScanIssueKind) []ScanIssueKind {
	out := make([]ScanIssueKind, 0, len(kinds))
	for _, kind := range scanIssueKinds {
		if slices.Contains(kinds, kind) {
			out = append(out, kind)
		}
	}
	return out
}

func CountScanIssues(issues []ScanIssue) ScanIssueCounts {
	var counts ScanIssueCounts
	for _, issue := range issues {
		if issue.Severity == IssueFailed {
			counts.Failed++
		} else {
			counts.Substituted++
		}
		if issue.Unregistered {
			counts.Unregistered++
		}
	}
	return counts
}

type scanIssueKey struct {
	Severity int    `json:"s"`
	FileName string `json:"f"`
	RootName string `json:"r"`
	Folder   string `json:"p"`
	Location string `json:"l"`
	Video    int64  `json:"v"`
}

func (i ScanIssue) sortKey() scanIssueKey {
	severity := 0
	if i.Severity != IssueFailed {
		severity = 1
	}
	return scanIssueKey{
		Severity: severity, FileName: i.FileName, RootName: i.RootName,
		Folder: i.Folder.Path, Location: i.Path, Video: i.VideoID,
	}
}

func compareIssueKeys(a, b scanIssueKey) int {
	return cmp.Or(
		cmp.Compare(a.Severity, b.Severity),
		cmp.Compare(a.FileName, b.FileName),
		cmp.Compare(a.RootName, b.RootName),
		cmp.Compare(a.Folder, b.Folder),
		cmp.Compare(a.Location, b.Location),
		cmp.Compare(a.Video, b.Video),
	)
}

type ScanIssuePage struct {
	ScanID     int64
	Items      []ScanIssue
	NextCursor string
}

// PageScanIssues uses the last item's sort key to avoid duplicates after additions/deletions; malformed cursors yield ErrInvalidCursor.
func PageScanIssues(issues []ScanIssue, cursor string, limit int) ([]ScanIssue, string, error) {
	start := 0
	if cursor != "" {
		after, err := decodeIssueCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		start, _ = slices.BinarySearchFunc(issues, after, func(issue ScanIssue, key scanIssueKey) int {
			return compareIssueKeys(issue.sortKey(), key)
		})
		for start < len(issues) && compareIssueKeys(issues[start].sortKey(), after) <= 0 {
			start++
		}
	}
	end := min(len(issues), start+max(1, limit))
	page := issues[start:end]
	if end >= len(issues) {
		return page, "", nil
	}
	next, err := encodeIssueCursor(page[len(page)-1].sortKey())
	if err != nil {
		return nil, "", err
	}
	return page, next, nil
}

func encodeIssueCursor(key scanIssueKey) (string, error) {
	body, err := json.Marshal(key)
	if err != nil {
		return "", fmt.Errorf("cannot encode the issue cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func decodeIssueCursor(cursor string) (scanIssueKey, error) {
	body, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return scanIssueKey{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	var key scanIssueKey
	if err := json.Unmarshal(body, &key); err != nil || (key.Severity != 0 && key.Severity != 1) {
		return scanIssueKey{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	return key, nil
}
