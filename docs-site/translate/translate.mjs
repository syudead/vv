// Translates the published English documents into the docs-ja tree.
//
//   node translate/translate.mjs --root <repo> --out <docs-ja checkout>
//        [--engine llama|fake] [--server http://127.0.0.1:8080]
//        [--budget-seconds 18000] [--summary <file>]
//
// For each source it keeps <out>/<path> (the translation),
// <out>/.meta/<path>.json (hashes the site compares) and
// <out>/.memory/<path>.json (segment translations, so an unchanged segment is
// never sent to the model again). Translations whose source is gone are
// removed. The process exits non-zero when any file failed; reaching the time
// budget is not a failure.
import crypto from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { execFileSync } from 'node:child_process'
import { parseArgs } from 'node:util'
import { extractSegments, restore, assemble, stripTags } from './segments.mjs'
import { structureDiff } from './structure.mjs'
import { loadGlossary, termsFor, missedTerms } from './glossary.mjs'
import { llamaEngine, fakeEngine } from './engine.mjs'

const here = path.dirname(new URL(import.meta.url).pathname)

export const publishedRoots = ['docs', 'specs', 'ARCHITECTURE.md']
export const pendingList = 'scripts/sddguard/japanese-pending.txt'

const sha256 = (s) => crypto.createHash('sha256').update(s).digest('hex')

// publishedSources lists the English documents the site publishes. Files
// still waiting for their English rewrite have no English source yet.
export function publishedSources(root, { includePending = false } = {}) {
  const out = execFileSync('git', ['-C', root, 'ls-files', '-z', '--', ...publishedRoots], { encoding: 'utf8' })
  const pendingFile = path.join(root, pendingList)
  const pending = new Set(
    fs.existsSync(pendingFile)
      ? fs
          .readFileSync(pendingFile, 'utf8')
          .split('\n')
          .map((l) => l.trim())
          .filter((l) => l && !l.startsWith('#'))
      : [],
  )
  return out
    .split('\0')
    .filter((p) => p.endsWith('.md') && (includePending || !pending.has(p)))
    .sort()
}

function readJSON(file, fallback) {
  try {
    return JSON.parse(fs.readFileSync(file, 'utf8'))
  } catch {
    return fallback
  }
}

function writeFile(file, content) {
  fs.mkdirSync(path.dirname(file), { recursive: true })
  fs.writeFileSync(file, content)
}

function removeFile(file) {
  fs.rmSync(file, { force: true })
}

