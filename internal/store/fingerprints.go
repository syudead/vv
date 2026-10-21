package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"time"

	"modernc.org/sqlite"

	"github.com/syudead/vv/internal/domain"
)

// Fingerprints and candidate pairs are rebuildable indexes removed when their content loses its last reference.

func fingerprintMissingCondition(alias string) string {
	return alias + `.seek_thumbnail_state = '` + string(domain.SeekThumbnailDone) + `' and ` +
		alias + `.content_key <> '' and not exists (
		select 1 from video_fingerprints f where f.content_key = ` + alias + `.content_key and f.version = ` +
		strconv.Itoa(domain.FingerprintVersion) + `)`
}

// ApplyFingerprintForJob writes only if the claimed content, location and generation still match, rebuilding candidates in the same transaction.
func (s *IngestStore) ApplyFingerprintForJob(
	ctx context.Context, job domain.Job, fingerprint domain.Fingerprint,
) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot start saving the fingerprint (id=%d): %w", job.VideoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil {
		return false, fmt.Errorf("cannot check the fingerprint job location (id=%d): %w", job.VideoID, err)
	}
	if !current {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `insert into video_fingerprints (content_key, version, interval_ms, hashes, updated_at)
		values (?, ?, ?, ?, ?)
		on conflict (content_key) do update
		   set version = excluded.version, interval_ms = excluded.interval_ms,
		       hashes = excluded.hashes, updated_at = excluded.updated_at`,
		job.ContentKey, fingerprint.Version, fingerprint.IntervalMs, fingerprint.Encode(), time.Now().Unix(),
	); err != nil {
		return false, fmt.Errorf("cannot save the fingerprint (id=%d): %w", job.VideoID, err)
	}
	c := &changes{}
	finder := pairFinder{jobID: job.ID, key: job.ContentKey, force: job.Force}
	if err := rebuildVersionCandidates(ctx, tx, c, job.ContentKey, fingerprint, finder, false); err != nil {
		return false, err
	}
	if err := clearFailedIssue(ctx, tx, domain.JobFingerprint, job.VideoID); err != nil {
		return false, err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return false, fmt.Errorf("cannot commit the fingerprint (id=%d): %w", job.VideoID, err)
	}
	return true, nil
}

// Compare current-version fingerprints using hashes and intervals; invalid or incomparable values return -1.
const fingerprintDistanceFunction = "vv_fingerprint_distance"

func init() {
	err := sqlite.RegisterDeterministicScalarFunction(fingerprintDistanceFunction, 4,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			a, okA := decodeFingerprintArgs(args[0], args[1])
			b, okB := decodeFingerprintArgs(args[2], args[3])
			if !okA || !okB {
				return int64(-1), nil
			}
			distance, ok := domain.CompareFingerprints(a, b)
			if !ok {
				return int64(-1), nil
			}
			return int64(distance), nil
		})
	if err != nil {
		panic(fmt.Sprintf("cannot register %s: %v", fingerprintDistanceFunction, err))
	}
}

func decodeFingerprintArgs(hashes, interval driver.Value) (domain.Fingerprint, bool) {
	var data []byte
	switch value := hashes.(type) {
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		return domain.Fingerprint{}, false
	}
	intervalMs, ok := interval.(int64)
	if !ok {
		return domain.Fingerprint{}, false
	}
	fingerprint, err := domain.DecodeFingerprint(data, domain.FingerprintVersion, intervalMs)
	if err != nil {
		return domain.Fingerprint{}, false
	}
	return fingerprint, true
}

// Rebuild candidates using matching durations and fingerprints, excluding dismissed or bundled pairs; reconnect activities and close superseded pairs.
// keepListed keeps the key's rows listed for judging again whose pair is still bundled or dismissed, for a rebuild
// after a membership change rather than a new fingerprint.
func rebuildVersionCandidates(
	ctx context.Context, tx *sql.Tx, c *changes, key string, fingerprint domain.Fingerprint, finder pairFinder,
	keepListed bool,
) error {
	if err := computeVersionCandidates(ctx, tx, key, fingerprint, keepListed); err != nil {
		return err
	}
	if finder.force {
		if err := listRejudgeCandidates(ctx, tx, key); err != nil {
			return err
		}
	}
	now := time.Now()
	if err := syncPairActivities(ctx, tx, c, []string{key}, finder, now); err != nil {
		return err
	}
	return closeDroppedPairActivities(ctx, tx, c, key, now)
}

