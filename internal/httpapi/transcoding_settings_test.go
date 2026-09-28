package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// memoryEncoderStore は app.TranscodeSettingsStore の代わりで、保存値をメモリに持つ。
type memoryEncoderStore struct {
	mu      sync.Mutex
	value   string
	found   bool
	saves   int
	saveErr error
}

func (s *memoryEncoderStore) TranscodeEncoderChoice(context.Context) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value, s.found, nil
}

func (s *memoryEncoderStore) SaveTranscodeEncoderChoice(_ context.Context, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.value, s.found = value, true
	s.saves++
	return nil
}

func (s *memoryEncoderStore) stored() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value, s.found
}

// fixedEncoderChecker は方式ごとに決め打ちの確認結果を返す。release が nil でなければ、
// 閉じるまで確認を終えない（確認中を模す）。
type fixedEncoderChecker struct {
	results map[domain.VideoEncoder]domain.EncoderAvailability
	release chan struct{}
}

func (c fixedEncoderChecker) CheckEncoder(ctx context.Context, encoder domain.VideoEncoder) domain.EncoderAvailability {
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
		}
	}
	if result, ok := c.results[encoder]; ok {
		return result
	}
	return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonCheckFailed}
}

// linuxCheckResults は linux の確認対象のうち NVENC だけが使える結果である。
func linuxCheckResults() map[domain.VideoEncoder]domain.EncoderAvailability {
	return map[domain.VideoEncoder]domain.EncoderAvailability{
		domain.VideoEncoderNVENC: {Encoder: domain.VideoEncoderNVENC, State: domain.EncoderAvailable},
		domain.VideoEncoderQSV: {
			Encoder: domain.VideoEncoderQSV, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonEncoderMissing,
		},
		domain.VideoEncoderVAAPI: {
			Encoder: domain.VideoEncoderVAAPI, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonCheckFailed,
			Detail: "Failed to open /dev/dri/renderD128",
		},
	}
}

// newCheckedTranscodeSettings は linux として確認を終えた本物の app.TranscodeSettings を返す。
func newCheckedTranscodeSettings(t *testing.T, store *memoryEncoderStore) *app.TranscodeSettings {
	t.Helper()
	settings := newTranscodeSettings(t, store, fixedEncoderChecker{results: linuxCheckResults()})
	settings.StartChecks(context.Background())
	<-settings.Done()
	return settings
}

