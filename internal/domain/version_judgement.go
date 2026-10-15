package domain

import (
	"cmp"
	"time"
)

// 「同じ動画かもしれない」候補の自動の判定（specs/054-auto-dup/data-model.md、research.md R-4〜R-8）。
// 判定は候補の行の列に置き、結果は所有者の操作と同じ行（集まり・「違う動画」の記録）を書く。

// VersionJudgement は候補の行の判定の状態である。待っている行は状態を持たない（null）。
type VersionJudgement string

const (
	// VersionJudgementRunning は判定中である。同時に 1 行だけ。
	VersionJudgementRunning VersionJudgement = "running"
	// VersionJudgementUndecided は決められなかった組で、候補の一覧に残り、取り出されない。
	VersionJudgementUndecided VersionJudgement = "undecided"
	// VersionJudgementFailed は判定に失敗した組で、RetryAt を過ぎると取り出せる。
	VersionJudgementFailed VersionJudgement = "failed"
	// VersionJudgementNoConnection はモデルが要るのに Ollama の接続が保存されていない組で、要求を送らず、
	// 接続が保存されるまで取り出されない（research.md R-13）。
	VersionJudgementNoConnection VersionJudgement = "no_connection"
)

// VersionJudgeJob は取り出した候補の 1 組である。Keys は key_a < key_b の内容の鍵、Fingerprints は
// その順の 2 本の指紋（コマごとの規則が読む、research.md R-1）。Attempts はこれまでの失敗の数。
type VersionJudgeJob struct {
	Keys         [2]string
	Fingerprints [2]Fingerprint
	Attempts     int
}

// VersionVerdict は判定の結果の種類である。
type VersionVerdict int

const (
	// VersionSame は同じ動画で、2 つの側を束ねる（衝突すれば決められないにする、R-6）。
	VersionSame VersionVerdict = iota + 1
	// VersionDifferent は違う動画で、所有者の「違う動画」と同じ記録を書く。
	VersionDifferent
	// VersionUndecided は決められなかった。Probability を持つ。
	VersionUndecided
	// VersionFailed は判定に失敗した。Error を持つ。
	VersionFailed
	// VersionNoConnection はモデルが要るのに接続が保存されていない（research.md R-13）。
	VersionNoConnection
)

// String はログに出す名前である。
func (v VersionVerdict) String() string {
	switch v {
	case VersionSame:
		return "same"
	case VersionDifferent:
		return "different"
	case VersionUndecided:
		return "undecided"
	case VersionFailed:
		return "failed"
	case VersionNoConnection:
		return "no_connection"
	default:
		return "unknown"
	}
}

// VersionJudgeResult は取り出した組の判定の結果である。Probability は VersionUndecided のときの
// モデルの答え（無ければ nil）、Error は VersionFailed のときの提供元のメッセージ。
type VersionJudgeResult struct {
	Verdict     VersionVerdict
	Probability *float64
	Error       string
}

// VersionCandidatesChanged は候補の一覧が変わったかもしれないことを表す（research.md R-10）。
// 判定の結果の反映と、集まりから外す操作が確定後に発行する。
type VersionCandidatesChanged struct{}

func (VersionCandidatesChanged) event() {}

// versionJudgeRetryDelays は 1・2・3 回目の失敗の後に待つ時間で、4 回目からは versionJudgeRetryDaily
// （research.md R-5）。
var versionJudgeRetryDelays = []time.Duration{10 * time.Minute, time.Hour, 6 * time.Hour}

const versionJudgeRetryDaily = 24 * time.Hour

// VersionJudgeRetryAt は attempts 回目の失敗を now に記録した組を、再び取り出せる時刻である
// （research.md R-5）。10 分、1 時間、6 時間、以後は毎日。attempts が 1 未満なら 1 回目とみなす。
func VersionJudgeRetryAt(attempts int, now time.Time) time.Time {
	delay := versionJudgeRetryDaily
	if index := max(attempts, 1) - 1; index < len(versionJudgeRetryDelays) {
		delay = versionJudgeRetryDelays[index]
	}
	return now.Add(delay)
}

