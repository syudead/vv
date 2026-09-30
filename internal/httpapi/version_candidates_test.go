package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 候補の一覧と「違う動画」の記録（specs/030-video-versions/contracts/screen-api.md §5）は、
// 本物の認証・保存層で確かめる（#583）。guestFixture の a・b・c はどれも尺 60 秒で、a と b に
// 一致する指紋、c に違う指紋を書く。

// candidateFingerprint は 12 コマの指紋を返す。seed が同じなら同じ場面で、flip のビットを反転する。
func candidateFingerprint(seed, flip uint64) domain.Fingerprint {
	frames := make([]domain.FrameHash, 12)
	state := seed
	for i := range frames {
		state = state*6364136223846793005 + 1442695040888963407
		frames[i] = domain.FrameHash{Hash: state ^ flip}
	}
	return domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: 5000, Frames: frames}
}

// writeFingerprints は取り込みの fingerprint の段階を本物の保存層で進める。fingerprints の動画に
// シーク用サムネイルの完了を記録し、積まれた指紋の仕事で指紋を書く（候補はこの取引で作られる）。
func (f *guestFixture) writeFingerprints(t *testing.T, fingerprints map[string]domain.Fingerprint) {
	t.Helper()
	ctx := context.Background()
	ingest := f.env.db.Ingest()
	byID := map[int64]domain.Fingerprint{}
	for name, fingerprint := range fingerprints {
		byID[f.ids[name]] = fingerprint
		if err := ingest.EnqueueJob(ctx, domain.JobSeekThumbnail, f.ids[name]); err != nil {
			t.Fatal(err)
		}
	}
	for {
		job, err := ingest.ClaimJob(ctx, domain.JobSeekThumbnail)
		if errors.Is(err, domain.ErrNoJob) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := byID[job.VideoID]; ok {
			if _, err := ingest.SetSeekThumbnailStateForJob(ctx, job, domain.SeekThumbnailDone, domain.SubstitutionNone); err != nil {
				t.Fatal(err)
			}
		}
		if err := ingest.CompleteClaimedJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	f.runFingerprintJobs(t, byID)
}

// refingerprint は動画の指紋の仕事を積み直して書き直す（再スキャンの取り込みに当たる）。
func (f *guestFixture) refingerprint(t *testing.T, fingerprints map[string]domain.Fingerprint) {
	t.Helper()
	byID := map[int64]domain.Fingerprint{}
	for name, fingerprint := range fingerprints {
		byID[f.ids[name]] = fingerprint
		if err := f.env.db.Ingest().EnqueueJob(context.Background(), domain.JobFingerprint, f.ids[name]); err != nil {
			t.Fatal(err)
		}
	}
	f.runFingerprintJobs(t, byID)
}

func (f *guestFixture) runFingerprintJobs(t *testing.T, byID map[int64]domain.Fingerprint) {
	t.Helper()
	ctx := context.Background()
	ingest := f.env.db.Ingest()
	for {
		job, err := ingest.ClaimJob(ctx, domain.JobFingerprint)
		if errors.Is(err, domain.ErrNoJob) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if fingerprint, ok := byID[job.VideoID]; ok {
			if applied, err := ingest.ApplyFingerprintForJob(ctx, job, fingerprint); err != nil || !applied {
				t.Fatalf("ApplyFingerprintForJob(%d) = %v, %v", job.VideoID, applied, err)
			}
		}
		if err := ingest.CompleteClaimedJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
}

func candidateFixtureFingerprints() map[string]domain.Fingerprint {
	return map[string]domain.Fingerprint{
		"a": candidateFingerprint(1, 0),
		"b": candidateFingerprint(1, 0b11),
		"c": candidateFingerprint(2, 0),
	}
}

func (f *guestFixture) listCandidates(t *testing.T) gen.VersionCandidatePage {
	t.Helper()
	rec := f.env.get("/api/version-candidates", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("候補の一覧: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	return decode[gen.VersionCandidatePage](t, rec)
}

func (f *guestFixture) dismiss(ids []int64, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"videoIds": ids})
	return f.env.serve(authRequest{method: http.MethodPost, target: "/api/version-candidates/dismiss",
		body: string(body), cookies: cookies})
}

func candidateIDs(page gen.VersionCandidatePage) [][]int64 {
	out := [][]int64{}
	for _, item := range page.Items {
		out = append(out, []int64{item.Videos[0].Id, item.Videos[1].Id})
	}
	return out
}

// 所有者の一覧は指紋が一致する a と b の組を、どちらも GET /api/videos/{id} と同じ形で返し、
// 束ねると消える（受け入れ条件 3・4）。
func TestVersionCandidatesListAndBundle(t *testing.T) {
	f := newGuestFixture(t, true)
	f.writeFingerprints(t, candidateFixtureFingerprints())
	a, b := f.ids["a"], f.ids["b"]

	page := f.listCandidates(t)
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("候補 = %v (total %d), want [[%d %d]]", candidateIDs(page), page.Total, a, b)
	}
	item := page.Items[0]
	if len(item.Videos) != 2 || item.Videos[0].Id != min(a, b) || item.Videos[1].Id != max(a, b) {
		t.Fatalf("候補の動画 = %v, want id の昇順の [%d %d]", candidateIDs(page), a, b)
	}
	if item.Distance != 2 {
		t.Errorf("distance = %d, want 2", item.Distance)
	}
	for _, video := range item.Videos {
		if video.Location == nil || video.Folder == nil || video.DurationMs == nil || video.Title == "" {
			t.Errorf("候補の動画 %d に所在・フォルダ・尺・題名が無い: %+v", video.Id, video)
		}
	}
	// a は再生位置とタグを持つ（詳細と同じ値）。
	for _, video := range item.Videos {
		if video.Id == a && (video.Progress == nil || !slices.Contains(tagNames(video.Tags), guestSecretTag)) {
			t.Errorf("候補の a に再生位置かタグが無い: progress = %+v, tags = %v", video.Progress, tagNames(video.Tags))
		}
	}

	if rec := f.bundle(bundleBody([]int64{a, b}, a), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("束ねる: status = %d: %s", rec.Code, rec.Body)
	}
	if page := f.listCandidates(t); page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("束ねたあとの候補 = %v (total %d)", candidateIDs(page), page.Total)
	}
}

