package main

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// repoFiles は版管理下（と無視されていない未追跡）のファイルの集合である。
// 閲覧サイトが出してよいのはこの中のものだけで、.env のような無視された
// ファイルは一覧に現れないので外へ出ない。
type repoFiles struct {
	// all は git の一覧にあるパスからそのパス自身への対応。要求の URL から
	// ファイルを開くときは、要求の文字列ではなくここから引いた値を使う。
	all  map[string]string
	docs []string // Markdown だけ。並びは sortDocs の順。
}

func newRepoFiles(paths []string) repoFiles {
	files := repoFiles{all: make(map[string]string, len(paths))}
	for _, p := range paths {
		if p == "" {
			continue
		}
		files.all[p] = p
		if strings.HasSuffix(p, ".md") {
			files.docs = append(files.docs, p)
		}
	}
	sortDocs(files.docs)
	return files
}

// lookup は一覧にある p を、一覧に記録したパスとして返す。
func (f repoFiles) lookup(p string) (string, bool) {
	listed, ok := f.all[p]
	return listed, ok
}

func (f repoFiles) has(p string) bool {
	_, ok := f.all[p]
	return ok
}

func (f repoFiles) isDoc(p string) bool { return f.has(p) && strings.HasSuffix(p, ".md") }

// isDir は p の下に1つでもファイルがあるかを返す。
func (f repoFiles) isDir(p string) bool {
	prefix := strings.TrimSuffix(p, "/") + "/"
	if prefix == "/" {
		return true
	}
	for name := range f.all {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// dirIndex はディレクトリを開いたときに見せる文書を返す。
func (f repoFiles) dirIndex(dir string) (string, bool) {
	for _, name := range []string{"README.md", "index.md"} {
		if doc, ok := f.lookup(path.Join(dir, name)); ok {
			return doc, true
		}
	}
	return "", false
}

// sortDocs はリポジトリ直下の文書を先に、残りをパスの順に並べる。直下の
// README.md が常に先頭に来るよう、ディレクトリより前に置く。
func sortDocs(docs []string) {
	sort.Slice(docs, func(i, j int) bool {
		a, b := docs[i], docs[j]
		aTop, bTop := !strings.Contains(a, "/"), !strings.Contains(b, "/")
		if aTop != bTop {
			return aTop
		}
		if aTop && (a == "README.md") != (b == "README.md") {
			return a == "README.md"
		}
		return a < b
	})
}

// pageURL は文書のパスを閲覧サイト上のパスへ変える。ディレクトリ構成を
// そのまま保つので、文書どうしの相対リンクは拡張子を変えるだけで通る。
func pageURL(doc string) string { return strings.TrimSuffix(doc, ".md") + ".html" }

// rootPrefix は page から根へ戻る相対パスを返す（例: docs/a.md → "../"）。
// 相対パスだけで組むので、書き出した HTML は file:// でも開ける。
func rootPrefix(doc string) string {
	return strings.Repeat("../", strings.Count(doc, "/"))
}

// tocEntry は右側の目次の1行である。
type tocEntry struct {
	Level int
	ID    string
	Text  string
}

// rendered は1文書を描いた結果である。
type rendered struct {
	Title string
	HTML  string
	TOC   []tocEntry
	// Assets は本文が参照している、文書以外の手元のファイル（画像など）。
	// 静的書き出しではこれを一緒に写す。
	Assets []string
}

type renderer struct {
	files  repoFiles
	github string // 例: https://github.com/syudead/vv
	ref    string // 例: main
}

// assetExts は閲覧サイトが自前で出すファイルの拡張子。これ以外（ソースや
// 設定ファイル）へのリンクは GitHub の表示へ向ける。構文の色付けや行番号は
// GitHub の方がよく、ここで作り直す理由がない。
var assetExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".webp": true,
}

func (r renderer) render(doc string, source []byte) (rendered, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		// 生の HTML は出さない（goldmark の既定）。未追跡の文書も出すので、文書に
		// 書かれたスクリプトを閲覧者のブラウザで動かさないためである。文書が使って
		// いる生の HTML は注釈（<!-- -->）だけで、出さなくても見た目は変わらない。
	)
	root := md.Parser().Parse(text.NewReader(source))
	// id は描いた後の文字から作る。goldmark の自動 id は見出しの生の行を使うので、
	// リンクや強調の記号まで id に混ざり、GitHub の anchor と食い違う。
	ids := newGitHubIDs()

	out := rendered{Title: path.Base(doc)}
	assets := map[string]bool{}
	titled := false
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Heading:
			label := plainText(node, source)
			if node.Level == 1 && !titled {
				out.Title, titled = label, true
			}
			id := ids.generate(label)
			node.SetAttributeString("id", []byte(id))
			if node.Level == 2 || node.Level == 3 {
				out.TOC = append(out.TOC, tocEntry{Level: node.Level, ID: id, Text: label})
			}
		case *ast.Link:
			node.Destination = []byte(r.rewrite(doc, string(node.Destination), assets))
		case *ast.Image:
			node.Destination = []byte(r.rewrite(doc, string(node.Destination), assets))
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return rendered{}, err
	}

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, root); err != nil {
		return rendered{}, fmt.Errorf("%s を HTML にできません: %w", doc, err)
	}
	out.HTML = buf.String()
	for a := range assets {
		out.Assets = append(out.Assets, a)
	}
	sort.Strings(out.Assets)
	return out, nil
}

