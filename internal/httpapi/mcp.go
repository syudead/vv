package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// MCP uses stateless Streamable HTTP and authenticated external handlers; REST responses become structured content, with non-2xx marked isError to share validation and error mapping.

func newMCPHandler(srv *server) http.Handler {
	api := http.NewServeMux()
	registerExternalAPI(api, srv)
	tools := &mcpTools{api: api}

	server := mcp.NewServer(&mcp.Implementation{Name: "vv", Version: srv.build.Version}, nil)
	tools.register(server)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		// Disable SDK Host rebinding rejection for local reverse proxies; mandatory explicit Bearer authentication prevents browsers from acquiring owner authority through rebinding.
		DisableLocalhostProtection: true,
		// Add JSON-RPC envelope overhead to the REST body cap so both accept the same batch size.
		MaxRequestBodyBytes: externalBodyLimit + 64<<10,
		Logger:              srv.logger,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCtx := r.Context()
		watch := mcpRequestWatch(func(cancel func(error)) func() bool {
			return context.AfterFunc(requestCtx, func() { cancel(context.Cause(requestCtx)) })
		})
		handler.ServeHTTP(w, r.WithContext(context.WithValue(requestCtx, mcpRequestWatchKey{}, watch)))
	})
}

// The SDK detaches tool cancellation from HTTP requests; reconnect it so token revocation stops running tools.
type mcpRequestWatch func(cancel func(error)) (stop func() bool)

type mcpRequestWatchKey struct{}

type mcpTools struct {
	// The internal API mux skips authentication because /mcp already passed the boundary.
	api http.Handler
}

type (
	mcpNoInput               struct{}
	mcpListVideos            = extgen.ListVideosParams
	mcpLookupVideo           = extgen.LookupVideoParams
	mcpListTags              = extgen.ListTagsParams
	mcpListRejectedTagNames  = extgen.ListRejectedTagNamesParams
	mcpForgetRejectedTagName = extgen.ForgetRejectedTagNameParams
)

const mcpListTagsDefaultLimit = 100

