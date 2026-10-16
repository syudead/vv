package store

import "github.com/syudead/vv/internal/domain"

// フォルダ由来のタグ（017 の data-model.md §4）。
//
// 動画の祖先のフォルダ名（video_folder_names）が tag_names の名前（元の名前か
// シノニム）に完全一致すれば、その動画にそのタグが付いているものとして扱う。
// 付いていることは保存せず、読み出しのたびにこの突き合わせで導くので、タグの
// 作成・改名・シノニム・削除・統合は次の読み出しからそのまま効く。
//
// 手で付けた分（video_tags）は利用者データの鍵（userKeyExpr、specs/030-video-versions/
// data-model.md §3）で動画に結ぶ。内容の識別子が空の
// 動画は手で付けた分を持てないので、フォルダ名の分も持たないものとして扱い、
// Video.tags（content_key で引く）と絞り込み・検索・本数が食い違わないように
// する。

// videoHasTagCondition は、動画（別名 alias の videos）に、あるタグが手で
// 付いているかフォルダ名から付いているかの条件句を返す。userKey は動画の利用者データの鍵の式
// （ふつうは userKeyExpr(alias)）である。呼び出し側はタグの id を2つ（手で付けた分、フォルダ名の分）
// 引数として渡す。
func videoHasTagCondition(alias, userKey string) string {
	return videoTagSourcesCondition(alias, userKey, func(tagIDColumn string) string { return tagIDColumn + ` = ?` })
}

// videoHasTagOfKindCondition は、動画（別名 alias の videos）に、ある種類のタグが手で
// 付いているかフォルダ名から付いているかの条件句を返す。userKey は videoHasTagCondition と同じで、
// 呼び出し側は種類の id を2つ（手で付けた分、フォルダ名の分）引数として渡す
// （specs/051-tag-kinds/data-model.md の Rules）。
func videoHasTagOfKindCondition(alias, userKey string) string {
	return videoTagSourcesCondition(alias, userKey, func(tagIDColumn string) string {
		return tagIDColumn + ` in (select id from tags where kind_id = ?)`
	})
}

// videoTagSourcesCondition は、動画（別名 alias の videos）に、tagMatch が当たるタグが手で
// 付いているかフォルダ名から付いているかの条件句を返す。tagMatch はタグの id の列を受けて
// その列に対する条件を返し、2つの出所それぞれに1回ずつ使われる。
//
// どちらの出所も相関しない副問い合わせ（in）にする。SQLite はタグの付いた鍵と動画の集合を
// 問い合わせごとに 1 度だけ作り、動画 1 本ごとにはそれを引くだけになる。動画 1 本ごとに exists を
// 評価すると、スマートフォルダが 1 回の読み出しで 2 万本を何度も絞るときに遅い
// （specs/052-smart-folders/research.md R-13）。フォルダ名の分はタグの名前から引く（cross join で
// 結ぶ順を決める）。
func videoTagSourcesCondition(alias, userKey string, tagMatch func(tagIDColumn string) string) string {
	return `(` + alias + `.content_key <> '' and (` + userKey +
		` in (select vt.content_key from video_tags vt where ` + tagMatch("vt.tag_id") + `) ` +
		`or ` + alias + `.id in (select vfn.video_id from tag_names tn cross join video_folder_names vfn on vfn.name = tn.name ` +
		`where ` + tagMatch("tn.tag_id") + `)))`
}

// taggedVideosSQL は、いまライブラリにある見せる動画（所有者から見た shownVideoCondition。
// 集まりは実効の代表の1本、specs/030-video-versions/data-model.md §4）とそれに付いたタグの組
// (tag_id, video_id) を、どちらかの出所で付いていれば1行ずつ（重複なしで）
// 返す副問い合わせである。extra は両方の出所に足す条件（先頭に and を付けて
// 渡す）で、その引数は呼び出し側が2回（手で付けた分、フォルダ名の分）渡す。
func taggedVideosSQL(extra string) string {
	return `select vt.tag_id as tag_id, v.id as video_id from videos v
		join video_tags vt on vt.content_key = ` + userKeyExpr("v") + `
		where v.content_key <> '' and ` + registeredVideoCondition("v") + ` and ` + shownVideoCondition("v", domain.AudienceOwner) + extra + `
		union
		select tn.tag_id as tag_id, v.id as video_id from video_folder_names vfn
		join tag_names tn on tn.name = vfn.name
		join videos v on v.id = vfn.video_id and v.content_key <> ''
		where ` + registeredVideoCondition("v") + ` and ` + shownVideoCondition("v", domain.AudienceOwner) + extra
}
