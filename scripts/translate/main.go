// translate generates the Japanese edition of the documentation site from the
// English documents under docs/ and specs/. It is the body of
// `task docs-translate`; docs/design-docs/translation-pipeline.md explains the
// design.
//
// Each document is split into segments (paragraphs, list items, table cells,
// headings). A segment already in the translation memory is reused, so a run
// only sends the prose that changed. With -offline no request is made and an
// untranslated segment keeps its English text.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// notice opens every translated page. It is written in English and translated
// like the rest of the page; {source} becomes the link to the original.
const notice = "> [!NOTE]\n> This page is a machine translation of the [English original]({source}). " +
	"The English version is authoritative.\n\n"

const home = "docs-site/index.md"

func main() {
	offline := flag.Bool("offline", false, "make no requests; untranslated segments keep their English text")
	out := flag.String("out", "docs-site/ja", "directory for the translated pages (the site's /ja/ locale)")
	tmPath := flag.String("memory", "docs-site/.translation/ja.json", "translation memory file")
	glossaryPath := flag.String("glossary", "docs-site/i18n/glossary.tsv", "fixed term translations (English<TAB>Japanese)")
	jobs := flag.Int("jobs", 4, "concurrent translation requests")
	maxNew := flag.Int("max-new", 0, "stop requesting after this many new segments (0: no limit)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, options{
		offline: *offline, out: *out, memory: *tmPath, glossary: *glossaryPath, jobs: *jobs, maxNew: *maxNew,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "translate:", err)
		os.Exit(1)
	}
}

type options struct {
	offline  bool
	out      string
	memory   string
	glossary string
	jobs     int
	maxNew   int
}

type document struct {
	src, out string
	pieces   []piece
}

