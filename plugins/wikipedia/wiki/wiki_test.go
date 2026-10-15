package wiki

import (
	"net/url"
	"reflect"
	"testing"
)

// fakeGet は API のエンドポイントに決まった本文で答え、受け取った URL を記録する Getter を返す。
func fakeGet(t *testing.T, body string, ok bool) (Getter, *[]string) {
	t.Helper()
	var calls []string
	return func(u string) ([]byte, bool) {
		calls = append(calls, u)
		return []byte(body), ok
	}, &calls
}

func TestLang(t *testing.T) {
	for setting, want := range map[string]string{"": "ja", " EN ": "en", "zh-yue": "zh-yue"} {
		got, err := Lang(setting)
		if err != nil || got != want {
			t.Errorf("Lang(%q) = %q, %v; want %q", setting, got, err, want)
		}
	}
	for _, bad := range []string{"x", "evil.example/", "ja?x=1"} {
		if _, err := Lang(bad); err == nil {
			t.Errorf("Lang(%q) returned no error", bad)
		}
	}
}

func TestSearch(t *testing.T) {
	get, calls := fakeGet(t, `{"query":{"search":[
		{"title":"千と千尋の神隠し","snippet":"<span class=\"searchmatch\">千と千尋</span>の神隠し &amp; 宮崎駿"}
	]}}`, true)
	got, err := Search(get, "ja", "千と千尋")
	if err != nil {
		t.Fatal(err)
	}
	want := []Candidate{{
		URL:    "https://ja.wikipedia.org/wiki/%E5%8D%83%E3%81%A8%E5%8D%83%E5%B0%8B%E3%81%AE%E7%A5%9E%E9%9A%A0%E3%81%97",
		Title:  "千と千尋の神隠し",
		Detail: "千と千尋の神隠し & 宮崎駿",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Search = %#v; want %#v", got, want)
	}
	u, _ := url.Parse((*calls)[0])
	if u.Host != "ja.wikipedia.org" || u.Query().Get("srsearch") != "千と千尋" {
		t.Errorf("Search called %s", (*calls)[0])
	}
}

func TestSearchUnreachable(t *testing.T) {
	get, _ := fakeGet(t, "", false)
	if _, err := Search(get, "en", "x"); err == nil {
		t.Error("Search returned no error for an unreachable site")
	}
}

func TestFetch(t *testing.T) {
	get, calls := fakeGet(t, `{"query":{"pages":[{"title":"千と千尋の神隠し","categories":[
		{"title":"Category:2001年のアニメ映画"},{"title":"Category:スタジオジブリの長編映画"}
	]}]}}`, true)
	got, err := Fetch(get, "https://ja.m.wikipedia.org/wiki/%E5%8D%83%E3%81%A8%E5%8D%83%E5%B0%8B%E3%81%AE%E7%A5%9E%E9%9A%A0%E3%81%97")
	if err != nil {
		t.Fatal(err)
	}
	want := WorkInfo{Title: "千と千尋の神隠し", Tags: []string{"2001年のアニメ映画", "スタジオジブリの長編映画"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Fetch = %#v; want %#v", got, want)
	}
	u, _ := url.Parse((*calls)[0])
	if u.Host != "ja.wikipedia.org" || u.Query().Get("titles") != "千と千尋の神隠し" {
		t.Errorf("Fetch called %s", (*calls)[0])
	}
}

func TestFetchMissing(t *testing.T) {
	get, _ := fakeGet(t, `{"query":{"pages":[{"title":"Nope","missing":true}]}}`, true)
	if _, err := Fetch(get, "https://en.wikipedia.org/wiki/Nope"); err == nil {
		t.Error("Fetch returned no error for a missing article")
	}
}

func TestParseArticleURL(t *testing.T) {
	lang, title, err := ParseArticleURL("https://en.wikipedia.org/wiki/Spirited_Away")
	if err != nil || lang != "en" || title != "Spirited Away" {
		t.Errorf("ParseArticleURL = %q, %q, %v", lang, title, err)
	}
	for _, bad := range []string{
		"https://example.com/wiki/X", "https://en.wikipedia.org/w/index.php?title=X",
		"https://en.wikipedia.org/wiki/", "https://wikipedia.org.example.com/wiki/X",
	} {
		if _, _, err := ParseArticleURL(bad); err == nil {
			t.Errorf("ParseArticleURL(%q) returned no error", bad)
		}
	}
}
