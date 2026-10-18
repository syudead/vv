package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A pipeline entry groups one video's import run with the follow-up processes that run started. Every write of a
// member's state re-rolls its pipelines in the same transaction, so a pipeline's stored state agrees with its members.

// openImportPipeline writes a pipeline for a new import entry, with its subject and decider, and makes the entry its
// first member; scanID is the scan that queued the work, or zero.
func openImportPipeline(
	ctx context.Context, q activityWriter, c *changes, member domain.ActivityEntry, scanID int64, now time.Time,
) error {
	id, err := recordActivity(ctx, q, c, domain.ActivityEntry{
		Kind: domain.ActivityPipeline, Subject: member.Subject, Status: domain.ActivityRunning,
		Reason: domain.ActivityWaiting, Decider: member.Decider, ScanID: scanID,
	}, now)
	if err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `insert into activity_pipeline_members (pipeline_id, entry_id, joined_at)
		values (?, ?, ?)`, id, member.ID, now.UnixMilli()); err != nil {
		return fmt.Errorf("cannot open the import pipeline (entry=%d): %w", member.ID, err)
	}
	return rollUpPipeline(ctx, q, id, now.UnixMilli())
}

// joinPipelines makes entryID a member of every pipeline the entries in of belong to; an entry of none joins none.
func joinPipelines(ctx context.Context, q activityWriter, entryID int64, of []int64, now time.Time) error {
	encoded, err := json.Marshal(of)
	if err != nil {
		return fmt.Errorf("cannot build the continued entry ids: %w", err)
	}
	if _, err := q.ExecContext(ctx, `insert into activity_pipeline_members (pipeline_id, entry_id, joined_at)
		select distinct pipeline_id, ?, ? from activity_pipeline_members
		where entry_id in (select value from json_each(?))
		on conflict (pipeline_id, entry_id) do nothing`, entryID, now.UnixMilli(), string(encoded)); err != nil {
		return fmt.Errorf("cannot join the pipelines (entry=%d): %w", entryID, err)
	}
	return rollUpPipelines(ctx, q, []int64{entryID}, now.UnixMilli())
}

// joinJobPipelines adds a follow-up process's entry to every pipeline of the finishing job's import entry. A stage
// result's transaction calls it after writing the follow-up's entry; a job from before pipelines joins nothing.
func joinJobPipelines(ctx context.Context, q activityWriter, jobID, entryID int64, now time.Time) error {
	var importID sql.NullInt64
	err := q.QueryRowContext(ctx, `select activity_id from jobs where id = ?`, jobID).Scan(&importID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !importID.Valid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the job's import activity (job=%d): %w", jobID, err)
	}
	return joinPipelines(ctx, q, entryID, []int64{importID.Int64}, now)
}

// rollUpPipelines re-rolls every pipeline the entries belong to. A pipeline is never a member, so it stops here.
func rollUpPipelines(ctx context.Context, q activityWriter, entryIDs []int64, at int64) error {
	if len(entryIDs) == 0 {
		return nil
	}
	encoded, err := json.Marshal(entryIDs)
	if err != nil {
		return fmt.Errorf("cannot build the member ids: %w", err)
	}
	rows, err := q.QueryContext(ctx, `select distinct pipeline_id from activity_pipeline_members
		where entry_id in (select value from json_each(?)) order by pipeline_id`, string(encoded))
	if err != nil {
		return fmt.Errorf("cannot read the member's pipelines: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pipelines []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("cannot read the member's pipelines: %w", err)
		}
		pipelines = append(pipelines, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the member's pipelines: %w", err)
	}
	_ = rows.Close()
	for _, id := range pipelines {
		if err := rollUpPipeline(ctx, q, id, at); err != nil {
			return err
		}
	}
	return nil
}

func rollUpPipeline(ctx context.Context, q activityWriter, id int64, at int64) error {
	members, err := pipelineMembers(ctx, q, id)
	if err != nil {
		return err
	}
	var status, reason, outcome string
	if err := q.QueryRowContext(ctx, `select status, coalesce(reason, ''), coalesce(outcome, '')
		from activity_entries where id = ?`, id).Scan(&status, &reason, &outcome); err != nil {
		return fmt.Errorf("cannot read the pipeline (id=%d): %w", id, err)
	}
	current := domain.ActivityState{Status: domain.ActivityStatus(status), Reason: domain.ActivityReason(reason),
		Outcome: domain.ActivityOutcome(outcome)}
	if next := domain.PipelineState(members); next != current {
		return storeActivityState(ctx, q, id, next, at)
	}
	return nil
}

func pipelineMembers(ctx context.Context, q activityWriter, id int64) ([]domain.PipelineMember, error) {
	rows, err := q.QueryContext(ctx, `select e.id, e.kind, e.status, coalesce(e.reason, ''), coalesce(e.outcome, ''),
			coalesce(e.superseded_by, 0)
		from activity_pipeline_members m join activity_entries e on e.id = m.entry_id
		where m.pipeline_id = ? order by m.joined_at, m.entry_id`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the pipeline's members (id=%d): %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	var members []domain.PipelineMember
	for rows.Next() {
		var member domain.PipelineMember
		var kind, status, reason, outcome string
		if err := rows.Scan(&member.ID, &kind, &status, &reason, &outcome, &member.SupersededBy); err != nil {
			return nil, fmt.Errorf("cannot read the pipeline's members (id=%d): %w", id, err)
		}
		member.Kind = domain.ActivityKind(kind)
		member.State = domain.ActivityState{Status: domain.ActivityStatus(status), Reason: domain.ActivityReason(reason),
			Outcome: domain.ActivityOutcome(outcome)}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the pipeline's members (id=%d): %w", id, err)
	}
	return members, nil
}

// runningScanID returns the scan in progress, or zero; at most one scan runs at a time.
func runningScanID(ctx context.Context, q rowQueryer) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `select id from scans where state = 'running'`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("cannot read the running scan: %w", err)
	}
	return id, nil
}
