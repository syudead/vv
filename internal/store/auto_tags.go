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

// 自動タグ付けと Ollama の設定のキーである（docs/design-docs/auto-tagging.md）。
const (
	// autoTagFeatureKey は自動タグ付けの機能を使うかで、"true" か "false"。行が無ければ切。
	autoTagFeatureKey = "auto_tag.feature_enabled"
	// autoTagAfterImportKey は取り込みのあとに自動で判定するかで、"true" か "false"。
	autoTagAfterImportKey        = "auto_tag.after_import"
	autoTagThresholdKey          = "auto_tag.threshold"
	autoTagTentativeThresholdKey = "auto_tag.tentative_threshold"

	ollamaEndpointKey = "ollama.endpoint"
	ollamaModelKey    = "ollama.model"
	// legacyOllamaEndpointKey と legacyOllamaModelKey は、接続先を自動タグ付けの設定と一緒に
	// 持っていた版のキーである。新しいキーが無ければ読む。
	legacyOllamaEndpointKey = "auto_tag.endpoint"
	legacyOllamaModelKey    = "auto_tag.model"
)

// AutoTagSettings は保存した自動タグ付けの設定を返す。行の無い項目は既定値
// （domain.DefaultAutoTagSettings）になる。読めない確率も既定値に倒す。
func (s *SettingsStore) AutoTagSettings(ctx context.Context) (domain.AutoTagSettings, error) {
	settings := domain.DefaultAutoTagSettings()
	var err error
	if settings.Enabled, err = readBoolSetting(ctx, s.db.sql, autoTagFeatureKey); err != nil {
		return domain.AutoTagSettings{}, err
	}
	if settings.AfterImport, err = readBoolSetting(ctx, s.db.sql, autoTagAfterImportKey); err != nil {
		return domain.AutoTagSettings{}, err
	}
	if settings.Threshold, err = readFloatSetting(ctx, s.db.sql, autoTagThresholdKey, settings.Threshold); err != nil {
		return domain.AutoTagSettings{}, err
	}
	if settings.TentativeThreshold, err = readFloatSetting(ctx, s.db.sql, autoTagTentativeThresholdKey,
		settings.TentativeThreshold); err != nil {
		return domain.AutoTagSettings{}, err
	}
	// 仮の確率だけが保存されていないときなどに、確定の確率を越えないようにする。
	settings.TentativeThreshold = min(settings.TentativeThreshold, settings.Threshold)
	return settings, nil
}

// SaveAutoTagSettings は自動タグ付けの設定を 1 つの取引で保存する。値は呼び出し側が
// domain.ValidateAutoTagSettings で確かめたものを渡す。
func (s *SettingsStore) SaveAutoTagSettings(ctx context.Context, settings domain.AutoTagSettings) error {
	return writeSettings(ctx, s.db.sql, []settingValue{
		{autoTagFeatureKey, strconv.FormatBool(settings.Enabled)},
		{autoTagAfterImportKey, strconv.FormatBool(settings.AfterImport)},
		{autoTagThresholdKey, strconv.FormatFloat(settings.Threshold, 'f', -1, 64)},
		{autoTagTentativeThresholdKey, strconv.FormatFloat(settings.TentativeThreshold, 'f', -1, 64)},
	})
}

// OllamaSettings は保存した Ollama の接続先を返す。行が無ければ既定値
// （domain.DefaultOllamaSettings）になる。
func (s *SettingsStore) OllamaSettings(ctx context.Context) (domain.OllamaSettings, error) {
	settings := domain.DefaultOllamaSettings()
	for _, item := range []struct {
		keys   []string
		target *string
	}{
		{[]string{ollamaEndpointKey, legacyOllamaEndpointKey}, &settings.Endpoint},
		{[]string{ollamaModelKey, legacyOllamaModelKey}, &settings.Model},
	} {
		for _, key := range item.keys {
			value, found, err := readSetting(ctx, s.db.sql, key)
			if err != nil {
				return domain.OllamaSettings{}, err
			}
			if found {
				*item.target = value
				break
			}
		}
	}
	return settings, nil
}

// OllamaConnectionSaved は所有者が Ollama の接続先を保存したか（接続先の行があるか）を返す。
// OllamaSettings は行が無ければ既定の接続先に倒すので、こちらで行の有無を見る
// （specs/054-auto-dup/research.md R-13）。前の版のキーの行も保存とみなす。
func (s *SettingsStore) OllamaConnectionSaved(ctx context.Context) (bool, error) {
	for _, key := range []string{ollamaEndpointKey, legacyOllamaEndpointKey} {
		_, found, err := readSetting(ctx, s.db.sql, key)
		if err != nil || found {
			return found, err
		}
	}
	return false, nil
}

