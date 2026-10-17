package store

import (
	"strings"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
)

// 検索式を所在1行に対する SQL の条件句に組み立てる。どの所在を範囲にし、動画ごとにどうまとめるかは
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
// 所有者の OR には作品情報の品番と、結び付いた人物・団体の名前と別名への instr も
// 加える（specs/047-video-work-info/data-model.md「Search」、research.md R-6）。
//
// ゲストの検索はタグの名前とシノニムにも作品情報にも照合せず、所在の条件だけにする
// （specs/016-single-account-auth/contracts/guest-api.md §3、047 research.md R-10）。
// ゲストに見せないタグの名前や作品情報が、当たる動画の有無から漏れないようにするため。
func searchExprCondition(expr domain.SearchExpr, alias string, audience domain.Audience) (string, []any) {
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
			condition := `(` + locationCondition + `)`
			if audience.IsOwner() {
				condition = `(` + locationCondition + ` or ` + ownerFactsMatchCondition(alias) + `)`
				// 鍵の集合（タグ名の手で付けた分、作品情報の品番・人物・団体）を 2 回と、タグのフォルダ名の
				// 分で、同じ語を 9 つ渡す。
				args = append(args, text, text, text, text, text, text, text, text, text)
			}
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

// ownerFactsMatchCondition は、所在（別名 alias）の動画に付いたタグの元の名前かシノニムのどれか、
// または作品情報の品番・人物・団体のどれかが語に当たるかの条件句を返す。呼び出し側が FoldForMatch
// 済みの語を 9 つ（タグの手で付けた分と作品情報の品番・人物・団体の鍵の集合を userKeyVideosSQL の
// 2 か所に 1 回ずつ、タグのフォルダ名の分を 1 つ）引数として渡す。語に当たる鍵の集合を動画の id に
// 引き直してから所在の動画を照らすので、所在 1 行ごとに鍵を求めない。
//
// タグは手で付けた分（video_tags）とフォルダ名から付いている分（video_folder_names、017 の
// data-model.md §4）のどちらでもよい。手で付けた分と作品情報は利用者データの鍵（userKeyExpr、
// specs/030-video-versions/data-model.md §3）で結び、作品情報は品番の照合形（code_key）か、結び付いた
// 人物・団体の名前か別名の照合形（search_key）を照らす（specs/047-video-work-info/data-model.md
// 「Search」）。名前の照合形は名前の行にあるので、人物の改名や統合では動画側の鍵を書き直さない
// （R-6）。説明・日付・URL・外部 ID は照合しない。内容の識別子が空の動画は再生位置を持たない
// （listing.go の playback_progress の join）のと同じ理由で照合から除く。空文字列どうしが一致して
// 無関係な行を拾わないためで、フォルダ名の分も同じ扱いにして Video.tags（content_key で引く）と
// 食い違わないようにする。フォルダ名の分は語に当たるタグの名前から引く（cross join で結ぶ順を
// 決める）。フォルダ名の行を全部読むと、2 万本では検索 1 回に十数 ms かかる。
func ownerFactsMatchCondition(alias string) string {
	// 名前の表（タグ名・人物名・団体名）を外側にして、当たった名前の id から動画の行を索引で引く。
	// 統計の無い計画器は動画の行の表（video_tags など）を外側にして全部読むので、cross join で順を
	// 固定する。
	keys := `select vt.content_key from tag_names tn cross join video_tags vt on vt.tag_id = tn.tag_id where instr(tn.search_key, ?) > 0 ` +
		`union select wi.content_key from video_work_info wi where instr(wi.code_key, ?) > 0 ` +
		`union select vp.content_key from people_names pn ` +
		`cross join video_work_people vp on vp.person_id = pn.person_id where instr(pn.search_key, ?) > 0 ` +
		`union select vo.content_key from organization_names onm ` +
		`cross join video_work_organizations vo on vo.organization_id = onm.organization_id where instr(onm.search_key, ?) > 0`
	return `(` + alias + `.video_id in (` + userKeyVideosSQL(keys) + `) ` +
		`or ` + alias + `.video_id in (select vfn.video_id from tag_names tn ` +
		`cross join tag_names folder_tn on folder_tn.tag_id = tn.tag_id ` +
		`cross join video_folder_names vfn on vfn.name = folder_tn.name ` +
		`join videos v on v.id = vfn.video_id and v.content_key <> '' ` +
		`where instr(tn.search_key, ?) > 0))`
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
