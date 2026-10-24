package domain

import (
	"errors"
	"slices"
)

// VVMDM bundles one work-info plugin, vv.link, that reads any video link, and the plugins that declare domains are
// tried for a link in the owner's order (docs/design-docs/link-fetch.md).

const (
	LinkPluginName = "vv.link"
	// LinkPluginDigest stands for the bundled code; approving it approves the plugin VVMDM ships.
	LinkPluginDigest = "builtin:vv.link"
	// AnyLinkDomain is the bundled plugin's only domain; a manifest cannot declare it.
	AnyLinkDomain = "*"
)

var (
	// ErrWorkInfoPluginBuiltin rejects deleting the bundled plugin or uploading a module under its name.
	ErrWorkInfoPluginBuiltin = errors.New("the plugin is bundled with VVMDM")
	// ErrLinkOrderChanged rejects an order that does not name exactly the plugins that declare domains.
	ErrLinkOrderChanged = errors.New("the plugins that handle links changed since they were listed")
	// ErrWorkInfoLinkUnsupported reports a fetch link whose host matches none of the work-info plugin's domains.
	ErrWorkInfoLinkUnsupported = errors.New("no domain of the work-info plugin matches the link")
)

// LinkPluginManifest is the bundled plugin's manifest, which the registry row takes from the code.
var LinkPluginManifest = WorkInfoPluginManifest{
	Contract:    WorkInfoPluginContract,
	Kind:        WorkInfoPluginKindWork,
	Name:        LinkPluginName,
	Version:     "1",
	Description: "Reads the title, description, date and image of a video link from the site's oEmbed endpoint or the page's shared metadata.",
	Operations:  []WorkInfoOperation{WorkInfoFetch},
	Settings:    []WorkInfoPluginSetting{},
	Domains:     []string{AnyLinkDomain},
}

// HandlesLinks reports whether the plugin takes part in the link order.
func (p WorkInfoPlugin) HandlesLinks() bool {
	return p.Kind() == WorkInfoPluginKindWork && len(p.Manifest.Domains) > 0
}

// LinkPluginsFor returns the work-info plugins that would run for link, in link order: approved, able to fetch,
// with every required setting set, and declaring a domain that matches.
func LinkPluginsFor(plugins []WorkInfoPlugin, link string) []WorkInfoPlugin {
	var out []WorkInfoPlugin
	for _, p := range plugins {
		if p.HandlesLinks() && p.State() == WorkInfoPluginApproved && p.Manifest.Supports(WorkInfoFetch) &&
			len(MissingWorkInfoPluginSettings(p.Manifest.Settings, p.Values)) == 0 &&
			LinkMatchesDomains(link, p.Manifest.Domains) {
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, func(a, b WorkInfoPlugin) int { return a.LinkRank - b.LinkRank })
	return out
}

// CheckLinkOrder accepts order only when it names each plugin that handles links exactly once.
func CheckLinkOrder(plugins []WorkInfoPlugin, order []int64) error {
	var current []int64
	for _, p := range plugins {
		if p.HandlesLinks() {
			current = append(current, p.ID)
		}
	}
	requested := slices.Clone(order)
	slices.Sort(current)
	slices.Sort(requested)
	if !slices.Equal(current, requested) {
		return ErrLinkOrderChanged
	}
	return nil
}

// FetchedDetailFields lists, in comparison order, the detail fields fetched carries a value for.
func FetchedDetailFields(fetched FetchedWorkInfo) []WorkInfoField {
	var out []WorkInfoField
	for _, field := range WorkInfoFields {
		if fetched.HasDetail(field) {
			out = append(out, field)
		}
	}
	return out
}

// EmptyWorkInfoFields keeps the fields of fields that stored has no value for, so a fill never replaces or extends
// what the owner entered.
func EmptyWorkInfoFields(stored WorkInfo, fields []WorkInfoField) []WorkInfoField {
	var out []WorkInfoField
	for _, field := range fields {
		if !stored.hasField(field) {
			out = append(out, field)
		}
	}
	return out
}

func (w WorkInfo) hasField(field WorkInfoField) bool {
	return detailHasValue(field, detailValues{
		code: w.Code, description: w.Description, urls: len(w.URLs), externalIDs: len(w.ExternalIDs),
		date: func(kind DateKind) bool {
			return slices.ContainsFunc(w.Dates, func(d WorkDate) bool { return d.Kind == kind })
		},
		person: func(role PersonRole) bool {
			return slices.ContainsFunc(w.People, func(p WorkPerson) bool { return p.Role == role })
		},
		organization: func(role OrganizationRole) bool {
			return slices.ContainsFunc(w.Organizations, func(o WorkOrganization) bool { return o.Role == role })
		},
	})
}
