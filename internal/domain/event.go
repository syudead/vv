package domain

// Event は状態の変化である。発行する側は、誰がそれを受け取るか（画面への
// 知らせ・ワーカーの起床・生成物の削除）を知らない。受け取る側の登録は
// cmd/mdm の1か所で行う。
//
// 保存層が発行するものは、取引が確定した後にだけ発行する。
type Event interface {
	event()
}

// VideoIngestChanged は動画の取り込みの状態が変わったことを表す。
type VideoIngestChanged struct {
	VideoID int64
	// Stage は変化を起こした段階で、その段階の1件の成否が記録された。
	// 動画の行が消えたときは空である。
	Stage JobKind
}

// JobsQueued は仕事が積まれた（または取り出せるようになった）段階を表す。
// 同じ種類は重ならない。
type JobsQueued struct {
	Kinds []JobKind
}

// ProcessingChanged は段階ごとの残りの仕事が変わったかもしれないことを表す。
type ProcessingChanged struct{}

// ScanChanged は直近の走査の状態が変わったことを表す。
type ScanChanged struct{}

// ContentUnreferenced は、内容の識別子を参照していた動画の行が消え、確定の
// 時点で参照が無くなったことを表す。同じ内容の動画がすぐに取り込み直される
// こともあるので、生成物を消す側は消す直前に参照を確かめ直すこと。
type ContentUnreferenced struct {
	ContentKeys []string
}

func (VideoIngestChanged) event()  {}
func (JobsQueued) event()          {}
func (ProcessingChanged) event()   {}
func (ScanChanged) event()         {}
func (ContentUnreferenced) event() {}
