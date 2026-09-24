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

// MaxFolderPreviews はフォルダカードに差し込むサムネイルの上限である。
const MaxFolderPreviews = 4

// ErrInvalidFolderPath はフォルダの相対パスが規則に反することを表す。
var ErrInvalidFolderPath = errors.New("フォルダの指定が正しくありません")

// FolderLocation はフォルダの集計に要る所在1件である。フォルダは保存せず、
// 取り込み済みの所在のパスから要求のたびに導く。
type FolderLocation struct {
	// Path は所在の絶対パス。
	Path           string
	VideoID        int64
	ContentKey     string
	ThumbnailState ThumbnailState
}

// FolderPreview はフォルダカードに差し込むサムネイル1件である。
type FolderPreview struct {
	VideoID    int64
	ContentKey string
}

// FolderSummary はフォルダカード1枚分の集計である。
type FolderSummary struct {
	RootID int64
	// Path は登録フォルダからの `/` 区切りの相対パス。登録フォルダ自身は空文字。
	Path     string
	Name     string
	RootPath string
	// VideoCount は直下の動画の件数。孫以降のフォルダの動画は数えない。
	VideoCount int
	// FolderCount は直下の子フォルダの件数。
	FolderCount int
	// Previews は直下の動画のうちサムネイル生成済みのもの。
	Previews []FolderPreview
}

// FolderListing は開いたフォルダと、その直下の子フォルダである。
type FolderListing struct {
	Folder  FolderSummary
	Folders []FolderSummary
}

// FolderScope はフォルダの動画の問い合わせで対象にする所在の範囲である。
// 値は api/openapi.yaml の scope パラメータに対応する。空は FolderScopeDirect と
// 同じに扱う。
type FolderScope string

const (
	// FolderScopeDirect はフォルダ直下の所在だけを対象にする（既定）。
	FolderScopeDirect FolderScope = "direct"
	// FolderScopeSubtree はフォルダとその配下すべての所在を対象にする。
	FolderScopeSubtree FolderScope = "subtree"
)

// Valid は既知の値かどうかを返す。
func (s FolderScope) Valid() bool {
	return s == FolderScopeDirect || s == FolderScopeSubtree
}

// FolderVideoQuery はフォルダの動画の問い合わせ条件である。
type FolderVideoQuery struct {
	// Dir はフォルダの絶対パス。
	Dir string
	// Scope は対象にする所在の範囲。空は FolderScopeDirect と同じ。
	Scope FolderScope
	// Query は VideoQuery.Query と同じ書き方の検索語。照合は Scope の範囲に
	// ある所在だけを対象にする。
	Query string
	// Watch は視聴状態の絞り込み。空は WatchAll と同じ。
	Watch WatchFilter
	// PlayableOnly はブラウザで再生できると確定した動画だけにする。
	PlayableOnly bool
	Sort         VideoSort
	// Seed は VideoQuery.Seed と同じ。
	Seed   int64
	Cursor string
	Limit  int
}

// ValidateFolderPath は登録フォルダからの相対パスを検査する。空文字は登録
// フォルダ自身を指す。空の段・先頭や末尾の `/`・`.`・`..`・NUL を断るので、
// 検査を通ったパスを登録フォルダへ連結しても外へ出ることはない。
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
		// 区切りが `\` の OS では、段の中の `\` も区切りとして解釈されてしまう。
		if filepath.Separator != '/' && strings.ContainsRune(segment, filepath.Separator) {
			return ErrInvalidFolderPath
		}
	}
	return nil
}

