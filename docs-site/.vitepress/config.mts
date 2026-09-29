// 文書サイトの設定。リポジトリの docs/ と specs/ だけをそのままの構成で公開する。
// 文書は置き場所を変えずに読むので、リポジトリの根を srcDir にし、公開しない
// ものを srcExclude で外す。
//
// 英語の原文を / に、scripts/translate が生成した日本語版（docs-site/ja/）を
// /ja/ に出す（docs/design-docs/translation-pipeline.md）。
import fs from 'node:fs'
import path from 'node:path'
import { slug } from 'github-slugger'
import { type DefaultTheme } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'
import type MarkdownIt from 'markdown-it'

const repoRoot = path.resolve(import.meta.dirname, '../..')
const github = 'https://github.com/syudead/vv'
const branch = 'main'

// 公開する文書のディレクトリ。これ以外の Markdown（エージェント向けの
// .agents/ や .claude/、リポジトリ直下の README など）は出さない。
const published = ['docs', 'specs']

// 日本語版の置き場。task docs-translate を走らせる前は無いので、そのときは
// 日本語の locale を作らない。
const jaDir = 'docs-site/ja'
const hasJa = fs.existsSync(path.join(repoRoot, jaDir))

// リポジトリ直下のうち、公開するものとこのサイト自身以外を全て外す。新しい
// ディレクトリが増えても、公開する側へ入れない限り出ない。
const srcExclude = [
  '**/node_modules/**',
  'docs-site/.vitepress/**',
  'docs-site/.translation/**',
  // 型の中の相対リンクは置き先から見た形なので、ここでは解決できない。
  // リンクは GitHub の表示へ向く（unpublished）。
  'docs/templates/**',
  ...fs
    .readdirSync(repoRoot, { withFileTypes: true })
    .filter((e) => !published.includes(e.name) && e.name !== 'docs-site')
    .map((e) => (e.isDirectory() ? `${e.name}/**` : e.name)),
]

// README.md はディレクトリの入口なので index として出す。/docs/how-to/ の
// ようなディレクトリの URL で開ける。
function rewrite(id: string): string {
  if (id === 'docs-site/index.md') return 'index.md'
  if (id.startsWith(jaDir + '/')) id = 'ja/' + id.slice(jaDir.length + 1)
  return id.replace(/(^|\/)README\.md$/, '$1index.md')
}

const imageExt = /\.(png|jpe?g|gif|svg|webp)$/i

const unpublished = ['docs/templates']

function isPublished(p: string): boolean {
  const under = (dir: string) => p === dir || p.startsWith(dir + '/')
  return [...published, jaDir].some(under) && !unpublished.some(under)
}

function stat(p: string): fs.Stats | undefined {
  try {
    return fs.statSync(path.join(repoRoot, p))
  } catch {
    return undefined
  }
}

function hasIndex(dir: string): boolean {
  return ['README.md', 'index.md'].some((name) => stat(path.posix.join(dir, name))?.isFile())
}

// resolveLink は文書 from（リポジトリの根からのパス）の中のリンク先を、サイトで
// 通る形に変える。公開する文書と画像はそのまま（README.md は index.md へ）、
// それ以外のリポジトリ内のファイルとディレクトリは GitHub の表示へ向ける。
// 見つからないリンクは手を加えず、VitePress のリンク切れ検査に任せる。
export function resolveLink(from: string, href: string): string {
  if (!href || href.startsWith('#') || /^[a-z][a-z0-9+.-]*:/i.test(href) || href.startsWith('//')) {
    return href
  }
  const hashAt = href.indexOf('#')
  const target = hashAt >= 0 ? href.slice(0, hashAt) : href
  const hash = hashAt >= 0 ? href.slice(hashAt) : ''
  if (!target) return href

  let decoded: string
  try {
    decoded = decodeURI(target)
  } catch {
    return href
  }
  const resolved = decoded.startsWith('/')
    ? path.posix.normalize(decoded.slice(1))
    : path.posix.normalize(path.posix.join(path.posix.dirname(from), decoded))
  if (resolved.startsWith('..')) return href
  const info = stat(resolved)
  if (!info) return href

  if (isPublished(resolved)) {
    // 画像は Vite が実際のファイルの位置から読むので、相対のまま渡す。
    if (info.isFile() && imageExt.test(resolved)) return href
    // 文書どうしのリンクは、書き換え後のページの位置から解決される。日本語版は
    // docs-site/ja/ から /ja/ へ階層が1つ浅くなるので、サイトの根からの形にする。
    if (info.isFile() && resolved.endsWith('.md')) return '/' + rewrite(resolved) + hash
    if (info.isDirectory() && hasIndex(resolved)) {
      const index = ['README.md', 'index.md'].find((name) => stat(path.posix.join(resolved, name))?.isFile())!
      return '/' + rewrite(path.posix.join(resolved, index)).replace(/index\.md$/, '') + hash
    }
  }
  const kind = info.isDirectory() ? 'tree' : 'blob'
  const repoPath = resolved === '.' ? '' : `/${kind}/${branch}/${encodeURI(resolved)}`
  return github + repoPath + hash
}

