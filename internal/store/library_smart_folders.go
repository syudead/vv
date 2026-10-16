package store

// ライブラリの一番上のスマートフォルダ（specs/052-smart-folders/data-model.md「Library items」、
// research.md R-7・R-14）。絞り込みの無いライブラリには親の無いフォルダを、お気に入りのみにはお気に入りに
// したフォルダを深さによらず、グループのカードや動画と混ぜて並べる。項目はフォルダの要約だけを持ち、
// 並べ替えの値はフォルダの一致する動画（smart_folder_matches.go）から 1 つの問い合わせで求める。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// librarySmartFolder はライブラリの一番上に並べるフォルダ 1 つである。
type librarySmartFolder struct {
	summary domain.SmartFolderSummary
	// values は項目の列 added_at・mtime・created_at・duration_ms・size_bytes・played_at の値で、
	// 一致する動画の最大（時刻、Unix 秒）と合計（長さと大きさ）である。一致する動画が無ければどれも
	// nil で、並べ替えでは値の無い項目として末尾に置く（itemOrder）。
	values [6]any
}

// librarySmartFolderIDs は、一覧の条件 q でライブラリの一番上に出すスマートフォルダの id を
// id の順に返す。q が出さない条件なら空である。見る人に見せるかは確かめない。
func librarySmartFolderIDs(ctx context.Context, tx *sql.Tx, q domain.VideoQuery) ([]int64, error) {
	if !q.ListsSmartFolders() {
		return nil, nil
	}
	query := `select id from smart_folders where parent_id is null order by id`
	if q.FavoriteOnly {
		query = `select smart_folder_id from smart_folder_favorites order by smart_folder_id`
	}
	ids, err := queryInt64s(ctx, tx, query)
	if err != nil {
		return nil, fmt.Errorf("cannot read the library smart folders: %w", err)
	}
	return ids, nil
}

// librarySmartFolders は、一覧の条件 q でライブラリの一番上に出すスマートフォルダを、見る人
// （audience）に見せるものだけ、並べ替えの値とともに返す。ゲストには再生順の空のフォルダを出さない
// （research.md R-8）。now は追加日の絞り込みの基準時刻で、一覧と同じ値を使う。
func librarySmartFolders(ctx context.Context, tx *sql.Tx, audience domain.Audience, q domain.VideoQuery, now int64) ([]librarySmartFolder, error) {
	ids, err := librarySmartFolderIDs(ctx, tx, q)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	targets, err := shownSmartFolderTargets(ctx, tx, audience, ids, now)
	if err != nil {
		return nil, err
	}
	folders := make([]librarySmartFolder, len(targets))
	for i, target := range targets {
		folders[i].summary = target.summary
	}
	if err := readSmartFolderSortValues(ctx, tx, audience, targets, folders); err != nil {
		return nil, err
	}
	return folders, nil
}

// readSmartFolderSortValues はフォルダ targets の並べ替えの値を、smart_base を 1 度だけ求める
// 1 つの問い合わせで読み、同じ位置の folders[i].values に入れる。
func readSmartFolderSortValues(ctx context.Context, tx *sql.Tx, audience domain.Audience, targets []smartFolderTarget, folders []librarySmartFolder) error {
	query := smartFolderBranches(targets, func(query *smartFolderQuery, index int, _ smartFolderTarget, match smartMatch) {
		query.parts = append(query.parts, `select `+strconv.Itoa(index)+`, max(added_at), max(mtime), max(created_at),
			sum(duration_ms), sum(size_bytes), max(played_at) from smart_base videos where `+match.condition)
		query.partArgs = append(query.partArgs, match.args...)
	})
	rows, err := query.run(ctx, tx, audience)
	if err != nil {
		return fmt.Errorf("cannot read the smart folder sort values: %w", err)
	}
	if rows == nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var index int
		var values [6]sql.NullInt64
		if err := rows.Scan(&index, &values[0], &values[1], &values[2], &values[3], &values[4], &values[5]); err != nil {
			return fmt.Errorf("cannot read the smart folder sort values: %w", err)
		}
		for i, value := range values {
			if value.Valid {
				folders[index].values[i] = value.Int64
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the smart folder sort values: %w", err)
	}
	for index, earlier := range query.same {
		folders[index].values = folders[earlier].values
	}
	return nil
}

// smartFolderItemsSQL は、ライブラリの一番上に並べるスマートフォルダ folders（spec.smartFolders）を
// 項目にする、items に足す union の句とその引数を返す。フォルダが無ければどちらも空である。項目の
// id はフォルダの id を負にした値で、動画とグループの項目の id（正）と重ならない（カーソルは種類を
// 運ぶ）。題名とお気に入りにした日時はフォルダのもので、ほかの値は一致する動画から求めた値である
// （data-model.md「Library items」）。視聴状態は持たない（視聴状態の絞り込みではフォルダを出さない）。
func smartFolderItemsSQL(folders []librarySmartFolder) (string, []any) {
	if len(folders) == 0 {
		return "", nil
	}
	rows := make([][]any, 0, len(folders))
	for _, folder := range folders {
		rows = append(rows, append([]any{folder.summary.ID}, folder.values[:]...))
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		// 数と null だけの並びなので起きない。
		panic(fmt.Sprintf("cannot encode smart folder sort values: %v", err))
	}
	return `
		union all
		select null, -f.id, null, null, json_extract(c.value, '$[1]'), json_extract(c.value, '$[2]'),
			json_extract(c.value, '$[3]'), f.title_key, json_extract(c.value, '$[4]'), json_extract(c.value, '$[5]'),
			json_extract(c.value, '$[6]'),
			(select ff.favorited_at from smart_folder_favorites ff where ff.smart_folder_id = f.id),
			null
		from json_each(?) c join smart_folders f on f.id = json_extract(c.value, '$[0]')`, []any{string(encoded)}
}

// librarySmartFolderSummary はライブラリに並べるフォルダ folders のうち id のものの要約を返す。
func librarySmartFolderSummary(folders []librarySmartFolder, id int64) (*domain.SmartFolderSummary, bool) {
	for i := range folders {
		if folders[i].summary.ID == id {
			return &folders[i].summary, true
		}
	}
	return nil, false
}