func (t *mcpTools) register(server *mcp.Server) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	destructive, notDestructive := true, false

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_videos",
		Description: "List the videos in the registered media folders in the order they were added " +
			"(GET /api/v1/videos). Pass nextCursor back as cursor until it is empty to read every video.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListVideos) (*mcp.CallToolResult, any, error) {
		query := url.Values{}
		if in.Cursor != nil {
			query.Set("cursor", *in.Cursor)
		}
		if in.Limit != nil {
			query.Set("limit", strconv.Itoa(*in.Limit))
		}
		return t.call(ctx, http.MethodGet, "/videos", query, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_video",
		Description: "Look up one video by exactly one of id, contentKey or path " +
			"(GET /api/v1/videos/lookup). path is the absolute path of a location, byte for byte.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpLookupVideo) (*mcp.CallToolResult, any, error) {
		query := url.Values{}
		if in.Id != nil {
			query.Set("id", strconv.FormatInt(*in.Id, 10))
		}
		if in.ContentKey != nil {
			query.Set("contentKey", *in.ContentKey)
		}
		if in.Path != nil {
			query.Set("path", *in.Path)
		}
		return t.call(ctx, http.MethodGet, "/videos/lookup", query, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_tags",
		Description: "List tags with their synonyms, video count and creation time, one page at a time " +
			"(GET /api/v1/tags). Each tag carries its kind, or null without one. q matches a name or synonym " +
			"ignoring width, case and kana; tentative and unused narrow the list; kind is a kind id from " +
			"list_tag_kinds for that kind's tags, or none for tags without a kind; sort is name, countDesc, countAsc, " +
			"createdDesc or createdAsc. limit is 1 to 200 and defaults to 100 here. total counts the matching tags " +
			"and totalAll every tag. Pass nextCursor back as cursor, with the same q, tentative, unused, kind and " +
			"sort, until it is absent.",
		InputSchema: listTagsInputSchema(),
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListTags) (*mcp.CallToolResult, any, error) {
		query := url.Values{}
		if in.Q != nil {
			query.Set("q", *in.Q)
		}
		if in.Tentative != nil {
			query.Set("tentative", strconv.FormatBool(*in.Tentative))
		}
		if in.Unused != nil {
			query.Set("unused", strconv.FormatBool(*in.Unused))
		}
		if in.Kind != nil {
			query.Set("kind", *in.Kind)
		}
		if in.Sort != nil {
			query.Set("sort", string(*in.Sort))
		}
		if in.Cursor != nil {
			query.Set("cursor", *in.Cursor)
		}
		// Only MCP defaults to one page; REST retains its all-results default.
		limit := mcpListTagsDefaultLimit
		if in.Limit != nil {
			limit = *in.Limit
		}
		query.Set("limit", strconv.Itoa(limit))
		return t.call(ctx, http.MethodGet, "/tags", query, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "merge_tags",
		Description: "Merge source tags into a target tag in one transaction (POST /api/v1/tags/merge). " +
			"Each source's name and synonyms become synonyms of the target, its videos move to the target, " +
			"and the source is deleted; the target becomes a confirmed tag. sourceIds holds 1 to 20000 ids and " +
			"must not contain targetId (merge_same_tag). Missing sources are skipped and returned in notFoundIds; " +
			"a missing target is tag_not_found and changes nothing.",
		InputSchema: inputSchemaFor[extgen.TagMergeRequest]("merge_tags"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in extgen.TagMergeRequest) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, "/tags/merge", nil, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "rename_tag",
		Description: "Change a tag's original name (POST /api/v1/tags/rename). A tentative tag becomes confirmed " +
			"when its name changes. If the name is another tag's name or synonym, or one of this tag's synonyms, " +
			"the result is a conflict with reason tag_name_taken and that tag's tagId and tagName, and nothing changes.",
		InputSchema: inputSchemaFor[extgen.TagRenameRequest]("rename_tag"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &notDestructive, IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in extgen.TagRenameRequest) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, "/tags/rename", nil, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_tag_synonyms",
		Description: "Add a name to a tag's synonyms or remove one (POST /api/v1/tags/synonyms), returning the tag " +
			"after the change. add confirms a tentative tag. If the name is this tag's own name or another tag's " +
			"synonym, add fails with tag_name_taken. If the name is another tag's original name, add fails with " +
			"tag_merge_required and that tag's tagId and tagName, and nothing changes; call again with mergeTagId " +
			"set to that tagId to merge that tag into this one. remove of a name that is not a synonym changes nothing.",
		InputSchema: tagSynonymsInputSchema(),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in extgen.TagSynonymsRequest) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, "/tags/synonyms", nil, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "batch_tags",
		Description: "Confirm, reject or delete tags, or set or clear their kind, in one transaction " +
			"(POST /api/v1/tags/batch). confirm and reject apply to tentative tags, delete to confirmed tags. reject " +
			"deletes the tag and adds its name to the rejected names, so a later tentative attach of that name is " +
			"skipped; delete does not remember the name. setKind gives each tag the kind kindId (an id from " +
			"list_tag_kinds; a missing kind is tag_kind_not_found and changes nothing) and clearKind removes each " +
			"tag's kind; neither confirms a tentative tag, and kindId is sent with setKind only. ids holds 1 to 20000 " +
			"ids; for one tag send ids: [id]. The result splits the deduplicated ids into appliedIds, notFoundIds and " +
			"notApplicableIds (a tag the action does not apply to, such as one that already has the kind, left unchanged).",
		InputSchema: tagBatchInputSchema(),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, t.post("/tags/batch"))

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_rejected_tag_names",
		Description: "List the rejected tag names in natural name order, one page at a time " +
			"(GET /api/v1/tags/rejected-names). A tentative attach of a rejected name is skipped. limit is 1 to 200 " +
			"and defaults to 100; total counts every rejected name. Pass nextCursor back as cursor until it is absent.",
		InputSchema: rejectedTagNamesInputSchema(),
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListRejectedTagNames) (*mcp.CallToolResult, any, error) {
		query := url.Values{}
		if in.Cursor != nil {
			query.Set("cursor", *in.Cursor)
		}
		if in.Limit != nil {
			query.Set("limit", strconv.Itoa(*in.Limit))
		}
		return t.call(ctx, http.MethodGet, "/tags/rejected-names", query, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "forget_rejected_tag_name",
		Description: "Remove one name from the rejected tag names (DELETE /api/v1/tags/rejected-names), so the next " +
			"tentative attach of that name creates a tag again. The name is normalized as tag names are. The result is " +
			"the matched name and removed, which is false when the name was not rejected and nothing changed.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpForgetRejectedTagName) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodDelete, "/tags/rejected-names", url.Values{"name": {in.Name}}, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_video_tags",
		Description: "Add, remove or replace tags on videos by tag name or synonym (POST /api/v1/video-tags). " +
			"add creates missing tags; replace makes the manually added tags exactly the given set. " +
			"If any video cannot be found, nothing is changed. " +
			"With tentative: true, add and replace create new tags as tentative tags and skip rejected names, " +
			"returning them in skippedTags.",
		InputSchema: videoTagsInputSchema(),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in extgen.VideoTagsRequest) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, "/video-tags", nil, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_video_display_names",
		Description: "Set or clear the display names of videos (POST /api/v1/video-display-names). " +
			"A display name replaces the file-name title everywhere; null or a blank name clears it. " +
			"Up to 20000 items in one transaction; if any item fails, nothing is changed.",
		InputSchema: inputSchemaFor[extgen.VideoDisplayNamesRequest]("update_video_display_names"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in extgen.VideoDisplayNamesRequest) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, "/video-display-names", nil, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_video_thumbnails",
		Description: "Set or clear the representative thumbnail position of videos in milliseconds " +
			"(POST /api/v1/video-thumbnails). null returns to the automatic position. Up to 20 items. " +
			"Every item is checked first; then thumbnails are made in order, and if one cannot be made " +
			"the error's index tells where it stopped: the items before it were changed, that item and later ones were not.",
		InputSchema: inputSchemaFor[extgen.VideoThumbnailsRequest]("update_video_thumbnails"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in extgen.VideoThumbnailsRequest) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, "/video-thumbnails", nil, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "start_scan",
		Description: "Start scanning the media folders (POST /api/v1/scans). " +
			"If a scan is already running, it returns that scan instead of starting another.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &notDestructive, IdempotentHint: false},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoInput) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodPost, "/scans", nil, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_current_scan",
		Description: "Get the status of the latest scan (GET /api/v1/scans/current).",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoInput) (*mcp.CallToolResult, any, error) {
		return t.call(ctx, http.MethodGet, "/scans/current", nil, nil)
	})

	t.registerWorkInfo(server)
	t.registerTagKinds(server)
}

