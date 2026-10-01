package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// 動画の更新日時（specs/033-video-dates/data-model.md §3・§4、research.md R-1〜R-3）。
// 所有者が vv 上で動画の情報を最後に編集した日時を、内容の識別子に結ぶ video_edits の 1 行で
// 持つ。行が無い動画の更新日時は追加日時（videos.added_at）で、読み出しが倒す。進めるのは
// 表示名・代表サムネイルの位置・公開の設定・手で付けるタグの付け外しが実際に値を変えた
// 内容の識別子だけで、それぞれの役割の型が自分の取引の中で touchEditedAt を呼ぶ。

// editedAtColumn は動画を返す読み出し（videoColumns・listColumns）の末尾に足す列で、
// scanVideo が domain.Video.EditedAt に写す。内容の識別子が空の動画は追加日時になる。
const editedAtColumn = `coalesce((select e.edited_at from video_edits e
		where e.content_key = videos.content_key and videos.content_key <> ''), videos.added_at) as edited_at`

// touchEditedAt は contentKeys の更新日時を now にする。空の内容の識別子は飛ばす。now は
// 操作の取引を始めた時刻で、同じ取引の対象はすべて同じ値になる。鍵の数によらず 1 文で書く
// （一括のタグ付けで 2 万件を進めても、書き込みの取引の中の文は増えない）。
func touchEditedAt(ctx context.Context, tx *sql.Tx, contentKeys []string, now time.Time) error {
	if len(contentKeys) == 0 {
		return nil
	}
	encoded, err := json.Marshal(contentKeys)
	if err != nil {
		return fmt.Errorf("cannot build content keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`insert into video_edits (content_key, edited_at)
		 select distinct value, ? from json_each(?) where value <> ''
		 on conflict (content_key) do update set edited_at = excluded.edited_at`,
		now.Unix(), string(encoded),
	); err != nil {
		return fmt.Errorf("cannot record the edited time: %w", err)
	}
	return nil
}

// touchEditedAtForUserKeys は利用者データの鍵 userKeys を内容の識別子へ広げ（集まりの鍵なら
// 全メンバー、contentKeysForUserKeys）、その更新日時を now にする。
func touchEditedAtForUserKeys(ctx context.Context, tx *sql.Tx, userKeys []string, now time.Time) error {
	if len(userKeys) == 0 {
		return nil
	}
	keys, err := contentKeysForUserKeys(ctx, tx, userKeys)
	if err != nil {
		return err
	}
	return touchEditedAt(ctx, tx, keys, now)
}
