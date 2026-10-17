package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// update_video_work_info と list_people はルートと同じ構造の内容を返し、list_people の limit は既定で 100、
// update_person の null は REST と同じく一覧を空にする。
func TestMCPWorkInfoTools(t *testing.T) {
	f := newMCPFixture(t, Options{})
	session := f.connect(t)
	ctx := context.Background()
	for i := range 101 {
		if _, err := f.env.db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Name: fmt.Sprintf("人物 %03d", i)}); err != nil {
			t.Fatal(err)
		}
	}

	var page extgen.PersonList
	if callTool(t, session, "list_people", nil, &page) || len(page.Items) != 100 || page.NextCursor == nil || page.TotalAll != 101 {
		t.Errorf("list_people の既定の 1 ページ = %d 件, nextCursor %v, totalAll %d", len(page.Items), page.NextCursor, page.TotalAll)
	}
	var created extgen.Person
	if callTool(t, session, "create_person", map[string]any{
		"name": "出演者", "aliases": []any{map[string]any{"name": "Performer"}},
		"externalIds": []any{map[string]any{"source": "example", "value": "42"}},
	}, &created) || created.Id == 0 {
		t.Fatalf("create_person = %+v", created)
	}
	var found extgen.PersonList
	if callTool(t, session, "list_people", map[string]any{"source": "example", "externalId": "42"}, &found) ||
		len(found.Items) != 1 || found.Items[0].Id != created.Id {
		t.Errorf("list_people の外部 ID の検索 = %+v", found)
	}
	var updated extgen.Person
	if callTool(t, session, "update_person", map[string]any{"id": created.Id, "aliases": nil}, &updated) ||
		updated.Aliases == nil || len(updated.Aliases) != 0 || len(updated.ExternalIds) != 1 {
		t.Errorf("update_person の aliases: null = %+v", updated)
	}

	var written extgen.VideoWorkInfoResponse
	if callTool(t, session, "update_video_work_info", map[string]any{"items": []any{map[string]any{
		"video": map[string]any{"path": f.pathA},
		"workInfo": map[string]any{
			"code": "ABC-1", "people": []any{map[string]any{"role": "performer", "person": map[string]any{"id": created.Id}}},
		},
	}}}, &written) || len(written.Items) != 1 || written.Items[0].Video.Id != f.videoA {
		t.Fatalf("update_video_work_info = %+v", written)
	}
	rec := f.env.serve(authRequest{
		method: http.MethodPost, target: "/api/v1/video-work-info", header: bearer(f.secret),
		body: fmt.Sprintf(`{"items":[{"video":{"id":%d},"workInfo":{"code":"ABC-1","people":[{"role":"performer","person":{"id":%d}}]}}]}`,
			f.videoA, created.Id),
	})
	var viaREST extgen.VideoWorkInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &viaREST); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("REST: %d %s", rec.Code, rec.Body)
	}
	if !reflect.DeepEqual(written, viaREST) {
		t.Errorf("MCP と REST の結果が違う: %+v / %+v", written, viaREST)
	}
	var video extgen.ExternalVideo
	if callTool(t, session, "get_video", map[string]any{"id": f.videoA}, &video) || video.WorkInfo.Code == nil ||
		*video.WorkInfo.Code != "ABC-1" || len(video.WorkInfo.People) != 1 || video.WorkInfo.People[0].Person.Name != "出演者" {
		t.Errorf("get_video の workInfo = %+v", video.WorkInfo)
	}

	var failure extgen.Error
	if !callTool(t, session, "update_video_work_info", map[string]any{"items": []any{map[string]any{
		"video":    map[string]any{"id": f.videoA},
		"workInfo": map[string]any{"people": []any{map[string]any{"role": "performer", "person": map[string]any{"id": 999999}}}},
	}}}, &failure) || failure.Reason == nil || *failure.Reason != extgen.PersonNotFound || failure.Field == nil || failure.Index == nil {
		t.Errorf("無い人物の update_video_work_info = %+v", failure)
	}
}