// Type-derived schemas need explicit action enum values.
func videoTagsInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[extgen.VideoTagsRequest](nil)
	if err != nil {
		panic(fmt.Sprintf("update_video_tags input schema: %v", err))
	}
	if action := schema.Properties["action"]; action != nil {
		action.Enum = []any{string(extgen.VideoTagsRequestActionAdd), string(extgen.VideoTagsRequestActionRemove),
			string(extgen.VideoTagsRequestActionReplace)}
	}
	return schema
}

func tagSynonymsInputSchema() *jsonschema.Schema {
	schema := inputSchemaFor[extgen.TagSynonymsRequest]("update_tag_synonyms")
	if action := schema.Properties["action"]; action != nil {
		action.Enum = []any{string(extgen.TagSynonymsRequestActionAdd), string(extgen.TagSynonymsRequestActionRemove)}
	}
	return schema
}

// Add enum, range, length, and kind constraints absent from the Go types.
func listTagsInputSchema() *jsonschema.Schema {
	schema := withListInputLimits(inputSchemaFor[mcpListTags]("list_tags"), mcpListTagsDefaultLimit,
		string(extgen.TagSortName), string(extgen.TagSortCountDesc), string(extgen.TagSortCountAsc),
		string(extgen.TagSortCreatedDesc), string(extgen.TagSortCreatedAsc))
	if kind := schema.Properties["kind"]; kind != nil {
		kind.Pattern = "^([1-9][0-9]*|none)$"
	}
	return schema
}

