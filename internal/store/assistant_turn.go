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

// Every transaction that changes a turn publishes AssistantChanged after commit, so the panel re-reads it.

const assistantTurnColumns = `id, status, request, context, coalesce(previous_turn_id, 0), progress,
	coalesce(answer, ''), judged, directive, clarify, coalesce(proposal_id, 0), stop, unchecked, started_at, finished_at`

func scanAssistantTurn(row rowScanner) (domain.AssistantTurn, error) {
	var (
		t                         domain.AssistantTurn
		status, context, progress string
		directive, clarify, stop  sql.NullString
		unchecked                 sql.NullString
		startedAt                 int64
		finishedAt                sql.NullInt64
	)
	if err := row.Scan(&t.ID, &status, &t.Request, &context, &t.PreviousTurnID, &progress, &t.Answer, &t.Judged,
		&directive, &clarify, &t.ProposalID, &stop, &unchecked, &startedAt, &finishedAt); err != nil {
		return domain.AssistantTurn{}, err
	}
	t.Status = domain.AssistantTurnStatus(status)
	t.StartedAt, t.FinishedAt = time.UnixMilli(startedAt), millisTime(finishedAt)
	for _, part := range []struct {
		raw  sql.NullString
		into any
	}{
		{sql.NullString{String: context, Valid: true}, &t.Context},
		{sql.NullString{String: progress, Valid: true}, &t.Progress},
		{directive, &t.Directive},
		{clarify, &t.Clarify},
		{stop, &t.Stop},
		{unchecked, &t.Unchecked},
	} {
		if !part.raw.Valid {
			continue
		}
		if err := json.Unmarshal([]byte(part.raw.String), part.into); err != nil {
			return domain.AssistantTurn{}, fmt.Errorf("cannot read assistant turn %d: %w", t.ID, err)
		}
	}
	return t, nil
}

