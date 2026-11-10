package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// fakeBackground records the requested states and answers the last one with work, as app.BackgroundPause does.
type fakeBackground struct {
	mu        sync.Mutex
	paused    bool
	work      domain.BackgroundWork
	requested []bool
}

func (f *fakeBackground) Status(context.Context, domain.Audience) (domain.BackgroundWork, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	work := f.work
	work.Paused = f.paused
	return work, nil
}

func (f *fakeBackground) SetPaused(_ context.Context, _ domain.Audience, paused bool) (domain.BackgroundWork, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paused = paused
	f.requested = append(f.requested, paused)
	return domain.BackgroundWork{Paused: paused}, nil
}

func (f *fakeBackground) requests() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.requested...)
}

type backgroundFixture struct {
	env        *authEnv
	background *fakeBackground
	owner      *http.Cookie
}

func newBackgroundFixture(t *testing.T) *backgroundFixture {
	t.Helper()
	f := &backgroundFixture{background: &fakeBackground{}}
	f.env = newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options { return Options{Background: f.background} })
	f.owner = f.env.setup()
	return f
}

func (f *backgroundFixture) put(body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: http.MethodPut, target: "/api/background/pause", body: body, cookies: cookies})
}

func TestBackgroundWorkAnswersTheSwitchBeforeAndAfterAChange(t *testing.T) {
	f := newBackgroundFixture(t)

	rec := f.env.get("/api/background", f.owner)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("GET = %d, Cache-Control %q: %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body)
	}
	if got := decode[gen.BackgroundWork](t, rec); got.Paused {
		t.Fatalf("GET before a change = %+v, want not paused", got)
	}

	rec = f.put(`{"paused":true}`, f.owner)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("PUT = %d, Cache-Control %q: %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body)
	}
	if got := decode[gen.BackgroundWork](t, rec); !got.Paused {
		t.Fatalf("PUT answer = %+v, want paused", got)
	}
	if got := f.background.requests(); len(got) != 1 || !got[0] {
		t.Fatalf("requested = %v, want [true]", got)
	}
	if got := decode[gen.BackgroundWork](t, f.env.get("/api/background", f.owner)); !got.Paused {
		t.Fatalf("GET after the pause = %+v, want paused", got)
	}
}

// The running list and the groups keep their order; waiting belongs to scan and featureOff to the job groups.
func TestBackgroundWorkAnswersTheRunningWorkAndGroups(t *testing.T) {
	f := newBackgroundFixture(t)
	f.background.work = domain.BackgroundWork{
		Running: []domain.BackgroundActivity{
			{Group: domain.BackgroundScan, Kind: domain.BackgroundRegistering, FileName: "a.mp4"},
			{Group: domain.BackgroundAutoTag, Kind: "auto_tag", Video: &domain.BackgroundVideo{ID: 7, Title: "Clip"}},
		},
		Groups: []domain.BackgroundGroupStatus{
			{Group: domain.BackgroundScan, Remaining: 4, Waiting: true},
			{Group: domain.BackgroundAutoTag, Remaining: 2, FeatureOff: true},
		},
	}

	rec := f.env.get("/api/background", f.owner)
	want := `{"groups":[{"group":"scan","remaining":4,"waiting":true},` +
		`{"featureOff":true,"group":"auto_tag","remaining":2}],"paused":false,"running":[` +
		`{"fileName":"a.mp4","group":"scan","kind":"registering"},` +
		`{"group":"auto_tag","kind":"auto_tag","video":{"id":7,"title":"Clip"}}]}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("GET = %s\nwant %s", got, want)
	}
}

func TestSetBackgroundPauseRejectsAnInvalidBody(t *testing.T) {
	f := newBackgroundFixture(t)

	for _, body := range []string{``, `{}`, `{"paused":"yes"}`, `{"paused":null}`, `nope`} {
		if rec := f.put(body, f.owner); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, rec.Code)
		}
	}
	if got := f.background.requests(); len(got) != 0 {
		t.Fatalf("requested = %v, want none", got)
	}
}

func TestBackgroundWorkIsOwnerOnly(t *testing.T) {
	f := newBackgroundFixture(t)

	assertUnauthenticated(t, "GET", f.env.get("/api/background"))
	assertUnauthenticated(t, "PUT", f.put(`{"paused":true}`))
	if got := f.background.requests(); len(got) != 0 {
		t.Fatalf("requested = %v, want none", got)
	}
}

// The switch concerns the owner's screens only: a guest or the paired node's stream never receives it.
func TestBackgroundEventsAreForTheOwnersScreens(t *testing.T) {
	events := NewEvents()
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	server := &server{}
	for _, tc := range []struct {
		guest, peer bool
		want        string
	}{
		{want: "event: background\ndata: {}\n\n"},
		{guest: true},
		{peer: true},
	} {
		events.BackgroundChanged()
		response := httptest.NewRecorder()
		if err := server.writePendingEvents(context.Background(), response, sub, tc.guest, tc.peer); err != nil {
			t.Fatal(err)
		}
		if response.Body.String() != tc.want {
			t.Fatalf("guest %t, peer %t: %q, want %q", tc.guest, tc.peer, response.Body.String(), tc.want)
		}
	}
}
