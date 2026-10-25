package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type fakeCodeEdits struct {
	rows       []domain.VideoCodeRow
	applyErr   error
	summaryIDs []int64
	applied    []domain.VideoCodeEdit
}

func (f *fakeCodeEdits) CodeSummary(_ context.Context, ids []int64) ([]domain.VideoCodeRow, error) {
	f.summaryIDs = ids
	return f.rows, nil
}

func (f *fakeCodeEdits) ApplyCodes(_ context.Context, items []domain.VideoCodeEdit) (int, error) {
	f.applied = items
	return len(items), f.applyErr
}

// The 1,000 limit counts the distinct videos after smart folders resolve, and an over-limit request never reaches the store.
func TestSummarizeVideoCodeEditsLimitCountsResolvedVideos(t *testing.T) {
	members := make([]int64, domain.VideoCodeEditsMaxItems)
	for i := range members {
		members[i] = int64(i + 2)
	}
	for _, tc := range []struct {
		name     string
		videoIDs string
		status   int
	}{
		{"1,000 resolved", "[2]", http.StatusOK},
		{"1,001 resolved", "[1]", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeCodeEdits{}
			handler := newTestServer(t, Options{VideoCodeEdits: store, SmartFolders: &memberFolders{members: members}})

			rec := postJSON(t, handler, "/api/video-code-edits/summary", `{"videoIds":`+tc.videoIDs+`,"smartFolderIds":[7]}`)
			if tc.status == http.StatusOK {
				if rec.Code != http.StatusOK || len(store.summaryIDs) != domain.VideoCodeEditsMaxItems {
					t.Errorf("status = %d, store got %d videos; want 200 with 1,000", rec.Code, len(store.summaryIDs))
				}
				return
			}
			assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), wantError{
				status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos,
				limit: domain.VideoCodeEditsMaxItems,
			})
			if store.summaryIDs != nil {
				t.Errorf("an over-limit request reached the store with %d videos", len(store.summaryIDs))
			}
		})
	}
}

func TestSummarizeVideoCodeEditsAnswersListItems(t *testing.T) {
	file := sampleVideo(1, "clip")
	file.FileTitle = "clip"
	virtual := domain.Video{ID: 2, Title: "Virtual", ContentKey: "virtual:1", UserKey: "virtual:1", FileTitle: "Virtual"}
	store := &fakeCodeEdits{rows: []domain.VideoCodeRow{{Video: file, Code: "ABC-1"}, {Video: virtual}}}
	handler := newTestServer(t, Options{VideoCodeEdits: store})

	rec := postJSON(t, handler, "/api/video-code-edits/summary", `{"videoIds":[1,2]}`)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("status = %d, Cache-Control = %q: %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body)
	}
	rows := decode[gen.VideoCodeEditSummary](t, rec).Rows
	if len(rows) != 2 || ptrValue(rows[0].Code) != "ABC-1" || ptrValue(rows[0].Video.FileTitle) != "clip" {
		t.Fatalf("rows = %+v, want the file video with its code and fileTitle first", rows)
	}
	if !rows[1].Video.Virtual || rows[1].Video.FileTitle != nil || rows[1].Code != nil {
		t.Errorf("virtual row = %+v, want a virtual video without fileTitle or code", rows[1])
	}
}

func TestApplyVideoCodeEditsNamesTheFailingItem(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		reason gen.ErrorReason
		field  string
	}{
		{"missing video", domain.ErrNotFound, http.StatusNotFound, reasonVideoNotFound, "items[1].videoId"},
		{"changed video", domain.ErrContentChanged, http.StatusConflict, reasonContentChanged, "items[1].contentVersion"},
		{"two items on one key", domain.ErrDuplicateVideoCodeEdit, http.StatusBadRequest, "", "items[1].videoId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeCodeEdits{applyErr: &domain.VideoCodeEditAtError{Index: 1, Err: tc.err}}
			handler := newTestServer(t, Options{VideoCodeEdits: store})

			rec := postJSON(t, handler, "/api/video-code-edits", `{"items":[
				{"videoId":1,"contentVersion":"a","code":" ABC-1 "},{"videoId":2,"contentVersion":"b","code":"ABC-2"}]}`)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			body := decodeErrorBody(t, rec.Body.Bytes())
			if ptrValue(body.Reason) != tc.reason || ptrValue(body.Field) != tc.field {
				t.Errorf("error = %+v, want %q at %q", body, tc.reason, tc.field)
			}
			want := domain.VideoCodeEdit{VideoID: 1, ContentVersion: "a", Code: "ABC-1"}
			if len(store.applied) != 2 || store.applied[0] != want {
				t.Errorf("store items = %+v, want the two items with trimmed codes", store.applied)
			}
		})
	}
}
