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

const (
	// Stored as true or false; absent means disabled.
	autoTagFeatureKey            = "auto_tag.feature_enabled"
	autoTagAfterImportKey        = "auto_tag.after_import"
	autoTagThresholdKey          = "auto_tag.threshold"
	autoTagTentativeThresholdKey = "auto_tag.tentative_threshold"

	ollamaEndpointKey = "ollama.endpoint"
	ollamaModelKey    = "ollama.model"
	// Absent or empty until the owner saves a generation model.
	ollamaGenerationModelKey = "ollama.generation_model"
	// Read legacy connection keys only when the current keys are absent.
	legacyOllamaEndpointKey = "auto_tag.endpoint"
	legacyOllamaModelKey    = "auto_tag.model"
)

// AutoTagSettings uses domain defaults for missing or invalid values.
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
	// A missing tentative threshold must not exceed the confirmed threshold.
	settings.TentativeThreshold = min(settings.TentativeThreshold, settings.Threshold)
	return settings, nil
}

// SaveAutoTagSettings expects values validated by domain.ValidateAutoTagSettings.
func (s *SettingsStore) SaveAutoTagSettings(ctx context.Context, settings domain.AutoTagSettings) error {
	return writeSettings(ctx, s.db.sql, []settingValue{
		{autoTagFeatureKey, strconv.FormatBool(settings.Enabled)},
		{autoTagAfterImportKey, strconv.FormatBool(settings.AfterImport)},
		{autoTagThresholdKey, strconv.FormatFloat(settings.Threshold, 'f', -1, 64)},
		{autoTagTentativeThresholdKey, strconv.FormatFloat(settings.TentativeThreshold, 'f', -1, 64)},
	})
}

// OllamaSettings uses domain defaults when no connection is saved.
func (s *SettingsStore) OllamaSettings(ctx context.Context) (domain.OllamaSettings, error) {
	settings, _, err := ollamaConnection(ctx, s.db.sql)
	return settings, err
}

// OllamaConnectionSaved distinguishes saved settings, including legacy keys, from default values.
func (s *SettingsStore) OllamaConnectionSaved(ctx context.Context) (bool, error) {
	_, saved, err := ollamaConnection(ctx, s.db.sql)
	return saved, err
}

