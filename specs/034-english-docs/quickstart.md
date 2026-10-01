# Quickstart: first live translation run

The translator's behaviour is covered by tests with a fake engine, and the
model's speed and quality by a sample run (see the translation unit in
[plan.md](plan.md#translate-published-english-documents-into-japanese-automatically)).
These steps check the parts that need `main`. Run them once after the
integration PR is merged.

## Prerequisites

- GitHub Pages is already set to deploy from GitHub Actions
  ([docs-site.md](../../docs/how-to/docs-site.md)). No secret is needed.
- The first full run takes about 10 hours of runner time. It is spread over the
  push run and the following nightly runs (R-1).

## Steps

| Step | Expected result |
| --- | --- |
| Run the `Docs site` workflow with `workflow_dispatch` on `main` | The English site deploys at once; `translate` commits to `docs-ja` and its summary lists translated files, English-kept segments, glossary misses and remaining files; a second deploy follows |
| Open `https://syudead.github.io/vv/ja/` | Translated pages have the same headings, tables, lists and Mermaid diagrams as the English pages; code, paths and URLs are unchanged; untranslated pages carry the banner |
| Wait for nightly runs until the summary lists no remaining files | Every published page under `/ja/` is translated |
| From a `/ja/` page, follow a link to a heading in another document | It lands on the heading |
| Change one paragraph of an English document on `main` | The next run translates only that paragraph, within minutes |
| Delete or rename an English document on `main` | The next run removes or moves its translation |
