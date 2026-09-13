package domain

import "time"

// 視聴済みと再開位置の規則（R-111）。
const (
	// CompletionTailMs は「末尾まで見た」とみなす残り時間（ミリ秒）。
	// 終わりのクレジットを飛ばしても視聴済みになる。
	CompletionTailMs = 15_000
	// CompletionRatio は「ほぼ見た」とみなす割合。
	// 長い動画では 15 秒の残りが厳しすぎるので、割合でも判定する。
	CompletionRatio = 0.95
	// MinResumeMs はこれ未満の位置を「見ていない」とみなす下限（ミリ秒）。
	// 数秒だけ再生して閉じた動画を中途半端な位置から始めない。
	MinResumeMs = 5_000
)

// Progress は再生位置の記録である。鍵は content_key なので、ファイルを
// 移動・改名・置き直しても引き継がれる（FR-025／FR-026）。
type Progress struct {
	PositionMs int64
	DurationMs int64
	Completed  bool
	UpdatedAt  time.Time
}

// EvaluateProgress は申告された位置から、保存する値を決める（R-111）。
//
// 視聴済みの判定はサーバー側で行い、クライアントの申告は採らない。
// クライアントごとに判定が揺れると、一覧の表示と再生画面が食い違う。
//
// 外部 I/O に依存しないので、この規則は単体テストだけで検証できる。
func EvaluateProgress(positionMs, durationMs int64) Progress {
	position := max(positionMs, 0)

	// 尺が分かっているなら、その外へは出さない。壊れた申告をそのまま
	// 保存すると、再開位置が尺の外へ飛ぶ。
	if durationMs > 0 {
		position = min(position, durationMs)
	}

	return Progress{
		PositionMs: position,
		DurationMs: durationMs,
		Completed:  isCompleted(position, durationMs),
	}
}

// isCompleted は視聴済みかどうかを判定する。
//
// 2つの条件を or にするのは、短い動画では 15 秒が尺の大半を占め、長い動画
// では 5% が数分になるためである。どちらか一方だけでは片側が破綻する。
func isCompleted(positionMs, durationMs int64) bool {
	if durationMs <= 0 {
		// 尺が分からなければ「見終わった」とは言えない。
		return false
	}
	if positionMs >= durationMs-CompletionTailMs {
		return true
	}
	return float64(positionMs)/float64(durationMs) >= CompletionRatio
}

// ResumePosition は次に開いたときの再生開始位置を返す（R-111）。
//
// 見終わった動画は先頭から始める。末尾から再開させても、利用者にできることが
// 無い。ほとんど見ていない（5 秒未満）ものも先頭に戻す。
func (p Progress) ResumePosition() int64 {
	if p.Completed || p.PositionMs < MinResumeMs {
		return 0
	}
	return p.PositionMs
}
