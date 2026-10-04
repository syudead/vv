package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 同じパスの中身の引き継ぎ（specs/030-video-versions/data-model.md §5、research.md R-5）。
//
// UpsertVideo は、同じパスの中身が変わって新しい動画の行を作り、前の動画の行が所在を失って
// 消えるとき、前の尺が分かっていれば video_successions に後継の候補を記録する。新しい中身の
// 尺は解析が終わるまで分からず、前の中身が同じ走査の中で別のパスに現れうる（ファイルの
// 入れ替え）ので、判定は記録した走査が done で閉じたあと（FinishScan）か、そのあとの解析の
// 結果を書く取引で行う。どちらも applySuccession を呼ぶ。

// recordSuccession は、パスの中身が oldKey から newKey に変わって前の動画の行が消えたことを
// 記録する。oldDurationMs が分からなければ（解析前・失敗）記録しない。newKey の既存の行は
// 置き換える。
func recordSuccession(ctx context.Context, tx *sql.Tx, newKey, oldKey string, oldDurationMs sql.NullInt64, now int64) error {
	if newKey == "" || oldKey == "" || !oldDurationMs.Valid || oldDurationMs.Int64 <= 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `insert or replace into video_successions
		(new_key, old_key, old_duration_ms, ready, created_at) values (?, ?, ?, 0, ?)`,
		newKey, oldKey, oldDurationMs.Int64, now); err != nil {
		return fmt.Errorf("cannot record the succession: %w", err)
	}
	return nil
}

// cancelSuccessionsFrom は、前の中身が key の記録を消す。前の中身が別のパスの動画として
// 現れたので、同じパスの新しい中身はその後継ではない（Edge Case「ファイルの入れ替え」）。
func cancelSuccessionsFrom(ctx context.Context, tx *sql.Tx, key string) error {
	if _, err := tx.ExecContext(ctx, `delete from video_successions where old_key = ?`, key); err != nil {
		return fmt.Errorf("cannot cancel the succession: %w", err)
	}
	return nil
}

// releaseContentIndex は、動画の行を消して内容の参照が無くなった鍵の索引の行を、同じ取引で
// 消す（data-model.md §1）。
func releaseContentIndex(ctx context.Context, tx *sql.Tx, released []domain.DeletedVideo) error {
	if len(released) == 0 {
		return nil
	}
	keys := make([]string, 0, len(released))
	for _, video := range released {
		keys = append(keys, video.ContentKey)
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("cannot build content keys: %w", err)
	}
	for _, statement := range []string{
		`delete from video_successions where new_key in (select value from json_each(?))`,
		`delete from video_fingerprints where content_key in (select value from json_each(?))`,
		`delete from video_version_candidates where key_a in (select value from json_each(?1))
		    or key_b in (select value from json_each(?1))`,
	} {
		if _, err := tx.ExecContext(ctx, statement, string(encoded)); err != nil {
			return fmt.Errorf("cannot release the content index: %w", err)
		}
	}
	return nil
}

// applyReadySuccessions は done で閉じる走査の取引で呼ぶ。全行を判定してよい状態（ready = 1）
// にし、新しい中身の尺が分かっている行を判定する。引き継いだとき知らせる動画の id を返す。
func applyReadySuccessions(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	if _, err := tx.ExecContext(ctx, `update video_successions set ready = 1`); err != nil {
		return nil, fmt.Errorf("cannot mark the successions ready: %w", err)
	}
	ready, err := judgeableSuccessions(ctx, tx)
	if err != nil {
		return nil, err
	}
	var changed []int64
	for _, p := range ready {
		ids, err := applySuccession(ctx, tx, p.newKey, p.durationMs)
		if err != nil {
			return nil, err
		}
		changed = append(changed, ids...)
	}
	return normalizeIDs(changed), nil
}

// judgeableSuccession は判定する記録の新しい鍵と、その動画の尺である。
type judgeableSuccession struct {
	newKey     string
	durationMs int64
}