func withListInputLimits(schema *jsonschema.Schema, defaultLimit int, sorts ...any) *jsonschema.Schema {
	if sort := schema.Properties["sort"]; sort != nil {
		sort.Enum = sorts
	}
	if limit := schema.Properties["limit"]; limit != nil {
		minimum, maximum := 1.0, float64(domain.MaxTagPageLimit)
		limit.Minimum, limit.Maximum = &minimum, &maximum
		limit.Default = json.RawMessage(strconv.Itoa(defaultLimit))
	}
	if q := schema.Properties["q"]; q != nil {
		maxLength := maxQueryLength
		q.MaxLength = &maxLength
	}
	return schema
}

// Pass raw batch input through to preserve omitted versus null kindId semantics.
func tagBatchInputSchema() *jsonschema.Schema {
	schema := inputSchemaFor[extgen.TagBatchRequest]("batch_tags")
	if action := schema.Properties["action"]; action != nil {
		action.Enum = []any{string(extgen.Confirm), string(extgen.Reject), string(extgen.Delete),
			string(extgen.SetKind), string(extgen.ClearKind)}
	}
	return schema
}

func rejectedTagNamesInputSchema() *jsonschema.Schema {
	schema := inputSchemaFor[mcpListRejectedTagNames]("list_rejected_tag_names")
	if limit := schema.Properties["limit"]; limit != nil {
		minimum, maximum := 1.0, float64(domain.MaxTagPageLimit)
		limit.Minimum, limit.Maximum = &minimum, &maximum
		limit.Default = json.RawMessage(strconv.Itoa(rejectedTagNamePageDefaultLimit))
	}
	return schema
}

func inputSchemaFor[T any](tool string) *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("%s input schema: %v", tool, err))
	}
	return schema
}

func (t *mcpTools) call(ctx context.Context, method, path string, query url.Values, body any) (*mcp.CallToolResult, any, error) {
	if watch, ok := ctx.Value(mcpRequestWatchKey{}).(mcpRequestWatch); ok {
		var cancel context.CancelCauseFunc
		ctx, cancel = context.WithCancelCause(ctx)
		defer cancel(nil)
		defer watch(cancel)()
	}
	target := externalAPIBase + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("encode the arguments: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, nil, fmt.Errorf("build the request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := &toolResponse{header: http.Header{}, status: http.StatusOK}
	t.api.ServeHTTP(rec, req)

	raw := bytes.TrimSpace(rec.body.Bytes())
	if !json.Valid(raw) {
		return nil, nil, fmt.Errorf("the operation returned status %d without a JSON body", rec.status)
	}
	return &mcp.CallToolResult{
		IsError:           rec.status < 200 || rec.status >= 300,
		Content:           []mcp.Content{&mcp.TextContent{Text: string(raw)}},
		StructuredContent: json.RawMessage(raw),
	}, nil, nil
}

type toolResponse struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func (r *toolResponse) Header() http.Header { return r.header }

func (r *toolResponse) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status, r.wroteHeader = status, true
	}
}

func (r *toolResponse) Write(p []byte) (int, error) {
	r.wroteHeader = true
	return r.body.Write(p)
}
