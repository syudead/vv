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
		for _, operation := range row.Operations() {
			operations[operation] = true
		}
	}
	// 80 shipped rows on 74 operations, plus 8 schema rows, 2 fetch-apply rows and the thumbnail edit on 5.
	if len(seen) != 91 || len(operations) != 89 {
		t.Errorf("行 %d / 操作 %d, want 91 / 89", len(seen), len(operations))
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
