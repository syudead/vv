package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 自動タグ付けの設定のキーである（docs/design-docs/auto-tagging.md）。
const (
	autoTagEnabledKey   = "auto_tag.enabled"
	autoTagEndpointKey  = "auto_tag.endpoint"
	autoTagModelKey     = "auto_tag.model"
	autoTagThresholdKey = "auto_tag.threshold"
	// autoTagTentativeThresholdKey は仮に付けるのに要る確率のキーである。
	autoTagTentativeThresholdKey = "auto_tag.tentative_threshold"
)

// AutoTagSettings は保存した自動タグ付けの設定を返す。行の無い項目は既定値
// （domain.DefaultAutoTagSettings）になる。読めない閾値も既定値に倒す。
func (s *SettingsStore) AutoTagSettings(ctx context.Context) (domain.AutoTagSettings, error) {
	settings := domain.DefaultAutoTagSettings()
	value, found, err := readSetting(ctx, s.db.sql, autoTagEnabledKey)
	if err != nil {
		return domain.AutoTagSettings{}, err
	}
	settings.Enabled = found && value == strconv.FormatBool(true)
	if value, found, err = readSetting(ctx, s.db.sql, autoTagEndpointKey); err != nil {
		return domain.AutoTagSettings{}, err
	} else if found {
		settings.Endpoint = value
	}
	if value, found, err = readSetting(ctx, s.db.sql, autoTagModelKey); err != nil {
		return domain.AutoTagSettings{}, err
	} else if found {
		settings.Model = value
	}
	if value, found, err = readSetting(ctx, s.db.sql, autoTagThresholdKey); err != nil {
		return domain.AutoTagSettings{}, err
	} else if found {
		if threshold, err := strconv.ParseFloat(value, 64); err == nil {
			settings.Threshold = threshold
		}
	}
	if value, found, err = readSetting(ctx, s.db.sql, autoTagTentativeThresholdKey); err != nil {
		return domain.AutoTagSettings{}, err
	} else if found {
		if threshold, err := strconv.ParseFloat(value, 64); err == nil {
			settings.TentativeThreshold = threshold
		}
	}
	// 仮の閾値だけが保存されていない（この版より前に保存した）ときなどに、確定の閾値を
	// 越えないようにする。
	settings.TentativeThreshold = min(settings.TentativeThreshold, settings.Threshold)
	return settings, nil
}

// SaveAutoTagSettings は自動タグ付けの設定を 1 つの取引で保存する。値は呼び出し側が
// domain.NormalizeAutoTagSettings で確かめたものを渡す。
func (s *SettingsStore) SaveAutoTagSettings(ctx context.Context, settings domain.AutoTagSettings) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	values := [][2]string{
		{autoTagEnabledKey, strconv.FormatBool(settings.Enabled)},
		{autoTagEndpointKey, settings.Endpoint},
		{autoTagModelKey, settings.Model},
		{autoTagThresholdKey, strconv.FormatFloat(settings.Threshold, 'f', -1, 64)},
		{autoTagTentativeThresholdKey, strconv.FormatFloat(settings.TentativeThreshold, 'f', -1, 64)},
	}
	now := time.Now().Unix()
	for _, kv := range values {
		if _, err := tx.ExecContext(ctx, `
			insert into settings (key, value, updated_at) values (?, ?, ?)
			on conflict (key) do update set value = excluded.value, updated_at = excluded.updated_at`,
			kv[0], kv[1], now); err != nil {
			return fmt.Errorf("cannot save setting %s: %w", kv[0], err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot save the auto-tagging settings: %w", err)
	}
	return nil
}

// QueueVideos は videoIDs のうち、登録フォルダの下に所在を持つ動画の内容を待ち行列へ積み、
// 積んだ件数を返す。既にある行の扱いは mode で決まり、判定中の行は動かさない。
func (s *AutoTagStore) QueueVideos(ctx context.Context, videoIDs []int64, mode domain.AutoTagQueueMode) (int, error) {
	if len(videoIDs) == 0 {
		return 0, nil
	}
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return 0, fmt.Errorf("cannot build video id values: %w", err)
	}
	return s.queue(ctx, `select v.content_key from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and `+
		registeredVideoCondition("v"), []any{string(encoded)}, mode)
}

