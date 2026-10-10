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

// ErrInvalidWatchHistoryFilter は視聴履歴の絞り込みが all・inProgress・watched のどれでもないことを表す。
var ErrInvalidWatchHistoryFilter = errors.New("invalid watch history filter")

// ErrInvalidWatchHistoryPeriod は日付への移動が YYYY-MM-DD でも YYYY-MM でもないことを表す。
var ErrInvalidWatchHistoryPeriod = errors.New("invalid watch history period")

// WatchHistoryFilter は視聴履歴を動画のいまの視聴状態で絞る条件である（research.md R-9）。値は
// api/openapi.yaml の WatchHistoryFilter に対応する。視聴の 1 回に「未視聴」は無いので、一覧の
// WatchFilter と違って unwatched を持たない。
type WatchHistoryFilter string

const (
	// WatchHistoryAll は絞り込まない（既定）。動画の無い件もここにだけ出る。
	WatchHistoryAll WatchHistoryFilter = "all"
	// WatchHistoryInProgress は動画が視聴途中の件だけにする。
	WatchHistoryInProgress WatchHistoryFilter = "inProgress"
	// WatchHistoryWatched は動画が視聴済みの件だけにする。
	WatchHistoryWatched WatchHistoryFilter = "watched"
)

// ParseWatchHistoryFilter は絞り込みの値を読む。3 つの値のほかは ErrInvalidWatchHistoryFilter。
func ParseWatchHistoryFilter(value string) (WatchHistoryFilter, error) {
	switch filter := WatchHistoryFilter(value); filter {
	case WatchHistoryAll, WatchHistoryInProgress, WatchHistoryWatched:
		return filter, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidWatchHistoryFilter, strconv.Quote(value))
	}
}

// WatchFilter は同じ状態を表す一覧の絞り込みを返す。保存層が一覧と同じ条件句
// （watchCondition）を使うためで、WatchHistoryAll と空は WatchAll になる。
func (f WatchHistoryFilter) WatchFilter() WatchFilter {
	switch f {
	case WatchHistoryInProgress:
		return WatchInProgress
	case WatchHistoryWatched:
		return WatchWatched
	default:
		return WatchAll
	}
}

// WatchHistoryQuery は視聴履歴の一覧と日付の一覧の条件である（specs/043-watch-history/data-model.md
// 「domain values added」）。Filter が空なら WatchHistoryAll と同じ。Search が空なら検索で絞らない。
// Before がゼロでなければ played_at がそれより前の件だけにする（日付の一覧は Before を見ない）。
type WatchHistoryQuery struct {
	Filter WatchHistoryFilter
	Search SearchExpr
	Before time.Time
}

// WatchHistoryPeriod は日付への移動の先で、日（YYYY-MM-DD）か月（YYYY-MM）である。Day が 0 なら月。
type WatchHistoryPeriod struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseWatchHistoryPeriod は YYYY-MM-DD か YYYY-MM を読む。桁の欠けたもの（2026-9）、
// 無い日（2026-02-30）とほかの形は ErrInvalidWatchHistoryPeriod。
func ParseWatchHistoryPeriod(value string) (WatchHistoryPeriod, error) {
	invalid := fmt.Errorf("%w: %s", ErrInvalidWatchHistoryPeriod, strconv.Quote(value))
	switch len(value) {
	case len("2006-01-02"):
		day, err := time.Parse("2006-01-02", value)
		if err != nil {
			return WatchHistoryPeriod{}, invalid
		}
		return WatchHistoryPeriod{Year: day.Year(), Month: day.Month(), Day: day.Day()}, nil
	case len("2006-01"):
		month, err := time.Parse("2006-01", value)
		if err != nil {
			return WatchHistoryPeriod{}, invalid
		}
		return WatchHistoryPeriod{Year: month.Year(), Month: month.Month()}, nil
	default:
		return WatchHistoryPeriod{}, invalid
	}
}

// End は loc で見たその日か月が終わった直後の時刻（次の日か次の月の最初の時刻）を返す
// （research.md R-11）。夏時間の切り替えの日も loc の暦で数える。
//
// 0 時に時計が進む地域（America/Santiago など）では次の日の 0 時が無く、time.Date は切り替え前の
// 時差でそれを解いて前の日の 23 時を返す。そのときは切り替えの時刻、つまり次の日の最初の実在する
// 時刻を返し、日付の一覧（ListWatchHistoryDays）がその日に数える最後の 1 時間を落とさない。
func (p WatchHistoryPeriod) End(loc *time.Location) time.Time {
	year, month, day := p.Year, p.Month, p.Day+1
	if p.Day == 0 {
		month, day = p.Month+1, 1
	}
	end := time.Date(year, month, day, 0, 0, 0, 0, loc)
	// 正規化した次の日（月や年の繰り上がりを含む）と、end の loc での日付を比べる。
	next := time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
	if y, m, d := end.Date(); y != next.Year() || m != next.Month() || d != next.Day() {
		_, end = end.ZoneBounds()
	}
	return end
}
