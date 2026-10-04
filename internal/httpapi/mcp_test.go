package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/store"
)

// /mcp の MCP サーバー（specs/026-external-api/contracts/mcp.md）を、公式の SDK のクライアントと
// 本物の保存先・Bearer の境界で確かめる。

// bearerTransport は要求に Authorization: Bearer を付ける。
type bearerTransport struct{ secret string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.secret)
	return http.DefaultTransport.RoundTrip(r)
}

type mcpFixture struct {
	env    *authEnv
	owner  *http.Cookie
	secret string
	url    string
	videoA int64
	pathA  string
}

func newMCPFixture(t *testing.T, opts Options) mcpFixture {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		library := db.Library()
		if opts.Tags == nil {
			opts.Tags = db.Tags()
		}
		opts.Videos, opts.ExternalVideos = library, library
		if opts.Overrides == nil {
			opts.Overrides = db.Overrides()
		}
		if opts.ThumbnailPicker == nil {
			opts.ThumbnailPicker = &fakeThumbnailPicker{db: db}
		}
		return opts
	})
	f := mcpFixture{env: env, owner: env.setup()}
	f.secret = env.createAPIToken(f.owner, "agent").Secret
	dir := t.TempDir()
	ctx := context.Background()
	if _, err := env.db.Settings().AddMediaFolder(ctx, dir); err != nil {
		t.Fatal(err)
	}
	f.pathA = filepath.Join(dir, "a.mp4")
	result, err := env.db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: f.pathA, Title: "a", ContentKey: "key-a", SizeBytes: 1,
		MTime: time.Unix(0, 0), AddedAt: time.Unix(1000, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.videoA = result.ID
	server := httptest.NewServer(env.handler)
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

func (f mcpFixture) connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "vv-test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   f.url + "/mcp",
		HTTPClient: &http.Client{Transport: bearerTransport{secret: f.secret}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// callTool はツールを呼び、structured content を out に読む。isError は結果の isError。
func callTool(t *testing.T, session *mcp.ClientSession, name string, args any, out any) (isError bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s: %v: %s", name, err, raw)
	}
	if len(result.Content) != 1 {
		t.Fatalf("%s: content = %d 個", name, len(result.Content))
	}
	if text, ok := result.Content[0].(*mcp.TextContent); !ok || !json.Valid([]byte(text.Text)) {
		t.Errorf("%s: 文の content が JSON でない: %#v", name, result.Content[0])
	}
	return result.IsError
}

// SDK のクライアントから接続すると 14 のツールが契約の注記つきで並び、update_video_tags の
// 結果が REST の lookup に、update_video_display_names の結果が get_video に出る。
func TestMCPToolsMatchExternalAPI(t *testing.T) {
	scans := &fakeScans{}
	f := newMCPFixture(t, Options{Scans: scans, Build: domain.BuildInfo{Version: "1.2.3"}})
	session := f.connect(t)

	if got := session.InitializeResult().ServerInfo; got.Name != "vv" || got.Version != "1.2.3" {
		t.Errorf("serverInfo = %+v", got)
	}
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	type hints struct{ readOnly, destructive, idempotent bool }
	want := map[string]hints{
		"list_videos":       {readOnly: true},
		"get_video":         {readOnly: true},
		"list_tags":         {readOnly: true},
		"get_current_scan":  {readOnly: true},
		"update_video_tags": {destructive: true, idempotent: true},
		"start_scan":        {},
		// specs/029-video-overrides/contracts/external-api.md §3
		"update_video_display_names": {destructive: true, idempotent: true},
		"update_video_thumbnails":    {destructive: true, idempotent: true},
		// specs/039-external-tag-admin/contracts/external-api.md §8
		"merge_tags":               {destructive: true, idempotent: true},
		"rename_tag":               {destructive: false, idempotent: true},
		"update_tag_synonyms":      {destructive: true, idempotent: true},
		"batch_tags":               {destructive: true, idempotent: true},
		"list_rejected_tag_names":  {readOnly: true},
		"forget_rejected_tag_name": {destructive: true, idempotent: true},
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		w, ok := want[tool.Name]
		if !ok || tool.Annotations == nil {
			t.Errorf("tool %q: annotations = %+v", tool.Name, tool.Annotations)
			continue
		}
		got := hints{readOnly: tool.Annotations.ReadOnlyHint, idempotent: tool.Annotations.IdempotentHint}
		if !w.readOnly {
			if tool.Annotations.DestructiveHint == nil {
				t.Errorf("tool %q: destructiveHint が無い", tool.Name)
			} else {
				got.destructive = *tool.Annotations.DestructiveHint
			}
		}
		if got != w {
			t.Errorf("tool %q: hints = %+v, want %+v", tool.Name, got, w)
		}
	}
	if len(names) != len(want) || len(names) != 14 {
		t.Errorf("tools = %v", names)
	}

	// 名前で付けたタグが REST の lookup に出る。
	var tagged extgen.VideoTagsResponse
	args := map[string]any{"videos": []any{map[string]any{"path": f.pathA}}, "action": "add", "tags": []string{"from-mcp"}}
	if callTool(t, session, "update_video_tags", args, &tagged) {
		t.Fatalf("update_video_tags が誤り: %+v", tagged)
	}
	if len(tagged.Items) != 1 || tagged.Items[0].Video.Id != f.videoA || len(tagged.Items[0].Tags) != 1 ||
		tagged.Items[0].Tags[0].Name != "from-mcp" || !tagged.Items[0].Tags[0].Manual {
		t.Errorf("update_video_tags = %+v", tagged)
	}
	rec := f.env.serve(authRequest{
		method: http.MethodGet, target: "/api/v1/videos/lookup?id=" + strconv.FormatInt(f.videoA, 10), header: bearer(f.secret),
	})
	var looked extgen.ExternalVideo
	if err := json.Unmarshal(rec.Body.Bytes(), &looked); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("lookup: %d %s", rec.Code, rec.Body)
	}
	if len(looked.Tags) != 1 || looked.Tags[0].Name != "from-mcp" {
		t.Errorf("lookup の tags = %+v", looked.Tags)
	}

	// 読み出しのツールは REST の本文と同じ形を返す。
	var page extgen.ExternalVideoPage
	if callTool(t, session, "list_videos", map[string]any{"limit": 1}, &page) ||
		len(page.Items) != 1 || page.Items[0].Id != f.videoA || page.NextCursor != "" {
		t.Errorf("list_videos = %+v", page)
	}
	var video extgen.ExternalVideo
	if callTool(t, session, "get_video", map[string]any{"contentKey": "key-a"}, &video) ||
		video.Id != f.videoA || len(video.Locations) != 1 || video.Locations[0].Path != f.pathA {
		t.Errorf("get_video = %+v", video)
	}
	if video.Title != "a" || video.FileTitle != "a" || video.DisplayName != nil || video.ThumbnailPositionMs != nil {
		t.Errorf("get_video の上書きの項目 = title %q, fileTitle %q, displayName %v, thumbnailPositionMs %v",
			video.Title, video.FileTitle, video.DisplayName, video.ThumbnailPositionMs)
	}

	// 構造化された出力に updatedAt・fileCreatedAt が出る（specs/033-video-dates/contracts/
	// external-api.md §1）。
	var rawVideo map[string]json.RawMessage
	callTool(t, session, "get_video", map[string]any{"id": f.videoA}, &rawVideo)
	for _, key := range []string{"updatedAt", "fileCreatedAt"} {
		if _, ok := rawVideo[key]; !ok {
			t.Errorf("get_video の出力に %s が無い: %v", key, rawVideo)
		}
	}
	if !video.UpdatedAt.After(video.AddedAt) || !video.FileCreatedAt.Equal(time.Unix(0, 0)) {
		t.Errorf("get_video: updatedAt %v, addedAt %v, fileCreatedAt %v", video.UpdatedAt, video.AddedAt, video.FileCreatedAt)
	}

	// 表示名を付けると get_video と list_videos に出る。
	var named extgen.VideoDisplayNamesResponse
	if callTool(t, session, "update_video_display_names", map[string]any{"items": []any{
		map[string]any{"video": map[string]any{"path": f.pathA}, "displayName": "From MCP"},
	}}, &named) || len(named.Items) != 1 || named.Items[0].Title != "From MCP" || named.Items[0].FileTitle != "a" {
		t.Errorf("update_video_display_names = %+v", named)
	}
	video = extgen.ExternalVideo{}
	if callTool(t, session, "get_video", map[string]any{"id": f.videoA}, &video) ||
		video.Title != "From MCP" || video.DisplayName == nil || *video.DisplayName != "From MCP" || video.FileTitle != "a" {
		t.Errorf("表示名の後の get_video = %+v", video)
	}
	page = extgen.ExternalVideoPage{}
	if callTool(t, session, "list_videos", nil, &page) || len(page.Items) != 1 || page.Items[0].Title != "From MCP" {
		t.Errorf("表示名の後の list_videos = %+v", page)
	}
	// null で解除する。
	named = extgen.VideoDisplayNamesResponse{}
	if callTool(t, session, "update_video_display_names", map[string]any{"items": []any{
		map[string]any{"video": map[string]any{"id": f.videoA}, "displayName": nil},
	}}, &named) || len(named.Items) != 1 || named.Items[0].DisplayName != nil || named.Items[0].Title != "a" {
		t.Errorf("update_video_display_names の解除 = %+v", named)
	}

	var tags extgen.TagList
	if callTool(t, session, "list_tags", nil, &tags) ||
		len(tags.Items) != 1 || tags.Items[0].Name != "from-mcp" || tags.Items[0].VideoCount != 1 {
		t.Errorf("list_tags = %+v", tags)
	}
	var started, current extgen.ExternalScan
	if callTool(t, session, "start_scan", nil, &started) || started.Id == 0 || scans.started != 1 {
		t.Errorf("start_scan = %+v", started)
	}
	if callTool(t, session, "get_current_scan", nil, &current) || current.Id != started.Id {
		t.Errorf("get_current_scan = %+v", current)
	}
}

