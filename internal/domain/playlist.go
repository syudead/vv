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

// PlaylistItem はプレイリストの見せる項目 1 つである。Video と Group のどちらか一方だけを持ち、
// どちらも所有者として読んだ値である。
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