// shard/shards split the sources across parallel jobs; only an unsharded run
// (or the job that merges the shards) prunes. dryRun lists the sources that
// need work without calling the engine.
export async function translateTree({
  root,
  out,
  engine,
  model,
  glossary,
  budgetMs = Infinity,
  now = Date.now,
  shard = 0,
  shards = 1,
  prune = shards === 1,
  dryRun = false,
  only = null,
}) {
  const started = now()
  const modelHash = sha256(JSON.stringify(model))
  const glossaryHash = sha256(JSON.stringify(glossary))
  const all = publishedSources(root)
  // only limits the run to named sources (a preview); it never prunes.
  const sources = only ? all.filter((rel) => only.includes(rel)) : all
  if (only) prune = false
  const report = {
    translated: [],
    unchanged: 0,
    pending: [],
    failed: [],
    englishSegments: [],
    termMisses: [],
    remaining: [],
    removed: [],
  }

  for (const [index, rel] of sources.entries()) {
    if (index % shards !== shard) continue
    const source = fs.readFileSync(path.join(root, rel), 'utf8')
    const sourceHash = sha256(source)
    const metaFile = path.join(out, '.meta', rel + '.json')
    const meta = readJSON(metaFile, null)
    if (meta && meta.sourceHash === sourceHash && meta.modelHash === modelHash && meta.glossaryHash === glossaryHash) {
      report.unchanged++
      continue
    }
    if (dryRun) {
      report.pending.push(rel)
      continue
    }
    if (now() - started > budgetMs) {
      report.remaining.push(rel)
      continue
    }

    const memoryFile = path.join(out, '.memory', rel + '.json')
    const memory = readJSON(memoryFile, {})
    const used = {}
    const segments = extractSegments(source)
    const translations = []
    let english = 0
    try {
      for (const seg of segments) {
        const terms = termsFor(glossary, stripTags(seg.text))
        const key = sha256(JSON.stringify([seg.kind, seg.text, terms, modelHash]))
        let value = memory[key]
        if (value === undefined) {
          value = null
          // Soft line breaks inside a paragraph are not content; the model
          // sees one line. A restored segment must keep the inline code, links
          // and emphasis of its source, or it is retried.
          const shown = seg.text.replace(/\s*\n\s*/g, ' ')
          for (let attempt = 0; attempt < 3 && value === null; attempt++) {
            const result = await engine.translate(shown, terms, { attempt })
            value = restore(seg, result.text)
            if (value !== null && seg.kind !== 'mermaid') {
              const original = source.slice(seg.start, seg.end)
              if (structureDiff(original, value.replace(/ \{#[^}\s]+\}$/, ''))) value = null
            }
          }
          if (value === null) {
            english++
            report.englishSegments.push(`${rel}: ${stripTags(seg.text).slice(0, 80)}`)
          } else {
            for (const miss of missedTerms(terms, value)) report.termMisses.push(`${rel}: ${miss.en} → ${miss.ja}`)
          }
        }
        if (value !== null) used[key] = value
        translations.push(value)
      }
    } catch (err) {
      report.failed.push(`${rel}: ${err.message}`)
      continue
    }

    const translated = assemble(source, segments, translations)
    const diff = structureDiff(source, translated)
    if (diff) {
      report.failed.push(`${rel}: structure changed (${diff})`)
      continue
    }
    writeFile(path.join(out, rel), translated)
    writeFile(memoryFile, JSON.stringify(used, null, 1) + '\n')
    writeFile(metaFile, JSON.stringify({ sourceHash, modelHash, glossaryHash, englishSegments: english }, null, 1) + '\n')
    report.translated.push(rel)
  }

  if (prune && !dryRun) report.removed = pruneTree({ root, out, sources: all })
  return report
}

// pruneTree removes translations, metadata and memory whose source is gone,
// and returns the translations it removed.
export function pruneTree({ root, out, sources = publishedSources(root) }) {
  const keep = new Set(sources)
  const removed = []
  for (const file of walk(out)) {
    const rel = path.relative(out, file).split(path.sep).join('/')
    if (rel === '.git' || rel === 'README.md') continue
    const src = rel.replace(/^\.(meta|memory)\//, '').replace(/\.json$/, '')
    if (!keep.has(src)) {
      removeFile(file)
      if (!rel.startsWith('.')) removed.push(rel)
    }
  }
  return removed
}

function* walk(dir) {
  if (!fs.existsSync(dir)) return
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === '.git') continue
    const p = path.join(dir, entry.name)
    if (entry.isDirectory()) yield* walk(p)
    else yield p
  }
}

export function summary(report) {
  const list = (items) => (items.length ? items.map((i) => `- ${i}`).join('\n') : '- none')
  return [
    '## Japanese translation',
    '',
    '| Result | Files |',
    '| --- | --- |',
    `| Translated | ${report.translated.length} |`,
    `| Unchanged | ${report.unchanged} |`,
    `| Removed | ${report.removed.length} |`,
    `| Failed | ${report.failed.length} |`,
    `| Left for the next run (time budget) | ${report.remaining.length} |`,
    '',
    '### Failed',
    list(report.failed),
    '',
    '### Segments kept in English',
    list(report.englishSegments),
    '',
    '### Glossary misses',
    list(report.termMisses),
    '',
    '### Left for the next run',
    list(report.remaining),
    '',
  ].join('\n')
}

async function main() {
  const { values } = parseArgs({
    options: {
      root: { type: 'string', default: path.resolve(here, '../..') },
      out: { type: 'string' },
      engine: { type: 'string', default: 'llama' },
      server: { type: 'string', default: 'http://127.0.0.1:8080' },
      'budget-seconds': { type: 'string', default: '0' },
      summary: { type: 'string' },
      shard: { type: 'string', default: '0' },
      shards: { type: 'string', default: '1' },
      prune: { type: 'boolean' },
      'dry-run': { type: 'boolean', default: false },
      'prune-only': { type: 'boolean', default: false },
      only: { type: 'string' },
    },
  })
  if (!values.out) throw new Error('--out is required')
  const model = readJSON(path.join(here, 'model.json'), null)
  const glossary = loadGlossary(path.join(here, 'glossary.tsv'))
  const engine = values.engine === 'fake' ? fakeEngine() : llamaEngine({ server: values.server, family: model.family })
  const budget = Number(values['budget-seconds'])
  const shards = Number(values.shards)
  if (values['prune-only']) {
    const removed = pruneTree({ root: values.root, out: values.out })
    for (const rel of removed) console.log(`removed ${rel}`)
    return
  }
  const report = await translateTree({
    root: values.root,
    out: values.out,
    engine,
    model,
    glossary,
    budgetMs: budget > 0 ? budget * 1000 : Infinity,
    shard: Number(values.shard),
    shards,
    prune: values.prune ?? shards === 1,
    dryRun: values['dry-run'],
    only: values.only ? values.only.split(',') : null,
  })
  if (values['dry-run']) {
    // One line per source that needs work, for the workflow to count.
    for (const rel of report.pending) console.log(rel)
    return
  }
  const text = summary(report)
  console.log(text)
  if (values.summary) fs.appendFileSync(values.summary, text)
  if (report.failed.length) process.exitCode = 1
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  await main()
}