// 操作の誤りは isError: true と、外部連携 API の誤りと同じ本文で返る。
func TestMCPToolErrorsUseExternalErrorBody(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	session := f.connect(t)

	var body extgen.Error
	args := map[string]any{
		"videos": []any{map[string]any{"id": f.videoA}, map[string]any{"contentKey": "missing"}},
		"action": "add", "tags": []string{"never"},
	}
	if !callTool(t, session, "update_video_tags", args, &body) {
		t.Fatal("引けない動画で isError にならない")
	}
	if body.Code != extgen.ErrorCodeNotFound || body.Reason == nil || *body.Reason != extgen.VideoNotFound ||
		body.Index == nil || *body.Index != 1 || body.Message == "" {
		t.Errorf("誤りの本文 = %+v", body)
	}
	var tags extgen.TagList
	if callTool(t, session, "list_tags", nil, &tags) || len(tags.Items) != 0 {
		t.Errorf("何も反映しないはずが tags = %+v", tags)
	}

	body = extgen.Error{}
	if !callTool(t, session, "update_video_tags", map[string]any{
		"videos": []any{map[string]any{"id": f.videoA}}, "action": "add", "tags": []string{" "},
	}, &body) || body.Reason == nil || *body.Reason != extgen.TagNameEmpty || body.Index == nil || *body.Index != 0 {
		t.Errorf("空の名前: %+v", body)
	}

	body = extgen.Error{}
	if !callTool(t, session, "get_video", map[string]any{"id": f.videoA, "path": f.pathA}, &body) ||
		body.Code != extgen.ErrorCodeInvalidRequest {
		t.Errorf("2 つの指定: %+v", body)
	}

	body = extgen.Error{}
	if !callTool(t, session, "list_videos", map[string]any{"cursor": "nope"}, &body) ||
		body.Reason == nil || *body.Reason != extgen.InvalidCursor {
		t.Errorf("不正なカーソル: %+v", body)
	}

	// サムネイルの位置は解析前の動画を 409 duration_unknown と index で断る。
	body = extgen.Error{}
	if !callTool(t, session, "update_video_thumbnails", map[string]any{"items": []any{
		map[string]any{"video": map[string]any{"id": f.videoA}, "positionMs": 1000},
	}}, &body) || body.Code != extgen.ErrorCodeConflict || body.Reason == nil || *body.Reason != extgen.DurationUnknown ||
		body.Index == nil || *body.Index != 0 {
		t.Errorf("解析前のサムネイル: %+v", body)
	}

	body = extgen.Error{}
	if !callTool(t, session, "get_current_scan", nil, &body) || body.Reason == nil || *body.Reason != extgen.NoScan {
		t.Errorf("走査が無い: %+v", body)
	}
}

