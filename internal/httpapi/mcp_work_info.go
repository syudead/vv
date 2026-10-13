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

// 作品情報と人物・団体の MCP のツール（specs/047-video-work-info/contracts/external-api.md「MCP tools」）。
// 他のツールと同じく、外部連携 API の同じ操作を call で呼ぶ。
//
// 本文を取るツールは入力を json.RawMessage のまま REST へ渡す。生成型に読み直すと null と省略の区別が
// 失われ、update_person の aliases: null（一覧を空にする）や parentId: null（親を外す）が「今のまま」に
// 変わってしまうためである。入力の形（inputSchema）は本文の型から導く。

// mcpListCredits は list_people と list_organizations の入力で、GET /api/v1/people の問い合わせと同じ形である。
type mcpListCredits = extgen.ListPeopleParams

// mcpListCreditsDefaultLimit は list_people・list_organizations が limit を省かれたときに入れる 1 ページの
// 件数である（list_tags と同じ）。
const mcpListCreditsDefaultLimit = 100

// mcpCreditTools は人物と団体で違う、ツールの名前・経路・説明の言葉である。
type mcpCreditTools struct {
	// path は経路の頭（/people・/organizations）。
	path string
	// one と many は説明に使う単数・複数の名前、list・create・update・merge・remove はツールの名前。
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

// registerWorkInfo は update_video_work_info と、人物・団体の 5 つずつのツールを載せる。
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
	parent := ""
	if kind.one == "organization" {
		parent = " parentId sets the parent organization."
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: kind.list,
		Description: "List " + kind.many + " with their aliases, external IDs and video count, one page at a time " +
			"(GET /api/v1" + kind.path + "). q matches a name or alias ignoring width, case and kana; sort is name, " +
			"countDesc, countAsc, createdDesc or createdAsc; limit is 1 to 200 and defaults to 100 here. Pass nextCursor " +
			"back as cursor, with the same q and sort, until it is absent. Give source and externalId together instead " +
			"to look up the one " + kind.one + " holding that external ID: items is empty when there is none. Look up " +
			"before creating, and use the returned id in update_video_work_info.",
		InputSchema: creditListInputSchema(kind.list),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListCredits) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodGet, kind.path, creditListQuery(in), nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: kind.create,
		Description: "Create one " + kind.one + " with a name, aliases and external IDs (POST /api/v1" + kind.path + ")." +
			parent + " An external ID held by another " + kind.one + " is a conflict with reason external_id_taken; " +
			"look that " + kind.one + " up by the external ID and use its id instead. Each call creates a new " + kind.one + ".",
		InputSchema: inputSchemaFor[extgen.CreditInput](kind.create),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: notDestructive, IdempotentHint: false},
	}, t.post(kind.path))

	mcp.AddTool(server, &mcp.Tool{
		Name: kind.update,
		Description: "Change the " + kind.one + " named by id (POST /api/v1" + kind.path + "/update). Only the properties given " +
			"change; a given list replaces the stored one and null empties it." + parent + " Up to 50 aliases and 20 " +
			"external IDs.",
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

// post は入力をそのまま本文にして POST する、ツールの処理である。
func (t *mcpTools) post(path string) mcp.ToolHandlerFor[json.RawMessage, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in json.RawMessage) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, path, nil, in)
	}
}

// creditListQuery は list_people・list_organizations の入力を問い合わせにする。limit を省けば
// mcpListCreditsDefaultLimit にする（REST の既定の全件は変えない、list_tags と同じ）。
func creditListQuery(in mcpListCredits) url.Values {
	query := url.Values{}
	for name, value := range map[string]*string{"q": in.Q, "cursor": in.Cursor, "source": in.Source, "externalId": in.ExternalId} {
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

// creditListInputSchema は list_people・list_organizations の入力の形である（list_tags と同じ規則）。
func creditListInputSchema(tool string) *jsonschema.Schema {
	return withListInputLimits(inputSchemaFor[mcpListCredits](tool), mcpListCreditsDefaultLimit,
		string(extgen.CreditSortName), string(extgen.CreditSortCountDesc), string(extgen.CreditSortCountAsc),
		string(extgen.CreditSortCreatedDesc), string(extgen.CreditSortCreatedAsc))
}
