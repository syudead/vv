package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func createRuleFolder(t *testing.T, db *DB, name string, rule domain.SmartRule) domain.SmartFolder {
	t.Helper()
	folder, err := db.SmartFolders().CreateSmartFolder(context.Background(),
		domain.SmartFolderCreate{Name: name, Rule: rule})
	if err != nil {
		t.Fatalf("CreateSmartFolder(%q) error = %v", name, err)
	}
	return folder
}

func smartRuleJSON(t *testing.T, q queryRower, id int64) string {
	t.Helper()
	var rule string
	if err := q.QueryRowContext(context.Background(), `select rule from smart_folders where id = ?`, id).Scan(&rule); err != nil {
		t.Fatal(err)
	}
	return rule
}

func createTagID(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	tag, err := db.Tags().CreateTag(context.Background(), name)
	if err != nil {
		t.Fatalf("CreateTag(%q) error = %v", name, err)
	}
	return tag.ID
}

func TestMergeTagsRewritesSmartFolderRules(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b, c := createTagID(t, db, "a"), createTagID(t, db, "b"), createTagID(t, db, "c")

	onlyA := createRuleFolder(t, db, "A", domain.SmartRule{Tags: []int64{a}})
	both := createRuleFolder(t, db, "AB", domain.SmartRule{Tags: []int64{a, b}})
	mixed := createRuleFolder(t, db, "CA", domain.SmartRule{Q: "x", Tags: []int64{c, a}})
	other := createRuleFolder(t, db, "C", domain.SmartRule{Tags: []int64{c}})

	if _, err := db.Tags().MergeTags(ctx, b, []int64{a}); err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{
		onlyA.ID: fmt.Sprintf(`{"tags":[%d]}`, b),
		both.ID:  fmt.Sprintf(`{"tags":[%d]}`, b),
		mixed.ID: fmt.Sprintf(`{"q":"x","tags":[%d,%d]}`, c, b),
		other.ID: fmt.Sprintf(`{"tags":[%d]}`, c),
	}
	for id, rule := range want {
		if got := smartRuleJSON(t, db.sql, id); got != rule {
			t.Errorf("folder %d rule = %s, want %s", id, got, rule)
		}
	}
	// Merge-driven reference rewrites are not owner edits and must not advance timestamps.
	folder, err := db.SmartFolders().SmartFolder(ctx, onlyA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if folder.UpdatedAt != onlyA.UpdatedAt || !folder.OwnerKept {
		t.Errorf("folder after the merge = %+v, want updated_at %d kept", folder, onlyA.UpdatedAt)
	}
}

func TestTagMergeRewritesRulesInItsTransactionAndRollsBack(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := createTagID(t, db, "a"), createTagID(t, db, "b")
	folder := createRuleFolder(t, db, "AB", domain.SmartRule{Tags: []int64{a, b}})
	original := smartRuleJSON(t, db.sql, folder.ID)

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := mergeTagsInto(ctx, tx, b, []int64{a}); err != nil {
		t.Fatal(err)
	}
	if got, want := smartRuleJSON(t, tx, folder.ID), fmt.Sprintf(`{"tags":[%d]}`, b); got != want {
		t.Errorf("rule inside the merge = %s, want %s", got, want)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := smartRuleJSON(t, db.sql, folder.ID); got != original {
		t.Errorf("rule after the rollback = %s, want %s", got, original)
	}
}

func TestMergeCreditsRewritesSmartFolderRules(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	x, y := createPerson(t, db, "X"), createPerson(t, db, "Y")
	ox, oy := createOrganization(t, db, "OX", nil), createOrganization(t, db, "OY", nil)
	// Overlapping person/organization IDs must rewrite only the merged kind.
	byPerson := createRuleFolder(t, db, "P", domain.SmartRule{Person: x.ID, Organization: ox.ID})
	byOrganization := createRuleFolder(t, db, "O", domain.SmartRule{Organization: ox.ID})

	if _, err := db.Credits().Merge(ctx, domain.CreditPerson, y.ID, []int64{x.ID}); err != nil {
		t.Fatal(err)
	}
	if got, want := smartRuleJSON(t, db.sql, byPerson.ID),
		fmt.Sprintf(`{"person":%d,"organization":%d}`, y.ID, ox.ID); got != want {
		t.Errorf("rule after the person merge = %s, want %s", got, want)
	}
	if _, err := db.Credits().Merge(ctx, domain.CreditOrganization, oy.ID, []int64{ox.ID}); err != nil {
		t.Fatal(err)
	}
	if got, want := smartRuleJSON(t, db.sql, byPerson.ID),
		fmt.Sprintf(`{"person":%d,"organization":%d}`, y.ID, oy.ID); got != want {
		t.Errorf("rule after the organization merge = %s, want %s", got, want)
	}
	if got, want := smartRuleJSON(t, db.sql, byOrganization.ID),
		fmt.Sprintf(`{"organization":%d}`, oy.ID); got != want {
		t.Errorf("organization rule = %s, want %s", got, want)
	}
}

func TestRefusedCreditMergeRewritesNoRule(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createPerson(t, db, "T")
	targetAliases := make([]domain.Alias, 30)
	for i := range targetAliases {
		targetAliases[i] = domain.Alias{Name: fmt.Sprintf("T%d", i)}
	}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, target.ID, domain.CreditPatch{Aliases: &targetAliases}); err != nil {
		t.Fatal(err)
	}
	sourceAliases := make([]domain.Alias, 25)
	for i := range sourceAliases {
		sourceAliases[i] = domain.Alias{Name: fmt.Sprintf("S%d", i)}
	}
	source := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S", Aliases: sourceAliases})
	folder := createRuleFolder(t, db, "S", domain.SmartRule{Person: source.ID})
	original := smartRuleJSON(t, db.sql, folder.ID)

	// Limits are checked after rule rewrites, so rollback must restore the rules too.
	_, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{source.ID})
	var invalid *domain.InvalidWorkInfoError
	if !errors.As(err, &invalid) {
		t.Fatalf("Merge error = %v, want the alias limit", err)
	}
	if got := smartRuleJSON(t, db.sql, folder.ID); got != original {
		t.Errorf("rule after the refused merge = %s, want %s", got, original)
	}
}

func TestDeletingAReferenceRewritesNoRule(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a := createTagID(t, db, "a")
	person := createPerson(t, db, "P")
	folder := createRuleFolder(t, db, "A", domain.SmartRule{Tags: []int64{a}, Person: person.ID})
	original := smartRuleJSON(t, db.sql, folder.ID)

	if err := db.Tags().DeleteTag(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.Credits().Delete(ctx, domain.CreditPerson, person.ID); err != nil {
		t.Fatal(err)
	}
	if got := smartRuleJSON(t, db.sql, folder.ID); got != original {
		t.Errorf("rule after the deletes = %s, want %s", got, original)
	}
}
