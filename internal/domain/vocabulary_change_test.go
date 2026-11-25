package domain

import "testing"

// Only a person can be unnamed, since an organization's empty list is refused.
func TestUnnameIsValidForANamedPersonOnly(t *testing.T) {
	cases := []struct {
		name   string
		change VocabularyChange
		want   bool
	}{
		{"person", VocabularyChange{Kind: VocabularyPerson, Action: VocabularyUnname, Name: "佐藤"}, true},
		{"organization", VocabularyChange{Kind: VocabularyOrganization, Action: VocabularyUnname, Name: "スタジオ"}, false},
		{"tag", VocabularyChange{Kind: VocabularyTag, Action: VocabularyUnname, Name: "猫"}, false},
		{"no name", VocabularyChange{Kind: VocabularyPerson, Action: VocabularyUnname}, false},
		{"new name", VocabularyChange{Kind: VocabularyPerson, Action: VocabularyUnname, Name: "佐藤", NewName: "鈴木"}, false},
	}
	for _, tc := range cases {
		if got := tc.change.Valid(); got != tc.want {
			t.Errorf("%s: Valid() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
