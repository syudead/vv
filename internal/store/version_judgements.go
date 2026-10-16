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

// 「同じ動画かもしれない」候補の自動の判定（specs/054-auto-dup/data-model.md、research.md R-4〜R-8）。
// 判定は video_version_candidates の列に置く。結果は所有者の操作と同じ行を書く: 「同じ」は
// VersionStore.Bundle と同じ planBundle・applyBundle で束ね、「違う」は VersionStore.Dismiss と同じ
// dismissPair で記録する。結果を反映した取引の確定後に domain.VersionCandidatesChanged を発行する。

// ClaimJudgement は取り出せる候補のうち最も古い 1 組を判定中にし、両方の指紋と共に返す
// （data-model.md「Rules」）。取り出せるのは、両方の鍵に登録の所在を持つ動画があって一覧に出る組の
// うち、集まりから外した組（video_version_unbundles）と判定を保留した組（video_version_judging_holds）で
// なく、判定が null か、failed で retry_at を過ぎたもの。判定中にするのは 1 文で行い、同じ取引で組の
// version_judgement の記録を running/processing にする（無ければ開く）。無ければ domain.ErrNoJob。設定の入と
// 切は呼び出し側が見る。
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
	// 組の記録を判定中にする。中断や失敗で attention の記録も、同じ記録が running に戻る（research.md R-4）。
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

// judgeablePairs は判定できる候補の組 c を選ぶ from と where である。一覧に出る組
// （candidatePairsCondition）のうち、集まりから外した組（video_version_unbundles）と、所有者が自動の「違う」を
// 取り消して判定を保留した組（video_version_judging_holds、specs/055-activity/research.md R-7）でないもの。
// 取り出しと次の再試行の時刻と、記録を開く組が同じ組を見るように共有する。
var judgeablePairs = candidatePairsFrom + candidatePairsCondition() + `
	and not exists (select 1 from video_version_unbundles u where u.key_a = c.key_a and u.key_b = c.key_b)
	and not exists (select 1 from video_version_judging_holds h where h.key_a = c.key_a and h.key_b = c.key_b)`

// storedContentFingerprint は内容 key の保存した指紋を読む。無ければ found は偽。
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

// RecoverRunningJudgements は判定中のまま残った行（前回の終了で途切れたもの、判定のあいだに切られたもの）を
// 待ちに戻し、件数を返す。attempts は保つ。起動時に、判定を始める前に呼ぶ（auto_tag_queue と同じ）。
// running/processing の記録は待ちに戻す。起動時の回復で attention/interrupted にした記録はそのまま残り、次の
// 取り出しで running に戻る（specs/055-activity/data-model.md「Rules」）。
func (s *VersionStore) RecoverRunningJudgements(ctx context.Context) (int, error) {
	return s.requeueJudgements(ctx, domain.VersionJudgementRunning, "cannot recover interrupted version judgements")
}

// requeueJudgements は判定が from の行を待ち（null）に戻し、その行の記録のうち状態が from に当たるものを待ちに
// 戻して、行の数を返す。
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

// scanRequeuedActivityIDs は待ちに戻した行の activity_id（無ければ 0）を読み切り、0 でない id と行の数を返す。
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

