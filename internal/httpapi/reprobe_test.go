package httpapi

import (
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakeReprober は保存層の RetryProbe と同じ規則で状態を戻し、積んだジョブを数える。
// シーク用プレビューの置き場は無いものとする。
type fakeReprober struct {
	library *fakeLibrary
	jobs    map[domain.JobKind]int
}

func (f *fakeReprober) catalog() *fakeCatalog {
	return &fakeCatalog{retry: func(video domain.Video) error { return f.retryProbe(video.ID, true) }}
}

func (f *fakeReprober) retryProbe(id int64, seekThumbnailMissing bool) error {
	video, ok := f.library.videos[id]
	if !ok {
		return domain.ErrNotFound
	}
	if video.ProbeState != domain.ProbeStateFailed {
		return domain.ErrProbeNotFailed
	}
	video.ProbeState = domain.ProbeStatePending
	video.ProbeError = ""
	f.jobs[domain.JobProbe]++
	if video.ThumbnailState != domain.ThumbnailStateDone || seekThumbnailMissing {
		if video.ThumbnailState != domain.ThumbnailStateDone {
			video.ThumbnailState = domain.ThumbnailStatePending
		}
		f.jobs[domain.JobThumbnail]++
	}
	if video.PreviewState == domain.PreviewStateFailed {
		video.PreviewState = domain.PreviewStatePending
	}
	f.library.videos[id] = video
	return nil
}

func failedProbeVideo() domain.Video {
	video := sampleVideo(1, "壊れた動画")
	video.ProbeState = domain.ProbeStateFailed
	video.ProbeError = "moov atom not found"
	video.DurationMs = nil
	video.VideoCodec = ""
	video.ThumbnailState = domain.ThumbnailStateFailed
	video.PreviewState = domain.PreviewStateFailed
	return video
}

// 失敗した動画への要求は 202 と更新後の動画を返す。続けて送ると 2 回目は
// 409 probe_not_failed で、ジョブは増えない。
func TestReprobeVideo(t *testing.T) {
	library := &fakeLibrary{videos: map[int64]domain.Video{1: failedProbeVideo()}}
	reprober := &fakeReprober{library: library, jobs: map[domain.JobKind]int{}}
	handler := newTestServer(t, Options{Videos: library, Catalog: reprober.catalog()})

	rec := do(t, handler, http.MethodPost, "/api/videos/1/probe")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}
	got := decode[gen.Video](t, rec)
	if got.ProbeState != gen.VideoProbeStatePending || got.ProbeError != nil {
		t.Fatalf("probeState = %q, probeError = %v", got.ProbeState, got.ProbeError)
	}
	if got.ThumbnailState != gen.VideoThumbnailStatePending || got.PreviewState != gen.VideoPreviewStatePending {
		t.Fatalf("thumbnailState = %q, previewState = %q", got.ThumbnailState, got.PreviewState)
	}
	if got.Location != nil {
		t.Error("読み取りのやり直しの応答に location が載っている")
	}

	assertErrorCode(t, do(t, handler, http.MethodPost, "/api/videos/1/probe"), http.StatusConflict, codeProbeNotFailed)
	if reprober.jobs[domain.JobProbe] != 1 || reprober.jobs[domain.JobThumbnail] != 1 {
		t.Fatalf("jobs = %v, want probe 1 / thumbnail 1", reprober.jobs)
	}
}

// 読み取り済みの動画は 409、知らない id は 404 になる。
func TestReprobeVideoRejectsNonFailedAndUnknown(t *testing.T) {
	library := &fakeLibrary{videos: map[int64]domain.Video{1: sampleVideo(1, "読み取り済み")}}
	reprober := &fakeReprober{library: library, jobs: map[domain.JobKind]int{}}
	handler := newTestServer(t, Options{Videos: library, Catalog: reprober.catalog()})

	assertErrorCode(t, do(t, handler, http.MethodPost, "/api/videos/1/probe"), http.StatusConflict, codeProbeNotFailed)
	assertErrorCode(t, do(t, handler, http.MethodPost, "/api/videos/99/probe"), http.StatusNotFound, codeNotFound)
	if len(reprober.jobs) != 0 {
		t.Fatalf("jobs = %v, want none", reprober.jobs)
	}
}