// 「違う動画」と記録すると候補から消え、指紋を書き直しても（再スキャン）出ない（受け入れ条件 5）。
func TestVersionCandidatesDismiss(t *testing.T) {
	f := newGuestFixture(t, true)
	f.writeFingerprints(t, candidateFixtureFingerprints())
	a, b := f.ids["a"], f.ids["b"]

	rec := f.dismiss([]int64{b, a}, f.owner)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("却下: status = %d: %s", rec.Code, rec.Body)
	}
	if page := f.listCandidates(t); page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("却下したあとの候補 = %v", candidateIDs(page))
	}
	f.refingerprint(t, candidateFixtureFingerprints())
	if page := f.listCandidates(t); page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("再スキャンのあとの候補 = %v", candidateIDs(page))
	}
	// 候補に無い組も記録できる。
	if rec := f.dismiss([]int64{a, f.ids["c"]}, f.owner); rec.Code != http.StatusNoContent {
		t.Errorf("候補に無い組の却下: status = %d: %s", rec.Code, rec.Body)
	}
}

// id が 2 つでない・同じなら 400、無い動画なら 404。
func TestVersionCandidatesDismissErrors(t *testing.T) {
	f := newGuestFixture(t, true)
	f.writeFingerprints(t, candidateFixtureFingerprints())
	a, b := f.ids["a"], f.ids["b"]
	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	for label, ids := range map[string][]int64{
		"1 つ":   {a},
		"3 つ":   {a, b, f.ids["c"]},
		"同じ id": {a, a},
		"空":     {},
	} {
		rec := f.dismiss(ids, f.owner)
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), invalid)
	}
	rec := f.dismiss([]int64{a, 999_999}, f.owner)
	assertErrorBody(t, "無い動画", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound})
	if page := f.listCandidates(t); len(page.Items) != 1 {
		t.Errorf("誤りのあとの候補 = %v, want 1 組", candidateIDs(page))
	}
}

// ゲストはどちらの経路も 401 で、何も変わらない。
func TestVersionCandidatesForGuest(t *testing.T) {
	f := newGuestFixture(t, true)
	f.writeFingerprints(t, candidateFixtureFingerprints())
	for label, rec := range map[string]*httptest.ResponseRecorder{
		"一覧": f.env.get("/api/version-candidates"),
		"却下": f.dismiss([]int64{f.ids["a"], f.ids["b"]}),
	} {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("ゲストの%s: status = %d, want 401: %s", label, rec.Code, rec.Body)
		}
	}
	if page := f.listCandidates(t); len(page.Items) != 1 {
		t.Errorf("ゲストの操作のあとの候補 = %v, want 1 組", candidateIDs(page))
	}
}
