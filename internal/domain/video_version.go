package domain

import (
	"cmp"
	"errors"
	"slices"
)

// 同じ動画の別バージョンの集まり（specs/030-video-versions/data-model.md §2、research.md R-1・R-2）。
// 集まりは利用者データで、集まりのタグ・再生位置・公開の設定は集まり自身の鍵（user_key）に
// 置く。保存層が動画ごとの利用者データの鍵を Video.UserKey に埋める。

// ErrNotBundled は、集まりに属さない動画に代表の変更・解除を求めたことを表す。
var ErrNotBundled = errors.New("the video is not bundled")

// ErrRepresentativeNotSelected は、束ねる動画に代表の動画が含まれないことを表す。
var ErrRepresentativeNotSelected = errors.New("the representative is not among the selected videos")

// ErrTooFewVersions は、束ねる動画が 2 本未満であることを表す。
var ErrTooFewVersions = errors.New("at least two videos are needed to bundle")

// VideoVersionsRef は、集まりのメンバーの動画の詳細に載せる集まりの要約である。
type VideoVersionsRef struct {
	// Count は見る人に見せてよい所在を持つメンバーの本数である。
	Count int
	// RepresentativeID は実効の代表（data-model.md §4）の動画の id である。
	RepresentativeID int64
}

// VideoVersions は集まりの全バージョンである。Items は代表が先頭で、続きは題名の
// 自然順（同じなら id）。集まりに属さない動画では、その 1 本だけを持つ。
type VideoVersions struct {
	RepresentativeID int64
	Items            []Video
}

// SortVideoVersions は items を、representativeID を先頭に、続きを題名の自然順
// （同じなら id）に並べる。
func SortVideoVersions(items []Video, representativeID int64) {
	slices.SortStableFunc(items, func(a, b Video) int {
		switch {
		case a.ID == b.ID:
			return 0
		case a.ID == representativeID:
			return -1
		case b.ID == representativeID:
			return 1
		}
		if order := CompareNatural(a.Title, b.Title); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

// VideoBundleChanged は動画の束ね（集まりのメンバー・代表）が変わったことを表す
// （research.md R-9）。VideoIDs は影響した動画の id である。
type VideoBundleChanged struct {
	VideoIDs []int64
}

func (VideoBundleChanged) event() {}

// successionToleranceMinMs は、尺がほぼ同じとみなす差の下限（1 秒）である。
const successionToleranceMinMs = 1000

// DurationsMatch は 2 つの尺（ミリ秒）がほぼ同じかを返す。差が 1 秒と長い方の 0.5% の大きい方
// 以下なら一致する。0 以下の尺（分からない尺）は一致しない（data-model.md §2、research.md R-5）。
// 同じパスの中身の引き継ぎと、指紋の候補の尺の条件がこの規則を使う。
func DurationsMatch(a, b int64) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= max(successionToleranceMinMs, max(a, b)*5/1000)
}

// MaxVersionCandidates は候補の一覧が一度に返す組の上限である（data-model.md §7）。
const MaxVersionCandidates = 200

// VersionCandidate は「同じ動画かもしれない」候補の 1 組である（research.md R-7）。Videos は
// id の小さい順で、Distance は 2 本の指紋のハミング距離の中央値（CompareFingerprints）。
type VersionCandidate struct {
	Videos   [2]Video
	Distance int
}

// VersionCandidatePage は候補の一覧である。Items は新しい順で最大 MaxVersionCandidates 組、
// Total は全件の数。
type VersionCandidatePage struct {
	Items []VersionCandidate
	Total int
}
