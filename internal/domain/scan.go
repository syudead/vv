package domain

// ScanResult は走査1回の集計である。scans 行の元になる値で、永続化の手段は
// 知らない。取り込みの進捗として API に出る（R-108）。
//
// 個別のファイルの失敗で走査全体を中止しないため（FR-008）、Failed は
// 「最後まで走った上で取り込めなかった数」を表す。走査そのものが失敗した場合
// （対象ディレクトリが読めない等）は scans.error に別途記録する。
type ScanResult struct {
	// Total は走査で見つけた対象ファイルの数。
	Total int
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

// Completed は取り込みを終えた数を返す。進捗の分子として使う。
// 失敗した分も「もう処理しない」という意味では進んでいるが、利用者に見せる
// completed は成功した数であるべきなので、ここには含めない。
func (r ScanResult) Completed() int {
	return r.Added + r.Updated + r.Moved
}
