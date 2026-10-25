package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// ActionRunner calls the screen API handler of an assistant catalog row in process, as the owner,
// the way /mcp calls the external API mux; it implements app.ActionRunner.
type ActionRunner struct {
	// api is the generated mux without the auth and mutation boundaries: the caller is the server itself.
	api http.Handler
}

// assistantRoute is how a catalog operation is sent; tests keep it equal to api/openapi.yaml.
type assistantRoute struct {
	method string
	path   string
	body   bool
	// actionField receives the catalog row's action.
	actionField string
	// actionKeys are fields whose objects, or array elements, take the action as their key (add or remove);
	// a key of another action is refused.
	actionKeys []string
	// current lists the request fields copied from GET path before the call's own fields replace them.
	current []string
}

var assistantRoutes = map[string]assistantRoute{
	"addGroupItems":       {method: http.MethodPost, path: "/api/groups/{id}/items", body: true},
	"addPlaylistItems":    {method: http.MethodPost, path: "/api/playlists/{id}/items", body: true},
	"addTagSynonym":       {method: http.MethodPost, path: "/api/tags/{id}/synonyms", body: true},
	"applyVideoCodeEdits": {method: http.MethodPost, path: "/api/video-code-edits", body: true},
	"applyVideoEdits": {
		method: http.MethodPost, path: "/api/video-edits", body: true, actionKeys: []string{"tags", "credits"},
	},
	"autoTagVideo":                  {method: http.MethodPost, path: "/api/videos/{id}/auto-tag"},
	"batchTags":                     {method: http.MethodPost, path: "/api/tags/batch", body: true, actionField: "action"},
	"bundleVideos":                  {method: http.MethodPost, path: "/api/video-bundles", body: true},
	"changeSmartFolder":             {method: http.MethodPatch, path: "/api/smart-folders/{id}", body: true},
	"confirmFaceCandidate":          {method: http.MethodPost, path: "/api/people/{id}/face-candidates/{otherId}/confirm"},
	"confirmTag":                    {method: http.MethodPost, path: "/api/tags/{id}/confirm"},
	"confirmVideoTag":               {method: http.MethodPost, path: "/api/videos/{id}/tags/{tagId}/confirm"},
	"createGroup":                   {method: http.MethodPost, path: "/api/groups", body: true},
	"createOrganization":            {method: http.MethodPost, path: "/api/organizations", body: true},
	"createPerson":                  {method: http.MethodPost, path: "/api/people", body: true},
	"createPlaylist":                {method: http.MethodPost, path: "/api/playlists", body: true},
	"createSmartFolder":             {method: http.MethodPost, path: "/api/smart-folders", body: true},
	"createTag":                     {method: http.MethodPost, path: "/api/tags", body: true},
	"createTagKind":                 {method: http.MethodPost, path: "/api/tag-kinds", body: true},
	"createVirtualVideo":            {method: http.MethodPost, path: "/api/virtual-videos", body: true},
	"deleteCollections":             {method: http.MethodPost, path: "/api/collections/delete", body: true},
	"deleteOrganization":            {method: http.MethodDelete, path: "/api/organizations/{id}"},
	"deletePerson":                  {method: http.MethodDelete, path: "/api/people/{id}"},
	"deletePersonImage":             {method: http.MethodDelete, path: "/api/people/{id}/image"},
	"deletePlaylist":                {method: http.MethodDelete, path: "/api/playlists/{id}"},
	"deleteSmartFolder":             {method: http.MethodDelete, path: "/api/smart-folders/{id}"},
	"deleteTag":                     {method: http.MethodDelete, path: "/api/tags/{id}"},
	"deleteTagKind":                 {method: http.MethodDelete, path: "/api/tag-kinds/{id}"},
	"dismissFaceCandidate":          {method: http.MethodPost, path: "/api/people/{id}/face-candidates/{otherId}/dismiss"},
	"dismissVersionCandidate":       {method: http.MethodPost, path: "/api/version-candidates/dismiss", body: true},
	"dissolveGroup":                 {method: http.MethodDelete, path: "/api/groups/{id}"},
	"forgetRejectedTagName":         {method: http.MethodDelete, path: "/api/tags/rejected-names"},
	"makeRepresentativeVersion":     {method: http.MethodPost, path: "/api/videos/{id}/make-representative"},
	"mergeOrganizations":            {method: http.MethodPost, path: "/api/organizations/{id}/merge", body: true},
	"mergePeople":                   {method: http.MethodPost, path: "/api/people/{id}/merge", body: true},
	"mergeTag":                      {method: http.MethodPost, path: "/api/tags/{id}/merge", body: true},
	"mergeVirtualVideo":             {method: http.MethodPost, path: "/api/virtual-videos/{id}/merge", body: true},
	"moveGroupItem":                 {method: http.MethodPost, path: "/api/groups/{id}/items/{itemId}/move", body: true},
	"movePlaylistItem":              {method: http.MethodPost, path: "/api/playlists/{id}/items/{itemId}/move", body: true},
	"rejectTag":                     {method: http.MethodPost, path: "/api/tags/{id}/reject"},
	"rejectVideoTag":                {method: http.MethodPost, path: "/api/videos/{id}/tags/{tagId}/reject"},
	"rejudgeVersionCandidate":       {method: http.MethodPost, path: "/api/version-candidates/rejudge", body: true},
	"removeGroupItems":              {method: http.MethodPost, path: "/api/groups/{id}/items/remove", body: true},
	"removePlaylistItem":            {method: http.MethodDelete, path: "/api/playlists/{id}/items/{itemId}"},
	"removeTagSynonym":              {method: http.MethodDelete, path: "/api/tags/{id}/synonyms"},
	"renameGroup":                   {method: http.MethodPatch, path: "/api/groups/{id}", body: true},
	"renamePlaylist":                {method: http.MethodPatch, path: "/api/playlists/{id}", body: true},
	"renameTag":                     {method: http.MethodPatch, path: "/api/tags/{id}", body: true},
	"renameTagKind":                 {method: http.MethodPatch, path: "/api/tag-kinds/{id}", body: true},
	"reopenFaceCandidate":           {method: http.MethodPost, path: "/api/people/{id}/face-candidates/{otherId}/reopen"},
	"reprobeVideo":                  {method: http.MethodPost, path: "/api/videos/{id}/probe"},
	"setFolderGrouping":             {method: http.MethodPut, path: "/api/folders/{rootId}/grouping", body: true},
	"setTagKindCardFirst":           {method: http.MethodPut, path: "/api/tag-kinds/{id}/card-first", body: true},
	"setVideoDisplayName":           {method: http.MethodPut, path: "/api/videos/{id}/display-name", body: true},
	"setVideoThumbnailPosition":     {method: http.MethodPut, path: "/api/videos/{id}/thumbnail-position", body: true},
	"setVideoWorkInfo":              {method: http.MethodPut, path: "/api/videos/{id}/work-info", body: true},
	"startAutoTagging":              {method: http.MethodPost, path: "/api/auto-tagging/runs", body: true},
	"startFaceRecognition":          {method: http.MethodPost, path: "/api/face-recognition/runs", body: true},
	"startRescan":                   {method: http.MethodPost, path: "/api/scans/rescan", body: true},
	"startScan":                     {method: http.MethodPost, path: "/api/scans", body: true},
	"tagGroup":                      {method: http.MethodPost, path: "/api/groups/{id}/tag"},
	"unbundleVideo":                 {method: http.MethodPost, path: "/api/videos/{id}/unbundle"},
	"updateFavorites":               {method: http.MethodPut, path: "/api/favorites", body: true},
	"updateOrganization":            {method: http.MethodPatch, path: "/api/organizations/{id}", body: true},
	"updatePerson":                  {method: http.MethodPatch, path: "/api/people/{id}", body: true},
	"updateVideoTags":               {method: http.MethodPost, path: "/api/video-tags", body: true, actionField: "action"},
	"updateVideoVisibility":         {method: http.MethodPut, path: "/api/video-visibility", body: true},
	"updateAutoImportSettings":      settingsRoute("/api/settings/auto-import", "enabled"),
	"updateAutoTaggingSettings":     settingsRoute("/api/settings/auto-tagging", "enabled", "afterImport", "threshold", "tentativeThreshold"),
	"updateComfyUISettings":         settingsRoute("/api/settings/comfyui", "provider", "url", "model"),
	"updateFaceRecognitionSettings": settingsRoute("/api/settings/face-recognition", "enabled", "recognizer"),
	"updateOllamaSettings":          settingsRoute("/api/settings/ollama", "endpoint", "model", "generationModel"),
	"updateTranscodingSettings":     settingsRoute("/api/settings/transcoding", "videoEncoder"),
	"updateVersionJudgingSettings":  settingsRoute("/api/settings/version-judging", "enabled"),
}

