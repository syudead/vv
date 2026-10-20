package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestEditVideoCreditsAppliesToSelectedVideos(t *testing.T) {
	f := newGuestFixture(t, true)
	sub := f.subscribeVideoEvents(t)
	performer := f.createWorkInfoPerson(t, "出演者")

	rec := f.ownerRequest(http.MethodPost, "/api/video-credits", jsonBody(map[string]any{
		"videoIds": []int64{f.ids["a"], f.ids["b"]},
		"edits":    []map[string]any{{"kind": "person", "role": "performer", "action": "add", "ids": []int64{performer}}},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.VideoCreditsResponse](t, rec); got.Applied != 2 {
		t.Errorf("applied = %d, want 2", got.Applied)
	}
	sub.take()
	video, err := f.env.db.WorkInfo().ByUserKeys(context.Background(), []string{"content-key-b"})
	if err != nil {
		t.Fatal(err)
	}
	if people := video["content-key-b"].People; len(people) != 1 || people[0].Person.ID != performer {
		t.Errorf("b people = %+v, want the performer", people)
	}
}

func TestEditVideoCreditsReportsTheEditField(t *testing.T) {
	f := newGuestFixture(t, true)

	rec := f.ownerRequest(http.MethodPost, "/api/video-credits", jsonBody(map[string]any{
		"videoIds": []int64{f.ids["a"]},
		"edits":    []map[string]any{{"kind": "person", "role": "performer", "action": "add", "ids": []int64{999_999}}},
	}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
	body := decodeErrorBody(t, rec.Body.Bytes())
	if body.Reason == nil || *body.Reason != gen.ErrorReasonPersonNotFound || body.Field == nil || *body.Field != "edits[0].ids[0]" {
		t.Errorf("error = %+v, want person_not_found at edits[0].ids[0]", body)
	}
}
