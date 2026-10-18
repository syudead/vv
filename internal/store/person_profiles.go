package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Profiles are columns on people plus ordered rows in people_links; organizations have none.

// personProfileColumnNames is the order scannedProfile.targets and personProfileArgs use.
var personProfileColumnNames = []string{
	"gender", "birth_date", "death_date", "birthplace", "height_cm", "bust_cm", "waist_cm", "hips_cm",
	"cup", "blood_type", "career_start_year", "career_end_year", "bio",
}

var personProfileColumns = strings.Join(personProfileColumnNames, ", ")

type scannedProfile struct {
	texts [7]sql.NullString
	ints  [6]sql.NullInt64
}

func (s *scannedProfile) targets() []any {
	t := &s.texts
	n := &s.ints
	return []any{&t[0], &t[1], &t[2], &t[3], &n[0], &n[1], &n[2], &n[3], &t[4], &t[5], &n[4], &n[5], &t[6]}
}

func (s *scannedProfile) profile() domain.PersonProfile {
	t := s.texts
	n := s.ints
	return domain.PersonProfile{
		Gender: t[0].String, BirthDate: t[1].String, DeathDate: t[2].String, Birthplace: t[3].String,
		HeightCm: optionalInt(n[0]), BustCm: optionalInt(n[1]), WaistCm: optionalInt(n[2]), HipsCm: optionalInt(n[3]),
		Cup: t[4].String, BloodType: t[5].String, CareerStartYear: optionalInt(n[4]), CareerEndYear: optionalInt(n[5]),
		Bio: t[6].String,
	}
}

func optionalInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}

func optionalIntArg(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func personProfileArgs(p domain.PersonProfile) []any {
	return []any{
		nullString(p.Gender), nullString(p.BirthDate), nullString(p.DeathDate), nullString(p.Birthplace),
		optionalIntArg(p.HeightCm), optionalIntArg(p.BustCm), optionalIntArg(p.WaistCm), optionalIntArg(p.HipsCm),
		nullString(p.Cup), nullString(p.BloodType), optionalIntArg(p.CareerStartYear), optionalIntArg(p.CareerEndYear),
		nullString(p.Bio),
	}
}

// writePersonProfile replaces the stored profile with a normalized one.
func writePersonProfile(ctx context.Context, tx *sql.Tx, id int64, profile domain.PersonProfile) error {
	statement := `update people set `
	for i, column := range personProfileColumnNames {
		if i > 0 {
			statement += `, `
		}
		statement += column + ` = ?`
	}
	args := append(personProfileArgs(profile), id)
	if _, err := tx.ExecContext(ctx, statement+` where id = ?`, args...); err != nil {
		return fmt.Errorf("cannot save the profile of person %d: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `delete from people_links where person_id = ?`, id); err != nil {
		return fmt.Errorf("cannot replace the links of person %d: %w", id, err)
	}
	for position, url := range profile.Links {
		if _, err := tx.ExecContext(ctx, `insert into people_links (person_id, position, url) values (?, ?, ?)`,
			id, position, url); err != nil {
			return fmt.Errorf("cannot save the links of person %d: %w", id, err)
		}
	}
	return nil
}

// addPersonProfiles reads the profiles, favorite marks and links of a page of people in two statements.
func addPersonProfiles(ctx context.Context, tx *sql.Tx, items []domain.Credit, index map[int64]int, encodedIDs string) error {
	rows, err := tx.QueryContext(ctx, `select id, exists (select 1 from person_favorites pf where pf.person_id = people.id), `+
		personProfileColumns+` from people where id in (select value from json_each(?))`, encodedIDs)
	if err != nil {
		return fmt.Errorf("cannot read the profiles of people: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var favorite bool
		var scanned scannedProfile
		if err := rows.Scan(append([]any{&id, &favorite}, scanned.targets()...)...); err != nil {
			return fmt.Errorf("cannot read the profiles of people: %w", err)
		}
		items[index[id]].Profile = scanned.profile()
		items[index[id]].Favorite = favorite
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the profiles of people: %w", err)
	}
	err = eachCreditRow(ctx, tx, `select person_id, url, '' from people_links
		 where person_id in (select value from json_each(?)) order by person_id, position`, encodedIDs,
		func(id int64, url, _ string) {
			items[index[id]].Profile.Links = append(items[index[id]].Profile.Links, url)
		})
	if err != nil {
		return fmt.Errorf("cannot read the links of people: %w", err)
	}
	return nil
}

// fillMergedPersonProfile fills each empty target field and an empty link list from the first source in sourceIDs
// order whose value keeps the profile valid; it must run before the sources are deleted.
func fillMergedPersonProfile(ctx context.Context, tx *sql.Tx, targetID int64, sourceIDs []int64) error {
	ids := append([]int64{targetID}, sourceIDs...)
	encoded, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("cannot build person ids: %w", err)
	}
	people := make([]domain.Credit, len(ids))
	index := make(map[int64]int, len(ids))
	for i, id := range ids {
		index[id] = i
	}
	if err := addPersonProfiles(ctx, tx, people, index, string(encoded)); err != nil {
		return err
	}
	sources := make([]domain.PersonProfile, len(sourceIDs))
	for i := range sources {
		sources[i] = people[i+1].Profile
	}
	current := people[0].Profile
	filled := domain.FillPersonProfile(current, sources, time.Now())
	if filled.Equal(current) {
		return nil
	}
	return writePersonProfile(ctx, tx, targetID, filled)
}
