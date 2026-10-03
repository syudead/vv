import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { execFileSync } from 'node:child_process'
import { translateTree } from './translate.mjs'
import { fakeEngine } from './engine.mjs'
import { buildJaTree, pageStatus } from './site.mjs'

const model = { family: 'hy', repo: 'test', file: 'test.gguf' }
const glossary = [{ en: 'tentative tag', ja: '仮のタグ' }]

function repo(files) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'vv-translate-'))
  execFileSync('git', ['init', '-q', root])
  write(root, files)
  return root
}

function write(root, files) {
  for (const [rel, body] of Object.entries(files)) {
    const p = path.join(root, rel)
    fs.mkdirSync(path.dirname(p), { recursive: true })
    if (body === null) fs.rmSync(p)
    else fs.writeFileSync(p, body)
  }
  execFileSync('git', ['-C', root, 'add', '-A'])
}

const base = {
  'docs/a.md': '# Alpha\n\nFirst paragraph.\n\nSecond paragraph.\n',
  'specs/b.md': '# Beta\n\nConfirm each tentative tag.\n',
  'scripts/sddguard/japanese-pending.txt': '# pending\nspecs/c.md\n',
  'specs/c.md': '# 日本語の文書\n',
  'web/README.md': '# Not published\n',
}

test('translates published sources and skips pending ones', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  const engine = fakeEngine()
  const report = await translateTree({ root, out, engine, model, glossary })
  assert.deepEqual(report.translated, ['docs/a.md', 'specs/b.md'])
  assert.ok(!fs.existsSync(path.join(out, 'specs/c.md')))
  assert.ok(!fs.existsSync(path.join(out, 'web/README.md')))
  assert.match(fs.readFileSync(path.join(out, 'specs/b.md'), 'utf8'), /仮のタグ/)
})

test('a changed source sends only its changed segments', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  await translateTree({ root, out, engine: fakeEngine(), model, glossary })
  write(root, { 'docs/a.md': '# Alpha\n\nFirst paragraph.\n\nA new second paragraph.\n' })
  const engine = fakeEngine()
  const report = await translateTree({ root, out, engine, model, glossary })
  assert.deepEqual(report.translated, ['docs/a.md'])
  assert.equal(report.unchanged, 1)
  assert.equal(engine.calls, 1)
})

test('an unchanged tree sends nothing', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  await translateTree({ root, out, engine: fakeEngine(), model, glossary })
  const engine = fakeEngine()
  const report = await translateTree({ root, out, engine, model, glossary })
  assert.equal(engine.calls, 0)
  assert.equal(report.translated.length, 0)
})

test('a deleted source removes its translation, metadata and memory', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  await translateTree({ root, out, engine: fakeEngine(), model, glossary })
  write(root, { 'docs/a.md': null })
  const report = await translateTree({ root, out, engine: fakeEngine(), model, glossary })
  assert.deepEqual(report.removed, ['docs/a.md'])
  for (const p of ['docs/a.md', '.meta/docs/a.md.json', '.memory/docs/a.md.json']) {
    assert.ok(!fs.existsSync(path.join(out, p)), p)
  }
})

test('a glossary change retranslates only segments with that term', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  await translateTree({ root, out, engine: fakeEngine(), model, glossary })
  const engine = fakeEngine()
  await translateTree({ root, out, engine, model, glossary: [{ en: 'tentative tag', ja: '暫定タグ' }] })
  assert.equal(engine.calls, 1)
  assert.match(fs.readFileSync(path.join(out, 'specs/b.md'), 'utf8'), /暫定タグ/)
})

test('a model change retranslates everything', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  await translateTree({ root, out, engine: fakeEngine(), model, glossary })
  const engine = fakeEngine()
  await translateTree({ root, out, engine, model: { ...model, file: 'other.gguf' }, glossary })
  assert.equal(engine.calls, 5)
})

test('a segment that loses its tags twice stays English and is reported', async () => {
  const root = repo({ ...base, 'docs/a.md': '# Alpha\n\nRead `code` here.\n' })
  const out = path.join(root, '.docs-ja')
  const report = await translateTree({ root, out, engine: fakeEngine({ dropTags: true }), model, glossary })
  assert.equal(report.englishSegments.length, 1)
  assert.match(fs.readFileSync(path.join(out, 'docs/a.md'), 'utf8'), /Read `code` here\./)
})

test('an engine error keeps the old translation and reports the file', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  await translateTree({ root, out, engine: fakeEngine(), model, glossary })
  const before = fs.readFileSync(path.join(out, 'docs/a.md'), 'utf8')
  write(root, { 'docs/a.md': '# Alpha\n\nChanged.\n' })
  const report = await translateTree({ root, out, engine: fakeEngine({ fail: true }), model, glossary })
  assert.equal(report.failed.length, 1)
  assert.equal(fs.readFileSync(path.join(out, 'docs/a.md'), 'utf8'), before)
})

test('the time budget leaves the rest for the next run', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  let t = 0
  const report = await translateTree({ root, out, engine: fakeEngine(), model, glossary, budgetMs: 5, now: () => (t += 10) })
  assert.deepEqual(report.translated, [])
  assert.deepEqual(report.remaining, ['docs/a.md', 'specs/b.md'])
})

test('the site marks stale and untranslated pages', async () => {
  const root = repo(base)
  const ja = path.join(root, '.docs-ja')
  await translateTree({ root, out: ja, engine: fakeEngine(), model, glossary })
  write(root, { 'docs/a.md': '# Alpha\n\nChanged.\n', 'docs/new.md': '# New\n' })
  const counts = buildJaTree({ root, ja })
  assert.deepEqual(counts, { current: 1, stale: 1, untranslated: 1 })
  assert.match(fs.readFileSync(path.join(root, 'ja/docs/a.md'), 'utf8'), /^---\ntranslation: "stale"/)
  assert.match(fs.readFileSync(path.join(root, 'ja/docs/new.md'), 'utf8'), /translation: "untranslated"[\s\S]*# New/)
  assert.equal(pageStatus('x', null, null), 'untranslated')
})

test('shards split the sources and a dry run calls no engine', async () => {
  const root = repo(base)
  const out = path.join(root, '.docs-ja')
  const engine = fakeEngine()
  const dry = await translateTree({ root, out, engine, model, glossary, dryRun: true })
  assert.deepEqual(dry.pending, ['docs/a.md', 'specs/b.md'])
  assert.equal(engine.calls, 0)
  const first = await translateTree({ root, out, engine: fakeEngine(), model, glossary, shard: 0, shards: 2 })
  const second = await translateTree({ root, out, engine: fakeEngine(), model, glossary, shard: 1, shards: 2 })
  assert.deepEqual(first.translated, ['docs/a.md'])
  assert.deepEqual(second.translated, ['specs/b.md'])
})
