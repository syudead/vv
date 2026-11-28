import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import {test} from 'node:test'
import {remark} from 'remark'
import remarkValidateLinks from 'remark-validate-links'
import remarkExplicitAnchors from './anchors.mjs'
import {
  englishProse,
  indexLinksSiblings,
  linksToSDDArtifact,
  noNumberedHeadings,
  noSDDArtifactLinks,
  noTemplatePlaceholders,
  numberedHeading
} from './rules.mjs'

const root = path.join(os.tmpdir(), 'vv-remark-rules')

async function lint(plugins, filePath, value, cwd = root) {
  const file = await remark().use(plugins).process({cwd, path: filePath, value})
  return file.messages.map((m) => m.line ?? null)
}

test('numberedHeading matches section numbers, not names', function () {
  const cases = {
    '4. Rules': true,
    '2.1. Reads': true,
    '4) Rules': true,
    '0. Migration': true,
    '2.1 API': false,
    Rules: false,
    'R-3: Glossary': false,
    '2026 release notes': false,
    'Step 1 — set up': false
  }
  for (const [text, want] of Object.entries(cases)) {
    assert.equal(numberedHeading.test(text), want, text)
  }
})

test('noNumberedHeadings reads the heading as written', async function () {
  const lines = await lint([noNumberedHeadings], 'a.md', '# 4. Rules\n\n## `1. code`\n')
  assert.deepEqual(lines, [1])
})

test('englishProse reports Japanese prose lines outside code', async function () {
  const body = 'Plain English.\n\nQuote `検索` inline.\n\n```text\n日本語のログ\n```\n\nこれは日本語です。\n\nMixed `ok` and 日本語.\n'
  assert.deepEqual(await lint([englishProse], 'a.md', body), [9, 11])
})

test('englishProse skips exempt files', async function () {
  const plugin = [englishProse, {exempt: ['.github/pull_request_template.md']}]
  assert.deepEqual(await lint([plugin], '.github/pull_request_template.md', '日本語'), [])
})

test('noTemplatePlaceholders reports markers in specs prose only', async function () {
  const body = 'Goal: [FEATURE]\n\nQuoted `[FEATURE]` marker.\n'
  const plugin = [noTemplatePlaceholders, {dir: 'specs/'}]
  assert.deepEqual(await lint([plugin], 'specs/active-feature/plan.md', body), [1])
  assert.deepEqual(await lint([plugin], 'specs/active-feature/assets/note.md', body), [])
  assert.deepEqual(await lint([plugin], 'docs/plan.md', body), [])
})

test('indexLinksSiblings reports a sibling document the index does not link', async function (t) {
  const cwd = fs.mkdtempSync(path.join(os.tmpdir(), 'vv-index-'))
  t.after(() => fs.rmSync(cwd, {recursive: true, force: true}))
  const dir = path.join(cwd, 'docs', 'design-docs')
  fs.mkdirSync(path.join(dir, 'nested.md'), {recursive: true})
  fs.writeFileSync(path.join(dir, 'linked.md'), '')
  fs.writeFileSync(path.join(dir, 'missing.md'), '')
  const plugin = [indexLinksSiblings, {indexes: ['docs/design-docs/index.md']}]
  const file = await remark()
    .use([plugin])
    .process({cwd, path: 'docs/design-docs/index.md', value: '[Linked](linked.md)\n'})
  assert.deepEqual(
    file.messages.map((m) => m.reason),
    ['docs/design-docs/missing.md is not linked from this index. Add it so the document can be found']
  )
})

test('linksToSDDArtifact recognizes local and repository links into specs/', function () {
  const source = path.join(root, 'docs', 'design-docs', 'example.md')
  const cases = {
    '../../specs/active-feature/plan.md#decisions': true,
    '/specs/active-feature/research.md': true,
    'https://github.com/syudead/vv/blob/main/specs/active-feature/plan.md': true,
    'https://example.org/specs/protocol/index.html': false,
    'catalog-organization.md': false,
    '../product-specs/spec-quality.md': false,
    '#specs': false
  }
  for (const [target, want] of Object.entries(cases)) {
    assert.equal(linksToSDDArtifact(root, source, target), want, target)
  }
})

test('noSDDArtifactLinks checks permanent documents only', async function () {
  const body = 'See [plan](../specs/active-feature/plan.md).\n'
  assert.deepEqual(await lint([noSDDArtifactLinks], 'docs/a.md', body), [1])
  assert.deepEqual(await lint([noSDDArtifactLinks], 'specs/README.md', body), [])
})

test('explicit and HTML anchors resolve as link targets', async function () {
  const body = '# Title {#custom}\n\n<a id="first"></a><a name="second"></a>\n\n[a](#custom) [b](#first) [c](#second) [d](#title-custom)\n'
  const validate = [remarkValidateLinks, {repository: false}]
  const lines = await lint([remarkExplicitAnchors, validate], 'a.md', body)
  assert.deepEqual(lines, [5])
})
