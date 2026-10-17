package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 直近の実行の標準エラーの経路。実行の行は保存先に直接書く。

func TestWorkInfoPluginLastRunRoute(t *testing.T) {
	e := newPluginEnv(t)
	added := decodePlugin(t, "追加", e.uploadParts([]byte(pluginManifest), 1, e.owner), http.StatusCreated)
	if added.LastRunAt != nil {
		t.Fatalf("lastRunAt = %d before any run, want none", *added.LastRunAt)
	}

	rec := e.get(pluginPath(added.Id, "/last-run"), e.owner)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "{}" {
		t.Fatalf("no run: %d %s, want 200 {}", rec.Code, rec.Body)
	}

	ranAt := time.Unix(1_800_000_000, 0)
	if err := e.db.WorkInfoPlugins().SaveWorkInfoPluginRun(context.Background(), domain.WorkInfoPluginRun{
		PluginID: added.Id, RanAt: ranAt,
		WorkInfoRunLog: domain.WorkInfoRunLog{
			Operation: domain.WorkInfoFetch, Failure: domain.WorkInfoRunTimedOut, Stderr: "key=[secret]\n", Truncated: true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	rec = e.get(pluginPath(added.Id, "/last-run"), e.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("kept run: %d %s", rec.Code, rec.Body)
	}
	var body gen.WorkInfoPluginLastRun
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	failure := gen.WorkInfoPluginRunFailureTimedOut
	want := gen.WorkInfoPluginRun{
		RanAt: ranAt.Unix(), Operation: gen.Fetch, Failure: &failure, Stderr: "key=[secret]\n", Truncated: true,
	}
	if body.Run == nil || body.Run.RanAt != want.RanAt || body.Run.Operation != want.Operation ||
		body.Run.Failure == nil || *body.Run.Failure != failure || body.Run.Stderr != want.Stderr || !body.Run.Truncated {
		t.Fatalf("run = %s, want %+v", rec.Body, want)
	}

	list := e.get("/api/work-info-plugins", e.owner)
	if !strings.Contains(list.Body.String(), `"lastRunAt":1800000000`) {
		t.Fatalf("list does not carry lastRunAt: %s", list.Body)
	}

	rec = e.get(pluginPath(added.Id+100, "/last-run"), e.owner)
	assertErrorBody(t, "知らないプラグイン", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})

	assertUnauthenticated(t, "GET last-run", e.get(pluginPath(added.Id, "/last-run")))
}
