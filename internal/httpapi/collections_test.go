package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type collectionsFunc func(context.Context, domain.CollectionDelete) (domain.CollectionsDeleted, error)

func (f collectionsFunc) DeleteCollections(ctx context.Context, del domain.CollectionDelete) (domain.CollectionsDeleted, error) {
	return f(ctx, del)
}

func TestDeleteCollectionsPassesDeduplicatedTargets(t *testing.T) {
	calls := 0
	handler := newTestServer(t, Options{Collections: collectionsFunc(func(_ context.Context, del domain.CollectionDelete) (domain.CollectionsDeleted, error) {
		calls++
		want := domain.CollectionDelete{GroupIDs: []int64{3, 1}, SmartFolderIDs: []int64{4}}
		if !reflect.DeepEqual(del, want) {
			t.Errorf("delete = %+v, want %+v", del, want)
		}
		return domain.CollectionsDeleted{DissolvedGroups: 2, DeletedSmartFolders: 1}, nil
	})})
	rec := jsonRequest(t, handler, http.MethodPost, "/api/collections/delete", `{"groupIds":[3,1,3],"smartFolderIds":[4,4]}`)
	if rec.Code != http.StatusOK || calls != 1 {
		t.Fatalf("status = %d, calls = %d: %s", rec.Code, calls, rec.Body)
	}
	if got := decode[gen.CollectionsDeleteResponse](t, rec); got.DissolvedGroups != 2 || got.DeletedSmartFolders != 1 {
		t.Errorf("response = %+v", got)
	}
}

func TestDeleteCollectionsRejectsInvalidRequestsAndGuests(t *testing.T) {
	calls := 0
	store := collectionsFunc(func(context.Context, domain.CollectionDelete) (domain.CollectionsDeleted, error) {
		calls++
		return domain.CollectionsDeleted{}, nil
	})
	handler := newTestServer(t, Options{Collections: store})
	tooMany := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxVideoTagsIDs}
	for _, body := range []string{
		`{}`,
		`{"groupIds":[],"smartFolderIds":[]}`,
		`{"groupIds":` + idsJSON(maxVideoTagsIDs) + `,"smartFolderIds":[1]}`,
	} {
		rec := jsonRequest(t, handler, http.MethodPost, "/api/collections/delete", body)
		assertErrorBody(t, "target count", rec.Code, rec.Body.Bytes(), tooMany)
	}
	assertStatus(t, "unknown field", jsonRequest(t, handler, http.MethodPost, "/api/collections/delete", `{"videoIds":[1]}`),
		http.StatusBadRequest)
	guest := newTestServer(t, Options{Collections: store, Auth: guestEventsAuth{}})
	assertUnauthenticated(t, "guest", jsonRequest(t, guest, http.MethodPost, "/api/collections/delete", `{"groupIds":[1]}`))
	if calls != 0 {
		t.Errorf("rejected requests reached storage %d times", calls)
	}
}