func newTranscodeSettings(t *testing.T, store *memoryEncoderStore, checker app.EncoderChecker) *app.TranscodeSettings {
	t.Helper()
	settings, err := app.NewTranscodeSettings(context.Background(), app.TranscodeSettingsOptions{
		Store: store, Checker: checker, GOOS: "linux", Logger: discardLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func putTranscoding(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/settings/transcoding", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func assertTranscodingResponse(t *testing.T, label string, rec *httptest.ResponseRecorder, want gen.TranscodingSettings) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", label, rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("%s: Cache-Control = %q", label, got)
	}
	got := decode[gen.TranscodingSettings](t, rec)
	if got.VideoEncoder != want.VideoEncoder || got.EffectiveEncoder != want.EffectiveEncoder || got.Checking != want.Checking {
		t.Errorf("%s: videoEncoder/effectiveEncoder/checking = %s/%s/%v, want %s/%s/%v", label,
			got.VideoEncoder, got.EffectiveEncoder, got.Checking, want.VideoEncoder, want.EffectiveEncoder, want.Checking)
	}
	switch {
	case want.FallbackReason == nil && got.FallbackReason != nil:
		t.Errorf("%s: fallbackReason = %q, want なし", label, *got.FallbackReason)
	case want.FallbackReason != nil && (got.FallbackReason == nil || *got.FallbackReason != *want.FallbackReason):
		t.Errorf("%s: fallbackReason = %v, want %q", label, got.FallbackReason, *want.FallbackReason)
	}
	if len(got.Encoders) != len(want.Encoders) {
		t.Fatalf("%s: encoders = %+v, want %+v", label, got.Encoders, want.Encoders)
	}
	for i := range want.Encoders {
		g, w := got.Encoders[i], want.Encoders[i]
		if g.Encoder != w.Encoder || g.State != w.State || (g.Reason == nil) != (w.Reason == nil) ||
			(g.Reason != nil && *g.Reason != *w.Reason) {
			t.Errorf("%s: encoders[%d] = %+v (reason %v), want %+v (reason %v)", label, i, g, g.Reason, w, w.Reason)
		}
	}
}

func unavailableReason(reason gen.EncoderUnavailableReason) *gen.EncoderUnavailableReason {
	return &reason
}

func fallbackReason(reason gen.EncoderFallbackReason) *gen.EncoderFallbackReason { return &reason }

// linuxCheckedEncoders は linuxCheckResults を確認し終えたときの encoders である。確認の
// 補足（Detail）は応答に出ない。
func linuxCheckedEncoders() []gen.EncoderAvailability {
	return []gen.EncoderAvailability{
		{Encoder: gen.VideoEncoderNvenc, State: gen.EncoderAvailabilityStateAvailable},
		{Encoder: gen.VideoEncoderQsv, State: gen.EncoderAvailabilityStateUnavailable, Reason: unavailableReason(gen.EncoderMissing)},
		{Encoder: gen.VideoEncoderVaapi, State: gen.EncoderAvailabilityStateUnavailable, Reason: unavailableReason(gen.CheckFailed)},
		{Encoder: gen.VideoEncoderVideotoolbox, State: gen.EncoderAvailabilityStateUnavailable, Reason: unavailableReason(gen.UnsupportedOs)},
	}
}

func TestGetTranscodingSettingsReturnsContractShape(t *testing.T) {
	store := &memoryEncoderStore{}
	handler := newTestServer(t, Options{TranscodeSettings: newCheckedTranscodeSettings(t, store)})

	rec := do(t, handler, http.MethodGet, "/api/settings/transcoding")
	assertTranscodingResponse(t, "未選択", rec, gen.TranscodingSettings{
		VideoEncoder: gen.VideoEncoderChoiceSoftware, EffectiveEncoder: gen.VideoEncoderSoftware,
		Encoders: linuxCheckedEncoders(),
	})
	if strings.Contains(rec.Body.String(), "renderD128") {
		t.Errorf("確認の補足が応答に出ている: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "fallbackReason") {
		t.Errorf("software の選択に fallbackReason がある: %s", rec.Body)
	}
}

func TestGetTranscodingSettingsWhileChecking(t *testing.T) {
	store := &memoryEncoderStore{value: "nvenc", found: true}
	release := make(chan struct{})
	settings := newTranscodeSettings(t, store, fixedEncoderChecker{results: linuxCheckResults(), release: release})
	settings.StartChecks(context.Background())
	t.Cleanup(func() {
		close(release)
		<-settings.Done()
	})
	handler := newTestServer(t, Options{TranscodeSettings: settings})

	checking := gen.EncoderAvailability{State: gen.EncoderAvailabilityStateChecking}
	encoders := []gen.EncoderAvailability{checking, checking, checking,
		{Encoder: gen.VideoEncoderVideotoolbox, State: gen.EncoderAvailabilityStateUnavailable, Reason: unavailableReason(gen.UnsupportedOs)}}
	for i, encoder := range []gen.VideoEncoder{gen.VideoEncoderNvenc, gen.VideoEncoderQsv, gen.VideoEncoderVaapi} {
		encoders[i].Encoder = encoder
	}
	assertTranscodingResponse(t, "確認中", do(t, handler, http.MethodGet, "/api/settings/transcoding"), gen.TranscodingSettings{
		VideoEncoder: gen.VideoEncoderChoiceNvenc, EffectiveEncoder: gen.VideoEncoderSoftware,
		FallbackReason: fallbackReason(gen.EncoderFallbackReasonChecking), Checking: true, Encoders: encoders,
	})
	// 確認中のハードウェアの方式は選べない。
	rec := putTranscoding(t, handler, `{"videoEncoder":"nvenc"}`)
	assertErrorBody(t, "確認中の nvenc", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonEncoderUnavailable,
	})
}

func TestUpdateTranscodingSettingsSavesAcceptedChoices(t *testing.T) {
	store := &memoryEncoderStore{}
	handler := newTestServer(t, Options{TranscodeSettings: newCheckedTranscodeSettings(t, store)})

	cases := []struct {
		choice    gen.VideoEncoderChoice
		effective gen.VideoEncoder
		fallback  *gen.EncoderFallbackReason
	}{
		{gen.VideoEncoderChoiceNvenc, gen.VideoEncoderNvenc, nil},
		{gen.VideoEncoderChoiceSoftware, gen.VideoEncoderSoftware, nil},
		{gen.VideoEncoderChoiceAuto, gen.VideoEncoderNvenc, nil},
	}
	for _, tc := range cases {
		label := string(tc.choice)
		rec := putTranscoding(t, handler, `{"videoEncoder":"`+label+`"}`)
		want := gen.TranscodingSettings{
			VideoEncoder: tc.choice, EffectiveEncoder: tc.effective, FallbackReason: tc.fallback, Encoders: linuxCheckedEncoders(),
		}
		assertTranscodingResponse(t, "PUT "+label, rec, want)
		if value, found := store.stored(); !found || value != label {
			t.Errorf("PUT %s: 保存値 = %q (found %v)", label, value, found)
		}
		assertTranscodingResponse(t, "PUT のあとの GET "+label, do(t, handler, http.MethodGet, "/api/settings/transcoding"), want)
	}
}

func TestUpdateTranscodingSettingsRejectsWithoutSaving(t *testing.T) {
	store := &memoryEncoderStore{value: "auto", found: true}
	handler := newTestServer(t, Options{TranscodeSettings: newCheckedTranscodeSettings(t, store)})

	cases := []struct {
		label string
		body  string
		want  wantError
	}{
		{"使えない方式", `{"videoEncoder":"vaapi"}`, wantError{
			status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonEncoderUnavailable,
		}},
		{"この OS に無い方式", `{"videoEncoder":"videotoolbox"}`, wantError{
			status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonEncoderUnavailable,
		}},
		{"列挙に無い値", `{"videoEncoder":"x264"}`, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
		{"値が無い", `{}`, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
		{"知らない項目", `{"videoEncoder":"software","extra":1}`, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
		{"JSON でない", `software`, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
	}
	for _, tc := range cases {
		rec := putTranscoding(t, handler, tc.body)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), tc.want)
		if value, _ := store.stored(); value != "auto" || store.saves != 0 {
			t.Errorf("%s: 保存値 = %q, 保存 %d 回", tc.label, value, store.saves)
		}
	}

	// Content-Type が JSON でない本文は、経路に届く前に 400 になる。
	req := httptest.NewRequest(http.MethodPut, "/api/settings/transcoding", strings.NewReader(`{"videoEncoder":"software"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assertErrorBody(t, "text/plain", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
}

func TestUpdateTranscodingSettingsReportsSaveFailure(t *testing.T) {
	store := &memoryEncoderStore{saveErr: errors.New("disk I/O error")}
	handler := newTestServer(t, Options{TranscodeSettings: newCheckedTranscodeSettings(t, store), Logger: discardLogger()})

	rec := putTranscoding(t, handler, `{"videoEncoder":"auto"}`)
	assertErrorBody(t, "保存の失敗", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusInternalServerError, code: gen.ErrorCodeInternal})
	if strings.Contains(rec.Body.String(), "disk I/O") {
		t.Errorf("保存の誤りが応答に出ている: %s", rec.Body)
	}
	assertTranscodingResponse(t, "失敗のあとの GET", do(t, handler, http.MethodGet, "/api/settings/transcoding"), gen.TranscodingSettings{
		VideoEncoder: gen.VideoEncoderChoiceSoftware, EffectiveEncoder: gen.VideoEncoderSoftware, Encoders: linuxCheckedEncoders(),
	})
}

func TestTranscodingSettingsWithoutConfigurationIsInternalError(t *testing.T) {
	handler := newTestServer(t, Options{Logger: discardLogger()})
	for _, rec := range []*httptest.ResponseRecorder{
		do(t, handler, http.MethodGet, "/api/settings/transcoding"),
		putTranscoding(t, handler, `{"videoEncoder":"software"}`),
	} {
		assertErrorBody(t, "設定なし", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusInternalServerError, code: gen.ErrorCodeInternal})
	}
}

// ゲストは方式も一覧も見られず、変えられない（親 Issue #370 要件 13）。
func TestTranscodingSettingsAreOwnerOnly(t *testing.T) {
	store := &memoryEncoderStore{}
	env := newAuthEnv(t, t.TempDir(), Options{TranscodeSettings: newCheckedTranscodeSettings(t, store)})
	env.setup()

	assertUnauthenticated(t, "ゲストの GET", env.get("/api/settings/transcoding"))
	assertUnauthenticated(t, "ゲストの PUT", env.serve(authRequest{
		method: http.MethodPut, target: "/api/settings/transcoding", body: `{"videoEncoder":"nvenc"}`,
	}))
	crossSite := env.serve(authRequest{
		method: http.MethodPut, target: "/api/settings/transcoding", body: `{"videoEncoder":"nvenc"}`,
		cookies: []*http.Cookie{env.login(false)}, header: map[string]string{"Sec-Fetch-Site": "cross-site"},
	})
	assertErrorBody(t, "別サイトからの PUT", crossSite.Code, crossSite.Body.Bytes(), wantError{
		status: http.StatusForbidden, code: gen.ErrorCodeForbidden, reason: gen.ErrorReasonCrossOrigin,
	})
	if _, found := store.stored(); found {
		t.Error("拒んだ要求で保存された")
	}
}
