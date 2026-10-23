package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type criteriaLibrary struct {
	LibraryItems
	calls []domain.VideoQuery
}

func (s *criteriaLibrary) Group(_ context.Context, _ domain.Audience, _ int64, _ []int64, query domain.VideoQuery) (domain.GroupPage, error) {
	s.calls = append(s.calls, query)
	return domain.GroupPage{}, domain.ErrNotFound
}
func (s *criteriaLibrary) GroupItems(_ context.Context, _ domain.Audience, _ int64, _ string, _ int, query domain.VideoQuery) (domain.GroupPage, error) {
	s.calls = append(s.calls, query)
	return domain.GroupPage{}, domain.ErrNotFound
}

type criteriaFolders struct {
	failingSmartFolders
	calls []domain.VideoQuery
}

func (s *criteriaFolders) SmartFolderContents(_ context.Context, _ domain.Audience, _ int64, _ string, _ int, query domain.VideoQuery) (domain.SmartFolderPage, error) {
	s.calls = append(s.calls, query)
	return domain.SmartFolderPage{}, domain.ErrNotFound
}

func TestCollectionRoutesForwardTemporaryCriteriaAndRejectInvalidInputs(t *testing.T) {
	library, folders := &criteriaLibrary{}, &criteriaFolders{}
	handler := newTestServer(t, Options{Library: library, SmartFolders: folders})
	for _, path := range []string{"/api/groups/1?", "/api/groups/1/items?cursor=next&", "/api/smart-folders/1?", "/api/smart-folders/1/items?cursor=next&"} {
		library.calls = nil
		folders.calls = nil
		rec := jsonRequest(t, handler, http.MethodGet, path+"query=clip&favorite=true&sort=random&seed=37", "")
		calls := append(library.calls, folders.calls...)
		if rec.Code != http.StatusNotFound || len(calls) != 1 {
			t.Fatalf("%s: status %d, calls %+v", path, rec.Code, calls)
		}
		q := calls[0]
		if q.Query != "clip" || !q.FavoriteOnly || q.Sort != domain.SortRandom || q.Seed != 37 {
			t.Fatalf("%s: %+v", path, q)
		}
		for _, invalid := range []string{"sort=unknown", "sort=random&seed=-1"} {
			library.calls = nil
			folders.calls = nil
			rec = jsonRequest(t, handler, http.MethodGet, path+invalid, "")
			if rec.Code != http.StatusBadRequest || len(library.calls)+len(folders.calls) != 0 {
				t.Fatalf("invalid %s%s = %d", path, invalid, rec.Code)
			}
		}
	}
}