func assistantTurnWhere(ctx context.Context, q rowQueryer, where string, args ...any) (domain.AssistantTurn, error) {
	t, err := scanAssistantTurn(q.QueryRowContext(ctx,
		`select `+assistantTurnColumns+` from assistant_turns where `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AssistantTurn{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.AssistantTurn{}, fmt.Errorf("cannot read the assistant turn: %w", err)
	}
	return t, nil
}

// CreateTurn stores a running turn; while another one runs it returns that turn with domain.ErrAssistantBusy.
func (s *AssistantStore) CreateTurn(
	ctx context.Context, in domain.AssistantTurnInput, now time.Time,
) (domain.AssistantTurn, error) {
	encoded, err := json.Marshal(in.Context)
	if err != nil {
		return domain.AssistantTurn{}, fmt.Errorf("cannot encode the assistant turn context: %w", err)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.AssistantTurn{}, fmt.Errorf("cannot start the assistant turn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	running, err := assistantTurnWhere(ctx, tx, `status = ?`, string(domain.AssistantTurnRunning))
	if err == nil {
		return running, domain.ErrAssistantBusy
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.AssistantTurn{}, err
	}
	var previous any
	if in.PreviousTurnID > 0 {
		previous = in.PreviousTurnID
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `insert into assistant_turns (status, request, context, previous_turn_id,
		started_at) values (?, ?, ?, ?, ?) returning id`, string(domain.AssistantTurnRunning), in.Text,
		string(encoded), previous, now.UnixMilli()).Scan(&id); err != nil {
		return domain.AssistantTurn{}, fmt.Errorf("cannot start the assistant turn: %w", err)
	}
	t, err := assistantTurnWhere(ctx, tx, `id = ?`, id)
	if err != nil {
		return domain.AssistantTurn{}, err
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, domain.AssistantChanged{}); err != nil {
		return domain.AssistantTurn{}, fmt.Errorf("cannot start the assistant turn: %w", err)
	}
	return t, nil
}

// Turn reads one turn in any status; a missing or expired one is domain.ErrNotFound.
func (s *AssistantStore) Turn(ctx context.Context, id int64) (domain.AssistantTurn, error) {
	return assistantTurnWhere(ctx, s.db.read, `id = ?`, id)
}

// RunningTurn returns domain.ErrNoJob when no turn runs.
func (s *AssistantStore) RunningTurn(ctx context.Context) (domain.AssistantTurn, error) {
	t, err := assistantTurnWhere(ctx, s.db.read, `status = ?`, string(domain.AssistantTurnRunning))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.AssistantTurn{}, domain.ErrNoJob
	}
	return t, err
}

// ActiveTurns lists, newest first, the running turn and the turns finished at or after since.
func (s *AssistantStore) ActiveTurns(ctx context.Context, since time.Time) ([]domain.AssistantTurn, error) {
	rows, err := s.db.read.QueryContext(ctx, `select `+assistantTurnColumns+` from assistant_turns
		where status = ? or finished_at >= ? order by id desc`, string(domain.AssistantTurnRunning), since.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("cannot list the assistant turns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	turns := []domain.AssistantTurn{}
	for rows.Next() {
		t, err := scanAssistantTurn(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot list the assistant turns: %w", err)
		}
		turns = append(turns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot list the assistant turns: %w", err)
	}
	return turns, nil
}

// RecordTurnProgress replaces the progress items of a running turn.
func (s *AssistantStore) RecordTurnProgress(ctx context.Context, id int64, progress []domain.AssistantProgressItem) error {
	encoded, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("cannot encode the assistant turn progress: %w", err)
	}
	return s.updateRunningTurn(ctx, `update assistant_turns set progress = ? where id = ? and status = ?`,
		string(encoded), id, string(domain.AssistantTurnRunning))
}

// FinishTurn moves a running turn once to its finished status with its result and progress.
func (s *AssistantStore) FinishTurn(ctx context.Context, t domain.AssistantTurn, now time.Time) error {
	if t.Status == domain.AssistantTurnRunning {
		return errors.New("cannot finish an assistant turn as running")
	}
	values := make([]any, 0, 5)
	for _, part := range []any{t.Progress, t.Directive, t.Clarify, t.Stop, t.Unchecked} {
		encoded, err := encodeOptionalJSON(part)
		if err != nil {
			return fmt.Errorf("cannot encode the assistant turn: %w", err)
		}
		values = append(values, encoded)
	}
	var answer, proposal any
	if t.Answer != "" {
		answer = t.Answer
	}
	if t.ProposalID > 0 {
		proposal = t.ProposalID
	}
	if values[0] == nil {
		values[0] = "[]"
	}
	return s.updateRunningTurn(ctx, `update assistant_turns set status = ?, progress = ?, directive = ?,
		clarify = ?, stop = ?, unchecked = ?, answer = ?, judged = ?, proposal_id = ?, finished_at = ?
		where id = ? and status = ?`, string(t.Status), values[0], values[1], values[2], values[3], values[4], answer,
		t.Judged, proposal, now.UnixMilli(), t.ID, string(domain.AssistantTurnRunning))
}

// encodeOptionalJSON answers nil for a nil pointer or slice, so the column stays null.
func encodeOptionalJSON(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) == "null" {
		return nil, err
	}
	return string(encoded), nil
}

// updateRunningTurn runs one guarded update of a running turn and commits it.
func (s *AssistantStore) updateRunningTurn(ctx context.Context, statement string, args ...any) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot update the assistant turn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("cannot update the assistant turn: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot update the assistant turn: %w", err)
	}
	if affected != 1 {
		return domain.ErrAssistantTurnNotRunning
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, domain.AssistantChanged{}); err != nil {
		return fmt.Errorf("cannot update the assistant turn: %w", err)
	}
	return nil
}

// RecoverTurns marks the turns a restart stopped as interrupted.
func (s *AssistantStore) RecoverTurns(ctx context.Context, now time.Time) (int, error) {
	return s.changeTurns(ctx, "recover", `update assistant_turns set status = ?, finished_at = ? where status = ?`,
		string(domain.AssistantTurnInterrupted), now.UnixMilli(), string(domain.AssistantTurnRunning))
}

// ExpireTurns deletes the turns finished before the time.
func (s *AssistantStore) ExpireTurns(ctx context.Context, before time.Time) (int, error) {
	return s.changeTurns(ctx, "expire", `delete from assistant_turns where finished_at < ?`, before.UnixMilli())
}

func (s *AssistantStore) changeTurns(ctx context.Context, verb, statement string, args ...any) (int, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot %s the assistant turns: %w", verb, err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return 0, fmt.Errorf("cannot %s the assistant turns: %w", verb, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot %s the assistant turns: %w", verb, err)
	}
	var events []domain.Event
	if changed > 0 {
		events = append(events, domain.AssistantChanged{})
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, events...); err != nil {
		return 0, fmt.Errorf("cannot %s the assistant turns: %w", verb, err)
	}
	return int(changed), nil
}
