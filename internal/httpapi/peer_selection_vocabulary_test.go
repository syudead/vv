package httpapi

import (
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestTranslateEditCreatesAPersonNamedInSeveralCreditsOnce(t *testing.T) {
	const otherID, createdID = 7, 42
	var creates []string
	v := editVocabulary{
		own:  func(int64) (int64, bool) { return 0, false },
		name: func(domain.VocabularyKind, int64) (string, bool, error) { return "Alice", true, nil },
		id:   func(domain.VocabularyKind, string) (int64, error) { return 0, nil },
		create: func(kind domain.CreditKind, name string) (int64, error) {
			creates = append(creates, string(kind)+":"+name)
			return createdID, nil
		},
	}
	edit := domain.VideoEdit{Credits: []domain.CreditEdit{
		{Kind: domain.CreditPerson, Role: "performer", Add: []int64{otherID}},
		{Kind: domain.CreditPerson, Role: "director", Add: []int64{otherID}},
	}}

	got, err := translateEdit(edit, v)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(creates, []string{"person:Alice"}) {
		t.Errorf("creates = %v, want one person:Alice", creates)
	}
	if len(got.Credits) != 2 || !slices.Equal(got.Credits[0].Add, []int64{createdID}) || !slices.Equal(got.Credits[1].Add, []int64{createdID}) {
		t.Errorf("credits = %+v, want both roles adding %d", got.Credits, createdID)
	}
}
