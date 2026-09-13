package store

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// searchRoute は検索に使う経路である（R-110 / TD-001）。
type searchRoute int

const (
	// routeNone は絞り込まない（検索語が空）。
	routeNone searchRoute = iota
	// routeMatch は videos_fts MATCH を使う。3文字以上の検索語。
	routeMatch
	// routeLike は同じ FTS5 表への LIKE '%…%' を使う。1〜2文字の検索語。
	routeLike
)

// matchMinLength は MATCH 経路に回す最小の文字数である。
//
// trigram は3文字単位で索引を作るため、2文字以下の検索語は MATCH に
// 一致しない。日本語では「旅行」「花火」のような2文字の検索語が現実に
// 多いので、そこは LIKE に振り分ける（TD-001 / 001 R-001 の実測）。
const matchMinLength = 3

// routeFor は検索語から経路を選ぶ。
//
// 数えるのは符号位置ではなく、利用者が1文字と見るまとまりである。NFC へ
// 正規化してから符号位置を数えると、結合文字で書かれた「が」（か + 濁点）が
// 1文字に畳まれる。ここを取り違えると、2文字の入力が MATCH 経路へ回って
// 何も返らない。
//
// 絵文字の ZWJ 連結のように NFC で畳まれない組み合わせは、なお複数として
// 数える。境界が1文字ずれても、経路の選択が変わるだけで結果が壊れることは
// ないので、書記素分割の依存を増やしてまで正確にはしない。
func routeFor(query string) searchRoute {
	normalized := normalizeQuery(query)
	if normalized == "" {
		return routeNone
	}
	if len([]rune(normalized)) >= matchMinLength {
		return routeMatch
	}
	return routeLike
}

// normalizeQuery は検索語を突き合わせられる形にする。
//
// NFC へ正規化するのは、保存しているパスと題名も NFC だからである（R-107）。
// macOS から NFD で送られた入力も、これで同じ表記に揃う。
func normalizeQuery(query string) string {
	return norm.NFC.String(strings.TrimSpace(query))
}

// searchFilter は検索の条件句と引数を返す。絞り込まない場合は空の句を返す。
//
// どちらの経路も videos_fts（trigram）を引く。LIKE 経路も trigram 索引で
// 処理されることは実行計画で確認済みである（fts_test.go）。
//
// 並び順は呼び出し側（一覧）が決める。関連度（bm25）を使わないのは、
// LIKE 経路に関連度が無く、2つの経路で並びが変わると利用者から見て
// 不可解になるためである（R-110）。
func searchFilter(query string) (condition string, args []any) {
	normalized := normalizeQuery(query)

	switch routeFor(query) {
	case routeMatch:
		return `videos.id in (select rowid from videos_fts where videos_fts match ?)`,
			[]any{quoteMatchQuery(normalized)}

	case routeLike:
		// title と path の双方を見る。題名は拡張子を除いたファイル名なので
		// ほぼ同じだが、ディレクトリ名で絞りたい場合に path が効く。
		return `videos.id in (select rowid from videos_fts where title like ? escape '\' ` +
				`or path like ? escape '\')`,
			[]any{likePattern(normalized), likePattern(normalized)}

	default:
		return "", nil
	}
}

// quoteMatchQuery は検索語を FTS5 の文字列として渡せる形にする。
//
// FTS5 の問い合わせ構文では " * : ^ - ( ) や AND / OR / NEAR が演算子に
// なる。包まないと、利用者が打った記号が構文誤りになって検索そのものが
// 失敗し、画面には「検索が壊れた」ようにしか見えない。全体を二重引用符で
// 包み、中の二重引用符は2つ重ねて字面に戻す。
func quoteMatchQuery(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

// likePattern は LIKE の部分一致の模様を組み立てる。
//
// 利用者が打った % と _ は LIKE のワイルドカードなので、字面として
// 扱えるよう逃がす。逃がさないと "100%" のような検索語が全件に一致する。
func likePattern(query string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	return "%" + escaped + "%"
}
