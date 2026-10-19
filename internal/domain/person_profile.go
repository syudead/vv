package domain

import (
	"slices"
	"strings"
	"time"
)

// Profile limits; the lasting rules are in docs/design-docs/people.md.
const (
	PersonBirthplaceMaxLength = 200
	PersonBioMaxLength        = 5_000
	PersonCupMaxLength        = 3
	PersonLinksMaxCount       = 20
	PersonHeightMinCm         = 50
	PersonHeightMaxCm         = 300
	PersonMeasurementMinCm    = 30
	PersonMeasurementMaxCm    = 300
	PersonCareerMinYear       = 1900
	PersonCareerMaxYear       = 9999
)

var (
	PersonGenders    = []string{"female", "male", "other"}
	PersonBloodTypes = []string{"A", "B", "O", "AB"}
)

// PersonProfile uses "" and nil for unknown values; normalized Links is nil when empty.
type PersonProfile struct {
	Gender          string
	BirthDate       string
	DeathDate       string
	Birthplace      string
	HeightCm        *int
	BustCm          *int
	WaistCm         *int
	HipsCm          *int
	Cup             string
	BloodType       string
	CareerStartYear *int
	CareerEndYear   *int
	Bio             string
	Links           []string
}

// Equal compares values, treating nil and empty link lists alike.
func (p PersonProfile) Equal(other PersonProfile) bool {
	return p.Gender == other.Gender && p.BirthDate == other.BirthDate && p.DeathDate == other.DeathDate &&
		p.Birthplace == other.Birthplace && p.Cup == other.Cup && p.BloodType == other.BloodType && p.Bio == other.Bio &&
		equalOptionalInt(p.HeightCm, other.HeightCm) && equalOptionalInt(p.BustCm, other.BustCm) &&
		equalOptionalInt(p.WaistCm, other.WaistCm) && equalOptionalInt(p.HipsCm, other.HipsCm) &&
		equalOptionalInt(p.CareerStartYear, other.CareerStartYear) &&
		equalOptionalInt(p.CareerEndYear, other.CareerEndYear) && slices.Equal(p.Links, other.Links)
}

func equalOptionalInt(a, b *int) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// PersonProfileField names a profile property as the APIs spell it.
type PersonProfileField string

const (
	ProfileGender          PersonProfileField = "gender"
	ProfileBirthDate       PersonProfileField = "birthDate"
	ProfileDeathDate       PersonProfileField = "deathDate"
	ProfileBirthplace      PersonProfileField = "birthplace"
	ProfileHeightCm        PersonProfileField = "heightCm"
	ProfileBustCm          PersonProfileField = "bustCm"
	ProfileWaistCm         PersonProfileField = "waistCm"
	ProfileHipsCm          PersonProfileField = "hipsCm"
	ProfileCup             PersonProfileField = "cup"
	ProfileBloodType       PersonProfileField = "bloodType"
	ProfileCareerStartYear PersonProfileField = "careerStartYear"
	ProfileCareerEndYear   PersonProfileField = "careerEndYear"
	ProfileBio             PersonProfileField = "bio"
	ProfileLinks           PersonProfileField = "links"
)

var personProfileCopies = map[PersonProfileField]func(dst *PersonProfile, src PersonProfile){
	ProfileGender:          func(d *PersonProfile, s PersonProfile) { d.Gender = s.Gender },
	ProfileBirthDate:       func(d *PersonProfile, s PersonProfile) { d.BirthDate = s.BirthDate },
	ProfileDeathDate:       func(d *PersonProfile, s PersonProfile) { d.DeathDate = s.DeathDate },
	ProfileBirthplace:      func(d *PersonProfile, s PersonProfile) { d.Birthplace = s.Birthplace },
	ProfileHeightCm:        func(d *PersonProfile, s PersonProfile) { d.HeightCm = s.HeightCm },
	ProfileBustCm:          func(d *PersonProfile, s PersonProfile) { d.BustCm = s.BustCm },
	ProfileWaistCm:         func(d *PersonProfile, s PersonProfile) { d.WaistCm = s.WaistCm },
	ProfileHipsCm:          func(d *PersonProfile, s PersonProfile) { d.HipsCm = s.HipsCm },
	ProfileCup:             func(d *PersonProfile, s PersonProfile) { d.Cup = s.Cup },
	ProfileBloodType:       func(d *PersonProfile, s PersonProfile) { d.BloodType = s.BloodType },
	ProfileCareerStartYear: func(d *PersonProfile, s PersonProfile) { d.CareerStartYear = s.CareerStartYear },
	ProfileCareerEndYear:   func(d *PersonProfile, s PersonProfile) { d.CareerEndYear = s.CareerEndYear },
	ProfileBio:             func(d *PersonProfile, s PersonProfile) { d.Bio = s.Bio },
	ProfileLinks:           func(d *PersonProfile, s PersonProfile) { d.Links = s.Links },
}

