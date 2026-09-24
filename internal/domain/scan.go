package domain

// ScanResult は走査1回の集計である。scans 行の元になる値で、永続化の手段は
// 知らない。取り込みの進捗として API に出る。
//
// 個別のファイルの失敗で走査全体を中止しないため、Failed は
// 「最後まで走った上で取り込めなかった数」を表す。走査そのものが失敗した場合
// （対象ディレクトリが読めない等）は scans.error に別途記録する。
type ScanResult struct {
	// Total は追加・更新のために実際の取り込み処理が必要なファイルの数。
	// 変更のないファイルと、走査後に削除する所在は含めない。
	Total int
	// Processed は Total のうち取り込み処理を正常に終えた数。
	Processed int
	// Added は新しく取り込んだ数。
	Added int
	// Updated は既存の行を更新した数（サイズか mtime が変わったもの）。
	Updated int
	// Moved は既知の内容を新しいpathで発見した数。移動・改名に加え、
	// 同じ内容の別location追加を含む。
	Moved int
	// Removed は実体が無くなって行を消した数。
	Removed int
	// Failed は取り込めなかった数。
	Failed int
}

// Completed は取り込みを正常に終えた対象数を返す。進捗の分子として使う。
// 結果の内訳とは独立して数える。内容が同じでも metadata の確認と反映が必要な
// 対象や、将来追加される結果種別でも、処理を終えれば進捗は進むためである。
func (r ScanResult) Completed() int {
	return r.Processed
}
