package domain

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
)

type OnboardingStep string

const (
	OnboardingFolders OnboardingStep = "folders"

	OnboardingScan OnboardingStep = "scan"

	OnboardingInterests OnboardingStep = "interests"

	OnboardingDone OnboardingStep = "done"
)

func (s OnboardingStep) Valid() bool {
	switch s {
	case OnboardingFolders, OnboardingScan, OnboardingInterests, OnboardingDone:
		return true
	}
	return false
}

func DefaultOnboardingStep(hasMediaFolder bool) OnboardingStep {
	if hasMediaFolder {
		return OnboardingDone
	}
	return OnboardingFolders
}

type Interest struct {
	ID string

	Name    string
	Synonym string
}

const InterestTagKind = "Genre"

//go:embed interest_catalog.json
var interestCatalogJSON []byte

var InterestCatalog = mustInterestCatalog(interestCatalogJSON)

func mustInterestCatalog(data []byte) []Interest {
	var entries []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Synonym string `json:"synonym"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		panic(fmt.Sprintf("the built-in interest catalog is broken: %v", err))
	}
	catalog := make([]Interest, len(entries))
	for i, entry := range entries {
		catalog[i] = Interest{ID: entry.ID, Name: entry.Name, Synonym: entry.Synonym}
	}
	return catalog
}

var ErrInvalidInterestPick = errors.New("invalid interest pick")

func PickInterests(ids []string) ([]Interest, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: no interest", ErrInvalidInterestPick)
	}
	picked := make([]Interest, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return nil, fmt.Errorf("%w: interest %q twice", ErrInvalidInterestPick, id)
		}
		seen[id] = true
		interest, ok := interestByID(id)
		if !ok {
			return nil, fmt.Errorf("%w: unknown interest %q", ErrInvalidInterestPick, id)
		}
		picked = append(picked, interest)
	}
	return picked, nil
}

func interestByID(id string) (Interest, bool) {
	for _, interest := range InterestCatalog {
		if interest.ID == id {
			return interest, true
		}
	}
	return Interest{}, false
}

type InterestStatus struct {
	Interest
	HasFolder bool
}

type InterestPickResult struct {
	Folders []SmartFolder

	Queued int
}

// OnboardingChanged signals a setup progress change to owner clients.
type OnboardingChanged struct{}

func (OnboardingChanged) event() {}