func settingsRoute(path string, current ...string) assistantRoute {
	return assistantRoute{method: http.MethodPut, path: path, body: true, current: current}
}

// Run implements app.ActionRunner.
func (r *ActionRunner) Run(ctx context.Context, call domain.AssistantCall) (domain.AssistantCallResult, error) {
	row, ok := domain.LookupAssistantAction(call.ActionID)
	if !ok {
		return domain.AssistantCallResult{}, fmt.Errorf("%w: %q", domain.ErrAssistantActionUnknown, call.ActionID)
	}
	route, ok := assistantRoutes[row.Operation]
	if !ok {
		return domain.AssistantCallResult{}, fmt.Errorf("%w: %q has no route", domain.ErrAssistantActionUnknown, row.ID)
	}
	target, err := route.target(call)
	if err != nil {
		return domain.AssistantCallResult{}, fmt.Errorf("assistant action %s: %w", row.ID, err)
	}
	if !route.body {
		return r.send(ctx, route.method, target, nil)
	}
	body := map[string]any{}
	if len(route.current) > 0 {
		if body, err = r.current(ctx, target, route.current); err != nil {
			return domain.AssistantCallResult{}, err
		}
	}
	maps.Copy(body, call.Body)
	if route.actionField != "" {
		body[route.actionField] = row.Action
	}
	if err := route.checkActionKeys(row.Action, body); err != nil {
		return domain.AssistantCallResult{}, fmt.Errorf("assistant action %s: %w", row.ID, err)
	}
	return r.send(ctx, route.method, target, body)
}

