package domain

import (
	"encoding/json"
	"fmt"
	"slices"
)

// A person plugin shares contract 1 with work-info plugins and adds kind, sources, a fetch source and a person
// response (docs/design-docs/person-plugins.md).

type WorkInfoPluginKind string

const (
	WorkInfoPluginKindWork   WorkInfoPluginKind = "work"
	WorkInfoPluginKindPerson WorkInfoPluginKind = "person"
)

const (
	PersonPluginMaxSources = 10
	PersonPluginMaxDomains = 10
	// PersonPluginDomainMaxLength is the longest DNS host name.
	PersonPluginDomainMaxLength = 253
)

// DeclaresSource reports whether a person plugin's manifest lists source.
func (m WorkInfoPluginManifest) DeclaresSource(source string) bool {
	return slices.Contains(m.Sources, source)
}

// checkPluginKind runs after the shared checks; hasSources and hasDomains tell an explicit field, null included,
// apart from an absent one.
func checkPluginKind(m WorkInfoPluginManifest, hasSources, hasDomains bool) *WorkInfoPluginManifestError {
	switch m.Kind {
	case WorkInfoPluginKindWork:
		if hasSources {
			return manifestInvalid("sources", "is allowed only when kind is person")
		}
		if !hasDomains {
			return nil
		}
		if !m.Supports(WorkInfoFetch) {
			return manifestInvalid("operations", "must list fetch for a plugin that declares domains")
		}
		return checkPluginDomains(m.Domains)
	case WorkInfoPluginKindPerson:
	default:
		return manifestInvalid("kind", "must be work or person")
	}
	if len(m.Operations) != 2 {
		return manifestInvalid("operations", "must list both search and fetch for a person plugin")
	}
	if len(m.Sources) == 0 || len(m.Sources) > PersonPluginMaxSources {
		return manifestInvalid("sources", fmt.Sprintf("must list 1 to %d sources", PersonPluginMaxSources))
	}
	for i, source := range m.Sources {
		normalized, err := normalizeWorkInfoText("", source, ExternalIDSourceMaxLength, false)
		if err != nil || normalized == "" || normalized != source {
			return manifestInvalid(fmt.Sprintf("sources[%d]", i),
				fmt.Sprintf("must be 1 to %d characters without surrounding spaces or control characters",
					ExternalIDSourceMaxLength))
		}
		if slices.Index(m.Sources, source) != i {
			return manifestInvalid(fmt.Sprintf("sources[%d]", i), "must differ from the other sources")
		}
	}
	if hasDomains {
		return checkPluginDomains(m.Domains)
	}
	return nil
}

// checkPluginDomains accepts bare lowercase host names, so a scheme, port or path never passes.
func checkPluginDomains(domains []string) *WorkInfoPluginManifestError {
	if len(domains) == 0 || len(domains) > PersonPluginMaxDomains {
		return manifestInvalid("domains", fmt.Sprintf("must list 1 to %d domains", PersonPluginMaxDomains))
	}
	for i, domain := range domains {
		if !isPluginDomain(domain) {
			return manifestInvalid(fmt.Sprintf("domains[%d]", i), fmt.Sprintf(
				"must be a host name of 1 to %d characters of a-z, 0-9, - and .", PersonPluginDomainMaxLength))
		}
		if slices.Index(domains, domain) != i {
			return manifestInvalid(fmt.Sprintf("domains[%d]", i), "must differ from the other domains")
		}
	}
	return nil
}