// FolderDir は登録フォルダと相対パスからフォルダの絶対パスを作る。rel は
// ValidateFolderPath を通っていること。
func FolderDir(root, rel string) string {
	if rel == "" {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

// FolderName はフォルダの表示名を返す。相対パスの最後の段で、登録フォルダ
// 自身は絶対パスの最後の段である。`/` のように最後の段が無い登録フォルダは
// 絶対パスそのものを名前にする。
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

// SummarizeFolder は所在の一覧から、フォルダ自身と直下の子フォルダの集計を
// 作る。locations はフォルダ配下（深さを問わない）の所在で、パスの昇順で
// なくてもよい。
//
// 2つ目の戻り値はフォルダが存在するかどうかである。登録フォルダ自身は所在が
// 無くても存在し、それより下は配下に所在が1件以上あるときだけ存在する。
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

// SortFolders はフォルダを名前の自然順に並べる。同順位は元の文字列、最後に
// 登録フォルダの絶対パスで決着させる。
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

// CompareNatural は名前を自然順で比べる。数字の連続は数値として比べ
// （`2` < `10`）、それ以外は大文字小文字を区別せずに比べる。数値が等しい
// 数字の連続（`01` と `1`）は同順位とし、呼び出し側が元の文字列で決着させる。
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

// compareDigits は数字の連続を、桁あふれせずに数値として比べる。
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

// relativeSegments は dir から path への相対パスを段に分ける。path が dir の
// 下に無ければ false を返す。
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

// addVideo は直下の所在を1件数える。同じ動画の所在が同じフォルダに2つ
// あっても1本と数え、プレビューはパスの昇順で先の所在の順番に入れる。
func (f *folderAccumulator) addVideo(location FolderLocation) {
	if _, seen := f.videos[location.VideoID]; seen {
		return
	}
	f.videos[location.VideoID] = struct{}{}
	if location.ThumbnailState == ThumbnailStateDone && len(f.previews) < MaxFolderPreviews {
		f.previews = append(f.previews, FolderPreview{VideoID: location.VideoID, ContentKey: location.ContentKey})
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

// VideoFolder は一覧に出す所在が置かれたフォルダである。値は api/openapi.yaml の
// VideoFolder に対応する。
type VideoFolder struct {
	// RootID は所在を含む登録メディアフォルダの識別子。
	RootID int64
	// Path は登録フォルダから所在の置かれたフォルダまでの `/` 区切りの相対パス。
	// 登録フォルダの直下なら空文字。
	Path string
}

// LocateVideoFolder は所在の絶対パスから、それを含む登録フォルダと、そこから
// 所在の置かれたフォルダまでの相対パスを求める。含む登録フォルダが無ければ
// false を返す。登録フォルダは入れ子にならない（登録時に断る）ので、含むものは
// 高々1つである。
//
// 含むかどうかは、保存側の「登録フォルダの下にある」条件
// （internal/store の registeredLocationCondition）と同じ規則で決める。Windows では
// `/` と `\` の両方を区切りとし、ASCII の大文字小文字を区別しない。ほかの OS では
// その OS の区切りだけを使う。
func LocateVideoFolder(roots []MediaFolder, locationPath string) (VideoFolder, bool) {
	return locateVideoFolderFor(roots, locationPath, runtime.GOOS == "windows", filepath.Separator)
}

func locateVideoFolderFor(roots []MediaFolder, locationPath string, windows bool, separator rune) (VideoFolder, bool) {
	separators := string(separator)
	if windows {
		separators = `/\`
	}
	for _, root := range roots {
		rest, ok := pathBelowRoot(root.Path, locationPath, separators, windows)
		if !ok {
			continue
		}
		segments := strings.FieldsFunc(rest, func(r rune) bool { return strings.ContainsRune(separators, r) })
		if len(segments) > 0 {
			// 最後の段は所在のファイル名で、フォルダには含めない。
			segments = segments[:len(segments)-1]
		}
		return VideoFolder{RootID: root.ID, Path: strings.Join(segments, "/")}, true
	}
	return VideoFolder{}, false
}

// pathBelowRoot は path が root 自身か root の下にあるとき、root より後ろの部分を
// 返す。root の末尾の区切りは落としてから比べる。
func pathBelowRoot(root, path, separators string, windows bool) (string, bool) {
	equal := func(a, b string) bool { return a == b }
	if windows {
		// SQLite の lower() と同じく ASCII だけを畳む。長さが変わらないので、
		// 接頭辞の長さでそのまま切り出せる。
		equal = func(a, b string) bool { return asciiLower(a) == asciiLower(b) }
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

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
