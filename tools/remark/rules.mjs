// Repository rules for tracked Markdown, run by `task check-docs` through
// remark-cli (config.mjs). Each rule turns a writing or SDD convention from
// docs/design-docs/writing-quality.md into a lint failure.
import fs from 'node:fs'
import path from 'node:path'
import {lintRule} from 'unified-lint-rule'
import {visit} from 'unist-util-visit'
import {proseLines, relativePath} from './prose.mjs'

// "4. Rules", "2.1. Reads" and "4) Rules" are section numbers; a bare version
// ("2.1 API") or a year ("2026 release notes") is a name.
export const numberedHeading = /^\d+(\.\d+)*[.)]\s/

// A section number enters the anchor, so adding a section breaks the links to
// the sections after it (W-9).
export const noNumberedHeadings = lintRule(
  'vv:no-numbered-headings',
  function (tree, file) {
    const source = String(file)
    visit(tree, 'heading', function (node) {
      const first = node.children[0]
      const last = node.children.at(-1)
      if (!first?.position || !last?.position) return
      const text = source.slice(first.position.start.offset, last.position.end.offset)
      if (numberedHeading.test(text)) {
        file.message(
          `Numbered heading "${text}". Drop the number and refer to the section by its name`,
          node
        )
      }
    })
  }
)

const japanese = /[\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Han}　-〿！-｠]/u

// Documents are in English (W-1). Quoted screen text and user data belong in
// code, which proseLines blanks. Options: {exempt: [repository-relative paths]}.
export const englishProse = lintRule('vv:english-prose', function (tree, file, options) {
  if ((options?.exempt ?? []).includes(relativePath(file))) return
  proseLines(tree, file).forEach(function (line, i) {
    if (japanese.test(line)) {
      file.message(
        'Japanese prose. Write documents in English; put quoted screen text or user data in inline code',
        {line: i + 1, column: 1}
      )
    }
  })
})

// Fragments the SDD templates carry for the author to replace. A Plan's
// `[NEEDS CLARIFICATION: ...]` is deliberately absent: it may stay open between
// rounds, which a repository-wide check cannot judge.
export const templatePlaceholders = [
  '[REMOVE IF UNUSED]',
  '[e.g.,',
  '[Assumption about',
  '[Measurable metric',
  '[Child Issue title]',
  '[FEATURE]',
  '[###-feature-name]',
  '[PRINCIPLE_'
]

// Fails on a placeholder left in prose under specs/, outside any `assets`
// directory. Options: {dir: 'specs/'}.
export const noTemplatePlaceholders = lintRule(
  'vv:no-template-placeholders',
  function (tree, file, options) {
    const rel = relativePath(file)
    if (!rel.startsWith(options?.dir ?? 'specs/') || rel.split('/').includes('assets')) return
    proseLines(tree, file).forEach(function (line, i) {
      for (const marker of templatePlaceholders) {
        if (line.includes(marker)) {
          file.message(
            `Template placeholder "${marker}" was delivered unfilled. Fill the section, or remove it when it does not apply`,
            {line: i + 1, column: line.indexOf(marker) + 1}
          )
        }
      }
    })
  }
)

function linkedUrls(tree) {
  const urls = []
  visit(tree, ['link', 'image', 'definition'], function (node) {
    urls.push(node)
  })
  return urls
}

// Each listed index links every sibling Markdown document (AGENTS.md).
// Options: {indexes: [repository-relative index.md paths]}.
export const indexLinksSiblings = lintRule(
  'vv:index-links-siblings',
  function (tree, file, options) {
    const rel = relativePath(file)
    if (!(options?.indexes ?? []).includes(rel)) return
    const linked = new Set(linkedUrls(tree).map((node) => node.url))
    const dir = path.dirname(path.resolve(file.cwd, file.path))
    for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
      const name = entry.name
      if (entry.isDirectory() || name === 'index.md' || !name.endsWith('.md')) continue
      if (!linked.has(name)) {
        file.message(
          `${path.posix.dirname(rel)}/${name} is not linked from this index. Add it so the document can be found`
        )
      }
    }
  }
)

const remoteArtifact = /\/specs\/[^/#]+\//

/** Reports whether a link target from `source` points into `root`/specs. */
export function linksToSDDArtifact(root, source, target) {
  if (target.includes('://')) {
    return target.startsWith('https://github.com/syudead/vv/') && remoteArtifact.test(target)
  }
  const file = target.split('#')[0]
  if (file === '') return false
  const resolved = file.startsWith('/')
    ? path.join(root, file)
    : path.join(path.dirname(source), file)
  const rel = path.relative(path.join(root, 'specs'), resolved)
  return rel !== '..' && !rel.startsWith('..' + path.sep) && !path.isAbsolute(rel)
}

function isPermanent(rel) {
  return rel === 'ARCHITECTURE.md' || rel === 'README.md' || rel.startsWith('docs/')
}

// Permanent documents outlive a feature's specs/ directory, so they cite design
// docs, not SDD artifacts, even while the linked file still exists.
export const noSDDArtifactLinks = lintRule('vv:no-sdd-artifact-links', function (tree, file) {
  if (!isPermanent(relativePath(file))) return
  const source = path.resolve(file.cwd, file.path)
  for (const node of linkedUrls(tree)) {
    if (linksToSDDArtifact(file.cwd, source, node.url)) {
      file.message(
        `Link to temporary SDD artifact ${node.url}. Retain the lasting decision in docs/design-docs and cite that instead`,
        node
      )
    }
  }
})