function linkRewriter(md: MarkdownIt) {
  md.core.ruler.after('inline', 'vv-links', (state) => {
    const file: string | undefined = state.env?.path
    if (!file) return
    let from = path.relative(repoRoot, file).split(path.sep).join('/')
    // 検索の索引を作るときは、書き換え後のパス（ja/...）で渡ってくる。
    if (from.startsWith('ja/')) from = jaDir + from.slice(2)
    for (const block of state.tokens) {
      for (const token of block.children ?? []) {
        const attr = token.type === 'link_open' ? 'href' : token.type === 'image' ? 'src' : ''
        const value = attr && token.attrGet(attr)
        if (value) token.attrSet(attr, resolveLink(from, value))
      }
    }
  })
}

// 日本語は語を空白で区切らないので、そのままでは全文検索がほとんど当たらない。
// 漢字・かな・カナの連なりは2文字ずつの組に分けて索引にする。連なりの最後の
// 1文字も単独で加え、どの文字も何かの語の先頭になるようにする。前方一致の
// 検索で、語末の1文字（「シーク」の「ク」）だけを探しても当たる。
function tokenize(text: string): string[] {
  const tokens: string[] = []
  for (const part of text.toLowerCase().split(/[\s\p{P}\p{S}]+/u)) {
    if (!part) continue
    for (const run of part.match(/[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}ー]+|[^\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}ー]+/gu) ?? []) {
      if (/^[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}ー]+$/u.test(run) && run.length > 1) {
        const chars = [...run]
        for (let i = 0; i < chars.length - 1; i++) tokens.push(chars[i] + chars[i + 1])
        tokens.push(chars[chars.length - 1])
      } else {
        tokens.push(run)
      }
    }
  }
  return tokens
}

