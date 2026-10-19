package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// The list shows a pipeline with its members' summaries, and the detail of one entry adds its members, pipelines and
// stage rows; both read members through attachPipelineMembers.

// ActivityDetail returns an entry with its members, pipelines and stage rows, resolving videos for the owner, or
// domain.ErrNotFound.
func (s *ActivityStore) ActivityDetail(ctx context.Context, id int64) (domain.ActivityDetail, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ActivityDetail{}, fmt.Errorf("cannot read the activity entry (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	entry, err := readActivityEntry(ctx, tx, id)
	if err != nil {
		return domain.ActivityDetail{}, err
	}
	entries := []domain.ActivityEntry{entry}
	if err := attachActivityStages(ctx, tx, entries); err != nil {
		return domain.ActivityDetail{}, err
	}
	if err := attachPipelineMembers(ctx, tx, domain.AudienceOwner, entries, memberMatch{}); err != nil {
		return domain.ActivityDetail{}, err
	}
	pipelines, err := readActivityEntries(ctx, tx, domain.AudienceOwner, `select `+activityColumns+
		` from activity_pipeline_members pm join activity_entries e on e.id = pm.pipeline_id`+activityJoins+
		` where pm.entry_id = ? order by e.id desc`, id)
	if err != nil {
		return domain.ActivityDetail{}, err
	}
	return domain.ActivityDetail{ActivityEntry: entries[0], Pipelines: pipelines}, nil
}

// readActivityEntry reads one entry with its tags, progress and videos for the owner, or domain.ErrNotFound.
func readActivityEntry(ctx context.Context, q interface {
	rowQueryer
	rowsQueryer
}, id int64,
) (domain.ActivityEntry, error) {
	entry, err := scanOneActivityEntry(ctx, q, id)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	entries := []domain.ActivityEntry{entry}
	if err := attachActivityDetails(ctx, q, domain.AudienceOwner, entries); err != nil {
		return domain.ActivityEntry{}, err
	}
	return entries[0], nil
}

// scanOneActivityEntry reads one entry's row without its tags, progress or videos, or domain.ErrNotFound.
func scanOneActivityEntry(ctx context.Context, q rowQueryer, id int64) (domain.ActivityEntry, error) {
	entry, err := scanActivityEntry(q.QueryRowContext(ctx, `select `+activityColumns+activityFrom+` where e.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ActivityEntry{}, fmt.Errorf("activity entry %d: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot read the activity entry (id=%d): %w", id, err)
	}
	return entry, nil
}

func readActivityEntries(
	ctx context.Context, q rowsQueryer, audience domain.Audience, query string, args ...any,
) ([]domain.ActivityEntry, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the activity entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []domain.ActivityEntry
	for rows.Next() {
		entry, err := scanActivityEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the activity entries: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the activity entries: %w", err)
	}
	_ = rows.Close()
	return entries, attachActivityDetails(ctx, q, audience, entries)
}

// memberMatch marks the listed members that satisfy the list's filter: the per-entry filters of activityFilter
// without status, or status alone when it is the only filter. Its zero value leaves Matched nil.
type memberMatch struct {
	condition string
	args      []any
}

func activityMemberMatch(query domain.ActivityQuery) memberMatch {
	conditions, args := activityEntryFilter(query, "e", true)
	if len(conditions) == 0 && query.Status != "" {
		conditions, args = []string{`e.status = ?`}, []any{string(query.Status)}
	}
	if len(conditions) == 0 {
		return memberMatch{}
	}
	return memberMatch{condition: `(` + strings.Join(conditions, ` and `) + `)`, args: args}
}

type pipelineMemberRow struct {
	pipelineID   int64
	supersededBy int64
	matched      bool
}

// attachPipelineMembers reads the members of the pipelines among entries in join order, with their tags, progress,
// videos and stage rows, and marks the replaced ones.
func attachPipelineMembers(
	ctx context.Context, q rowsQueryer, audience domain.Audience, entries []domain.ActivityEntry, match memberMatch,
) error {
	index, encoded := activityEntryIDs(entries, func(entry domain.ActivityEntry) bool {
		return entry.Kind == domain.ActivityPipeline
	})
	if len(index) == 0 {
		return nil
	}
	matched := `0`
	if match.condition != "" {
		matched = match.condition
	}
	rows, err := q.QueryContext(ctx, `select pm.pipeline_id, coalesce(e.superseded_by, 0), `+matched+`, `+
		activityColumns+` from activity_pipeline_members pm join activity_entries e on e.id = pm.entry_id`+
		activityJoins+` where pm.pipeline_id in (select value from json_each(?))
		order by pm.pipeline_id, pm.joined_at, pm.entry_id`, append(slices.Clone(match.args), encoded)...)
	if err != nil {
		return fmt.Errorf("cannot read the pipeline members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var memberRows []pipelineMemberRow
	var members []domain.ActivityEntry
	for rows.Next() {
		var row pipelineMemberRow
		entry, err := scanActivityEntry(prefixedRow{rows, []any{&row.pipelineID, &row.supersededBy, &row.matched}})
		if err != nil {
			return fmt.Errorf("cannot read the pipeline members: %w", err)
		}
		memberRows, members = append(memberRows, row), append(members, entry)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the pipeline members: %w", err)
	}
	_ = rows.Close()
	if err := attachMemberDetails(ctx, q, audience, members); err != nil {
		return err
	}
	for pipelineID, i := range index {
		entries[i].Members = pipelineMembersOf(pipelineID, memberRows, members, match.condition != "")
	}
	return nil
}

// attachMemberDetails reads each member once, so a pair entry in two listed pipelines gets the same details.
func attachMemberDetails(ctx context.Context, q rowsQueryer, audience domain.Audience, members []domain.ActivityEntry) error {
	var unique []domain.ActivityEntry
	seen := map[int64]bool{}
	for _, member := range members {
		if !seen[member.ID] {
			seen[member.ID] = true
			unique = append(unique, member)
		}
	}
	if err := attachActivityDetails(ctx, q, audience, unique); err != nil {
		return err
	}
	if err := attachActivityStages(ctx, q, unique); err != nil {
		return err
	}
	byID := make(map[int64]domain.ActivityEntry, len(unique))
	for _, member := range unique {
		byID[member.ID] = member
	}
	for i := range members {
		members[i] = byID[members[i].ID]
	}
	return nil
}

func pipelineMembersOf(
	pipelineID int64, rows []pipelineMemberRow, entries []domain.ActivityEntry, marked bool,
) []domain.ActivityMember {
	var roll []domain.PipelineMember
	var out []domain.ActivityMember
	for i, row := range rows {
		if row.pipelineID != pipelineID {
			continue
		}
		member := domain.ActivityMember{ActivityEntry: entries[i]}
		if marked {
			member.Matched = &row.matched
		}
		roll = append(roll, domain.PipelineMember{ID: entries[i].ID, SupersededBy: row.supersededBy})
		out = append(out, member)
	}
	replaced := domain.ReplacedPipelineMembers(roll)
	for i := range out {
		out[i].Replaced = replaced[out[i].ID]
	}
	return out
}

// prefixedRow scans leading columns into prefix before handing the rest to the caller's destinations.
type prefixedRow struct {
	row    rowScanner
	prefix []any
}

func (r prefixedRow) Scan(dest ...any) error {
	return r.row.Scan(append(slices.Clone(r.prefix), dest...)...)
}

// attachActivityStages reads the stage rows of the import entries among entries, in JobKinds order.
func attachActivityStages(ctx context.Context, q rowsQueryer, entries []domain.ActivityEntry) error {
	index, encoded := activityEntryIDs(entries, func(entry domain.ActivityEntry) bool {
		return entry.Kind == domain.ActivityImport
	})
	if len(index) == 0 {
		return nil
	}
	rows, err := q.QueryContext(ctx, `select entry_id, stage, state, coalesce(detail, ''), updated_at
		from activity_entry_stages where entry_id in (select value from json_each(?))`, encoded)
	if err != nil {
		return fmt.Errorf("cannot read the import stages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var entryID, updatedAt int64
		var stage, state string
		var row domain.ActivityStage
		if err := rows.Scan(&entryID, &stage, &state, &row.Detail, &updatedAt); err != nil {
			return fmt.Errorf("cannot read the import stages: %w", err)
		}
		row.Stage, row.State, row.UpdatedAt = domain.JobKind(stage), domain.ActivityStageState(state), time.UnixMilli(updatedAt)
		entry := &entries[index[entryID]]
		entry.Stages = append(entry.Stages, row)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the import stages: %w", err)
	}
	for i := range entries {
		slices.SortFunc(entries[i].Stages, func(a, b domain.ActivityStage) int {
			return slices.Index(domain.JobKinds, a.Stage) - slices.Index(domain.JobKinds, b.Stage)
		})
	}
	return nil
}