// mcpCall は tools/call の JSON-RPC の本文である。stateless なので initialize を要らない。
const mcpListTagsCall = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_tags","arguments":{}}}`

func mcpRequest(header map[string]string) authRequest {
	h := map[string]string{"Accept": "application/json, text/event-stream"}
	for k, v := range header {
		h[k] = v
	}
	return authRequest{method: http.MethodPost, target: "/mcp", body: mcpListTagsCall, header: h}
}

// トークンが無い・無効・失効済みなら、MCP の処理に入る前に境界が 401 を返す。
func TestMCPRejectsMissingAndInvalidTokens(t *testing.T) {
	f := newMCPFixture(t, Options{})

	assertBearerUnauthenticated(t, "トークン無し", f.env.serve(mcpRequest(nil)))
	assertBearerUnauthenticated(t, "無効なトークン", f.env.serve(mcpRequest(bearer("vvt_"+strings.Repeat("A", 43)))))
	assertBearerUnauthenticated(t, "形式違い", f.env.serve(mcpRequest(bearer("not-a-token"))))
	cookieOnly := mcpRequest(nil)
	cookieOnly.cookies = []*http.Cookie{f.owner}
	assertBearerUnauthenticated(t, "Cookie だけ", f.env.serve(cookieOnly))

	created := f.env.createAPIToken(f.owner, "revoked")
	f.env.serve(authRequest{
		method: http.MethodDelete, target: "/api/api-tokens/" + strconv.FormatInt(created.Token.Id, 10),
		cookies: []*http.Cookie{f.owner},
	})
	assertBearerUnauthenticated(t, "失効済み", f.env.serve(mcpRequest(bearer(created.Secret))))

	// 有効なトークンなら JSON で答える。別の Origin でも通す（research.md R-4）。
	ok := mcpRequest(map[string]string{"Authorization": "Bearer " + f.secret, "Origin": "https://elsewhere.example"})
	rec := f.env.serve(ok)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("有効なトークン: %d %q %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
	var reply struct {
		Result mcp.CallToolResult `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || reply.Result.IsError {
		t.Errorf("tools/call: %s", rec.Body)
	}

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		rec := f.env.serve(authRequest{method: method, target: "/mcp", header: bearer(f.secret)})
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /mcp: status = %d", method, rec.Code)
		}
	}
}

// 同じ PC のリバースプロキシ越し（localhost で受け、Host は公開名）でも断らない。
func TestMCPAcceptsProxiedHost(t *testing.T) {
	f := newMCPFixture(t, Options{})
	req, err := http.NewRequest(http.MethodPost, f.url+"/mcp", strings.NewReader(mcpListTagsCall))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "vv.example"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+f.secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d", res.StatusCode)
	}
}

// 画面での失効は、そのトークンで実行中のツールを打ち切る（research.md R-9）。
func TestMCPRevokeAbortsInFlightTool(t *testing.T) {
	tags := blockingTags{entered: make(chan struct{}), ended: make(chan error, 1)}
	f := newMCPFixture(t, Options{Tags: tags})
	created := f.env.createAPIToken(f.owner, "in-flight")

	done := make(chan struct{})
	go func() {
		defer close(done)
		f.env.serve(mcpRequest(bearer(created.Secret)))
	}()
	<-tags.entered
	rec := f.env.serve(authRequest{
		method: http.MethodDelete, target: "/api/api-tokens/" + strconv.FormatInt(created.Token.Id, 10),
		cookies: []*http.Cookie{f.owner},
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("失効: status = %d", rec.Code)
	}
	select {
	case err := <-tags.ended:
		if err == nil {
			t.Fatal("失効で打ち切られなかった")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("失効で打ち切られなかった")
	}
	<-done
}
