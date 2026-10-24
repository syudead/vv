package wasmplugin

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// pageMeta collects the head's metadata by source; parsePage picks among them in the order JSON-LD, Open Graph,
// Twitter card, then plain HTML.
type pageMeta struct {
	jsonLD, openGraph, twitter, plain linkInfo
	oembed                            string
}

// parsePage reads the metadata of an HTML page served at base, resolving relative image and oEmbed links.
func parsePage(body []byte, base *url.URL) pageInfo {
	var meta pageMeta
	z := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return meta.result(base)
		case html.StartTagToken, html.SelfClosingTagToken:
			meta.readTag(z)
		case html.EndTagToken:
			if name, _ := z.TagName(); atom.Lookup(name) == atom.Body {
				return meta.result(base)
			}
		}
	}
}

func (m *pageMeta) readTag(z *html.Tokenizer) {
	name, hasAttr := z.TagName()
	attrs := map[string]string{}
	for hasAttr {
		var key, value []byte
		key, value, hasAttr = z.TagAttr()
		attrs[strings.ToLower(string(key))] = string(value)
	}
	switch atom.Lookup(name) {
	case atom.Meta:
		m.readMeta(attrs)
	case atom.Link:
		if strings.EqualFold(attrs["type"], "application/json+oembed") && strings.Contains(attrs["rel"], "alternate") {
			m.oembed = firstNonEmpty(m.oembed, attrs["href"])
		}
	case atom.Title:
		if z.Next() == html.TextToken {
			m.plain.title = firstNonEmpty(m.plain.title, cleanText(string(z.Text())))
		}
	case atom.Script:
		if strings.EqualFold(attrs["type"], "application/ld+json") && z.Next() == html.TextToken {
			m.jsonLD.fill(readJSONLD(z.Text()))
		}
	}
}

func (m *pageMeta) readMeta(attrs map[string]string) {
	key := strings.ToLower(firstNonEmpty(attrs["property"], firstNonEmpty(attrs["name"], attrs["itemprop"])))
	value := cleanText(attrs["content"])
	if value == "" {
		return
	}
	switch key {
	case "og:title":
		m.openGraph.fill(linkInfo{title: value})
	case "og:description":
		m.openGraph.fill(linkInfo{description: value})
	case "og:image", "og:image:secure_url", "og:image:url":
		m.openGraph.fill(linkInfo{images: []string{value}})
	case "og:video:release_date", "video:release_date", "article:published_time", "uploaddate", "datepublished":
		m.openGraph.fill(linkInfo{date: workDate(value)})
	case "twitter:title":
		m.twitter.fill(linkInfo{title: value})
	case "twitter:description":
		m.twitter.fill(linkInfo{description: value})
	case "twitter:image", "twitter:image:src":
		m.twitter.fill(linkInfo{images: []string{value}})
	case "description":
		m.plain.fill(linkInfo{description: value})
	}
}

func (m *pageMeta) result(base *url.URL) pageInfo {
	var info linkInfo
	for _, source := range []linkInfo{m.jsonLD, m.openGraph, m.twitter, m.plain} {
		info.fill(source)
	}
	images := info.images[:0]
	for _, image := range info.images {
		if resolved := resolveLink(base, image); resolved != "" {
			images = append(images, resolved)
		}
	}
	info.images = images
	return pageInfo{info: info, oembed: resolveLink(base, m.oembed)}
}

// resolveLink keeps an absolute http or https link, resolved against base; anything else is dropped.
func resolveLink(base *url.URL, ref string) string {
	if ref == "" {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ""
	}
	if base != nil {
		parsed = base.ResolveReference(parsed)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

// readJSONLD reads the first VideoObject in a JSON-LD script, whether alone, in a list or in an @graph.
func readJSONLD(text []byte) linkInfo {
	var raw any
	if json.Unmarshal(text, &raw) != nil {
		return linkInfo{}
	}
	if video := findVideoObject(raw); video != nil {
		return linkInfo{
			title:       cleanText(jsonString(video["name"])),
			description: cleanText(jsonString(video["description"])),
			date:        workDate(firstNonEmpty(jsonString(video["uploadDate"]), jsonString(video["datePublished"]))),
			images:      jsonImages(video["thumbnailUrl"]),
		}
	}
	return linkInfo{}
}

func findVideoObject(raw any) map[string]any {
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if found := findVideoObject(item); found != nil {
				return found
			}
		}
	case map[string]any:
		if isVideoType(v["@type"]) {
			return v
		}
		return findVideoObject(v["@graph"])
	}
	return nil
}

func isVideoType(t any) bool {
	switch v := t.(type) {
	case string:
		return v == "VideoObject"
	case []any:
		for _, item := range v {
			if item == "VideoObject" {
				return true
			}
		}
	}
	return false
}

func jsonString(v any) string {
	s, _ := v.(string)
	return s
}

// jsonImages reads a thumbnailUrl given as a string, a list of strings or ImageObjects.
func jsonImages(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case map[string]any:
		return []string{jsonString(t["url"])}
	case []any:
		var out []string
		for _, item := range t {
			out = append(out, jsonImages(item)...)
		}
		return out
	}
	return nil
}
