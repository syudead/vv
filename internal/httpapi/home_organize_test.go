package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// POST /api/home/organize は rebuild を待ちに入れ、202 で整理役の状態を返す（specs/057-home/
// contracts/screen-api.md）。整理役は Run で動かさないので、実行は待ちに残る。最後の実行があれば添える。
func TestOrganizeHomeQueuesARebuild(t *testing.T) {
	var db *store.DB
	env := newAuthEnvWith(t, t.TempDir(), func(opened *store.DB) Options {
		db = opened
		return Options{HomeOrganizer: app.NewHomeOrganizer(app.HomeOrganizerOptions{Store: opened.Home()})}
	})
	owner := env.setup()
	last := domain.HomeOrganizeLast{Run: domain.HomeExtend, Outcome: domain.HomeOrganizeUnchanged, FinishedAt: 1234}
	if err := db.Home().SaveLastOrganize(context.Background(), last); err != nil {
		t.Fatal(err)
	}

	rec := env.serve(authRequest{method: http.MethodPost, target: "/api/home/organize", cookies: []*http.Cookie{owner}})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	state := decode[gen.HomeOrganizer](t, rec)
	if state.Running || state.Pending == nil || *state.Pending != gen.HomeOrganizerPendingRebuild {
		t.Errorf("state = %+v, want a pending rebuild", state)
	}
	if state.Last == nil || state.Last.Run != gen.HomeOrganizeRunRunExtend ||
		state.Last.Outcome != gen.HomeOrganizeRunOutcomeUnchanged || state.Last.FinishedAt != 1234 {
		t.Errorf("last = %+v, want the saved extend", state.Last)
	}
	if run, found, err := db.Home().PendingOrganize(context.Background()); err != nil || !found || run != domain.HomeRebuild {
		t.Errorf("pending setting = %q %v %v, want rebuild", run, found, err)
	}
}

// 経路は所有者だけで、セッションの無い閲覧者は通らない。別オリジンからの書き込みは 403 である。
func TestOrganizeHomeIsForTheOwnerOnly(t *testing.T) {
	var db *store.DB
	env := newAuthEnvWith(t, t.TempDir(), func(opened *store.DB) Options {
		db = opened
		return Options{HomeOrganizer: app.NewHomeOrganizer(app.HomeOrganizerOptions{Store: opened.Home()})}
	})
	owner := env.setup()
	route := authRequest{method: http.MethodPost, target: "/api/home/organize"}
	assertErrorCode(t, env.serve(route), http.StatusUnauthorized, gen.ErrorCodeUnauthenticated)
	route.cookies = []*http.Cookie{owner}
	route.header = map[string]string{"Origin": "https://attacker.example"}
	assertErrorCode(t, env.serve(route), http.StatusForbidden, gen.ErrorCodeForbidden)
	if _, found, err := db.Home().PendingOrganize(context.Background()); err != nil || found {
		t.Errorf("pending setting found = %v %v, want nothing queued", found, err)
	}
}
