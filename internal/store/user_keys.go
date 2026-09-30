package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// 利用者データの鍵（specs/030-video-versions/data-model.md §3、research.md R-2）。
//
// 動画のタグ・再生位置・公開の設定（video_tags・playback_progress・public_videos）は、
// 集まりのメンバーなら集まりの user_key（'bundle:<id>'）に、そうでなければ content_key に
// 置く。読み書きのどこか 1 か所でも content_key のまま残ると「タグは見えるのに絞ると
// 出ない」といった食い違いになるので、鍵は userKeyExpr の 1 つの式で決める。
// 表示名とサムネイルの位置（video_overrides）、生成物は内容ごとで、content_key のまま。

// userKeyExpr は、動画（別名 alias の videos）の利用者データの鍵を返す式である。
// 空の content_key の動画では空文字になり、呼び出し側が今までどおり
// 空の content_key を除く条件で利用者データに結ばない。
func userKeyExpr(alias string) string {
	return `coalesce((select ukb.user_key from video_bundle_members ukm ` +
		`join video_bundles ukb on ukb.id = ukm.bundle_id ` +
		`where ukm.content_key = ` + alias + `.content_key), ` + alias + `.content_key)`
}

// bundleUserKey は集まりの id から user_key を作る。
func bundleUserKey(id int64) string {
	return fmt.Sprintf("bundle:%d", id)
}

// userKeysForVideoIDs は動画の id の集合を、いまライブラリにある動画の利用者データの鍵へ
// 引き直す。同じ集まりの動画は 1 つの鍵にまとめる。引けない id と空の content_key の動画は
// 含めない（registeredContentKeysForVideoIDs と同じ規則）。タグの付け外し・要約と公開の
// 切り替えが使う。
func userKeysForVideoIDs(ctx context.Context, q queryExecer, videoIDs []int64) ([]string, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}

	query := `select distinct ` + userKeyExpr("v") + ` from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and ` +
		registeredVideoCondition("v")
	rows, err := q.QueryContext(ctx, query, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	return keys, nil
}

// contentKeysForUserKeys は利用者データの鍵の集合を、それが指す内容の content_key へ広げる。
// 集まりの鍵はその全メンバーの content_key に、それ以外の鍵はそのままになる。公開の
// 切り替えが、ゲストの配信の打ち切り（内容ごと）に渡す鍵を作るのに使う。
func contentKeysForUserKeys(ctx context.Context, q queryExecer, userKeys []string) ([]string, error) {
	if len(userKeys) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(userKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build user keys: %w", err)
	}
	rows, err := q.QueryContext(ctx, `
		with selected(user_key) as (select value from json_each(?))
		select s.user_key from selected s
		 where not exists (select 1 from video_bundles b where b.user_key = s.user_key)
		union
		select m.content_key from selected s
		  join video_bundles b on b.user_key = s.user_key
		  join video_bundle_members m on m.bundle_id = b.id`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map user keys to content keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot map user keys to content keys: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot map user keys to content keys: %w", err)
	}
	return keys, nil
}
