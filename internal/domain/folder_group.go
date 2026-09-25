package domain

import (
	"cmp"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// FolderIndexVersion はフォルダの索引（グループの割り当てと動画ごとの祖先
// フォルダ名）を作る規則の版である。BuildFolderIndex か FolderKey の規則を
// 変えたら上げる。起動時に保存した版と違えば索引を作り直す
// （specs/017-folder-groups/data-model.md §3）。
const FolderIndexVersion = 1

// FolderGroupMode はフォルダごとの例外である。値は保存する
// folder_group_overrides.mode と同じ。
type FolderGroupMode string

const (
	// FolderGroupUngroup は「まとめを解除」。そのフォルダをグループにしない。
	FolderGroupUngroup FolderGroupMode = "ungroup"
	// FolderGroupDirect は「直下をまとめる」。子フォルダがあっても、登録フォルダ
	// そのものでも、直下の動画をグループにする。
	FolderGroupDirect FolderGroupMode = "group_direct"
)

// Valid は既知の値かどうかを返す。
func (m FolderGroupMode) Valid() bool {
	return m == FolderGroupUngroup || m == FolderGroupDirect
}

// FolderIndexLocation は索引を作るのに要る所在1件である。
type FolderIndexLocation struct {
	VideoID int64
	// Path は所在の絶対パス。
	Path string
}

// FolderGroupAssignment は1つのグループである。
type FolderGroupAssignment struct {
	// Key はフォルダを指す鍵（FolderKey）。グループの同一性はこれで決める。
	Key string
	// Path はフォルダの絶対パスの綴り。そのフォルダの下の所在のうちパスの最小の
	// ものから取る。登録フォルダそのものなら登録フォルダのパスである。
	Path string
	// Name はフォルダ名（FolderName と同じ最後の段）。
	Name string
	// TitleKey は題名順の並べ替えの鍵（NaturalSortKey(Name)）。
	TitleKey string
	// VideoIDs はメンバーの動画 id をグループの中の並びで持つ。
	VideoIDs []int64
}

// FolderIndex はフォルダの索引の中身である。
type FolderIndex struct {
	// Groups はグループを Key の順に持つ。1本の動画は多くても1つのグループに入る。
	Groups []FolderGroupAssignment
	// FolderNames は動画ごとの祖先フォルダ名（NormalizeTagName を通したもの）を、
	// 重複を除いて名前の順に持つ。名前の無い動画は入らない。
	FolderNames map[int64][]string
}

// FolderKey はフォルダの絶対パスを、フォルダを同一視する鍵にする。末尾の区切りを
// 落とし、Windows では `/` を `\` にそろえて ASCII の大文字小文字を畳む。登録
// フォルダの判定（LocateVideoFolder）と同じ区切りと大文字小文字の規則である。
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

// BuildFolderIndex は、登録フォルダ・その下の所在・例外（FolderKey → mode）から、
// グループの割り当てと動画ごとの祖先フォルダ名を作る
// （specs/017-folder-groups/data-model.md §2・§4）。
//
// locations のうちどの登録フォルダの下にも無いものは使わない。順番は問わない。
func BuildFolderIndex(roots []MediaFolder, locations []FolderIndexLocation, overrides map[string]FolderGroupMode) FolderIndex {
	return buildFolderIndexFor(roots, locations, overrides, runtime.GOOS == "windows", filepath.Separator)
}

// folderIndexLocation は登録フォルダの下にあると分かった所在1件である。
type folderIndexLocation struct {
	videoID int64
	path    string
	root    MediaFolder
	// dirs は登録フォルダそのもの（深さ 0）から所在の親フォルダまでの、各段の
	// フォルダの絶対パスの綴り。
	dirs []string
	// segments は登録フォルダより下の段で、最後はファイル名である。
	segments []string
}

// folderState はフォルダ1つについて集めたものである。
type folderState struct {
	path     string
	root     MediaFolder
	rel      string
	hasChild bool
	direct   []siblingEntry
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
	// パスのバイト順に並べる。動画ごとに最初に出た所在が代表の所在になり、
	// フォルダごとに最初に出た所在からフォルダの綴りを取る。
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
		// 親より上の段は、子フォルダの下に所在を持つ。
		for depth := range parent {
			folderOf(entry, depth).hasChild = true
		}
		parentState := folderOf(entry, parent)
		if _, seen := represented[entry.videoID]; !seen {
			represented[entry.videoID] = struct{}{}
			parentState.direct = append(parentState.direct, siblingEntry{id: entry.videoID, name: entry.segments[len(entry.segments)-1]})
		}
		// 祖先フォルダ名は、代表に限らずすべての所在から取る。
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

	index := FolderIndex{Groups: []FolderGroupAssignment{}, FolderNames: make(map[int64][]string, len(names))}
	for key, state := range folders {
		if !groupsFolder(key, state, overrides, rootKeys) {
			continue
		}
		members := slices.Clone(state.direct)
		slices.SortFunc(members, compareSiblings)
		ids := make([]int64, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.id)
		}
		name := FolderName(state.root.Path, state.rel)
		index.Groups = append(index.Groups, FolderGroupAssignment{
			Key: key, Path: state.path, Name: name, TitleKey: NaturalSortKey(name), VideoIDs: ids,
		})
	}
	slices.SortFunc(index.Groups, func(a, b FolderGroupAssignment) int { return strings.Compare(a.Key, b.Key) })
	for videoID, list := range names {
		slices.Sort(list)
		index.FolderNames[videoID] = list
	}
	return index
}

// groupsFolder はフォルダがグループになるかを決める
// （specs/017-folder-groups/data-model.md §2）。
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

// placeLocation は所在を含む登録フォルダを探し、各段のフォルダの綴りを求める。
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
					// 前の段までを1つのフォルダとして記録する。
					entry.dirs = append(entry.dirs, location.Path[:offset+previousEnd])
				}
				entry.segments = append(entry.segments, rest[start:i])
				previousEnd = i
			}
			start = i + 1
		}
		if len(entry.segments) == 0 {
			// 登録フォルダそのものを指す所在はファイルではない。
			return folderIndexLocation{}, false
		}
		return entry, true
	}
	return folderIndexLocation{}, false
}
