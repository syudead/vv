package store

import (
	"context"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestGroupCardsReturnReadFailuresInsteadOfPartialCards(t *testing.T) {
	db := migratedDB(t)
	video := looseVideo(t, db, "cover")
	id := createGroup(t, db, "group", domain.GroupAdd{VideoIDs: []int64{video}})
	execSQL(t, db, `update videos set thumbnail_state = 'done'`)
	for _, tc := range []struct{ name, fault, message string }{
		{"summary", `update group_values set duration_ms = 'invalid duration'`, "invalid duration"},
		{"preview", `update videos set width = 'invalid width'`, "invalid width"},
		{"tags", `alter table tag_names rename to unavailable_tag_names`, "cannot read group card tags"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tx, err := db.sql.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.ExecContext(ctx, tc.fault); err != nil {
				t.Fatal(err)
			}
			cards, err := loadGroupCards(ctx, tx, domain.AudienceOwner, []int64{id})
			if err == nil || !strings.Contains(err.Error(), tc.message) || cards != nil {
				t.Fatalf("cards = %+v, error = %v; want no partial cards and %q", cards, err, tc.message)
			}
		})
	}
}
