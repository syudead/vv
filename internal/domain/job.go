package domain

import "errors"

// JobKind は待ち行列に積む仕事の種類である。走査そのものはジョブにせず、
// 重い処理（外部プロセスを起こすもの）だけを積む（R-106）。
type JobKind string

const (
	// JobProbe は ffprobe によるメタデータの取得。
	JobProbe JobKind = "probe"
	// JobThumbnail は ffmpeg による静止画の抽出。
	JobThumbnail JobKind = "thumbnail"
)

// MaxJobAttempts は諦めるまでの試行回数である。止めないと、壊れたファイル
// 1つが並列度1のワーカーを永久に占有する（R-106）。
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
	// LocationSetMaxID はclaim時点に存在したlocation IDの上限。処理中に
	// locationが追加された場合、古いsnapshotで完了・最終失敗にしないために使う。
	LocationSetMaxID int64
	// LastLocation はclaim時点の登録済みlocation集合で最後の候補かを表す。
	LastLocation bool
	// Attempts はこの取り出しを含めた試行回数。
	Attempts int
}
