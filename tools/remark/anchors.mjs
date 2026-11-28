// Exposes the anchors the docs site and GitHub accept beyond heading slugs, as
// `data.hProperties.id`, which remark-validate-links reads as a landmark: an
// explicit `{#id}` after a heading's text (it replaces the slug), and HTML
// `<a id="...">` or `<a name="...">`.
import {visit} from 'unist-util-visit'

const explicitId = /[ \t]*\{#([^}\s]+)\}$/
const htmlAnchor = /<a\s+(?:id|name)="([^"]+)"/g

export default function remarkExplicitAnchors() {
  return function (tree) {
    visit(tree, 'heading', function (node) {
      const last = node.children.at(-1)
      const match = last?.type === 'text' ? explicitId.exec(last.value) : null
      if (!match) return
      last.value = last.value.slice(0, match.index)
      node.data = {...node.data, hProperties: {...node.data?.hProperties, id: match[1]}}
    })
    visit(tree, 'html', function (node, index, parent) {
      if (!parent || index === undefined) return
      const ids = [...node.value.matchAll(htmlAnchor)].map((m) => m[1])
      const anchors = ids.map((id) => ({type: 'html', value: '', data: {hProperties: {id}}}))
      parent.children.splice(index + 1, 0, ...anchors)
      return index + 1 + anchors.length
    })
  }
}
