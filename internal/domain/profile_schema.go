package domain

import "time"

// profileSchema lets person and organization profiles share field-wise patching and merge filling.
type profileSchema[P any, F comparable] struct {
	fields    []F
	copies    map[F]func(dst *P, src P)
	equal     func(a, b P) bool
	normalize func(p P, today time.Time) (P, error)
}

func (s profileSchema[P, F]) apply(current P, fields []F, values P) P {
	next := current
	for _, field := range fields {
		if copyField, ok := s.copies[field]; ok {
			copyField(&next, values)
		}
	}
	return next
}

func (s profileSchema[P, F]) only(p P, field F) P {
	var out P
	s.copies[field](&out, p)
	return out
}

func (s profileSchema[P, F]) fieldEqual(a, b P, field F) bool {
	return s.equal(s.only(a, field), s.only(b, field))
}

func (s profileSchema[P, F]) fieldEmpty(p P, field F) bool {
	var zero P
	return s.equal(s.only(p, field), zero)
}

// fillField copies field from source when it is empty in target and set in source, and returns the normalized
// result, or target unchanged when the copy would make the profile invalid.
func (s profileSchema[P, F]) fillField(target, source P, field F, today time.Time) (P, profileFill) {
	if !s.fieldEmpty(target, field) || s.fieldEmpty(source, field) {
		return target, profileFillNone
	}
	return s.copyField(target, source, field, today)
}

// replaceField copies field from source when it is set there and differs in target, with fillField's results.
func (s profileSchema[P, F]) replaceField(target, source P, field F, today time.Time) (P, profileFill) {
	if s.fieldEmpty(source, field) || s.fieldEqual(target, source, field) {
		return target, profileFillNone
	}
	return s.copyField(target, source, field, today)
}

func (s profileSchema[P, F]) copyField(target, source P, field F, today time.Time) (P, profileFill) {
	candidate := target
	s.copies[field](&candidate, source)
	normalized, err := s.normalize(candidate, today)
	if err != nil {
		return target, profileFillInvalid
	}
	return normalized, profileFilled
}

// fill fills each empty target field from the first source whose value keeps the profile valid.
func (s profileSchema[P, F]) fill(target P, sources []P, today time.Time) P {
	out := target
	for _, field := range s.fields {
		for _, source := range sources {
			if next, fill := s.fillField(out, source, field, today); fill == profileFilled {
				out = next
				break
			}
		}
	}
	return out
}

// revertFilled clears each field a merge filled, from before to filled, while current still holds the filled value.
func (s profileSchema[P, F]) revertFilled(current, before, filled P) P {
	out := current
	for _, field := range s.fields {
		if s.fieldEqual(before, filled, field) || !s.fieldEqual(current, filled, field) {
			continue
		}
		s.copies[field](&out, before)
	}
	return out
}

type profileFill int

const (
	profileFillNone profileFill = iota
	profileFilled
	profileFillInvalid
)
