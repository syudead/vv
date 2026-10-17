package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type favoritesFunc func(context.Context, domain.FavoriteChange) (domain.FavoriteApplied, error)

func (f favoritesFunc) SetFavorites(ctx context.Context, change domain.FavoriteChange) (domain.FavoriteApplied, error) {
	return f(ctx, change)
}

func TestUpdateFavoritesPassesDeduplicatedTargets(t *testing.T) {
	for _, favorite := range []bool{true, false} {
		calls := 0
		handler := newTestServer(t, Options{Favorites: favoritesFunc(func(_ context.Context, change domain.FavoriteChange) (domain.FavoriteApplied, error) {
			calls++
			want := domain.FavoriteChange{VideoIDs: []int64{3, 1}, GroupIDs: []int64{3}, SmartFolderIDs: []int64{4, 2}, Favorite: favorite}
			if !reflect.DeepEqual(change, want) {
				t.Errorf("change = %+v, want %+v", change, want)
			}
			return domain.FavoriteApplied{Videos: 2, Groups: 0, SmartFolders: 1}, nil
		})})
		body := `{"videoIds":[3,1,3],"groupIds":[3,3],"smartFolderIds":[4,2,4],"favorite":` + strconv.FormatBool(favorite) + `}`
		rec := jsonRequest(t, handler, http.MethodPut, "/api/favorites", body)
		if rec.Code != http.StatusOK || calls != 1 {
			t.Fatalf("status = %d, calls = %d: %s", rec.Code, calls, rec.Body)
		}
		got := decode[gen.FavoritesResponse](t, rec)
		if got.AppliedVideos != 2 || got.AppliedGroups != 0 || got.AppliedSmartFolders != 1 {
			t.Errorf("response = %+v", got)
		}
		if rec.Header().Get("Cache-Control") != cacheNoStore {
			t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
		}
	}
}

func TestUpdateFavoritesRejectsInvalidRequests(t *testing.T) {
	calls := 0
	handler := newTestServer(t, Options{Favorites: favoritesFunc(func(context.Context, domain.FavoriteChange) (domain.FavoriteApplied, error) {
		calls++
		return domain.FavoriteApplied{}, nil
	})})
	for _, body := range []string{
		`{"favorite":true}`,
		`{"videoIds":[],"groupIds":[],"smartFolderIds":[],"favorite":true}`,
		`{"videoIds":` + idsJSON(maxVideoTagsIDs+1) + `,"favorite":true}`,
		`{"videoIds":` + idsJSON(maxVideoTagsIDs) + `,"groupIds":[1],"favorite":true}`,
		`{"groupIds":` + idsJSON(maxVideoTagsIDs) + `,"smartFolderIds":[1],"favorite":true}`,
	} {
		rec := jsonRequest(t, handler, http.MethodPut, "/api/favorites", body)
		assertErrorBody(t, "target count", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxVideoTagsIDs})
	}
	for _, body := range []string{
		`{"videoIds":[1]}`,
		`{"videoIds":[1],"favorite":null}`,
		`{"videoIds":[1],"favorite":true,"extra":1}`,
		`{"videoIds":[1],"favorite":true,"folders":[{"rootId":1,"path":"show"}]}`,
	} {
		assertStatus(t, "invalid fields", jsonRequest(t, handler, http.MethodPut, "/api/favorites", body), http.StatusBadRequest)
	}
	if calls != 0 {
		t.Errorf("invalid requests reached storage %d times", calls)
	}
}

func TestUpdateFavoritesAcceptsFullBatchAfterDeduplication(t *testing.T) {
	// The count itself is the HTTP contract; none of these IDs are stored.
	ids := make([]int64, maxVideoTagsIDs)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	for _, field := range []string{"videoIds", "groupIds", "smartFolderIds"} {
		t.Run(field, func(t *testing.T) {
			calls := 0
			handler := newTestServer(t, Options{Favorites: favoritesFunc(func(_ context.Context, change domain.FavoriteChange) (domain.FavoriteApplied, error) {
				calls++
				var got []int64
				switch field {
				case "videoIds":
					got = change.VideoIDs
				case "groupIds":
					got = change.GroupIDs
				case "smartFolderIds":
					got = change.SmartFolderIDs
				}
				if !slices.Equal(got, ids) {
					t.Errorf("%s: got %d IDs, want exact ordered IDs", field, len(got))
				}
				return domain.FavoriteApplied{}, nil
			})})
			encoded := idsJSON(maxVideoTagsIDs)
			body := `{"` + field + `":` + strings.TrimSuffix(encoded, "]") + `,1],"favorite":true}`
			rec := jsonRequest(t, handler, http.MethodPut, "/api/favorites", body)
			if rec.Code != http.StatusOK || calls != 1 {
				t.Fatalf("status = %d, calls = %d: %s", rec.Code, calls, rec.Body)
			}
		})
	}
}

func TestUpdateFavoritesRejectsGuestsBeforeStorage(t *testing.T) {
	calls := 0
	handler := newTestServer(t, Options{Auth: guestEventsAuth{}, Favorites: favoritesFunc(func(context.Context, domain.FavoriteChange) (domain.FavoriteApplied, error) {
		calls++
		return domain.FavoriteApplied{}, nil
	})})
	rec := jsonRequest(t, handler, http.MethodPut, "/api/favorites", `{"videoIds":[1],"favorite":true}`)
	assertUnauthenticated(t, "guest update", rec)
	if calls != 0 {
		t.Errorf("guest reached storage %d times", calls)
	}
}

func TestUpdateFavoritesMapsStorageFailure(t *testing.T) {
	calls := 0
	handler := newTestServer(t, Options{Favorites: favoritesFunc(func(context.Context, domain.FavoriteChange) (domain.FavoriteApplied, error) {
		calls++
		return domain.FavoriteApplied{}, errors.New("storage unavailable")
	})})
	rec := jsonRequest(t, handler, http.MethodPut, "/api/favorites", `{"videoIds":[1],"favorite":true}`)
	assertStatus(t, "storage failure", rec, http.StatusInternalServerError)
	if calls != 1 {
		t.Errorf("storage calls = %d, want 1", calls)
	}
}

func TestUpdateFavoritesMapsBodyLimitWithoutStorage(t *testing.T) {
	calls := 0
	handler := newTestServer(t, Options{Favorites: favoritesFunc(func(context.Context, domain.FavoriteChange) (domain.FavoriteApplied, error) {
		calls++
		return domain.FavoriteApplied{}, nil
	})})
	req := httptest.NewRequest(http.MethodPut, "/api/favorites", errReader{&http.MaxBytesError{Limit: favoritesBodyLimit}})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assertErrorBody(t, "body limit", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	if !strings.Contains(rec.Body.String(), strconv.Itoa(favoritesBodyLimit)) {
		t.Errorf("missing limit: %s", rec.Body)
	}
	if calls != 0 {
		t.Errorf("oversized body reached storage %d times", calls)
	}
}