// ApplyJudgement は判定中の組 keys（key_a < key_b）に結果を反映し、反映したかを返す。組が消えたか
// 判定中でなくなっていれば結果を捨てる。1 つの取引で行い、確定後に domain.VersionCandidatesChanged
// （束ねたときは束ねる操作の知らせも）を発行する。
//
//   - VersionSame: 両側（集まりに属せばその全メンバー）を読み直し、側をまたぐ「違う」と分かっている組が
//     あれば undecided にする（R-6）。無ければ R-7 の代表で束ね、候補は束ねる手順が消す。どちらかの鍵の
//     動画がいまライブラリに無ければ待ちに戻す。
//   - VersionDifferent: 「違う動画」を記録して候補を消す。
//   - VersionUndecided: undecided にして Probability を残す。
//   - VersionFailed: attempts を 1 増やし、retry_at と Error を残す（R-5）。
//   - VersionNoConnection: no_connection にする（R-13）。接続の保存で待ちに戻る。
func (s *VersionStore) ApplyJudgement(ctx context.Context, keys [2]string, result domain.VersionJudgeResult) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot apply the version judgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var attempts int
	var activityID sql.NullInt64
	err = tx.QueryRowContext(ctx, `select attempts, activity_id from video_version_candidates
		where key_a = ? and key_b = ? and judgement = 'running'`, keys[0], keys[1]).Scan(&attempts, &activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot read the version candidate: %w", err)
	}
	c := &changes{}
	now := time.Now()
	// 束ねると候補の行が消えるので、反映する前に組の記録を引く。
	entryID, err := pairActivityFor(ctx, tx, c, keys, activityID.Int64,
		domain.VersionJudgementActivityState(domain.VersionJudgementRunning), now)
	if err != nil {
		return false, err
	}
	applied, err := applyVerdict(ctx, tx, keys, attempts, result)
	if err != nil {
		return false, err
	}
	if err := recordJudgementActivity(ctx, tx, c, entryID, keys, result, applied, now); err != nil {
		return false, err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return false, fmt.Errorf("cannot apply the version judgement: %w", err)
	}
	s.db.publishEvents(append(applied.events, domain.VersionCandidatesChanged{})...)
	return true, nil
}

// appliedVerdict は applyVerdict が書いた結果である。bundled は「同じ」で束ねたことで、members は合わさった
// まとまりの全メンバーの鍵、undoKeys は加わった側の鍵と元の代表、events は確定後に発行する束ねる操作の知らせ。
type appliedVerdict struct {
	events   []domain.Event
	bundled  bool
	members  []string
	undoKeys domain.VersionBundleUndoKeys
}

// applyVerdict は判定中の組 keys に result を反映する（ApplyJudgement の各場合）。attempts はこれまでの失敗の数。
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

// NextJudgementRetry は failed の組のうち、いちばん早く再び取り出せる時刻を返す（research.md R-5）。
// ClaimJudgement が取り出せる組（judgeablePairs）だけを数える。無ければ found は偽。
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

// ResetNoConnectionJudgements は接続が無くて待っていた組（no_connection）を待ち（null）に戻し、件数を
// 返す。Ollama の接続が保存されたときに呼ぶ（research.md R-13、data-model.md「Rules」）。その組の
// attention/no_connection の記録も待ちに戻り、判定が終わると同じ記録が結果になる（specs/055-activity）。
func (s *VersionStore) ResetNoConnectionJudgements(ctx context.Context) (int, error) {
	return s.requeueJudgements(ctx, domain.VersionJudgementNoConnection,
		"cannot return the version judgements waiting for a connection")
}

// setJudgement は組の判定を state にし、probability を残す。retry_at と judgement_error は消す。
// state が空なら待ち（null）に戻す。
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

// failJudgement は組を attempts 回目の失敗にし、再び取り出せる時刻とメッセージを残す。
func failJudgement(ctx context.Context, tx *sql.Tx, keys [2]string, attempts int, message string) error {
	retryAt := domain.VersionJudgeRetryAt(attempts, time.Now()).Unix()
	if _, err := tx.ExecContext(ctx, `update video_version_candidates
		   set judgement = 'failed', attempts = ?, retry_at = ?, judgement_error = ?, probability = null
		 where key_a = ? and key_b = ?`, attempts, retryAt, message, keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot record the version judgement failure: %w", err)
	}
	return nil
}

// judgedSide は「同じ」の組の片側である。videoID はその鍵の動画、members はその側の全メンバーの鍵
// （集まりに属さなければ鍵だけ）、representativeKey はその側の代表の鍵（集まりの代表、属さなければ鍵）、
// side は代表を選ぶのに使う値。
type judgedSide struct {
	videoID           int64
	members           []string
	representativeKey string
	side              domain.VersionSide
}