func computeVersionCandidates(
	ctx context.Context, tx *sql.Tx, key string, fingerprint domain.Fingerprint, keepListed bool,
) error {
	clear := `delete from video_version_candidates as c where (c.key_a = ?1 or c.key_b = ?1)`
	if keepListed {
		// A listed pair that is neither bundled nor dismissed any more is listed again as an ordinary candidate.
		clear += ` and (c.rejudge_of is null or (` + undecidedPairCondition + `))`
	}
	if _, err := tx.ExecContext(ctx, clear, key); err != nil {
		return fmt.Errorf("cannot clear the version candidates: %w", err)
	}
	if fingerprint.Version != domain.FingerprintVersion {
		return nil
	}
	var duration sql.NullInt64
	err := tx.QueryRowContext(ctx, `select duration_ms from videos where content_key = ?`, key).Scan(&duration)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the duration for the version candidates: %w", err)
	}
	if !duration.Valid || duration.Int64 <= 0 {
		return nil
	}
	// Match domain.DurationsMatch: allow the greater of one second or 0.5% of the longer duration.
	if _, err := tx.ExecContext(ctx, `insert or ignore into video_version_candidates (key_a, key_b, distance, created_at)
		select min(?1, c.content_key), max(?1, c.content_key), c.distance, ?2
		  from (select f.content_key,
		               `+fingerprintDistanceFunction+`(?3, ?4, f.hashes, f.interval_ms) as distance
		          from video_fingerprints f
		          join videos v on v.content_key = f.content_key and v.content_key <> ''
		         where f.content_key <> ?1 and f.version = ?5
		           and v.duration_ms > 0
		           and abs(v.duration_ms - ?6) <= max(1000, max(v.duration_ms, ?6) * 5 / 1000)
		           and not exists (select 1 from video_version_dismissals d
		                            where d.key_a = min(?1, f.content_key) and d.key_b = max(?1, f.content_key))
		           and not exists (select 1 from video_bundle_members ma
		                             join video_bundle_members mb on mb.bundle_id = ma.bundle_id
		                            where ma.content_key = ?1 and mb.content_key = f.content_key)) c
		 where c.distance between 0 and ?7`,
		key, time.Now().Unix(), fingerprint.Encode(), fingerprint.IntervalMs,
		domain.FingerprintVersion, duration.Int64, domain.FingerprintMatchMaxDistance,
	); err != nil {
		return fmt.Errorf("cannot compute the version candidates: %w", err)
	}
	return nil
}

// Prune after bundling or succession changes membership or dismissal keys; a re-judged pair is bundled or dismissed
// by definition, so it stays until its verdict is applied.
func pruneVersionCandidates(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `delete from video_version_candidates
		where rejudge_of is null
		  and (exists (select 1 from video_version_dismissals d
		                where d.key_a = video_version_candidates.key_a and d.key_b = video_version_candidates.key_b)
		    or exists (select 1 from video_bundle_members ma
		                 join video_bundle_members mb on mb.bundle_id = ma.bundle_id
		                where ma.content_key = video_version_candidates.key_a
		                  and mb.content_key = video_version_candidates.key_b))`); err != nil {
		return fmt.Errorf("cannot prune the version candidates: %w", err)
	}
	return nil
}

// The nonempty content-key predicate enables the partial index; without it SQLite scans both video tables.
const candidatePairsFrom = `from video_version_candidates c
	join videos va on va.content_key = c.key_a and va.content_key <> ''
	join videos vb on vb.content_key = c.key_b and vb.content_key <> ''
	where `

// Both keys need registered locations; dismissed pairs and pairs in the same bundle are excluded.
func candidatePairsCondition() string {
	return registeredPairCondition() + ` and ` + undecidedPairCondition
}

func registeredPairCondition() string {
	return registeredVideoCondition("va") + ` and ` + registeredVideoCondition("vb")
}

const undecidedPairCondition = `not exists (select 1 from video_version_dismissals d where d.key_a = c.key_a and d.key_b = c.key_b)
	and not exists (select 1 from video_bundle_members ma join video_bundle_members mb on mb.bundle_id = ma.bundle_id
	                 where ma.content_key = c.key_a and mb.content_key = c.key_b)`

