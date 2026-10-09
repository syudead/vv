package domain

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// 視聴履歴（specs/043-watch-history/data-model.md）。再生 1 回につき 1 件で、鍵は再生した
// バージョンの content_key である。

// ErrInvalidPlaybackID は視聴の識別子が RFC 4122 の文字列の形でないことを表す。
var ErrInvalidPlaybackID = errors.New("invalid playback id")

// playbackIDLength は RFC 4122 の文字列の形（8-4-4-4-12 の 16 進）の長さである。
const playbackIDLength = 36

// Play は再生位置の保存が視聴について伝える値である。PlaybackID は検証済み、ContentKey は
// 再生した動画の content_key（空なら履歴を書かない）、Title はその時点の有効な題名である。
type Play struct {
	PlaybackID string
	ContentKey string
	Title      string
}

// WatchHistoryEntry は視聴履歴の 1 件である。Video はその内容の動画がいまライブラリにあって
// 見る人が開けるときだけ入り、無ければ nil で、Title（書いたときの題名の写し）だけを見せる。
type WatchHistoryEntry struct {
	ID       int64
	PlayedAt time.Time
	Title    string
	Video    *Video
}

// WatchHistoryPage は視聴履歴の 1 ページである。NextCursor は続きがあるときだけ入る。
type WatchHistoryPage struct {
	Items      []WatchHistoryEntry
	NextCursor string
}

// WatchHistoryCursor は視聴履歴の一覧のカーソルの中身で、前のページの最後の件の played_at
// （Unix ミリ秒）と id である。並びは (played_at desc, id desc) なので、次のページはこの組より
// 小さい件から始まる。
type WatchHistoryCursor struct {
	PlayedAtMs int64 `json:"p"`
	ID         int64 `json:"i"`
}

// EncodeWatchHistoryCursor はカーソルを不透明な文字列にする。問題の一覧のカーソル
// （encodeIssueCursor）と同じく、JSON を URL に使える base64 で包む。
func EncodeWatchHistoryCursor(cursor WatchHistoryCursor) string {
	body, _ := json.Marshal(cursor) // 整数 2 つの構造体の Marshal は失敗しない。
	return base64.RawURLEncoding.EncodeToString(body)
}

// DecodeWatchHistoryCursor は EncodeWatchHistoryCursor の逆である。読めないもの、欄の欠けたもの、
// id が 1 未満のものは ErrInvalidCursor を返す。
func DecodeWatchHistoryCursor(cursor string) (WatchHistoryCursor, error) {
	body, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return WatchHistoryCursor{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	var fields struct {
		PlayedAtMs *int64 `json:"p"`
		ID         *int64 `json:"i"`
	}
	if err := json.Unmarshal(body, &fields); err != nil || fields.PlayedAtMs == nil || fields.ID == nil || *fields.ID < 1 {
		return WatchHistoryCursor{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	return WatchHistoryCursor{PlayedAtMs: *fields.PlayedAtMs, ID: *fields.ID}, nil
}

// ValidatePlaybackID は id が RFC 4122 の文字列の形（36 文字、8-4-4-4-12 の 16 進）かを確かめる。
// 版や変種の桁は問わない。形が違えば ErrInvalidPlaybackID を返す。
func ValidatePlaybackID(id string) error {
	if len(id) != playbackIDLength {
		return ErrInvalidPlaybackID
	}
	for i := range len(id) {
		c := id[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return ErrInvalidPlaybackID
			}
		default:
			if !isHexDigit(c) {
				return ErrInvalidPlaybackID
			}
		}
	}
	return nil
}

func isHexDigit(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}
