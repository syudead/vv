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
	for field, at := range fetchedProfileInts {
		var n int
		if value, ok := properties[string(field)]; ok && json.Unmarshal(value, &n) == nil {
			*at(&out) = &n
		}
	}
	for _, link := range lenientList[string](properties[string(ProfileLinks)]) {
		if link = trimWorkInfoText(link); link != "" {
			out.Links = append(out.Links, link)
		}
	}
	return out
}

// FillFetchedPersonProfile fills each empty field of target with the fetched person's value and reports, under the
// person's role field, each value that would make the profile invalid.
func FillFetchedPersonProfile(target PersonProfile, fetched FetchedPerson, today time.Time) (PersonProfile, []SkippedWorkInfoValue) {
	out := target
	var skipped []SkippedWorkInfoValue
	for _, field := range PersonProfileFields {
		next, fill := fillProfileField(out, fetched.Profile, field, today)
		switch fill {
		case profileFilled:
			out = next
		case profileFillInvalid:
			skipped = append(skipped, SkippedWorkInfoValue{
				Field:  PersonField(fetched.Role),
				Value:  fetched.Name + ": " + string(field) + " " + profileFieldText(fetched.Profile, field),
				Reason: SkippedWorkInfoInvalid,
			})
		}
	}
	return out, skipped
}

// ShownFetchedProfile keeps, normalized, each fetched value that is valid with the ones before it, for the
// comparison; the apply reports the others as skipped.
func ShownFetchedProfile(p PersonProfile, today time.Time) PersonProfile {
	var out PersonProfile
	for _, field := range PersonProfileFields {
		if next, fill := fillProfileField(out, p, field, today); fill == profileFilled {
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