var PersonProfileFields = []PersonProfileField{
	ProfileGender, ProfileBirthDate, ProfileDeathDate, ProfileBirthplace, ProfileHeightCm, ProfileBustCm, ProfileWaistCm,
	ProfileHipsCm, ProfileCup, ProfileBloodType, ProfileCareerStartYear, ProfileCareerEndYear, ProfileBio, ProfileLinks,
}

// PersonProfilePatch replaces each field in Fields with its value in Values; an empty value clears the field.
type PersonProfilePatch struct {
	Fields []PersonProfileField
	Values PersonProfile
}

func (p PersonProfilePatch) Apply(current PersonProfile) PersonProfile {
	next := current
	for _, field := range p.Fields {
		if copyField, ok := personProfileCopies[field]; ok {
			copyField(&next, p.Values)
		}
	}
	return next
}

// FillPersonProfile fills each empty target field from the first source whose value keeps the profile valid,
// so a merge never pairs a death date before the birth date or a career end before its start.
func FillPersonProfile(target PersonProfile, sources []PersonProfile, today time.Time) PersonProfile {
	out := target
	for _, field := range PersonProfileFields {
		for _, source := range sources {
			next, fill := fillProfileField(out, source, field, today)
			if fill == profileFilled {
				out = next
				break
			}
		}
	}
	return out
}

type profileFill int

const (
	profileFillNone profileFill = iota
	profileFilled
	profileFillInvalid
)

// fillProfileField copies field from source when it is empty in target and set in source, and returns the
// normalized result, or target unchanged when the copy would make the profile invalid.
func fillProfileField(target, source PersonProfile, field PersonProfileField, today time.Time) (PersonProfile, profileFill) {
	copyField := personProfileCopies[field]
	if !profileFieldEmpty(target, copyField) || profileFieldEmpty(source, copyField) {
		return target, profileFillNone
	}
	candidate := target
	copyField(&candidate, source)
	normalized, err := NormalizePersonProfile(candidate, today)
	if err != nil {
		return target, profileFillInvalid
	}
	return normalized, profileFilled
}

func profileFieldEmpty(p PersonProfile, copyField func(*PersonProfile, PersonProfile)) bool {
	var only PersonProfile
	copyField(&only, p)
	return only.Equal(PersonProfile{})
}

// NormalizePersonProfile trims text, clears empty values and returns the first InvalidWorkInfoError with a profile.<name> field.
func NormalizePersonProfile(input PersonProfile, today time.Time) (PersonProfile, error) {
	out := input
	var err error
	if out, err = normalizeProfileTexts(out); err != nil {
		return PersonProfile{}, err
	}
	if out, err = normalizeProfileDates(out, today); err != nil {
		return PersonProfile{}, err
	}
	if err = checkProfileNumbers(out); err != nil {
		return PersonProfile{}, err
	}
	if out.Links, err = normalizeProfileLinks(input.Links); err != nil {
		return PersonProfile{}, err
	}
	return out, nil
}

func profileField(name PersonProfileField) string {
	return "profile." + string(name)
}

