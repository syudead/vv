package main

import "github.com/syudead/vv/internal/domain"

// A person plugin's run prints its result with the contract's field names after VVMDM's limits, like a work-info
// plugin's.

type keptPersonCandidate struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type keptAlias struct {
	Name     string `json:"name"`
	Language string `json:"language,omitempty"`
}

type keptProfile struct {
	Gender          string   `json:"gender,omitempty"`
	BirthDate       string   `json:"birthDate,omitempty"`
	DeathDate       string   `json:"deathDate,omitempty"`
	Birthplace      string   `json:"birthplace,omitempty"`
	HeightCm        *int     `json:"heightCm,omitempty"`
	BustCm          *int     `json:"bustCm,omitempty"`
	WaistCm         *int     `json:"waistCm,omitempty"`
	HipsCm          *int     `json:"hipsCm,omitempty"`
	Cup             string   `json:"cup,omitempty"`
	BloodType       string   `json:"bloodType,omitempty"`
	CareerStartYear *int     `json:"careerStartYear,omitempty"`
	CareerEndYear   *int     `json:"careerEndYear,omitempty"`
	Bio             string   `json:"bio,omitempty"`
	Links           []string `json:"links,omitempty"`
}

type keptPerson struct {
	Name        string           `json:"name"`
	Aliases     []keptAlias      `json:"aliases,omitempty"`
	ExternalIDs []keptExternalID `json:"externalIds"`
	Image       string           `json:"image,omitempty"`
	// Profile is omitted when the plugin gave no property.
	Profile *keptProfile `json:"profile,omitempty"`
}

func keptPersonCandidates(candidates []domain.PersonCandidate) any {
	out := make([]keptPersonCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, keptPersonCandidate{Source: c.Source, ID: c.ID, Name: c.Name, URL: c.URL, Detail: c.Detail})
	}
	return struct {
		Candidates []keptPersonCandidate `json:"candidates"`
	}{out}
}

func toKeptPerson(p domain.FetchedPluginPerson) keptPerson {
	out := keptPerson{Name: p.Name, ExternalIDs: toKeptExternalIDs(p.ExternalIDs), Image: p.ImageURL}
	for _, a := range p.Aliases {
		out.Aliases = append(out.Aliases, keptAlias{Name: a.Name, Language: a.Language})
	}
	if !p.Profile.Equal(domain.PersonProfile{}) {
		profile := keptProfile(p.Profile)
		out.Profile = &profile
	}
	return out
}
