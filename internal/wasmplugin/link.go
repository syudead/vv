package wasmplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html/charset"

	"github.com/syudead/vv/internal/domain"
)

// The bundled vv.link plugin reads a video link's title, description, date and image: first from the site's official
// oEmbed endpoint, which answers when the site refuses to serve its pages to programs, then from the page's JSON-LD,
// Open Graph and Twitter card metadata (docs/design-docs/link-fetch.md).

// oembedProvider is a site whose official oEmbed endpoint takes the link as its url parameter.
type oembedProvider struct {
	domains  []string
	endpoint string
}

var oembedProviders = []oembedProvider{
	{[]string{"youtube.com", "youtu.be", "youtube-nocookie.com"}, "https://www.youtube.com/oembed?format=json&url="},
	{[]string{"vimeo.com"}, "https://vimeo.com/api/oembed.json?url="},
	{[]string{"dailymotion.com", "dai.ly"}, "https://www.dailymotion.com/services/oembed?format=json&url="},
	{[]string{"tiktok.com"}, "https://www.tiktok.com/oembed?url="},
	{[]string{"streamable.com"}, "https://api.streamable.com/oembed.json?url="},
	{[]string{"nicovideo.jp", "nico.ms"}, "https://embed.nicovideo.jp/oembed?format=json&url="},
}

const (
	jsonAccept = "application/json"
	htmlAccept = "text/html,application/xhtml+xml"
)

// linkInfo is what the plugin found; the first source to give a value keeps it.
type linkInfo struct {
	title, description, date string
	images                   []string
}

func (l *linkInfo) fill(other linkInfo) {
	l.title = firstNonEmpty(l.title, other.title)
	l.description = firstNonEmpty(l.description, other.description)
	l.date = firstNonEmpty(l.date, other.date)
	for _, image := range other.images {
		if image != "" && !slices.Contains(l.images, image) {
			l.images = append(l.images, image)
		}
	}
}

func (l linkInfo) empty() bool {
	return l.title == "" && l.description == "" && len(l.images) == 0
}

// runLinkPlugin answers a fetch by link with the plugin response JSON, as a module would write it.
func runLinkPlugin(ctx context.Context, s *httpSession, req domain.WorkInfoRunRequest) []byte {
	if req.Operation != domain.WorkInfoFetch || req.URL == "" {
		return linkError("vv.link fetches by link only.")
	}
	var info linkInfo
	triedOEmbed := false
	if endpoint := oembedEndpoint(req.URL); endpoint != "" {
		triedOEmbed = true
		info.fill(readOEmbed(ctx, s, endpoint))
	}
	page, status := readPage(ctx, s, req.URL)
	info.fill(page.info)
	if !triedOEmbed && page.oembed != "" && (info.title == "" || len(info.images) == 0) {
		info.fill(readOEmbed(ctx, s, page.oembed))
	}
	if info.empty() {
		if status != 0 && (status < 200 || status > 299) {
			return linkError(fmt.Sprintf("The site answered %d %s for the link.", status, http.StatusText(status)))
		}
		return linkError("The link has no title, description or image to read.")
	}
	return linkResponse(info)
}

func oembedEndpoint(link string) string {
	for _, p := range oembedProviders {
		if domain.LinkMatchesDomains(link, p.domains) {
			return p.endpoint + url.QueryEscape(link)
		}
	}
	return ""
}

// readOEmbed reads an oEmbed JSON answer; any failure gives nothing, so the page can still answer.
func readOEmbed(ctx context.Context, s *httpSession, endpoint string) linkInfo {
	resp := s.get(ctx, endpoint, jsonAccept)
	if resp == nil || resp.status != http.StatusOK {
		return linkInfo{}
	}
	var raw struct {
		Title        string `json:"title"`
		Description  string `json:"description"`
		UploadDate   string `json:"upload_date"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if json.Unmarshal(resp.body, &raw) != nil {
		return linkInfo{}
	}
	info := linkInfo{title: cleanText(raw.Title), description: cleanText(raw.Description), date: workDate(raw.UploadDate)}
	info.images = append(info.images, betterThumbnails(raw.ThumbnailURL)...)
	return info
}

var youtubeThumbnail = regexp.MustCompile(`^(https://i\.ytimg\.com/vi/[^/]+/)hqdefault\.jpg$`)

// betterThumbnails puts YouTube's full-size thumbnail before the small one its oEmbed names; a missing full-size image
// is dropped like any cover that cannot be fetched.
func betterThumbnails(thumbnail string) []string {
	if m := youtubeThumbnail.FindStringSubmatch(thumbnail); m != nil {
		return []string{m[1] + "maxresdefault.jpg", thumbnail}
	}
	return []string{thumbnail}
}

type pageInfo struct {
	info linkInfo
	// oembed is the page's own oEmbed discovery link.
	oembed string
}

// readPage reads the page's metadata and returns the response status, or zero when no response arrived.
func readPage(ctx context.Context, s *httpSession, link string) (pageInfo, int) {
	resp := s.get(ctx, link, htmlAccept)
	if resp == nil {
		return pageInfo{}, 0
	}
	if resp.status != http.StatusOK || !strings.Contains(resp.header.Get("Content-Type"), "html") {
		return pageInfo{}, resp.status
	}
	reader, err := charset.NewReader(bytes.NewReader(resp.body), resp.header.Get("Content-Type"))
	if err != nil {
		reader = bytes.NewReader(resp.body)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return pageInfo{}, resp.status
	}
	return parsePage(body, resp.url), resp.status
}

func linkResponse(info linkInfo) []byte {
	work := map[string]any{}
	if info.title != "" {
		work["title"] = domain.TruncateRunes(info.title, domain.DisplayNameMaxLength)
	}
	if info.description != "" {
		work["description"] = domain.TruncateRunes(info.description, domain.WorkInfoLimits.DescriptionMaxLength)
	}
	if info.date != "" {
		work["dates"] = []map[string]string{{"kind": string(domain.DatePublished), "date": info.date}}
	}
	if len(info.images) > 0 {
		work["covers"] = info.images
	}
	out, _ := json.Marshal(map[string]any{"workInfo": work})
	return out
}

func linkError(message string) []byte {
	out, _ := json.Marshal(map[string]any{"error": map[string]string{"message": message}})
	return out
}

var spaces = regexp.MustCompile(`\s+`)

func cleanText(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

var leadingDate = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})`)

// workDate keeps the calendar date at the start of an ISO 8601 or oEmbed date-time.
func workDate(s string) string {
	if m := leadingDate.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		return m[1]
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
