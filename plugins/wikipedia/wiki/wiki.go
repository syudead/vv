// Package wiki は Wikipedia 作品情報プラグインの中身である。Wikipedia の MediaWiki API で記事を
// 探し（search）、記事の題名とカテゴリをタグとして返す（fetch）。HTTP はプラグインの host 関数を
// 包んだ Getter で受け取るので、wasip1 でなくてもテストできる。
package wiki

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// DefaultLang は設定 lang が無いときに引く Wikipedia の言語である。
const DefaultLang = "ja"

// maxDetail は候補の detail（記事の抜粋）に残す最大の文字数である。
const maxDetail = 120

// Getter は url へ GET を送り、200 の応答の本文を返す。読めなければ ok が false である。
type Getter func(url string) (body []byte, ok bool)

// Candidate は search の候補 1 件である。
type Candidate struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// WorkInfo は fetch の結果である。
type WorkInfo struct {
	Title string   `json:"title,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

var (
	langPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,15}$`)
	hostPattern = regexp.MustCompile(`^([a-z][a-z0-9-]{1,15})(?:\.m)?\.wikipedia\.org$`)
	markup      = regexp.MustCompile(`<[^>]*>`)
)

// Lang は設定 lang を確かめて返す。空なら DefaultLang である。
func Lang(setting string) (string, error) {
	lang := strings.TrimSpace(strings.ToLower(setting))
	if lang == "" {
		return DefaultLang, nil
	}
	if !langPattern.MatchString(lang) {
		return "", fmt.Errorf("the language setting %q is not a Wikipedia language code such as ja or en", setting)
	}
	return lang, nil
}

// Search は lang の Wikipedia で query を全文検索し、記事の候補を返す。
func Search(get Getter, lang, query string) ([]Candidate, error) {
	params := url.Values{
		"action": {"query"}, "list": {"search"}, "srsearch": {query}, "srlimit": {"20"},
		"format": {"json"}, "formatversion": {"2"},
	}
	var res struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := callAPI(get, lang, params, &res); err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(res.Query.Search))
	for _, hit := range res.Query.Search {
		candidates = append(candidates, Candidate{
			URL: ArticleURL(lang, hit.Title), Title: hit.Title, Detail: plainText(hit.Snippet),
		})
	}
	return candidates, nil
}

// Fetch は Wikipedia の記事の URL から、記事の題名と、隠しでないカテゴリの名前をタグとして返す。
// 転送ページは転送先の記事を読む。
func Fetch(get Getter, articleURL string) (WorkInfo, error) {
	lang, title, err := ParseArticleURL(articleURL)
	if err != nil {
		return WorkInfo{}, err
	}
	params := url.Values{
		"action": {"query"}, "prop": {"categories"}, "titles": {title}, "clshow": {"!hidden"},
		"cllimit": {"max"}, "redirects": {"1"}, "format": {"json"}, "formatversion": {"2"},
	}
	var res struct {
		Query struct {
			Pages []struct {
				Title      string `json:"title"`
				Missing    bool   `json:"missing"`
				Categories []struct {
					Title string `json:"title"`
				} `json:"categories"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := callAPI(get, lang, params, &res); err != nil {
		return WorkInfo{}, err
	}
	if len(res.Query.Pages) == 0 || res.Query.Pages[0].Missing {
		return WorkInfo{}, fmt.Errorf("no Wikipedia article is named %q", title)
	}
	page := res.Query.Pages[0]
	info := WorkInfo{Title: page.Title}
	for _, category := range page.Categories {
		info.Tags = append(info.Tags, categoryName(category.Title))
	}
	return info, nil
}

// ArticleURL は lang の Wikipedia にある記事 title の URL を返す。
func ArticleURL(lang, title string) string {
	return "https://" + lang + ".wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
}

// ParseArticleURL は記事の URL（モバイル版の xx.m.wikipedia.org も可）から言語と題名を取り出す。
func ParseArticleURL(raw string) (lang, title string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", errors.New("the URL cannot be read")
	}
	m := hostPattern.FindStringSubmatch(strings.ToLower(u.Hostname()))
	if m == nil || !strings.HasPrefix(u.Path, "/wiki/") {
		return "", "", errors.New("the URL is not a Wikipedia article URL (https://<lang>.wikipedia.org/wiki/<title>)")
	}
	title = strings.ReplaceAll(strings.TrimPrefix(u.Path, "/wiki/"), "_", " ")
	if title == "" {
		return "", "", errors.New("the URL names no Wikipedia article")
	}
	return m[1], title, nil
}

// callAPI は lang の MediaWiki API を params で呼び、JSON の応答を out に読む。
func callAPI(get Getter, lang string, params url.Values, out any) error {
	endpoint := "https://" + lang + ".wikipedia.org/w/api.php?" + params.Encode()
	body, ok := get(endpoint)
	if !ok {
		return fmt.Errorf("could not reach %s.wikipedia.org", lang)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s.wikipedia.org answered something other than the expected JSON", lang)
	}
	return nil
}

// categoryName は "Category:日本のアニメ作品" のようなカテゴリの題名から名前空間を外す。
func categoryName(title string) string {
	if _, name, ok := strings.Cut(title, ":"); ok {
		return name
	}
	return title
}

// plainText は検索の抜粋から HTML を外し、maxDetail 文字に切り詰める。
func plainText(snippet string) string {
	text := strings.Join(strings.Fields(html.UnescapeString(markup.ReplaceAllString(snippet, ""))), " ")
	if runes := []rune(text); len(runes) > maxDetail {
		return string(runes[:maxDetail]) + "…"
	}
	return text
}
