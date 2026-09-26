package store

// フォルダ由来のタグ（017 の data-model.md §4）。
//
// 動画の祖先のフォルダ名（video_folder_names）が tag_names の名前（元の名前か
// シノニム）に完全一致すれば、その動画にそのタグが付いているものとして扱う。
// 付いていることは保存せず、読み出しのたびにこの突き合わせで導くので、タグの
// 作成・改名・シノニム・削除・統合は次の読み出しからそのまま効く。
//
// 手で付けた分（video_tags）は content_key で動画に結ぶ。内容の識別子が空の
// 動画は手で付けた分を持てないので、フォルダ名の分も持たないものとして扱い、
// Video.tags（content_key で引く）と絞り込み・検索・本数が食い違わないように
// する。

// videoHasTagCondition は、動画（別名 alias の videos）に、あるタグが手で
// 付いているかフォルダ名から付いているかの条件句を返す。呼び出し側はタグの
// id を2つ（手で付けた分、フォルダ名の分）引数として渡す。
func videoHasTagCondition(alias string) string {
	return `(exists (select 1 from video_tags vt where vt.content_key = ` + alias + `.content_key and vt.tag_id = ?) ` +
		`or exists (select 1 from video_folder_names vfn join tag_names tn on tn.name = vfn.name ` +
		`where vfn.video_id = ` + alias + `.id and ` + alias + `.content_key <> '' and tn.tag_id = ?))`
}

// taggedVideosSQL は、いまライブラリにある動画とそれに付いたタグの組
// (tag_id, video_id) を、どちらかの出所で付いていれば1行ずつ（重複なしで）
// 返す副問い合わせである。extra は両方の出所に足す条件（先頭に and を付けて
// 渡す）で、その引数は呼び出し側が2回（手で付けた分、フォルダ名の分）渡す。
func taggedVideosSQL(extra string) string {
	return `select vt.tag_id as tag_id, v.id as video_id from video_tags vt
		join videos v on v.content_key = vt.content_key
		where ` + registeredVideoCondition("v") + extra + `
		union
		select tn.tag_id as tag_id, v.id as video_id from video_folder_names vfn
		join tag_names tn on tn.name = vfn.name
		join videos v on v.id = vfn.video_id and v.content_key <> ''
		where ` + registeredVideoCondition("v") + extra
}
