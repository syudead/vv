// Splits an English Markdown document into translatable segments and puts
// translations back in place.
//
// A segment is the inline content of one block (paragraph, heading, table
// cell) or one Mermaid label. Inline code, link destinations, HTML and
// identifier-like words become numbered <sN> tags, so the model never sees
// them and they come back byte for byte. The translated document is the source
// with each segment's span replaced, so block structure cannot change.
import { unified } from 'unified'
import remarkParse from 'remark-parse'
import remarkGfm from 'remark-gfm'
import remarkFrontmatter from 'remark-frontmatter'
import GithubSlugger from 'github-slugger'

const parser = unified().use(remarkParse).use(remarkGfm).use(remarkFrontmatter, ['yaml', 'toml'])

export function parse(markdown) {
  return parser.parse(markdown)
}

// Words that are identifiers rather than English: paths, file names,
// snake_case, dotted names and flags. They are protected like inline code.
const identifier =
  /(?:[A-Za-z0-9_.-]+\/[A-Za-z0-9_./-]*[A-Za-z0-9_-]|\b[A-Za-z0-9-]+\.(?:md|mdx|ts|tsx|js|mjs|go|ya?ml|json|toml|sql|sh|css|html)\b|\b[a-z0-9]+(?:_[a-z0-9]+)+\b|\b[a-z]+[A-Z][A-Za-z0-9]*\b|(?<![\w-])--?[a-z][a-z0-9-]+)/g

