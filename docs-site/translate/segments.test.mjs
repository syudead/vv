import { test } from 'node:test'
import assert from 'node:assert/strict'
import { extractSegments, restore, assemble } from './segments.mjs'
import { structureDiff } from './structure.mjs'

const doc = `# Where translations live

Read [ARCHITECTURE.md](../ARCHITECTURE.md) before you change the \`internal/httpapi\` package.
See ![the scan status](status.png) and **all** files.

| Option | Verdict |
| --- | --- |
| **Chosen** | Rejected: slow \\| big |

- A list item that names docs/how-to as a path.

\`\`\`go
// This comment is code and stays English.
func main() {}
\`\`\`

\`\`\`mermaid
flowchart LR
  push[push to main] -->|commit| ja[(branch docs-ja)]
\`\`\`

## Duplicate
## Duplicate
`

// fake translation: mark every run of words outside tags, keep tags.
const fake = (seg) => restore(seg, seg.text.replace(/(^|>)([^<]+)/g, (_, a, b) => `${a}訳${b}`))

test('translation keeps headings, tables, lists, links and code', () => {
  const segs = extractSegments(doc)
  const out = assemble(doc, segs, segs.map(fake))
  assert.equal(structureDiff(doc, out), null)
  assert.match(out, /\[訳ARCHITECTURE\.md\]\(\.\.\/ARCHITECTURE\.md\)|\[ARCHITECTURE\.md\]\(\.\.\/ARCHITECTURE\.md\)/)
  assert.match(out, /`internal\/httpapi`/)
  assert.match(out, /\]\(status\.png\)/)
  assert.match(out, /\/\/ This comment is code and stays English\./)
  assert.match(out, /docs\/how-to/)
})

test('headings carry their English anchor', () => {
  const segs = extractSegments(doc)
  const out = assemble(doc, segs, segs.map(fake))
  assert.match(out, /^# 訳Where translations live \{#where-translations-live\}$/m)
  assert.match(out, /^## 訳Duplicate \{#duplicate\}$/m)
  assert.match(out, /^## 訳Duplicate \{#duplicate-1\}$/m)
})

test('an explicit anchor is kept and not duplicated', () => {
  const src = '## Storage {#storage}\n'
  const segs = extractSegments(src)
  assert.equal(segs.length, 1)
  assert.equal(assemble(src, segs, segs.map(fake)), '## 訳Storage {#storage}\n')
})

test('Mermaid changes only in its labels', () => {
  const segs = extractSegments(doc)
  const labels = segs.filter((s) => s.kind === 'mermaid').map((s) => s.text)
  assert.deepEqual(labels, ['push to main', 'commit', 'branch docs-ja'])
  const out = assemble(doc, segs, segs.map(fake))
  assert.match(out, /push\[訳push to main\] -->\|訳commit\| ja\[\(訳branch docs-ja\)\]/)
})

test('an output that loses a tag is rejected', () => {
  const [, para] = extractSegments(doc)
  assert.equal(restore(para, para.text.replace(/<\/?s1>/g, '')), null)
  assert.equal(restore(para, para.text.replace('<s2>', '<s2><s2>')), null)
})

test('a table cell escapes a pipe the model introduced', () => {
  const segs = extractSegments('| A |\n| --- |\n| one cell |\n')
  const cell = segs.find((s) => s.text === 'one cell')
  assert.equal(restore(cell, '一つ | セル'), '一つ \\| セル')
})

test('a segment without translation keeps its English text', () => {
  const segs = extractSegments(doc)
  const out = assemble(doc, segs, segs.map(() => null))
  assert.equal(out.replace(/ \{#[^}]+\}/g, ''), doc)
})

test('Mermaid labels translated into Japanese keep the structure', () => {
  const src = '```mermaid\nflowchart LR\n  push[push to main] -->|commit| ja[(branch docs-ja)]\n```\n'
  const segs = extractSegments(src)
  const out = assemble(src, segs, segs.map(() => 'ブランチへの反映'))
  assert.equal(structureDiff(src, out), null)
  assert.match(out, /push\[ブランチへの反映\] -->\|ブランチへの反映\| ja\[\(ブランチへの反映\)\]/)
})
