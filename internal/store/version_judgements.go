package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Automatic judgements share owner bundle/dismiss helpers and publish candidate changes only after commit.

// ClaimJudgement atomically claims the oldest eligible pair and its activity; the caller checks whether judging is enabled.
func (s *VersionStore) ClaimJudgement(ctx context.Context) (domain.VersionJudgeJob, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VersionJudgeJob{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var job domain.VersionJudgeJob
	var activityID sql.NullInt64
	err = tx.QueryRowContext(ctx, `update video_version_candidates
		   set judgement = 'running', retry_at = null, judgement_error = null, probability = null
		 where (key_a, key_b) = (select c.key_a, c.key_b `+judgeablePairs+`
		         and (c.judgement is null or (c.judgement = 'failed' and c.retry_at <= ?))
		       order by c.created_at, c.key_a, c.key_b limit 1)
		returning key_a, key_b, attempts, activity_id`, time.Now().Unix()).
		Scan(&job.Keys[0], &job.Keys[1], &job.Attempts, &activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VersionJudgeJob{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.VersionJudgeJob{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	c := &changes{}
	now := time.Now()
	processing := domain.VersionJudgementActivityState(domain.VersionJudgementRunning)
	id, err := pairActivityFor(ctx, tx, c, job.Keys, activityID.Int64, processing, now)
	if err != nil {
		return domain.VersionJudgeJob{}, err
	}
	if err := updateActivity(ctx, tx, c, id, activityUpdate{state: processing}, now); err != nil {
		return domain.VersionJudgeJob{}, err
	}
	for i, key := range job.Keys {
		fingerprint, found, err := storedContentFingerprint(ctx, tx, key)
		if err != nil {
			return domain.VersionJudgeJob{}, err
		}
		if !found {
			return domain.VersionJudgeJob{}, fmt.Errorf("the version candidate has no fingerprint (key=%s)", key)
		}
		job.Fingerprints[i] = fingerprint
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return domain.VersionJudgeJob{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	return job, nil
}

// Share eligibility across claiming, retry scheduling and activity creation; exclude unbundled and held pairs.
// A re-judged pair is claimable although it is bundled or dismissed.
var judgeablePairs = candidatePairsFrom + registeredPairCondition() + `
	and (c.rejudge_of is not null or (` + undecidedPairCondition + `))
	and not exists (select 1 from video_version_unbundles u where u.key_a = c.key_a and u.key_b = c.key_b)
	and not exists (select 1 from video_version_judging_holds h where h.key_a = c.key_a and h.key_b = c.key_b)`

func storedContentFingerprint(ctx context.Context, tx *sql.Tx, key string) (domain.Fingerprint, bool, error) {
	var version int
	var intervalMs int64
	var hashes []byte
	err := tx.QueryRowContext(ctx, `select version, interval_ms, hashes from video_fingerprints where content_key = ?`,
		key).Scan(&version, &intervalMs, &hashes)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Fingerprint{}, false, nil
	}
	if err != nil {
		return domain.Fingerprint{}, false, fmt.Errorf("cannot read the fingerprint: %w", err)
	}
	fingerprint, err := domain.DecodeFingerprint(hashes, version, intervalMs)
	if err != nil {
		return domain.Fingerprint{}, false, fmt.Errorf("cannot read the fingerprint (key=%s): %w", key, err)
	}
	return fingerprint, true, nil
}

// RecoverRunningJudgements preserves attempts and startup-interrupted activities; the next claim returns the same activity to running.
func (s *VersionStore) RecoverRunningJudgements(ctx context.Context) (int, error) {
	return s.requeueJudgements(ctx, domain.VersionJudgementRunning, "cannot recover interrupted version judgements")
}

func (s *VersionStore) requeueJudgements(ctx context.Context, from domain.VersionJudgement, failure string) (int, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", failure, err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `update video_version_candidates set judgement = null where judgement = ?
		returning coalesce(activity_id, 0)`, string(from))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", failure, err)
	}
	ids, count, err := scanRequeuedActivityIDs(rows)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", failure, err)
	}
	c := &changes{}
	if err := returnPairActivitiesToWaiting(ctx, tx, c, ids, domain.VersionJudgementActivityState(from), time.Now()); err != nil {
		return 0, err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return 0, fmt.Errorf("%s: %w", failure, err)
	}
	return count, nil
}

func scanRequeuedActivityIDs(rows *sql.Rows) ([]int64, int, error) {
	defer func() { _ = rows.Close() }()
	var ids []int64
	count := 0
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		count++
		if id != 0 {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return ids, count, rows.Close()
}

// ApplyJudgement discards stale claims and rechecks both sides atomically; conflicting same verdicts become undecided, and failures retain retry state.
func (s *VersionStore) ApplyJudgement(ctx context.Context, keys [2]string, result domain.VersionJudgeResult) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot apply the version judgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	claimed := claimedCandidate{keys: keys}
	var activityID, rejudgeOf sql.NullInt64
	err = tx.QueryRowContext(ctx, `select attempts, activity_id, rejudge_of from video_version_candidates
		where key_a = ? and key_b = ? and judgement = 'running'`, keys[0], keys[1]).
		Scan(&claimed.attempts, &activityID, &rejudgeOf)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot read the version candidate: %w", err)
	}
	c := &changes{}
	now := time.Now()
	// Read the activity before bundling removes its candidate row.
	claimed.entryID, err = pairActivityFor(ctx, tx, c, keys, activityID.Int64,
		domain.VersionJudgementActivityState(domain.VersionJudgementRunning), now)
	if err != nil {
		return false, err
	}
	var events []domain.Event
	if rejudgeOf.Valid {
		events, err = applyRejudgement(ctx, tx, c, claimed, rejudgeOf.Int64, result, now)
	} else {
		events, err = applyFirstJudgement(ctx, tx, c, claimed, result, now)
	}
	if err != nil {
		return false, err
	}
	if err := s.db.commitWith(ctx, tx, c, append(events, domain.VersionCandidatesChanged{})...); err != nil {
		return false, fmt.Errorf("cannot apply the version judgement: %w", err)
	}
	return true, nil
}

// claimedCandidate is a running candidate and the activity entry that reports its judgement.
type claimedCandidate struct {
	keys     [2]string
	attempts int
	entryID  int64
}

func applyFirstJudgement(
	ctx context.Context, tx *sql.Tx, c *changes, claimed claimedCandidate, result domain.VersionJudgeResult, now time.Time,
) ([]domain.Event, error) {
	applied, err := applyVerdict(ctx, tx, claimed.keys, claimed.attempts, result)
	if err != nil {
		return nil, err
	}
	return applied.events, recordJudgementActivity(ctx, tx, c, claimed.entryID, claimed.keys, result, applied, now)
}

// Retain joined-side keys and the original representative for undo, plus committed bundle notifications.
type appliedVerdict struct {
	events   []domain.Event
	bundled  bool
	members  []string
	undoKeys domain.VersionBundleUndoKeys
}

func applyVerdict(
	ctx context.Context, tx *sql.Tx, keys [2]string, attempts int, result domain.VersionJudgeResult,
) (appliedVerdict, error) {
	switch result.Verdict {
	case domain.VersionSame:
		return bundleSamePair(ctx, tx, keys)
	case domain.VersionDifferent:
		return appliedVerdict{}, dismissPair(ctx, tx, keys[0], keys[1])
	case domain.VersionUndecided:
		return appliedVerdict{}, setJudgement(ctx, tx, keys, domain.VersionJudgementUndecided, result.Probability)
	case domain.VersionFailed:
		return appliedVerdict{}, failJudgement(ctx, tx, keys, attempts+1, result.Error)
	case domain.VersionNoConnection:
		return appliedVerdict{}, setJudgement(ctx, tx, keys, domain.VersionJudgementNoConnection, nil)
	default:
		return appliedVerdict{}, fmt.Errorf("unknown version verdict %d", result.Verdict)
	}
}

// NextJudgementRetry considers only pairs eligible under the same rules as ClaimJudgement.
func (s *VersionStore) NextJudgementRetry(ctx context.Context) (time.Time, bool, error) {
	var retryAt sql.NullInt64
	err := s.db.sql.QueryRowContext(ctx, `select min(c.retry_at) `+judgeablePairs+`
		and c.judgement = 'failed' and c.retry_at is not null`).Scan(&retryAt)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("cannot read the next version judgement retry: %w", err)
	}
	if !retryAt.Valid {
		return time.Time{}, false, nil
	}
	return time.Unix(retryAt.Int64, 0), true, nil
}

// ResetNoConnectionJudgements requeues candidates and their existing activities after saving a connection.
func (s *VersionStore) ResetNoConnectionJudgements(ctx context.Context) (int, error) {
	return s.requeueJudgements(ctx, domain.VersionJudgementNoConnection,
		"cannot return the version judgements waiting for a connection")
}

// Clear retry metadata on state change; an empty state means waiting.
func setJudgement(ctx context.Context, tx *sql.Tx, keys [2]string, state domain.VersionJudgement, probability *float64) error {
	var value any
	if state != "" {
		value = string(state)
	}
	if _, err := tx.ExecContext(ctx, `update video_version_candidates
		   set judgement = ?, probability = ?, retry_at = null, judgement_error = null
		 where key_a = ? and key_b = ?`, value, probability, keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot record the version judgement: %w", err)
	}
	return nil
}

func failJudgement(ctx context.Context, tx *sql.Tx, keys [2]string, attempts int, message string) error {
	retryAt := domain.VersionJudgeRetryAt(attempts, time.Now()).Unix()
	if _, err := tx.ExecContext(ctx, `update video_version_candidates
		   set judgement = 'failed', attempts = ?, retry_at = ?, judgement_error = ?, probability = null
		 where key_a = ? and key_b = ?`, attempts, retryAt, message, keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot record the version judgement failure: %w", err)
	}
	return nil
}

// A judged side includes all bundle members and the representative-selection facts.
type judgedSide struct {
	videoID           int64
	members           []string
	representativeKey string
	side              domain.VersionSide
}

// Recheck both sides before bundling; conflicts become undecided and missing library videos return the pair to waiting.
func bundleSamePair(ctx context.Context, tx *sql.Tx, keys [2]string) (appliedVerdict, error) {
	var sides [2]judgedSide
	for i, key := range keys {
		side, err := readJudgedSide(ctx, tx, key)
		if errors.Is(err, domain.ErrNotFound) {
			return appliedVerdict{}, setJudgement(ctx, tx, keys, "", nil)
		}
		if err != nil {
			return appliedVerdict{}, err
		}
		sides[i] = side
	}
	differing, err := differingPairs(ctx, tx, append(append([]string{}, sides[0].members...), sides[1].members...))
	if err != nil {
		return appliedVerdict{}, err
	}
	if domain.VersionSidesConflict(sides[0].members, sides[1].members, differing) {
		return appliedVerdict{}, setJudgement(ctx, tx, keys, domain.VersionJudgementUndecided, nil)
	}
	chosen := domain.ChooseVersionRepresentative([2]domain.VersionSide{sides[0].side, sides[1].side})
	representativeID := sides[chosen].side.Video.ID
	plan, err := planBundle(ctx, tx, uniqueIDs([]int64{sides[0].videoID, sides[1].videoID, representativeID}), representativeID)
	if err != nil {
		return appliedVerdict{}, err
	}
	outcome, err := applyBundle(ctx, tx, plan)
	if err != nil {
		return appliedVerdict{}, err
	}
	joined := sides[1-chosen]
	return appliedVerdict{
		events: outcome.events(), bundled: true, members: plan.members,
		undoKeys: domain.VersionBundleUndoKeys{Members: joined.members, Representative: joined.representativeKey},
	}, nil
}

// Expand bundle membership and use the owner's effective representative; missing library videos fail.
func readJudgedSide(ctx context.Context, tx *sql.Tx, key string) (judgedSide, error) {
	videoID, err := videoIDForContent(ctx, tx, key)
	if err != nil {
		return judgedSide{}, err
	}
	side := judgedSide{videoID: videoID, members: []string{key}, representativeKey: key}
	representativeID, userKey := videoID, key
	bundle, bundled, err := bundleOf(ctx, tx, key)
	if err != nil {
		return judgedSide{}, err
	}
	if bundled {
		side.side.Bundled, userKey, side.representativeKey = true, bundle.userKey, bundle.representativeKey
		if side.members, err = bundleMemberKeys(ctx, tx, bundle.id); err != nil {
			return judgedSide{}, err
		}
		refs, err := bundleVersionsRefs(ctx, tx, domain.AudienceOwner, []string{key})
		if err != nil {
			return judgedSide{}, err
		}
		if ref, ok := refs[key]; ok && ref.RepresentativeID != 0 {
			representativeID = ref.RepresentativeID
		}
	}
	if side.side.Video, err = getVideo(ctx, tx, domain.AudienceOwner, representativeID); err != nil {
		return judgedSide{}, err
	}
	if side.side.OwnerData, err = hasOwnerData(ctx, tx, userKey, side.side.Video.ContentKey); err != nil {
		return judgedSide{}, err
	}
	return side, nil
}

// Check shared user-data keys for tags/progress/visibility/work info, and content keys for display names/applied thumbnails.
var ownerDataCondition = buildOwnerDataCondition()

func buildOwnerDataCondition() string {
	conditions := []string{
		`exists (select 1 from video_tags where content_key = ?1 and tentative = 0)`,
		`exists (select 1 from playback_progress where content_key = ?1)`,
		`exists (select 1 from public_videos where content_key = ?1)`,
		`exists (select 1 from video_overrides where content_key = ?2 and display_name is not null)`,
		`exists (select 1 from thumbnail_images where content_key = ?2)`,
	}
	for _, t := range userDataTables {
		if strings.HasPrefix(t.table, "video_work_") {
			conditions = append(conditions, `exists (select 1 from `+t.table+` where content_key = ?1)`)
		}
	}
	return strings.Join(conditions, " or ")
}

func hasOwnerData(ctx context.Context, tx *sql.Tx, userKey, contentKey string) (bool, error) {
	var found bool
	if err := tx.QueryRowContext(ctx, `select `+ownerDataCondition, userKey, contentKey).Scan(&found); err != nil {
		return false, fmt.Errorf("cannot read the owner's data: %w", err)
	}
	return found, nil
}

func differingPairs(ctx context.Context, tx *sql.Tx, keys []string) ([][2]string, error) {
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	var pairs [][2]string
	for _, table := range []string{"video_version_dismissals", "video_version_unbundles"} {
		rows, err := tx.QueryContext(ctx, `select key_a, key_b from `+table+`
			where key_a in (select value from json_each(?1)) and key_b in (select value from json_each(?1))`, string(encoded))
		if err != nil {
			return nil, fmt.Errorf("cannot read the %s pairs: %w", table, err)
		}
		pairs, err = appendKeyPairs(rows, pairs)
		if err != nil {
			return nil, fmt.Errorf("cannot read the %s pairs: %w", table, err)
		}
	}
	return pairs, nil
}

func appendKeyPairs(rows *sql.Rows, pairs [][2]string) ([][2]string, error) {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var pair [2]string
		if err := rows.Scan(&pair[0], &pair[1]); err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pairs, rows.Close()
}

func recordUnbundledPairs(ctx context.Context, tx *sql.Tx, key string, members []string, now int64) error {
	for _, other := range members {
		if other == key {
			continue
		}
		if _, err := tx.ExecContext(ctx, `insert or ignore into video_version_unbundles (key_a, key_b, created_at)
			values (min(?1, ?2), max(?1, ?2), ?3)`, key, other, now); err != nil {
			return fmt.Errorf("cannot record the unbundled pair: %w", err)
		}
	}
	return nil
}

// Rebuild only when a fingerprint exists, reconnecting candidates to their pair activities.
func rebuildCandidatesOf(ctx context.Context, tx *sql.Tx, c *changes, key string) error {
	fingerprint, found, err := storedContentFingerprint(ctx, tx, key)
	if err != nil || !found {
		return err
	}
	return rebuildVersionCandidates(ctx, tx, c, key, fingerprint, pairFinder{}, true)
}

const versionJudgingEnabledKey = "version_judging.enabled"

// VersionJudgingEnabled defaults to false unless the stored value is exactly true.
func (s *SettingsStore) VersionJudgingEnabled(ctx context.Context) (bool, error) {
	return versionJudgingEnabled(ctx, s.db.sql)
}

func versionJudgingEnabled(ctx context.Context, q rowQueryer) (bool, error) {
	value, found, err := readSetting(ctx, q, versionJudgingEnabledKey)
	if err != nil {
		return false, err
	}
	return found && value == strconv.FormatBool(true), nil
}

// SaveVersionJudgingEnabled opens activities for eligible waiting candidates when enabled; disabling preserves them for later resumption.
func (s *SettingsStore) SaveVersionJudgingEnabled(ctx context.Context, enabled bool) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot save setting %s: %w", versionJudgingEnabledKey, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := writeSetting(ctx, tx, versionJudgingEnabledKey, strconv.FormatBool(enabled)); err != nil {
		return err
	}
	c := &changes{}
	if enabled {
		if err := syncPairActivities(ctx, tx, c, nil, pairFinder{}, time.Now()); err != nil {
			return err
		}
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot save setting %s: %w", versionJudgingEnabledKey, err)
	}
	return nil
}