// judgeableSuccessions は新しい中身の尺が分かっている記録を読み切る。
func judgeableSuccessions(ctx context.Context, tx *sql.Tx) ([]judgeableSuccession, error) {
	rows, err := tx.QueryContext(ctx, `select s.new_key, v.duration_ms from video_successions s
		join videos v on v.content_key = s.new_key and v.content_key <> ''
		where v.duration_ms is not null order by s.new_key`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the successions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []judgeableSuccession
	for rows.Next() {
		var j judgeableSuccession
		if err := rows.Scan(&j.newKey, &j.durationMs); err != nil {
			return nil, fmt.Errorf("cannot read the successions: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the successions: %w", err)
	}
	return out, nil
}

// applySuccession は contentKey の判定してよい（ready = 1）記録を読んで消し、尺が合い、前の
// 中身を参照する動画が無ければ、前の中身の利用者データと集まりの位置を新しい中身へ付け替える
// （data-model.md §5）。引き継いだとき、確定後に domain.VideoBundleChanged で知らせる動画の
// id を返す（引き継がなければ空）。
func applySuccession(ctx context.Context, tx *sql.Tx, contentKey string, durationMs int64) ([]int64, error) {
	if contentKey == "" {
		return nil, nil
	}
	var oldKey string
	var oldDurationMs int64
	err := tx.QueryRowContext(ctx, `select old_key, old_duration_ms from video_successions
		where new_key = ? and ready = 1`, contentKey).Scan(&oldKey, &oldDurationMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the succession: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `delete from video_successions where new_key = ?`, contentKey); err != nil {
		return nil, fmt.Errorf("cannot remove the succession: %w", err)
	}
	if !domain.DurationsMatch(oldDurationMs, durationMs) {
		return nil, nil
	}
	var referenced int
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from videos where content_key = ?)`, oldKey).Scan(&referenced); err != nil {
		return nil, fmt.Errorf("cannot read the previous content: %w", err)
	}
	if referenced != 0 {
		return nil, nil
	}
	var newID int64
	err = tx.QueryRowContext(ctx, `select id from videos where content_key = ?`, contentKey).Scan(&newID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the succeeding video: %w", err)
	}

	bundle, bundled, err := bundleOf(ctx, tx, oldKey)
	if err != nil {
		return nil, err
	}
	if bundled {
		if err := moveBundleMember(ctx, tx, bundle, oldKey, contentKey); err != nil {
			return nil, err
		}
	}
	// 集まりのメンバーだった場合も、前の鍵自身の行（集まりに入る前の値）を付け替える。
	if err := moveUserData(ctx, tx, oldKey, contentKey); err != nil {
		return nil, err
	}
	if err := moveDismissals(ctx, tx, oldKey, contentKey); err != nil {
		return nil, err
	}
	// 付け替えた集まりと却下で、新しい鍵の候補が同じ集まりや却下の組になりうる（data-model.md §1 の
	// 不変条件）。
	if err := pruneVersionCandidates(ctx, tx); err != nil {
		return nil, err
	}

	changed := []int64{newID}
	if bundled {
		keys, err := bundleMemberKeys(ctx, tx, bundle.id)
		if err != nil {
			return nil, err
		}
		ids, err := videoIDsForContentKeys(ctx, tx, keys)
		if err != nil {
			return nil, err
		}
		changed = append(changed, ids...)
		// 見せる動画（data-model.md §4）が変わりうるので、束ねる操作と同じく作り直す。
		if err := rebuildFolderIndex(ctx, tx); err != nil {
			return nil, err
		}
	}
	return normalizeIDs(changed), nil
}

// moveBundleMember は集まりのメンバーの鍵 oldKey を newKey に付け替え、代表だったなら代表も
// 付け替える。集まりの値には触らない。newKey が既にどれかの集まりのメンバーなら、その集まりの
// 値を持っているので付け替えない。
func moveBundleMember(ctx context.Context, tx *sql.Tx, bundle bundleRef, oldKey, newKey string) error {
	if _, already, err := bundleOf(ctx, tx, newKey); err != nil {
		return err
	} else if already {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `update video_bundle_members set content_key = ? where content_key = ?`, newKey, oldKey); err != nil {
		return fmt.Errorf("cannot move the bundle member: %w", err)
	}
	if bundle.representativeKey == oldKey {
		if _, err := tx.ExecContext(ctx, `update video_bundles set representative_key = ?, updated_at = ? where id = ?`,
			newKey, time.Now().Unix(), bundle.id); err != nil {
			return fmt.Errorf("cannot move the bundle representative: %w", err)
		}
	}
	return nil
}

// moveUserData は鍵 from の再生位置・公開の設定・お気に入り・更新日時・タグを鍵 to へ付け替える。
// 再生位置・公開の設定・お気に入り・更新日時は from に行があれば to の行を置き換え、タグは和にする。
func moveUserData(ctx context.Context, tx *sql.Tx, from, to string) error {
	for _, table := range []string{"playback_progress", "public_videos", "video_favorites", "video_edits"} {
		if _, err := tx.ExecContext(ctx, `delete from `+table+` where content_key = ?
			and exists (select 1 from `+table+` where content_key = ?)`, to, from); err != nil {
			return fmt.Errorf("cannot move the %s values: %w", table, err)
		}
		if _, err := tx.ExecContext(ctx, `update `+table+` set content_key = ? where content_key = ?`, to, from); err != nil {
			return fmt.Errorf("cannot move the %s values: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `insert or ignore into video_tags (content_key, tag_id, created_at)
		select ?, tag_id, created_at from video_tags where content_key = ?`, to, from); err != nil {
		return fmt.Errorf("cannot move the video_tags values: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `delete from video_tags where content_key = ?`, from); err != nil {
		return fmt.Errorf("cannot move the video_tags values: %w", err)
	}
	return nil
}

// moveDismissals は「違う動画」の組の鍵 from を to に付け替える。組は並べ直し、重複は 1 つにし、
// to 自身との組は捨てる。
func moveDismissals(ctx context.Context, tx *sql.Tx, from, to string) error {
	pairs, err := dismissalsOf(ctx, tx, from)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from video_version_dismissals where key_a = ? or key_b = ?`, from, from); err != nil {
		return fmt.Errorf("cannot move the dismissals: %w", err)
	}
	for _, p := range pairs {
		if p.other == to {
			continue
		}
		if _, err := tx.ExecContext(ctx, `insert or ignore into video_version_dismissals (key_a, key_b, created_at)
			values (min(?, ?), max(?, ?), ?)`, to, p.other, to, p.other, p.createdAt); err != nil {
			return fmt.Errorf("cannot move the dismissals: %w", err)
		}
	}
	return nil
}

