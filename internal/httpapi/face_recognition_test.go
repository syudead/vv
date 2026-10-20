package httpapi

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// faceRuntimeStub reports a fixed state and records the recognizers it was asked to download.
type faceRuntimeStub struct {
	status domain.FaceRuntimeStatus
	starts []domain.FaceRecognizer
}

func (*faceRuntimeStub) Recognizers() []domain.FaceRecognizerDownload {
	return []domain.FaceRecognizerDownload{{
		Recognizer: domain.FaceRecognizerArcFaceR50, DownloadBytes: 174, License: "non-commercial", NonCommercial: true,
	}}
}

func (r *faceRuntimeStub) Status(domain.FaceRecognizer) domain.FaceRuntimeStatus { return r.status }

func (r *faceRuntimeStub) Start(rec domain.FaceRecognizer) error {
	r.starts = append(r.starts, rec)
	return nil
}

func (*faceRuntimeStub) Stop() {}

func newFaceRecognitionEnv(t *testing.T, rt *faceRuntimeStub) (*authEnv, *http.Cookie) {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		analysis := app.NewFaceRecognizer(app.FaceRecognizerOptions{Queue: db.Faces(), Settings: db.Settings(), Runtime: rt})
		return Options{
			FaceRecognition: app.NewFaceRecognitionSettings(app.FaceRecognitionSettingsOptions{
				Settings: db.Settings(), Queue: db.Faces(), Runtime: rt, Analysis: analysis,
			}),
			FaceAnalysis: analysis,
		}
	})
	return env, env.setup()
}

func TestFaceRecognitionDownloadIsRefusedWhileOffAndStartsWhileOn(t *testing.T) {
	rt := &faceRuntimeStub{status: domain.FaceRuntimeStatus{State: domain.FaceRuntimeDownloading, BytesDone: 5, BytesTotal: 10}}
	env, owner := newFaceRecognitionEnv(t, rt)
	send := func(method, target, body string) *http.Response {
		return env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{owner}}).Result()
	}

	off := env.serve(authRequest{method: http.MethodPost, target: "/api/face-recognition/download", cookies: []*http.Cookie{owner}})
	if body := decode[gen.Error](t, off); off.Code != http.StatusConflict || body.Reason == nil ||
		*body.Reason != gen.ErrorReasonFaceRecognitionOff {
		t.Fatalf("download while off: status = %d: %s", off.Code, off.Body)
	}
	if res := send(http.MethodPut, "/api/settings/face-recognition", `{"enabled":true,"recognizer":"other"}`); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown recognizer: status = %d", res.StatusCode)
	}
	if res := send(http.MethodPut, "/api/settings/face-recognition", `{"enabled":true,"recognizer":"arcface_r50"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("turning on: status = %d", res.StatusCode)
	}
	on := env.serve(authRequest{method: http.MethodPost, target: "/api/face-recognition/download", cookies: []*http.Cookie{owner}})
	if on.Code != http.StatusAccepted {
		t.Fatalf("download while on: status = %d: %s", on.Code, on.Body)
	}
	got := decode[gen.FaceRecognitionSettings](t, on)
	if !got.Enabled || got.Recognizer != "arcface_r50" || got.Runtime.State != gen.FaceRuntimeStateDownloading ||
		got.Runtime.BytesDone == nil || *got.Runtime.BytesDone != 5 || got.Runtime.BytesTotal == nil || *got.Runtime.BytesTotal != 10 ||
		len(got.Recognizers) != 1 || !got.Recognizers[0].NonCommercial {
		t.Fatalf("download response = %+v", got)
	}
	want := []domain.FaceRecognizer{domain.FaceRecognizerArcFaceR50, domain.FaceRecognizerArcFaceR50}
	if !slices.Equal(rt.starts, want) {
		t.Fatalf("starts = %v, want %v (the save, then the download)", rt.starts, want)
	}
}

func TestFaceRecognitionIsOwnerOnly(t *testing.T) {
	env, _ := newFaceRecognitionEnv(t, &faceRuntimeStub{})
	assertUnauthenticated(t, "GET", env.get("/api/settings/face-recognition"))
	for _, req := range []authRequest{
		{method: http.MethodPut, target: "/api/settings/face-recognition", body: `{"enabled":true,"recognizer":"sface_int8"}`},
		{method: http.MethodPost, target: "/api/face-recognition/download"},
		{method: http.MethodPost, target: "/api/face-recognition/runs", body: `{"scope":"missing"}`},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, env.serve(req))
	}
}

func TestFaceRecognitionRunsAreRefusedWhileOff(t *testing.T) {
	env, owner := newFaceRecognitionEnv(t, &faceRuntimeStub{})
	send := func(method, target, body string) *httptest.ResponseRecorder {
		return env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{owner}})
	}

	off := send(http.MethodPost, "/api/face-recognition/runs", `{"scope":"missing"}`)
	if body := decode[gen.Error](t, off); off.Code != http.StatusConflict || body.Reason == nil ||
		*body.Reason != gen.ErrorReasonFaceRecognitionOff {
		t.Fatalf("runs while off: status = %d: %s", off.Code, off.Body)
	}
	// all is gone: an analysed video is analysed again only by a recognizer switch.
	for _, scope := range []string{"some", "all"} {
		if res := send(http.MethodPost, "/api/face-recognition/runs", `{"scope":"`+scope+`"}`); res.Code != http.StatusBadRequest {
			t.Fatalf("scope %s: status = %d", scope, res.Code)
		}
	}
	if res := send(http.MethodPut, "/api/settings/face-recognition", `{"enabled":true,"recognizer":"sface_int8"}`); res.Code != http.StatusOK {
		t.Fatalf("turning on: status = %d", res.Code)
	}
	on := send(http.MethodPost, "/api/face-recognition/runs", `{"scope":"missing"}`)
	if got := decode[gen.FaceRecognitionQueued](t, on); on.Code != http.StatusAccepted || got.Queued != 0 {
		t.Fatalf("runs while on: status = %d, body = %+v", on.Code, got)
	}
}
