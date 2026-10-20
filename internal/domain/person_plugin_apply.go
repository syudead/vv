package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// A person plugin's result applies per item: profile fields fill, or replace on the person page, and lists append; the
// rules are shared by the person page and the work-info lookups (docs/design-docs/person-plugins.md).

// ErrPersonExternalIDMissing reports a person without a stored external ID that the person plugin accepts.
var ErrPersonExternalIDMissing = errors.New("the person has no external ID the plugin accepts")

// ErrPersonNotFound tells a missing person apart from a missing plugin in person plugin fetches.
var ErrPersonNotFound = errors.New("the person does not exist")

// SkippedWorkInfoTaken skips an external ID another person holds; nothing is merged.
const SkippedWorkInfoTaken SkippedWorkInfoReason = "taken"

// PersonPluginItem names one applicable part of a fetched person: image, aliases, externalIds or profile.<field>.
type PersonPluginItem string

const (
	PersonItemImage       PersonPluginItem = "image"
	PersonItemAliases     PersonPluginItem = "aliases"
	PersonItemExternalIDs PersonPluginItem = "externalIds"
)

// PersonProfileItem is the item of one profile field.
func PersonProfileItem(field PersonProfileField) PersonPluginItem {
	return PersonPluginItem(profileField(field))
}

// ProfileField returns the profile field the item names, if any.
func (i PersonPluginItem) ProfileField() (PersonProfileField, bool) {
	name, ok := strings.CutPrefix(string(i), "profile.")
	field := PersonProfileField(name)
	return field, ok && slices.Contains(PersonProfileFields, field)
}

// Valid reports whether the item names a part of a fetched person.
func (i PersonPluginItem) Valid() bool {
	_, profile := i.ProfileField()
	return profile || i == PersonItemImage || i == PersonItemAliases || i == PersonItemExternalIDs
}

// HasItem reports whether the fetched person has a value for item; the image is the caller's, since VVMDM keeps it
// apart from the plugin's values.
func (p FetchedPluginPerson) HasItem(item PersonPluginItem) bool {
	switch item {
	case PersonItemAliases:
		return len(p.Aliases) > 0
	case PersonItemExternalIDs:
		return len(p.ExternalIDs) > 0
	}
	field, ok := item.ProfileField()
	return ok && !profileFieldEmpty(p.Profile, personProfileCopies[field])
}

// PersonFetchID picks the first stored ID, in stored (source, value) order, whose source the plugin declares.
func PersonFetchID(stored []ExternalID, sources []string) (ExternalID, bool) {
	at := slices.IndexFunc(stored, func(id ExternalID) bool { return slices.Contains(sources, id.Source) })
	if at < 0 {
		return ExternalID{}, false
	}
	return stored[at], true
}

// SkippedPersonValue is one fetched value an apply left out; HolderName names the person holding a taken ID.
type SkippedPersonValue struct {
	Item       PersonPluginItem
	Value      string
	Reason     SkippedWorkInfoReason
	HolderName string
}

// PersonItemFill applies the selected Items of Fetched to Current; Holders maps each normalized fetched ID another
// person holds to that person's name.
type PersonItemFill struct {
	Current Credit
	Fetched FetchedPluginPerson
	Items   []PersonPluginItem
	Holders map[ExternalID]string
	Today   time.Time
	// Replace writes a selected profile field that already holds another value; otherwise only empty fields fill.
	Replace bool
}

// PersonItemFilled is the person's next aliases, external IDs and profile, with the values left out in item order.
type PersonItemFilled struct {
	Aliases     []Alias
	ExternalIDs []ExternalID
	Profile     PersonProfile
	Skipped     []SkippedPersonValue
}

// FillPersonItems fills empty profile fields, or replaces them under Replace, and appends missing aliases and external IDs within their limits; a
// value that breaks a rule is skipped as invalid, and an ID another person holds as taken.
func FillPersonItems(in PersonItemFill) PersonItemFilled {
	out := PersonItemFilled{
		Aliases: slices.Clone(in.Current.Aliases), ExternalIDs: slices.Clone(in.Current.ExternalIDs),
		Profile: in.Current.Profile,
	}
	selected := func(item PersonPluginItem) bool { return slices.Contains(in.Items, item) }
	out.writeProfile(in, selected)
	if selected(PersonItemAliases) {
		out.appendAliases(in.Current.Name, in.Fetched.Aliases)
	}
	if selected(PersonItemExternalIDs) {
		out.appendExternalIDs(in.Fetched.ExternalIDs, in.Holders)
	}
	return out
}

