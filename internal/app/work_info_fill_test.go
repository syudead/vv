package app

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeFillTargets struct {
	targets []domain.WorkInfoFillTarget
}

func (f *fakeFillTargets) WorkInfoFillTargets(context.Context, []int64) ([]domain.WorkInfoFillTarget, error) {
	return f.targets, nil
}

type titledVideos map[int64]string

func (v titledVideos) GetVideo(_ context.Context, _ domain.Audience, id int64) (domain.Video, error) {
	title, ok := v[id]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return domain.Video{ID: id, Title: title}, nil
}

// A target searches by its code, or by its title without one; a video gone since is left out.
func TestWorkInfoFillQueriesUseTheCodeOrTheTitle(t *testing.T) {
	e := newFetchEnv()
	e.plugins.videos = titledVideos{fetchVideo: "Summer trip", otherVideo: "Winter"}
	e.plugins.fillTargets = &fakeFillTargets{targets: []domain.WorkInfoFillTarget{
		{VideoID: fetchVideo}, {VideoID: otherVideo, Code: "ABC-1"}, {VideoID: 99},
	}}
	got, err := e.plugins.WorkInfoFillQueries(context.Background(), []int64{fetchVideo, otherVideo, 99})
	want := []domain.WorkInfoFillQuery{
		{VideoID: fetchVideo, Title: "Summer trip", Query: "Summer trip"}, {VideoID: otherVideo, Title: "Winter", Query: "ABC-1"},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("WorkInfoFillQueries = %+v, %v; want %+v", got, err, want)
	}
}
