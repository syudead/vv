package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestApplyVideoEditsWritesEveryPart(t *testing.T) {
	f := newGuestFixture(t, true)
	performer := f.createWorkInfoPerson(t, "出演者")

	rec := f.ownerRequest(http.MethodPost, "/api/video-edits", jsonBody(map[string]any{
		"videoIds": []int64{f.ids["a"], f.ids["b"]},
		"tags":     map[string]any{"add": []map[string]any{{"name": "新"}}, "remove": []int64{f.tagID}},
		"credits":  []map[string]any{{"kind": "person", "role": "performer", "add": []int64{performer}}},
		"dates":    []map[string]any{{"kind": "release", "date": "2020"}},
		"public":   false,
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.VideoEditResponse](t, rec); got.Applied != 2 || len(got.Tags) != 1 || got.Tags[0].Name != "新" {
		t.Errorf("response = %+v, want 2 applied and the tag 新", got)
	}
	infos, err := f.env.db.WorkInfo().ByUserKeys(context.Background(), []string{"content-key-b"})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.WorkInfo{
		Dates:  []domain.WorkDate{{Kind: domain.DateRelease, Date: "2020"}},
		People: []domain.WorkPerson{{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: performer}}},
	}
	if !infos["content-key-b"].Equal(want) {
		t.Errorf("b work info = %+v, want %+v", infos["content-key-b"], want)
	}
	assertStatus(t, "guest read of a after private", f.env.get("/api/videos/"+strconv.FormatInt(f.ids["a"], 10)), http.StatusNotFound)
}

func TestApplyVideoEditsReportsTheRequestField(t *testing.T) {
	f := newGuestFixture(t, true)
	cases := []struct {
		name   string
		body   map[string]any
		status int
		code   gen.ErrorCode
		reason gen.ErrorReason
		field  string
	}{
		{"no change", map[string]any{"tags": map[string]any{"add": []any{}}}, http.StatusBadRequest, codeInvalidRequest, "", ""},
		{"blank tag name", map[string]any{"tags": map[string]any{"add": []map[string]any{{"name": " "}}}},
			http.StatusBadRequest, codeInvalidRequest, gen.ErrorReasonTagNameEmpty, "tags.add[0].name"},
		{"missing tag", map[string]any{"tags": map[string]any{"remove": []int64{999_999}}},
			http.StatusNotFound, codeTagNotFound, "", ""},
		{"missing credit", map[string]any{"credits": []map[string]any{{"kind": "person", "role": "director", "add": []int64{999_999}}}},
			http.StatusNotFound, codeNotFound, gen.ErrorReasonPersonNotFound, "credits[0].add[0]"},
		{"invalid date", map[string]any{"dates": []map[string]any{{"kind": "release", "date": "2020-13"}}},
			http.StatusBadRequest, codeInvalidRequest, gen.ErrorReasonWorkInfoInvalidDate, "dates[0].date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.body["videoIds"] = []int64{f.ids["a"]}
			rec := f.ownerRequest(http.MethodPost, "/api/video-edits", jsonBody(tc.body))
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			body := decodeErrorBody(t, rec.Body.Bytes())
			if body.Code != tc.code || ptrValue(body.Reason) != tc.reason || ptrValue(body.Field) != tc.field {
				t.Errorf("error = %+v, want %s/%q at %q", body, tc.code, tc.reason, tc.field)
			}
		})
	}
}

func ptrValue[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

func TestSummarizeVideoEditsCountsTheSelection(t *testing.T) {
	f := newGuestFixture(t, true)
	performer := f.createWorkInfoPerson(t, "出演者")
	if _, err := f.env.db.WorkInfo().Set(context.Background(), f.ids["a"], domain.WorkInfo{
		People: []domain.WorkPerson{{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: performer}}},
	}); err != nil {
		t.Fatal(err)
	}

	rec := f.ownerRequest(http.MethodPost, "/api/video-edits/summary", jsonBody(map[string]any{
		"videoIds": []int64{f.ids["a"], f.ids["b"], f.ids["c"]},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.VideoEditSummary](t, rec)
	if got.Total != 3 || got.Public != 1 || len(got.Tags) != 1 || got.Tags[0].Count != 2 || len(got.Dates) != 0 ||
		len(got.People) != 1 || got.People[0].Person.Name != "出演者" || got.People[0].Count != 1 {
		t.Errorf("summary = %+v, want 3 videos, 1 public, the shared tag on 2 and the performer on 1", got)
	}
}

type videoEditsFunc func(context.Context, []int64, domain.VideoEdit) (domain.VideoEditResult, error)

func (f videoEditsFunc) Summary(context.Context, []int64) (domain.VideoEditSummary, error) {
	return domain.VideoEditSummary{}, nil
}

func (f videoEditsFunc) Apply(ctx context.Context, ids []int64, edit domain.VideoEdit) (domain.VideoEditResult, error) {
	return f(ctx, ids, edit)
}

func TestApplyVideoEditsEndsGuestStreamsOfVideosMadePrivate(t *testing.T) {
	ledger := newGuestLedger()
	srv := &server{
		videoEdits: videoEditsFunc(func(context.Context, []int64, domain.VideoEdit) (domain.VideoEditResult, error) {
			return domain.VideoEditResult{Applied: 1, ContentKeys: []string{"key"}}, nil
		}),
		guests: ledger, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	stream, release, ok := ledger.track(context.Background(), httptest.NewRecorder(), "key", ledger.generation())
	if !ok {
		t.Fatal("the guest stream was not tracked")
	}
	defer release()

	req := httptest.NewRequest(http.MethodPost, "/api/video-edits", strings.NewReader(`{"videoIds":[7],"public":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ApplyVideoEdits(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if stream.Err() == nil {
		t.Error("the guest stream of the video made private is still running")
	}
}
