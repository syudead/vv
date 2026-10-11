package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// プレイリスト（specs/044-playlists/data-model.md「domain values added」）。

// PlaylistNameMaxLength はプレイリストの名前の上限（符号位置数）である。SQLite の length() と
// 同じ数え方で、表の check と同じ値である（research.md R-11）。
const PlaylistNameMaxLength = 100

// Playlist はプレイリスト 1 つの要約である。ItemCount は見せる項目（research.md R-4）の数、
// UpdatedAt は名前か項目を最後に変えた時刻である。
type Playlist struct {
	ID        int64
	Name      string
	ItemCount int
	UpdatedAt time.Time
}

// PlaylistItem はプレイリストの項目 1 つである。見せる項目は Video と Group のどちらか一方だけを持ち、
// どちらも所有者として読んだ値である。見せない項目（PlaylistPlaybackOf の入力にだけ現れる）は
// どちらも持たない。
type PlaylistItem struct {
	ID    int64
	Video *Video
	Group *LibraryGroup
}

// PlaylistAdd はプレイリストへの追加の入力である。VideoIDs の動画と FolderPaths のフォルダ
// （絶対パス）のグループを、この順で末尾に足す（research.md R-5）。
type PlaylistAdd struct {
	VideoIDs    []int64
	FolderPaths []string
}

// PlaylistAdded は追加の結果である。Added は末尾に足した項目の数、Existing は引けたが既に
// プレイリストにあった項目の数である。ライブラリに無い id と今グループでないフォルダは
// どちらにも数えない。
type PlaylistAdded struct {
	Added    int
	Existing int
}

// ErrPlaylistNotFound はプレイリストが無いことを表す。errors.Is(err, ErrNotFound) も真になる。
var ErrPlaylistNotFound = fmt.Errorf("playlist %w", ErrNotFound)

// ErrPlaylistItemNotFound はプレイリストの項目が無いことを表す。errors.Is(err, ErrNotFound) も
// 真になる。
var ErrPlaylistItemNotFound = fmt.Errorf("playlist item %w", ErrNotFound)

// ErrInvalidPlaylistName はプレイリストの名前として受け付けられない入力を表す。API では
// invalid_request（400）になる。ValidatePlaylistName は具体的な理由を運ぶ
// *InvalidPlaylistNameError を返し、errors.Is(err, ErrInvalidPlaylistName) は真になる。
var ErrInvalidPlaylistName = errors.New("invalid playlist name")

// PlaylistNameProblem はプレイリストの名前の規則違反の種類である。
type PlaylistNameProblem int

const (
	// PlaylistNameEmpty は前後の空白を取り除くと空になることを表す。
	PlaylistNameEmpty PlaylistNameProblem = iota + 1
	// PlaylistNameTooLong は PlaylistNameMaxLength 符号位置を超えることを表す。
	PlaylistNameTooLong
)

// InvalidPlaylistNameError はプレイリストの名前の規則違反の具体的な理由を運ぶ。
type InvalidPlaylistNameError struct {
	Problem PlaylistNameProblem
}

func (e *InvalidPlaylistNameError) Error() string {
	switch e.Problem {
	case PlaylistNameEmpty:
		return "playlist name is empty"
	case PlaylistNameTooLong:
		return fmt.Sprintf("playlist name must be at most %d characters", PlaylistNameMaxLength)
	default:
		return ErrInvalidPlaylistName.Error()
	}
}

func (e *InvalidPlaylistNameError) Unwrap() error { return ErrInvalidPlaylistName }

// ValidatePlaylistName は前後の Unicode White_Space を取り除いた名前を返す。空か
// PlaylistNameMaxLength 符号位置を超えれば *InvalidPlaylistNameError である。同じ名前は
// 重複してよい（research.md R-11）。
func ValidatePlaylistName(input string) (string, error) {
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", &InvalidPlaylistNameError{Problem: PlaylistNameEmpty}
	}
	if utf8.RuneCountInString(trimmed) > PlaylistNameMaxLength {
		return "", &InvalidPlaylistNameError{Problem: PlaylistNameTooLong}
	}
	return trimmed, nil
}

// PlaylistStep は連続再生で開く動画 1 本と、それが属する項目である。
type PlaylistStep struct {
	ItemID int64
	Video  Video
}

