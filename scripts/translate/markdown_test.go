package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func keep(target string) string { return target }

// fake marks each translated segment, keeping its tags where they are.
func fake(s *segment) (string, bool) {
	if !s.needsTranslation() {
		return "", false
	}
	out, err := s.restore("JA[" + s.text + "]")
	return out, err == nil
}

func TestRenderTranslatesProseAndKeepsStructure(t *testing.T) {
	src := strings.Join([]string{
		"# Running vv",
		"",
		"Start the `task up` command and open",
		"[the settings](settings.md#media-folders).",
		"",
		"```bash",
		"# not a heading",
		"task up",
		"```",
		"",
		"| Name | Meaning |",
		"| --- | --- |",
		"| `id` | The video ID |",
		"",
		"- First item",
		"  continues here",
		"1. Step one",
		"",
		"> [!NOTE]",
		"> Read this.",
		"",
		"<!-- comment",
		"stays -->",
		"## Running vv",
	}, "\n")
	got := render(parse(src, keep), fake)
	want := strings.Join([]string{
		"# JA[Running vv] {#running-vv}",
		"",
		"JA[Start the `task up` command and open [the settings](settings.md#media-folders).]",
		"",
		"```bash",
		"# not a heading",
		"task up",
		"```",
		"",
		"| JA[Name] | JA[Meaning] |",
		"| --- | --- |",
		"| `id` | JA[The video ID] |",
		"",
		"- JA[First item continues here]",
		"1. JA[Step one]",
		"",
		"> [!NOTE]",
		"> JA[Read this.]",
		"",
		"<!-- comment",
		"stays -->",
		"## JA[Running vv] {#running-vv-1}",
	}, "\n")
	if got != want {
		t.Errorf("render mismatch\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func TestFrontMatterTranslatesValuesAndPointsLinksAtTheLocale(t *testing.T) {
	src := "---\nlayout: home\nhero:\n  text: Design docs\n  actions:\n    - text: How-to\n      link: /docs/how-to/\n---\n"
	got := render(parse(src, keep), fake)
	for _, want := range []string{"layout: home", "  text: JA[Design docs]", "    - text: JA[How-to]", "      link: /ja/docs/how-to/"} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("front matter lacks %q:\n%s", want, got)
		}
	}
}

func TestRestoreRejectsBrokenTags(t *testing.T) {
	s := protect("Use `x` and [docs](a.md).", keep)
	if s.text != "Use <x0/> and <a1>docs</a1>." {
		t.Fatalf("protected text = %q", s.text)
	}
	for _, bad := range []string{
		"<x0/> を使う。",              // link dropped
		"<x0/> <x0/> <a1>文書</a1>", // duplicated
		"<x0/> </a1>文書<a1>",       // closed before opened
		"<x0/> <a1>文書</a1> <x9/>", // invented
	} {
		if _, err := s.restore(bad); err == nil {
			t.Errorf("restore(%q) accepted a broken translation", bad)
		}
	}
	got, err := s.restore("<x0/> と <a1>文書</a1> を使う。")
	if err != nil || got != "`x` と [文書](a.md) を使う。" {
		t.Errorf("restore = %q, %v", got, err)
	}
}

func TestSegmentsWithoutWordsAreNotSent(t *testing.T) {
	for _, text := range []string{"`200`", "42", "<https://example.com>"} {
		if protect(text, keep).needsTranslation() {
			t.Errorf("%q would be sent for translation", text)
		}
	}
}

func TestLinkRewriterReanchorsLinksOutsideTheTranslatedTree(t *testing.T) {
	translated := func(p string) bool { return strings.HasPrefix(p, "docs/") && strings.HasSuffix(p, ".md") }
	rw := linkRewriter("docs/how-to/running-vv.md", "docs-site/ja/docs/how-to/running-vv.md", translated)
	cases := map[string]string{
		"hosting-vv.md#setup":         "hosting-vv.md#setup",
		"../screenshots/a.png":        "../../../../docs/screenshots/a.png",
		"../../ARCHITECTURE.md#intro": "../../../../ARCHITECTURE.md#intro",
		"https://example.com":         "https://example.com",
		"#local":                      "#local",
	}
	for in, want := range cases {
		if got := rw(in); got != want {
			t.Errorf("rewrite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChatTranslatorSendsGlossaryTermsInUse(t *testing.T) {
	var system string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		system = req.Messages[0].Content
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":" translated \n"}}]}`))
	}))
	defer srv.Close()

	tr, err := newTranslator(config{BaseURL: srv.URL + "/v1", Model: "m", APIKey: "k",
		Glossary: []term{{en: "folder group", ja: "FG"}, {en: "tag", ja: "T"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.Translate(context.Background(), "Open the folder group.")
	if err != nil || got != "translated" {
		t.Fatalf("Translate = %q, %v", got, err)
	}
	if !strings.Contains(system, "folder group => FG") || strings.Contains(system, "tag => T") {
		t.Errorf("system prompt glossary is wrong:\n%s", system)
	}
}

func TestPlamoTranslatorUsesTheTranslationPrompt(t *testing.T) {
	var prompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Prompt string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		prompt = req.Prompt
		_, _ = w.Write([]byte(`{"choices":[{"text":"out"}]}`))
	}))
	defer srv.Close()

	tr, err := newTranslator(config{Provider: "plamo", BaseURL: srv.URL, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Translate(context.Background(), "Hello"); err != nil {
		t.Fatal(err)
	}
	want := "<|plamo:op|>dataset\ntranslation\n<|plamo:op|>input lang=English\nHello\n<|plamo:op|>output lang=Japanese\n"
	if prompt != want {
		t.Errorf("prompt = %q", prompt)
	}
}

func TestParagraphStartingWithAutolinkIsTranslated(t *testing.T) {
	src := "<https://example.com> opens the page.\n\n<details>\n"
	got := render(parse(src, keep), fake)
	want := "JA[<https://example.com> opens the page.]\n\n<details>\n"
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}
