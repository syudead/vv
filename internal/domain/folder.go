package domain

import (
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxFolderPreviews = 4

var ErrInvalidFolderPath = errors.New("invalid folder path")

// Folders are derived from imported locations on each request, not persisted.
type FolderLocation struct {
	Path           string
	VideoID        int64
	ContentKey     string
	ThumbnailState ThumbnailState
	PreviewState   PreviewState
	// ThumbnailPositionMs and ThumbnailRevision version thumbnail URLs, matching Video.
	ThumbnailPositionMs    *int64
	ThumbnailRevision      int64
	ThumbnailImageRevision int64
	Builds                 ArtifactBuilds
}

type FolderPreview struct {
	VideoID                int64
	ContentKey             string
	PreviewState           PreviewState
	ThumbnailPositionMs    *int64
	ThumbnailRevision      int64
	ThumbnailImageRevision int64
	Builds                 ArtifactBuilds
}

type FolderSummary struct {
	RootID int64
	// Path is /-separated relative to the registered root, or empty for the root itself.
	Path     string
	Name     string
	RootPath string
	// VideoCount includes only direct children.
	VideoCount  int
	FolderCount int
	Previews    []FolderPreview
}

type FolderListing struct {
	Folder  FolderSummary
	Folders []FolderSummary
}

// FolderScope matches the API scope parameter; empty means FolderScopeDirect.
type FolderScope string

const (
	FolderScopeDirect  FolderScope = "direct"
	FolderScopeSubtree FolderScope = "subtree"
)

func (s FolderScope) Valid() bool {
	return s == FolderScopeDirect || s == FolderScopeSubtree
}

type FolderVideoQuery struct {
	Dir   string
	Scope FolderScope
	// VideoQuery's search, filters, sort and paging apply to the locations within Scope; Kind is ignored.
	VideoQuery
}

// ValidateFolderPath rejects empty segments, leading/trailing slashes, dot segments, and NUL to prevent escaping the root; empty denotes the root itself.
func ValidateFolderPath(rel string) error {
	if rel == "" {
		return nil
	}
	if !utf8.ValidString(rel) || strings.ContainsRune(rel, 0) {
		return ErrInvalidFolderPath
	}
	for segment := range strings.SplitSeq(rel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ErrInvalidFolderPath
		}
		// On backslash-separator platforms, backslashes inside segments would also traverse directories.
		if filepath.Separator != '/' && strings.ContainsRune(segment, filepath.Separator) {
			return ErrInvalidFolderPath
		}
	}
	return nil
}

