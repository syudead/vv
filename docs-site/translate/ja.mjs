// Japanese translations of the published English documents.
//
// A translation of <path> lives at translations/ja/<path>. The doc-translator
// subagent writes it; this tool pins its anchors, records which English
// version it translates, and checks it.
//
//   node docs-site/translate/ja.mjs stamp <path>...   pin anchors, record the source hash
//   node docs-site/translate/ja.mjs check             fail on a broken or orphaned translation,
//                                                     warn on a stale or missing one
//   node docs-site/translate/ja.mjs status            list sources whose translation is stale or missing
import crypto from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { execFileSync } from 'node:child_process'
import { headings, parse } from './markdown.mjs'
import { structureDiff } from './structure.mjs'

const here = path.dirname(new URL(import.meta.url).pathname)
export const defaultRoot = path.resolve(here, '../..')
export const translationsDir = 'translations/ja'
export const publishedRoots = ['docs', 'specs', 'ARCHITECTURE.md']

export const sha256 = (s) => crypto.createHash('sha256').update(s).digest('hex')

// publishedSources lists the English documents the site publishes.
export function publishedSources(root) {
  const out = execFileSync('git', ['-C', root, 'ls-files', '-z', '--', ...publishedRoots], { encoding: 'utf8' })
  return out
    .split('\0')
    .filter((p) => p.endsWith('.md'))
    .sort()
}

export function translationFile(root, rel) {
  return path.join(root, translationsDir, rel)
}

// readTranslation splits a translation into its front matter fields and body.
export function readTranslation(file) {
  if (!fs.existsSync(file)) return null
  const text = fs.readFileSync(file, 'utf8')
  const m = text.match(/^---\n([\s\S]*?)\n---\n\n?/)
  const fields = {}
  if (m) {
    for (const line of m[1].split('\n')) {
      const kv = line.match(/^(\w+):\s*(.*)$/)
      if (kv) fields[kv[1]] = kv[2].replace(/^"(.*)"$/, '$1')
    }
  }
  return { fields, body: m ? text.slice(m[0].length) : text }
}

