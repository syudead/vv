package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// People and organizations share SQL with trusted table names supplied by creditTables.

type creditTables struct {
	kind domain.CreditKind
	// Entity, name/alias, external-ID and video-link tables.
	entity, names, externalIDs, links string
	// The entity reference column shared by name, external-ID and link tables.
	idColumn   string
	cursorKind string
	// Organizations have parent_id; people do not.
	hasParent bool
	smartRef  domain.SmartRuleRefKind
}

var creditTablesByKind = map[domain.CreditKind]creditTables{
	domain.CreditPerson: {
		kind: domain.CreditPerson, entity: "people", names: "people_names", externalIDs: "people_external_ids",
		links: "video_work_people", idColumn: "person_id", cursorKind: "people", smartRef: domain.SmartRefPerson,
	},
	domain.CreditOrganization: {
		kind: domain.CreditOrganization, entity: "organizations", names: "organization_names",
		externalIDs: "organization_external_ids", links: "video_work_organizations",
		idColumn: "organization_id", cursorKind: "organizations", hasParent: true,
		smartRef: domain.SmartRefOrganization,
	},
}

func creditTablesFor(kind domain.CreditKind) (creditTables, error) {
	t, ok := creditTablesByKind[kind]
	if !ok {
		return creditTables{}, fmt.Errorf("unknown credit kind: %q", kind)
	}
	return t, nil
}

func (t creditTables) q(statement string) string {
	return strings.NewReplacer(
		"{entity}", t.entity, "{names}", t.names, "{ext}", t.externalIDs, "{links}", t.links, "{id}", t.idColumn,
	).Replace(statement)
}

// People expose a null parent in the common listed relation.
func (t creditTables) parentColumn() string {
	if t.hasParent {
		return `e.parent_id`
	}
	return `null`
}

// Only people have a birth date to sort by.
func (t creditTables) birthKeyColumn() string {
	if t.kind == domain.CreditPerson {
		return `e.birth_key`
	}
	return `null`
}

// Get returns domain.ErrNotFound for a missing entity.
func (s *CreditStore) Get(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Credit{}, fmt.Errorf("cannot start reading the %s: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	credit, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	return credit, tx.Commit()
}

// Lookup returns at most one entity because external IDs are unique per kind.
func (s *CreditStore) Lookup(ctx context.Context, kind domain.CreditKind, source, value string) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Credit{}, fmt.Errorf("cannot start reading the %s: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	holder, found, err := externalIDHolder(ctx, tx, t, domain.ExternalID{Source: source, Value: value}, 0)
	if err != nil {
		return domain.Credit{}, err
	}
	if !found {
		return domain.Credit{}, domain.ErrNotFound
	}
	credit, err := creditByID(ctx, tx, t, holder)
	if err != nil {
		return domain.Credit{}, err
	}
	return credit, tx.Commit()
}

// Create normalizes input and validates external-ID ownership and organization parents; people ignore ParentID.
func (s *CreditStore) Create(ctx context.Context, kind domain.CreditKind, input domain.CreditInput) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
	normalized, err := domain.NormalizeCredit(input)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var parentID any
	if t.hasParent && normalized.ParentID != nil {
		if err := checkCreditParent(ctx, tx, t, 0, *normalized.ParentID); err != nil {
			return domain.Credit{}, err
		}
		parentID = *normalized.ParentID
	}
	if err := checkExternalIDsFree(ctx, tx, t, 0, normalized.ExternalIDs, input.ExternalIDs); err != nil {
		return domain.Credit{}, err
	}
	id, err := insertCredit(ctx, tx, t, normalized, parentID)
	if err != nil {
		return domain.Credit{}, err
	}
	credit, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	if err := s.db.commitEvents(ctx, tx); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot create the %s: %w", kind, err)
	}
	return credit, nil
}

