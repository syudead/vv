// Documentation site. Publishes English documents in docs/ and ARCHITECTURE.md
// directly from the repository; other paths are excluded with srcExclude.
import fs from 'node:fs'
import path from 'node:path'
import { slug } from 'github-slugger'
import { defineConfig, type DefaultTheme } from 'vitepress'
import type MarkdownIt from 'markdown-it'

const repoRoot = path.resolve(import.meta.dirname, '../..')
const github = 'https://github.com/syudead/vv'
const branch = 'main'

// Published documents. Other Markdown (agent instructions in .agents/ and
// .claude/, the root README) is not published.
const published = ['docs', 'ARCHITECTURE.md']

// New root directories stay private unless explicitly included.
const srcExclude = [
  '**/node_modules/**',
  'docs-site/.vitepress/**',
  ...fs
    .readdirSync(repoRoot, { withFileTypes: true })
    .filter((e) => !published.includes(e.name) && e.name !== 'docs-site')
    .map((e) => (e.isDirectory() ? `${e.name}/**` : e.name)),
]

// Serve README.md as index so directory URLs work.
function rewrite(id: string): string {
  if (id === 'docs-site/index.md') return 'index.md'
  return id.replace(/(^|\/)README\.md$/, '$1index.md')
}

const imageExt = /\.(png|jpe?g|gif|svg|webp)$/i

function isPublished(p: string): boolean {
  return published.some((dir) => p === dir || p.startsWith(dir + '/'))
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

// resolveLink rewrites a link in document `from` (a path from the repository
// root) so it works on the site. Published documents and images stay as they
// are (README.md becomes index.md); other repository files and directories
// point at GitHub. Links that resolve to nothing are left alone for
// VitePress's dead-link check.
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
    if (info.isFile() && (resolved.endsWith('.md') || imageExt.test(resolved))) {
      return target.replace(/(^|\/)README\.md$/, '$1index.md') + hash
    }
    if (info.isDirectory() && hasIndex(resolved)) return href
  }
  const kind = info.isDirectory() ? 'tree' : 'blob'
  const repoPath = resolved === '.' ? '' : `/${kind}/${branch}/${encodeURI(resolved)}`
  return github + repoPath + hash
}

function linkRewriter(md: MarkdownIt) {
  md.core.ruler.after('inline', 'vv-links', (state) => {
    const file: string | undefined = state.env?.path
    if (!file) return
    const from = path.relative(repoRoot, file).split(path.sep).join('/')
    for (const block of state.tokens) {
      for (const token of block.children ?? []) {
        const attr = token.type === 'link_open' ? 'href' : token.type === 'image' ? 'src' : ''
        const value = attr && token.attrGet(attr)
        if (value) token.attrSet(attr, resolveLink(from, value))
      }
    }
  })
}

// A ```mermaid block becomes a component that draws it in the browser.
function mermaidFence(md: MarkdownIt) {
  const fence = md.renderer.rules.fence!
  md.renderer.rules.fence = (tokens, idx, options, env, self) => {
    const token = tokens[idx]
    if (token.info.trim() !== 'mermaid') return fence(tokens, idx, options, env, self)
    return `<MermaidDiagram code="${encodeURIComponent(token.content)}" />`
  }
}

function titleOf(file: string): string {
  const source = fs.readFileSync(path.join(repoRoot, file), 'utf8')
  const m = source.match(/^#[ \t]+(.+?)[ \t#]*$/m)
  return m ? m[1].replace(/\s*\{#[^}]*\}$/, '').replace(/[`*]/g, '').trim() : path.posix.basename(file, '.md')
}

function pageLink(file: string): string {
  return '/' + rewrite(file).replace(/(^|\/)index\.md$/, '$1').replace(/\.md$/, '')
}

// Put each directory entry first, then sort documents by filename rather than title.
function sidebarFor(dir: string, collapsed: boolean): DefaultTheme.SidebarItem[] {
  if (!stat(dir)?.isDirectory()) return []
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
  if (index) items.push({ text: 'Overview', link: pageLink(path.posix.join(dir, index.name)) })
  for (const name of docs) {
    const file = path.posix.join(dir, name)
    items.push({ text: titleOf(file), link: pageLink(file) })
  }
  for (const name of dirs) {
    const sub = path.posix.join(dir, name)
    items.push({ text: name, collapsed, items: sidebarFor(sub, true) })
  }
  return items
}

function hasMarkdown(dir: string): boolean {
  return fs
    .readdirSync(path.join(repoRoot, dir), { withFileTypes: true, recursive: true })
    .some((e) => e.isFile() && e.name.endsWith('.md'))
}

const navigation: DefaultTheme.Config = {
  nav: [
    { text: 'Design', link: '/docs/design-docs/' },
    { text: 'Product specs', link: '/docs/product-specs/' },
    { text: 'How-to', link: '/docs/how-to/' },
    { text: 'Architecture', link: '/ARCHITECTURE' },
  ],
  sidebar: {
    '/docs/': [
      { text: 'Design', items: sidebarFor('docs/design-docs', true) },
      { text: 'Product specs', items: sidebarFor('docs/product-specs', true) },
      { text: 'How-to', items: sidebarFor('docs/how-to', true) },
    ],
  },
  editLink: { pattern: `${github}/edit/${branch}/:path`, text: 'Edit on GitHub' },
}

export default defineConfig({
  title: 'vv docs',
  base: '/vv/',
  srcDir: '..',
  srcExclude,
  rewrites: rewrite,
  cleanUrls: true,
  // Examples such as http://localhost:8080 in the guides point at the reader's
  // own machine.
  ignoreDeadLinks: 'localhostLinks',
  lastUpdated: true,
  lang: 'en',
  description: 'Design documents, specifications and guides for vv',
  vite: {
    // The documents live outside docs-site/, so no node_modules is visible from
    // them. Imports in code generated from documents resolve from docs-site/.
    resolve: {
      alias: [
        {
          find: /^vue(\/.*)?$/,
          replacement: path.join(import.meta.dirname, '../node_modules/vue') + '$1',
        },
      ],
    },
    server: { fs: { allow: [repoRoot] } },
  },
  markdown: {
    anchor: { slugify: slug },
    headers: { slugify: slug },
    config: (md) => md.use(linkRewriter).use(mermaidFence),
  },
  themeConfig: {
    ...navigation,
    outline: { level: [2, 3] },
    search: {
      provider: 'local',
      options: {
        miniSearch: {
          searchOptions: { prefix: true, combineWith: 'AND' },
        },
      },
    },
    socialLinks: [{ icon: 'github', link: github }],
  },
})
