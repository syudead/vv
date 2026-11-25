package store

import (
	"context"
	"reflect"
	"testing"
)

const creditNamePositionsVersion = 98

// The canonical row becomes position 0 and the aliases follow in the order the screens listed them, unfolded, so the
// full-width Ｂ comes after C; going back restores the canonical flag on position 0.
func TestCreditNamePositionsMigrationNumbersTheNames(t *testing.T) {
	db, provider := migrateTo(t, creditNamePositionsVersion-1)
	ctx := context.Background()
	execAll(t, db,
		`insert into people (id, created_at) values (1, 0), (2, 0)`,
		`insert into people_names (person_id, name, canonical, language) values
			(1, 'Ｂ', 0, null), (1, '山田', 1, null), (1, 'C', 0, 'en'), (2, 'Solo', 1, null)`,
		`insert into organizations (id, created_at) values (1, 0)`,
		`insert into organization_names (organization_id, name, canonical) values
			(1, 'x10', 0), (1, 'Org', 1), (1, 'x9', 0)`)

	if _, err := provider.UpTo(ctx, creditNamePositionsVersion); err != nil {
		t.Fatal(err)
	}
	wantPeople := [][]any{
		{int64(1), int64(0), "山田", nil}, {int64(1), int64(1), "C", "en"}, {int64(1), int64(2), "Ｂ", nil},
		{int64(2), int64(0), "Solo", nil},
	}
	if got := queryRows(t, db.sql, `select person_id, position, name, language from people_names
		order by person_id, position`, 4); !reflect.DeepEqual(got, wantPeople) {
		t.Fatalf("people_names = %v, want %v", got, wantPeople)
	}
	wantOrganizations := [][]any{{int64(0), "Org"}, {int64(1), "x9"}, {int64(2), "x10"}}
	if got := queryRows(t, db.sql, `select position, name from organization_names order by position`,
		2); !reflect.DeepEqual(got, wantOrganizations) {
		t.Fatalf("organization_names = %v, want %v", got, wantOrganizations)
	}
	execAll(t, db, `insert into people_names (person_id, name, position) values (2, 'Duo', 1)`)

	if _, err := provider.DownTo(ctx, creditNamePositionsVersion-1); err != nil {
		t.Fatal(err)
	}
	wantDown := [][]any{
		{int64(1), "C", int64(0)}, {int64(1), "山田", int64(1)}, {int64(1), "Ｂ", int64(0)},
		{int64(2), "Duo", int64(0)}, {int64(2), "Solo", int64(1)},
	}
	if got := queryRows(t, db.sql, `select person_id, name, canonical from people_names order by person_id, name`,
		3); !reflect.DeepEqual(got, wantDown) {
		t.Fatalf("people_names after down = %v, want %v", got, wantDown)
	}
}