// SaveOllamaSettings は Ollama の接続先を 1 つの取引で保存する。値は呼び出し側が
// domain.NormalizeOllamaSettings で確かめたものを渡す。
func (s *SettingsStore) SaveOllamaSettings(ctx context.Context, settings domain.OllamaSettings) error {
	return writeSettings(ctx, s.db.sql, []settingValue{
		{ollamaEndpointKey, settings.Endpoint},
		{ollamaModelKey, settings.Model},
	})
}

func readBoolSetting(ctx context.Context, q *sql.DB, key string) (bool, error) {
	value, found, err := readSetting(ctx, q, key)
	return found && value == strconv.FormatBool(true), err
}

// readFloatSetting は数の設定を読む。行が無いか読めなければ fallback を返す。
func readFloatSetting(ctx context.Context, q *sql.DB, key string, fallback float64) (float64, error) {
	value, found, err := readSetting(ctx, q, key)
	if err != nil || !found {
		return fallback, err
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback, nil
	}
	return parsed, nil
}

// settingValue は保存する設定の 1 つである。キーと値を別のフィールドに持ち、誤りの文に入れる
// キーが値（Comfy Cloud の API キーなど）と同じ配列の要素にならないようにする。
type settingValue struct {
	key   string
	value string
}

// writeSettings は複数の設定を 1 つの取引で保存する。誤りの文にはキーだけを入れ、値は入れない。
func writeSettings(ctx context.Context, q *sql.DB, values []settingValue) error {
	tx, err := q.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	for _, setting := range values {
		if _, err := tx.ExecContext(ctx, `
			insert into settings (key, value, updated_at) values (?, ?, ?)
			on conflict (key) do update set value = excluded.value, updated_at = excluded.updated_at`,
			setting.key, setting.value, now); err != nil {
			return fmt.Errorf("cannot save setting %s: %w", setting.key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot save settings: %w", err)
	}
	return nil
}

// QueueVideos は videoIDs のうち、登録フォルダの下に所在を持つ動画の内容を待ち行列へ積み、
// 積んだ件数を返す。既にある行の扱いは mode で決まり、判定中の行は動かさない。積んだ内容ごとに、decider が
// 始めた running/waiting の auto_tag の記録を開く（specs/055-activity/data-model.md）。
func (s *AutoTagStore) QueueVideos(
	ctx context.Context, videoIDs []int64, mode domain.AutoTagQueueMode, decider domain.ActivityDecider,
) (int, error) {
	if len(videoIDs) == 0 {
		return 0, nil
	}
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return 0, fmt.Errorf("cannot build video id values: %w", err)
	}
	return s.queue(ctx, `select v.content_key from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and `+
		registeredVideoCondition("v"), []any{string(encoded)}, mode, decider)
}

// QueueLibrary は登録フォルダの下に所在を持つすべての動画の内容を待ち行列へ積み、積んだ件数を
// 返す。既にある行の扱いと記録は QueueVideos と同じ。
func (s *AutoTagStore) QueueLibrary(ctx context.Context, mode domain.AutoTagQueueMode, decider domain.ActivityDecider) (int, error) {
	return s.queue(ctx, `select v.content_key from videos v
		where v.content_key <> '' and `+registeredVideoCondition("v"), nil, mode, decider)
}

// queueableStates は積み方 mode が積む行の状態である。空文字は行が無いことを表す。
func queueableStates(mode domain.AutoTagQueueMode) (string, error) {
	switch mode {
	case domain.AutoTagQueueIfNew:
		return `''`, nil
	case domain.AutoTagQueueUnlessDone:
		return `'', 'failed'`, nil
	case domain.AutoTagQueueAgain:
		return `'', 'done', 'failed'`, nil
	default:
		return "", fmt.Errorf("unknown auto-tagging queue mode %d", mode)
	}
}

// queue は selectKeys が返す内容の識別子を mode に従って 1 つの取引で積み、内容ごとに記録を開いて行の
// activity_id に持たせる。確定後に domain.ActivityChanged を発行する。
func (s *AutoTagStore) queue(
	ctx context.Context, selectKeys string, args []any, mode domain.AutoTagQueueMode, decider domain.ActivityDecider,
) (int, error) {
	states, err := queueableStates(mode)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	subjects, err := queueableAutoTagSubjects(ctx, tx, `select k.content_key, `+contentLabelExpr("k.content_key")+`
		from (select distinct content_key from (`+selectKeys+`)) k
		where coalesce((select q.state from auto_tag_queue q where q.content_key = k.content_key), '') in (`+states+`)
		order by k.content_key`, args)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	c := &changes{}
	for _, subject := range subjects {
		id, err := recordActivity(ctx, tx, c,
			autoTagActivityEntry(subject.Key, subject.Label, domain.ActivityWaiting, decider), now)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			insert into auto_tag_queue (content_key, state, error, queued_at, updated_at, activity_id)
			values (?, 'queued', '', ?, ?, ?)
			on conflict (content_key) do update set state = 'queued', error = '', queued_at = excluded.queued_at,
				updated_at = excluded.updated_at, activity_id = excluded.activity_id`,
			subject.Key, now.UnixMilli(), now.UnixMilli(), id); err != nil {
			return 0, fmt.Errorf("cannot queue videos for auto-tagging: %w", err)
		}
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return 0, fmt.Errorf("cannot queue videos for auto-tagging: %w", err)
	}
	return len(subjects), nil
}

// queueableAutoTagSubjects は積む内容の鍵と表示名を読む。
func queueableAutoTagSubjects(ctx context.Context, tx *sql.Tx, query string, args []any) ([]domain.ActivitySubject, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos to queue for auto-tagging: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subjects []domain.ActivitySubject
	for rows.Next() {
		subject := domain.ActivitySubject{Kind: domain.ActivitySubjectVideo}
		if err := rows.Scan(&subject.Key, &subject.Label); err != nil {
			return nil, fmt.Errorf("cannot read the videos to queue for auto-tagging: %w", err)
		}
		subjects = append(subjects, subject)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos to queue for auto-tagging: %w", err)
	}
	return subjects, nil
}

// Claim は積んだ順にいちばん古い判定を判定中にして返す。無ければ domain.ErrNoJob。行の記録を processing に
// し、記録が running でなければ新しい記録を開く。
func (s *AutoTagStore) Claim(ctx context.Context) (domain.AutoTagJob, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.AutoTagJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	var key string
	var activityID int64
	err = tx.QueryRowContext(ctx, `
		update auto_tag_queue set state = 'running', updated_at = ?
		where content_key = (select content_key from auto_tag_queue where state = 'queued'
			order by queued_at, content_key limit 1)
		returning content_key, coalesce(activity_id, 0)`, now.UnixMilli()).Scan(&key, &activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AutoTagJob{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.AutoTagJob{}, fmt.Errorf("cannot claim an auto-tagging job: %w", err)
	}
	c := &changes{}
	if err := claimAutoTagActivity(ctx, tx, c, key, activityID, now); err != nil {
		return domain.AutoTagJob{}, err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return domain.AutoTagJob{}, fmt.Errorf("cannot claim an auto-tagging job: %w", err)
	}
	return domain.AutoTagJob{ContentKey: key}, nil
}

// Subject は内容の識別子の動画を所有者の見え方で、代表の所在を含む登録フォルダ（入れ子なら
// 最も深いもの）とともに返す。登録フォルダの下に所在を持つ動画が無ければ domain.ErrNotFound。
func (s *AutoTagStore) Subject(ctx context.Context, contentKey string) (domain.AutoTagVideo, error) {
	id, err := videoIDForContent(ctx, s.db.sql, contentKey)
	if err != nil {
		return domain.AutoTagVideo{}, err
	}
	video, err := getVideo(ctx, s.db.sql, domain.AudienceOwner, id)
	if err != nil {
		return domain.AutoTagVideo{}, err
	}
	folders, err := listMediaFolders(ctx, s.db.sql)
	if err != nil {
		return domain.AutoTagVideo{}, err
	}
	subject := domain.AutoTagVideo{Video: video}
	for _, folder := range folders {
		if domain.PathInsideRoot(folder.Path, video.Path) && len(folder.Path) > len(subject.MediaFolder) {
			subject.MediaFolder = folder.Path
		}
	}
	if subject.MediaFolder == "" {
		return domain.AutoTagVideo{}, domain.ErrNotFound
	}
	return subject, nil
}

// Candidates は内容の動画に聞く既存のタグを名前の順で返す。仮タグ、その動画（集まりの
// メンバーなら集まり）に確定して付いているタグ、その動画で所有者が却下したタグは除く。
// 仮に付いているタグは、判定し直して確定することがあるので含める。動画がライブラリに
// 無ければ domain.ErrNotFound。
func (s *AutoTagStore) Candidates(ctx context.Context, contentKey string) ([]domain.AutoTagCandidate, error) {
	videoID, err := videoIDForContent(ctx, s.db.sql, contentKey)
	if err != nil {
		return nil, err
	}
	keys, err := userKeysForVideoIDs(ctx, s.db.sql, []int64{videoID})
	if err != nil {
		return nil, err
	}
	if len(keys) != 1 {
		return nil, domain.ErrNotFound
	}
	rows, err := s.db.sql.QueryContext(ctx, `
		select t.id, n.name, n.canonical from tags t
		  join tag_names n on n.tag_id = t.id
		 where t.tentative = 0
		   and not exists (select 1 from video_tags vt
		                    where vt.content_key = ?1 and vt.tag_id = t.id and vt.tentative = 0)
		   and not exists (select 1 from auto_tag_rejections r
		                    where r.content_key = ?1 and r.tag_id = t.id)
		 order by t.id, n.canonical desc, n.name`, keys[0])
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
// 同じ取引で行の記録を done/completed にし、decider が空でなければそれを判断したものとして書き、付けたタグを
// 確率とともに残す。確定の後に domain.AutoTagApplied と domain.ActivityChanged を発行する。
func (s *AutoTagStore) Finish(
	ctx context.Context, contentKey string, selection domain.AutoTagSelection, decider domain.ActivityDecider,
) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var state string
	var activityID int64
	err = tx.QueryRowContext(ctx, `select state, coalesce(activity_id, 0) from auto_tag_queue where content_key = ?`,
		contentKey).Scan(&state, &activityID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state != "running") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the auto-tagging job: %w", err)
	}
	now := time.Now()
	c := &changes{}
	completed := domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityCompleted}

	videoID, err := videoIDForContent(ctx, tx, contentKey)
	if errors.Is(err, domain.ErrNotFound) {
		if _, err := tx.ExecContext(ctx, `delete from auto_tag_queue where content_key = ?`, contentKey); err != nil {
			return fmt.Errorf("cannot remove the auto-tagging job: %w", err)
		}
		if err := closeAutoTagActivity(ctx, tx, c, activityID, completed, "", "", now); err != nil {
			return err
		}
		return s.db.commit(ctx, tx, c)
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

	written, err := writeAutoTags(ctx, tx, userKey, selection, now)
	if err != nil {
		return err
	}
	if len(written) > 0 {
		if err := touchEditedAtForUserKeys(ctx, tx, []string{userKey}, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `update auto_tag_queue set state = 'done', error = '', updated_at = ?
		where content_key = ?`, now.UnixMilli(), contentKey); err != nil {
		return fmt.Errorf("cannot finish the auto-tagging job: %w", err)
	}
	if err := closeAutoTagActivity(ctx, tx, c, activityID, completed, decider, "", now); err != nil {
		return err
	}
	if activityID != 0 {
		if err := recordAutoTagActivityTags(ctx, tx, activityID, userKey, written, selection.Probabilities, now); err != nil {
			return err
		}
	}
	// 集まりのメンバーなら、タグは集まりの鍵に付き、すべてのメンバーに見える。メンバーの
	// どの動画を開いている画面にも知らせる。
	affected, err := videoIDsSharingUserKey(ctx, tx, videoID)
	if err != nil {
		return err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
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
	// 提案のタグを残すか却下するのは所有者の操作である（specs/055-activity/research.md R-6）。
	kind := domain.ActivityTagKeep
	if reject {
		kind = domain.ActivityTagReject
	}
	c := &changes{}
	if err := recordOwnerVideoActivity(ctx, tx, c, kind, ownerTagChanges([]string{userKey}, tagID), now); err != nil {
		return err
	}
	affected, err := videoIDsSharingUserKey(ctx, tx, videoID)
	if err != nil {
		return err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot settle the tentative tag (tag=%d): %w", tagID, err)
	}
	s.db.publishEvents(domain.AutoTagApplied{VideoIDs: affected})
	return nil
}

// Fail は判定中の行を失敗にし、理由を残す。同じ取引で行の記録を attention/failed にし、理由を detail に書く。
// 行が判定中でなくなっていれば何もしない。
func (s *AutoTagStore) Fail(ctx context.Context, contentKey, reason string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	var activityID int64
	err = tx.QueryRowContext(ctx, `update auto_tag_queue set state = 'failed', error = ?, updated_at = ?
		where content_key = ? and state = 'running' returning coalesce(activity_id, 0)`,
		reason, now.UnixMilli(), contentKey).Scan(&activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot record the auto-tagging failure: %w", err)
	}
	c := &changes{}
	if err := closeAutoTagActivity(ctx, tx, c, activityID,
		domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityFailed}, "", reason, now); err != nil {
		return err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
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
	return videoIDsWithUserKey(ctx, tx, `(select `+userKeyExpr("t")+` from videos t where t.id = ?)`, videoID)
}

// videoIDsForUserKey は利用者データの鍵 userKey を持つ動画（集まりならすべてのメンバー）の id を、id の順で返す。
func videoIDsForUserKey(ctx context.Context, tx *sql.Tx, userKey string) ([]int64, error) {
	return videoIDsWithUserKey(ctx, tx, `?`, userKey)
}

// videoIDsWithUserKey は利用者データの鍵が式 keyExpr（引数 arg を 1 つ取る）の値である動画の id を、id の順で
// 返す。
func videoIDsWithUserKey(ctx context.Context, tx *sql.Tx, keyExpr string, arg any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `select v.id from videos v
		where v.content_key <> '' and `+userKeyExpr("v")+` = `+keyExpr+`
		order by v.id`, arg)
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
