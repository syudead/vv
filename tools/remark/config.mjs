// remark-cli configuration for `task check-docs`; see
// docs/how-to/development.md#validate-changes for the rules it enforces.
import remarkFrontmatter from 'remark-frontmatter'
import remarkGfm from 'remark-gfm'
import remarkLintNoTrailingSpaces from 'remark-lint-no-trailing-spaces'
import remarkValidateLinks from 'remark-validate-links'
import remarkExplicitAnchors from './anchors.mjs'
import {
  englishProse,
  indexLinksSiblings,
  noNumberedHeadings,
  noSDDArtifactLinks,
  noTemplatePlaceholders
} from './rules.mjs'

export default {
  plugins: [
    remarkFrontmatter,
    remarkGfm,
    // Before remark-validate-links, which reads the anchors it adds.
    remarkExplicitAnchors,
    // Naming the repository resolves its github.com URLs and `/` paths locally.
    [remarkValidateLinks, {repository: 'https://github.com/syudead/vv', root: '.'}],
    remarkLintNoTrailingSpaces,
    noNumberedHeadings,
    // PR bodies are Japanese, and the template is their skeleton.
    [englishProse, {exempt: ['.github/pull_request_template.md']}],
    [indexLinksSiblings, {indexes: ['docs/design-docs/index.md', 'docs/product-specs/index.md']}],
    [noTemplatePlaceholders, {dir: 'specs/'}],
    noSDDArtifactLinks
  ]
}
