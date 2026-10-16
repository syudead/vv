package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// ホームの経路（specs/057-home/contracts/screen-api.md）。GET /api/home と GET /api/home/shelves は
// 所有者とゲストが読め（accessRoutes）、PUT /api/home/shelves は /api/* の既定どおり所有者だけである。
// 所有者だけの欄（organizer と byOrganizer）はゲストの応答から外す。

// Home はホームの棚の読み出しと並びの書き込みである。internal/store の *HomeStore が満たす。
type Home interface {
	// Home は見る人のホームの要約を返す。organizer は所有者にだけ入る。
	Home(ctx context.Context, audience domain.Audience) (domain.Home, error)
	// ShelfItems は棚 keys の最初の項目を keys の順に返す。もう無い棚の鍵は落とす。
	ShelfItems(ctx context.Context, audience domain.Audience, keys []domain.HomeShelfKey) (domain.HomeShelfPage, error)
	// SetShelves は棚を ids のフォルダにする。ids が規則に合わなければ domain.ErrInvalidHomeShelves、
	// 名指すフォルダが無ければ domain.ErrNotFound である。
	SetShelves(ctx context.Context, ids []int64) error
}

// GetHome はホームの棚の要約と整理役の状態を返す（GET /api/home）。
func (s *server) GetHome(w http.ResponseWriter, r *http.Request) {
	if !s.homeConfigured(w) {
		return
	}
	audience := audienceFrom(r.Context())
	home, err := s.home.Home(r.Context(), audience)
	if err != nil {
		s.internalError(w, "Could not load the home.", err)
		return
	}
	out := gen.Home{Shelves: make([]gen.HomeShelf, 0, len(home.Shelves))}
	for _, shelf := range home.Shelves {
		out.Shelves = append(out.Shelves, toAPIHomeShelf(audience, shelf))
	}
	if home.NoVideos {
		empty := gen.NoVideos
		out.Empty = &empty
	}
	if audience.IsOwner() && home.Organizer != nil {
		out.Organizer = toAPIHomeOrganizer(*home.Organizer)
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// ListHomeShelfItems は棚の最初の項目を返す（GET /api/home/shelves）。
func (s *server) ListHomeShelfItems(w http.ResponseWriter, r *http.Request, params gen.ListHomeShelfItemsParams) {
	keys, err := domain.ParseHomeShelfKeys(params.Keys)
	if err != nil {
		s.invalidRequest(w, fmt.Sprintf("keys must name between 1 and %d shelves, each folder:<id>, continue, recent, unshelved or recommended.",
			domain.MaxHomeShelfKeys))
		return
	}
	if !s.homeConfigured(w) {
		return
	}
	audience := audienceFrom(r.Context())
	page, err := s.home.ShelfItems(r.Context(), audience, keys)
	if err != nil {
		s.internalError(w, "Could not load the home shelves.", err)
		return
	}
	out := gen.HomeShelfItemsList{Shelves: make([]gen.HomeShelfItems, 0, len(page.Shelves))}
	for _, shelf := range page.Shelves {
		items, err := s.presentLibraryItems(r.Context(), audience, page.Roots, shelf.Items)
		if err != nil {
			s.internalError(w, "Could not load the home shelves.", err)
			return
		}
		out.Shelves = append(out.Shelves, gen.HomeShelfItems{Key: shelf.Key.String(), Items: items, Total: shelf.Total})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// SetHomeShelves はどのフォルダを棚にするかとその並びを書く（PUT /api/home/shelves）。
func (s *server) SetHomeShelves(w http.ResponseWriter, r *http.Request) {
	if !s.homeConfigured(w) {
		return
	}
	var body struct {
		FolderIDs *[]int64 `json:"folderIds"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.FolderIDs == nil {
		s.invalidRequest(w, "folderIds is required.")
		return
	}
	err := s.home.SetShelves(r.Context(), *body.FolderIDs)
	switch {
	case errors.Is(err, domain.ErrInvalidHomeShelves):
		s.invalidRequest(w, fmt.Sprintf("folderIds must name at most %d smart folders, each once.", domain.MaxHomeShelves))
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "A smart folder in folderIds does not exist.")
		return
	case err != nil:
		s.internalError(w, "Could not save the home shelves.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// homeConfigured は保存先が無ければ 500 を書いて false を返す。
func (s *server) homeConfigured(w http.ResponseWriter) bool {
	if s.home == nil {
		s.internalError(w, "Home storage is not configured.", nil)
		return false
	}
	return true
}

// toAPIHomeShelf は棚の要約を応答の形にする。byOrganizer はフォルダの棚の所有者の応答にだけ載せる。
func toAPIHomeShelf(audience domain.Audience, shelf domain.HomeShelf) gen.HomeShelf {
	out := gen.HomeShelf{Key: shelf.Key.String(), Kind: gen.HomeShelfKind(shelf.Key.Kind), Name: shelf.Name}
	if shelf.Key.Kind != domain.HomeShelfFolder {
		return out
	}
	id := shelf.Key.FolderID
	out.FolderId = &id
	if audience.IsOwner() {
		byOrganizer := shelf.ByOrganizer
		out.ByOrganizer = &byOrganizer
	}
	return out
}

// toAPIHomeOrganizer は整理役の状態を応答の形にする。
func toAPIHomeOrganizer(state domain.HomeOrganizerState) *gen.HomeOrganizer {
	out := &gen.HomeOrganizer{Running: state.Running}
	if state.Pending != "" {
		pending := gen.HomeOrganizerPending(state.Pending)
		out.Pending = &pending
	}
	if state.Last != nil {
		out.Last = &gen.HomeOrganizeRun{
			Run: gen.HomeOrganizeRunRun(state.Last.Run), Outcome: gen.HomeOrganizeRunOutcome(state.Last.Outcome),
			FinishedAt: state.Last.FinishedAt, Added: state.Last.Added, Removed: state.Last.Removed,
		}
	}
	return out
}