func run(ctx context.Context, o options) error {
	sources, err := sourceFiles()
	if err != nil {
		return err
	}
	isSource := map[string]bool{}
	for _, s := range sources {
		isSource[s] = true
	}
	translated := func(p string) bool {
		if isSource[p] || isSource[path.Join(p, "README.md")] || isSource[path.Join(p, "index.md")] {
			return true
		}
		return false
	}

	var docs []document
	for _, src := range sources {
		body, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		out := path.Join(o.out, src)
		if src == home {
			out = path.Join(o.out, "index.md")
		}
		text := string(body)
		if src != home {
			rel, err := relPath(path.Dir(out), src)
			if err != nil {
				return err
			}
			text = insertNotice(text, strings.ReplaceAll(notice, "{source}", rel))
		}
		docs = append(docs, document{src: src, out: out, pieces: parse(text, linkRewriter(src, out, translated))})
	}

	memory, err := loadMemory(o.memory)
	if err != nil {
		return err
	}
	// The glossary is part of each memory key, so it is read even offline.
	glossary, err := loadGlossary(o.glossary)
	if err != nil {
		return err
	}
	pending := map[string]*segment{}
	for _, d := range docs {
		for _, p := range d.pieces {
			if p.seg != nil && p.seg.needsTranslation() {
				key := memoryKey(p.seg, glossary)
				if _, ok := memory[key]; !ok {
					pending[key] = p.seg
				}
			}
		}
	}

	var failed int
	if len(pending) > 0 && !o.offline {
		tr, err := newTranslator(config{
			Provider: os.Getenv("VV_TRANSLATE_PROVIDER"),
			BaseURL:  os.Getenv("VV_TRANSLATE_BASE_URL"),
			Model:    os.Getenv("VV_TRANSLATE_MODEL"),
			APIKey:   os.Getenv("VV_TRANSLATE_API_KEY"),
			Glossary: glossary,
		})
		if err != nil {
			return err
		}
		failed, err = translateAll(ctx, tr, pending, memory, o)
		if err != nil {
			// The service being down must not stop the site from building:
			// what is not translated yet stays English until the next run.
			fmt.Fprintf(os.Stderr, "translate: stopped requesting: %v\n", err)
		}
	}

	used := map[string]bool{}
	lookup := func(s *segment) (string, bool) {
		if !s.needsTranslation() {
			return "", false
		}
		key := memoryKey(s, glossary)
		ja, ok := memory[key]
		if !ok {
			return "", false
		}
		used[key] = true
		restored, err := s.restore(ja)
		return restored, err == nil
	}
	if err := os.RemoveAll(o.out); err != nil {
		return err
	}
	for _, d := range docs {
		if err := os.MkdirAll(filepath.Dir(d.out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(d.out, []byte(render(d.pieces, lookup)), 0o644); err != nil {
			return err
		}
	}

	missing := 0
	for text := range pending {
		if _, ok := memory[text]; !ok {
			missing++
		}
	}
	// Keep only the entries this run used, so the memory does not grow with
	// prose that no longer exists.
	for text := range memory {
		if !used[text] {
			delete(memory, text)
		}
	}
	if err := saveMemory(o.memory, memory); err != nil {
		return err
	}
	fmt.Printf("translate: %d documents, %d segments in memory, %d untranslated, %d rejected\n",
		len(docs), len(memory), missing, failed)
	return nil
}

// sourceFiles lists the published English documents: the tracked Markdown
// under docs/ and specs/ (minus the templates), and the site's home page.
func sourceFiles() ([]string, error) {
	out, err := exec.Command("git", "ls-files", "-z", "--", "docs/*.md", "specs/*.md", home).Output()
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" && !strings.HasPrefix(f, "docs/templates/") {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	return files, nil
}

// insertNotice puts the notice after the front matter and the H1.
func insertNotice(doc, notice string) string {
	lines := strings.SplitAfter(doc, "\n")
	for k, l := range lines {
		if strings.HasPrefix(l, "# ") {
			return strings.Join(lines[:k+1], "") + "\n" + notice + strings.TrimLeft(strings.Join(lines[k+1:], ""), "\n")
		}
	}
	return notice + doc
}

func translateAll(ctx context.Context, tr translator, pending map[string]*segment, memory map[string]string, o options) (int, error) {
	keys := make([]string, 0, len(pending))
	for k := range pending {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if o.maxNew > 0 && len(keys) > o.maxNew {
		keys = keys[:o.maxNew]
	}
	fmt.Printf("translate: requesting %d new segments\n", len(keys))

	var mu sync.Mutex
	var failed int
	var fatal error
	work := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < max(o.jobs, 1); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for key := range work {
				ja, err := translateOne(ctx, tr, pending[key])
				mu.Lock()
				switch {
				case err == nil:
					memory[key] = ja
				case errors.Is(err, errRejected):
					failed++
					fmt.Fprintf(os.Stderr, "translate: rejected %q: %v\n", short(pending[key].text), err)
				case fatal == nil:
					fatal = err
				}
				mu.Unlock()
			}
		}()
	}
	for _, k := range keys {
		mu.Lock()
		stop := fatal != nil
		mu.Unlock()
		if stop || ctx.Err() != nil {
			break
		}
		work <- k
	}
	close(work)
	wg.Wait()
	return failed, fatal
}

var errRejected = errors.New("translation rejected")

// memoryKey is the translation-memory key of a segment: its tagged English
// text, plus a fingerprint of the glossary entries the segment uses. Editing
// one of those entries changes the key, so the segment is translated again
// with the new term; segments that use no changed entry keep their key and
// are reused, offline included.
func memoryKey(s *segment, glossary []term) string {
	terms := matchingTerms(glossary, s.text)
	if len(terms) == 0 {
		return s.text
	}
	sum := sha256.Sum256([]byte(strings.Join(terms, "\n")))
	return s.text + "\x00glossary:" + hex.EncodeToString(sum[:8])
}

// translateOne asks twice; a translation that breaks the segment's tags is
// discarded rather than published.
func translateOne(ctx context.Context, tr translator, s *segment) (string, error) {
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		ja, err := tr.Translate(ctx, s.text)
		if err != nil {
			return "", err
		}
		if _, err := s.restore(ja); err != nil {
			last = err
			continue
		}
		return ja, nil
	}
	return "", fmt.Errorf("%w: %w", errRejected, last)
}

func short(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

func loadMemory(p string) (map[string]string, error) {
	m := map[string]string{}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	return m, nil
}

// saveMemory writes the memory with sorted keys (encoding/json sorts map
// keys), so an unchanged memory is byte-identical.
func saveMemory(p string, m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o644)
}

func loadGlossary(p string) ([]term, error) {
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var terms []term
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		en, ja, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("%s: %q has no tab", p, line)
		}
		terms = append(terms, term{en: strings.TrimSpace(en), ja: strings.TrimSpace(ja)})
	}
	// Longer terms first, so "folder group" is offered before "folder".
	sort.SliceStable(terms, func(a, b int) bool { return len(terms[a].en) > len(terms[b].en) })
	return terms, sc.Err()
}
