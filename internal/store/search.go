package store

import (
	"strings"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
)

// 検索式を所在1行に対する SQL の条件句に組み立てる（specs/013-library-search/plan.md
// Structural Decisions 2）。どの所在を範囲にし、動画ごとにどうまとめるかは
// listing.go の chosenLocationsCTE が決める。

// matchMinLength は MATCH で調べる語の最小の文字数（照合形の符号位置の数）である。
//
// trigram は3文字単位で索引を作るため、2文字以下の語は MATCH に一致しない。
// 日本語では「旅行」「花火」のような2文字の語が現実に多いので、そこは instr で
// 調べる。照合形は NFKC を掛けてあるので、NFD の「が」（か + 濁点）は1文字に
// 畳まれている。
const matchMinLength = 3

// termUsesMatch は語を location_search_fts の MATCH で調べるかを返す。
func termUsesMatch(text string) bool {
	return utf8.RuneCountInString(text) >= matchMinLength
}

// searchExprCondition は検索式を、所在（別名 alias）1行に対する条件句と引数に
// 組み立てる（Structural Decisions 2）。式が空なら空の句を返す。
//
// 3文字以上の語は location_search_fts の MATCH、1〜2文字の語は search_key への
// instr で部分一致を調べる。instr は LIKE と違ってワイルドカードを持たないので、
// 利用者が打った % や _ を逃がす必要が無い。AND・OR・NOT は SQL の and・or・not で
// 結ぶ。
//
// 語1つごとの条件は、所在の条件（MATCH か instr）と、その所在の動画に付いた
// タグ名（元の名前・シノニムの両方）への instr の OR に広げる
// （specs/014-video-tags/data-model.md §7）。除外語はこの OR 全体の否定にする。
// タグは動画の単位なので、語の長さに関係なく instr で調べ、全文索引は足さない。
func searchExprCondition(expr domain.SearchExpr, alias string) (string, []any) {
	if expr.Empty() {
		return "", nil
	}
	var args []any
	clauses := make([]string, 0, len(expr.Clauses))
	for _, clause := range expr.Clauses {
		terms := make([]string, 0, len(clause.Terms))
		for _, term := range clause.Terms {
			// search_key は題名と相対パスを改行でつないでいる。フレーズの中の改行は
			// 空白にそろえ、2つの境目をまたいで当たらないようにする。
			text := strings.ReplaceAll(term.Text, "\n", " ")
			var locationCondition string
			if termUsesMatch(text) {
				locationCondition = alias + `.id in (select rowid from location_search_fts where location_search_fts match ?)`
				args = append(args, quoteMatchPhrase(text))
			} else {
				locationCondition = `instr(` + alias + `.search_key, ?) > 0`
				args = append(args, text)
			}
			condition := `(` + locationCondition + ` or ` + tagNameMatchCondition(alias) + `)`
			args = append(args, text)
			if term.Negated {
				condition = `not ` + condition
			}
			terms = append(terms, condition)
		}
		if len(terms) == 1 {
			clauses = append(clauses, terms[0])
		} else {
			clauses = append(clauses, `(`+strings.Join(terms, " or ")+`)`)
		}
	}
	return strings.Join(clauses, " and "), args
}

// tagNameMatchCondition は、所在（別名 alias）の動画に付いたタグの元の名前か
// シノニムのどれかが語に当たるかの条件句を返す。video_locations は content_key
// を持たないので、videos を経て動画に結ぶ（data-model.md §7）。呼び出し側が
// FoldForMatch 済みの語を1つ引数として渡す。内容の識別子が空の動画は
// 再生位置を持たない（listing.go の playback_progress の join）のと同じ理由で
// タグの照合からも除く。空文字列どうしが一致して無関係な行を拾わないため。
func tagNameMatchCondition(alias string) string {
	return `exists (select 1 from video_tags vt ` +
		`join tag_names tn on tn.tag_id = vt.tag_id ` +
		`join videos v on v.content_key = vt.content_key and v.content_key <> '' ` +
		`where v.id = ` + alias + `.video_id and instr(tn.search_key, ?) > 0)`
}

// quoteMatchPhrase は語を FTS5 の1つのフレーズとして渡せる形にする。
//
// FTS5 の問い合わせ構文では " * : ^ - ( ) や AND / OR / NEAR が演算子に
// なる。包まないと、利用者が打った記号が構文誤りになって検索そのものが
// 失敗し、画面には「検索が壊れた」ようにしか見えない。全体を二重引用符で
// 包み、中の二重引用符は2つ重ねて字面に戻す。
func quoteMatchPhrase(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}