// FolderDir requires rel to pass ValidateFolderPath.
func FolderDir(root, rel string) string {
	if rel == "" {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

// FolderName uses the final path segment, falling back to the absolute root when no segment exists.
func FolderName(root, rel string) string {
	if rel != "" {
		return rel[strings.LastIndexByte(rel, '/')+1:]
	}
	base := filepath.Base(root)
	if base == "." || base == string(filepath.Separator) || base == "/" || strings.HasSuffix(base, ":"+string(filepath.Separator)) {
		return root
	}
	return base
}

// SummarizeFolder accepts unordered descendant locations; the root always exists, while subfolders exist only when they contain locations.
func SummarizeFolder(root MediaFolder, rel string, locations []FolderLocation) (FolderListing, bool) {
	dir := FolderDir(root.Path, rel)
	sorted := slices.Clone(locations)
	slices.SortFunc(sorted, func(a, b FolderLocation) int { return strings.Compare(a.Path, b.Path) })

	self := newFolderAccumulator()
	children := map[string]*folderAccumulator{}
	found := false
	for _, location := range sorted {
		segments, ok := relativeSegments(dir, location.Path)
		if !ok || len(segments) == 0 {
			continue
		}
		found = true
		if len(segments) == 1 {
			self.addVideo(location)
			continue
		}
		child := children[segments[0]]
		if child == nil {
			child = newFolderAccumulator()
			children[segments[0]] = child
			self.folders[segments[0]] = struct{}{}
		}
		if len(segments) == 2 {
			child.addVideo(location)
		} else {
			child.folders[segments[1]] = struct{}{}
		}
	}
	if rel != "" && !found {
		return FolderListing{}, false
	}

	listing := FolderListing{
		Folder:  self.summary(root, rel),
		Folders: make([]FolderSummary, 0, len(children)),
	}
	for name, child := range children {
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		listing.Folders = append(listing.Folders, child.summary(root, childRel))
	}
	SortFolders(listing.Folders)
	return listing, true
}

// SortFolders breaks natural-name ties by original spelling, then absolute root path.
func SortFolders(folders []FolderSummary) {
	slices.SortStableFunc(folders, func(a, b FolderSummary) int {
		if order := CompareNatural(a.Name, b.Name); order != 0 {
			return order
		}
		if order := strings.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.RootPath, b.RootPath)
	})
}

// CompareNatural compares digit runs numerically and other characters case-insensitively; equal numbers such as 01 and 1 require a caller-provided tie-breaker.
func CompareNatural(a, b string) int {
	for a != "" && b != "" {
		ra, sizeA := utf8.DecodeRuneInString(a)
		rb, sizeB := utf8.DecodeRuneInString(b)
		if isDigit(ra) && isDigit(rb) {
			digitsA, restA := splitDigits(a)
			digitsB, restB := splitDigits(b)
			if order := compareDigits(digitsA, digitsB); order != 0 {
				return order
			}
			a, b = restA, restB
			continue
		}
		la, lb := unicode.ToLower(ra), unicode.ToLower(rb)
		if la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		a, b = a[sizeA:], b[sizeB:]
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	default:
		return 1
	}
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func splitDigits(s string) (digits, rest string) {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	return s[:end], s[end:]
}

func compareDigits(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func relativeSegments(dir, path string) ([]string, bool) {
	if !PathWithinRoot(dir, path) {
		return nil, false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." {
		return nil, true
	}
	return strings.Split(filepath.ToSlash(rel), "/"), true
}

type folderAccumulator struct {
	videos   map[int64]struct{}
	folders  map[string]struct{}
	previews []FolderPreview
}

func newFolderAccumulator() *folderAccumulator {
	return &folderAccumulator{videos: map[int64]struct{}{}, folders: map[string]struct{}{}}
}

// addVideo counts each video once per folder and orders previews by the first location path.
func (f *folderAccumulator) addVideo(location FolderLocation) {
	if _, seen := f.videos[location.VideoID]; seen {
		return
	}
	f.videos[location.VideoID] = struct{}{}
	if location.ThumbnailState == ThumbnailStateDone && len(f.previews) < MaxFolderPreviews {
		f.previews = append(f.previews, FolderPreview{
			VideoID: location.VideoID, ContentKey: location.ContentKey, PreviewState: location.PreviewState,
			ThumbnailPositionMs: location.ThumbnailPositionMs, ThumbnailRevision: location.ThumbnailRevision,
			ThumbnailImageRevision: location.ThumbnailImageRevision, Builds: location.Builds,
		})
	}
}

func (f *folderAccumulator) summary(root MediaFolder, rel string) FolderSummary {
	previews := f.previews
	if previews == nil {
		previews = []FolderPreview{}
	}
	return FolderSummary{
		RootID:      root.ID,
		Path:        rel,
		Name:        FolderName(root.Path, rel),
		RootPath:    root.Path,
		VideoCount:  len(f.videos),
		FolderCount: len(f.folders),
		Previews:    previews,
	}
}

type VideoFolder struct {
	RootID int64
	// Path is /-separated relative to the registered root and empty for direct children.
	Path string
}

// LocateVideoFolder follows storage's registeredLocationCondition: Windows accepts both separators and folds ASCII case; other systems use their native separator.
func LocateVideoFolder(roots []MediaFolder, locationPath string) (VideoFolder, bool) {
	return locateVideoFolderFor(roots, locationPath, runtime.GOOS == "windows", filepath.Separator)
}

func locateVideoFolderFor(roots []MediaFolder, locationPath string, windows bool, separator rune) (VideoFolder, bool) {
	return locateFor(roots, locationPath, windows, separator, true)
}

// LocateFolder follows LocateVideoFolder but retains the final segment; the root has an empty relative path.
func LocateFolder(roots []MediaFolder, folderPath string) (VideoFolder, bool) {
	return locateFolderFor(roots, folderPath, runtime.GOOS == "windows", filepath.Separator)
}

func locateFolderFor(roots []MediaFolder, folderPath string, windows bool, separator rune) (VideoFolder, bool) {
	return locateFor(roots, folderPath, windows, separator, false)
}

// LocateFolderKey also handles root keys whose trailing separators were stripped, including the empty key for /.
func LocateFolderKey(roots []MediaFolder, key string) (VideoFolder, bool) {
	return locateFolderKeyFor(roots, key, runtime.GOOS == "windows", filepath.Separator)
}

func locateFolderKeyFor(roots []MediaFolder, key string, windows bool, separator rune) (VideoFolder, bool) {
	if folder, ok := locateFolderFor(roots, key, windows, separator); ok {
		return folder, true
	}
	for _, root := range roots {
		if folderKeyFor(root.Path, windows, separator) == key {
			return VideoFolder{RootID: root.ID}, true
		}
	}
	return VideoFolder{}, false
}

func locateFor(roots []MediaFolder, path string, windows bool, separator rune, dropLast bool) (VideoFolder, bool) {
	separators := registrationSeparatorsFor(windows, separator)
	for _, root := range roots {
		rest, ok := pathBelowRoot(root.Path, path, separators, windows)
		if !ok {
			continue
		}
		segments := strings.FieldsFunc(rest, func(r rune) bool { return strings.ContainsRune(separators, r) })
		if dropLast && len(segments) > 0 {
			segments = segments[:len(segments)-1]
		}
		return VideoFolder{RootID: root.ID, Path: strings.Join(segments, "/")}, true
	}
	return VideoFolder{}, false
}

func registrationSeparatorsFor(windows bool, separator rune) string {
	if windows {
		return `/\`
	}
	return string(separator)
}

func pathBelowRoot(root, path, separators string, windows bool) (string, bool) {
	equal := func(a, b string) bool { return a == b }
	if windows {
		// ASCII-only folding matches SQLite lower() and preserves byte lengths for slicing.
		equal = func(a, b string) bool { return LowerASCII(a) == LowerASCII(b) }
	}
	if equal(path, root) {
		return "", true
	}
	trimmed := strings.TrimRight(root, separators)
	if len(path) <= len(trimmed) || !equal(path[:len(trimmed)], trimmed) {
		return "", false
	}
	if !strings.ContainsRune(separators, rune(path[len(trimmed)])) {
		return "", false
	}
	return path[len(trimmed)+1:], true
}

// LowerASCII matches SQLite lower() so Go and SQL agree on Windows root containment.
func LowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
