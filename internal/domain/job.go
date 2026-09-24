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

// JobState は待ち行列の行の状態である。値は jobs.state 列に対応する。
type JobState string

const (
	// JobQueued は取り出されるのを待っている。
	JobQueued JobState = "queued"
	// JobRunning はワーカーが専有して処理している。
	JobRunning JobState = "running"
	// JobDone は処理を終えた。
	JobDone JobState = "done"
	// JobFailed は試行回数の上限まで失敗して諦めた。
	JobFailed JobState = "failed"
)

// ClaimAttempts は、専有したときに記録する試行回数を返す。attempts は専有前に
// 記録されていた回数である。
//
// 1回の試行は、登録済みの所在を path の順に1巡することを指す。ある所在で
// 失敗した仕事は、次に専有されると同じ試行のまま次の所在で続きを試す。
// startsRound は、この専有が新しい巡回の始まりかどうかである。直前に試した
// 所在が無いとき（初めての専有）と、直前の所在より後ろに登録済みの所在が
// 無く先頭へ戻るときに真になる。
func ClaimAttempts(attempts int, startsRound bool) int {
	if startsRound {
		return attempts + 1
	}
	return attempts
}

// JobStateAfterFailure は、専有した仕事が失敗したあとの状態を返す。
//
// 専有したときの所在がもう当てにならない（claimCurrent が偽）なら、その失敗は
// 今の所在の失敗ではないので数えずに queued へ戻す。所在の集合が変わったのに
// 古い集合での失敗で諦めると、今ある所在を1度も試さずに終わる。
//
// そうでなければ、試行回数が MaxJobAttempts に達し、かつ巡回の最後の所在
// （lastLocation）で失敗したときだけ failed で止める。まだ試していない所在が
// 残っているうちは、上限に達していても諦めない。
func JobStateAfterFailure(attempts int, lastLocation, claimCurrent bool) JobState {
	if claimCurrent && lastLocation && attempts >= MaxJobAttempts {
		return JobFailed
	}
	return JobQueued
}

// JobClaimCondition は、待ち行列の仕事を取り出してよい条件である。保存側の
// 問い合わせはこの条件をそのまま SQL の条件へ写す。
type JobClaimCondition struct {
	// RegisteredLocation は、登録済みのメディアフォルダの下に所在がある動画に
	// 限ることを表す。登録前の所在は保持するが処理しない。フォルダを登録すれば
	// 同じ仕事をそのまま再開でき、登録外のパスをワーカーへ渡すこともない。
	RegisteredLocation bool
	// ProbeFinished は、動画の解析（probe）が pending でなくなっている
	// （done か failed になっている）ことを求める。
	ProbeFinished bool
}

// ClaimConditionFor は kind の仕事を取り出してよい条件を返す。
//
// どの種類も登録済みの所在がある動画に限る。サムネイルは解析が終わるまで
// 取り出さない。抽出位置は動画の長さで決まり、解析より先に作ると長さの
// 分からない位置で固定される。段階ごとのワーカーは並行して動くので、積んだ
// 順では解析が先になる保証が無い。
func ClaimConditionFor(kind JobKind) JobClaimCondition {
	return JobClaimCondition{
		RegisteredLocation: true,
		ProbeFinished:      kind == JobThumbnail,
	}
}

// Allows は、動画の今の状態でこの条件を満たすかを返す。hasRegisteredLocation は
// 登録済みのメディアフォルダの下に所在が1つ以上あるか、probe は動画の解析の
// 状態である。
func (c JobClaimCondition) Allows(hasRegisteredLocation bool, probe ProbeState) bool {
	if c.RegisteredLocation && !hasRegisteredLocation {
		return false
	}
	if c.ProbeFinished && probe == ProbeStatePending {
		return false
	}
	return true
}
