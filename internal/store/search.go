package store

import (
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// searchRoute は検索に使う経路である。
type searchRoute int

const (
	// routeNone は絞り込まない（検索語が空）。
	routeNone searchRoute = iota
	// routeMatch は location_search_fts MATCH を使う。3文字以上の検索語。
	routeMatch
	// routeInstr は search_key への instr を使う。1〜2文字の検索語。
	routeInstr
)

// matchMinLength は MATCH 経路に回す最小の文字数である。
//
// trigram は3文字単位で索引を作るため、2文字以下の検索語は MATCH に
// 一致しない。日本語では「旅行」「花火」のような2文字の検索語が現実に
// 多いので、そこは instr に振り分ける。
const matchMinLength = 3

// routeFor は検索語から経路を選ぶ。
//
// 数えるのは符号位置ではなく、利用者が1文字と見るまとまりである。照合形
// （NFKC）へ正規化してから符号位置を数えると、結合文字で書かれた「が」
// （か + 濁点）が1文字に畳まれる。ここを取り違えると、2文字の入力が MATCH
// 経路へ回って何も返らない。
//
// 絵文字の ZWJ 連結のように正規化で畳まれない組み合わせは、なお複数として
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
	return routeInstr
}

// normalizeQuery は検索語を search_key と突き合わせられる照合形にする。
//
// search_key と同じ domain.FoldForMatch を掛けるので、NFC・NFD、全角半角、
// 大文字小文字、ひらがなとカタカナの違いが同じ表記に揃う
// （specs/013-library-search/data-model.md §3）。
func normalizeQuery(query string) string {
	return domain.FoldForMatch(strings.TrimSpace(query))
}

// searchFilter は検索の条件句と引数を返す。絞り込まない場合は空の句を返す。
//
// どちらの経路も所在ごとの search_key を引く。3文字以上は
// location_search_fts（trigram）の MATCH、1〜2文字は search_key への instr で
// 部分一致を調べる。instr は LIKE と違ってワイルドカードを持たないので、
// 利用者が打った % や _ を逃がす必要が無い。
//
// 並び順は呼び出し側（一覧）が決める。関連度（bm25）を使わないのは、
// instr 経路に関連度が無く、2つの経路で並びが変わると利用者から見て
// 不可解になるためである。
func searchFilter(query string) (condition string, args []any) {
	normalized := normalizeQuery(query)

	switch routeFor(query) {
	case routeMatch:
		return `videos.id in (select l.video_id from video_locations l where ` + registeredLocationCondition("l") +
				` and l.id in (select rowid from location_search_fts where location_search_fts match ?))`,
			[]any{quoteMatchQuery(normalized)}

	case routeInstr:
		return `videos.id in (select l.video_id from video_locations l where ` + registeredLocationCondition("l") +
				` and instr(l.search_key, ?) > 0)`,
			[]any{normalized}

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