const headingLine = /^(#{1,6})([ \t]+.*?)?[ \t]*(\{#[^}\s]+\})?[ \t]*$/

// pinAnchors gives every heading of a translation the anchor of the English
// heading at the same position, so links between Japanese pages keep working.
export function pinAnchors(source, body) {
  const want = headings(source)
  const lines = body.split('\n')
  let fence = false
  let i = 0
  for (let n = 0; n < lines.length; n++) {
    if (/^\s*(```|~~~)/.test(lines[n])) fence = !fence
    if (fence) continue
    const m = lines[n].match(headingLine)
    if (!m || !m[2]) continue
    if (i >= want.length) throw new Error(`the translation has more headings than its source (${want.length})`)
    lines[n] = `${m[1]}${m[2].replace(/[ \t]+$/, '')} {#${want[i].anchor}}`
    i++
  }
  if (i !== want.length) throw new Error(`the translation has ${i} headings, its source ${want.length}`)
  return lines.join('\n')
}

export function stamp(root, rel) {
  const source = fs.readFileSync(path.join(root, rel), 'utf8')
  const file = translationFile(root, rel)
  const t = readTranslation(file)
  if (!t) throw new Error(`${rel}: no translation at ${path.relative(root, file)}`)
  const body = pinAnchors(source, t.body).replace(/\s*$/, '\n')
  fs.writeFileSync(file, `---\nsource: ${rel}\nsourceHash: ${sha256(source)}\n---\n\n${body}`)
}

// checkOne returns the problems of one translation: errors break the page or
// lose content, warnings mean the translation is behind its source.
export function checkOne(root, rel) {
  const errors = []
  const warnings = []
  const t = readTranslation(translationFile(root, rel))
  const sourcePath = path.join(root, rel)
  if (!fs.existsSync(sourcePath)) {
    errors.push('its English source no longer exists; delete or move the translation')
    return { errors, warnings }
  }
  const source = fs.readFileSync(sourcePath, 'utf8')
  if (t.fields.source !== rel || !t.fields.sourceHash) {
    errors.push('missing front matter; run `node docs-site/translate/ja.mjs stamp ' + rel + '`')
    return { errors, warnings }
  }
  if (t.fields.sourceHash !== sha256(source)) {
    warnings.push('the English source changed since this translation; retranslate it')
    return { errors, warnings }
  }
  const diff = structureDiff(source, t.body)
  if (diff) errors.push(`structure differs from the source (${diff})`)
  const want = headings(source).map((h) => h.anchor)
  const got = headings(t.body)
  if (got.length === want.length) {
    for (const [i, h] of got.entries()) {
      if (!h.explicit || h.anchor !== want[i]) {
        errors.push(`heading at line ${h.line} must end with {#${want[i]}}; run stamp`)
        break
      }
    }
  }
  if (softBreakLines(t.body).length) {
    warnings.push(
      `a Japanese paragraph is broken across lines (line ${softBreakLines(t.body).join(', ')}); ` +
        'the break renders as a space, so keep each paragraph on one line',
    )
  }
  return { errors, warnings }
}

// softBreakLines returns the lines of paragraphs that hold Japanese and a line
// break. A browser renders the break as a space between Japanese words.
export function softBreakLines(body) {
  const lines = []
  ;(function visit(node) {
    if (node.type === 'paragraph') {
      const raw = body.slice(node.position.start.offset, node.position.end.offset)
      if (raw.includes('\n') && /[\u3040-\u30ff\u4e00-\u9fff]/.test(raw)) lines.push(node.position.start.line)
      return
    }
    for (const child of node.children ?? []) visit(child)
  })(parse(body))
  return lines
}

export function translatedSources(root) {
  const dir = path.join(root, translationsDir)
  if (!fs.existsSync(dir)) return []
  return fs
    .readdirSync(dir, { recursive: true, withFileTypes: true })
    .filter((e) => e.isFile() && e.name.endsWith('.md'))
    .map((e) => path.relative(dir, path.join(e.parentPath ?? e.path, e.name)).split(path.sep).join('/'))
    .sort()
}

export function checkAll(root) {
  const report = { errors: [], warnings: [], untranslated: [] }
  for (const rel of translatedSources(root)) {
    const { errors, warnings } = checkOne(root, rel)
    for (const e of errors) report.errors.push({ rel, message: e })
    for (const w of warnings) report.warnings.push({ rel, message: w })
  }
  const translated = new Set(translatedSources(root))
  for (const rel of publishedSources(root)) if (!translated.has(rel)) report.untranslated.push(rel)
  return report
}

function main(argv) {
  const [command, ...args] = argv
  const root = defaultRoot
  const annotate = (level, rel, message) =>
    process.env.GITHUB_ACTIONS
      ? console.log(`::${level} file=${translationsDir}/${rel}::${message}`)
      : console.log(`${level}: ${translationsDir}/${rel}: ${message}`)

  if (command === 'stamp') {
    for (const rel of args) {
      stamp(root, rel)
      const { errors } = checkOne(root, rel)
      for (const e of errors) annotate('error', rel, e)
      if (errors.length) process.exitCode = 1
    }
    return
  }
  if (command === 'check') {
    const r = checkAll(root)
    for (const { rel, message } of r.errors) annotate('error', rel, message)
    for (const { rel, message } of r.warnings) annotate('warning', rel, message)
    console.log(
      `${translatedSources(root).length} translations: ${r.errors.length} errors, ${r.warnings.length} warnings, ` +
        `${r.untranslated.length} published documents without a translation`,
    )
    if (r.errors.length) process.exitCode = 1
    return
  }
  if (command === 'status') {
    const r = checkAll(root)
    for (const { rel } of r.warnings) console.log(`stale ${rel}`)
    for (const rel of r.untranslated) console.log(`missing ${rel}`)
    return
  }
  console.error('usage: ja.mjs stamp <path>... | check | status')
  process.exitCode = 2
}

if (process.argv[1] === new URL(import.meta.url).pathname) main(process.argv.slice(2))
