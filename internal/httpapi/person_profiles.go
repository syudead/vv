package httpapi

import (
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Profiles map field by field; the store has already normalized every value it returns.

func toAPIProfile(p domain.PersonProfile) gen.PersonProfile {
	out := gen.PersonProfile{
		Gender: (*gen.PersonProfileGender)(optionalString(p.Gender)), BirthDate: optionalString(p.BirthDate),
		DeathDate: optionalString(p.DeathDate), Birthplace: optionalString(p.Birthplace), HeightCm: p.HeightCm,
		BustCm: p.BustCm, WaistCm: p.WaistCm, HipsCm: p.HipsCm, Cup: optionalString(p.Cup),
		BloodType: (*gen.PersonProfileBloodType)(optionalString(p.BloodType)), CareerStartYear: p.CareerStartYear,
		CareerEndYear: p.CareerEndYear, Bio: optionalString(p.Bio),
	}
	if len(p.Links) > 0 {
		links := append([]string(nil), p.Links...)
		out.Links = &links
	}
	return out
}

func toExternalProfile(p domain.PersonProfile) extgen.PersonProfile {
	in := toAPIProfile(p)
	return extgen.PersonProfile{
		Gender: (*extgen.PersonProfileGender)(in.Gender), BirthDate: in.BirthDate, DeathDate: in.DeathDate,
		Birthplace: in.Birthplace, HeightCm: in.HeightCm, BustCm: in.BustCm, WaistCm: in.WaistCm, HipsCm: in.HipsCm,
		Cup: in.Cup, BloodType: (*extgen.PersonProfileBloodType)(in.BloodType), CareerStartYear: in.CareerStartYear,
		CareerEndYear: in.CareerEndYear, Bio: in.Bio, Links: in.Links,
	}
}

func fromAPIProfile(p gen.PersonProfile) domain.PersonProfile {
	return domain.PersonProfile{
		Gender: valueOrZero((*string)(p.Gender)), BirthDate: valueOrZero(p.BirthDate), DeathDate: valueOrZero(p.DeathDate),
		Birthplace: valueOrZero(p.Birthplace), HeightCm: p.HeightCm, BustCm: p.BustCm, WaistCm: p.WaistCm,
		HipsCm: p.HipsCm, Cup: valueOrZero(p.Cup), BloodType: valueOrZero((*string)(p.BloodType)),
		CareerStartYear: p.CareerStartYear, CareerEndYear: p.CareerEndYear, Bio: valueOrZero(p.Bio),
		Links: valueOrZero(p.Links),
	}
}

// A patch decodes null and absent alike; profilePatchOf tells them apart from the raw properties.
func fromAPIProfilePatch(p gen.PersonProfilePatch) domain.PersonProfile {
	return fromAPIProfile(gen.PersonProfile{
		Gender: (*gen.PersonProfileGender)(p.Gender), BirthDate: p.BirthDate, DeathDate: p.DeathDate,
		Birthplace: p.Birthplace, HeightCm: p.HeightCm, BustCm: p.BustCm, WaistCm: p.WaistCm, HipsCm: p.HipsCm,
		Cup: p.Cup, BloodType: (*gen.PersonProfileBloodType)(p.BloodType), CareerStartYear: p.CareerStartYear,
		CareerEndYear: p.CareerEndYear, Bio: p.Bio, Links: p.Links,
	})
}
