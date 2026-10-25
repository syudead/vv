package httpapi

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// pagedLibrary answers ListVideos with pages keyed by the request's cursor.
type pagedLibrary struct {
	Library
	pages map[string]domain.VideoPage
}

func (l pagedLibrary) ListVideos(_ context.Context, _ domain.Audience, q domain.VideoQuery) (domain.VideoPage, error) {
	return l.pages[q.Cursor], nil
}

func TestLocalMatchIDsReadsOnWhileTheMatchesRepeatHeldIDs(t *testing.T) {
	videos := func(ids ...int64) []domain.Video {
		out := []domain.Video{}
		for _, id := range ids {
			out = append(out, domain.Video{ID: id})
		}
		return out
	}
	s := &server{videos: pagedLibrary{pages: map[string]domain.VideoPage{
		"":     {Items: videos(1, 2), NextCursor: "next"},
		"next": {Items: videos(3)},
	}}}

	got, err := s.localMatchIDs(context.Background(), domain.VideoQuery{}, []int64{1, 2}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, 3) {
		t.Errorf("ids = %v, want the second page's 3: held 1 and 2 plus 3 is within the limit of 3", got)
	}
}
