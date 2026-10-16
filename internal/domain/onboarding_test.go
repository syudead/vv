package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestOnboardingStepValid(t *testing.T) {
	for _, step := range []OnboardingStep{OnboardingFolders, OnboardingScan, OnboardingInterests, OnboardingDone} {
		if !step.Valid() {
			t.Errorf("%q is not valid", step)
		}
	}
	for _, step := range []OnboardingStep{"", "Done", "later"} {
		if step.Valid() {
			t.Errorf("%q is valid", step)
		}
	}
	if DefaultOnboardingStep(true) != OnboardingDone || DefaultOnboardingStep(false) != OnboardingFolders {
		t.Error("the default step does not follow the media folders")
	}
}

// 一覧の id は重ならず、名前と同義語はタグの名前の規則で整えた形のままである。
func TestInterestCatalogNamesAreTagNames(t *testing.T) {
	if len(InterestCatalog) != 24 {
		t.Fatalf("catalog = %d interests, want 24", len(InterestCatalog))
	}
	seen := map[string]bool{}
	for _, interest := range InterestCatalog {
		if interest.ID == "" || seen[interest.ID] {
			t.Errorf("interest id %q is empty or repeated", interest.ID)
		}
		seen[interest.ID] = true
		for _, name := range []string{interest.Name, interest.Synonym} {
			if normalized, err := NormalizeTagName(name); err != nil || normalized != name {
				t.Errorf("NormalizeTagName(%q) = %q, %v", name, normalized, err)
			}
		}
	}
}

func TestPickInterests(t *testing.T) {
	got, err := PickInterests([]string{"music", "anime"})
	want := []Interest{InterestCatalog[5], InterestCatalog[0]}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("PickInterests = %+v, %v; want %+v", got, err, want)
	}
	for _, ids := range [][]string{nil, {}, {"anime", "anime"}, {"anime", "opera"}} {
		if _, err := PickInterests(ids); !errors.Is(err, ErrInvalidInterestPick) {
			t.Errorf("PickInterests(%q) error = %v, want ErrInvalidInterestPick", ids, err)
		}
	}
}