var assistantActionKeys = []string{"add", "remove"}

func (route assistantRoute) checkActionKeys(action string, body map[string]any) error {
	for _, field := range route.actionKeys {
		var edits []any
		switch value := body[field].(type) {
		case []any:
			edits = value
		case nil:
		default:
			edits = []any{value}
		}
		for _, edit := range edits {
			fields, _ := edit.(map[string]any)
			for _, key := range assistantActionKeys {
				if _, ok := fields[key]; ok && key != action {
					return fmt.Errorf("%s holds %s, which is not the action %s", field, key, action)
				}
			}
		}
	}
	return nil
}

// target fills the path parameters, escaping each value, and appends the query.
func (route assistantRoute) target(call domain.AssistantCall) (string, error) {
	var path strings.Builder
	rest := route.path
	for {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			path.WriteString(rest)
			break
		}
		end := strings.IndexByte(rest, '}')
		name := rest[start+1 : end]
		value, ok := call.Path[name]
		if !ok || value == "" {
			return "", fmt.Errorf("path parameter %s is missing", name)
		}
		path.WriteString(rest[:start])
		path.WriteString(url.PathEscape(value))
		rest = rest[end+1:]
	}
	if len(call.Query) == 0 {
		return path.String(), nil
	}
	query := url.Values{}
	for key, value := range call.Query {
		query.Set(key, value)
	}
	return path.String() + "?" + query.Encode(), nil
}

// current reads the settings and keeps the fields the update request takes.
func (r *ActionRunner) current(ctx context.Context, target string, fields []string) (map[string]any, error) {
	result, err := r.send(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(result.Body, &settings); err != nil {
		return nil, fmt.Errorf("read the current settings at %s: %w", target, err)
	}
	body := map[string]any{}
	for _, field := range fields {
		if value, ok := settings[field]; ok {
			body[field] = value
		}
	}
	return body, nil
}

func (r *ActionRunner) send(ctx context.Context, method, target string, body map[string]any) (domain.AssistantCallResult, error) {
	var reader io.Reader = http.NoBody
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return domain.AssistantCallResult{}, fmt.Errorf("encode the request of %s %s: %w", method, target, err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(withAudience(ctx, domain.AudienceOwner), method, target, reader)
	if err != nil {
		return domain.AssistantCallResult{}, fmt.Errorf("build %s %s: %w", method, target, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res := &capturedResponse{header: http.Header{}}
	r.api.ServeHTTP(res, req)
	if res.status >= 200 && res.status < 300 {
		return domain.AssistantCallResult{Status: res.status, Body: res.body.Bytes()}, nil
	}
	return domain.AssistantCallResult{}, callError(res)
}

// callError keeps the screen API's code and reason so the proposal row can show them.
func callError(res *capturedResponse) *domain.AssistantCallError {
	failure := &domain.AssistantCallError{Status: res.status, Code: string(codeInternal)}
	var payload struct {
		Code    string `json:"code"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(res.body.Bytes(), &payload); err != nil || payload.Code == "" {
		failure.Message = strings.TrimSpace(res.body.String())
		return failure
	}
	failure.Code, failure.Reason, failure.Message = payload.Code, payload.Reason, payload.Message
	return failure
}

// capturedResponse records a handler's response; the runner reads it once the handler returns.
type capturedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *capturedResponse) Header() http.Header { return c.header }

func (c *capturedResponse) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *capturedResponse) Write(p []byte) (int, error) {
	c.WriteHeader(http.StatusOK)
	return c.body.Write(p)
}