// PlaylistPlayback は動画ページがプレイリストから再生しているときの、今の位置と次に開く動画
// である（specs/044-playlists/data-model.md「Playback order」）。Position は見せる項目の中での
// 1 からの番号で、今の項目が見せない項目なら 0 である。Count は見せる項目の数である。Next は
// 連続再生で次に開く動画で、プレイリストの終わりなら nil である。PreviousItem と NextItem は
// 前後の見せる項目の開始の動画で、無ければ nil である。
type PlaylistPlayback struct {
	Playlist     Playlist
	ItemID       int64
	Position     int
	Count        int
	Next         *PlaylistStep
	PreviousItem *PlaylistStep
	NextItem     *PlaylistStep
}

// PlaylistCurrent は動画ページが今再生している項目と動画である。
type PlaylistCurrent struct {
	ItemID  int64
	VideoID int64
}

// Shown は項目を見せるかを返す。プレイリストの順序の入力では、見せない項目（動画やグループが
// 今ライブラリに引けない項目、research.md R-4）は Video も Group も持たない。
func (i PlaylistItem) Shown() bool {
	return i.Video != nil || i.Group != nil
}

// start は見せる項目を開いたときの動画である。動画の項目はその動画、グループの項目は
// GroupOpenIndex が選んだメンバー（LibraryGroup.OpenVideoID）で、見つからなければ最初の
// メンバーである。開く動画が無ければ nil である。
func (i PlaylistItem) start() *PlaylistStep {
	if i.Video != nil {
		return &PlaylistStep{ItemID: i.ID, Video: *i.Video}
	}
	if i.Group == nil || len(i.Group.Members) == 0 {
		return nil
	}
	member := i.Group.Members[0]
	for _, candidate := range i.Group.Members {
		if candidate.ID == i.Group.OpenVideoID {
			member = candidate
			break
		}
	}
	return &PlaylistStep{ItemID: i.ID, Video: member}
}

// PlaylistPlaybackOf はプレイリストの全項目 items（見せない項目も含めて位置の順）と、今の項目と
// 動画 current から、今の位置と次に開く動画を決める（specs/044-playlists/data-model.md
// 「Playback order」、research.md R-6）。順序の規則はすべてここにある。
//
//   - グループの項目では、今の動画がそのメンバーなら、グループの並びで後ろのメンバーへ進む。
//     開始より後ろの見終わったメンバーも再生する（要件 7）。後ろのメンバーが無いか、今の動画が
//     もうメンバーでなければ、次の見せる項目へ進む。
//   - 次の見せる項目の開始の動画は、動画の項目ならその動画、グループの項目なら GroupOpenIndex の
//     メンバーである。全部見終わったグループは最初のメンバーから始まる。
//   - 今の項目が見せない項目でも、その位置から Next・PreviousItem・NextItem を決め、Position は
//     0 である。同じ動画が別の項目にもあるときは current.ItemID が位置を決める。
//
// 返す値の Playlist は埋めない（呼び出し側が埋める）。current.ItemID が items に無ければ
// ErrPlaylistItemNotFound である。
func PlaylistPlaybackOf(items []PlaylistItem, current PlaylistCurrent) (PlaylistPlayback, error) {
	at := -1
	for i, item := range items {
		if item.ID == current.ItemID {
			at = i
			break
		}
	}
	if at < 0 {
		return PlaylistPlayback{}, ErrPlaylistItemNotFound
	}

	playback := PlaylistPlayback{ItemID: current.ItemID}
	for i, item := range items {
		if !item.Shown() {
			continue
		}
		playback.Count++
		if i == at {
			playback.Position = playback.Count
		}
	}
	for i := at - 1; i >= 0 && playback.PreviousItem == nil; i-- {
		if items[i].Shown() {
			playback.PreviousItem = items[i].start()
		}
	}
	for i := at + 1; i < len(items) && playback.NextItem == nil; i++ {
		if items[i].Shown() {
			playback.NextItem = items[i].start()
		}
	}

	playback.Next = playback.NextItem
	if group := items[at].Group; group != nil {
		for j, member := range group.Members {
			if member.ID == current.VideoID {
				if j+1 < len(group.Members) {
					playback.Next = &PlaylistStep{ItemID: current.ItemID, Video: group.Members[j+1]}
				}
				break
			}
		}
	}
	return playback, nil
}