// dismissal は鍵の「違う動画」の組の相手と、判断した時刻である。
type dismissal struct {
	other     string
	createdAt int64
}

// dismissalsOf は鍵 key を含む「違う動画」の組を読み切る。
func dismissalsOf(ctx context.Context, tx *sql.Tx, key string) ([]dismissal, error) {
	rows, err := tx.QueryContext(ctx, `select key_a, key_b, created_at from video_version_dismissals
		where key_a = ? or key_b = ?`, key, key)
	if err != nil {
		return nil, fmt.Errorf("cannot read the dismissals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []dismissal
	for rows.Next() {
		var a, b string
		var d dismissal
		if err := rows.Scan(&a, &b, &d.createdAt); err != nil {
			return nil, fmt.Errorf("cannot read the dismissals: %w", err)
		}
		d.other = a
		if a == key {
			d.other = b
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the dismissals: %w", err)
	}
	return out, nil
}

// successionEvents は引き継ぎで知らせる動画の id を、確定後に発行する変化にする。
func successionEvents(ids []int64) []domain.Event {
	if len(ids) == 0 {
		return nil
	}
	return []domain.Event{domain.VideoBundleChanged{VideoIDs: ids}}
}

// normalizeIDs は id を小さい順に並べ、重複を除く。
func normalizeIDs(ids []int64) []int64 {
	slices.Sort(ids)
	return slices.Compact(ids)
}
