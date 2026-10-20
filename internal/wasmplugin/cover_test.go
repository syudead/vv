package wasmplugin

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// runCovers has the probe return the cover URLs. The probe's people mode writes its setting raw after "people":, so
// the setting closes an empty people list and adds the covers member.
func runCovers(t *testing.T, rt *Runtime, covers ...string) (domain.WorkInfoRunResult, error) {
	t.Helper()
	encoded, err := json.Marshal(covers)
	if err != nil {
		t.Fatal(err)
	}
	req := fetchMode("people")
	req.Settings = map[string]string{"people": `[], "covers": ` + string(encoded)}
	got, _, err := rt.Run(context.Background(), probeDigest, probeModule(t), req, domain.WorkInfoRunNetwork{}, nil)
	return got, err
}

func TestCoversAreFetchedThroughVVAfterTheRun(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	got, err := runCovers(t, rt, "http://site.test/missing", "http://site.test/page")
	wantURLs := []string{"http://site.test/missing", "http://site.test/page"}
	if err != nil || !slices.Equal(got.WorkInfo.CoverURLs, wantURLs) || len(got.CoverImages) != 2 ||
		got.CoverImages[0] != nil || string(got.CoverImages[1]) != "hello" {
		t.Fatalf("Run(covers) = %q (urls %q), %v; want no first cover and the page body second",
			got.CoverImages, got.WorkInfo.CoverURLs, err)
	}
	site.mu.Lock()
	defer site.mu.Unlock()
	if len(site.userAgent) != 1 || site.userAgent[0] != defaultUserAgent {
		t.Fatalf("site saw User-Agent %q, want the default one", site.userAgent)
	}
}

func TestCoversThatCannotBeFetchedAreNil(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	failing := []string{"http://site.test/missing", "http://site.test/big", "http://site.test/broken", "http://missing.test/x"}
	got, err := runCovers(t, rt, append(failing, "ftp://site.test/x")...)
	if err != nil || !slices.Equal(got.WorkInfo.CoverURLs, failing) || len(got.CoverImages) != len(failing) ||
		slices.ContainsFunc(got.CoverImages, func(image []byte) bool { return image != nil }) {
		t.Fatalf("Run(covers) = %q (urls %q), %v; want a nil image for each kept URL", got.CoverImages,
			got.WorkInfo.CoverURLs, err)
	}
}

func TestCoverOnARefusedAddressFailsTheRun(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	for _, cover := range []string{"http://lan.test/cover.png", "http://site.test/to-lan"} {
		_, err := runCovers(t, rt, cover)
		wantFailure(t, err, domain.WorkInfoRunBlockedAddress)
		if runErrorOf(err).Host != "lan.test" && runErrorOf(err).Host != "192.168.1.20" {
			t.Fatalf("host = %q, want the refused host", runErrorOf(err).Host)
		}
	}
}

func TestCoverImageSendsNothingForAURLItCannotRequest(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	session := rt.run.net.session(domain.WorkInfoRunNetwork{})
	for _, raw := range []string{"", "http://[::1", "http://site.test/%zz"} {
		if got := session.image(context.Background(), raw); got != nil {
			t.Fatalf("image(%q) = %q, want nil", raw, got)
		}
	}
	site.mu.Lock()
	defer site.mu.Unlock()
	if len(site.hosts) != 0 {
		t.Fatalf("site saw %q, want no request", site.hosts)
	}
}