// rewrite は文書 doc の中のリンク先を閲覧サイト用に変える。
//   - 外部 URL と文書内の anchor はそのまま。
//   - 文書（.md）と、README.md / index.md を持つディレクトリは .html のページへ。
//   - 画像は手元から出す（assets に記録する）。
//   - それ以外のリポジトリ内のファイルとディレクトリは GitHub の表示へ。
func (r renderer) rewrite(doc, dest string, assets map[string]bool) string {
	if dest == "" || strings.HasPrefix(dest, "#") {
		return dest
	}
	if u, err := url.Parse(dest); err != nil || u.Scheme != "" || u.Host != "" {
		return dest
	}
	target, fragment, _ := strings.Cut(dest, "#")
	if fragment != "" {
		fragment = "#" + fragment
	}
	unescaped, err := url.PathUnescape(target)
	if err != nil {
		return dest
	}
	var resolved string
	if strings.HasPrefix(unescaped, "/") {
		resolved = path.Clean(strings.TrimPrefix(unescaped, "/"))
	} else {
		resolved = path.Join(path.Dir(doc), unescaped)
	}
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return dest
	}

	switch {
	case r.files.isDoc(resolved):
		return relativeURL(doc, pageURL(resolved)) + fragment
	case r.files.has(resolved) && assetExts[strings.ToLower(path.Ext(resolved))]:
		assets[resolved] = true
		return relativeURL(doc, resolved) + fragment
	case r.files.has(resolved):
		return r.githubURL("blob", resolved) + fragment
	case r.files.isDir(resolved):
		if index, ok := r.files.dirIndex(resolved); ok {
			return relativeURL(doc, pageURL(index)) + fragment
		}
		return r.githubURL("tree", resolved) + fragment
	}
	// 見つからないリンクは手を加えずに残す。切れたリンクは task check-docs が落とす。
	return dest
}

func (r renderer) githubURL(kind, p string) string {
	if p == "." {
		return r.github
	}
	return r.github + "/" + kind + "/" + r.ref + "/" + (&url.URL{Path: p}).EscapedPath()
}

// relativeURL は from の文書から見た to（根からのパス）への相対 URL を返す。
func relativeURL(from, to string) string {
	return rootPrefix(from) + (&url.URL{Path: to}).EscapedPath()
}

// plainText は見出しなどの中の文字だけを取り出す。
func plainText(n ast.Node, source []byte) string {
	var b strings.Builder
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch node := c.(type) {
			case *ast.Text:
				b.Write(node.Segment.Value(source))
				if node.SoftLineBreak() || node.HardLineBreak() {
					b.WriteByte(' ')
				}
			case *ast.String:
				b.Write(node.Value)
			case *ast.CodeSpan:
				for l := node.FirstChild(); l != nil; l = l.NextSibling() {
					if t, ok := l.(*ast.Text); ok {
						b.Write(t.Segment.Value(source))
					}
				}
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

// gitHubIDs は GitHub と同じ規則で見出しの id を作る。文書は日本語の見出しへ
// `#r-3-置き場と完成の印` のようにリンクしており、goldmark 既定の規則は ASCII
// 以外を落とすのでこのリンクが通らない。
type gitHubIDs struct{ used map[string]bool }

func newGitHubIDs() *gitHubIDs { return &gitHubIDs{used: map[string]bool{}} }

// generate は重なった見出しに -1, -2 と番号を足す（GitHub と同じ）。
func (g *gitHubIDs) generate(label string) string {
	base := slugify(label)
	id := base
	for n := 1; g.used[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	g.used[id] = true
	return id
}

// slugify は GitHub の見出し anchor の規則に倣う。小文字にし、文字・数字・
// 空白・- と _ 以外を落とし、空白を - にする。
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.Mn, r):
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "section"
	}
	return b.String()
}
