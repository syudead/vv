package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// The runner reaches the real handlers for a read: the query reaches the service and the body is the handler's.
func TestActionRunnerReadsThroughTheScreenHandler(t *testing.T) {
	tags := &fakeTags{tags: []domain.Tag{{ID: 3, Name: "karaoke", Synonyms: []string{}}}}
	_, runner := NewServer(Options{Tags: tags, Auth: ownerAuth{}, Assets: emptyAssets{}})

	result, err := runner.Read(context.Background(), domain.AssistantRead{
		Operation: "listTags", Arguments: json.RawMessage(`{"q":"kara","limit":5}`),
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if tags.lastListQuery.Search != "kara" || tags.lastListQuery.Limit != 5 {
		t.Errorf("サービスに渡った条件 = %+v, want q=kara limit=5", tags.lastListQuery)
	}
	var page struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(result.Body, &page); err != nil || len(page.Items) != 1 || page.Items[0].Name != "karaoke" {
		t.Errorf("結果 = %s (%v), want ハンドラーのタグ一覧", result.Body, err)
	}
}

// An image operation answers a reference, and the reference reads back the handler's bytes and content type.
func TestActionRunnerAnswersAnImageAsAReferenceThatReadsBack(t *testing.T) {
	credits := &faceAppearanceCredits{crop: []byte("jpeg-bytes")}
	_, runner := NewServer(Options{Credits: credits, Auth: ownerAuth{}, Assets: emptyAssets{}})

	result, err := runner.Read(context.Background(), domain.AssistantRead{
		Operation: "getFaceAppearanceCrop", Arguments: json.RawMessage(`{"id":9}`),
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var reference struct {
		Image       string `json:"image"`
		ContentType string `json:"contentType"`
	}
	if err := json.Unmarshal(result.Body, &reference); err != nil {
		t.Fatalf("参照を読めない: %s", result.Body)
	}
	if reference.Image != "getFaceAppearanceCrop?id=9" || reference.ContentType != "image/jpeg" {
		t.Errorf("参照 = %+v, want getFaceAppearanceCrop?id=9 と image/jpeg", reference)
	}

	image, err := runner.Image(context.Background(), reference.Image)
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if string(image.Bytes) != "jpeg-bytes" || image.ContentType != "image/jpeg" {
		t.Errorf("画像 = %q %s, want ハンドラーのバイト列と image/jpeg", image.Bytes, image.ContentType)
	}
}

func TestActionRunnerRefusesReadsOutsideTheReadTools(t *testing.T) {
	cases := []struct {
		name, operation, args string
		want                  error
	}{
		{"対象外の操作", "getPluginProxySettings", `{}`, domain.ErrAssistantNotReadTool},
		{"書き込みの操作", "deleteTag", `{"id":1}`, domain.ErrAssistantNotReadTool},
		{"ない操作", "listEverything", `{}`, domain.ErrAssistantNotReadTool},
		{"スキーマに合わない引数", "listTags", `{"limit":"many"}`, domain.ErrAssistantInvalidArguments},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &stubAPI{}
			runner := &ActionRunner{api: api}
			_, err := runner.Read(context.Background(), domain.AssistantRead{
				Operation: tc.operation, Arguments: json.RawMessage(tc.args),
			})
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if len(api.calls) != 0 {
				t.Errorf("拒否する前にハンドラーを %d 回呼んだ", len(api.calls))
			}
		})
	}
}

// The refusal of an excluded operation carries the reason and the page the assistant answers with.
func TestActionRunnerExplainsAnExcludedRead(t *testing.T) {
	runner := &ActionRunner{api: &stubAPI{}}
	_, err := runner.Read(context.Background(), domain.AssistantRead{Operation: "getPluginProxySettings"})
	var excluded *domain.AssistantExcludedError
	if !errors.As(err, &excluded) {
		t.Fatalf("err = %v, want *domain.AssistantExcludedError", err)
	}
	if excluded.Exclusion.Page != "/settings/network" || excluded.Exclusion.Reason == "" {
		t.Errorf("除外 = %+v, want 理由とネットワーク設定のページ", excluded.Exclusion)
	}
}

func TestActionRunnerMasksURLCredentialsInReadResults(t *testing.T) {
	api := &stubAPI{respond: func(*http.Request) (int, string) {
		return http.StatusOK, `{"endpoint":"socks5://user:pass@host:1080","plain":"http://host/a@b"}`
	}}
	runner := &ActionRunner{api: api}
	result, err := runner.Read(context.Background(), domain.AssistantRead{Operation: "getOllamaSettings"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := `{"endpoint":"socks5://***@host:1080","plain":"http://host/a@b"}`
	if string(result.Body) != want {
		t.Errorf("結果 = %s, want %s", result.Body, want)
	}
}

// A schema row's body is checked against the operation's schema before the handler runs.
func TestActionRunnerValidatesASchemaRowsBodyBeforeTheCall(t *testing.T) {
	credits := &faceAppearanceCredits{}
	_, runner := NewServer(Options{Credits: credits, Auth: ownerAuth{}, Assets: emptyAssets{}})
	call := domain.AssistantCall{
		ActionID: "moveFaceAppearances", Path: map[string]string{"id": "4"},
		Body: map[string]any{"appearanceIds": []int64{7}, "targetId": "someone"},
	}

	_, err := runner.Run(context.Background(), call)
	if !errors.Is(err, domain.ErrAssistantInvalidArguments) || len(credits.calls) != 0 {
		t.Fatalf("err = %v, 呼び出し %d 回, want 呼ぶ前の ErrAssistantInvalidArguments", err, len(credits.calls))
	}

	call.Body["targetId"] = 5
	if _, err := runner.Run(context.Background(), call); err != nil {
		t.Fatalf("正しい本文で Run: %v", err)
	}
	want := appearanceCall{personID: 4, appearances: []int64{7}, target: 5}
	if len(credits.calls) != 1 || !slices.Equal(credits.calls[0].appearances, want.appearances) ||
		credits.calls[0].personID != want.personID || credits.calls[0].target != want.target {
		t.Errorf("呼び出し = %+v, want %+v", credits.calls, want)
	}
}

// A composite row runs only the stage or cleanup operation the call names.
func TestActionRunnerRunsTheNamedStageOfACompositeRow(t *testing.T) {
	api := &stubAPI{}
	runner := &ActionRunner{api: api}
	call := domain.AssistantCall{
		ActionID: "editVideoThumbnail", Operation: "cancelThumbnailEdit",
		Path: map[string]string{"editId": "e1"},
	}
	if _, err := runner.Run(context.Background(), call); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(api.calls) != 1 || api.calls[0].target != "/api/thumbnail-edits/e1/cancel" {
		t.Errorf("呼び出し = %+v, want 取り消しの段階", api.calls)
	}

	for _, operation := range []string{"", "deleteVideo"} {
		call.Operation = operation
		if _, err := runner.Run(context.Background(), call); !errors.Is(err, domain.ErrAssistantActionUnknown) {
			t.Errorf("段階 %q: err = %v, want ErrAssistantActionUnknown", operation, err)
		}
	}
	if len(api.calls) != 1 {
		t.Errorf("段階の無い呼び出しでハンドラーを呼んだ: %+v", api.calls)
	}
}

func TestActionRunnerToolsCarryTheirKindsAndSchemas(t *testing.T) {
	tools := map[string]domain.AssistantTool{}
	for _, tool := range (&ActionRunner{}).Tools() {
		tools[tool.Name] = tool
	}

	if read := tools["listTags"]; read.Kind != domain.AssistantToolRead || len(read.Parameters) == 0 || read.Summary == "" {
		t.Errorf("listTags = %+v, want 引数のスキーマと要約のある読み取り", read)
	}
	if image := tools["getVideoFrame"]; image.Kind != domain.AssistantToolRead || !image.Image {
		t.Errorf("getVideoFrame = %+v, want 画像の読み取り", image)
	}
	if excluded := tools["getPluginProxySettings"]; excluded.Kind != domain.AssistantToolExcluded || excluded.Exclusion == nil {
		t.Errorf("getPluginProxySettings = %+v, want 理由つきの対象外", excluded)
	}
	move := tools["moveFaceAppearances"]
	if move.Kind != domain.AssistantToolWrite || move.Row == nil || !move.Row.Schema || len(move.Parameters) == 0 {
		t.Errorf("moveFaceAppearances = %+v, want スキーマ行の書き込み", move)
	}
	if named := tools["updateVideoTags.add"]; named.Kind != domain.AssistantToolWrite || named.Parameters != nil {
		t.Errorf("updateVideoTags.add = %+v, want 引数を自前で持つ名前の行", named)
	}
	if composite := tools["editVideoThumbnail"]; composite.Kind != domain.AssistantToolWrite || composite.Summary == "" {
		t.Errorf("editVideoThumbnail = %+v, want 複合行の書き込み", composite)
	}
	if _, ok := tools["deleteTag"]; !ok {
		t.Error("書き込みの行 deleteTag が道具に無い")
	}
	if _, ok := tools["startThumbnailEdit"]; ok {
		t.Error("複合行の段階が単独の道具になっている")
	}
}
