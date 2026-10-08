package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// autoImportWatcher は監視の代わりである。
type autoImportWatcher struct{}

func (autoImportWatcher) Arm([]string) error { return nil }
func (autoImportWatcher) Disarm()            {}

// autoImportScans は監視の走査の代わりで、始めようとした回数を数える。
type autoImportScans struct {
	mu     sync.Mutex
	starts int
}

func (s *autoImportScans) StartWatchScan(context.Context, []domain.DirtyDirectory) (domain.Scan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts++
	return domain.Scan{}, false, nil
}

func (s *autoImportScans) SupersedeWatchScan(context.Context) bool { return false }

func (s *autoImportScans) started() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts
}

type autoImportFixture struct {
	env   *authEnv
	scans *autoImportScans
	owner *http.Cookie
}

// newAutoImportFixture は本物の設定の保存先と app.AutoImport で経路を組む。監視と走査だけが偽である。
func newAutoImportFixture(t *testing.T) *autoImportFixture {
	t.Helper()
	f := &autoImportFixture{scans: &autoImportScans{}}
	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		auto := app.NewAutoImport(app.AutoImportOptions{
			Store: db.Settings(), Folders: db.Settings(), Watcher: autoImportWatcher{}, Scans: f.scans, Logger: discardLogger(),
		})
		if err := auto.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(auto.Stop)
		return Options{AutoImport: auto}
	})
	f.owner = f.env.setup()
	return f
}

func (f *autoImportFixture) put(body string, cookies ...*http.Cookie) *http.Response {
	rec := f.env.serve(authRequest{method: http.MethodPut, target: "/api/settings/auto-import", body: body, cookies: cookies})
	return rec.Result()
}

func TestGetAutoImportSettingsOnNewDatabaseIsEnabled(t *testing.T) {
	f := newAutoImportFixture(t)

	rec := f.env.get("/api/settings/auto-import", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	got := decode[gen.AutoImportSettings](t, rec)
	if !got.Enabled {
		t.Errorf("enabled = false on a new database")
	}
	// メディアフォルダが無いので監視するものが無く、off のままである。
	if got.Watch.State != gen.FolderWatchState(domain.FolderWatchOff) || got.Watch.Problem != nil || got.Watch.Path != nil {
		t.Errorf("watch = %+v", got.Watch)
	}
}

func TestPutAutoImportSettingsStoresChoiceWithoutScanning(t *testing.T) {
	f := newAutoImportFixture(t)
	ctx := context.Background()

	res := f.put(`{"enabled":false}`, f.owner)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if stored, err := f.env.db.Settings().AutoImport(ctx); err != nil || stored {
		t.Errorf("stored = %v, %v; want false", stored, err)
	}
	rec := f.env.get("/api/settings/auto-import", f.owner)
	if got := decode[gen.AutoImportSettings](t, rec); got.Enabled || got.Watch.State != gen.FolderWatchState(domain.FolderWatchOff) {
		t.Errorf("after off = %+v", got)
	}

	if res := f.put(`{"enabled":true}`, f.owner); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if stored, err := f.env.db.Settings().AutoImport(ctx); err != nil || !stored {
		t.Errorf("stored = %v, %v; want true", stored, err)
	}
	if n := f.scans.started(); n != 0 {
		t.Errorf("a scan was started %d times", n)
	}
}

func TestPutAutoImportSettingsRejectsInvalidBody(t *testing.T) {
	f := newAutoImportFixture(t)

	for _, body := range []string{`{}`, `{"enabled":"yes"}`, `{"enabled":null}`, `{"enabled":true,"x":1}`, `nope`} {
		res := f.put(body, f.owner)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, res.StatusCode)
		}
	}
	if stored, _ := f.env.db.Settings().AutoImport(context.Background()); !stored {
		t.Errorf("an invalid body changed the stored choice")
	}
}

// ゲスト（Cookie なし）には、オーナー専用の応答（未認証）が返り、何も変わらない。
func TestAutoImportSettingsAreOwnerOnly(t *testing.T) {
	f := newAutoImportFixture(t)

	assertUnauthenticated(t, "GET", f.env.get("/api/settings/auto-import"))
	rec := f.env.serve(authRequest{method: http.MethodPut, target: "/api/settings/auto-import", body: `{"enabled":false}`})
	assertUnauthenticated(t, "PUT", rec)
	if stored, _ := f.env.db.Settings().AutoImport(context.Background()); !stored {
		t.Errorf("a guest changed the stored choice")
	}
}

func TestScanEventCarriesOrigin(t *testing.T) {
	for _, origin := range []domain.ScanOrigin{domain.ScanOriginManual, domain.ScanOriginWatch} {
		got := toAPIScan(domain.Scan{ID: 1, State: domain.ScanRunning, Origin: origin})
		if string(got.Origin) != string(origin) {
			t.Errorf("origin = %q, want %q", got.Origin, origin)
		}
	}

	events := NewEvents()
	scans := &lockedScans{current: domain.Scan{ID: 4, State: domain.ScanRunning, Origin: domain.ScanOriginWatch}}
	handler := newTestServer(t, Options{Scans: scans, Events: events})
	stream, _ := openEvents(t, handler)
	event := nextEvent(t, stream)
	if event.name != "scan" {
		t.Fatalf("event = %q, want scan", event.name)
	}
	if !strings.Contains(event.data, `"origin":"watch"`) {
		t.Errorf("scan event = %s", event.data)
	}
}
