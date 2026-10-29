package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// DiscardProposal moves a proposed proposal to discarded when a follow-up replaces it. A missing proposal is
// domain.ErrNotFound and any other status domain.ErrAssistantProposalNotPending.
func (s *AssistantStore) DiscardProposal(ctx context.Context, id int64, now time.Time) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot discard the assistant proposal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	moved, err := moveAssistantProposal(ctx, tx, id, domain.AssistantProposed, domain.AssistantDiscarded,
		`finished_at = ?`, now.UnixMilli())
	if err != nil {
		return err
	}
	if !moved {
		if _, err := assistantProposalByID(ctx, tx, id); err != nil {
			return err
		}
		return domain.ErrAssistantProposalNotPending
	}
	if err := s.db.commitWith(ctx, tx, &changes{}, domain.AssistantChanged{}); err != nil {
		return fmt.Errorf("cannot discard the assistant proposal: %w", err)
	}
	return nil
}

// assistantNameLimit stops a lookup past the candidates a clarification offers.
const assistantNameLimit = 11

// assistantNameMatches finds ids by the stored folded key: tag names and synonyms, person and organization names and
// aliases, and the natural sort key of group and smart folder names.
var assistantNameMatches = map[domain.AssistantLabelKind]string{
	domain.AssistantLabelTag: `select distinct tag_id from tag_names where search_key = ? order by tag_id limit ?`,
	domain.AssistantLabelPerson: `select distinct person_id from people_names where search_key = ?
		order by person_id limit ?`,
	domain.AssistantLabelOrganization: `select distinct organization_id from organization_names where search_key = ?
		order by organization_id limit ?`,
	domain.AssistantLabelGroup:       `select id from groups where title_key = ? order by id limit ?`,
	domain.AssistantLabelSmartFolder: `select id from smart_folders where title_key = ? order by id limit ?`,
}

// assistantSimilarNameMatches find ids whose stored key contains the folded name; instr needs no escaping.
var assistantSimilarNameMatches = map[domain.AssistantLabelKind]string{
	domain.AssistantLabelTag: `select distinct tag_id from tag_names where instr(search_key, ?) > 0
		order by tag_id limit ?`,
	domain.AssistantLabelPerson: `select distinct person_id from people_names where instr(search_key, ?) > 0
		order by person_id limit ?`,
	domain.AssistantLabelOrganization: `select distinct organization_id from organization_names
		where instr(search_key, ?) > 0 order by organization_id limit ?`,
	domain.AssistantLabelGroup:       `select id from groups where instr(title_key, ?) > 0 order by id limit ?`,
	domain.AssistantLabelSmartFolder: `select id from smart_folders where instr(title_key, ?) > 0 order by id limit ?`,
}

// assistantFoldedLists are the small name lists folded in Go because they store no search key.
var assistantFoldedLists = map[domain.AssistantLabelKind]string{
	domain.AssistantLabelTagKind:  `select id, name from tag_kinds order by id`,
	domain.AssistantLabelPlaylist: `select id, name from playlists order by id`,
}

// FindNames returns the entities of kind whose name folds to the same key as name, with their video counts for
// tags, people and organizations.
func (s *AssistantStore) FindNames(
	ctx context.Context, kind domain.AssistantLabelKind, name string,
) ([]domain.AssistantCandidate, error) {
	return s.findNames(ctx, kind, name, false)
}

// FindSimilarNames returns the entities of kind whose folded name, synonym or alias contains the folded name, so
// a request that names part of a tag or person is answered with the candidates to choose from.
func (s *AssistantStore) FindSimilarNames(
	ctx context.Context, kind domain.AssistantLabelKind, name string,
) ([]domain.AssistantCandidate, error) {
	return s.findNames(ctx, kind, name, true)
}

func (s *AssistantStore) findNames(
	ctx context.Context, kind domain.AssistantLabelKind, name string, similar bool,
) ([]domain.AssistantCandidate, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot look up the %s name: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	ids, err := assistantMatchingIDs(ctx, tx, kind, name, similar)
	if err != nil {
		return nil, err
	}
	names := map[domain.AssistantLabelRef]string{}
	if len(ids) > 0 {
		if err := readAssistantNames(ctx, tx, kind, assistantLabelNames[kind], ids, names); err != nil {
			return nil, err
		}
	}
	candidates := make([]domain.AssistantCandidate, 0, len(ids))
	for _, id := range ids {
		count, err := assistantVideoCount(ctx, tx, kind, id)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, domain.AssistantCandidate{
			Kind: kind, ID: id, Name: names[domain.AssistantLabelRef{Kind: kind, ID: id}], Count: count,
		})
	}
	return candidates, tx.Commit()
}

func assistantMatchingIDs(
	ctx context.Context, tx *sql.Tx, kind domain.AssistantLabelKind, name string, similar bool,
) ([]int64, error) {
	if statement, ok := assistantFoldedLists[kind]; ok {
		return assistantFoldedIDs(ctx, tx, statement, domain.FoldForMatch(name), similar)
	}
	statements := assistantNameMatches
	if similar {
		statements = assistantSimilarNameMatches
	}
	statement, ok := statements[kind]
	if !ok {
		return nil, fmt.Errorf("the assistant cannot look up %q names", kind)
	}
	key := domain.FoldForMatch(name)
	if kind == domain.AssistantLabelGroup || kind == domain.AssistantLabelSmartFolder {
		key = domain.NaturalSortKey(name)
	}
	if key == "" {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, statement, key, assistantNameLimit)
	if err != nil {
		return nil, fmt.Errorf("cannot look up the %s name: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot look up the %s name: %w", kind, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot look up the %s name: %w", kind, err)
	}
	return ids, nil
}

func assistantFoldedIDs(ctx context.Context, tx *sql.Tx, statement, key string, similar bool) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return nil, fmt.Errorf("cannot read the names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("cannot read the names: %w", err)
		}
		folded := domain.FoldForMatch(name)
		matches := folded == key || (similar && strings.Contains(folded, key))
		if key != "" && matches && len(ids) < assistantNameLimit {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the names: %w", err)
	}
	return ids, nil
}

// assistantVideoCount is the count a clarification shows beside a candidate: the videos a tag is on or a person
// or organization is credited on; other kinds show none.
func assistantVideoCount(ctx context.Context, tx *sql.Tx, kind domain.AssistantLabelKind, id int64) (int, error) {
	switch kind {
	case domain.AssistantLabelTag:
		return videoCountByTagID(ctx, tx, id)
	case domain.AssistantLabelPerson:
		return creditImpact(ctx, tx, domain.CreditPerson, []int64{id})
	case domain.AssistantLabelOrganization:
		return creditImpact(ctx, tx, domain.CreditOrganization, []int64{id})
	default:
		return 0, nil
	}
}

// EntityNames reads the current names of the ids of kind that still exist.
func (s *AssistantStore) EntityNames(
	ctx context.Context, kind domain.AssistantLabelKind, ids []int64,
) (map[int64]string, error) {
	statement, ok := assistantLabelNames[kind]
	if !ok {
		return nil, fmt.Errorf("unknown assistant label kind %q", kind)
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the %s names: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	names := map[domain.AssistantLabelRef]string{}
	if err := readAssistantNames(ctx, tx, kind, statement, ids, names); err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(names))
	for ref, name := range names {
		out[ref.ID] = name
	}
	return out, tx.Commit()
}