// bundleSamePair は「同じ」の組の両側を読み直し、衝突しなければ R-7 の代表で束ねて、束ねる操作の
// 知らせと、取り消しのための加わった側（代表にならなかった側）の鍵と元の代表を返す。衝突すれば undecided に
// し、どちらかの鍵の動画がライブラリに無ければ待ちに戻す。
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

// readJudgedSide は鍵 key の側を読む。集まりに属せば、その全メンバーと、所有者から見た実効の代表を
// 代表の候補にする。鍵の動画がいまライブラリに無ければ domain.ErrNotFound。
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

// ownerDataCondition は、利用者データの鍵 ?1 と内容の鍵 ?2 に所有者が設定した値（確定したタグ・
// 再生位置・公開の設定・作品情報は利用者データの鍵、表示名・適用したサムネイル画像は内容の鍵）が
// あるかの式である（research.md R-7 の 1 番目の規則）。
var ownerDataCondition = buildOwnerDataCondition()

// buildOwnerDataCondition は ownerDataCondition を作る。作品情報の表は userDataTables から取る。
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

// hasOwnerData は利用者データの鍵 userKey と内容の鍵 contentKey に所有者の値があるかを返す。
func hasOwnerData(ctx context.Context, tx *sql.Tx, userKey, contentKey string) (bool, error) {
	var found bool
	if err := tx.QueryRowContext(ctx, `select `+ownerDataCondition, userKey, contentKey).Scan(&found); err != nil {
		return false, fmt.Errorf("cannot read the owner's data: %w", err)
	}
	return found, nil
}

// differingPairs は keys の 2 つの鍵の組のうち、「違う動画」と記録したか集まりから外した組を返す。
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

// appendKeyPairs は rows の (key_a, key_b) を pairs に足して閉じる。
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

// recordUnbundledPairs は集まりから外す鍵 key と、残る各メンバーの組を video_version_unbundles に書く
// （research.md R-8）。既にある組はそのまま。
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

// rebuildCandidatesOf は内容 key の保存した指紋で、その候補を作り直す（research.md R-8）。指紋が無ければ
// 何もしない。作り直した候補は組の記録に付き直す（rebuildVersionCandidates）。
func rebuildCandidatesOf(ctx context.Context, tx *sql.Tx, c *changes, key string) error {
	fingerprint, found, err := storedContentFingerprint(ctx, tx, key)
	if err != nil || !found {
		return err
	}
	return rebuildVersionCandidates(ctx, tx, c, key, fingerprint)
}

// versionJudgingEnabledKey は重複の自動判定の入と切のキーである。値は "true" か "false" で、行が
// 無ければ切（specs/054-auto-dup/data-model.md「Settings」）。
const versionJudgingEnabledKey = "version_judging.enabled"

// VersionJudgingEnabled は重複の自動判定が入かを返す。行が無いか "true" でなければ切である。
func (s *SettingsStore) VersionJudgingEnabled(ctx context.Context) (bool, error) {
	return versionJudgingEnabled(ctx, s.db.sql)
}

// versionJudgingEnabled は q（接続か取引）から重複の自動判定が入かを読む。
func versionJudgingEnabled(ctx context.Context, q rowQueryer) (bool, error) {
	value, found, err := readSetting(ctx, q, versionJudgingEnabledKey)
	if err != nil {
		return false, err
	}
	return found && value == strconv.FormatBool(true), nil
}

// SaveVersionJudgingEnabled は重複の自動判定の入と切を保存する。入れたときは、同じ取引で待っている判定できる
// 候補に組ごとの version_judgement の記録を開く（specs/055-activity/data-model.md「Kinds」）。切っても開いて
// いる記録は待ちのまま残り、入れ直すと再び取り出される。
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
		if err := syncPairActivities(ctx, tx, c, nil, time.Now()); err != nil {
			return err
		}
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot save setting %s: %w", versionJudgingEnabledKey, err)
	}
	return nil
}