// VersionSidesConflict は、側 a と側 b（それぞれ集まりの全メンバーか、束ねていない 1 本の内容の鍵）を
// 束ねると、違うと分かっている組（「違う動画」の記録と、集まりから外した組）をまたぐかを返す
// （research.md R-6）。同じ側の中の組はまたがない。differing の組の鍵の順は問わない。
func VersionSidesConflict(a, b []string, differing [][2]string) bool {
	side := make(map[string]int, len(a)+len(b))
	for _, key := range a {
		side[key] |= 1
	}
	for _, key := range b {
		side[key] |= 2
	}
	for _, pair := range differing {
		x, y := side[pair[0]], side[pair[1]]
		if (x == 1 && y == 2) || (x == 2 && y == 1) {
			return true
		}
	}
	return false
}

// VersionSide は「同じ」の組の片側の、代表の候補である（research.md R-7）。Video はその側の代表の
// 候補の動画（集まりに属せばその実効の代表、属さなければその動画）で、所有者から見た値。
// OwnerData はその側の利用者データの鍵にタグ・再生位置・公開の設定・表示名・適用したサムネイル画像・
// 作品情報のどれかがあるか、Bundled はその側が集まりに属するかである。
type VersionSide struct {
	Video     Video
	OwnerData bool
	Bundled   bool
}

// ChooseVersionRepresentative は「同じ」の組を束ねるとき代表にする側（0 か 1）を返す
// （research.md R-7）。片側だけが集まりに属せばその側。そうでなければ、所有者のデータを片側だけが
// 持つ、ブラウザーで再生できる、音声がある、画素が多い、ビットレートが高い、動画の id が小さい、の
// 順に最初に差の付いた規則で選ぶ。
func ChooseVersionRepresentative(sides [2]VersionSide) int {
	if sides[0].Bundled != sides[1].Bundled {
		return pickSide(sides[0].Bundled)
	}
	a, b := sides[0].Video, sides[1].Video
	for _, order := range []int{
		compareTrue(sides[0].OwnerData, sides[1].OwnerData),
		compareTrue(a.Playable, b.Playable),
		compareTrue(a.AudioCodec != "", b.AudioCodec != ""),
		cmp.Compare(videoPixels(b), videoPixels(a)),
		compareBitrate(a, b),
		cmp.Compare(a.ID, b.ID),
	} {
		if order != 0 {
			return pickSide(order < 0)
		}
	}
	return 0
}

// pickSide は first が真なら側 0、偽なら側 1 を返す。
func pickSide(first bool) int {
	if first {
		return 0
	}
	return 1
}

// compareTrue は a だけが真なら -1、b だけが真なら 1、同じなら 0 を返す（真の側を先にする）。
func compareTrue(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

// videoPixels は動画の画素数（幅 × 高さ）で、分からなければ 0。
func videoPixels(v Video) int64 {
	if v.Width == nil || v.Height == nil {
		return 0
	}
	return int64(*v.Width) * int64(*v.Height)
}

// compareBitrate はビットレート（大きさ ÷ 尺）の高い方を先にする順を返す。尺の分からない動画は
// 低い側に倒す。割り算をせず、大きさと尺を掛け合わせて比べる。
func compareBitrate(a, b Video) int {
	aKnown := a.DurationMs != nil && *a.DurationMs > 0
	bKnown := b.DurationMs != nil && *b.DurationMs > 0
	if !aKnown || !bKnown {
		return compareTrue(aKnown, bKnown)
	}
	// a.Size / a.Duration > b.Size / b.Duration ⇔ a.Size * b.Duration > b.Size * a.Duration。
	return cmp.Compare(float64(b.SizeBytes)*float64(*a.DurationMs), float64(a.SizeBytes)*float64(*b.DurationMs))
}
