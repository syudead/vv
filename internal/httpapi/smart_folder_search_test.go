package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type smartFolderSearchCall struct {
	audience domain.Audience
	id       int64
	cursor   string
	limit    int
	query    string
}
type searchSmartFolders struct {
	failingSmartFolders
	calls []smartFolderSearchCall
}

func (s *searchSmartFolders) SmartFolderContents(_ context.Context, audience domain.Audience, id int64, cursor string, limit int, query domain.VideoQuery) (domain.SmartFolderPage, error) {
	s.calls = append(s.calls, smartFolderSearchCall{audience, id, cursor, limit, query.Query})
	return domain.SmartFolderPage{}, domain.ErrNotFound
}

func TestSmartFolderSearchRoutesPassQuery(t *testing.T) {
	store := &searchSmartFolders{}
	handler := newTestServer(t, Options{SmartFolders: store})
	for _, suffix := range []string{"?", "/items?cursor=next&"} {
		store.calls = nil
		rec := jsonRequest(t, handler, http.MethodGet, "/api/smart-folders/3"+suffix+"limit=1&query="+url.QueryEscape("  clip OR cat  "), "")
		if rec.Code != http.StatusNotFound || len(store.calls) != 1 {
			t.Fatalf("response %d, calls %+v", rec.Code, store.calls)
		}
		got := store.calls[0]
		cursor := ""
		if strings.HasPrefix(suffix, "/items") {
			cursor = "next"
		}
		if got.id != 3 || got.cursor != cursor || got.limit != 1 || got.query != "  clip OR cat  " {
			t.Fatalf("call = %+v", got)
		}
		store.calls = nil
		rec = jsonRequest(t, handler, http.MethodGet, "/api/smart-folders/3"+suffix+"query="+strings.Repeat("x", maxQueryLength+1), "")
		if rec.Code != http.StatusBadRequest || len(store.calls) != 0 {
			t.Fatalf("long query response %d, calls %+v", rec.Code, store.calls)
		}
	}
}