func normalizeProfileTexts(p PersonProfile) (PersonProfile, error) {
	var err error
	if p.Birthplace, err = normalizeWorkInfoText(profileField(ProfileBirthplace), p.Birthplace, PersonBirthplaceMaxLength, false); err != nil {
		return p, err
	}
	if p.Bio, err = normalizeWorkInfoText(profileField(ProfileBio), p.Bio, PersonBioMaxLength, true); err != nil {
		return p, err
	}
	if p.Gender, err = normalizeProfileChoice(ProfileGender, p.Gender, PersonGenders, strings.ToLower); err != nil {
		return p, err
	}
	if p.BloodType, err = normalizeProfileChoice(ProfileBloodType, p.BloodType, PersonBloodTypes, strings.ToUpper); err != nil {
		return p, err
	}
	p.Cup, err = normalizeProfileCup(p.Cup)
	return p, err
}

func normalizeProfileChoice(name PersonProfileField, input string, choices []string, fold func(string) string) (string, error) {
	value, err := normalizeWorkInfoText(profileField(name), input, len(input), false)
	if err != nil || value == "" {
		return value, err
	}
	value = fold(value)
	if !slices.Contains(choices, value) {
		return "", invalidWorkInfo(profileField(name), WorkInfoOutOfRange, 0)
	}
	return value, nil
}

// Cups are one to three Latin letters, stored upper case.
func normalizeProfileCup(input string) (string, error) {
	field := profileField(ProfileCup)
	value, err := normalizeWorkInfoText(field, input, PersonCupMaxLength, false)
	if err != nil || value == "" {
		return value, err
	}
	value = strings.ToUpper(value)
	for i := 0; i < len(value); i++ {
		if value[i] < 'A' || value[i] > 'Z' {
			return "", invalidWorkInfo(field, WorkInfoOutOfRange, 0)
		}
	}
	return value, nil
}

// Dates share the work-date format; a death date may not precede the birth date at the precision both have.
func normalizeProfileDates(p PersonProfile, today time.Time) (PersonProfile, error) {
	todayText := today.UTC().Format(time.DateOnly)
	var err error
	for _, entry := range []struct {
		name  PersonProfileField
		value *string
	}{{ProfileBirthDate, &p.BirthDate}, {ProfileDeathDate, &p.DeathDate}} {
		field := profileField(entry.name)
		if *entry.value, err = normalizeWorkDate(field, *entry.value); err != nil {
			return p, err
		}
		if date := *entry.value; date != "" && date > todayText[:len(date)] {
			return p, invalidWorkInfo(field, WorkInfoOutOfRange, 0)
		}
	}
	if p.BirthDate != "" && p.DeathDate != "" {
		shared := min(len(p.BirthDate), len(p.DeathDate))
		if p.DeathDate[:shared] < p.BirthDate[:shared] {
			return p, invalidWorkInfo(profileField(ProfileDeathDate), WorkInfoOutOfRange, 0)
		}
	}
	return p, nil
}

func checkProfileNumbers(p PersonProfile) error {
	for _, entry := range []struct {
		name     PersonProfileField
		value    *int
		min, max int
	}{
		{ProfileHeightCm, p.HeightCm, PersonHeightMinCm, PersonHeightMaxCm},
		{ProfileBustCm, p.BustCm, PersonMeasurementMinCm, PersonMeasurementMaxCm},
		{ProfileWaistCm, p.WaistCm, PersonMeasurementMinCm, PersonMeasurementMaxCm},
		{ProfileHipsCm, p.HipsCm, PersonMeasurementMinCm, PersonMeasurementMaxCm},
		{ProfileCareerStartYear, p.CareerStartYear, PersonCareerMinYear, PersonCareerMaxYear},
		{ProfileCareerEndYear, p.CareerEndYear, PersonCareerMinYear, PersonCareerMaxYear},
	} {
		if entry.value != nil && (*entry.value < entry.min || *entry.value > entry.max) {
			return invalidWorkInfo(profileField(entry.name), WorkInfoOutOfRange, 0)
		}
	}
	if p.CareerStartYear != nil && p.CareerEndYear != nil && *p.CareerEndYear < *p.CareerStartYear {
		return invalidWorkInfo(profileField(ProfileCareerEndYear), WorkInfoOutOfRange, 0)
	}
	return nil
}

// Links follow the work URL rules: blanks are dropped, duplicates rejected and input order kept.
func normalizeProfileLinks(links []string) ([]string, error) {
	out, err := normalizeURLList(profileField(ProfileLinks), links, PersonLinksMaxCount)
	if len(out) == 0 {
		return nil, err
	}
	return out, err
}
