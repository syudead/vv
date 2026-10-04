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

// /mcp の MCP サーバー（specs/026-external-api/contracts/mcp.md、research.md R-8）。
//
// 転送は Streamable HTTP の stateless で、POST /mcp だけを受けて application/json で返す
// （GET・DELETE は SDK が 405 にする）。認証は境界（auth.go の Bearer の扱い）が済ませていて、
// ここに届く要求は API トークンで確かめた所有者のものだけである。
//
// ツールは外部連携 API の操作と 1 対 1 で、REST の経路と同じ externalServer のハンドラを
// 呼ぶ。引数を同じ操作の問い合わせと本文に組み立ててハンドラに渡し、応答の本文をそのまま
// structured content にする。2xx 以外は isError: true で、本文は外部連携 API の誤りと同じ
// { code, message, reason?, limit?, index? } である。引数の解釈・上限・誤りの写し方を REST と
// 二重に持たないための形である。

// newMCPHandler は /mcp に載せるハンドラを返す。
func newMCPHandler(srv *server) http.Handler {
	api := http.NewServeMux()
	registerExternalAPI(api, srv)
	tools := &mcpTools{api: api}

	server := mcp.NewServer(&mcp.Implementation{Name: "vv", Version: srv.build.Version}, nil)
	tools.register(server)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		// SDK の既定の DNS rebinding の防御は、localhost で受けた要求の Host が localhost で
		// なければ 403 にする。同じ PC のリバースプロキシ越しの公開（Host は公開名）を断って
		// しまう。/mcp は Authorization: Bearer がなければ境界が 401 にし、ブラウザは
		// Authorization を自動では付けないので、rebinding で所有者の権限は得られない
		// （同一オリジンの検査をかけないのと同じ理由、research.md R-4）。
		DisableLocalhostProtection: true,
		// 本文の上限は外部連携 API の本文（externalBodyLimit）に JSON-RPC の包みの分を足す。
		// update_video_tags に REST と同じ件数を渡せるようにする。
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

// mcpRequestWatch は /mcp の HTTP 要求が打ち切られたときに cancel を呼ぶよう登録し、登録を
// 外す関数を返す。SDK はツールの context を要求の取り消しから切り離す（値は引き継ぐ）ので、
// 失効・資格情報の変更で境界が要求を打ち切ったとき（research.md R-9）にツールも止まるよう、
// call がこれで取り消しをつなぐ。
type mcpRequestWatch func(cancel func(error)) (stop func() bool)

// mcpRequestWatchKey は mcpRequestWatch を context に載せる鍵である。
type mcpRequestWatchKey struct{}

// mcpTools はツールの呼び出しを外部連携 API のハンドラへ渡す。
type mcpTools struct {
	// api は外部連携 API の経路だけを載せた ServeMux である。境界は通さない（/mcp の要求が
	// 既に通っている）。
	api http.Handler
}

// 入力の形。外部連携 API の同じ操作の引数・本文と同じ JSON にする。
type (
	mcpNoInput     struct{}
	mcpListVideos  = extgen.ListVideosParams
	mcpLookupVideo = extgen.LookupVideoParams
	mcpListTags    = extgen.ListTagsParams
)

// mcpListTagsDefaultLimit は list_tags が limit を省かれたときに入れる 1 ページの件数である。
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
			"(GET /api/v1/tags). q matches a name or synonym ignoring width, case and kana; tentative and unused " +
			"narrow the list; sort is name, countDesc, countAsc, createdDesc or createdAsc. " +
			"limit is 1 to 200 and defaults to 100 here. total counts the matching tags and totalAll every tag. " +
			"Pass nextCursor back as cursor, with the same q, tentative, unused and sort, until it is absent.",
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
		if in.Sort != nil {
			query.Set("sort", string(*in.Sort))
		}
		if in.Cursor != nil {
			query.Set("cursor", *in.Cursor)
		}
		// REST の既定（省けば全件）は変えず、ツールだけが 1 ページを既定にする
		// （specs/039-external-tag-admin/research.md R-1）。
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
}

// videoTagsInputSchema は update_video_tags の入力の形である。型から導き、action に
// 契約の値を足す（型からは文字列としか分からない）。
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

// tagSynonymsInputSchema は update_tag_synonyms の入力の形である。型から導き、action に契約の値を
// 足す（update_video_tags と同じやり方）。
func tagSynonymsInputSchema() *jsonschema.Schema {
	schema := inputSchemaFor[extgen.TagSynonymsRequest]("update_tag_synonyms")
	if action := schema.Properties["action"]; action != nil {
		action.Enum = []any{string(extgen.TagSynonymsRequestActionAdd), string(extgen.TagSynonymsRequestActionRemove)}
	}
	return schema
}

// listTagsInputSchema は list_tags の入力の形である。型から導き、sort に契約の値を、
// limit と q に上限を足す（型からは文字列・整数としか分からない）。
func listTagsInputSchema() *jsonschema.Schema {
	schema := inputSchemaFor[mcpListTags]("list_tags")
	if sort := schema.Properties["sort"]; sort != nil {
		sort.Enum = []any{
			string(extgen.Name), string(extgen.CountDesc), string(extgen.CountAsc),
			string(extgen.CreatedDesc), string(extgen.CreatedAsc),
		}
	}
	if limit := schema.Properties["limit"]; limit != nil {
		minimum, maximum := 1.0, float64(domain.MaxTagPageLimit)
		limit.Minimum, limit.Maximum = &minimum, &maximum
		limit.Default = json.RawMessage(strconv.Itoa(mcpListTagsDefaultLimit))
	}
	if q := schema.Properties["q"]; q != nil {
		maxLength := maxQueryLength
		q.MaxLength = &maxLength
	}
	return schema
}

// inputSchemaFor は一括操作のツールの入力の形を型から導く。
func inputSchemaFor[T any](tool string) *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("%s input schema: %v", tool, err))
	}
	return schema
}

// call は外部連携 API の操作を呼び、応答をツールの結果にする。
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

// toolResponse は外部連携 API のハンドラの応答を受け取る http.ResponseWriter である。
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
