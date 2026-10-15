package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// お気に入り（specs/035-favorites/data-model.md §1〜§4、research.md R-1）。動画のお気に入りは
// 利用者データの鍵（userKeyExpr）に結ぶ video_favorites の行、グループのお気に入りはグループの
// id に結ぶ group_favorites の行（specs/048-groups/research.md R-10）、スマートフォルダのお気に入りは
// フォルダの id に結ぶ smart_folder_favorites の行（specs/052-smart-folders/data-model.md
// 「Migration」）の有無で表す。どれも作り直せない利用者データで、生成物の片付け・フォルダの索引の
// 作り直し・メディアフォルダの削除は触れない。group_favorites の行はグループの解散で、
// smart_folder_favorites の行はフォルダの削除で、外部キーの on delete cascade により消える。

// favoriteColumn は動画（videos）がお気に入りかを 0/1 で返す列の式である。domain.Video.Favorite
// に写す。形は publicColumn と同じである。
var favoriteColumn = `exists (select 1 from video_favorites fav where fav.content_key = ` + userKeyExpr("videos") +
	` and videos.content_key <> '')`

// favoriteNow は付け外しの時刻の元である。試験が時計を止めるために差し替える。
var favoriteNow = time.Now

// SetFavorites は change の動画・グループ・スマートフォルダのお気に入りを change.Favorite に
// そろえ、反映した数を返す（data-model.md §4）。
//
// 動画の id は利用者データの鍵へ引き直し（userKeysForVideoIDs）、その鍵に書く。Videos は鍵の数で
// なく、鍵を引けた異なる id の数である。グループとスマートフォルダは今あるもの（existingIDs）だけを
// 書く。ライブラリに無い動画の id・無いグループやフォルダの id は数えず誤りにしない。既に同じ状態の
// ものは数え、日時は変えない。
//
// favorited_at は Unix ミリ秒で、取引の中で max(今の時刻, 3 表の最大値 + 1) を 1 度だけ求め、
// この取引の対象すべてに使う。動画の更新日時（video_edits）は進めない（research.md R-8）。
// 全体を 1 つの取引で行い、途中で失敗したら何も残さない。ドメインイベントは発行しない。
func (s *FavoriteStore) SetFavorites(ctx context.Context, change domain.FavoriteChange) (domain.FavoriteApplied, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FavoriteApplied{}, fmt.Errorf("cannot update favorites: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	userKeys, err := userKeysForVideoIDs(ctx, tx, change.VideoIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}
	videos, err := countMappedVideoIDs(ctx, tx, change.VideoIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}
	groups, err := existingGroupIDs(ctx, tx, change.GroupIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}
	folders, err := existingIDs(ctx, tx, "smart_folders", change.SmartFolderIDs)
	if err != nil {
		return domain.FavoriteApplied{}, err
	}

	encodedKeys, err := json.Marshal(nonNil(userKeys))
	if err != nil {
		return domain.FavoriteApplied{}, fmt.Errorf("cannot build user keys: %w", err)
	}
	targets := []favoriteTable{
		{table: "video_favorites", column: "content_key", values: string(encodedKeys)},
	}
	for _, ids := range []struct {
		table, column string
		ids           []int64
	}{{"group_favorites", "group_id", groups}, {"smart_folder_favorites", "smart_folder_id", folders}} {
		encoded, err := json.Marshal(nonNil(ids.ids))
		if err != nil {
			return domain.FavoriteApplied{}, fmt.Errorf("cannot build %s ids: %w", ids.column, err)
		}
		targets = append(targets, favoriteTable{table: ids.table, column: ids.column, values: string(encoded)})
	}

	if change.Favorite {
		var at int64
		if err := tx.QueryRowContext(ctx, `select max(?, coalesce((select max(m) from (
				select max(favorited_at) as m from video_favorites
				union all select max(favorited_at) from group_favorites
				union all select max(favorited_at) from smart_folder_favorites)), 0) + 1)`,
			favoriteNow().UnixMilli()).Scan(&at); err != nil {
			return domain.FavoriteApplied{}, fmt.Errorf("cannot decide the favorited time: %w", err)
		}
		// 空の鍵は書かない。数の id は空文字と等しくならないので、同じ条件でどれも書く。
		for _, target := range targets {
			if _, err := tx.ExecContext(ctx, `insert or ignore into `+target.table+` (`+target.column+`, favorited_at)
				select distinct value, ? from json_each(?) where value <> ''`, at, target.values); err != nil {
				return domain.FavoriteApplied{}, fmt.Errorf("cannot update %s: %w", target.table, err)
			}
		}
	} else {
		for _, target := range targets {
			if _, err := tx.ExecContext(ctx, `delete from `+target.table+`
				where `+target.column+` in (select value from json_each(?))`, target.values); err != nil {
				return domain.FavoriteApplied{}, fmt.Errorf("cannot update %s: %w", target.table, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.FavoriteApplied{}, fmt.Errorf("cannot update favorites: %w", err)
	}
	return domain.FavoriteApplied{Videos: videos, Groups: len(groups), SmartFolders: len(folders)}, nil
}

// favoriteTable はお気に入りの表 1 つと、そこへ書く値（JSON の配列）である。
type favoriteTable struct {
	table, column, values string
}

// countMappedVideoIDs は videoIDs のうち、userKeysForVideoIDs が鍵を引ける異なる id の数を返す
// （同じ条件の count(distinct v.id)）。同じ集まりの id も 1 本ずつ数える。
func countMappedVideoIDs(ctx context.Context, tx *sql.Tx, videoIDs []int64) (int, error) {
	if len(videoIDs) == 0 {
		return 0, nil
	}
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return 0, fmt.Errorf("cannot build video ids: %w", err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `select count(distinct v.id) from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and `+
		registeredVideoCondition("v"), string(encoded)).Scan(&count); err != nil {
		return 0, fmt.Errorf("cannot count the videos: %w", err)
	}
	return count, nil
}

// existingGroupIDs は ids のうち今あるグループの id を、最初に現れた順に重複なしで返す。
func existingGroupIDs(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	return existingIDs(ctx, tx, "groups", ids)
}

// existingIDs は ids のうち表 table（groups か smart_folders）に今ある行の id を、最初に現れた順に
// 重複なしで返す。
func existingIDs(ctx context.Context, tx *sql.Tx, table string, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build %s ids: %w", table, err)
	}
	found, err := queryInt64s(ctx, tx, `select id from `+table+` where id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the %s: %w", table, err)
	}
	// 要求は最大 20,000 件なので、集合で引いて線形時間に保つ。
	pending := make(map[int64]bool, len(found))
	for _, id := range found {
		pending[id] = true
	}
	var out []int64
	for _, id := range ids {
		if pending[id] {
			out = append(out, id)
			delete(pending, id)
		}
	}
	return out, nil
}

// nonNil は nil のスライスを空のスライスにする。json_each に null でなく [] を渡すため。
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
