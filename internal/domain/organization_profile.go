package domain

import (
	"slices"
	"time"
)

// Organization profile limits; the lasting rules are in docs/design-docs/people.md.
const (
	OrganizationLocationMaxLength = 200
	OrganizationBioMaxLength      = PersonBioMaxLength
	OrganizationLinksMaxCount     = PersonLinksMaxCount
)

// OrganizationProfile uses "" for unknown values; normalized Links is nil when empty.
type OrganizationProfile struct {
	FoundedDate   string
	DissolvedDate string
	Location      string
	Bio           string
	Links         []string
}

// Equal compares values, treating nil and empty link lists alike.
func (p OrganizationProfile) Equal(other OrganizationProfile) bool {
	return p.FoundedDate == other.FoundedDate && p.DissolvedDate == other.DissolvedDate &&
		p.Location == other.Location && p.Bio == other.Bio && slices.Equal(p.Links, other.Links)
}

// OrganizationProfileField names a profile property as the APIs spell it.
type OrganizationProfileField string

const (
	OrganizationFoundedDate   OrganizationProfileField = "foundedDate"
	OrganizationDissolvedDate OrganizationProfileField = "dissolvedDate"
	OrganizationLocation      OrganizationProfileField = "location"
	OrganizationBio           OrganizationProfileField = "bio"
	OrganizationLinks         OrganizationProfileField = "links"
)

var OrganizationProfileFields = []OrganizationProfileField{
	OrganizationFoundedDate, OrganizationDissolvedDate, OrganizationLocation, OrganizationBio, OrganizationLinks,
}

var organizationProfiles = profileSchema[OrganizationProfile, OrganizationProfileField]{
	fields: OrganizationProfileFields,
	copies: map[OrganizationProfileField]func(dst *OrganizationProfile, src OrganizationProfile){
		OrganizationFoundedDate:   func(d *OrganizationProfile, s OrganizationProfile) { d.FoundedDate = s.FoundedDate },
		OrganizationDissolvedDate: func(d *OrganizationProfile, s OrganizationProfile) { d.DissolvedDate = s.DissolvedDate },
		OrganizationLocation:      func(d *OrganizationProfile, s OrganizationProfile) { d.Location = s.Location },
		OrganizationBio:           func(d *OrganizationProfile, s OrganizationProfile) { d.Bio = s.Bio },
		OrganizationLinks:         func(d *OrganizationProfile, s OrganizationProfile) { d.Links = s.Links },
	},
	equal:     OrganizationProfile.Equal,
	normalize: NormalizeOrganizationProfile,
}

// OrganizationProfilePatch replaces each field in Fields with its value in Values; an empty value clears the field.
type OrganizationProfilePatch struct {
	Fields []OrganizationProfileField
	Values OrganizationProfile
}

func (p OrganizationProfilePatch) Apply(current OrganizationProfile) OrganizationProfile {
	return organizationProfiles.apply(current, p.Fields, p.Values)
}

// FillOrganizationProfile fills each empty target field from the first source whose value keeps the profile valid.
func FillOrganizationProfile(target OrganizationProfile, sources []OrganizationProfile, today time.Time) OrganizationProfile {
	return organizationProfiles.fill(target, sources, today)
}

func organizationProfileField(name OrganizationProfileField) string {
	return "organizationProfile." + string(name)
}

// NormalizeOrganizationProfile trims text, clears empty values and returns the first InvalidWorkInfoError with an
// organizationProfile.<name> field; a dissolution may not precede the founding.
func NormalizeOrganizationProfile(input OrganizationProfile, today time.Time) (OrganizationProfile, error) {
	out := input
	var err error
	if out.Location, err = normalizeWorkInfoText(organizationProfileField(OrganizationLocation), out.Location,
		OrganizationLocationMaxLength, false); err != nil {
		return OrganizationProfile{}, err
	}
	if out.Bio, err = normalizeWorkInfoText(organizationProfileField(OrganizationBio), out.Bio,
		OrganizationBioMaxLength, true); err != nil {
		return OrganizationProfile{}, err
	}
	if out.FoundedDate, out.DissolvedDate, err = normalizeDateSpan(organizationProfileField(OrganizationFoundedDate),
		organizationProfileField(OrganizationDissolvedDate), out.FoundedDate, out.DissolvedDate, today); err != nil {
		return OrganizationProfile{}, err
	}
	if out.Links, err = normalizeURLList(organizationProfileField(OrganizationLinks), input.Links,
		OrganizationLinksMaxCount); err != nil {
		return OrganizationProfile{}, err
	}
	if len(out.Links) == 0 {
		out.Links = nil
	}
	return out, nil
}
