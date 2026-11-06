package domain

import (
	"context"
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

// PersonPluginItem names one applicable part of a fetched person: image, names, externalIds or profile.<field>.
type PersonPluginItem string

const (
	PersonItemImage       PersonPluginItem = "image"
	PersonItemNames       PersonPluginItem = "names"
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
	return profile || i == PersonItemImage || i == PersonItemNames || i == PersonItemExternalIDs
}

// HasItem reports whether the fetched person has a value for item; the image is the caller's, since VVMDM keeps it
// apart from the plugin's values.
func (p FetchedPluginPerson) HasItem(item PersonPluginItem) bool {
	switch item {
	case PersonItemNames:
		return len(p.Names) > 0
	case PersonItemExternalIDs:
		return len(p.ExternalIDs) > 0
	}
	field, ok := item.ProfileField()
	return ok && !personProfiles.fieldEmpty(p.Profile, field)
}

// PersonPluginSelection is what an apply takes from a fetched person: profile fields and the image as Items, and the
// names, by spelling, external IDs and links it appends one by one.
type PersonPluginSelection struct {
	Items       []PersonPluginItem
	Names       []string
	ExternalIDs []ExternalID
	Links       []string
}

// Select narrows p to the selected names, external IDs and links, in p's order, and returns the items an apply
// writes. It reports false for a list item among Items, an item or value p does not carry, the image without hasImage,
// or an empty selection.
func (p FetchedPluginPerson) Select(sel PersonPluginSelection, hasImage bool) (FetchedPluginPerson, []PersonPluginItem, bool) {
	if slices.ContainsFunc(sel.Items, isListItem) {
		return FetchedPluginPerson{}, nil, false
	}
	names, okNames := pick(p.Names, sel.Names, func(n CreditName) string { return n.Name })
	ids, okIDs := pick(p.ExternalIDs, sel.ExternalIDs, func(id ExternalID) ExternalID { return id })
	links, okLinks := pick(p.Profile.Links, sel.Links, func(link string) string { return link })
	if !okNames || !okIDs || !okLinks {
		return FetchedPluginPerson{}, nil, false
	}
	out := p
	out.Names, out.ExternalIDs, out.Profile.Links = names, ids, links
	items := slices.Clone(sel.Items)
	for _, list := range []struct {
		item  PersonPluginItem
		count int
	}{{PersonProfileItem(ProfileLinks), len(links)}, {PersonItemNames, len(names)}, {PersonItemExternalIDs, len(ids)}} {
		if list.count > 0 {
			items = append(items, list.item)
		}
	}
	valid := !slices.ContainsFunc(items, func(item PersonPluginItem) bool {
		return item == PersonItemImage && !hasImage || item != PersonItemImage && !out.HasItem(item)
	})
	return out, items, valid && len(items) > 0
}

func isListItem(item PersonPluginItem) bool {
	return item == PersonItemNames || item == PersonItemExternalIDs || item == PersonProfileItem(ProfileLinks)
}

// pick keeps the values of all whose key is selected, in all's order, and reports false when a selected key is not
// among them.
func pick[T any, K comparable](all []T, selected []K, key func(T) K) ([]T, bool) {
	var out []T
	for _, value := range all {
		if slices.Contains(selected, key(value)) {
			out = append(out, value)
		}
	}
	return out, len(out) == len(dedupe(selected))
}

func dedupe[K comparable](values []K) []K {
	var out []K
	for _, value := range values {
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
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

// PersonItemFilled is the person's next names, external IDs and profile, with the values left out in item order.
type PersonItemFilled struct {
	Names       []CreditName
	ExternalIDs []ExternalID
	Profile     PersonProfile
	Skipped     []SkippedPersonValue
}

// FillPersonItems fills empty profile fields, or replaces them under Replace, and appends missing names and external
// IDs within their limits; a value that breaks a rule is skipped as invalid, and an ID another person holds as taken.
// The first name appended to a provisional person becomes its display name.
func FillPersonItems(in PersonItemFill) PersonItemFilled {
	out := PersonItemFilled{
		Names: slices.Clone(in.Current.Names), ExternalIDs: slices.Clone(in.Current.ExternalIDs),
		Profile: in.Current.Profile,
	}
	selected := func(item PersonPluginItem) bool { return slices.Contains(in.Items, item) }
	out.writeProfile(in, selected)
	if selected(PersonProfileItem(ProfileLinks)) {
		out.appendLinks(in.Fetched.Profile.Links)
	}
	if selected(PersonItemNames) {
		out.appendNames(in.Fetched.Names)
	}
	if selected(PersonItemExternalIDs) {
		out.appendExternalIDs(in.Fetched.ExternalIDs, in.Holders)
	}
	return out
}

// writeProfile writes the selected profile fields; replaced fields are checked together first, then one at a time
// against the stored values when the set is invalid, so only the fields that break a rule are skipped.
func (f *PersonItemFilled) writeProfile(in PersonItemFill, selected func(PersonPluginItem) bool) {
	// Links append like names, so they never replace.
	fields := slices.DeleteFunc(slices.Clone(PersonProfileFields), func(field PersonProfileField) bool {
		return field == ProfileLinks || !selected(PersonProfileItem(field))
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

// A name the person already has is neither added nor reported.
func (f *PersonItemFilled) appendNames(fetched []CreditName) {
	for _, name := range fetched {
		normalized, err := normalizeCreditNameEntry("names[0]", name)
		switch {
		case err != nil:
			f.skip(PersonItemNames, name.Name, SkippedWorkInfoInvalid, "")
		case slices.ContainsFunc(f.Names, func(n CreditName) bool { return n.Name == normalized.Name }):
		case len(f.Names) >= CreditNamesMaxCount:
			f.skip(PersonItemNames, name.Name, SkippedWorkInfoTooMany, "")
		default:
			f.Names = append(f.Names, normalized)
		}
	}
}

// A fetched link is already valid, since the kept profile holds only values that pass the rules.
func (f *PersonItemFilled) appendLinks(fetched []string) {
	item := PersonProfileItem(ProfileLinks)
	for _, link := range fetched {
		switch {
		case slices.Contains(f.Profile.Links, link):
		case len(f.Profile.Links) >= PersonLinksMaxCount:
			f.skip(item, link, SkippedWorkInfoTooMany, "")
		default:
			f.Profile.Links = append(slices.Clone(f.Profile.Links), link)
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
	// Link is the link the fetch ran with; empty for a fetch by ID.
	Link string
}

// PersonFetchKey names what a person page fetch runs with: a candidate's Source and ID together, a Link, or nothing,
// when VVMDM picks the person's stored ID or link.
type PersonFetchKey struct {
	Source, ID, Link *string
}

// PersonPluginApply carries a kept person fetch to the store, which rechecks the plugin in its transaction.
type PersonPluginApply struct {
	PersonID   int64
	PluginID   int64
	Digest     string
	Generation int64
	// FetchedByPeer marks a fetch the paired node ran with its own plugin and checked before sending; this node
	// has no such plugin to recheck.
	FetchedByPeer bool
	Fetched       FetchedPluginPerson
	Items         []PersonPluginItem
}

// PersonPluginApplied is the person after the apply's transaction and the values it left out.
type PersonPluginApplied struct {
	Person  Credit
	Skipped []SkippedPersonValue
}

// PersonPluginPeer is the paired node as a person page fetch for one of its people reaches it: this node runs the
// plugin, and the peer holds the person, the writes and the face. Its people are addressed by their remote ids.
type PersonPluginPeer interface {
	// Get returns ErrNotFound for a missing person.
	Get(ctx context.Context, kind CreditKind, id int64) (Credit, error)
	// ApplyPersonPluginFetch writes on the peer without the image item, returning ErrNotFound for a missing person.
	ApplyPersonPluginFetch(ctx context.Context, apply PersonPluginApply) (PersonPluginApplied, error)
	// ReplacePersonImage makes data the person's face.
	ReplacePersonImage(ctx context.Context, id int64, data []byte) (int64, error)
}
