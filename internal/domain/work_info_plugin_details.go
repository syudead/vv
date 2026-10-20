package domain

import (
	"encoding/json"
	"slices"
)

// Trim and drop empty/unknown detail entries before enforcing limits; malformed fields are ignored under contract 1, while semantic validation is deferred to apply.

// ImageURL and Profile keep the plugin's values without the apply's validation; Image is the header-checked face
// VVMDM fetched from ImageURL.
type FetchedPerson struct {
	Role        PersonRole
	Name        string
	ExternalIDs []ExternalID
	ImageURL    string
	Profile     PersonProfile
	Image       *WorkInfoCover
}

type FetchedOrganization struct {
	Role        OrganizationRole
	Name        string
	ExternalIDs []ExternalID
}

// Keep raw JSON so malformed fields can be discarded independently.
type workDetailsJSON struct {
	Code          json.RawMessage `json:"code"`
	Description   json.RawMessage `json:"description"`
	Dates         json.RawMessage `json:"dates"`
	People        json.RawMessage `json:"people"`
	Organizations json.RawMessage `json:"organizations"`
	URLs          json.RawMessage `json:"urls"`
	ExternalIDs   json.RawMessage `json:"externalIds"`
	Covers        json.RawMessage `json:"covers"`
}

type fetchedDateJSON struct {
	Kind string `json:"kind"`
	Date string `json:"date"`
}

type fetchedCreditJSON struct {
	Role        string          `json:"role"`
	Name        string          `json:"name"`
	ExternalIDs json.RawMessage `json:"externalIds"`
	Image       json.RawMessage `json:"image"`
	Profile     json.RawMessage `json:"profile"`
}

type fetchedExternalIDJSON struct {
	Source string `json:"source"`
	Value  string `json:"value"`
}

func parseWorkDetails(raw workDetailsJSON, out *FetchedWorkInfo) {
	limits := WorkInfoLimits
	out.Code = lenientString(raw.Code)
	out.Description = lenientString(raw.Description)
	out.Dates = parseFetchedDates(lenientList[fetchedDateJSON](raw.Dates))
	out.People = parseFetchedCredits(lenientList[fetchedCreditJSON](raw.People), PersonRoles,
		limits.PeoplePerRoleMaxCount, func(role PersonRole, name string, ids []ExternalID, raw fetchedCreditJSON) FetchedPerson {
			return FetchedPerson{
				Role: role, Name: name, ExternalIDs: ids,
				ImageURL: fetchedImageURL(lenientString(raw.Image)), Profile: parseFetchedProfile(raw.Profile),
			}
		})
	// Organizations take neither an image nor a profile.
	out.Organizations = parseFetchedCredits(lenientList[fetchedCreditJSON](raw.Organizations), OrganizationRoles,
		limits.OrganizationsPerRoleMaxCount, func(role OrganizationRole, name string, ids []ExternalID, _ fetchedCreditJSON) FetchedOrganization {
			return FetchedOrganization{Role: role, Name: name, ExternalIDs: ids}
		})
	out.URLs = parseFetchedURLs(lenientList[string](raw.URLs), limits.URLsMaxCount)
	out.ExternalIDs = parseFetchedExternalIDs(raw.ExternalIDs)
	out.CoverURLs = parseFetchedImageURLs(raw.Covers, WorkInfoCoversMaxCount)
}

// Missing/nonstring values become empty strings.
func lenientString(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return trimWorkInfoText(s)
}

// Keep decodable array entries in order; missing/nonarray input yields nil.
func lenientList[T any](raw json.RawMessage) []T {
	var items []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []T
	for _, item := range items {
		var v T
		if json.Unmarshal(item, &v) == nil {
			out = append(out, v)
		}
	}
	return out
}

// Keep the first date per kind.
func parseFetchedDates(raw []fetchedDateJSON) []WorkDate {
	var out []WorkDate
	for _, d := range raw {
		kind := DateKind(trimWorkInfoText(d.Kind))
		date := trimWorkInfoText(d.Date)
		if !kind.Valid() || date == "" ||
			slices.ContainsFunc(out, func(kept WorkDate) bool { return kept.Kind == kind }) {
			continue
		}
		out = append(out, WorkDate{Kind: kind, Date: date})
	}
	return out
}

// Deduplicate credits per role by name and external-ID set, ignoring external-ID order, then enforce per-role limits.
func parseFetchedCredits[R ~string, C any](
	raw []fetchedCreditJSON, roles []R, limit int, build func(R, string, []ExternalID, fetchedCreditJSON) C,
) []C {
	type key struct {
		role R
		name string
		ids  string
	}
	var out []C
	seen := map[key]bool{}
	counts := map[R]int{}
	for _, c := range raw {
		role := R(trimWorkInfoText(c.Role))
		name := trimWorkInfoText(c.Name)
		if !slices.Contains(roles, role) || name == "" || counts[role] == limit {
			continue
		}
		ids := parseFetchedExternalIDs(c.ExternalIDs)
		// Sort a copy for identity while preserving result order.
		sortedIDs := slices.Clone(ids)
		SortExternalIDs(sortedIDs)
		idsKey, _ := json.Marshal(sortedIDs)
		k := key{role, name, string(idsKey)}
		if seen[k] {
			continue
		}
		seen[k] = true
		counts[role]++
		out = append(out, build(role, name, ids, c))
	}
	return out
}

func parseFetchedURLs(raw []string, limit int) []string {
	var out []string
	for _, u := range raw {
		u = trimWorkInfoText(u)
		if u == "" || slices.Contains(out, u) {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, u)
	}
	return out
}

// Drop partially blank external IDs and retain unique pairs up to the limit.
func parseFetchedExternalIDs(raw json.RawMessage) []ExternalID {
	return appendFetchedExternalIDs(nil, raw)
}

// appendFetchedExternalIDs appends raw's IDs that out lacks until out holds ExternalIDsMaxCount.
func appendFetchedExternalIDs(out []ExternalID, raw json.RawMessage) []ExternalID {
	for _, e := range lenientList[fetchedExternalIDJSON](raw) {
		id := ExternalID{Source: trimWorkInfoText(e.Source), Value: trimWorkInfoText(e.Value)}
		if id.Source == "" || id.Value == "" || slices.Contains(out, id) {
			continue
		}
		if len(out) == ExternalIDsMaxCount {
			break
		}
		out = append(out, id)
	}
	return out
}