// 見出し1を題として読む。サイドバーの表示に使う。
function titleOf(file: string): string {
  const source = fs.readFileSync(path.join(repoRoot, file), 'utf8')
  const m = source.match(/^#[ \t]+(.+?)[ \t#]*$/m)
  // 日本語版の見出しは英語の anchor を {#...} で持つので、題からは外す。
  return m ? m[1].replace(/\s*\{#[^}]+\}$/, '').replace(/[`*]/g, '').trim() : path.posix.basename(file, '.md')
}

function pageLink(file: string): string {
  return '/' + rewrite(file).replace(/(^|\/)index\.md$/, '$1').replace(/\.md$/, '')
}

// ディレクトリの構成どおりのサイドバーを作る。入口（README / index）を先頭に、
// 他の文書を題の順ではなくファイル名の順に並べる（番号付きの specs の順を保つ）。
function sidebarFor(dir: string, collapsed: boolean, overview = 'Overview'): DefaultTheme.SidebarItem[] {
  const entries = fs.readdirSync(path.join(repoRoot, dir), { withFileTypes: true })
  const index = entries.find((e) => e.isFile() && (e.name === 'README.md' || e.name === 'index.md'))
  const docs = entries
    .filter((e) => e.isFile() && e.name.endsWith('.md') && e !== index)
    .map((e) => e.name)
    .sort()
  const dirs = entries
    .filter((e) => e.isDirectory() && hasMarkdown(path.posix.join(dir, e.name)))
    .map((e) => e.name)
    .sort()
  const items: DefaultTheme.SidebarItem[] = []
  if (index) items.push({ text: overview, link: pageLink(path.posix.join(dir, index.name)) })
  for (const name of docs) {
    const file = path.posix.join(dir, name)
    items.push({ text: titleOf(file), link: pageLink(file) })
  }
  for (const name of dirs) {
    const sub = path.posix.join(dir, name)
    items.push({ text: name, collapsed, items: sidebarFor(sub, true, overview) })
  }
  return items
}

function hasMarkdown(dir: string): boolean {
  return fs
    .readdirSync(path.join(repoRoot, dir), { withFileTypes: true, recursive: true })
    .some((e) => e.isFile() && e.name.endsWith('.md'))
}

const sections = [
  { dir: 'docs/design-docs', en: 'Design docs', ja: '設計文書' },
  { dir: 'docs/product-specs', en: 'Product specs', ja: 'プロダクト仕様' },
  { dir: 'docs/how-to', en: 'How-to', ja: '手順' },
]
const specs = { en: 'Feature plans', ja: '機能ごとの計画' }

const enTheme: DefaultTheme.Config = {
  nav: [
    ...sections.map((s) => ({ text: s.en, link: `/${s.dir}/` })),
    { text: specs.en, link: '/specs/' },
  ],
  sidebar: {
    '/docs/': sections.map((s) => ({ text: s.en, items: sidebarFor(s.dir, true) })),
    '/specs/': [{ text: specs.en, items: sidebarFor('specs', true) }],
  },
  outline: { level: [2, 3] },
  editLink: { pattern: `${github}/edit/${branch}/:path`, text: 'Edit on GitHub' },
}

// 日本語版のサイトの枠の文言。文書の本文ではないので、翻訳の工程を通さない。
const jaTheme: DefaultTheme.Config = {
  nav: [
    ...sections.map((s) => ({ text: s.ja, link: `/ja/${s.dir}/` })),
    { text: specs.ja, link: '/ja/specs/' },
  ],
  sidebar: hasJa
    ? {
        '/ja/docs/': sections.map((s) => ({ text: s.ja, items: sidebarFor(`${jaDir}/${s.dir}`, true, '概要') })),
        '/ja/specs/': [{ text: specs.ja, items: sidebarFor(`${jaDir}/specs`, true, '概要') }],
      }
    : {},
  outline: { level: [2, 3], label: '目次' },
  lastUpdated: { text: '最終更新' },
  docFooter: { prev: '前のページ', next: '次のページ' },
  darkModeSwitchLabel: '配色',
  sidebarMenuLabel: 'メニュー',
  returnToTopLabel: '先頭へ戻る',
  langMenuLabel: '言語',
}

export default withMermaid({
  title: 'vv docs',
  base: '/vv/',
  srcDir: '..',
  srcExclude,
  rewrites: rewrite,
  cleanUrls: true,
  // 手順の中の http://localhost:8080 のような例は、閲覧者の手元を指すリンクである。
  ignoreDeadLinks: 'localhostLinks',
  lastUpdated: true,
  locales: {
    root: {
      label: 'English',
      lang: 'en',
      description: 'Design docs, specifications and procedures for vv',
      themeConfig: enTheme,
    },
    ...(hasJa
      ? {
          ja: {
            label: '日本語（機械翻訳）',
            lang: 'ja',
            description: 'vv の設計文書・仕様・手順（英語版からの機械翻訳）',
            themeConfig: jaTheme,
          },
        }
      : {}),
  },
  vite: {
    // 文書は docs-site/ の外にあるので、そこから見える node_modules が無い。
    // 文書から生成されるコードの import は docs-site/ の依存から解決する。
    resolve: {
      alias: [
        {
          find: /^vue(\/.*)?$/,
          replacement: path.join(import.meta.dirname, '../node_modules/vue') + '$1',
        },
      ],
    },
    server: { fs: { allow: [repoRoot] } },
    // mermaid の依存は CommonJS を含むので、開発サーバーでも前もって束ねる。
    optimizeDeps: { include: ['mermaid'] },
  },
  markdown: {
    anchor: { slugify: slug },
    headers: { slugify: slug },
    config: (md) => md.use(linkRewriter),
  },
  themeConfig: {
    search: {
      provider: 'local',
      options: {
        miniSearch: {
          options: { tokenize },
          searchOptions: { prefix: true, combineWith: 'AND' },
        },
        locales: {
          ja: {
            translations: {
              button: { buttonText: '検索', buttonAriaLabel: '検索' },
              modal: {
                noResultsText: '見つかりません',
                resetButtonTitle: '消去',
                footer: { selectText: '選ぶ', navigateText: '移動', closeText: '閉じる' },
              },
            },
          },
        },
      },
    },
    socialLinks: [{ icon: 'github', link: github }],
  },
})