// writeProfile writes the selected profile fields; replaced fields are checked together first, then one at a time
// against the stored values when the set is invalid, so only the fields that break a rule are skipped.
func (f *PersonItemFilled) writeProfile(in PersonItemFill, selected func(PersonPluginItem) bool) {
	fields := slices.DeleteFunc(slices.Clone(PersonProfileFields), func(field PersonProfileField) bool {
		return !selected(PersonProfileItem(field))
	})
	write := fillProfileField
	if in.Replace {
		if next, ok := replaceProfileFields(f.Profile, in.Fetched.Profile, fields, in.Today); ok {
			f.Profile = next
			return
		}
		write = replaceProfileField
	}
	for _, field := range fields {
		next, fill := write(f.Profile, in.Fetched.Profile, field, in.Today)
		switch fill {
		case profileFilled:
			f.Profile = next
		case profileFillInvalid:
			f.skip(PersonProfileItem(field), profileFieldText(in.Fetched.Profile, field), SkippedWorkInfoInvalid, "")
		}
	}
}

func (f *PersonItemFilled) skip(item PersonPluginItem, value string, reason SkippedWorkInfoReason, holder string) {
	f.Skipped = append(f.Skipped, SkippedPersonValue{Item: item, Value: value, Reason: reason, HolderName: holder})
}

// An alias equal to the name or a stored alias is already on the person and is neither added nor reported.
func (f *PersonItemFilled) appendAliases(name string, fetched []Alias) {
	for _, alias := range fetched {
		normalized, err := normalizeAliases(name, []Alias{alias})
		var invalid *InvalidWorkInfoError
		switch {
		case errors.As(err, &invalid) && invalid.Problem == WorkInfoDuplicate:
		case err != nil || len(normalized) != 1:
			f.skip(PersonItemAliases, alias.Name, SkippedWorkInfoInvalid, "")
		case slices.ContainsFunc(f.Aliases, func(a Alias) bool { return a.Name == normalized[0].Name }):
		case len(f.Aliases) >= CreditAliasesMaxCount:
			f.skip(PersonItemAliases, alias.Name, SkippedWorkInfoTooMany, "")
		default:
			f.Aliases = append(f.Aliases, normalized[0])
		}
	}
}

func (f *PersonItemFilled) appendExternalIDs(fetched []ExternalID, holders map[ExternalID]string) {
	for _, id := range fetched {
		shown := id.Source + ": " + id.Value
		normalized, ok := NormalizeExternalID(id)
		holder, taken := holders[normalized]
		switch {
		case !ok:
			f.skip(PersonItemExternalIDs, shown, SkippedWorkInfoInvalid, "")
		case slices.Contains(f.ExternalIDs, normalized):
		case taken:
			f.skip(PersonItemExternalIDs, shown, SkippedWorkInfoTaken, holder)
		case len(f.ExternalIDs) >= ExternalIDsMaxCount:
			f.skip(PersonItemExternalIDs, shown, SkippedWorkInfoTooMany, "")
		default:
			f.ExternalIDs = append(f.ExternalIDs, normalized)
		}
	}
}

// PersonPluginFetchResult is a kept person fetch as the person page sees it.
type PersonPluginFetchResult struct {
	ID        string
	ExpiresAt time.Time
	Person    FetchedPluginPerson
	// ImageCount is how many images VVMDM kept, served by index in the plugin's order.
	ImageCount int
}

// PersonPluginApply carries a kept person fetch to the store, which rechecks the plugin in its transaction.
type PersonPluginApply struct {
	PersonID   int64
	PluginID   int64
	Digest     string
	Generation int64
	Fetched    FetchedPluginPerson
	Items      []PersonPluginItem
}

// PersonPluginApplied is the person after the apply's transaction and the values it left out.
type PersonPluginApplied struct {
	Person  Credit
	Skipped []SkippedPersonValue
}
