package wasmplugin

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"testing"

	"golang.org/x/text/encoding/japanese"

	"github.com/syudead/vv/internal/domain"
)

// newLinkTestRuntime sends every request for a public test name to handler, which tells the sites apart by Host.
func newLinkTestRuntime(t *testing.T, handler http.HandlerFunc) *Runtime {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	rt := newTestRuntime(t)
	nc := rt.run.net
	nc.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.215.14")}, nil
	}
	nc.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if err := nc.control(network, address, nil); err != nil {
			return nil, err
		}
		var d net.Dialer
		return d.DialContext(ctx, network, server.Listener.Addr().String())
	}
	return rt
}

func runLink(t *testing.T, rt *Runtime, link string) (domain.WorkInfoRunResult, error) {
	t.Helper()
	req := domain.LinkPluginManifest.RunRequest(domain.WorkInfoFetch)
	req.URL = link
	got, _, err := rt.Run(context.Background(), domain.LinkPluginDigest, nil, req, domain.WorkInfoRunNetwork{}, nil)
	return got, err
}

func useOEmbedProvider(t *testing.T, provider oembedProvider) {
	t.Helper()
	previous := oembedProviders
	oembedProviders = []oembedProvider{provider}
	t.Cleanup(func() { oembedProviders = previous })
}

// A site that refuses its page still answers through its oEmbed endpoint, and VVMDM fetches the thumbnail it names.
func TestLinkPluginReadsOEmbedWhenThePageIsRefused(t *testing.T) {
	useOEmbedProvider(t, oembedProvider{[]string{"video.test"}, "http://oembed.test/oembed?url="})
	rt := newLinkTestRuntime(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "oembed.test" && r.URL.Query().Get("url") == "http://video.test/watch?v=1&utm_source=feed":
			_, _ = w.Write([]byte(`{"title":"  Night   drive ","thumbnail_url":"http://img.test/1.jpg"}`))
		case r.Host == "img.test":
			_, _ = w.Write([]byte("jpeg"))
		default:
			http.Error(w, "no robots", http.StatusForbidden)
		}
	})
	got, err := runLink(t, rt, "http://video.test/watch?v=1&utm_source=feed")
	if err != nil || got.WorkInfo.Title != "Night drive" ||
		!slices.Equal(got.WorkInfo.CoverURLs, []string{"http://img.test/1.jpg"}) || string(got.CoverImages[0]) != "jpeg" {
		t.Fatalf("Run = %+v, covers %q, %v; want the oEmbed title and its fetched thumbnail", got.WorkInfo, got.CoverImages, err)
	}
}

// A page's shared metadata gives the title, description, date and an image resolved against the page, in the page's
// own character set; a JSON-LD VideoObject wins over Open Graph.
func TestLinkPluginReadsThePagesSharedMetadata(t *testing.T) {
	useOEmbedProvider(t, oembedProvider{})
	page, err := japanese.ShiftJIS.NewEncoder().String(`<html><head>
		<meta charset="shift_jis"><title>Site title</title>
		<meta property="og:title" content="夜の散歩">
		<meta property="og:description" content="川沿いを歩く">
		<meta property="og:image" content="/images/cover.png">
		<meta property="article:published_time" content="2024-03-01T09:00:00+09:00">
		</head><body></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	ldPage := `<html><head><meta property="og:title" content="OG title">
		<script type="application/ld+json">{"@graph":[{"@type":"WebPage"},
		{"@type":"VideoObject","name":"LD title","uploadDate":"2023-05-06","thumbnailUrl":["https://img.test/ld.jpg"]}]}</script>
		</head></html>`
	rt := newLinkTestRuntime(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/sjis":
			_, _ = w.Write([]byte(page))
		case "/ld":
			_, _ = w.Write([]byte(ldPage))
		}
	})
	got, err := runLink(t, rt, "http://page.test/sjis")
	if err != nil || got.WorkInfo.Title != "夜の散歩" || got.WorkInfo.Description != "川沿いを歩く" ||
		!slices.Equal(got.WorkInfo.Dates, []domain.WorkDate{{Kind: domain.DatePublished, Date: "2024-03-01"}}) ||
		!slices.Equal(got.WorkInfo.CoverURLs, []string{"http://page.test/images/cover.png"}) {
		t.Fatalf("Shift_JIS page = %+v, %v", got.WorkInfo, err)
	}
	got, err = runLink(t, rt, "http://page.test/ld")
	if err != nil || got.WorkInfo.Title != "LD title" || got.WorkInfo.Dates[0].Date != "2023-05-06" ||
		got.WorkInfo.CoverURLs[0] != "https://img.test/ld.jpg" {
		t.Fatalf("JSON-LD page = %+v, %v; want the VideoObject's values", got.WorkInfo, err)
	}
}

// A page with nothing to read fails the run with the site's answer as the message.
func TestLinkPluginFailsWithTheSitesAnswer(t *testing.T) {
	useOEmbedProvider(t, oembedProvider{})
	rt := newLinkTestRuntime(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/empty" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body>login</body></html>`))
			return
		}
		http.NotFound(w, r)
	})
	for link, message := range map[string]string{
		"http://page.test/gone":  "The site answered 404 Not Found for the link.",
		"http://page.test/empty": "The link has no title, description or image to read.",
	} {
		_, err := runLink(t, rt, link)
		var runErr *domain.WorkInfoRunError
		if !errors.As(err, &runErr) || runErr.Failure != domain.WorkInfoRunFailed || runErr.PluginMessage != message {
			t.Errorf("Run(%s) = %v; want plugin_failed with %q", link, err, message)
		}
	}
}

func TestBetterThumbnailsPutsYouTubesFullSizeImageFirst(t *testing.T) {
	got := betterThumbnails("https://i.ytimg.com/vi/abc/hqdefault.jpg")
	if !slices.Equal(got, []string{"https://i.ytimg.com/vi/abc/maxresdefault.jpg", "https://i.ytimg.com/vi/abc/hqdefault.jpg"}) {
		t.Fatalf("betterThumbnails = %v", got)
	}
}
