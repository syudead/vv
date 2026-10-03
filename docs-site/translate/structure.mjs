// skeleton() reduces a Markdown document to what a translation must not
// change: block types and depths, list and table shapes, code blocks (Mermaid
// with its labels masked), and link and image destinations.
import { parse, maskMermaid } from './markdown.mjs'

export function skeleton(markdown) {
  const out = []
  function visit(node) {
    switch (node.type) {
      case 'heading':
        out.push(`h${node.depth}`)
        break
      case 'list':
        out.push(`list:${node.ordered ? 'ol' : 'ul'}:${node.children.length}`)
        break
      case 'table':
        out.push(`table:${node.children.length}x${node.children[0]?.children.length ?? 0}`)
        break
      case 'code':
        out.push(`code:${node.lang ?? ''}:${node.lang === 'mermaid' ? maskMermaid(node.value) : node.value}`)
        return
      case 'inlineCode':
        out.push(`ic:${node.value}`)
        return
      case 'link':
      case 'image':
        out.push(`${node.type}:${node.url}`)
        break
      case 'definition':
        out.push(`def:${node.identifier}:${node.url}`)
        break
      case 'blockquote':
      case 'thematicBreak':
      case 'html':
        out.push(node.type)
        break
    }
    for (const child of node.children ?? []) visit(child)
  }
  visit(parse(markdown))
  return out
}

// sameStructure compares two skeletons and returns the first difference, or
// null when they match. Inline code and links are compared as multisets,
// because translation may reorder them within a sentence.
export function structureDiff(source, translated) {
  const a = skeleton(source)
  const b = skeleton(translated)
  const blocks = (s) => s.filter((x) => !x.startsWith('ic:') && !x.startsWith('link:') && !x.startsWith('image:'))
  const inline = (s) => s.filter((x) => x.startsWith('ic:') || x.startsWith('link:') || x.startsWith('image:')).sort()
  const [ba, bb] = [blocks(a), blocks(b)]
  for (let i = 0; i < Math.max(ba.length, bb.length); i++) {
    if (ba[i] !== bb[i]) return `block ${i}: ${ba[i] ?? '(none)'} != ${bb[i] ?? '(none)'}`
  }
  const [ia, ib] = [inline(a), inline(b)]
  for (let i = 0; i < Math.max(ia.length, ib.length); i++) {
    if (ia[i] !== ib[i]) return `inline: ${ia[i] ?? '(none)'} != ${ib[i] ?? '(none)'}`
  }
  return null
}
