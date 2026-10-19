package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A forced fingerprint lists its key's automatically decided pairs again; the prior decision stays in force until the
// new verdict replaces it (docs/design-docs/duplicate-judging.md#judging-again-after-a-force-rescan).

// rejudgePrior is the automatic version_judgement entry that a re-judged candidate may replace.
type rejudgePrior struct {
	id       int64
	keys     [2]string
	outcome  domain.ActivityOutcome
	undoKeys string
	mark     int64
}

// The entry must be the pair's latest decision, automatic and not undone, and the owner must not have unbundled or
// held the pair; the caller appends a condition on ?9.
const rejudgePriorsQuery = `select e.id, e.subject_key, e.subject_key_b, e.outcome, coalesce(e.undo_keys, ''),
		coalesce(e.decided_mark, e.id)
	  from activity_entries e
	 where e.kind = ?1 and e.subject_kind = ?2 and e.status = ?3 and e.outcome in (?4, ?5)
	   and e.decider in (?6, ?7) and e.undone_by is null
	   and e.id = (select max(l.id) from activity_entries l
	                where l.kind = ?1 and l.subject_kind = ?2 and l.subject_key = e.subject_key
	                  and l.subject_key_b = e.subject_key_b and l.status = ?3 and l.outcome in (?4, ?5, ?8))
	   and not exists (select 1 from video_version_unbundles u
	                    where u.key_a = e.subject_key and u.key_b = e.subject_key_b)
	   and not exists (select 1 from video_version_judging_holds h
	                    where h.key_a = e.subject_key and h.key_b = e.subject_key_b)
	   and `

