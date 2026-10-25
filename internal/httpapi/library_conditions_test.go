package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// conditionListings records the queries both listings pass to the store; other operations are not used.
type conditionListings struct {
	Library
	LibraryItems
	videos, library []domain.VideoQuery
}

func (c *conditionListings) ListVideos(_ context.Context, _ domain.Audience, q domain.VideoQuery) (domain.VideoPage, error) {
	c.videos = append(c.videos, q)
	return domain.VideoPage{Items: []domain.Video{}, Limit: q.Limit}, nil
}

func (c *conditionListings) ListLibrary(_ context.Context, _ domain.Audience, q domain.VideoQuery) (domain.LibraryPage, error) {
	c.library = append(c.library, q)
	return domain.LibraryPage{Limit: q.Limit}, nil
}

func conditionServer(t *testing.T, auth Authenticator) (http.Handler, *conditionListings) {
	t.Helper()
	listings := &conditionListings{}
	return newTestServer(t, Options{Videos: listings, Library: listings, Auth: auth}), listings
}

func TestListsPassVideoConditionsToTheStore(t *testing.T) {
	handler, listings := conditionServer(t, ownerAuth{})
	// An explicit 0 bound is a condition, not an omitted one.
	query := "?minDurationMs=0&maxDurationMs=0&addedFrom=2025-03-01&addedTo=2025-03-02&watchState=inProgress"
	want := domain.VideoConditions{
		MinDurationMs: new(int64(0)), MaxDurationMs: new(int64(0)),
		AddedFrom: "2025-03-01", AddedTo: "2025-03-02", WatchState: domain.WatchStateInProgress,
	}
	for _, path := range []string{"/api/videos", "/api/library"} {
		if rec := do(t, handler, http.MethodGet, path+query); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", path, rec.Code, rec.Body)
		}
	}
	if len(listings.videos) != 1 || !reflect.DeepEqual(listings.videos[0].VideoConditions, want) {
		t.Errorf("ListVideos queries = %+v, want one with %+v", listings.videos, want)
	}
	if len(listings.library) != 1 || !reflect.DeepEqual(listings.library[0].VideoConditions, want) {
		t.Errorf("ListLibrary queries = %+v, want one with %+v", listings.library, want)
	}
}

func TestConditionFiltersRejectInvalidValuesAndGuestWatchState(t *testing.T) {
	owner, ownerListings := conditionServer(t, ownerAuth{})
	guest, guestListings := conditionServer(t, guestEventsAuth{})
	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	guestDenied := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonGuestFilterNotAllowed}
	for _, path := range []string{"/api/videos", "/api/library"} {
		for _, query := range []string{
			"minDurationMs=-1", "minDurationMs=1&maxDurationMs=0", "addedFrom=2025-13-01",
			"addedFrom=2025-03-02&addedTo=2025-03-01", "watchState=seen",
		} {
			rec := do(t, owner, http.MethodGet, path+"?"+query)
			assertErrorBody(t, path+"?"+query, rec.Code, rec.Body.Bytes(), invalid)
		}
		rec := do(t, guest, http.MethodGet, path+"?watchState=watched")
		assertErrorBody(t, "guest "+path+"?watchState=watched", rec.Code, rec.Body.Bytes(), guestDenied)
	}
	for _, listings := range []*conditionListings{ownerListings, guestListings} {
		if calls := len(listings.videos) + len(listings.library); calls != 0 {
			t.Errorf("store calls after rejected requests = %d, want 0", calls)
		}
	}
}

// conditionCards returns the given cards for every request.
type conditionCards struct {
	SmartFolders
	cards []domain.SmartFolderCard
}

func (c conditionCards) SmartFolderCards(context.Context, domain.Audience, []int64) ([]domain.SmartFolderCard, error) {
	return c.cards, nil
}

func TestSmartFolderCardsShowOwnersTheRuleConditions(t *testing.T) {
	store := conditionCards{cards: []domain.SmartFolderCard{
		{SmartFolderSummary: domain.SmartFolderSummary{ID: 1}, Conditions: domain.VideoConditions{
			MaxDurationMs: new(int64(0)), AddedFrom: "2025-03-01", WatchState: domain.WatchStateWatched,
		}},
		{SmartFolderSummary: domain.SmartFolderSummary{ID: 2}},
	}}
	want := gen.SmartFolderConditions{MaxDurationMs: new(int64(0)), AddedFrom: new("2025-03-01"), WatchState: new(gen.WatchStateWatched)}
	for _, tc := range []struct {
		name string
		auth Authenticator
		want *gen.SmartFolderConditions
	}{
		{"owner", ownerAuth{}, &want},
		{"guest", guestEventsAuth{}, nil},
	} {
		handler := newTestServer(t, Options{SmartFolders: store, Auth: tc.auth})
		rec := do(t, handler, http.MethodGet, "/api/smart-folders/cards?ids=1,2")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", tc.name, rec.Code, rec.Body)
		}
		cards := decode[gen.SmartFolderCards](t, rec).Cards
		if len(cards) != 2 || !reflect.DeepEqual(cards[0].Conditions, tc.want) || cards[1].Conditions != nil {
			t.Errorf("%s: cards = %+v, want conditions %+v then none", tc.name, cards, tc.want)
		}
	}
}
