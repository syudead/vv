package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/syudead/vv/internal/httpapi/extgen"
)

// Forward work-info tool bodies as RawMessage; decoding through generated structs would turn null clears/removals into omitted fields.

// MCP accepts integer limits, unlike REST's string parameter used to bypass parsing during external-ID lookup.
type mcpListCredits struct {
	Q          *extgen.CreditQuery      `json:"q,omitempty"`
	Sort       *extgen.CreditSort       `json:"sort,omitempty"`
	Cursor     *extgen.CreditCursor     `json:"cursor,omitempty"`
	Limit      *int                     `json:"limit,omitempty"`
	Source     *extgen.CreditSource     `json:"source,omitempty"`
	ExternalID *extgen.CreditExternalId `json:"externalId,omitempty"`
}

const mcpListCreditsDefaultLimit = 100

type mcpCreditTools struct {
	path                                string
	one, many                           string
	list, create, update, merge, remove string
}

var mcpCreditKinds = []mcpCreditTools{
	{path: "/people", one: "person", many: "people",
		list: "list_people", create: "create_person", update: "update_person", merge: "merge_people", remove: "delete_people"},
	{path: "/organizations", one: "organization", many: "organizations",
		list: "list_organizations", create: "create_organization", update: "update_organization",
		merge: "merge_organizations", remove: "delete_organizations"},
}

func (t *mcpTools) registerWorkInfo(server *mcp.Server) {
	destructive, notDestructive := true, false
	mcp.AddTool(server, &mcp.Tool{
		Name: "update_video_work_info",
		Description: "Replace the work information (code, description, dates, people, organizations, URLs and " +
			"external IDs) of videos (POST /api/v1/video-work-info). Each item's workInfo replaces the stored object: " +
			"an absent, null or blank property is cleared and {} clears everything. People and organizations are " +
			"referenced by id: find the id first with list_people or list_organizations by source and externalId, " +
			"and create the entity with create_person or create_organization only when none is found. " +
			"1 to 20000 items in one transaction; if any item fails, nothing is changed, and the error's index is the " +
			"item and field the property inside its workInfo. A later item for the same video wins.",
		InputSchema: inputSchemaFor[extgen.VideoWorkInfoRequest]("update_video_work_info"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, t.post("/video-work-info"))

	for _, kind := range mcpCreditKinds {
		t.registerCreditTools(server, kind, &destructive, &notDestructive)
	}
}

func (t *mcpTools) registerCreditTools(server *mcp.Server, kind mcpCreditTools, destructive, notDestructive *bool) {
	parent, profile, profileUpdate, ageSorts := "", "", "", ""
	if kind.one == "organization" {
		parent = " parentId sets the parent organization."
	} else {
		profile = " profile holds gender, birthDate and deathDate (YYYY, YYYY-MM or YYYY-MM-DD), birthplace, heightCm, " +
			"bustCm, waistCm, hipsCm, cup, bloodType, careerStartYear, careerEndYear, bio and up to 20 links."
		profileUpdate = profile + " Inside profile, each property given replaces that field and null clears it."
		ageSorts = ", or ageAsc (youngest first) or ageDesc, with people without a birth date last"
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: kind.list,
		Description: "List " + kind.many + " with their aliases, external IDs and video count, one page at a time " +
			"(GET /api/v1" + kind.path + "). q matches a name or alias ignoring width, case and kana; sort is name, " +
			"countDesc, countAsc, createdDesc or createdAsc" + ageSorts + "; limit is 1 to 200 and defaults to 100 here. Pass nextCursor " +
			"back as cursor, with the same q and sort, until it is absent. Give source and externalId together instead " +
			"to look up the one " + kind.one + " holding that external ID: items is empty when there is none. Look up " +
			"before creating, and use the returned id in update_video_work_info.",
		InputSchema: creditListInputSchema(kind),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListCredits) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodGet, kind.path, creditListQuery(in), nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: kind.create,
		Description: "Create one " + kind.one + " with a name, aliases and external IDs (POST /api/v1" + kind.path + ")." +
			parent + profile + " An external ID held by another " + kind.one + " is a conflict with reason external_id_taken; " +
			"look that " + kind.one + " up by the external ID and use its id instead. Each call creates a new " + kind.one + ".",
		InputSchema: inputSchemaFor[extgen.CreditInput](kind.create),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: notDestructive, IdempotentHint: false},
	}, t.post(kind.path))

	mcp.AddTool(server, &mcp.Tool{
		Name: kind.update,
		Description: "Change the " + kind.one + " named by id (POST /api/v1" + kind.path + "/update). Only the properties given " +
			"change; a given list replaces the stored one and null empties it." + parent + profileUpdate +
			" Up to 50 aliases and 20 external IDs.",
		InputSchema: inputSchemaFor[extgen.CreditUpdateRequest](kind.update),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: destructive, IdempotentHint: true},
	}, t.post(kind.path+"/update"))

	mcp.AddTool(server, &mcp.Tool{
		Name: kind.merge,
		Description: "Merge source " + kind.many + " into a target in one transaction (POST /api/v1" + kind.path + "/merge). " +
			"Each source's videos, names (as aliases) and external IDs move to the target and the source is deleted. " +
			"sourceIds holds 1 to 20000 ids and must not contain targetId; missing sources are returned in notFoundIds.",
		InputSchema: inputSchemaFor[extgen.CreditMergeRequest](kind.merge),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: destructive, IdempotentHint: true},
	}, t.post(kind.path+"/merge"))

	mcp.AddTool(server, &mcp.Tool{
		Name: kind.remove,
		Description: "Delete " + kind.many + " and their links to videos (POST /api/v1" + kind.path + "/delete). " +
			"ids holds 1 to 20000 ids. The result lists deletedIds and notFoundIds in ids order without duplicates.",
		InputSchema: inputSchemaFor[extgen.CreditDeleteRequest](kind.remove),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: destructive, IdempotentHint: true},
	}, t.post(kind.path+"/delete"))
}

func (t *mcpTools) post(path string) mcp.ToolHandlerFor[json.RawMessage, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in json.RawMessage) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, path, nil, in)
	}
}

// MCP credit lists default to one page while REST keeps its all-results default.
func creditListQuery(in mcpListCredits) url.Values {
	query := url.Values{}
	for name, value := range map[string]*string{"q": in.Q, "cursor": in.Cursor, "source": in.Source, "externalId": in.ExternalID} {
		if value != nil {
			query.Set(name, *value)
		}
	}
	if in.Sort != nil {
		query.Set("sort", string(*in.Sort))
	}
	limit := mcpListCreditsDefaultLimit
	if in.Limit != nil {
		limit = *in.Limit
	}
	query.Set("limit", strconv.Itoa(limit))
	return query
}

// Age sorts are offered for people only, matching the REST API's rejection of them for organizations.
func creditListInputSchema(kind mcpCreditTools) *jsonschema.Schema {
	sorts := []any{string(extgen.CreditSortName), string(extgen.CreditSortCountDesc), string(extgen.CreditSortCountAsc),
		string(extgen.CreditSortCreatedDesc), string(extgen.CreditSortCreatedAsc)}
	if kind.one == "person" {
		sorts = append(sorts, string(extgen.CreditSortAgeAsc), string(extgen.CreditSortAgeDesc))
	}
	return withListInputLimits(inputSchemaFor[mcpListCredits](kind.list), mcpListCreditsDefaultLimit, sorts...)
}
