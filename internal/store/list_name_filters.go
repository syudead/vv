package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// listFilters are a query's tag and kind filters resolved in the read's snapshot: existing tag IDs with the
// tags its names name, the dropped unknown IDs, the kind its kind name names, and whether a name matched
// nothing, which empties the list.
type listFilters struct {
	tagIDs, missingTagIDs []int64
	tagKindNameID         int64
	nothing               bool
}

// resolveListFilters drops unknown tag IDs, as the ID filters do, and resolves tag names by name or synonym and
// the kind name; an unknown name matches nothing.
func resolveListFilters(ctx context.Context, tx tagTx, q domain.VideoQuery) (listFilters, error) {
	var out listFilters
	var err error
	if out.tagIDs, out.missingTagIDs, err = existingTagIDs(ctx, tx, q.TagIDs); err != nil {
		return listFilters{}, err
	}
	for _, name := range q.TagNames {
		lookup, found, err := lookupTagName(ctx, tx, name)
		if err != nil {
			return listFilters{}, err
		}
		if !found {
			out.nothing = true
			continue
		}
		if !slices.Contains(out.tagIDs, lookup.tagID) {
			out.tagIDs = append(out.tagIDs, lookup.tagID)
		}
	}
	if q.TagKindName != "" {
		err := tx.QueryRowContext(ctx, `select id from tag_kinds where name = ?`, q.TagKindName).Scan(&out.tagKindNameID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			out.nothing = true
		case err != nil:
			return listFilters{}, fmt.Errorf("cannot look up the tag kind by name (%s): %w", q.TagKindName, err)
		}
	}
	return out, nil
}

// narrow adds the resolved kind and the empty match to spec, whose tag IDs the caller took from f.
func (f listFilters) narrow(spec listSpec) listSpec {
	spec.tagKindNameID = f.tagKindNameID
	spec.matchNothing = f.nothing
	return spec
}

// creditNameCondition matches a credit column against every entity whose canonical name is the argument.
func creditNameCondition(column, namesTable, idColumn string) string {
	return column + ` in (select ` + idColumn + ` from ` + namesTable + ` where name = ? and canonical = 1)`
}

// NameFilters is q with its tag, kind, person and organization IDs replaced by their canonical names, so the
// paired node can apply them. Unknown tag IDs are left out, as the ID filter drops them; found is false when a
// kind, person or organization ID names nothing, which matches nothing on either node.
func (s *LibraryStore) NameFilters(ctx context.Context, q domain.VideoQuery) (named domain.VideoQuery, found bool, err error) {
	named = q
	named.TagIDs, named.TagKindID, named.Person, named.Organization = nil, 0, 0, 0
	for _, id := range q.TagIDs {
		name, err := canonicalNameByTagID(ctx, s.db.read, id)
		switch {
		case errors.Is(err, domain.ErrTagNotFound):
			continue
		case err != nil:
			return domain.VideoQuery{}, false, err
		}
		if !slices.Contains(named.TagNames, name) {
			named.TagNames = append(named.TagNames, name)
		}
	}
	for _, lookup := range []struct {
		id    int64
		query string
		out   *string
	}{
		{q.TagKindID, `select name from tag_kinds where id = ?`, &named.TagKindName},
		{q.Person, `select name from people_names where person_id = ? and canonical = 1`, &named.PersonName},
		{q.Organization, `select name from organization_names where organization_id = ? and canonical = 1`, &named.OrganizationName},
	} {
		if lookup.id == 0 {
			continue
		}
		err := s.db.read.QueryRowContext(ctx, lookup.query, lookup.id).Scan(lookup.out)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return domain.VideoQuery{}, false, nil
		case err != nil:
			return domain.VideoQuery{}, false, fmt.Errorf("cannot read the filter's name (id=%d): %w", lookup.id, err)
		}
	}
	return named, true, nil
}

// VocabularyJoin reads this node's tags, tag kinds, people and organizations by canonical name, so the peer's
// entity of the same name takes the local id. People share a spelling, so a name takes the lowest id.
func (s *LibraryStore) VocabularyJoin(ctx context.Context) (domain.VocabularyJoin, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VocabularyJoin{}, fmt.Errorf("cannot start reading the vocabulary: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var join domain.VocabularyJoin
	for _, read := range []struct {
		query string
		out   *map[string]int64
	}{
		{`select name, min(tag_id) from tag_names where canonical = 1 group by name`, &join.Tags},
		{`select name, min(id) from tag_kinds group by name`, &join.TagKinds},
		{`select name, min(person_id) from people_names where canonical = 1 group by name`, &join.People},
		{`select name, min(organization_id) from organization_names where canonical = 1 group by name`, &join.Organizations},
	} {
		if *read.out, err = readNameIDs(ctx, tx, read.query); err != nil {
			return domain.VocabularyJoin{}, err
		}
	}
	return join, tx.Commit()
}

func readNameIDs(ctx context.Context, tx *sql.Tx, query string) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("cannot read the names of the vocabulary: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			return nil, fmt.Errorf("cannot read the names of the vocabulary: %w", err)
		}
		out[name] = id
	}
	return out, rows.Err()
}
