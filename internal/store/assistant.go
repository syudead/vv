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

// Every transaction that changes a proposal publishes AssistantChanged after commit, and the ones that move it
// past proposed also write its assistant_run entry.

const assistantProposalColumns = `id, status, request, coalesce(turn_id, 0), plan, steps, total, applied, failed,
	coalesce(error, ''), coalesce(activity_id, 0), created_at, approved_at, started_at, finished_at`

// assistantStepPlans is the plan column: the plans as the card showed them, in step order.
type assistantStepPlans struct {
	Steps []domain.AssistantPlan `json:"steps"`
}

func scanAssistantProposal(row rowScanner) (domain.AssistantProposal, error) {
	var (
		p                                 domain.AssistantProposal
		status, plan, steps, failure      string
		createdAt                         int64
		approvedAt, startedAt, finishedAt sql.NullInt64
	)
	if err := row.Scan(&p.ID, &status, &p.Request, &p.TurnID, &plan, &steps, &p.Counts.Total, &p.Counts.Applied,
		&p.Counts.Failed, &failure, &p.ActivityID, &createdAt, &approvedAt, &startedAt, &finishedAt); err != nil {
		return domain.AssistantProposal{}, err
	}
	p.Status = domain.AssistantProposalStatus(status)
	var err error
	if p.Steps, err = decodeAssistantSteps(plan, steps); err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot read the steps of proposal %d: %w", p.ID, err)
	}
	if failure != "" {
		p.Error = &domain.AssistantProposalError{}
		if err := json.Unmarshal([]byte(failure), p.Error); err != nil {
			return domain.AssistantProposal{}, fmt.Errorf("cannot read the error of proposal %d: %w", p.ID, err)
		}
	}
	p.CreatedAt = time.UnixMilli(createdAt)
	p.ApprovedAt, p.StartedAt, p.FinishedAt = millisTime(approvedAt), millisTime(startedAt), millisTime(finishedAt)
	return p, nil
}

// decodeAssistantSteps joins the plan column with the steps column; a step without a stored state is pending.
func decodeAssistantSteps(plan, states string) ([]domain.AssistantStep, error) {
	var plans assistantStepPlans
	if err := json.Unmarshal([]byte(plan), &plans); err != nil {
		return nil, err
	}
	var stored []domain.AssistantStepState
	if err := json.Unmarshal([]byte(states), &stored); err != nil {
		return nil, err
	}
	steps := domain.NewAssistantSteps(plans.Steps)
	for i := range min(len(steps), len(stored)) {
		steps[i].AssistantStepState = stored[i]
	}
	return steps, nil
}

// encodeAssistantStates encodes the steps column; a slice of plain structs always encodes.
func encodeAssistantStates(steps []domain.AssistantStep) string {
	states := make([]domain.AssistantStepState, len(steps))
	for i, step := range steps {
		states[i] = step.AssistantStepState
	}
	encoded, _ := json.Marshal(states)
	return string(encoded)
}

func millisTime(value sql.NullInt64) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return time.UnixMilli(value.Int64)
}

