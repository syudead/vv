// Builds the ja/ tree the documentation site publishes under /ja/.
//
//   node translate/site.mjs
//
// Every published English document gets a page under ja/: its translation
// from translations/ja/, marked stale when the English source changed since,
// or the English text marked untranslated. The tree is rebuilt from scratch on
// every run, so a deleted or renamed source leaves no page behind.
import fs from 'node:fs'
import path from 'node:path'
import { defaultRoot, publishedSources, readTranslation, sha256, translationFile } from './ja.mjs'

// withFrontmatter adds keys to a page's front matter, creating it if needed.
export function withFrontmatter(markdown, fields) {
  const lines = Object.entries(fields).map(([k, v]) => `${k}: ${JSON.stringify(v)}`)
  if (markdown.startsWith('---\n')) return `---\n${lines.join('\n')}\n${markdown.slice(4)}`
  return `---\n${lines.join('\n')}\n---\n\n${markdown}`
}

// pageStatus decides what a ja/ page shows for one English source.
export function pageStatus(source, translation) {
  if (translation == null || !translation.fields.sourceHash) return 'untranslated'
  return translation.fields.sourceHash === sha256(source) ? 'current' : 'stale'
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

export function buildJaTree({ root, out = path.join(root, 'ja') }) {
  fs.rmSync(out, { recursive: true, force: true })
  const counts = { current: 0, stale: 0, untranslated: 0 }
  for (const rel of publishedSources(root)) {
    const source = fs.readFileSync(path.join(root, rel), 'utf8')
    const translation = readTranslation(translationFile(root, rel))
    const status = pageStatus(source, translation)
    counts[status]++
    const body = status === 'untranslated' ? source : translation.body
    const target = path.join(out, rel)
    fs.mkdirSync(path.dirname(target), { recursive: true })
    fs.writeFileSync(target, withFrontmatter(body, { translation: status, sourcePath: rel }))
  }
  fs.writeFileSync(path.join(out, 'index.md'), home)
  return counts
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  const counts = buildJaTree({ root: defaultRoot })
  console.log(`ja/: ${counts.current} translated, ${counts.stale} stale, ${counts.untranslated} untranslated`)
}