// QueueLibrary は登録フォルダの下に所在を持つすべての動画の内容を待ち行列へ積み、積んだ件数を
// 返す。既にある行の扱いは QueueVideos と同じ。
func (s *AutoTagStore) QueueLibrary(ctx context.Context, mode domain.AutoTagQueueMode) (int, error) {
	return s.queue(ctx, `select v.content_key from videos v
		where v.content_key <> '' and `+registeredVideoCondition("v"), nil, mode)
}

// queue は selectKeys が返す内容の識別子を mode に従って積む。
func (s *AutoTagStore) queue(ctx context.Context, selectKeys string, args []any, mode domain.AutoTagQueueMode) (int, error) {
	var conflict string
	switch mode {
	case domain.AutoTagQueueIfNew:
		conflict = `do nothing`
	case domain.AutoTagQueueUnlessDone:
		conflict = `do update set state = 'queued', error = '', queued_at = excluded.queued_at,
			updated_at = excluded.updated_at where auto_tag_queue.state = 'failed'`
	case domain.AutoTagQueueAgain:
		conflict = `do update set state = 'queued', error = '', queued_at = excluded.queued_at,
			updated_at = excluded.updated_at where auto_tag_queue.state in ('done', 'failed')`
	default:
		return 0, fmt.Errorf("unknown auto-tagging queue mode %d", mode)
	}
	now := time.Now().UnixMilli()
	// on conflict を select からの insert に付けるときは、構文の曖昧さを避けるために where が要る。
	result, err := s.db.sql.ExecContext(ctx, `
		insert into auto_tag_queue (content_key, state, error, queued_at, updated_at)
		select content_key, 'queued', '', ?, ? from (`+selectKeys+`) where true
		on conflict (content_key) `+conflict,
		append([]any{now, now}, args...)...)
	if err != nil {
		return 0, fmt.Errorf("cannot queue videos for auto-tagging: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot queue videos for auto-tagging: %w", err)
	}
	return int(affected), nil
}

// Claim は積んだ順にいちばん古い判定を判定中にして返す。無ければ domain.ErrNoJob。
func (s *AutoTagStore) Claim(ctx context.Context) (domain.AutoTagJob, error) {
	var key string
	err := s.db.sql.QueryRowContext(ctx, `
		update auto_tag_queue set state = 'running', updated_at = ?
		where content_key = (select content_key from auto_tag_queue where state = 'queued'
			order by queued_at, content_key limit 1)
		returning content_key`, time.Now().UnixMilli()).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AutoTagJob{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.AutoTagJob{}, fmt.Errorf("cannot claim an auto-tagging job: %w", err)
	}
	return domain.AutoTagJob{ContentKey: key}, nil
}

// Subject は内容の識別子の動画を所有者の見え方で返す。登録フォルダの下に所在を持つ動画が
// 無ければ domain.ErrNotFound。
func (s *AutoTagStore) Subject(ctx context.Context, contentKey string) (domain.Video, error) {
	id, err := videoIDForContent(ctx, s.db.sql, contentKey)
	if err != nil {
		return domain.Video{}, err
	}
	return getVideo(ctx, s.db.sql, domain.AudienceOwner, id)
}

