package app

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// fetched reads the fetch result the fetchId names, as the fetch tool of operation answered it in this turn.
func (d *planDraft) fetched(operation string, out any) bool {
	if d.args.FetchID == "" {
		return d.t.refuse("%s needs the fetchId that %s answered", d.row.ID, operation)
	}
	var result fetchedResult
	if d.t.writes != nil {
		result = d.t.writes.fetches[d.args.FetchID]
	}
	if result.operation != operation || json.Unmarshal(result.body, out) != nil {
		return d.t.refuse("no %s in this request answered the fetchId %s; call %s first", operation,
			d.args.FetchID, operation)
	}
	if len(d.args.Fields) == 0 {
		return d.t.refuse("%s needs the fields to apply", d.row.ID)
	}
	if d.plan.Call.Path == nil {
		d.plan.Call.Path = map[string]string{}
	}
	d.plan.Call.Path["fetchId"] = d.args.FetchID
	return true
}

// fetchedField records one applied field on the card with the value the fetch answered.
func (d *planDraft) fetchedField(name string, value any) {
	d.literal(domain.AssistantArgument(name), value)
}

// fetchedWork is the part of a work-info fetch result the apply step chooses from.
type fetchedWork struct {
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Dates       []struct {
		Kind string `json:"kind"`
		Date string `json:"date"`
	} `json:"dates"`
	People []struct {
		Role string `json:"role"`
		Name string `json:"name"`
	} `json:"people"`
	Organizations []struct {
		Role string `json:"role"`
		Name string `json:"name"`
	} `json:"organizations"`
	URLs        []string          `json:"urls"`
	ExternalIDs []json.RawMessage `json:"externalIds"`
	Covers      []json.RawMessage `json:"covers"`
}

// applyWorkInfoFetch applies the named fields of a fetch result read in this turn to its video. A whole field
// takes its value; people and organizations, which may name a role, take every fetched entry of it, and urls and
// external ids every fetched one.
func applyWorkInfoFetch(_ context.Context, d *planDraft) (bool, error) {
	var work fetchedWork
	if !d.fetched("fetchWorkInfo", &work) {
		return false, nil
	}
	fields := []string{}
	for _, field := range d.args.Fields {
		name, role, _ := strings.Cut(field, ".")
		var ok bool
		switch name {
		case "people", "organizations":
			ok = d.fetchedCredits(&work, name, role)
		case "dates":
			ok = d.fetchedDates(&work, role, &fields)
		case "urls", "externalIds":
			ok = d.fetchedLists(&work, name)
		default:
			ok = d.fetchedWhole(&work, name, &fields)
		}
		if !ok {
			return d.t.refuse("the fetch has no %s to apply", field), nil
		}
	}
	d.body("fields", fields)
	return true, nil
}

// fetchedWhole adds a field applied as a whole: title, tags, code, description or the first cover.
func (d *planDraft) fetchedWhole(work *fetchedWork, name string, fields *[]string) bool {
	values := map[string]any{"title": work.Title, "code": work.Code, "description": work.Description}
	switch {
	case name == "tags" && len(work.Tags) > 0:
		d.fetchedField(name, work.Tags)
	case name == "cover" && len(work.Covers) > 0:
		d.body("coverIndex", 0)
		d.fetchedField(name, 1)
	case values[name] != nil && values[name] != "":
		d.fetchedField(name, values[name])
	default:
		return false
	}
	*fields = append(*fields, name)
	return true
}

// fetchedDates adds the fetched dates of one kind, or of every kind.
func (d *planDraft) fetchedDates(work *fetchedWork, kind string, fields *[]string) bool {
	found := false
	for _, date := range work.Dates {
		if kind != "" && date.Kind != kind {
			continue
		}
		name := "dates." + date.Kind
		if !slices.Contains(*fields, name) {
			*fields = append(*fields, name)
		}
		d.fetchedField(name, date.Date)
		found = true
	}
	return found
}

