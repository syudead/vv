package wasmplugin

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// runCover は probe に cover の URL を返させる。
func runCover(t *testing.T, rt *Runtime, cover string) (domain.WorkInfoRunResult, error) {
	t.Helper()
	req := fetchMode("cover")
	req.Settings = map[string]string{"cover": cover}
	return rt.Run(context.Background(), probeDigest, probeModule(t), req)
}

func TestCoverIsFetchedThroughVVAfterTheRun(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	got, err := runCover(t, rt, "http://site.test/page")
	if err != nil || got.WorkInfo.CoverURL != "http://site.test/page" || string(got.CoverImage) != "hello" {
		t.Fatalf("Run(cover) = %q (url %q), %v; want the page body as the cover", got.CoverImage, got.WorkInfo.CoverURL, err)
	}
	site.mu.Lock()
	defer site.mu.Unlock()
	if len(site.userAgent) != 1 || site.userAgent[0] != defaultUserAgent {
		t.Fatalf("site saw User-Agent %q, want the default one", site.userAgent)
	}
}

// 取れないカバー画像は、実行を失敗させずに無いものとする。
func TestCoverThatCannotBeFetchedIsDropped(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	for _, cover := range []string{
		"http://site.test/missing", "http://site.test/big", "http://site.test/broken", "http://missing.test/x",
		"ftp://site.test/x",
	} {
		got, err := runCover(t, rt, cover)
		if err != nil || got.CoverImage != nil || got.WorkInfo.Title != "cover" {
			t.Fatalf("Run(cover %s) = %q, %v; want the work info without a cover", cover, got.CoverImage, err)
		}
	}
}

// LAN を指すカバー画像は、プラグインの要求と同じく実行を blocked_address にする。
func TestCoverOnARefusedAddressFailsTheRun(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	for _, cover := range []string{"http://lan.test/cover.png", "http://site.test/to-lan"} {
		_, err := runCover(t, rt, cover)
		wantFailure(t, err, domain.WorkInfoRunBlockedAddress)
		if runErrorOf(err).Host != "lan.test" && runErrorOf(err).Host != "192.168.1.20" {
			t.Fatalf("host = %q, want the refused host", runErrorOf(err).Host)
		}
	}
}

// プラグインの実行を経ない URL でも、要求を組み立てられなければ何も送らない。
func TestCoverImageSendsNothingForAURLItCannotRequest(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	session := &httpSession{net: rt.run.net}
	for _, raw := range []string{"", "http://[::1", "http://site.test/%zz"} {
		if got := session.coverImage(context.Background(), raw); got != nil {
			t.Fatalf("coverImage(%q) = %q, want nil", raw, got)
		}
	}
	site.mu.Lock()
	defer site.mu.Unlock()
	if len(site.hosts) != 0 {
		t.Fatalf("site saw %q, want no request", site.hosts)
	}
}