// The caller must normalize input and validate external-ID availability and parent existence.
func insertCredit(ctx context.Context, tx *sql.Tx, t creditTables, input domain.CreditInput, parentID any) (int64, error) {
	statement := `insert into {entity} (created_at) values (?)`
	args := []any{time.Now().Unix()}
	if t.hasParent {
		statement = `insert into {entity} (created_at, parent_id) values (?, ?)`
		args = append(args, parentID)
	}
	res, err := tx.ExecContext(ctx, t.q(statement), args...)
	if err != nil {
		return 0, fmt.Errorf("cannot create the %s: %w", t.kind, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot create the %s: %w", t.kind, err)
	}
	if err := writeCreditNames(ctx, tx, t, id, input.Name, input.Aliases); err != nil {
		return 0, err
	}
	if err := writeCreditExternalIDs(ctx, tx, t, id, input.ExternalIDs); err != nil {
		return 0, err
	}
	if t.kind == domain.CreditPerson && !input.Profile.Equal(domain.PersonProfile{}) {
		if err := writePersonProfile(ctx, tx, id, input.Profile); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// Update replaces supplied fields and whole lists, validates the merged input and rejects parent cycles; unchanged values are not written.
func (s *CreditStore) Update(ctx context.Context, kind domain.CreditKind, id int64, patch domain.CreditPatch) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	next, err := patchedCredit(current, patch)
	if err != nil {
		return domain.Credit{}, err
	}
	changes := creditChanges(t, current, next)
	if !changes.any() {
		return current, tx.Commit()
	}
	if err := applyCreditChanges(ctx, tx, t, id, next, patch, changes); err != nil {
		return domain.Credit{}, err
	}
	updated, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	// Smart-folder notices and collection values depend on names and links, not on profiles.
	if err := s.db.commitMatchChangeIf(ctx, tx, changes.matched()); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot update the %s: %w", kind, err)
	}
	return updated, nil
}

// When aliases are unchanged, a duplicate can only come from the new name; report it against the submitted name field.
func patchedCredit(current domain.Credit, patch domain.CreditPatch) (domain.CreditInput, error) {
	merged := domain.CreditInput{
		Name: current.Name, Aliases: current.Aliases, ExternalIDs: current.ExternalIDs, Profile: current.Profile,
	}
	if current.Parent != nil {
		parent := current.Parent.ID
		merged.ParentID = &parent
	}
	if patch.Name != nil {
		merged.Name = *patch.Name
	}
	if patch.Aliases != nil {
		merged.Aliases = *patch.Aliases
	}
	if patch.ExternalIDs != nil {
		merged.ExternalIDs = *patch.ExternalIDs
	}
	if patch.ParentSet {
		merged.ParentID = patch.ParentID
	}
	if patch.Profile != nil {
		merged.Profile = patch.Profile.Apply(current.Profile)
	}
	if current.Provisional && patch.Name == nil {
		return domain.NormalizeProvisionalCredit(merged)
	}
	normalized, err := domain.NormalizeCredit(merged)
	var invalid *domain.InvalidWorkInfoError
	if patch.Aliases == nil && errors.As(err, &invalid) && invalid.Problem == domain.WorkInfoDuplicate &&
		strings.HasPrefix(invalid.Field, "aliases[") {
		return domain.CreditInput{}, &domain.InvalidWorkInfoError{Field: "name", Problem: domain.WorkInfoDuplicate}
	}
	return normalized, err
}

type creditChangeSet struct {
	names, externalIDs, parent, profile bool
}

func (c creditChangeSet) any() bool { return c.matched() || c.profile }

// matched reports changes that can alter smart-folder matches.
func (c creditChangeSet) matched() bool { return c.names || c.externalIDs || c.parent }

// Compare aliases and external IDs in read order; people have no parent to compare.
func creditChanges(t creditTables, current domain.Credit, next domain.CreditInput) creditChangeSet {
	aliases := slices.Clone(next.Aliases)
	domain.SortAliases(aliases)
	externalIDs := slices.Clone(next.ExternalIDs)
	domain.SortExternalIDs(externalIDs)
	var changes creditChangeSet
	changes.names = current.Name != next.Name || !slices.Equal(current.Aliases, aliases)
	changes.externalIDs = !slices.Equal(current.ExternalIDs, externalIDs)
	if t.hasParent {
		var currentParent int64
		if current.Parent != nil {
			currentParent = current.Parent.ID
		}
		var nextParent int64
		if next.ParentID != nil {
			nextParent = *next.ParentID
		}
		changes.parent = currentParent != nextParent
	}
	changes.profile = t.kind == domain.CreditPerson && !current.Profile.Equal(next.Profile)
	return changes
}

func applyCreditChanges(ctx context.Context, tx *sql.Tx, t creditTables, id int64, next domain.CreditInput,
	patch domain.CreditPatch, changes creditChangeSet,
) error {
	if changes.externalIDs {
		if err := checkExternalIDsFree(ctx, tx, t, id, next.ExternalIDs, *patch.ExternalIDs); err != nil {
			return err
		}
	}
	if changes.parent {
		var parentID any
		if next.ParentID != nil {
			if err := checkCreditParent(ctx, tx, t, id, *next.ParentID); err != nil {
				return err
			}
			parentID = *next.ParentID
		}
		if _, err := tx.ExecContext(ctx, t.q(`update {entity} set parent_id = ? where id = ?`), parentID, id); err != nil {
			return fmt.Errorf("cannot change the parent of %s %d: %w", t.kind, id, err)
		}
	}
	if changes.names {
		if _, err := tx.ExecContext(ctx, t.q(`delete from {names} where {id} = ?`), id); err != nil {
			return fmt.Errorf("cannot replace the names of %s %d: %w", t.kind, id, err)
		}
		if err := writeCreditNames(ctx, tx, t, id, next.Name, next.Aliases); err != nil {
			return err
		}
	}
	if changes.externalIDs {
		if _, err := tx.ExecContext(ctx, t.q(`delete from {ext} where {id} = ?`), id); err != nil {
			return fmt.Errorf("cannot replace the external IDs of %s %d: %w", t.kind, id, err)
		}
		if err := writeCreditExternalIDs(ctx, tx, t, id, next.ExternalIDs); err != nil {
			return err
		}
	}
	if changes.profile {
		return writePersonProfile(ctx, tx, id, next.Profile)
	}
	return nil
}

// Delete cascades names, IDs and links, clears child parents, compacts affected link positions, and deletes a
// person's face data.
func (s *CreditStore) Delete(ctx context.Context, kind domain.CreditKind, id int64) error {
	t, err := creditTablesFor(kind)
	if err != nil {
		return err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := existingCreditIDs(ctx, tx, t, []int64{id})
	if err != nil {
		return err
	}
	if !existing[id] {
		return domain.ErrNotFound
	}
	affected, err := creditLinkLists(ctx, tx, t, []int64{id})
	if err != nil {
		return err
	}
	if t.kind == domain.CreditPerson {
		if err := deletePersonFaceData(ctx, tx, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, t.q(`delete from {entity} where id = ?`), id); err != nil {
		return fmt.Errorf("cannot delete %s %d: %w", kind, id, err)
	}
	if err := renumberCreditLinks(ctx, tx, t, affected); err != nil {
		return err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return fmt.Errorf("cannot delete %s %d: %w", kind, id, err)
	}
	return nil
}

// Merge transfers links, aliases, external IDs, children and face data and fills empty profile fields, a missing face and a missing canonical name atomically; reject self-merges, parent cycles and merged limits.
func (s *CreditStore) Merge(ctx context.Context, kind domain.CreditKind, targetID int64, sourceIDs []int64) (domain.CreditMergeOutcome, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.CreditMergeOutcome{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.CreditMergeOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	merged, notFound, err := mergeSources(ctx, tx, t, targetID, sourceIDs)
	if err != nil {
		return domain.CreditMergeOutcome{}, err
	}
	if len(merged) > 0 {
		if err := mergeCreditsInto(ctx, tx, t, targetID, merged); err != nil {
			return domain.CreditMergeOutcome{}, err
		}
		if err := checkMergedCreditLimits(ctx, tx, t, targetID); err != nil {
			return domain.CreditMergeOutcome{}, err
		}
	}
	credit, err := creditByID(ctx, tx, t, targetID)
	if err != nil {
		return domain.CreditMergeOutcome{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.CreditMergeOutcome{}, fmt.Errorf("cannot merge into %s %d: %w", kind, targetID, err)
	}
	return domain.CreditMergeOutcome{Credit: credit, NotFoundIDs: notFound}, nil
}

// Validate the target, self-merges and parent cycles; deduplicate sources in input order and report missing IDs.
func mergeSources(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64, sourceIDs []int64) ([]int64, []int64, error) {
	unique := uniqueTagIDs(sourceIDs)
	existing, err := existingCreditIDs(ctx, tx, t, append([]int64{targetID}, unique...))
	if err != nil {
		return nil, nil, err
	}
	if !existing[targetID] {
		return nil, nil, domain.ErrNotFound
	}
	if slices.Contains(unique, targetID) {
		return nil, nil, domain.ErrMergeSameCredit
	}
	notFound := []int64{}
	merged := make([]int64, 0, len(unique))
	for _, id := range unique {
		if existing[id] {
			merged = append(merged, id)
		} else {
			notFound = append(notFound, id)
		}
	}
	if t.hasParent && len(merged) > 0 {
		ancestors, err := organizationAncestors(ctx, tx, targetID)
		if err != nil {
			return nil, nil, err
		}
		for _, id := range merged {
			if ancestors[id] {
				return nil, nil, domain.ErrCreditParentCycle
			}
		}
	}
	return merged, notFound, nil
}

// Use a fixed number of statements regardless of source count, excluding position compaction.
func mergeCreditsInto(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64, sourceIDs []int64) error {
	encoded, err := json.Marshal(sourceIDs)
	if err != nil {
		return fmt.Errorf("cannot build %s ids: %w", t.kind, err)
	}
	if t.kind == domain.CreditPerson {
		if err := fillMergedPersonProfile(ctx, tx, targetID, sourceIDs); err != nil {
			return err
		}
		faceFrom, err := fillMergedPersonImage(ctx, tx, targetID, sourceIDs)
		if err != nil {
			return err
		}
		if faceFrom != 0 {
			if err := moveFaceVector(ctx, tx, targetID, faceFrom); err != nil {
				return err
			}
		}
		if err := mergePersonFaceData(ctx, tx, targetID, string(encoded)); err != nil {
			return err
		}
		// A target that is not yet a favorite takes the earliest source favorite time; the sources' rows cascade below.
		if _, err := tx.ExecContext(ctx, `insert or ignore into person_favorites (person_id, favorited_at)
			select ?1, min(favorited_at) from person_favorites
			 where person_id in (select value from json_each(?2)) having count(*) > 0`,
			targetID, string(encoded)); err != nil {
			return fmt.Errorf("cannot keep the favorite of person %d: %w", targetID, err)
		}
	}
	affected, err := creditLinkLists(ctx, tx, t, sourceIDs)
	if err != nil {
		return err
	}
	statements := []struct{ what, statement string }{
		// A target without a canonical name takes the first named source's, before the rest become aliases.
		{"name the target", `insert into {names} ({id}, name, canonical, language, search_key, sort_key, search_version)
			select ?1, n.name, 1, null, n.search_key, n.sort_key, n.search_version
			  from json_each(?2) s join {names} n on n.{id} = s.value and n.canonical = 1
			 where not exists (select 1 from {names} where {id} = ?1 and canonical = 1)
			 order by s.key limit 1`},
		// Preserve link positions; uniqueness leaves conflicting source rows for the following delete.
		{"move the links", `update or ignore {links} set {id} = ?1 where {id} in (select value from json_each(?2))`},
		{"drop the links", `delete from {links} where {id} in (select value from json_each(?2))`},
		// Insert source names as aliases, deduplicating through the primary key.
		{"move the names", `insert or ignore into {names} ({id}, name, canonical, language, search_key, sort_key, search_version)
			select ?1, name, 0, language, search_key, sort_key, search_version from {names}
			 where {id} in (select value from json_each(?2))`},
		// External IDs are unique per kind, so the target cannot already own a source ID.
		{"move the external IDs", `update {ext} set {id} = ?1 where {id} in (select value from json_each(?2))`},
	}
	if t.hasParent {
		statements = append(statements, struct{ what, statement string }{
			"move the children", `update organizations set parent_id = ?1 where parent_id in (select value from json_each(?2))`,
		})
	}
	statements = append(statements, struct{ what, statement string }{
		"delete the sources", `delete from {entity} where id in (select value from json_each(?2))`,
	})
	for _, s := range statements {
		if _, err := tx.ExecContext(ctx, t.q(s.statement), targetID, string(encoded)); err != nil {
			return fmt.Errorf("cannot %s to %s %d: %w", s.what, t.kind, targetID, err)
		}
	}
	if err := renumberCreditLinks(ctx, tx, t, affected); err != nil {
		return err
	}
	return rewriteSmartRuleRefs(ctx, tx, t.smartRef, targetID, sourceIDs)
}

// Reject merged limits before commit so the caller can roll back the entire merge.
func checkMergedCreditLimits(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64) error {
	var aliases, externalIDs int
	if err := tx.QueryRowContext(ctx, t.q(`select
		(select count(*) from {names} where {id} = ?1 and canonical = 0),
		(select count(*) from {ext} where {id} = ?1)`), targetID).Scan(&aliases, &externalIDs); err != nil {
		return fmt.Errorf("cannot count the names of %s %d: %w", t.kind, targetID, err)
	}
	if aliases > domain.CreditAliasesMaxCount {
		return &domain.InvalidWorkInfoError{Field: "aliases", Problem: domain.WorkInfoTooMany, Limit: domain.CreditAliasesMaxCount}
	}
	if externalIDs > domain.ExternalIDsMaxCount {
		return &domain.InvalidWorkInfoError{Field: "externalIds", Problem: domain.WorkInfoTooMany, Limit: domain.ExternalIDsMaxCount}
	}
	return nil
}

func existingCreditIDs(ctx context.Context, tx *sql.Tx, t creditTables, ids []int64) (map[int64]bool, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build %s ids: %w", t.kind, err)
	}
	rows, err := tx.QueryContext(ctx, t.q(`select id from {entity} where id in (select value from json_each(?))`), string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s ids: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	existing := make(map[int64]bool, len(ids))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read %s ids: %w", t.kind, err)
		}
		existing[id] = true
	}
	return existing, rows.Err()
}

// Stop at previously visited parents defensively, although stored hierarchies must be acyclic.
func organizationAncestors(ctx context.Context, tx *sql.Tx, id int64) (map[int64]bool, error) {
	ancestors := map[int64]bool{}
	current := id
	for {
		var parent sql.NullInt64
		err := tx.QueryRowContext(ctx, `select parent_id from organizations where id = ?`, current).Scan(&parent)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !parent.Valid) {
			return ancestors, nil
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read the parent of organization %d: %w", current, err)
		}
		if parent.Int64 == id || ancestors[parent.Int64] {
			return ancestors, nil
		}
		ancestors[parent.Int64] = true
		current = parent.Int64
	}
}

// Reject missing parents and any parent chain that reaches the child itself.
func checkCreditParent(ctx context.Context, tx *sql.Tx, t creditTables, id, parentID int64) error {
	existing, err := existingCreditIDs(ctx, tx, t, []int64{parentID})
	if err != nil {
		return err
	}
	if !existing[parentID] {
		return &domain.CreditNotFoundError{Kind: t.kind, ID: parentID, Field: "parentId"}
	}
	if id == 0 {
		return nil
	}
	if parentID == id {
		return domain.ErrCreditParentCycle
	}
	ancestors, err := organizationAncestors(ctx, tx, parentID)
	if err != nil {
		return err
	}
	if ancestors[id] {
		return domain.ErrCreditParentCycle
	}
	return nil
}

func externalIDHolder(ctx context.Context, tx *sql.Tx, t creditTables, id domain.ExternalID, exceptID int64) (int64, bool, error) {
	var holder int64
	err := tx.QueryRowContext(ctx, t.q(`select {id} from {ext} where source = ? and value = ? and {id} <> ?`),
		id.Source, id.Value, exceptID).Scan(&holder)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("cannot look up the external ID of a %s: %w", t.kind, err)
	}
	return holder, true, nil
}

// Report conflicts at the index in the caller's original external-ID list.
func checkExternalIDsFree(ctx context.Context, tx *sql.Tx, t creditTables, selfID int64, normalized, raw []domain.ExternalID) error {
	for _, id := range normalized {
		holder, found, err := externalIDHolder(ctx, tx, t, id, selfID)
		if err != nil {
			return err
		}
		if found {
			return &domain.CreditExternalIDTakenError{
				Kind: t.kind, Field: fmt.Sprintf("externalIds[%d]", domain.ExternalIDIndex(raw, id)), HolderID: holder,
			}
		}
	}
	return nil
}

func writeCreditNames(ctx context.Context, tx *sql.Tx, t creditTables, id int64, name string, aliases []domain.Alias) error {
	statement := t.q(`insert into {names} ({id}, name, canonical, language, search_key, sort_key, search_version)
		values (?, ?, ?, ?, ?, ?, ?)`)
	write := func(name string, canonical bool, language string) error {
		var lang any
		if language != "" {
			lang = language
		}
		if _, err := tx.ExecContext(ctx, statement, id, name, boolToInt(canonical), lang,
			domain.FoldForMatch(name), domain.NaturalSortKey(name), domain.SearchKeyVersion); err != nil {
			return fmt.Errorf("cannot save the names of %s %d: %w", t.kind, id, err)
		}
		return nil
	}
	if err := write(name, true, ""); err != nil {
		return err
	}
	for _, alias := range aliases {
		if err := write(alias.Name, false, alias.Language); err != nil {
			return err
		}
	}
	return nil
}

func writeCreditExternalIDs(ctx context.Context, tx *sql.Tx, t creditTables, id int64, ids []domain.ExternalID) error {
	statement := t.q(`insert into {ext} ({id}, source, value) values (?, ?, ?)`)
	for _, external := range ids {
		if _, err := tx.ExecContext(ctx, statement, id, external.Source, external.Value); err != nil {
			return fmt.Errorf("cannot save the external IDs of %s %d: %w", t.kind, id, err)
		}
	}
	return nil
}

type creditLinkList struct {
	contentKey, role string
}

// Return link lists requiring compaction after deletion or merge.
func creditLinkLists(ctx context.Context, tx *sql.Tx, t creditTables, ids []int64) ([]creditLinkList, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build %s ids: %w", t.kind, err)
	}
	rows, err := tx.QueryContext(ctx, t.q(`select distinct content_key, role from {links}
		where {id} in (select value from json_each(?))`), string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the links of a %s: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	var lists []creditLinkList
	for rows.Next() {
		var list creditLinkList
		if err := rows.Scan(&list.contentKey, &list.role); err != nil {
			return nil, fmt.Errorf("cannot read the links of a %s: %w", t.kind, err)
		}
		lists = append(lists, list)
	}
	return lists, rows.Err()
}

// Compact positions in ascending order after reading them all; each destination is already free, avoiding primary-key collisions.
func renumberCreditLinks(ctx context.Context, tx *sql.Tx, t creditTables, lists []creditLinkList) error {
	update := t.q(`update {links} set position = ? where content_key = ? and role = ? and position = ?`)
	for _, list := range lists {
		positions, err := creditLinkPositions(ctx, tx, t, list)
		if err != nil {
			return err
		}
		for i, position := range positions {
			if position == i {
				continue
			}
			if _, err := tx.ExecContext(ctx, update, i, list.contentKey, list.role, position); err != nil {
				return fmt.Errorf("cannot renumber the links of a %s: %w", t.kind, err)
			}
		}
	}
	return nil
}

func creditLinkPositions(ctx context.Context, tx *sql.Tx, t creditTables, list creditLinkList) ([]int, error) {
	rows, err := tx.QueryContext(ctx, t.q(`select position from {links} where content_key = ? and role = ? order by position`),
		list.contentKey, list.role)
	if err != nil {
		return nil, fmt.Errorf("cannot read the links of a %s: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	var positions []int
	for rows.Next() {
		var position int
		if err := rows.Scan(&position); err != nil {
			return nil, fmt.Errorf("cannot read the links of a %s: %w", t.kind, err)
		}
		positions = append(positions, position)
	}
	return positions, rows.Err()
}

// RefreshSearchKeys upgrades credit name and work-code keys at startup after tag keys are refreshed.
func (s *CreditStore) RefreshSearchKeys(ctx context.Context) (int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	refreshed := 0
	for _, kind := range []domain.CreditKind{domain.CreditPerson, domain.CreditOrganization} {
		n, err := refreshCreditNameKeys(ctx, tx, creditTablesByKind[kind])
		if err != nil {
			return 0, err
		}
		refreshed += n
	}
	n, err := refreshWorkInfoCodeKeys(ctx, tx)
	if err != nil {
		return 0, err
	}
	refreshed += n
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cannot save the credit search keys: %w", err)
	}
	return refreshed, nil
}

type staleKeyRow struct {
	id    any
	value string
}

func staleKeyRows(ctx context.Context, tx *sql.Tx, query string) ([]staleKeyRow, error) {
	rows, err := tx.QueryContext(ctx, query, domain.SearchKeyVersion)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var stale []staleKeyRow
	for rows.Next() {
		var row staleKeyRow
		if err := rows.Scan(&row.id, &row.value); err != nil {
			return nil, err
		}
		stale = append(stale, row)
	}
	return stale, rows.Err()
}

func refreshCreditNameKeys(ctx context.Context, tx *sql.Tx, t creditTables) (int, error) {
	stale, err := staleKeyRows(ctx, tx, t.q(`select {id}, name from {names} where search_version < ?`))
	if err != nil {
		return 0, fmt.Errorf("cannot read the names of %s entities: %w", t.kind, err)
	}
	update := t.q(`update {names} set search_key = ?, sort_key = ?, search_version = ? where {id} = ? and name = ?`)
	for _, row := range stale {
		if _, err := tx.ExecContext(ctx, update, domain.FoldForMatch(row.value), domain.NaturalSortKey(row.value),
			domain.SearchKeyVersion, row.id, row.value); err != nil {
			return 0, fmt.Errorf("cannot save the search key of a %s name: %w", t.kind, err)
		}
	}
	return len(stale), nil
}

func refreshWorkInfoCodeKeys(ctx context.Context, tx *sql.Tx) (int, error) {
	stale, err := staleKeyRows(ctx, tx,
		`select content_key, coalesce(code, '') from video_work_info where search_version < ?`)
	if err != nil {
		return 0, fmt.Errorf("cannot read the work information codes: %w", err)
	}
	for _, row := range stale {
		if _, err := tx.ExecContext(ctx,
			`update video_work_info set code_key = ?, search_version = ? where content_key = ?`,
			domain.FoldForMatch(row.value), domain.SearchKeyVersion, row.id); err != nil {
			return 0, fmt.Errorf("cannot save the code key of the work information: %w", err)
		}
	}
	return len(stale), nil
}