// Candidates returns newest eligible pairs with owner-visible details and an uncapped total; each pair orders videos by ID.
func (s *VersionStore) Candidates(ctx context.Context) (domain.VersionCandidatePage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VersionCandidatePage{}, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	page := domain.VersionCandidatePage{Items: []domain.VersionCandidate{}}
	condition := candidatePairsCondition()
	if err := tx.QueryRowContext(ctx, `select count(*) `+candidatePairsFrom+condition).Scan(&page.Total); err != nil {
		return domain.VersionCandidatePage{}, fmt.Errorf("cannot count the version candidates: %w", err)
	}
	pairs, err := candidatePairIDs(ctx, tx, condition)
	if err != nil {
		return domain.VersionCandidatePage{}, err
	}

	videos := map[int64]domain.Video{}
	for _, p := range pairs {
		var item domain.VersionCandidate
		for i, id := range p.ids {
			video, ok := videos[id]
			if !ok {
				video, err = getVideo(ctx, tx, domain.AudienceOwner, id)
				if err != nil {
					return domain.VersionCandidatePage{}, err
				}
				videos[id] = video
			}
			item.Videos[i] = video
		}
		item.Distance = p.distance
		item.Judgement = p.judgement
		page.Items = append(page.Items, item)
	}
	if err := tx.Commit(); err != nil {
		return domain.VersionCandidatePage{}, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	return page, nil
}

type candidatePair struct {
	ids       [2]int64
	distance  int
	judgement domain.VersionCandidateJudgement
}

func candidatePairIDs(ctx context.Context, tx *sql.Tx, condition string) ([]candidatePair, error) {
	rows, err := tx.QueryContext(ctx, `select va.id, vb.id, c.distance, coalesce(c.judgement, ''),
		coalesce(c.activity_id, 0), c.probability, coalesce(c.judgement_error, ''), c.retry_at,
		coalesce((select e.evidence from activity_entries e where e.id = c.activity_id), ''),
		exists (select 1 from video_version_unbundles u where u.key_a = c.key_a and u.key_b = c.key_b)
		  or exists (select 1 from video_version_judging_holds h where h.key_a = c.key_a and h.key_b = c.key_b)
		`+candidatePairsFrom+condition+`
		order by c.created_at desc, c.key_a, c.key_b limit ?`, domain.MaxVersionCandidates)
	if err != nil {
		return nil, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pairs []candidatePair
	for rows.Next() {
		p, err := scanCandidatePair(rows)
		if err != nil {
			return nil, err
		}
		if p.ids[0] > p.ids[1] {
			p.ids[0], p.ids[1] = p.ids[1], p.ids[0]
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	return pairs, nil
}

func scanCandidatePair(rows *sql.Rows) (candidatePair, error) {
	var (
		p           candidatePair
		state       string
		probability sql.NullFloat64
		retryAt     sql.NullInt64
		evidence    string
		held        bool
	)
	if err := rows.Scan(&p.ids[0], &p.ids[1], &p.distance, &state, &p.judgement.ActivityID, &probability,
		&p.judgement.Error, &retryAt, &evidence, &held); err != nil {
		return candidatePair{}, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	p.judgement.State = domain.VersionJudgement(state)
	if held && state == "" {
		p.judgement.State = domain.VersionJudgementHeld
	}
	p.judgement.LeftVideoID = p.ids[0]
	if probability.Valid {
		value := probability.Float64
		p.judgement.Probability = &value
	}
	if retryAt.Valid && p.judgement.State == domain.VersionJudgementFailed {
		p.judgement.RetryAt = time.Unix(retryAt.Int64, 0)
	}
	var err error
	if p.judgement.Evidence, err = domain.DecodeVersionJudgeEvidence(evidence); err != nil {
		return candidatePair{}, err
	}
	return p, nil
}

// Dismiss persists the sorted pair even outside candidates; reject missing videos or identical content without changes.
func (s *VersionStore) Dismiss(ctx context.Context, videoIDs [2]int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot dismiss the version candidate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := sortedPairKeys(ctx, tx, videoIDs)
	if err != nil {
		return err
	}
	keyA, keyB := keys[0], keys[1]
	if err := dismissPair(ctx, tx, keyA, keyB); err != nil {
		return err
	}
	c := &changes{}
	now := time.Now()
	if err := closePairActivities(ctx, tx, c, []string{keyA, keyB}, domain.ActivityDecidedByOwner,
		domain.ActivityByOwner, 0, now); err != nil {
		return err
	}
	if err := recordOwnerDismissal(ctx, tx, c, [2]string{keyA, keyB}, now); err != nil {
		return err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot dismiss the version candidate: %w", err)
	}
	return nil
}

// sortedPairKeys returns the two videos' content keys in key order; identical content is ErrTooFewVersions.
func sortedPairKeys(ctx context.Context, tx *sql.Tx, videoIDs [2]int64) ([2]string, error) {
	var keys [2]string
	for i, id := range videoIDs {
		key, err := bundlableContentKey(ctx, tx, id)
		if err != nil {
			return keys, err
		}
		keys[i] = key
	}
	if keys[0] == keys[1] {
		return keys, domain.ErrTooFewVersions
	}
	if keys[0] > keys[1] {
		keys[0], keys[1] = keys[1], keys[0]
	}
	return keys, nil
}

// Owner and automatic dismissals share this path; keys must be sorted.
func dismissPair(ctx context.Context, tx *sql.Tx, keyA, keyB string) error {
	if _, err := tx.ExecContext(ctx, `insert or ignore into video_version_dismissals (key_a, key_b, created_at)
		values (?, ?, ?)`, keyA, keyB, time.Now().Unix()); err != nil {
		return fmt.Errorf("cannot record the dismissal: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `delete from video_version_candidates where key_a = ? and key_b = ?`,
		keyA, keyB); err != nil {
		return fmt.Errorf("cannot remove the version candidate: %w", err)
	}
	return nil
}