func isPluginDomain(s string) bool {
	if s == "" || len(s) > PersonPluginDomainMaxLength {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

// PersonCandidate is a person plugin's search result: an external ID with a name; URL is a display link only.
type PersonCandidate struct {
	Source string
	ID     string
	Name   string
	URL    string
	Detail string
}

// FetchedPluginPerson keeps the plugin's values without the apply's validation; ExternalIDs starts with the
// request's (source, id) unless the fetch went by link.
type FetchedPluginPerson struct {
	Name        string
	Aliases     []Alias
	ExternalIDs []ExternalID
	// ImageURLs are the first PersonPluginImagesMaxCount distinct valid image URLs in result order.
	ImageURLs []string
	Profile   PersonProfile
}

type personResponseJSON struct {
	Candidates []struct {
		Source string `json:"source"`
		ID     string `json:"id"`
		Name   string `json:"name"`
		URL    string `json:"url"`
		Detail string `json:"detail"`
	} `json:"candidates"`
	Person *struct {
		Name        json.RawMessage `json:"name"`
		Aliases     json.RawMessage `json:"aliases"`
		ExternalIDs json.RawMessage `json:"externalIds"`
		Images      json.RawMessage `json:"images"`
		Profile     json.RawMessage `json:"profile"`
	} `json:"person"`
	Error *runErrorJSON `json:"error"`
}

type fetchedAliasJSON struct {
	Name     string `json:"name"`
	Language string `json:"language"`
}

func parsePersonPluginResponse(req WorkInfoRunRequest, output []byte) (WorkInfoRunResult, error) {
	var raw personResponseJSON
	if err := decodeRunResponse(output, &raw, func() *runErrorJSON { return raw.Error }); err != nil {
		return WorkInfoRunResult{}, err
	}
	switch {
	case req.Operation == WorkInfoSearch && raw.Candidates != nil:
		return parsePersonCandidates(raw, req.Sources)
	case req.Operation == WorkInfoFetch && raw.Person != nil:
		name := lenientString(raw.Person.Name)
		if name == "" {
			break
		}
		var ids []ExternalID
		if req.URL == "" {
			ids = []ExternalID{{Source: req.Source, Value: req.ID}}
		}
		ids = appendFetchedExternalIDs(ids, raw.Person.ExternalIDs)
		return WorkInfoRunResult{Person: FetchedPluginPerson{
			Name:        name,
			Aliases:     parseFetchedAliases(raw.Person.Aliases),
			ExternalIDs: ids,
			ImageURLs:   parseFetchedImageURLs(raw.Person.Images, PersonPluginImagesMaxCount),
			Profile:     parseFetchedProfile(raw.Person.Profile),
		}}, nil
	}
	return WorkInfoRunResult{}, &WorkInfoRunError{Failure: WorkInfoRunFailed}
}

// Candidates beyond the retained limit are ignored; a retained one needs a declared source, a fetchable ID, a name and
// an absolute HTTP(S) link.
func parsePersonCandidates(raw personResponseJSON, sources []string) (WorkInfoRunResult, error) {
	kept := raw.Candidates[:min(len(raw.Candidates), WorkInfoPluginMaxCandidates)]
	candidates := make([]PersonCandidate, 0, len(kept))
	for _, c := range kept {
		id, err := NormalizeWorkInfoSearchQuery(c.ID)
		name := trimWorkInfoText(c.Name)
		if !slices.Contains(sources, c.Source) || err != nil || name == "" || !isWorkURL(c.URL) {
			return WorkInfoRunResult{}, &WorkInfoRunError{Failure: WorkInfoRunFailed}
		}
		candidates = append(candidates, PersonCandidate{Source: c.Source, ID: id, Name: name, URL: c.URL, Detail: c.Detail})
	}
	return WorkInfoRunResult{PersonCandidates: candidates}, nil
}

// parseFetchedImageURLs drops invalid and repeated URLs and keeps the first limit.
func parseFetchedImageURLs(raw json.RawMessage, limit int) []string {
	var out []string
	for _, item := range lenientList[string](raw) {
		if len(out) == limit {
			break
		}
		if url := fetchedImageURL(trimWorkInfoText(item)); url != "" && !slices.Contains(out, url) {
			out = append(out, url)
		}
	}
	return out
}

// parseFetchedAliases trims names, drops nameless and repeated names, and keeps the first CreditAliasesMaxCount;
// languages are validated by the apply.
func parseFetchedAliases(raw json.RawMessage) []Alias {
	var out []Alias
	for _, a := range lenientList[fetchedAliasJSON](raw) {
		if len(out) == CreditAliasesMaxCount {
			break
		}
		name := trimWorkInfoText(a.Name)
		if name == "" || slices.ContainsFunc(out, func(kept Alias) bool { return kept.Name == name }) {
			continue
		}
		out = append(out, Alias{Name: name, Language: trimWorkInfoText(a.Language)})
	}
	return out
}