// Candidates は判定に回す既存のタグを、仮タグを除いて名前の順で返す。
func (s *AutoTagStore) Candidates(ctx context.Context) ([]domain.AutoTagCandidate, error) {
	rows, err := s.db.sql.QueryContext(ctx, `
		select t.id, n.name, n.canonical from tags t
		  join tag_names n on n.tag_id = t.id
		 where t.tentative = 0
		 order by t.id, n.canonical desc, n.name`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var candidates []domain.AutoTagCandidate
	for rows.Next() {
		var id int64
		var name string
		var canonical bool
		if err := rows.Scan(&id, &name, &canonical); err != nil {
			return nil, fmt.Errorf("cannot read the tags: %w", err)
		}
		if canonical || len(candidates) == 0 || candidates[len(candidates)-1].ID != id {
			candidates = append(candidates, domain.AutoTagCandidate{ID: id, Name: name})
			continue
		}
		last := &candidates[len(candidates)-1]
		last.Synonyms = append(last.Synonyms, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the tags: %w", err)
	}
	domain.SortAutoTagCandidates(candidates)
	return candidates, nil
}

// Finish は判定中の行を終え、selection のタグ（今もあるもの）をその内容の動画へ付ける。
// Confirmed は確定した付与として付け、仮に付いていれば確定する。Tentative は仮に付け、既に
// 付いていれば（手で付けた分も）そのままにする。所有者がこの動画で却下したタグは付けない。
// 行が判定中でなくなっていた（積み直された）なら何もしない。動画が消えていれば行を消す。
// 確定の後に domain.AutoTagApplied を発行する。
func (s *AutoTagStore) Finish(ctx context.Context, contentKey string, selection domain.AutoTagSelection) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var state string
	err = tx.QueryRowContext(ctx, `select state from auto_tag_queue where content_key = ?`, contentKey).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state != "running") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the auto-tagging job: %w", err)
	}

	videoID, err := videoIDForContent(ctx, tx, contentKey)
	if errors.Is(err, domain.ErrNotFound) {
		if _, err := tx.ExecContext(ctx, `delete from auto_tag_queue where content_key = ?`, contentKey); err != nil {
			return fmt.Errorf("cannot remove the auto-tagging job: %w", err)
		}
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	keys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(keys) != 1 {
		return fmt.Errorf("cannot find the user key of video %d", videoID)
	}
	userKey := keys[0]

	now := time.Now()
	confirmed, err := attachableAutoTags(ctx, tx, userKey, selection.Confirmed)
	if err != nil {
		return err
	}
	for _, tagID := range confirmed {
		if _, err := attachTagToVideoIDs(ctx, tx, []int64{videoID}, tagID, now); err != nil {
			return err
		}
	}
	tentative, err := attachableAutoTags(ctx, tx, userKey, selection.Tentative)
	if err != nil {
		return err
	}
	var added bool
	for _, tagID := range tentative {
		result, err := tx.ExecContext(ctx, `insert or ignore into video_tags (content_key, tag_id, created_at, tentative)
			values (?, ?, ?, 1)`, userKey, tagID, now.Unix())
		if err != nil {
			return fmt.Errorf("cannot add a tentative tag (tag=%d): %w", tagID, err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return fmt.Errorf("cannot add a tentative tag (tag=%d): %w", tagID, err)
		} else if affected > 0 {
			added = true
		}
	}
	if added {
		if err := touchEditedAtForUserKeys(ctx, tx, []string{userKey}, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `update auto_tag_queue set state = 'done', error = '', updated_at = ?
		where content_key = ?`, now.UnixMilli(), contentKey); err != nil {
		return fmt.Errorf("cannot finish the auto-tagging job: %w", err)
	}
	// 集まりのメンバーなら、タグは集まりの鍵に付き、すべてのメンバーに見える。メンバーの
	// どの動画を開いている画面にも知らせる。
	affected, err := videoIDsSharingUserKey(ctx, tx, videoID)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot finish the auto-tagging job: %w", err)
	}
	s.db.publishEvents(domain.AutoTagApplied{VideoIDs: affected})
	return nil
}

// attachableAutoTags は tagIDs のうち、今もあり、利用者データの鍵 userKey の動画で所有者が
// 却下していないタグの id を渡した順で返す。
func attachableAutoTags(ctx context.Context, tx *sql.Tx, userKey string, tagIDs []int64) ([]int64, error) {
	existing, _, err := existingTagIDs(ctx, tx, tagIDs)
	if err != nil || len(existing) == 0 {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `select tag_id from auto_tag_rejections where content_key = ?`, userKey)
	if err != nil {
		return nil, fmt.Errorf("cannot read the rejected tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	rejected := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the rejected tags: %w", err)
		}
		rejected[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the rejected tags: %w", err)
	}
	var out []int64
	for _, id := range existing {
		if !rejected[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// ConfirmTentative は動画 videoID に仮に付いたタグ tagID を確定する。仮に付いていなければ
// domain.ErrNotFound。確定の後に domain.AutoTagApplied を発行する。
func (s *AutoTagStore) ConfirmTentative(ctx context.Context, videoID, tagID int64) error {
	return s.settleTentative(ctx, videoID, tagID, false)
}

// RejectTentative は動画 videoID に仮に付いたタグ tagID を外し、この動画にこのタグを自動
// タグ付けで付け直さないよう覚える。仮に付いていなければ domain.ErrNotFound。
func (s *AutoTagStore) RejectTentative(ctx context.Context, videoID, tagID int64) error {
	return s.settleTentative(ctx, videoID, tagID, true)
}

func (s *AutoTagStore) settleTentative(ctx context.Context, videoID, tagID int64, reject bool) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	keys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(keys) != 1 {
		return domain.ErrNotFound
	}
	userKey := keys[0]
	statement := `update video_tags set tentative = 0 where content_key = ? and tag_id = ? and tentative = 1`
	if reject {
		statement = `delete from video_tags where content_key = ? and tag_id = ? and tentative = 1`
	}
	result, err := tx.ExecContext(ctx, statement, userKey, tagID)
	if err != nil {
		return fmt.Errorf("cannot settle the tentative tag (tag=%d): %w", tagID, err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("cannot settle the tentative tag (tag=%d): %w", tagID, err)
	} else if affected == 0 {
		return domain.ErrNotFound
	}
	now := time.Now()
	if reject {
		if _, err := tx.ExecContext(ctx, `insert into auto_tag_rejections (content_key, tag_id, rejected_at)
			values (?, ?, ?) on conflict (content_key, tag_id) do update set rejected_at = excluded.rejected_at`,
			userKey, tagID, now.Unix()); err != nil {
			return fmt.Errorf("cannot remember the rejected tag (tag=%d): %w", tagID, err)
		}
	}
	if err := touchEditedAtForUserKeys(ctx, tx, []string{userKey}, now); err != nil {
		return err
	}
	affected, err := videoIDsSharingUserKey(ctx, tx, videoID)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot settle the tentative tag (tag=%d): %w", tagID, err)
	}
	s.db.publishEvents(domain.AutoTagApplied{VideoIDs: affected})
	return nil
}

// Fail は判定中の行を失敗にし、理由を残す。行が判定中でなくなっていれば何もしない。
func (s *AutoTagStore) Fail(ctx context.Context, contentKey, reason string) error {
	if _, err := s.db.sql.ExecContext(ctx, `update auto_tag_queue set state = 'failed', error = ?, updated_at = ?
		where content_key = ? and state = 'running'`, reason, time.Now().UnixMilli(), contentKey); err != nil {
		return fmt.Errorf("cannot record the auto-tagging failure: %w", err)
	}
	return nil
}

// RequeueRunning は判定中のまま残った行（前回の終了で途切れたもの）を積み直し、件数を返す。
// 起動時に、判定を始める前に呼ぶ。
func (s *AutoTagStore) RequeueRunning(ctx context.Context) (int, error) {
	result, err := s.db.sql.ExecContext(ctx, `update auto_tag_queue set state = 'queued', updated_at = ?
		where state = 'running'`, time.Now().UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted auto-tagging jobs: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted auto-tagging jobs: %w", err)
	}
	return int(affected), nil
}

// Counts は待ち行列の状態ごとの件数と、直近の失敗の理由を返す。
func (s *AutoTagStore) Counts(ctx context.Context) (domain.AutoTagCounts, error) {
	var counts domain.AutoTagCounts
	err := s.db.sql.QueryRowContext(ctx, `
		select
			coalesce(sum(state = 'queued'), 0),
			coalesce(sum(state = 'running'), 0),
			coalesce(sum(state = 'done'), 0),
			coalesce(sum(state = 'failed'), 0),
			coalesce((select error from auto_tag_queue where state = 'failed'
				order by updated_at desc, content_key limit 1), '')
		from auto_tag_queue`).Scan(&counts.Queued, &counts.Running, &counts.Done, &counts.Failed, &counts.LastError)
	if err != nil {
		return domain.AutoTagCounts{}, fmt.Errorf("cannot count auto-tagging jobs: %w", err)
	}
	return counts, nil
}

// videoIDForContent は内容の識別子の動画のうち、登録フォルダの下に所在を持つものの id を返す。
// 無ければ domain.ErrNotFound。
func videoIDForContent(ctx context.Context, q rowQueryer, contentKey string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `select v.id from videos v where v.content_key = ? and v.content_key <> '' and `+
		registeredVideoCondition("v"), contentKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("cannot read the video: %w", err)
	}
	return id, nil
}

// videoIDsSharingUserKey は videoID の動画と利用者データの鍵が同じ動画（集まりのメンバー、
// 束ねていなければその動画だけ）の id を、id の順で返す。
func videoIDsSharingUserKey(ctx context.Context, tx *sql.Tx, videoID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `select v.id from videos v
		where v.content_key <> '' and `+userKeyExpr("v")+` = (
			select `+userKeyExpr("t")+` from videos t where t.id = ?)
		order by v.id`, videoID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos sharing the tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the videos sharing the tags: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos sharing the tags: %w", err)
	}
	return ids, nil
}
