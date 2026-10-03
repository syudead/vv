// Builds the ja/ tree the documentation site publishes under /ja/.
//
//   node translate/site.mjs [--root <repo>] [--ja <docs-ja checkout>]
//
// Every published English document gets a page under ja/: its translation
// when docs-ja has one, marked stale when the English source changed since,
// or the English text marked untranslated. The tree is rebuilt from scratch on
// every run, so a deleted or renamed source leaves no page behind.
import crypto from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { parseArgs } from 'node:util'
import { publishedSources } from './translate.mjs'

const here = path.dirname(new URL(import.meta.url).pathname)
const sha256 = (s) => crypto.createHash('sha256').update(s).digest('hex')

// withFrontmatter adds keys to a page's front matter, creating it if needed.
export function withFrontmatter(markdown, fields) {
  const lines = Object.entries(fields).map(([k, v]) => `${k}: ${JSON.stringify(v)}`)
  if (markdown.startsWith('---\n')) return `---\n${lines.join('\n')}\n${markdown.slice(4)}`
  return `---\n${lines.join('\n')}\n---\n\n${markdown}`
}

// pageStatus decides what a ja/ page shows for one English source.
export function pageStatus(source, translation, meta) {
  if (translation == null || meta == null) return 'untranslated'
  return meta.sourceHash === sha256(source) ? 'current' : 'stale'
}

const home = `---
layout: home

hero:
  name: vv docs
  text: 設計文書・仕様・手順
  tagline: 英語の文書から自動で作った和訳です。正本は英語版です。
  actions:
    - theme: brand
      text: 設計文書
      link: /ja/docs/design-docs/
    - theme: alt
      text: English
      link: /
---
`

export function buildJaTree({ root, ja, out = path.join(root, 'ja') }) {
  fs.rmSync(out, { recursive: true, force: true })
  const counts = { current: 0, stale: 0, untranslated: 0 }
  for (const rel of publishedSources(root)) {
    const source = fs.readFileSync(path.join(root, rel), 'utf8')
    const read = (p) => (ja && fs.existsSync(p) ? fs.readFileSync(p, 'utf8') : null)
    const translation = read(path.join(ja ?? '', rel))
    const metaText = read(path.join(ja ?? '', '.meta', rel + '.json'))
    const status = pageStatus(source, translation, metaText && JSON.parse(metaText))
    counts[status]++
    const body = status === 'untranslated' ? source : translation
    const target = path.join(out, rel)
    fs.mkdirSync(path.dirname(target), { recursive: true })
    fs.writeFileSync(target, withFrontmatter(body, { translation: status, sourcePath: rel }))
  }
  fs.writeFileSync(path.join(out, 'index.md'), home)
  return counts
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  const { values } = parseArgs({
    options: {
      root: { type: 'string', default: path.resolve(here, '../..') },
      ja: { type: 'string', default: path.resolve(here, '../../.docs-ja') },
    },
  })
  const ja = fs.existsSync(values.ja) ? values.ja : null
  const counts = buildJaTree({ root: values.root, ja })
  console.log(`ja/: ${counts.current} translated, ${counts.stale} stale, ${counts.untranslated} untranslated`)
}