// SavedOllamaSettings reads the settings and saved flag in one snapshot.
func (s *SettingsStore) SavedOllamaSettings(ctx context.Context) (domain.OllamaSettings, bool, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.OllamaSettings{}, false, fmt.Errorf("cannot start reading the Ollama settings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	settings, saved, err := ollamaConnection(ctx, tx)
	if err != nil {
		return domain.OllamaSettings{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.OllamaSettings{}, false, fmt.Errorf("cannot finish reading the Ollama settings: %w", err)
	}
	return settings, saved, nil
}

// Include legacy keys when detecting a saved connection; use defaults for absent values.
func ollamaConnection(ctx context.Context, q rowQueryer) (domain.OllamaSettings, bool, error) {
	settings := domain.DefaultOllamaSettings()
	saved := false
	for _, item := range []struct {
		keys   []string
		target *string
		marks  bool
	}{
		{[]string{ollamaEndpointKey, legacyOllamaEndpointKey}, &settings.Endpoint, true},
		{[]string{ollamaModelKey, legacyOllamaModelKey}, &settings.Model, false},
		{[]string{ollamaGenerationModelKey}, &settings.GenerationModel, false},
	} {
		for _, key := range item.keys {
			value, found, err := readSetting(ctx, q, key)
			if err != nil {
				return domain.OllamaSettings{}, false, err
			}
			if found {
				*item.target = value
				saved = saved || item.marks
				break
			}
		}
	}
	return settings, saved, nil
}

// SaveOllamaSettings expects values normalized by domain.NormalizeOllamaSettings.
func (s *SettingsStore) SaveOllamaSettings(ctx context.Context, settings domain.OllamaSettings) error {
	return writeSettings(ctx, s.db.sql, []settingValue{
		{ollamaEndpointKey, settings.Endpoint},
		{ollamaModelKey, settings.Model},
		{ollamaGenerationModelKey, settings.GenerationModel},
	})
}

func readBoolSetting(ctx context.Context, q rowQueryer, key string) (bool, error) {
	value, found, err := readSetting(ctx, q, key)
	return found && value == strconv.FormatBool(true), err
}

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

// Keep keys separate from values so diagnostics cannot accidentally expose secrets.
type settingValue struct {
	key   string
	value string
}

// Write settings atomically; errors include keys but never values.
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

// QueueVideos queues registered content and opens activities; mode controls existing work, but running jobs never
// move. The owner's request is claimed first, and also moves the videos' queued jobs first.
func (s *AutoTagStore) QueueVideos(
	ctx context.Context, videoIDs []int64, mode domain.AutoTagQueueMode, decider domain.ActivityDecider,
) (int, error) {
	priority := domain.JobPriorityImport
	if decider == domain.ActivityByOwner {
		priority = domain.JobPriorityOwner
	}
	return s.queueVideos(ctx, videoIDs, mode, contentRequest{decider: decider, priority: priority})
}

// RetryVideo queues the video again for the owner's retry of entryID, unless it is running; the new entry joins
// every pipeline of the retried entry, and its job is claimed first.
func (s *AutoTagStore) RetryVideo(ctx context.Context, videoID, entryID int64) (int, error) {
	return s.queueVideos(ctx, []int64{videoID}, domain.AutoTagQueueAgain, contentRequest{
		decider: domain.ActivityByOwner, continued: []int64{entryID}, priority: domain.JobPriorityOwner,
	})
}

func (s *AutoTagStore) queueVideos(
	ctx context.Context, videoIDs []int64, mode domain.AutoTagQueueMode, req contentRequest,
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
		registeredVideoCondition("v"), []any{string(encoded)}, mode, req)
}

// QueueLibrary applies QueueVideos semantics to all registered content, in the import class: a bulk request must not
// hold back the owner's requests on single videos.
func (s *AutoTagStore) QueueLibrary(ctx context.Context, mode domain.AutoTagQueueMode, decider domain.ActivityDecider) (int, error) {
	return s.queue(ctx, `select v.content_key from videos v
		where v.content_key <> '' and `+registeredVideoCondition("v"), nil, mode,
		contentRequest{decider: decider, priority: domain.JobPriorityImport})
}

// An empty state represents content neither queued nor recorded.
func queueableStates(mode domain.AutoTagQueueMode) (string, error) {
	switch mode {
	case domain.AutoTagQueueIfNew:
		return `''`, nil
	case domain.AutoTagQueueUnlessDone:
		return `'', 'failed'`, nil
	case domain.AutoTagQueueAgain:
		return queueAgainStates, nil
	default:
		return "", fmt.Errorf("unknown auto-tagging queue mode %d", mode)
	}
}

func (s *AutoTagStore) queue(
	ctx context.Context, selectKeys string, args []any, mode domain.AutoTagQueueMode, req contentRequest,
) (int, error) {
	states, err := queueableStates(mode)
	if err != nil {
		return 0, err
	}
	return s.db.queueContentFor(ctx, autoTagQueue, selectKeys, args, states, req)
}

// Subject returns the owner view with the deepest registered folder containing its representative location.
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

// Candidates excludes tentative tag definitions, confirmed attachments and rejected tags; tentative attachments remain eligible for promotion.
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

// Finish applies only to a still-running job, preserves owner attachments and rejections, and records the result and
// activity in the same transaction; the job ends with it.
func (s *AutoTagStore) Finish(
	ctx context.Context, contentKey string, selection domain.AutoTagSelection, decider domain.ActivityDecider,
) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	activityID, running, err := runningContent(ctx, tx, autoTagQueue, contentKey)
	if err != nil || !running {
		return err
	}
	now := time.Now()
	c := &changes{}
	completed := domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityCompleted}

	videoID, err := videoIDForContent(ctx, tx, contentKey)
	if errors.Is(err, domain.ErrNotFound) {
		if err := endContentJob(ctx, tx, autoTagQueue, contentKey, "", "", now); err != nil {
			return err
		}
		if err := closeQueueActivity(ctx, tx, c, activityID, completed, "", "", now); err != nil {
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
	if err := endContentJob(ctx, tx, autoTagQueue, contentKey, string(domain.JobDone), "", now); err != nil {
		return err
	}
	if err := closeQueueActivity(ctx, tx, c, activityID, completed, decider, "", now); err != nil {
		return err
	}
	if activityID != 0 {
		if err := recordAutoTagActivityTags(ctx, tx, activityID, userKey, written, selection.Probabilities); err != nil {
			return err
		}
	}
	// Bundle tags belong to every member; notify screens showing any member.
	affected, err := videoIDsSharingUserKey(ctx, tx, videoID)
	if err != nil {
		return err
	}
	if err := s.db.commitWith(ctx, tx, c, domain.AutoTagApplied{VideoIDs: affected}); err != nil {
		return fmt.Errorf("cannot finish the auto-tagging job: %w", err)
	}
	return nil
}

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

// ConfirmTentative returns domain.ErrNotFound unless the attachment is tentative.
func (s *AutoTagStore) ConfirmTentative(ctx context.Context, videoID, tagID int64) error {
	return s.settleTentative(ctx, videoID, tagID, false)
}

// RejectTentative removes the suggestion and prevents automatic reattachment; missing suggestions return domain.ErrNotFound.
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
	if err := s.db.commitWith(ctx, tx, c, domain.AutoTagApplied{VideoIDs: affected}); err != nil {
		return fmt.Errorf("cannot settle the tentative tag (tag=%d): %w", tagID, err)
	}
	return nil
}

func (s *AutoTagStore) Counts(ctx context.Context) (domain.AutoTagCounts, error) {
	return s.db.contentCounts(ctx, autoTagQueue)
}

// Only videos with a registered location qualify; otherwise return domain.ErrNotFound.
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

func videoIDsSharingUserKey(ctx context.Context, tx *sql.Tx, videoID int64) ([]int64, error) {
	return videoIDsWithUserKey(ctx, tx, `(select `+userKeyExpr("t")+` from videos t where t.id = ?)`, videoID)
}

func videoIDsForUserKey(ctx context.Context, tx *sql.Tx, userKey string) ([]int64, error) {
	return videoIDsWithUserKey(ctx, tx, `?`, userKey)
}

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
