// docsite はリポジトリの Markdown を読みやすく並べた閲覧サイトを出す。
// task docs（手元で開く）と task docs-build（静的な HTML に書き出す）の実体。
//
// 文書はディレクトリ構成のまま .md を .html に置き換えた場所に出すので、文書
// どうしの相対リンクはそのまま通る。サーバーは要求のたびに読み直すので、文書を
// 直してブラウザを再読み込みすればすぐ反映される。
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/syudead/vv/scripts/devtools"
)

//go:embed assets
var assetFS embed.FS

var pageTemplate = template.Must(template.ParseFS(assetFS, "assets/page.html"))

const (
	assetDir    = "_docsite"
	searchIndex = "search-index.js"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:6060", "待ち受けるアドレス")
	out := flag.String("out", "", "指定すると、サーバーを立てずにこのディレクトリへ静的な HTML を書き出す")
	github := flag.String("github", "https://github.com/syudead/vv", "ソースへのリンク先にするリポジトリの URL")
	ref := flag.String("ref", "main", "GitHub へのリンクに使うブランチ")
	flag.Parse()

	root, err := devtools.RepositoryRoot()
	if err != nil {
		log.Fatal(err)
	}
	s := site{root: root, github: *github, ref: *ref}
	if *out != "" {
		if err := s.export(*out); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := s.serve(*addr); err != nil {
		log.Fatal(err)
	}
}

type site struct {
	root   string
	github string
	ref    string
	// list はテストから差し替える。nil なら git から得る。
	list func() ([]string, error)
}

// files は出してよいファイルの一覧を得る。追跡中のものに加え、まだ add して
// いない新しい文書も書きながら確かめられるよう、無視されていない未追跡の
// ファイルも含める。
func (s site) files() (repoFiles, error) {
	if s.list != nil {
		paths, err := s.list()
		return newRepoFiles(paths), err
	}
	cmd := exec.Command("git", "-C", s.root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return repoFiles{}, fmt.Errorf("git ls-files に失敗しました: %w", err)
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		// 追跡中でも作業ツリーで消したファイルは出せない。
		if p != "" && fileExists(filepath.Join(s.root, filepath.FromSlash(p))) {
			paths = append(paths, p)
		}
	}
	return newRepoFiles(paths), nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// read と stat は os.Root 越しに開くので、どのパスを渡されてもリポジトリの
// 外（.. やシンボリックリンクの先）には出ない。
func (s site) read(p string) ([]byte, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(filepath.FromSlash(p))
}

func (s site) stat(p string) (os.FileInfo, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.Stat(filepath.FromSlash(p))
}

// navNode はサイドバーの木の1つの節である。
type navNode struct {
	Name     string
	Title    string
	URL      string // 文書なら current のページから見た相対 URL。ディレクトリは空。
	Current  bool
	Open     bool
	Children []*navNode
}

// buildNav は文書の一覧をディレクトリの木にする。current を含む枝は開いておく。
func buildNav(files repoFiles, titles map[string]string, current string) []*navNode {
	root := &navNode{}
	dirs := map[string]*navNode{"": root}
	var dirOf func(string) *navNode
	dirOf = func(dir string) *navNode {
		if n, ok := dirs[dir]; ok {
			return n
		}
		parent := dirOf(parentDir(dir))
		n := &navNode{Name: path.Base(dir)}
		parent.Children = append(parent.Children, n)
		dirs[dir] = n
		return n
	}
	for _, doc := range files.docs {
		dir := parentDir(doc)
		parent := dirOf(dir)
		parent.Children = append(parent.Children, &navNode{
			Name:    path.Base(doc),
			Title:   titles[doc],
			URL:     relativeURL(current, pageURL(doc)),
			Current: doc == current,
		})
		if doc == current {
			for d := dir; ; d = parentDir(d) {
				dirs[d].Open = true
				if d == "" {
					break
				}
			}
		}
	}
	// 文書をディレクトリより先に並べる。sortDocs の順を保つため安定な並べ替えにする。
	var order func(*navNode)
	order = func(n *navNode) {
		var files, subdirs []*navNode
		for _, c := range n.Children {
			if c.URL != "" {
				files = append(files, c)
			} else {
				subdirs = append(subdirs, c)
				order(c)
			}
		}
		n.Children = append(files, subdirs...)
	}
	order(root)
	return root.Children
}

func parentDir(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}

var firstHeading = regexp.MustCompile(`(?m)^#[ \t]+(.+?)[ \t#]*$`)

// titleOf は最初の見出し1を題にする。無ければファイル名にする。コードブロックの
// 中の # を拾う恐れはあるが、題に使う見出し1は文書の先頭にあるので実害はない。
func titleOf(doc string, source []byte) string {
	if m := firstHeading.FindSubmatch(source); m != nil {
		t := strings.NewReplacer("`", "", "**", "", "*", "").Replace(string(m[1]))
		return strings.TrimSpace(t)
	}
	return path.Base(doc)
}

func (s site) titles(files repoFiles) map[string]string {
	titles := make(map[string]string, len(files.docs))
	for _, doc := range files.docs {
		source, err := s.read(doc)
		if err != nil {
			titles[doc] = path.Base(doc)
			continue
		}
		titles[doc] = titleOf(doc, source)
	}
	return titles
}

type pageData struct {
	Title    string
	Doc      string
	Root     string
	Body     template.HTML
	TOC      []tocEntry
	Nav      []*navNode
	Source   string
	Modified string
}

// page は1文書を描いて HTML ページにする。
func (s site) page(files repoFiles, titles map[string]string, doc string) ([]byte, rendered, error) {
	source, err := s.read(doc)
	if err != nil {
		return nil, rendered{}, err
	}
	r := renderer{files: files, github: s.github, ref: s.ref}
	result, err := r.render(doc, source)
	if err != nil {
		return nil, rendered{}, err
	}
	data := pageData{
		Title:  result.Title,
		Doc:    doc,
		Root:   rootPrefix(doc),
		Body:   template.HTML(result.HTML), //nolint:gosec // リポジトリの文書を描いたもの
		TOC:    result.TOC,
		Nav:    buildNav(files, titles, doc),
		Source: r.githubURL("blob", doc),
	}
	if info, err := s.stat(doc); err == nil {
		data.Modified = info.ModTime().Format("2006-01-02 15:04")
	}
	var buf bytes.Buffer
	if err := pageTemplate.Execute(&buf, data); err != nil {
		return nil, rendered{}, fmt.Errorf("%s のページを組めません: %w", doc, err)
	}
	return buf.Bytes(), result, nil
}

type searchEntry struct {
	URL   string `json:"u"`
	Path  string `json:"p"`
	Title string `json:"t"`
	Body  string `json:"b"`
}

var (
	spaces = regexp.MustCompile(`\s+`)
	// markup は検索の抜粋に出しても読みにくいだけの Markdown の記号である。
	// リンクは文字だけを残し、見出しや強調の記号と表の区切りを落とす。
	linkMarkup = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	markup     = regexp.MustCompile("(?m)^#+ |[*`]+|^\\s*[-|: ]+$|\\|")
)

func searchText(source []byte) string {
	s := linkMarkup.ReplaceAllString(string(source), "$1")
	s = markup.ReplaceAllString(s, " ")
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

// searchIndexJS は全文検索用の索引を返す。fetch ではなく script で読ませるので、
// 書き出した HTML を file:// で開いても検索できる。
func (s site) searchIndexJS(files repoFiles, titles map[string]string) ([]byte, error) {
	entries := make([]searchEntry, 0, len(files.docs))
	for _, doc := range files.docs {
		source, err := s.read(doc)
		if err != nil {
			return nil, err
		}
		entries = append(entries, searchEntry{
			URL:   pageURL(doc),
			Path:  doc,
			Title: titles[doc],
			Body:  searchText(source),
		})
	}
	body, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	return append(append([]byte("window.DOCSITE_INDEX="), body...), ';', '\n'), nil
}

// serve は手元のブラウザ向けにサイトを出す。
func (s site) serve(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Printf("docs: http://%s/ で閲覧できます（Ctrl+C で終了）\n", listener.Addr())
	server := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
	return server.Serve(listener)
}

func (s site) handler() http.Handler {
	static, err := fs.Sub(assetFS, "assets")
	if err != nil {
		panic(err)
	}
	staticFiles := http.StripPrefix("/"+assetDir+"/", http.FileServer(http.FS(static)))
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+req.URL.Path), "/")
		files, err := s.files()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		switch {
		case p == assetDir+"/"+searchIndex:
			titles := s.titles(files)
			body, err := s.searchIndexJS(files, titles)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write(body)
			return
		case strings.HasPrefix(p, assetDir+"/"):
			staticFiles.ServeHTTP(w, req)
			return
		}

		doc, ok := docFor(files, p)
		if ok {
			if p == "" || strings.HasSuffix(req.URL.Path, "/") {
				// ディレクトリの URL で開いたときは、相対リンクが正しく解決される
				// よう文書のページへ移る。
				http.Redirect(w, req, "/"+pageURL(doc), http.StatusFound)
				return
			}
			body, _, err := s.page(files, s.titles(files), doc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(body)
			return
		}
		if asset, ok := files.lookup(p); ok && assetExts[strings.ToLower(path.Ext(asset))] {
			body, err := s.read(asset)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			var modified time.Time
			if info, err := s.stat(asset); err == nil {
				modified = info.ModTime()
			}
			http.ServeContent(w, req, path.Base(asset), modified, bytes.NewReader(body))
			return
		}
		http.NotFound(w, req)
	})
}

