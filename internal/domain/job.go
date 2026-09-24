package domain

import "errors"

// JobKind は待ち行列に積む仕事の種類である。走査そのものはジョブにせず、
// 重い処理（外部プロセスを起こすもの）だけを積む。
type JobKind string

const (
	// JobProbe は ffprobe によるメタデータの取得。
	JobProbe JobKind = "probe"
	// JobThumbnail は ffmpeg による静止画の抽出。
	JobThumbnail JobKind = "thumbnail"
	JobPreview   JobKind = "preview"
)

// MaxJobAttempts は諦めるまでの試行回数である。止めないと、壊れたファイル
// 1つが並列度1のワーカーを永久に占有する。
const MaxJobAttempts = 3

// ErrNoJob は待ち行列が空であることを表す。ワーカーはこれを見て待機に入る。
var ErrNoJob = errors.New("処理するジョブがありません")

// Job は待ち行列から専有した仕事である。
type Job struct {
	ID              int64
	Kind            JobKind
	VideoID         int64
	ContentKey      string
	LocationID      int64
	LocationVersion int64
	LocationPath    string
	// LocationGeneration はclaim時点のlocation集合世代。処理中にmembershipが
	// 変わった場合、古いsnapshotで完了・最終失敗にしないために使う。
	LocationGeneration int64
	// LastLocation はclaim時点の登録済みlocation集合で最後の候補かを表す。
	LastLocation bool
	// Attempts はこの取り出しを含めた試行回数。
	Attempts int
}

// JobKinds は取り込みの段階の順に並べたジョブの種類である。段階ごとに
// ワーカーを1本ずつ置くので、ここに無い種類は処理されない。
var JobKinds = []JobKind{JobProbe, JobThumbnail, JobPreview}

// Processing は、段階ごとに残っている仕事の数である。待ち行列に積まれている
// ものと処理中のものを数え、登録外の所在しかない動画の仕事は含めない
// （ワーカーが取り出さないので、数えると終わらない準備に見える）。
type Processing struct {
	Probe     int
	Thumbnail int
	Preview   int
}

// Remaining は全段階の残りの合計である。0 なら準備は終わっている。
func (p Processing) Remaining() int {
	return p.Probe + p.Thumbnail + p.Preview
}
