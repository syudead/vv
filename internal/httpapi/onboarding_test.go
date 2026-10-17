package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

type onboardingFixture struct {
	env   *authEnv
	owner *http.Cookie
}

// newOnboardingFixture は本物の保存先と app.Onboarding で経路を組む。自動タグ付けはつながない。
func newOnboardingFixture(t *testing.T) *onboardingFixture {
	t.Helper()
	f := &onboardingFixture{}
	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return Options{Onboarding: app.NewOnboarding(db.Home(), nil)}
	})
	f.owner = f.env.setup()
	return f
}

func (f *onboardingFixture) step(t *testing.T) gen.OnboardingStep {
	t.Helper()
	rec := f.env.get("/api/onboarding", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/onboarding status = %d: %s", rec.Code, rec.Body)
	}
	return decode[gen.Onboarding](t, rec).Step
}

func (f *onboardingFixture) send(method, target, body string, cookies ...*http.Cookie) int {
	return f.env.serve(authRequest{method: method, target: target, body: body, cookies: cookies}).Code
}

// 段階は、メディアフォルダが無ければ folders、あれば done が既定である。
func TestOnboardingStepDefaultsToTheMediaFolders(t *testing.T) {
	f := newOnboardingFixture(t)
	if got := f.step(t); got != gen.OnboardingStepFolders {
		t.Fatalf("step without a media folder = %q, want folders", got)
	}
	if _, err := f.env.db.Settings().AddMediaFolder(context.Background(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := f.step(t); got != gen.OnboardingStepDone {
		t.Fatalf("step with a media folder = %q, want done", got)
	}
}

// 2 つの端末の PUT は、後の段階が残る（Edge Cases、research.md R-3）。
func TestOnboardingKeepsTheLaterOfTwoWrites(t *testing.T) {
	f := newOnboardingFixture(t)
	for _, body := range []string{`{"step":"interests"}`, `{"step":"scan"}`} {
		if status := f.send(http.MethodPut, "/api/onboarding", body, f.owner); status != http.StatusOK {
			t.Fatalf("PUT %s status = %d", body, status)
		}
	}
	if got := f.step(t); got != gen.OnboardingStepScan {
		t.Fatalf("step = %q, want the later write, scan", got)
	}
}

// 知らない段階や分野の id は 400 invalid_request で、何も書かない。
func TestOnboardingRejectsUnknownStepsAndInterests(t *testing.T) {
	f := newOnboardingFixture(t)
	for _, body := range []string{`{"step":"later"}`, `{}`} {
		rec := f.env.serve(authRequest{method: http.MethodPut, target: "/api/onboarding", body: body, cookies: []*http.Cookie{f.owner}})
		assertErrorCode(t, rec, http.StatusBadRequest, codeInvalidRequest)
	}
	for _, body := range []string{`{"ids":["opera"]}`, `{"ids":[]}`, `{"ids":["anime","anime"]}`, `{}`} {
		rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/onboarding/interests", body: body, cookies: []*http.Cookie{f.owner}})
		assertErrorCode(t, rec, http.StatusBadRequest, codeInvalidRequest)
	}
	if got := f.step(t); got != gen.OnboardingStepFolders {
		t.Fatalf("step = %q, want folders unchanged", got)
	}
	shelves, err := f.env.db.Home().Shelves(context.Background())
	if err != nil || len(shelves) != 0 {
		t.Fatalf("shelves = %+v, %v; want none", shelves, err)
	}
}

// 分野を選ぶと足した棚を返し、一覧の onHome が真になる。2 度目は棚を足さない。
func TestOnboardingInterestsBecomeShelves(t *testing.T) {
	f := newOnboardingFixture(t)
	rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/onboarding/interests",
		body: `{"ids":["anime","music"]}`, cookies: []*http.Cookie{f.owner}})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d: %s", rec.Code, rec.Body)
	}
	result := decode[gen.InterestPickResult](t, rec)
	if len(result.Shelves) != 2 || result.Queued != 0 {
		t.Fatalf("result = %+v, want 2 shelves and nothing queued", result)
	}
	anime := result.Shelves[0]
	if anime.Name != "Anime" || anime.Kind != gen.HomeShelfKindFolder || anime.FolderId == nil ||
		anime.Key != "folder:"+strconv.FormatInt(*anime.FolderId, 10) {
		t.Fatalf("shelf = %+v", anime)
	}

	list := decode[gen.InterestList](t, f.env.get("/api/onboarding/interests", f.owner))
	if len(list.Interests) != 24 || list.Interests[0].Id != "anime" {
		t.Fatalf("interests = %+v", list.Interests)
	}
	for _, interest := range list.Interests {
		if want := interest.Id == "anime" || interest.Id == "music"; interest.OnHome != want {
			t.Errorf("%s onHome = %t, want %t", interest.Id, interest.OnHome, want)
		}
	}

	rec = f.env.serve(authRequest{method: http.MethodPost, target: "/api/onboarding/interests",
		body: `{"ids":["anime"]}`, cookies: []*http.Cookie{f.owner}})
	if again := decode[gen.InterestPickResult](t, rec); rec.Code != http.StatusOK || len(again.Shelves) != 0 {
		t.Fatalf("second pick = %d %+v, want no shelf", rec.Code, again)
	}
}

// 経路は所有者だけで、セッションの無い閲覧者は通らない。別オリジンからの書き込みは 403 である。
func TestOnboardingIsForTheOwnerOnly(t *testing.T) {
	f := newOnboardingFixture(t)
	routes := []authRequest{
		{method: http.MethodGet, target: "/api/onboarding"},
		{method: http.MethodPut, target: "/api/onboarding", body: `{"step":"done"}`},
		{method: http.MethodGet, target: "/api/onboarding/interests"},
		{method: http.MethodPost, target: "/api/onboarding/interests", body: `{"ids":["anime"]}`},
	}
	for _, route := range routes {
		assertErrorCode(t, f.env.serve(route), http.StatusUnauthorized, gen.ErrorCodeUnauthenticated)
	}
	for _, route := range routes[1:2] {
		route.cookies = []*http.Cookie{f.owner}
		route.header = map[string]string{"Origin": "https://attacker.example"}
		assertErrorCode(t, f.env.serve(route), http.StatusForbidden, gen.ErrorCodeForbidden)
	}
	if got := f.step(t); got != gen.OnboardingStepFolders {
		t.Fatalf("step = %q, want folders unchanged", got)
	}
}
