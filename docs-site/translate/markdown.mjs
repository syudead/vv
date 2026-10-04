// Markdown helpers shared by the translation checks and the site build.
import { unified } from 'unified'
import remarkParse from 'remark-parse'
import remarkGfm from 'remark-gfm'
import remarkFrontmatter from 'remark-frontmatter'
import GithubSlugger from 'github-slugger'

const parser = unified().use(remarkParse).use(remarkGfm).use(remarkFrontmatter, ['yaml', 'toml'])

export function parse(markdown) {
  return parser.parse(markdown)
}

const explicitAnchor = /\s*\{#([^}\s]+)\}\s*$/

function plainText(node) {
  if (node.type === 'text' || node.type === 'inlineCode') return node.value
  if (node.type === 'image') return node.alt ?? ''
  return (node.children ?? []).map(plainText).join('')
}

// headings lists a document's headings in order: depth, the anchor GitHub
// gives it (or its explicit {#id}), and the source span of its text.
export function headings(markdown) {
  const slugger = new GithubSlugger()
  const out = []
  ;(function visit(node) {
    if (node.type === 'heading') {
      const text = plainText(node)
      const explicit = text.match(explicitAnchor)
      out.push({
        depth: node.depth,
        anchor: explicit ? explicit[1] : slugger.slug(text),
        explicit: Boolean(explicit),
        line: node.position.start.line,
      })
      return
    }
    for (const child of node.children ?? []) visit(child)
  })(parse(markdown))
  return out
}

// Mermaid labels: [text], ("text"), {text}, |edge text|, participant X as Y,
// and note ...: text. Node ids, arrows and keywords stay as they are.
// The first alternative is a `subject: label` line (stateDiagram transitions and
// state descriptions, sequenceDiagram messages, notes): the subject holds no
// shape brackets except a state diagram's [*].
const mermaidLabel =
  /^[ \t]*(?:\[\*\]|[^\n:[\](){}|"])+?:[ \t]*([^\n]+)|\[\[?"?([^\]"\n]+?)"?\]?\]|\(\(?"?([^()"\n]+?)"?\)?\)|\{\{?"?([^{}"\n]+?)"?\}?\}|\|"?([^|"\n]+?)"?\||\bparticipant\s+\S+\s+as\s+([^\n]+)|\bnote\s+[^:\n]+:\s*([^\n]+)/gm

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

