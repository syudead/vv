package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// profileCodec maps one kind's profile onto columns of its entity table; links live in creditTables.profileLinks.
type profileCodec struct {
	columns []string
	// scan returns scan targets for columns and a function that stores the scanned values on c.
	scan  func(c *domain.Credit) ([]any, func())
	links func(c *domain.Credit) *[]string
	// write replaces the stored profile with the normalized one in input.
	write   func(ctx context.Context, tx *sql.Tx, id int64, input domain.CreditInput) error
	set     func(input domain.CreditInput) bool
	differs func(current domain.Credit, next domain.CreditInput) bool
	// fill fills the target's empty fields from sources and reports whether anything changed.
	fill func(target domain.Credit, sources []domain.Credit) (domain.CreditInput, bool)
}

// writeCreditProfile sets columns to args on entity id and replaces its ordered links.
func writeCreditProfile(ctx context.Context, tx *sql.Tx, t creditTables, id int64, columns []string, args []any,
	links []string,
) error {
	assignments := make([]string, len(columns))
	for i, column := range columns {
		assignments[i] = column + ` = ?`
	}
	statement := t.q(`update {entity} set ` + strings.Join(assignments, `, `) + ` where id = ?`)
	if _, err := tx.ExecContext(ctx, statement, append(args, id)...); err != nil {
		return fmt.Errorf("cannot save the profile of %s %d: %w", t.kind, id, err)
	}
	if _, err := tx.ExecContext(ctx, t.q(`delete from {profileLinks} where {id} = ?`), id); err != nil {
		return fmt.Errorf("cannot replace the links of %s %d: %w", t.kind, id, err)
	}
	for position, url := range links {
		if _, err := tx.ExecContext(ctx, t.q(`insert into {profileLinks} ({id}, position, url) values (?, ?, ?)`),
			id, position, url); err != nil {
			return fmt.Errorf("cannot save the links of %s %d: %w", t.kind, id, err)
		}
	}
	return nil
}

// addCreditProfiles reads the profiles, favorite marks, image revisions and links of a page in two statements.
func addCreditProfiles(ctx context.Context, tx *sql.Tx, t creditTables, items []domain.Credit, index map[int64]int,
	encodedIDs string,
) error {
	rows, err := tx.QueryContext(ctx, t.q(`select e.id, exists (select 1 from {favorites} f where f.{id} = e.id), `+
		t.imageRevision("e.id")+`, `+strings.Join(t.profile().columns, ", ")+
		` from {entity} e where e.id in (select value from json_each(?))`), encodedIDs)
	if err != nil {
		return fmt.Errorf("cannot read the profiles of %s entities: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, revision int64
		var favorite bool
		var scanned domain.Credit
		targets, done := t.profile().scan(&scanned)
		if err := rows.Scan(append([]any{&id, &favorite, &revision}, targets...)...); err != nil {
			return fmt.Errorf("cannot read the profiles of %s entities: %w", t.kind, err)
		}
		done()
		item := &items[index[id]]
		item.Profile, item.OrganizationProfile = scanned.Profile, scanned.OrganizationProfile
		item.Favorite = favorite
		item.ImageRevision = revision
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the profiles of %s entities: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select {id}, url, '' from {profileLinks}
		 where {id} in (select value from json_each(?)) order by {id}, position`), encodedIDs,
		func(id int64, url, _ string) {
			links := t.profile().links(&items[index[id]])
			*links = append(*links, url)
		})
	if err != nil {
		return fmt.Errorf("cannot read the links of %s entities: %w", t.kind, err)
	}
	return nil
}

// fillMergedProfile fills each empty target field and an empty link list from the first source in sourceIDs order
// whose value keeps the profile valid; it must run before the sources are deleted.
func fillMergedProfile(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64, sourceIDs []int64) error {
	ids := append([]int64{targetID}, sourceIDs...)
	encoded, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("cannot build %s ids: %w", t.kind, err)
	}
	credits := make([]domain.Credit, len(ids))
	index := make(map[int64]int, len(ids))
	for i, id := range ids {
		index[id] = i
	}
	if err := addCreditProfiles(ctx, tx, t, credits, index, string(encoded)); err != nil {
		return err
	}
	filled, changed := t.profile().fill(credits[0], credits[1:])
	if !changed {
		return nil
	}
	return t.profile().write(ctx, tx, targetID, filled)
}

// mergeFavorites makes a target that is not yet a favorite take the earliest source favorite time; the sources' rows
// cascade when they are deleted.
func mergeFavorites(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64, encodedSourceIDs string) error {
	if _, err := tx.ExecContext(ctx, t.q(`insert or ignore into {favorites} ({id}, favorited_at)
		select ?1, min(favorited_at) from {favorites}
		 where {id} in (select value from json_each(?2)) having count(*) > 0`),
		targetID, encodedSourceIDs); err != nil {
		return fmt.Errorf("cannot keep the favorite of %s %d: %w", t.kind, targetID, err)
	}
	return nil
}
