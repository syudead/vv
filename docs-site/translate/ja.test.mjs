import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { execFileSync } from 'node:child_process'
import { stamp, checkOne, checkAll, pinAnchors, readTranslation, softBreakLines } from './ja.mjs'
import { structureDiff } from './structure.mjs'
import { buildJaTree } from './site.mjs'

const source = `# Where translations live

Read [the guide](../how-to/a.md) before you change \`internal/httpapi\`.

| Option | Verdict |
| --- | --- |
| **Chosen** | Kept |

\`\`\`mermaid
flowchart LR
  push[push to main] -->|commit| ja[(branch docs-ja)]
\`\`\`

## Duplicate
## Duplicate
`

const translation = `# 翻訳の置き場所

\`internal/httpapi\` を変える前に[手順](../how-to/a.md)を読む。

| 案 | 判断 |
| --- | --- |
| **採用** | 残す |

\`\`\`mermaid
flowchart LR
  push[main への push] -->|コミット| ja[(docs-ja ブランチ)]
\`\`\`

## 重複
## 重複
`

function repo(files) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'vv-ja-'))
  execFileSync('git', ['init', '-q', root])
  for (const [rel, body] of Object.entries(files)) {
    const p = path.join(root, rel)
    fs.mkdirSync(path.dirname(p), { recursive: true })
    fs.writeFileSync(p, body)
  }
  execFileSync('git', ['-C', root, 'add', '-A'])
  return root
}

test('stamp pins English anchors and records the source', () => {
  const root = repo({ 'docs/a.md': source, 'translations/ja/docs/a.md': translation })
  stamp(root, 'docs/a.md')
  const t = readTranslation(path.join(root, 'translations/ja/docs/a.md'))
  assert.equal(t.fields.source, 'docs/a.md')
  assert.match(t.fields.sourceHash, /^[0-9a-f]{64}$/)
  assert.match(t.body, /^# 翻訳の置き場所 \{#where-translations-live\}$/m)
  assert.match(t.body, /^## 重複 \{#duplicate\}$/m)
  assert.match(t.body, /^## 重複 \{#duplicate-1\}$/m)
  assert.deepEqual(checkOne(root, 'docs/a.md'), { errors: [], warnings: [] })
})

test('stamp refuses a translation whose headings do not match', () => {
  assert.throws(() => pinAnchors(source, translation.replace('## 重複\n## 重複\n', '## 重複\n')), /headings/)
})

test('a translated Mermaid diagram keeps its structure', () => {
  assert.equal(structureDiff(source, translation), null)
})

test('check reports a lost table row, a changed link and a changed code span', () => {
  const root = repo({ 'docs/a.md': source, 'translations/ja/docs/a.md': translation })
  stamp(root, 'docs/a.md')
  const file = path.join(root, 'translations/ja/docs/a.md')
  const good = fs.readFileSync(file, 'utf8')
  for (const broken of [
    good.replace('| **採用** | 残す |\n', ''),
    good.replace('../how-to/a.md', '../how-to/b.md'),
    good.replace('`internal/httpapi`', '`internal/api`'),
  ]) {
    fs.writeFileSync(file, broken)
    assert.equal(checkOne(root, 'docs/a.md').errors.length, 1, broken)
  }
})

test('a changed source makes the translation stale, not broken', () => {
  const root = repo({ 'docs/a.md': source, 'translations/ja/docs/a.md': translation })
  stamp(root, 'docs/a.md')
  fs.writeFileSync(path.join(root, 'docs/a.md'), source + '\nNew paragraph.\n')
  const r = checkOne(root, 'docs/a.md')
  assert.equal(r.errors.length, 0)
  assert.match(r.warnings[0], /changed/)
})

test('a translation without a source is an error', () => {
  const root = repo({ 'docs/a.md': source, 'translations/ja/docs/gone.md': translation })
  const r = checkAll(root)
  assert.deepEqual(r.errors.map((e) => e.rel), ['gone.md'.replace(/^/, 'docs/')])
})

test('a missing translation is listed, not an error', () => {
  const root = repo({ 'docs/a.md': source, 'specs/b.md': '# B\n' })
  const r = checkAll(root)
  assert.deepEqual(r.untranslated, ['docs/a.md', 'specs/b.md'])
  assert.equal(r.errors.length, 0)
})

test('a Japanese paragraph broken across lines is reported', () => {
  assert.deepEqual(softBreakLines('一行目の文と\n二行目の文。\n\n一行だけの段落。\n'), [1])
  assert.deepEqual(softBreakLines('English\nlines are fine.\n'), [])
})

test('the site marks current, stale and untranslated pages', () => {
  const root = repo({
    'docs/a.md': source,
    'docs/b.md': '# B\n\nText.\n',
    'docs/c.md': '# C\n',
    'translations/ja/docs/a.md': translation,
    'translations/ja/docs/b.md': '# ビー\n\n本文。\n',
  })
  stamp(root, 'docs/a.md')
  stamp(root, 'docs/b.md')
  fs.writeFileSync(path.join(root, 'docs/b.md'), '# B\n\nChanged.\n')
  assert.deepEqual(buildJaTree({ root }), { current: 1, stale: 1, untranslated: 1 })
  assert.match(fs.readFileSync(path.join(root, 'ja/docs/a.md'), 'utf8'), /^---\ntranslation: "current"[\s\S]*翻訳の置き場所/)
  assert.match(fs.readFileSync(path.join(root, 'ja/docs/c.md'), 'utf8'), /translation: "untranslated"[\s\S]*# C/)
})

test('state and sequence diagram labels may be translated, ids may not', () => {
  const en = '```mermaid\nstateDiagram-v2\n  [*] --> Direct: direct playback\n  Direct --> Waiting: switch\n  Waiting: Track removed\n```\n\n```mermaid\nsequenceDiagram\n  A->>B: request\n```\n'
  const ja = '```mermaid\nstateDiagram-v2\n  [*] --> Direct: 直接再生\n  Direct --> Waiting: 切り替え\n  Waiting: トラックを外す\n```\n\n```mermaid\nsequenceDiagram\n  A->>B: 要求\n```\n'
  assert.equal(structureDiff(en, ja), null)
  assert.notEqual(structureDiff(en, ja.replace('Direct --> Waiting', 'Direct --> Wait')), null)
  assert.notEqual(structureDiff(en, ja.replace('A->>B', 'A->>C')), null)
})