func assistantProposalByID(ctx context.Context, q rowQueryer, id int64) (domain.AssistantProposal, error) {
	p, err := scanAssistantProposal(q.QueryRowContext(ctx,
		`select `+assistantProposalColumns+` from assistant_proposals where id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AssistantProposal{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot read the assistant proposal (id=%d): %w", id, err)
	}
	return p, nil
}

// CreateProposal stores the plans as a proposed proposal's pending steps, in card order, with their total; turnID
// is 0 for a proposal without a turn.
func (s *AssistantStore) CreateProposal(
	ctx context.Context, turnID int64, request string, plans []domain.AssistantPlan, now time.Time,
) (domain.AssistantProposal, error) {
	return s.storeProposal(ctx, turnID, request, plans, now, nil)
}

// proposalStep changes a just stored proposal in the same transaction.
type proposalStep func(tx *sql.Tx, c *changes, p domain.AssistantProposal) (domain.AssistantProposal, error)

// storeProposal stores a proposed proposal, runs then on it when given, and commits both.
func (s *AssistantStore) storeProposal(
	ctx context.Context, turnID int64, request string, plans []domain.AssistantPlan, now time.Time, then proposalStep,
) (domain.AssistantProposal, error) {
	encoded, err := json.Marshal(assistantStepPlans{Steps: plans})
	if err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot encode the assistant plan: %w", err)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot store the assistant proposal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	p, err := insertAssistantProposal(ctx, tx, turnID, request, encoded, domain.NewAssistantSteps(plans), now)
	if err != nil {
		return domain.AssistantProposal{}, err
	}
	c := &changes{}
	if then != nil {
		if p, err = then(tx, c, p); err != nil {
			return domain.AssistantProposal{}, err
		}
	}
	if err := s.db.commitWith(ctx, tx, c, domain.AssistantChanged{}); err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot store the assistant proposal: %w", err)
	}
	return p, nil
}

func insertAssistantProposal(
	ctx context.Context, tx *sql.Tx, turnID int64, request string, plan []byte, steps []domain.AssistantStep,
	now time.Time,
) (domain.AssistantProposal, error) {
	var turn any
	if turnID != 0 {
		turn = turnID
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `insert into assistant_proposals (status, request, turn_id, plan, steps, total,
		created_at) values (?, ?, ?, ?, ?, ?, ?) returning id`, string(domain.AssistantProposed), request, turn,
		string(plan), encodeAssistantStates(steps), domain.AssistantStepCounts(steps).Total,
		now.UnixMilli()).Scan(&id); err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot store the assistant proposal: %w", err)
	}
	return assistantProposalByID(ctx, tx, id)
}

// Proposal reads one proposal in any status; a missing one is domain.ErrNotFound.
func (s *AssistantStore) Proposal(ctx context.Context, id int64) (domain.AssistantProposal, error) {
	return assistantProposalByID(ctx, s.db.read, id)
}

// ActiveProposals lists, newest first, the approved and running proposals and those finished at or after since.
func (s *AssistantStore) ActiveProposals(ctx context.Context, since time.Time) ([]domain.AssistantProposal, error) {
	proposals, err := queryAssistantProposals(ctx, s.db.read, `where status in (?, ?)
		or (status in (?, ?, ?, ?) and finished_at >= ?) order by id desc`,
		string(domain.AssistantApproved), string(domain.AssistantRunning), string(domain.AssistantDone),
		string(domain.AssistantPartial), string(domain.AssistantStale), string(domain.AssistantFailed),
		since.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("cannot list the assistant proposals: %w", err)
	}
	return proposals, nil
}

// queryAssistantProposals reads the proposals the where clause, with its order, selects.
func queryAssistantProposals(
	ctx context.Context, q queryExecer, where string, args ...any,
) ([]domain.AssistantProposal, error) {
	rows, err := q.QueryContext(ctx, `select `+assistantProposalColumns+` from assistant_proposals `+where, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	proposals := []domain.AssistantProposal{}
	for rows.Next() {
		p, err := scanAssistantProposal(rows)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, p)
	}
	return proposals, rows.Err()
}

// ApproveProposal moves a proposed proposal to approved with the kept steps, skips the others and opens its
// assistant_run entry. A missing, discarded or expired proposal is domain.ErrNotFound; any other status returns the
// proposal with domain.ErrAssistantProposalNotPending; a selection domain.AssistantProposal.Select refuses returns
// its error. The expiry is checked in the guarded UPDATE, so a proposal the worker has not deleted yet still cannot
// be approved after domain.AssistantProposalTTL.
func (s *AssistantStore) ApproveProposal(
	ctx context.Context, id int64, kept []int, now time.Time,
) (domain.AssistantProposal, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot approve the assistant proposal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	p, err := approveAssistantProposal(ctx, tx, c, id, kept, now)
	if err != nil {
		return p, err
	}
	if err := s.db.commitWith(ctx, tx, c, domain.AssistantChanged{}); err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot approve the assistant proposal: %w", err)
	}
	return p, nil
}

// approveAssistantProposal is ApproveProposal inside the caller's transaction.
func approveAssistantProposal(
	ctx context.Context, tx *sql.Tx, c *changes, id int64, kept []int, now time.Time,
) (domain.AssistantProposal, error) {
	p, err := assistantProposalByID(ctx, tx, id)
	if err != nil {
		return domain.AssistantProposal{}, err
	}
	switch {
	case p.Status == domain.AssistantDiscarded:
		return domain.AssistantProposal{}, domain.ErrNotFound
	case p.Status != domain.AssistantProposed:
		return p, domain.ErrAssistantProposalNotPending
	case p.CreatedAt.Before(now.Add(-domain.AssistantProposalTTL)):
		return domain.AssistantProposal{}, domain.ErrNotFound
	}
	steps, err := p.Select(kept)
	if err != nil {
		return domain.AssistantProposal{}, err
	}
	counts := domain.AssistantStepCounts(steps)
	moved, err := moveAssistantProposalIf(ctx, tx, id, domain.AssistantProposed, domain.AssistantApproved,
		`created_at >= ?`, []any{now.Add(-domain.AssistantProposalTTL).UnixMilli()},
		`approved_at = ?, steps = ?, total = ?`, now.UnixMilli(), encodeAssistantStates(steps), counts.Total)
	if err != nil {
		return domain.AssistantProposal{}, err
	}
	if !moved {
		// The proposal read as proposed in this transaction, so only the expiry guard failed.
		return domain.AssistantProposal{}, domain.ErrNotFound
	}
	if p, err = assistantProposalByID(ctx, tx, id); err != nil {
		return domain.AssistantProposal{}, err
	}
	entryID, err := recordActivity(ctx, tx, c, domain.ActivityEntry{
		Kind:    domain.ActivityAssistantRun,
		Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectProposal, Key: strconv.FormatInt(id, 10), Label: p.Request},
		Status:  domain.ActivityRunning, Reason: domain.ActivityWaiting, Decider: domain.ActivityByOwner,
		Detail: domain.NewAssistantRunDetail(p.Steps).Encode(),
	}, now)
	if err != nil {
		return domain.AssistantProposal{}, err
	}
	if _, err := tx.ExecContext(ctx, `update assistant_proposals set activity_id = ? where id = ?`, entryID, id); err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot link the assistant proposal to its activity: %w", err)
	}
	p.ActivityID = entryID
	return p, nil
}

// moveAssistantProposal is one UPDATE guarded by the previous status, as domain.AssistantProposalStatus.CanMove
// allows; moved is false when the proposal was not in from.
func moveAssistantProposal(
	ctx context.Context, tx *sql.Tx, id int64, from, to domain.AssistantProposalStatus, set string, args ...any,
) (bool, error) {
	return moveAssistantProposalIf(ctx, tx, id, from, to, "", nil, set, args...)
}

// moveAssistantProposalIf is moveAssistantProposal with an extra where condition; an empty guard adds none.
func moveAssistantProposalIf(
	ctx context.Context, tx *sql.Tx, id int64, from, to domain.AssistantProposalStatus,
	guard string, guardArgs []any, set string, args ...any,
) (bool, error) {
	if !from.CanMove(to) {
		return false, fmt.Errorf("an assistant proposal cannot move from %s to %s", from, to)
	}
	statement := `update assistant_proposals set status = ?, ` + set + ` where id = ? and status = ?`
	if guard != "" {
		statement += ` and ` + guard
	}
	values := append(append(append([]any{string(to)}, args...), id, string(from)), guardArgs...)
	result, err := tx.ExecContext(ctx, statement, values...)
	if err != nil {
		return false, fmt.Errorf("cannot move the assistant proposal %d to %s: %w", id, to, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot move the assistant proposal %d to %s: %w", id, to, err)
	}
	return affected == 1, nil
}

// ClaimProposal moves the oldest approved proposal to running; domain.ErrNoJob when none waits.
func (s *AssistantStore) ClaimProposal(ctx context.Context, now time.Time) (domain.AssistantProposal, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot claim an assistant proposal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	err = tx.QueryRowContext(ctx, `select id from assistant_proposals where status = ? order by id limit 1`,
		string(domain.AssistantApproved)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AssistantProposal{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot claim an assistant proposal: %w", err)
	}
	if _, err := moveAssistantProposal(ctx, tx, id, domain.AssistantApproved, domain.AssistantRunning,
		`started_at = ?`, now.UnixMilli()); err != nil {
		return domain.AssistantProposal{}, err
	}
	p, err := assistantProposalByID(ctx, tx, id)
	if err != nil {
		return domain.AssistantProposal{}, err
	}
	c := &changes{}
	if p.ActivityID != 0 {
		if err := setActivityState(ctx, tx, c, p.ActivityID,
			domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityProcessing}, now); err != nil {
			return domain.AssistantProposal{}, err
		}
	}
	if err := s.db.commitWith(ctx, tx, c, domain.AssistantChanged{}); err != nil {
		return domain.AssistantProposal{}, fmt.Errorf("cannot claim an assistant proposal: %w", err)
	}
	return p, nil
}

// RecordProgress writes a running proposal's steps, their sums and its entry's detail.
func (s *AssistantStore) RecordProgress(ctx context.Context, id int64, steps []domain.AssistantStep, now time.Time) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot record the assistant progress: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	counts := domain.AssistantStepCounts(steps)
	var entryID int64
	err = tx.QueryRowContext(ctx, `update assistant_proposals set steps = ?, total = ?, applied = ?, failed = ?
		where id = ? and status = ? returning coalesce(activity_id, 0)`, encodeAssistantStates(steps),
		counts.Total, counts.Applied, counts.Failed, id, string(domain.AssistantRunning)).Scan(&entryID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("the assistant proposal %d is not running", id)
	}
	if err != nil {
		return fmt.Errorf("cannot record the assistant progress: %w", err)
	}
	c := &changes{}
	if err := writeAssistantRunDetail(ctx, tx, c, entryID, steps, now); err != nil {
		return err
	}
	if err := s.db.commitWith(ctx, tx, c, domain.AssistantChanged{}); err != nil {
		return fmt.Errorf("cannot record the assistant progress: %w", err)
	}
	return nil
}

func writeAssistantRunDetail(
	ctx context.Context, tx *sql.Tx, c *changes, entryID int64, steps []domain.AssistantStep, now time.Time,
) error {
	if entryID == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `update activity_entries set detail = ?, updated_at = ? where id = ?`,
		domain.NewAssistantRunDetail(steps).Encode(), now.UnixMilli(), entryID); err != nil {
		return fmt.Errorf("cannot update the assistant run entry (id=%d): %w", entryID, err)
	}
	c.activityChanged()
	return nil
}

// FinishProposal ends a running proposal with its result and closes its entry as done with the matching outcome.
func (s *AssistantStore) FinishProposal(
	ctx context.Context, id int64, result domain.AssistantRunResult, now time.Time,
) error {
	failure, err := encodeAssistantError(result.Error)
	if err != nil {
		return err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot finish the assistant proposal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	moved, err := moveAssistantProposal(ctx, tx, id, domain.AssistantRunning, result.Status,
		`steps = ?, total = ?, applied = ?, failed = ?, error = ?, finished_at = ?`, encodeAssistantStates(result.Steps),
		result.Counts.Total, result.Counts.Applied, result.Counts.Failed, failure, now.UnixMilli())
	if err != nil {
		return err
	}
	if !moved {
		return fmt.Errorf("the assistant proposal %d is not running", id)
	}
	p, err := assistantProposalByID(ctx, tx, id)
	if err != nil {
		return err
	}
	c := &changes{}
	if err := writeAssistantRunDetail(ctx, tx, c, p.ActivityID, p.Steps, now); err != nil {
		return err
	}
	if p.ActivityID != 0 {
		if err := setActivityState(ctx, tx, c, p.ActivityID,
			domain.ActivityState{Status: domain.ActivityDone, Outcome: result.ActivityOutcome()}, now); err != nil {
			return err
		}
	}
	if err := s.db.commitWith(ctx, tx, c, domain.AssistantChanged{}); err != nil {
		return fmt.Errorf("cannot finish the assistant proposal: %w", err)
	}
	return nil
}

func encodeAssistantError(failure *domain.AssistantProposalError) (any, error) {
	if failure == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(failure)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the assistant error: %w", err)
	}
	return string(encoded), nil
}

// ExpireProposals deletes the proposed proposals created before the cutoff.
func (s *AssistantStore) ExpireProposals(ctx context.Context, before time.Time) (int, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot expire the assistant proposals: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `delete from assistant_proposals where status = ? and created_at < ?`,
		string(domain.AssistantProposed), before.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("cannot expire the assistant proposals: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot expire the assistant proposals: %w", err)
	}
	var events []domain.Event
	if deleted > 0 {
		events = append(events, domain.AssistantChanged{})
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, events...); err != nil {
		return 0, fmt.Errorf("cannot expire the assistant proposals: %w", err)
	}
	return int(deleted), nil
}

// RecoverProposals runs at startup: it deletes every proposed proposal, whose thread is gone, and ends every
// running one as failed with the interrupted error, its entry needing attention. Approved ones wait for the worker.
func (s *AssistantStore) RecoverProposals(ctx context.Context, now time.Time) (int, error) {
	failure, err := encodeAssistantError(&domain.AssistantProposalError{Code: domain.AssistantInterrupted})
	if err != nil {
		return 0, err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot recover the assistant proposals: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	deleted, err := tx.ExecContext(ctx, `delete from assistant_proposals where status = ?`,
		string(domain.AssistantProposed))
	if err != nil {
		return 0, fmt.Errorf("cannot recover the assistant proposals: %w", err)
	}
	removed, err := deleted.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot recover the assistant proposals: %w", err)
	}
	if err := interruptAssistantSteps(ctx, tx); err != nil {
		return 0, err
	}
	entries, err := collectActivityIDs(tx.QueryContext(ctx, `update assistant_proposals set status = ?, error = ?,
		finished_at = ? where status = ? returning coalesce(activity_id, 0)`,
		string(domain.AssistantFailed), failure, now.UnixMilli(), string(domain.AssistantRunning)))
	if err != nil {
		return 0, fmt.Errorf("cannot recover the assistant proposals: %w", err)
	}
	c := &changes{}
	for _, entryID := range entries {
		if entryID == 0 {
			continue
		}
		if err := setActivityState(ctx, tx, c, entryID,
			domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityInterrupted}, now); err != nil {
			return 0, err
		}
	}
	var events []domain.Event
	if removed > 0 || len(entries) > 0 {
		events = append(events, domain.AssistantChanged{})
	}
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return 0, fmt.Errorf("cannot recover the assistant proposals: %w", err)
	}
	return int(removed) + len(entries), nil
}

// interruptAssistantSteps fails the running step of every running proposal and marks the steps after it notRun.
func interruptAssistantSteps(ctx context.Context, tx *sql.Tx) error {
	running, err := queryAssistantProposals(ctx, tx, `where status = ?`, string(domain.AssistantRunning))
	if err != nil {
		return fmt.Errorf("cannot read the running assistant proposals: %w", err)
	}
	for _, p := range running {
		steps := encodeAssistantStates(domain.InterruptAssistantSteps(p.Steps))
		if _, err := tx.ExecContext(ctx, `update assistant_proposals set steps = ? where id = ?`, steps, p.ID); err != nil {
			return fmt.Errorf("cannot interrupt the assistant proposal %d: %w", p.ID, err)
		}
	}
	return nil
}

// assistantLabelNames reads each kind's current display name by id; a provisional person reads as empty.
var assistantLabelNames = map[domain.AssistantLabelKind]string{
	domain.AssistantLabelTag: `select tag_id, name from tag_names where canonical = 1
		and tag_id in (select value from json_each(?))`,
	domain.AssistantLabelTagKind: `select id, name from tag_kinds where id in (select value from json_each(?))`,
	domain.AssistantLabelPerson: `select p.id, coalesce(n.name, '') from people p
		left join people_names n on n.person_id = p.id and n.canonical = 1
		where p.id in (select value from json_each(?))`,
	domain.AssistantLabelOrganization: `select o.id, coalesce(n.name, '') from organizations o
		left join organization_names n on n.organization_id = o.id and n.canonical = 1
		where o.id in (select value from json_each(?))`,
	domain.AssistantLabelPlaylist:    `select id, name from playlists where id in (select value from json_each(?))`,
	domain.AssistantLabelGroup:       `select id, name from groups where id in (select value from json_each(?))`,
	domain.AssistantLabelSmartFolder: `select id, name from smart_folders where id in (select value from json_each(?))`,
	domain.AssistantLabelFolderRoot:  `select id, path from media_folders where id in (select value from json_each(?))`,
}

// CurrentLabels re-reads the named entities and the videos of a plan from one snapshot, for the stale check.
func (s *AssistantStore) CurrentLabels(ctx context.Context, plan domain.AssistantPlan) (domain.AssistantCurrent, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.AssistantCurrent{}, fmt.Errorf("cannot read the assistant targets: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	byKind := map[domain.AssistantLabelKind][]int64{}
	keys := append([]string{}, plan.Targets.VideoKeys...)
	for _, label := range plan.Labels() {
		if label.Kind == domain.AssistantLabelVideo {
			keys = append(keys, label.Key)
			continue
		}
		byKind[label.Kind] = append(byKind[label.Kind], label.ID)
	}
	current := domain.AssistantCurrent{Names: map[domain.AssistantLabelRef]string{}}
	for kind, ids := range byKind {
		statement, ok := assistantLabelNames[kind]
		if !ok {
			return domain.AssistantCurrent{}, fmt.Errorf("unknown assistant label kind %q", kind)
		}
		if err := readAssistantNames(ctx, tx, kind, statement, ids, current.Names); err != nil {
			return domain.AssistantCurrent{}, err
		}
	}
	resolve := assistantVideoIDs
	if plan.ByVersion() {
		resolve = assistantVersionIDs
	}
	if current.Videos, err = resolve(ctx, tx, keys); err != nil {
		return domain.AssistantCurrent{}, err
	}
	return current, nil
}

func readAssistantNames(
	ctx context.Context, tx *sql.Tx, kind domain.AssistantLabelKind, statement string, ids []int64,
	names map[domain.AssistantLabelRef]string,
) error {
	encoded, _ := json.Marshal(ids) // A slice of integers always encodes.
	rows, err := tx.QueryContext(ctx, statement, string(encoded))
	if err != nil {
		return fmt.Errorf("cannot read the current %s names: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return fmt.Errorf("cannot read the current %s names: %w", kind, err)
		}
		names[domain.AssistantLabelRef{Kind: kind, ID: id}] = name
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the current %s names: %w", kind, err)
	}
	return nil
}

// VideoIDs maps the user keys still in the library to one current video id each; a gone key is absent. Only the
// later batches of a batched operation use it, and no version operation is batched.
func (s *AssistantStore) VideoIDs(ctx context.Context, keys []string) (map[string]int64, error) {
	return assistantVideoIDs(ctx, s.db.read, keys)
}

// assistantVideoIDs takes the lowest library video id of each key; the screen operations apply user data to the
// whole bundle through any of its versions.
func assistantVideoIDs(ctx context.Context, q queryExecer, keys []string) (map[string]int64, error) {
	ids := map[string]int64{}
	if len(keys) == 0 {
		return ids, nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build user keys: %w", err)
	}
	rows, err := q.QueryContext(ctx, `with selected(user_key) as (select value from json_each(?1))
		select v.content_key, min(v.id) from videos v
		 where v.content_key in (select user_key from selected) and v.content_key <> ''
		   and not exists (select 1 from video_bundle_members m where m.content_key = v.content_key)
		   and `+libraryVideoCondition("v")+`
		 group by v.content_key
		union all
		select b.user_key, min(v.id) from video_bundles b
		  join video_bundle_members m on m.bundle_id = b.id
		  join videos v on v.content_key = m.content_key and v.content_key <> ''
		 where b.user_key in (select user_key from selected) and `+libraryVideoCondition("v")+`
		 group by b.user_key`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map user keys to videos: %w", err)
	}
	return scanAssistantVideoIDs(rows, ids)
}

// assistantVersionIDs maps each content key to its own library video, bundled or not; a bundle's user key names
// no single version and is absent, so a plan holding one ends stale.
func assistantVersionIDs(ctx context.Context, q queryExecer, keys []string) (map[string]int64, error) {
	ids := map[string]int64{}
	if len(keys) == 0 {
		return ids, nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	rows, err := q.QueryContext(ctx, `select v.content_key, min(v.id) from videos v
		 where v.content_key in (select value from json_each(?)) and v.content_key <> ''
		   and `+libraryVideoCondition("v")+`
		 group by v.content_key`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map content keys to videos: %w", err)
	}
	return scanAssistantVideoIDs(rows, ids)
}

func scanAssistantVideoIDs(rows *sql.Rows, ids map[string]int64) (map[string]int64, error) {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		var id int64
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("cannot map keys to videos: %w", err)
		}
		ids[key] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot map keys to videos: %w", err)
	}
	return ids, nil
}

// MergeImpact counts the distinct library videos a merge of the sources moves: tags as tagImpact's merge counts
// them, people and organizations by their credits.
func (s *AssistantStore) MergeImpact(ctx context.Context, target domain.AssistantTarget, sourceIDs []int64) (int, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, fmt.Errorf("cannot count the merge impact: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	switch target {
	case domain.AssistantTargetTag:
		impact, err := tagImpact(ctx, tx, domain.TagImpactMerge, sourceIDs)
		return impact.VideoCount, err
	case domain.AssistantTargetPerson:
		return creditImpact(ctx, tx, domain.CreditPerson, sourceIDs)
	case domain.AssistantTargetOrganization:
		return creditImpact(ctx, tx, domain.CreditOrganization, sourceIDs)
	default:
		return 0, fmt.Errorf("no merge impact for the target %q", target)
	}
}

func creditImpact(ctx context.Context, tx *sql.Tx, kind domain.CreditKind, ids []int64) (int, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return 0, err
	}
	encoded, _ := json.Marshal(uniqueTagIDs(ids)) // A slice of integers always encodes.
	var count int
	err = tx.QueryRowContext(ctx, t.q(`select count(distinct l.content_key) from videos v
		join {links} l on l.content_key = `+userKeyExpr("v")+`
		where v.content_key <> '' and `+libraryVideoCondition("v")+` and `+
		shownVideoCondition("v", domain.AudienceOwner)+` and l.{id} in (select value from json_each(?))`),
		string(encoded)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cannot count the credited videos: %w", err)
	}
	return count, nil
}