const explicitAnchor = /\s*\{#[^}\s]+\}\s*$/
const hasWords = /[A-Za-z]{2,}/

const atomicTypes = new Set(['inlineCode', 'html', 'break', 'footnoteReference', 'inlineMath'])
const containerTypes = new Set(['emphasis', 'strong', 'delete', 'link', 'linkReference'])
const imageTypes = new Set(['image', 'imageReference'])

function offsets(node) {
  return [node.position.start.offset, node.position.end.offset]
}

// buildSegment turns the inline children of a block into tagged text. It
// returns the text and the tag table restore() needs.
function buildSegment(source, children, start, end) {
  const tags = new Map()
  let next = 1
  let cursor = start
  let text = ''

  function addText(raw) {
    let out = ''
    let last = 0
    for (const m of raw.matchAll(identifier)) {
      const id = next++
      tags.set(id, { kind: 'atomic', value: m[0] })
      out += raw.slice(last, m.index) + `<s${id}>${m[0]}</s${id}>`
      last = m.index + m[0].length
    }
    return out + raw.slice(last)
  }

  function walk(nodes, until) {
    let out = ''
    for (const node of nodes) {
      const [s, e] = offsets(node)
      if (s > cursor) out += addText(source.slice(cursor, s))
      if (node.type === 'text') {
        out += addText(source.slice(s, e))
      } else if (atomicTypes.has(node.type) || isAutolink(source, node)) {
        // The model sees inline code without its backticks, so it has no
        // Markdown to copy; restore() puts the original source back.
        const id = next++
        tags.set(id, { kind: 'atomic', value: source.slice(s, e) })
        const shown = node.type === 'inlineCode' ? node.value : source.slice(s, e)
        out += `<s${id}>${shown}</s${id}>`
      } else if (imageTypes.has(node.type)) {
        const raw = source.slice(s, e)
        const close = raw.lastIndexOf('](') >= 0 ? raw.lastIndexOf('](') : raw.lastIndexOf('][')
        const id = next++
        if (close < 2 || !hasWords.test(node.alt ?? '')) {
          tags.set(id, { kind: 'atomic', value: raw })
          out += `<s${id}>${raw}</s${id}>`
        } else {
          tags.set(id, { kind: 'wrap', prefix: '![', suffix: raw.slice(close) })
          out += `<s${id}>${raw.slice(2, close)}</s${id}>`
        }
      } else if (containerTypes.has(node.type) && node.children?.length) {
        const id = next++
        const [cs] = offsets(node.children[0])
        const [, ce] = offsets(node.children[node.children.length - 1])
        tags.set(id, { kind: 'wrap', prefix: source.slice(s, cs), suffix: source.slice(ce, e) })
        cursor = cs
        const inner = walk(node.children, ce)
        out += `<s${id}>${inner}</s${id}>`
      } else {
        const id = next++
        tags.set(id, { kind: 'atomic', value: source.slice(s, e) })
        out += `<s${id}>${source.slice(s, e)}</s${id}>`
      }
      cursor = e
    }
    if (until > cursor) out += addText(source.slice(cursor, until))
    cursor = until
    return out
  }

  text = walk(children, end)
  return { text, tags }
}

function isAutolink(source, node) {
  if (node.type !== 'link') return false
  const raw = source.slice(...offsets(node))
  return !raw.startsWith('[')
}

// Mermaid labels: [text], ("text"), {text}, |edge text|, participant X as Y,
// and note ...: text. Node ids, arrows and keywords stay as they are.
const mermaidLabel =
  /\[\[?"?([^\]"\n]+?)"?\]?\]|\(\(?"?([^()"\n]+?)"?\)?\)|\{\{?"?([^{}"\n]+?)"?\}?\}|\|"?([^|"\n]+?)"?\||\bparticipant\s+\S+\s+as\s+([^\n]+)|\bnote\s+[^:\n]+:\s*([^\n]+)/g

// mermaidLabels returns [start, end] offsets of the translatable labels in a
// Mermaid body.
export function mermaidLabels(value) {
  const out = []
  for (const m of value.matchAll(mermaidLabel)) {
    const group = m.slice(1).findIndex((g) => g !== undefined)
    const captured = m[group + 1]
    if (!captured) continue
    // Shapes such as [(db)], [[sub]] and [/in/] wrap the text in extra
    // delimiters; they belong to the shape, not the label.
    const lead = captured.match(/^[\s([{/\\>]*/)[0].length
    const trail = captured.match(/[\s)\]}/\\]*$/)[0].length
    const label = captured.slice(lead, captured.length - trail)
    if (!label) continue
    const at = m.index + m[0].indexOf(captured) + lead
    out.push([at, at + label.length, label])
  }
  return out
}

// maskMermaid replaces every label with `_`, leaving node ids, arrows and
// keywords: what a translation must not change.
export function maskMermaid(value) {
  let out = value
  for (const [start, end] of mermaidLabels(value).reverse()) out = out.slice(0, start) + '_' + out.slice(end)
  return out
}

function mermaidSegments(source, node) {
  const [s] = offsets(node)
  const raw = source.slice(...offsets(node))
  const bodyStart = s + raw.indexOf('\n') + 1
  return mermaidLabels(node.value)
    .filter(([, , label]) => hasWords.test(label))
    .map(([start, end, label]) => ({
      kind: 'mermaid',
      start: bodyStart + start,
      end: bodyStart + end,
      text: label,
      tags: new Map(),
    }))
}

// extractSegments returns the segments of a document in source order. Each
// segment carries `context`: the document title, the section it sits in, and
// for a table cell its column headings and row, for a paragraph the paragraph
// before it. A short segment translated alone loses its meaning ("Kind" as a
// person); the context is shown to the model but never translated.
export function extractSegments(source) {
  const tree = parse(source)
  const slugger = new GithubSlugger()
  const segments = []
  let title = ''
  const trail = [] // heading texts by depth
  let previous = ''
  let table = null // { columns, row }

  function context(kind) {
    const lines = []
    if (title) lines.push(`Document: ${title}`)
    const section = trail.filter(Boolean).slice(title ? 1 : 0)
    if (section.length) lines.push(`Section: ${section.join(' > ')}`)
    if (kind === 'tableCell' && table) {
      lines.push(`Table columns: ${table.columns.join(' | ')}`)
      if (table.row) lines.push(`Row: ${table.row}`)
    }
    if (kind === 'paragraph' && previous) lines.push(`Previous paragraph: ${clip(previous)}`)
    return lines.join('\n')
  }

  function visit(node) {
    if (node.type === 'heading') {
      if (!node.children.length) return
      const [start] = offsets(node.children[0])
      let [, end] = offsets(node.children[node.children.length - 1])
      const raw = source.slice(start, end)
      const anchor = raw.match(explicitAnchor)
      const heading = plainText(node).replace(explicitAnchor, '').trim()
      let slug
      if (anchor) {
        end -= anchor[0].length
        slug = null // the heading already pins its anchor
      } else {
        slug = slugger.slug(plainText(node))
      }
      const built = buildSegment(source, node.children, start, end)
      const text = anchor ? built.text.replace(explicitAnchor, '') : built.text
      const tags = built.tags
      trail.length = node.depth - 1
      const ctx = context('heading')
      if (hasWords.test(stripTags(text))) {
        segments.push({ kind: 'heading', start, end, text, tags, slug, context: ctx })
      }
      trail[node.depth - 1] = heading
      if (node.depth === 1 && !title) title = heading
      previous = ''
      return
    }
    if (node.type === 'table') {
      const columns = node.children[0].children.map((cell) => plainText(cell).trim())
      for (const [i, row] of node.children.entries()) {
        table = { columns, row: i === 0 ? '' : plainText(row.children[0] ?? { children: [] }).trim() }
        for (const cell of row.children) visit(cell)
      }
      table = null
      previous = ''
      return
    }
    if (node.type === 'paragraph' || node.type === 'tableCell') {
      if (!node.children.length) return
      const [start] = offsets(node.children[0])
      const [, end] = offsets(node.children[node.children.length - 1])
      const { text, tags } = buildSegment(source, node.children, start, end)
      if (hasWords.test(stripTags(text))) {
        segments.push({ kind: node.type, start, end, text, tags, context: context(node.type) })
      }
      if (node.type === 'paragraph') previous = plainText(node)
      return
    }
    if (node.type === 'code') {
      if (node.lang === 'mermaid') {
        for (const seg of mermaidSegments(source, node)) segments.push({ ...seg, context: context('mermaid') })
      }
      return
    }
    if (node.type === 'html' || node.type === 'yaml' || node.type === 'toml') return
    for (const child of node.children ?? []) visit(child)
  }
  visit(tree)
  return segments
}

function clip(text) {
  const flat = text.replace(/\s+/g, ' ').trim()
  return flat.length > 300 ? flat.slice(0, 300) + '…' : flat
}

function plainText(node) {
  if (node.type === 'text' || node.type === 'inlineCode') return node.value
  if (node.type === 'image') return node.alt ?? ''
  return (node.children ?? []).map(plainText).join('')
}

export function stripTags(text) {
  return text.replace(/<\/?s\d+>/g, '')
}

// restore turns a model output back into Markdown. It returns null when the
// output lost, duplicated or mis-nested a tag.
export function restore(segment, output) {
  if (segment.kind === 'mermaid') {
    const clean = stripTags(output).replace(/[\n\r]+/g, ' ').trim()
    return clean.replace(/[[\]{}()|"]/g, (c) => fullWidth[c])
  }
  const tokens = output.split(/(<\/?s\d+>)/)
  const stack = [{ id: 0, parts: [] }]
  const seen = new Set()
  for (const token of tokens) {
    const m = token.match(/^<(\/?)s(\d+)>$/)
    if (!m) {
      stack[stack.length - 1].parts.push(token)
      continue
    }
    const id = Number(m[2])
    if (!m[1]) {
      if (seen.has(id) || !segment.tags.has(id)) return null
      seen.add(id)
      stack.push({ id, parts: [] })
    } else {
      const top = stack.pop()
      if (!top || top.id !== id) return null
      const tag = segment.tags.get(id)
      const inner = top.parts.join('')
      stack[stack.length - 1].parts.push(tag.kind === 'atomic' ? tag.value : tag.prefix + inner + tag.suffix)
    }
  }
  if (stack.length !== 1 || seen.size !== segment.tags.size) return null
  let text = stack[0].parts.join('').replace(/\s*\n\s*/g, ' ').trim()
  if (segment.kind === 'tableCell') text = text.replace(/(?<!\\)\|/g, '\\|')
  if (segment.kind === 'heading' && segment.slug) text += ` {#${segment.slug}}`
  return text
}

const fullWidth = { '[': '［', ']': '］', '{': '｛', '}': '｝', '(': '（', ')': '）', '|': '｜', '"': '”' }

// assemble replaces each segment's span with its translation. A segment with
// no translation keeps its English text (a heading still gets its anchor).
export function assemble(source, segments, translations) {
  let out = source
  for (let i = segments.length - 1; i >= 0; i--) {
    const seg = segments[i]
    let value = translations[i]
    if (value == null) {
      value = source.slice(seg.start, seg.end)
      if (seg.kind === 'heading' && seg.slug) value += ` {#${seg.slug}}`
    }
    out = out.slice(0, seg.start) + value + out.slice(seg.end)
  }
  return out
}