// docFor は URL のパスに当たる文書を返す。"" とディレクトリは README.md か
// index.md を、x.html は x.md を指す。
func docFor(files repoFiles, p string) (string, bool) {
	if p == "" || files.isDir(p) {
		return files.dirIndex(p)
	}
	if strings.HasSuffix(p, ".html") {
		doc, ok := files.lookup(strings.TrimSuffix(p, ".html") + ".md")
		return doc, ok
	}
	return "", false
}

// export は全ての文書を out の下へ静的な HTML として書き出す。
func (s site) export(out string) error {
	if !filepath.IsAbs(out) {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		out = filepath.Join(wd, out)
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	files, err := s.files()
	if err != nil {
		return err
	}
	titles := s.titles(files)
	assets := map[string]bool{}
	for _, doc := range files.docs {
		body, result, err := s.page(files, titles, doc)
		if err != nil {
			return err
		}
		if err := writeFile(out, pageURL(doc), body); err != nil {
			return err
		}
		for _, a := range result.Assets {
			assets[a] = true
		}
	}
	for a := range assets {
		body, err := s.read(a)
		if err != nil {
			return err
		}
		if err := writeFile(out, a, body); err != nil {
			return err
		}
	}

	index, err := s.searchIndexJS(files, titles)
	if err != nil {
		return err
	}
	if err := writeFile(out, assetDir+"/"+searchIndex, index); err != nil {
		return err
	}
	err = fs.WalkDir(assetFS, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "assets/page.html" {
			return err
		}
		body, err := assetFS.ReadFile(p)
		if err != nil {
			return err
		}
		return writeFile(out, assetDir+"/"+strings.TrimPrefix(p, "assets/"), body)
	})
	if err != nil {
		return err
	}

	// 根の index.html は README へ案内する。
	home, ok := files.dirIndex("")
	if !ok {
		return errors.New("リポジトリ直下に README.md がありません")
	}
	redirect := fmt.Sprintf(`<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="0; url=%[1]s"><a href="%[1]s">%[1]s</a>`+"\n", pageURL(home))
	if err := writeFile(out, "index.html", []byte(redirect)); err != nil {
		return err
	}
	fmt.Printf("docs: %d 文書を %s へ書き出しました\n", len(files.docs), out)
	return nil
}

func writeFile(out, rel string, body []byte) error {
	dest := filepath.Join(out, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0o644)
}