func queryRejudgePriors(ctx context.Context, tx *sql.Tx, condition string, arg any) ([]rejudgePrior, error) {
	rows, err := tx.QueryContext(ctx, rejudgePriorsQuery+condition+` order by e.id`,
		string(domain.ActivityVersionJudgement), string(domain.ActivitySubjectPair), string(domain.ActivityDone),
		string(domain.ActivitySame), string(domain.ActivityDifferent), string(domain.ActivityByRule),
		string(domain.ActivityByAI), string(domain.ActivityDecidedByOwner), arg)
	if err != nil {
		return nil, fmt.Errorf("cannot read the automatic version judgements: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var priors []rejudgePrior
	for rows.Next() {
		var p rejudgePrior
		var outcome string
		if err := rows.Scan(&p.id, &p.keys[0], &p.keys[1], &outcome, &p.undoKeys, &p.mark); err != nil {
			return nil, fmt.Errorf("cannot read the automatic version judgements: %w", err)
		}
		p.outcome = domain.ActivityOutcome(outcome)
		priors = append(priors, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the automatic version judgements: %w", err)
	}
	return priors, rows.Close()
}

// held reports whether the decision is still in force under the undo rules; a bundle also returns the side its
// undo would split off, which must hold exactly one key of the pair.
func (p rejudgePrior) held(ctx context.Context, tx *sql.Tx) (judgedHold, bool, error) {
	if p.outcome == domain.ActivitySame {
		hold, err := judgedBundleHeld(ctx, tx, p.keys, p.undoKeys, p.mark)
		if err != nil {
			return judgedHold{}, false, err
		}
		joined := slices.Contains(hold.leaving, p.keys[0]) != slices.Contains(hold.leaving, p.keys[1])
		return hold, joined, nil
	}
	held, err := judgedDismissalHeld(ctx, tx, p.keys, p.mark)
	return judgedHold{}, held, err
}

func judgedDismissalHeld(ctx context.Context, tx *sql.Tx, keys [2]string, mark int64) (bool, error) {
	var dismissed, together bool
	if err := tx.QueryRowContext(ctx, `select
		exists (select 1 from video_version_dismissals where key_a = ?1 and key_b = ?2),
		exists (select 1 from video_bundle_members ma join video_bundle_members mb on mb.bundle_id = ma.bundle_id
		         where ma.content_key = ?1 and mb.content_key = ?2)`, keys[0], keys[1]).
		Scan(&dismissed, &together); err != nil {
		return false, fmt.Errorf("cannot read the dismissal: %w", err)
	}
	if !dismissed || together {
		return false, nil
	}
	redecided, err := ownerDismissedAfter(ctx, tx, keys, mark)
	return !redecided, err
}

// listRejudgeCandidates runs after computeVersionCandidates has cleared the key's rows; judging off lists nothing.
func listRejudgeCandidates(ctx context.Context, tx *sql.Tx, key string) error {
	enabled, err := versionJudgingEnabled(ctx, tx)
	if err != nil || !enabled {
		return err
	}
	priors, err := queryRejudgePriors(ctx, tx, `(e.subject_key = ?9 or e.subject_key_b = ?9)`, key)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	for _, prior := range priors {
		_, held, err := prior.held(ctx, tx)
		if err != nil {
			return err
		}
		if !held {
			continue
		}
		if _, err := tx.ExecContext(ctx, `insert or ignore into video_version_candidates
			(key_a, key_b, distance, created_at, rejudge_of)
			select fa.content_key, fb.content_key,
			       `+fingerprintDistanceFunction+`(fa.hashes, fa.interval_ms, fb.hashes, fb.interval_ms), ?3, ?4
			  from video_fingerprints fa join video_fingerprints fb on fb.content_key = ?2 and fb.version = ?5
			 where fa.content_key = ?1 and fa.version = ?5`,
			prior.keys[0], prior.keys[1], now, prior.id, domain.FingerprintVersion); err != nil {
			return fmt.Errorf("cannot list the pair for judging again: %w", err)
		}
	}
	return nil
}

// applyRejudgement applies a verdict over the prior automatic decision and removes the candidate. A prior the owner
// changed after listing leaves nothing to replace, so the entry closes as superseded.
func applyRejudgement(
	ctx context.Context, tx *sql.Tx, c *changes, claimed claimedCandidate, priorID int64,
	result domain.VersionJudgeResult, now time.Time,
) ([]domain.Event, error) {
	priors, err := queryRejudgePriors(ctx, tx, `e.id = ?9`, priorID)
	if err != nil {
		return nil, err
	}
	var hold judgedHold
	held := false
	if len(priors) == 1 {
		if hold, held, err = priors[0].held(ctx, tx); err != nil {
			return nil, err
		}
	}
	if !held {
		if err := removeVersionCandidate(ctx, tx, claimed.keys); err != nil {
			return nil, err
		}
		return nil, closeActivities(ctx, tx, c, `id = ?1`, domain.ActivitySuperseded, "", now, claimed.entryID)
	}
	events, err := applyVerdictOverPrior(ctx, tx, c, claimed, priors[0], hold, result, now)
	if err != nil {
		return nil, err
	}
	return events, removeVersionCandidate(ctx, tx, claimed.keys)
}

func applyVerdictOverPrior(
	ctx context.Context, tx *sql.Tx, c *changes, claimed claimedCandidate, prior rejudgePrior, hold judgedHold,
	result domain.VersionJudgeResult, now time.Time,
) ([]domain.Event, error) {
	bundled := prior.outcome == domain.ActivitySame
	switch {
	case result.Verdict == domain.VersionSame && bundled:
		return nil, confirmJudgedBundle(ctx, tx, c, claimed, prior, hold, result, now)
	case result.Verdict == domain.VersionDifferent && bundled:
		return splitJudgedBundle(ctx, tx, c, claimed, hold, result, now)
	case result.Verdict == domain.VersionSame:
		return bundleJudgedDismissal(ctx, tx, c, claimed, result, now)
	case result.Verdict == domain.VersionDifferent:
		return nil, recordJudgementActivity(ctx, tx, c, claimed.entryID, claimed.keys, result, appliedVerdict{}, now)
	default:
		// Undecided, failed and no connection record their reason on the row, which the entry reads.
		return applyFirstJudgement(ctx, tx, c, claimed, result, now)
	}
}

// The new entry takes over the prior's undo keys, so undoing it splits off the same side.
func confirmJudgedBundle(
	ctx context.Context, tx *sql.Tx, c *changes, claimed claimedCandidate, prior rejudgePrior, hold judgedHold,
	result domain.VersionJudgeResult, now time.Time,
) error {
	members, err := bundleMemberKeys(ctx, tx, hold.bundle.id)
	if err != nil {
		return err
	}
	undoKeys, err := domain.DecodeVersionBundleUndoKeys(prior.undoKeys)
	if err != nil {
		return err
	}
	applied := appliedVerdict{bundled: true, members: members, undoKeys: undoKeys}
	return recordJudgementActivity(ctx, tx, c, claimed.entryID, claimed.keys, result, applied, now)
}

// The split writes no video_version_unbundles row: the dismissal keeps the pair apart, and that row would keep the
// next force rescan from judging the pair.
func splitJudgedBundle(
	ctx context.Context, tx *sql.Tx, c *changes, claimed claimedCandidate, hold judgedHold,
	result domain.VersionJudgeResult, now time.Time,
) ([]domain.Event, error) {
	if err := dismissPair(ctx, tx, claimed.keys[0], claimed.keys[1]); err != nil {
		return nil, err
	}
	// Decide before the split's candidate rebuild closes open entries that lost their candidate.
	if err := recordJudgementActivity(ctx, tx, c, claimed.entryID, claimed.keys, result, appliedVerdict{}, now); err != nil {
		return nil, err
	}
	split, err := splitBundle(ctx, tx, c, hold.bundle, hold.leaving, hold.representative, false)
	if err != nil {
		return nil, err
	}
	return split.events(), nil
}

// A conflict or a missing video leaves the pair unbundled, so the dismissal returns as it was.
func bundleJudgedDismissal(
	ctx context.Context, tx *sql.Tx, c *changes, claimed claimedCandidate, result domain.VersionJudgeResult,
	now time.Time,
) ([]domain.Event, error) {
	keys := claimed.keys
	var createdAt int64
	if err := tx.QueryRowContext(ctx, `delete from video_version_dismissals where key_a = ? and key_b = ?
		returning created_at`, keys[0], keys[1]).Scan(&createdAt); err != nil {
		return nil, fmt.Errorf("cannot remove the dismissal: %w", err)
	}
	applied, err := bundleSamePair(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	if !applied.bundled {
		if _, err := tx.ExecContext(ctx, `insert into video_version_dismissals (key_a, key_b, created_at)
			values (?, ?, ?)`, keys[0], keys[1], createdAt); err != nil {
			return nil, fmt.Errorf("cannot restore the dismissal: %w", err)
		}
	}
	return applied.events, recordJudgementActivity(ctx, tx, c, claimed.entryID, keys, result, applied, now)
}

func removeVersionCandidate(ctx context.Context, tx *sql.Tx, keys [2]string) error {
	if _, err := tx.ExecContext(ctx, `delete from video_version_candidates where key_a = ? and key_b = ?`,
		keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot remove the version candidate: %w", err)
	}
	return nil
}
