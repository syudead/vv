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
// うち、集まりから外した組（video_version_unbundles）でなく、判定が null か、failed で retry_at を
// 過ぎたもの。判定中にするのは 1 文で行う。無ければ domain.ErrNoJob。設定の入と切は呼び出し側が見る。
func (s *VersionStore) ClaimJudgement(ctx context.Context) (domain.VersionJudgeJob, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VersionJudgeJob{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var job domain.VersionJudgeJob
	err = tx.QueryRowContext(ctx, `update video_version_candidates
		   set judgement = 'running', retry_at = null, judgement_error = null, probability = null
		 where (key_a, key_b) = (select c.key_a, c.key_b `+candidatePairsFrom+candidatePairsCondition()+`
		         and (c.judgement is null or (c.judgement = 'failed' and c.retry_at <= ?))
		         and not exists (select 1 from video_version_unbundles u where u.key_a = c.key_a and u.key_b = c.key_b)
		       order by c.created_at, c.key_a, c.key_b limit 1)
		returning key_a, key_b, attempts`, time.Now().Unix()).Scan(&job.Keys[0], &job.Keys[1], &job.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VersionJudgeJob{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.VersionJudgeJob{}, fmt.Errorf("cannot claim a version candidate: %w", err)
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
	if err := tx.Commit(); err != nil {
		return domain.VersionJudgeJob{}, fmt.Errorf("cannot claim a version candidate: %w", err)
	}
	return job, nil
}

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

// RecoverRunningJudgements は判定中のまま残った行（前回の終了で途切れたもの）を待ちに戻し、件数を
// 返す。attempts は保つ。起動時に、判定を始める前に呼ぶ（auto_tag_queue と同じ）。
func (s *VersionStore) RecoverRunningJudgements(ctx context.Context) (int, error) {
	result, err := s.db.sql.ExecContext(ctx,
		`update video_version_candidates set judgement = null where judgement = 'running'`)
	if err != nil {
		return 0, fmt.Errorf("cannot recover interrupted version judgements: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot recover interrupted version judgements: %w", err)
	}
	return int(affected), nil
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
func (s *VersionStore) ApplyJudgement(ctx context.Context, keys [2]string, result domain.VersionJudgeResult) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot apply the version judgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var attempts int
	err = tx.QueryRowContext(ctx, `select attempts from video_version_candidates
		where key_a = ? and key_b = ? and judgement = 'running'`, keys[0], keys[1]).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot read the version candidate: %w", err)
	}
	var events []domain.Event
	switch result.Verdict {
	case domain.VersionSame:
		events, err = bundleSamePair(ctx, tx, keys)
	case domain.VersionDifferent:
		err = dismissPair(ctx, tx, keys[0], keys[1])
	case domain.VersionUndecided:
		err = setJudgement(ctx, tx, keys, domain.VersionJudgementUndecided, result.Probability)
	case domain.VersionFailed:
		err = failJudgement(ctx, tx, keys, attempts+1, result.Error)
	default:
		err = fmt.Errorf("unknown version verdict %d", result.Verdict)
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("cannot apply the version judgement: %w", err)
	}
	s.db.publishEvents(append(events, domain.VersionCandidatesChanged{})...)
	return true, nil
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
// （集まりに属さなければ鍵だけ）、side は代表を選ぶのに使う値。
type judgedSide struct {
	videoID int64
	members []string
	side    domain.VersionSide
}

// bundleSamePair は「同じ」の組の両側を読み直し、衝突しなければ R-7 の代表で束ねて、束ねる操作の
// 知らせを返す。衝突すれば undecided にし、どちらかの鍵の動画がライブラリに無ければ待ちに戻す。
func bundleSamePair(ctx context.Context, tx *sql.Tx, keys [2]string) ([]domain.Event, error) {
	var sides [2]judgedSide
	for i, key := range keys {
		side, err := readJudgedSide(ctx, tx, key)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, setJudgement(ctx, tx, keys, "", nil)
		}
		if err != nil {
			return nil, err
		}
		sides[i] = side
	}
	differing, err := differingPairs(ctx, tx, append(append([]string{}, sides[0].members...), sides[1].members...))
	if err != nil {
		return nil, err
	}
	if domain.VersionSidesConflict(sides[0].members, sides[1].members, differing) {
		return nil, setJudgement(ctx, tx, keys, domain.VersionJudgementUndecided, nil)
	}
	representativeID := sides[domain.ChooseVersionRepresentative([2]domain.VersionSide{sides[0].side, sides[1].side})].side.Video.ID
	plan, err := planBundle(ctx, tx, uniqueIDs([]int64{sides[0].videoID, sides[1].videoID, representativeID}), representativeID)
	if err != nil {
		return nil, err
	}
	outcome, err := applyBundle(ctx, tx, plan)
	if err != nil {
		return nil, err
	}
	return outcome.events(), nil
}

// readJudgedSide は鍵 key の側を読む。集まりに属せば、その全メンバーと、所有者から見た実効の代表を
// 代表の候補にする。鍵の動画がいまライブラリに無ければ domain.ErrNotFound。
func readJudgedSide(ctx context.Context, tx *sql.Tx, key string) (judgedSide, error) {
	videoID, err := videoIDForContent(ctx, tx, key)
	if err != nil {
		return judgedSide{}, err
	}
	side := judgedSide{videoID: videoID, members: []string{key}}
	representativeID, userKey := videoID, key
	bundle, bundled, err := bundleOf(ctx, tx, key)
	if err != nil {
		return judgedSide{}, err
	}
	if bundled {
		side.side.Bundled, userKey = true, bundle.userKey
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
// 何もしない。
func rebuildCandidatesOf(ctx context.Context, tx *sql.Tx, key string) error {
	fingerprint, found, err := storedContentFingerprint(ctx, tx, key)
	if err != nil || !found {
		return err
	}
	return rebuildVersionCandidates(ctx, tx, key, fingerprint)
}

// versionJudgingEnabledKey は重複の自動判定の入と切のキーである。値は "true" か "false" で、行が
// 無ければ切（specs/054-auto-dup/data-model.md「Settings」）。
const versionJudgingEnabledKey = "version_judging.enabled"

// VersionJudgingEnabled は重複の自動判定が入かを返す。行が無いか "true" でなければ切である。
func (s *SettingsStore) VersionJudgingEnabled(ctx context.Context) (bool, error) {
	value, found, err := readSetting(ctx, s.db.sql, versionJudgingEnabledKey)
	if err != nil {
		return false, err
	}
	return found && value == strconv.FormatBool(true), nil
}

// SaveVersionJudgingEnabled は重複の自動判定の入と切を保存する。
func (s *SettingsStore) SaveVersionJudgingEnabled(ctx context.Context, enabled bool) error {
	return writeSetting(ctx, s.db.sql, versionJudgingEnabledKey, strconv.FormatBool(enabled))
}
