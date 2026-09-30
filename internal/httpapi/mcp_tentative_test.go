package httpapi

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/httpapi/extgen"
)

// MCP の仮の付与（specs/031-tentative-tags/contracts/external-api.md §2）。update_video_tags は
// REST と同じ結果になり、list_tags・get_video の出力に tentative が出る（受け入れ条件 15）。
func TestMCPUpdateVideoTagsTentative(t *testing.T) {
	f := newMCPFixture(t, Options{})
	session := f.connect(t)

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != "update_video_tags" {
			continue
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(schema), `"tentative"`) || !strings.Contains(tool.Description, "tentative") {
			t.Errorf("update_video_tags: description %q, input schema %s", tool.Description, schema)
		}
	}

	video := []any{map[string]any{"path": f.pathA}}
	var tagged extgen.VideoTagsResponse
	if callTool(t, session, "update_video_tags", map[string]any{
		"videos": video, "action": "add", "tags": []string{"高画質", "猫"}, "tentative": true,
	}, &tagged) {
		t.Fatalf("update_video_tags が誤り: %+v", tagged)
	}
	if tagged.SkippedTags == nil || len(tagged.SkippedTags) != 0 || len(tagged.Items) != 1 || len(tagged.Items[0].Tags) != 2 {
		t.Fatalf("update_video_tags = %+v", tagged)
	}
	var rejectID int64
	for _, tag := range tagged.Items[0].Tags {
		if !tag.Tentative {
			t.Errorf("仮で作ったタグが仮でない: %+v", tag)
		}
		if tag.Name == "高画質" {
			rejectID = tag.Id
		}
	}

	var tags extgen.TagList
	if callTool(t, session, "list_tags", nil, &tags) || len(tags.Items) != 2 || !tags.Items[0].Tentative || !tags.Items[1].Tentative {
		t.Errorf("list_tags = %+v", tags)
	}
	var got extgen.ExternalVideo
	if callTool(t, session, "get_video", map[string]any{"id": f.videoA}, &got) || len(got.Tags) != 2 || !got.Tags[0].Tentative {
		t.Errorf("get_video の tags = %+v", got.Tags)
	}

	if _, err := f.env.db.Tags().RejectTag(context.Background(), rejectID); err != nil {
		t.Fatal(err)
	}
	tagged = extgen.VideoTagsResponse{}
	if callTool(t, session, "update_video_tags", map[string]any{
		"videos": video, "action": "replace", "tags": []string{"高画質", "犬"}, "tentative": true,
	}, &tagged) {
		t.Fatalf("update_video_tags が誤り: %+v", tagged)
	}
	if !slices.Equal(tagged.SkippedTags, []string{"高画質"}) || len(tagged.Items) != 1 ||
		len(tagged.Items[0].Tags) != 1 || tagged.Items[0].Tags[0].Name != "犬" || !tagged.Items[0].Tags[0].Tentative {
		t.Errorf("却下した名前を含む replace = %+v", tagged)
	}
}
