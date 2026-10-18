package httpapi

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/syudead/vv/internal/httpapi/extgen"
)

// Forward raw tag-kind inputs to REST to preserve null versus omitted IDs.

func (t *mcpTools) registerTagKinds(server *mcp.Server) {
	destructive, notDestructive := true, false

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_tag_kinds",
		Description: "List every tag kind, such as genre or work, with its tag count, in id order (GET /api/v1/tag-kinds). " +
			"Use a kind's id as kind in list_tags or as kindId in batch_tags.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoInput) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodGet, "/tag-kinds", nil, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "create_tag_kind",
		Description: "Create one tag kind (POST /api/v1/tag-kinds). The name follows the tag name rules; a name " +
			"another kind has is a conflict with reason tag_kind_name_taken and changes nothing. Each call creates a " +
			"new kind.",
		InputSchema: inputSchemaFor[extgen.TagKindCreateRequest]("create_tag_kind"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &notDestructive, IdempotentHint: false},
	}, t.post("/tag-kinds"))

	mcp.AddTool(server, &mcp.Tool{
		Name: "rename_tag_kind",
		Description: "Rename the tag kind named by id (POST /api/v1/tag-kinds/update). The same name as now changes " +
			"nothing. A missing kind is tag_kind_not_found, and a name another kind has is tag_kind_name_taken.",
		InputSchema: inputSchemaFor[extgen.TagKindRenameRequest]("rename_tag_kind"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &notDestructive, IdempotentHint: true},
	}, t.post("/tag-kinds/update"))

	mcp.AddTool(server, &mcp.Tool{
		Name: "delete_tag_kinds",
		Description: "Delete tag kinds (POST /api/v1/tag-kinds/delete); their tags remain without a kind. ids holds " +
			"1 to 20000 ids. The result lists deletedIds and notFoundIds in ids order without duplicates.",
		InputSchema: inputSchemaFor[extgen.TagKindDeleteRequest]("delete_tag_kinds"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, t.post("/tag-kinds/delete"))
}
