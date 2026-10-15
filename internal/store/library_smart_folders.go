package store

// ライブラリの一番上のスマートフォルダ（specs/052-smart-folders/data-model.md「Library items」、
// research.md R-7）。絞り込みの無いライブラリには親の無いフォルダを、お気に入りのみにはお気に入りに
// したフォルダを深さによらず、グループのカードや動画と混ぜて並べる。カードの値はフォルダの中身を
// 読んで求める（smart_folder_reads.go の viewSmartFolder）。

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

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

// librarySmartFolders は、一覧の条件 q でライブラリの一番上に出すスマートフォルダのカードを、
// 見る人（audience）に見せるものだけ返す。ゲストには見せる動画の無いフォルダを出さない
// （research.md R-8）。now は追加日の絞り込みの基準時刻で、一覧と同じ値を使う。
func librarySmartFolders(ctx context.Context, tx *sql.Tx, audience domain.Audience, q domain.VideoQuery, now int64) ([]domain.LibrarySmartFolder, error) {
	ids, err := librarySmartFolderIDs(ctx, tx, q)
	if err != nil {
		return nil, err
	}
	cards := make([]domain.LibrarySmartFolder, 0, len(ids))
	for _, id := range ids {
		folder, err := readSmartFolder(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		rules, err := smartFolderPathRules(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		view, err := viewSmartFolder(ctx, tx, audience, folder, rules, now, nil)
		if err != nil {
			return nil, err
		}
		if view.shown(audience) {
			cards = append(cards, view.card)
		}
	}
	return cards, nil
}