// fetchedCredits adds the positions of the fetched people or organizations, of one role or of every role.
func (d *planDraft) fetchedCredits(work *fetchedWork, name, role string) bool {
	type credit struct{ Role, Name string }
	var credits []credit
	if name == "people" {
		for _, p := range work.People {
			credits = append(credits, credit{p.Role, p.Name})
		}
	} else {
		for _, o := range work.Organizations {
			credits = append(credits, credit{o.Role, o.Name})
		}
	}
	positions, _ := d.plan.Call.Body[name].([]int)
	var names []string
	for i, c := range credits {
		if (role == "" || c.Role == role) && !slices.Contains(positions, i) {
			positions = append(positions, i)
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return false
	}
	d.body(name, positions)
	d.fetchedField(name, names)
	return true
}

// fetchedLists adds every fetched url or external id.
func (d *planDraft) fetchedLists(work *fetchedWork, name string) bool {
	var values any
	switch {
	case name == "urls" && len(work.URLs) > 0:
		values = work.URLs
	case name == "externalIds" && len(work.ExternalIDs) > 0:
		values = work.ExternalIDs
	default:
		return false
	}
	d.body(name, values)
	d.fetchedField(name, values)
	return true
}

// fetchedPerson is the part of a person fetch result the apply step chooses from.
type fetchedPerson struct {
	Person struct {
		Aliases []struct {
			Name string `json:"name"`
		} `json:"aliases"`
		ExternalIDs []json.RawMessage `json:"externalIds"`
		Profile     map[string]any    `json:"profile"`
		ImageCount  int               `json:"imageCount"`
	} `json:"person"`
}

// applyPersonPluginFetch applies the named parts of a person fetch result read in this turn: the first image, the
// whole profile or one profile field, every alias, every external id, or every link.
func applyPersonPluginFetch(_ context.Context, d *planDraft) (bool, error) {
	var fetched fetchedPerson
	if !d.fetched("fetchPersonWithPlugin", &fetched) {
		return false, nil
	}
	p := fetched.Person
	items := []string{}
	for _, field := range d.args.Fields {
		name, part, _ := strings.Cut(field, ".")
		ok := false
		switch name {
		case "image":
			if ok = p.ImageCount > 0; ok {
				items = append(items, "image")
				d.body("imageIndex", 0)
				d.fetchedField("image", 1)
			}
		case "profile":
			ok = d.fetchedProfile(p.Profile, part, &items)
		case "aliases":
			ok = d.fetchedAliases(p.Aliases)
		case "externalIds", "links":
			ok = d.fetchedPersonList(name, p.ExternalIDs, p.Profile["links"])
		}
		if !ok {
			return d.t.refuse("the fetch has no %s to apply", field), nil
		}
	}
	d.body("items", items)
	return true, nil
}

// fetchedProfile adds one profile field, or every fetched one but the links.
func (d *planDraft) fetchedProfile(profile map[string]any, part string, items *[]string) bool {
	found := false
	for _, key := range slices.Sorted(maps.Keys(profile)) {
		if key == "links" || profile[key] == nil || (part != "" && key != part) {
			continue
		}
		item := "profile." + key
		if !slices.Contains(*items, item) {
			*items = append(*items, item)
		}
		d.fetchedField(item, profile[key])
		found = true
	}
	return found
}

func (d *planDraft) fetchedAliases(aliases []struct {
	Name string `json:"name"`
}) bool {
	names := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		names = append(names, alias.Name)
	}
	if len(names) == 0 {
		return false
	}
	d.body("aliases", names)
	d.fetchedField("aliases", names)
	return true
}

// fetchedPersonList adds every fetched external id, or every fetched profile link.
func (d *planDraft) fetchedPersonList(name string, externalIDs []json.RawMessage, links any) bool {
	var values any = externalIDs
	present := len(externalIDs) > 0
	if name == "links" {
		list, _ := links.([]any)
		values, present = list, len(list) > 0
	}
	if !present {
		return false
	}
	d.body(name, values)
	d.fetchedField(name, values)
	return true
}

// editVideoThumbnail edits the target video's thumbnail with the instruction, from the frame at atMs or, without
// it, from the current thumbnail; the worker runs the session's stages.
func editVideoThumbnail(_ context.Context, d *planDraft) (bool, error) {
	if !d.need(strings.TrimSpace(d.args.Instruction) != "") {
		return false, nil
	}
	if d.args.AtMs != nil {
		d.body("positionMs", *d.args.AtMs)
		d.literal(domain.AssistantArgAtMs, *d.args.AtMs)
	}
	d.body("prompt", d.args.Instruction)
	d.literal(domain.AssistantArgInstruction, d.args.Instruction)
	return true, nil
}
