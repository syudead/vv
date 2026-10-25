package domain

import "testing"

func TestAssistantCatalogRowIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	operations := map[string]bool{}
	for _, row := range AssistantCatalog() {
		if seen[row.ID] {
			t.Errorf("行 ID %s が重複している", row.ID)
		}
		seen[row.ID] = true
		operations[row.Operation] = true
	}
	// The contract counts 74 operations in 80 rows.
	if len(seen) != 80 || len(operations) != 74 {
		t.Errorf("行 %d / 操作 %d, want 80 / 74", len(seen), len(operations))
	}
}

func TestAssistantCatalogVariantsCarryActionAndMark(t *testing.T) {
	cases := []struct {
		id, operation, action string
		irreversible          bool
	}{
		{"batchTags.reject", "batchTags", "reject", true},
		{"batchTags.confirm", "batchTags", "confirm", false},
		{"updateVideoTags.remove", "updateVideoTags", "remove", false},
		{"renameTag", "renameTag", "", false},
	}
	for _, tc := range cases {
		row, ok := LookupAssistantAction(tc.id)
		if !ok {
			t.Fatalf("%s がカタログに無い", tc.id)
		}
		if row.Operation != tc.operation || row.Action != tc.action || row.Irreversible != tc.irreversible {
			t.Errorf("%s = (%s, %q, 戻せない %v), want (%s, %q, %v)",
				tc.id, row.Operation, row.Action, row.Irreversible, tc.operation, tc.action, tc.irreversible)
		}
	}
	if _, ok := LookupAssistantAction("batchTags"); ok {
		t.Error("アクションを選ぶ操作が変種なしの ID で引けてしまう")
	}
}
