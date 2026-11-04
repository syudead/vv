package store

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A person's profile is columns on people plus ordered rows in people_links.

// personProfileColumnNames is the order scannedProfile.targets and personProfileArgs use.
var personProfileColumnNames = []string{
	"gender", "birth_date", "death_date", "birthplace", "height_cm", "bust_cm", "waist_cm", "hips_cm",
	"cup", "blood_type", "career_start_year", "career_end_year", "bio",
}

type scannedProfile struct {
	texts [7]sql.NullString
	ints  [6]sql.NullInt64
}

func (s *scannedProfile) targets() []any {
	t := &s.texts
	n := &s.ints
	return []any{&t[0], &t[1], &t[2], &t[3], &n[0], &n[1], &n[2], &n[3], &t[4], &t[5], &n[4], &n[5], &t[6]}
}

func (s *scannedProfile) profile() domain.PersonProfile {
	t := s.texts
	n := s.ints
	return domain.PersonProfile{
		Gender: t[0].String, BirthDate: t[1].String, DeathDate: t[2].String, Birthplace: t[3].String,
		HeightCm: optionalInt(n[0]), BustCm: optionalInt(n[1]), WaistCm: optionalInt(n[2]), HipsCm: optionalInt(n[3]),
		Cup: t[4].String, BloodType: t[5].String, CareerStartYear: optionalInt(n[4]), CareerEndYear: optionalInt(n[5]),
		Bio: t[6].String,
	}
}

func optionalInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}

func optionalIntArg(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func personProfileArgs(p domain.PersonProfile) []any {
	return []any{
		nullString(p.Gender), nullString(p.BirthDate), nullString(p.DeathDate), nullString(p.Birthplace),
		optionalIntArg(p.HeightCm), optionalIntArg(p.BustCm), optionalIntArg(p.WaistCm), optionalIntArg(p.HipsCm),
		nullString(p.Cup), nullString(p.BloodType), optionalIntArg(p.CareerStartYear), optionalIntArg(p.CareerEndYear),
		nullString(p.Bio),
	}
}

// writePersonProfile replaces the stored profile with a normalized one.
func writePersonProfile(ctx context.Context, tx *sql.Tx, id int64, profile domain.PersonProfile) error {
	t := creditTablesByKind[domain.CreditPerson]
	columns := append(slices.Clone(personProfileColumnNames), "birth_key")
	args := append(personProfileArgs(profile), birthKeyArg(profile.BirthDate))
	return writeCreditProfile(ctx, tx, t, id, columns, args, profile.Links)
}

// birthKeyArg is domain.BirthKey, or null for an unknown date.
func birthKeyArg(date string) any {
	if key, ok := domain.BirthKey(date); ok {
		return key
	}
	return nil
}

var personProfileCodec = profileCodec{
	columns: personProfileColumnNames,
	scan: func(c *domain.Credit) ([]any, func()) {
		var scanned scannedProfile
		return scanned.targets(), func() { c.Profile = scanned.profile() }
	},
	links: func(c *domain.Credit) *[]string { return &c.Profile.Links },
	write: func(ctx context.Context, tx *sql.Tx, id int64, input domain.CreditInput) error {
		return writePersonProfile(ctx, tx, id, input.Profile)
	},
	set:     func(input domain.CreditInput) bool { return !input.Profile.Equal(domain.PersonProfile{}) },
	differs: func(c domain.Credit, next domain.CreditInput) bool { return !c.Profile.Equal(next.Profile) },
	fill: func(target domain.Credit, sources []domain.Credit) (domain.CreditInput, bool) {
		profiles := make([]domain.PersonProfile, len(sources))
		for i, source := range sources {
			profiles[i] = source.Profile
		}
		filled := domain.FillPersonProfile(target.Profile, profiles, time.Now())
		return domain.CreditInput{Profile: filled}, !filled.Equal(target.Profile)
	},
}
