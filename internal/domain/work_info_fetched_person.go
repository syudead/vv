package domain

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// A fetched person's profile keeps each property of the right JSON type; the apply validates the values
// (docs/design-docs/work-info-plugins.md).

var fetchedProfileTexts = map[PersonProfileField]func(*PersonProfile) *string{
	ProfileGender:     func(p *PersonProfile) *string { return &p.Gender },
	ProfileBirthDate:  func(p *PersonProfile) *string { return &p.BirthDate },
	ProfileDeathDate:  func(p *PersonProfile) *string { return &p.DeathDate },
	ProfileBirthplace: func(p *PersonProfile) *string { return &p.Birthplace },
	ProfileCup:        func(p *PersonProfile) *string { return &p.Cup },
	ProfileBloodType:  func(p *PersonProfile) *string { return &p.BloodType },
	ProfileBio:        func(p *PersonProfile) *string { return &p.Bio },
}

var fetchedProfileInts = map[PersonProfileField]func(*PersonProfile) **int{
	ProfileHeightCm:        func(p *PersonProfile) **int { return &p.HeightCm },
	ProfileBustCm:          func(p *PersonProfile) **int { return &p.BustCm },
	ProfileWaistCm:         func(p *PersonProfile) **int { return &p.WaistCm },
	ProfileHipsCm:          func(p *PersonProfile) **int { return &p.HipsCm },
	ProfileCareerStartYear: func(p *PersonProfile) **int { return &p.CareerStartYear },
	ProfileCareerEndYear:   func(p *PersonProfile) **int { return &p.CareerEndYear },
}

// parseFetchedProfile drops a property that is missing or of another JSON type and keeps the rest; texts and links
// are trimmed, and blank links are dropped.
func parseFetchedProfile(raw json.RawMessage) PersonProfile {
	var properties map[string]json.RawMessage
	var out PersonProfile
	if len(raw) == 0 || json.Unmarshal(raw, &properties) != nil {
		return out
	}
	for field, at := range fetchedProfileTexts {
		*at(&out) = lenientString(properties[string(field)])
	}
	out.Bio = fetchedMultiline(rawString(properties[string(ProfileBio)]), PersonBioMaxLength)
	for field, at := range fetchedProfileInts {
		var n int
		if value, ok := properties[string(field)]; ok && json.Unmarshal(value, &n) == nil {
			*at(&out) = &n
		}
	}
	out.Links = fetchedProfileLinks(properties[string(ProfileLinks)])
	return out
}

// FillFetchedPersonProfile fills each empty field of target with the fetched person's value and reports, under the
// person's role field, each value that would make the profile invalid.
func FillFetchedPersonProfile(target PersonProfile, fetched FetchedPerson, today time.Time) (PersonProfile, []SkippedWorkInfoValue) {
	return fillFetchedProfile(personProfiles, target, fetched.Profile, today, func(field PersonProfileField) SkippedWorkInfoValue {
		return SkippedWorkInfoValue{
			Field:  PersonField(fetched.Role),
			Value:  fetched.Name + ": " + string(field) + " " + profileFieldText(fetched.Profile, field),
			Reason: SkippedWorkInfoInvalid,
		}
	})
}

// FillFetchedOrganizationProfile is FillFetchedPersonProfile for an organization, reported under its role field.
func FillFetchedOrganizationProfile(
	target OrganizationProfile, fetched FetchedOrganization, today time.Time,
) (OrganizationProfile, []SkippedWorkInfoValue) {
	return fillFetchedProfile(organizationProfiles, target, fetched.Profile, today,
		func(field OrganizationProfileField) SkippedWorkInfoValue {
			return SkippedWorkInfoValue{
				Field:  OrganizationField(fetched.Role),
				Value:  fetched.Name + ": " + string(field) + " " + organizationProfileFieldText(fetched.Profile, field),
				Reason: SkippedWorkInfoInvalid,
			}
		})
}

func fillFetchedProfile[P any, F comparable](
	schema profileSchema[P, F], target, fetched P, today time.Time, skip func(F) SkippedWorkInfoValue,
) (P, []SkippedWorkInfoValue) {
	out := target
	var skipped []SkippedWorkInfoValue
	for _, field := range schema.fields {
		next, fill := schema.fillField(out, fetched, field, today)
		switch fill {
		case profileFilled:
			out = next
		case profileFillInvalid:
			skipped = append(skipped, skip(field))
		}
	}
	return out, skipped
}

func organizationProfileFieldText(p OrganizationProfile, field OrganizationProfileField) string {
	if field == OrganizationLinks {
		return strings.Join(p.Links, " ")
	}
	var only OrganizationProfile
	organizationProfiles.copies[field](&only, p)
	return TruncateRunes(only.FoundedDate+only.DissolvedDate+only.Location+only.Bio, SkippedDescriptionMaxRunes)
}

// parseFetchedOrganizationProfile reads an organization's profile as parseFetchedProfile reads a person's.
func parseFetchedOrganizationProfile(raw json.RawMessage) OrganizationProfile {
	var properties map[string]json.RawMessage
	var out OrganizationProfile
	if len(raw) == 0 || json.Unmarshal(raw, &properties) != nil {
		return out
	}
	out.FoundedDate = lenientString(properties[string(OrganizationFoundedDate)])
	out.DissolvedDate = lenientString(properties[string(OrganizationDissolvedDate)])
	out.Location = lenientString(properties[string(OrganizationLocation)])
	out.Bio = fetchedMultiline(rawString(properties[string(OrganizationBio)]), OrganizationBioMaxLength)
	out.Links = fetchedProfileLinks(properties[string(OrganizationLinks)])
	return out
}

// fetchedProfileLinks trims a fetched list of links and drops blank ones.
func fetchedProfileLinks(raw json.RawMessage) []string {
	var out []string
	for _, link := range lenientList[string](raw) {
		if link = fetchedText(link); link != "" {
			out = append(out, link)
		}
	}
	return out
}

// ShownFetchedProfile keeps, normalized, each fetched value that is valid with the ones before it, for the
// comparison; the apply reports the others as skipped.
func ShownFetchedProfile(p PersonProfile, today time.Time) PersonProfile {
	return shownFetchedProfile(personProfiles, p, today)
}

// ShownFetchedOrganizationProfile is ShownFetchedProfile for an organization.
func ShownFetchedOrganizationProfile(p OrganizationProfile, today time.Time) OrganizationProfile {
	return shownFetchedProfile(organizationProfiles, p, today)
}

func shownFetchedProfile[P any, F comparable](schema profileSchema[P, F], p P, today time.Time) P {
	var out P
	for _, field := range schema.fields {
		if next, fill := schema.fillField(out, p, field, today); fill == profileFilled {
			out = next
		}
	}
	return out
}

func profileFieldText(p PersonProfile, field PersonProfileField) string {
	if at, ok := fetchedProfileTexts[field]; ok {
		return TruncateRunes(*at(&p), SkippedDescriptionMaxRunes)
	}
	if at, ok := fetchedProfileInts[field]; ok && *at(&p) != nil {
		return strconv.Itoa(**at(&p))
	}
	return strings.Join(p.Links, " ")
}
