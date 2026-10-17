package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ホームの読み出し（specs/057-home/contracts/screen-api.md、research.md R-12、R-13）。ホームは棚の
// 要約を並べ、画面に入った棚の最初の項目を別に読む。棚はフォルダの棚と、そのあとに並ぶ決まった棚
// （HomeShelfKind の folder 以外）である。

// MaxHomeShelfKeys は GET /api/home/shelves が 1 回に読む棚の数の上限である。
const MaxHomeShelfKeys = 8

// HomeShelfItemLimit は棚 1 つの最初の項目の数である。
const HomeShelfItemLimit = 24

// ErrInvalidHomeShelfKey は棚の鍵の形が誤っていることを表す。API では 400 invalid_request である。
var ErrInvalidHomeShelfKey = errors.New("invalid home shelf key")

// HomeShelfKind は棚の種類である。
type HomeShelfKind string

const (
	// HomeShelfFolder はホームの位置を持つスマートフォルダの棚である（research.md R-7）。
	HomeShelfFolder HomeShelfKind = "folder"
	// HomeShelfContinue は所有者の続きから見る棚である（research.md R-13）。
	HomeShelfContinue HomeShelfKind = "continue"
	// HomeShelfRecent は最近追加した動画の棚で、新しい順に並ぶ。
	HomeShelfRecent HomeShelfKind = "recent"
	// HomeShelfUnshelved はどのフォルダの棚にも入らない動画の棚である（research.md R-11）。
	HomeShelfUnshelved HomeShelfKind = "unshelved"
	// HomeShelfRecommended は所有者へのおすすめの棚である（research.md R-13）。
	HomeShelfRecommended HomeShelfKind = "recommended"
)

// homeShelfFolderPrefix はフォルダの棚の鍵の前置きである。
const homeShelfFolderPrefix = "folder:"

// HomeShelfKey は棚の鍵である。フォルダの棚は FolderID を持ち、ほかの棚は 0 である。
type HomeShelfKey struct {
	Kind     HomeShelfKind
	FolderID int64
}

// FolderShelfKey はフォルダ id の棚の鍵を返す。
func FolderShelfKey(id int64) HomeShelfKey {
	return HomeShelfKey{Kind: HomeShelfFolder, FolderID: id}
}

// String は鍵を `folder:<id>` か種類の名前にする。
func (k HomeShelfKey) String() string {
	if k.Kind == HomeShelfFolder {
		return homeShelfFolderPrefix + strconv.FormatInt(k.FolderID, 10)
	}
	return string(k.Kind)
}

// ParseHomeShelfKey は `folder:<正の id>`、`continue`、`recent`、`unshelved`、`recommended` の
// どれかを読む。ほかの形は ErrInvalidHomeShelfKey を包む誤りである。
func ParseHomeShelfKey(value string) (HomeShelfKey, error) {
	if rest, ok := strings.CutPrefix(value, homeShelfFolderPrefix); ok {
		id, err := strconv.ParseInt(rest, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != rest {
			return HomeShelfKey{}, fmt.Errorf("%w: %q", ErrInvalidHomeShelfKey, value)
		}
		return FolderShelfKey(id), nil
	}
	switch kind := HomeShelfKind(value); kind {
	case HomeShelfContinue, HomeShelfRecent, HomeShelfUnshelved, HomeShelfRecommended:
		return HomeShelfKey{Kind: kind}, nil
	}
	return HomeShelfKey{}, fmt.Errorf("%w: %q", ErrInvalidHomeShelfKey, value)
}

// ParseHomeShelfKeys は棚の鍵の並びを読む。1 から MaxHomeShelfKeys 個でなければ、またはどれかの形が
// 誤っていれば ErrInvalidHomeShelfKey を包む誤りである。同じ鍵は最初の 1 つだけにする。
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

// HomeShelf は棚の要約 1 つである。
type HomeShelf struct {
	Key HomeShelfKey
	// Name はフォルダの名前で、決まった棚では空である（画面が名前を付ける）。
	Name string
	// ByOrganizer は整理役のフォルダの棚（owner_kept が 0）かで、所有者の応答にだけ載せる。
	ByOrganizer bool
}

// FolderShelf はフォルダ folder の棚の要約を返す。
func FolderShelf(folder SmartFolder) HomeShelf {
	return HomeShelf{Key: FolderShelfKey(folder.ID), Name: folder.Name, ByOrganizer: !folder.OwnerKept}
}

// Home はホームの要約である（GET /api/home）。
type Home struct {
	// Shelves はフォルダの棚を並びの順に、そのあとに決まった棚を並べる。ゲストには見せる動画の無い
	// フォルダの棚を入れない。
	Shelves []HomeShelf
	// Organizer は整理役の状態で、所有者にだけ入る（ゲストには nil）。
	Organizer *HomeOrganizerState
	// NoVideos は見る人にライブラリの動画が 1 本も見えないことを表す。
	NoVideos bool
}

// HomeShelfItems は棚 1 つの最初の項目である。
type HomeShelfItems struct {
	Key HomeShelfKey
	// Items は動画とグループの項目だけで、フォルダの棚の子フォルダは入れない（research.md R-14）。
	Items []LibraryItem
	// Total は棚の項目（動画とグループ）の数を、HomeShelfItemLimit + 1 を上限に数えたものである。上限なら
	// 1 ページより多い（specs/057-home/research.md R-12）。
	Total int
}

// HomeShelfPage は GET /api/home/shelves の読み出しの結果である。
type HomeShelfPage struct {
	// Shelves は求めた鍵の順で、もう無い棚の鍵は落とす。
	Shelves []HomeShelfItems
	// Roots は項目と同じ読み取りで読んだ登録フォルダである。
	Roots []MediaFolder
}
