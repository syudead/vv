# Work with the documentation site

The documents in `docs/`, `specs/` and `ARCHITECTURE.md` are published to GitHub
Pages on every merge to `main`, in English at <https://syudead.github.io/vv/>
and in Japanese under <https://syudead.github.io/vv/ja/>. The Japanese pages come
from `translations/ja/`, written by the `doc-translator` subagent; the design
behind them is in
[japanese-translation.md](../design-docs/japanese-translation.md).

Agent instructions (`.agents/`, `.claude/`), `AGENTS.md` and the root
`README.md` are not published.

## Prerequisites

- `task setup` has installed the `docs-site/` dependencies.

## Preview the site locally

1. Start the development server:

   ```bash
   mise exec --command "task docs"
   ```

2. Open the printed URL (<http://localhost:5174/vv/>). Edits to a document show
   at once.

The sidebar is built from the directory layout when the server starts. After
adding or removing a document, or changing its first heading, restart
`task docs`.

To build as CI does, run `task docs-build`. The output goes to
`docs-site/.vitepress/dist/` (not version controlled).

Japanese pages are generated before each build by
`docs-site/translate/site.mjs` into `ja/` (not version controlled), from
`translations/ja/`. A document without a translation shows its English text
under an "untranslated" notice.

## How publishing works

`.github/workflows/docs.yml` runs on every pull request and push to `main` that
touches published paths, `translations/` or `docs-site/`. It tests the
translation tools, checks the translations, and builds the site. On `main` it
also deploys the site. Repository Settings → Pages must have Source set to
"GitHub Actions".

## Translate a changed document

Translate after the English of the change is final, in the same pull request.

1. Hand the changed paths to the `doc-translator` subagent (for example,
   "translate `docs/how-to/docs-site.md`"). It writes
   `translations/ja/<path>` and runs `stamp` on it.
2. When a document is deleted or renamed, name both paths. The subagent deletes
   or moves the translation.
3. Run the check and commit the translations with the change:

   ```bash
   mise exec --command "task docs-test"
   ```

Never edit a translation by hand. To change a wording, fix the English source,
or the [translation rules](../design-docs/japanese-translation.md#translation-rules),
and translate again.

### Commands

| Command | What it does |
| --- | --- |
| `node docs-site/translate/ja.mjs stamp <path>...` | Pins each heading's English anchor, records the source hash, and checks the structure |
| `node docs-site/translate/ja.mjs check` (`task docs-test`) | Fails on a broken translation or one whose source is gone; warns on a stale one |
| `node docs-site/translate/ja.mjs status` (`task docs-ja-status`) | Lists published documents whose translation is stale or missing |

### What readers see

| Situation | What the reader sees | What fixes it |
| --- | --- | --- |
| The English document changed after its translation | The old translation with a notice that it is out of date | Translating the document again |
| A document without a translation | The English text with a notice that it is not translated yet | Translating the document |
| The English document was deleted or renamed | No Japanese page at the old path; `check` fails until the translation is deleted or moved | Deleting or moving the translation |

## Writing notes

- Link between documents with relative paths. A `README.md` becomes its
  directory's index page (`/docs/how-to/`).
- Links to files outside the published set (code, configuration, root
  documents other than `ARCHITECTURE.md`) point at GitHub on the site.
- A broken link fails the build. `http://localhost:…` examples are allowed.
- Heading anchors follow GitHub's rules, so the same `#anchor` works on GitHub
  and on the site. Japanese pages keep the English anchors.
- Write documents in English to
  [writing-quality.md](../design-docs/writing-quality.md).

## Configuration

The site configuration is `docs-site/.vitepress/config.mts`.

`docs-site/package.json` overrides vite to 6.4.3. VitePress 1.6.4 depends on
vite 5 and esbuild 0.21, which have known vulnerabilities (GHSA-4w7w-66w2-5vf9
and others) that the stable VitePress release has not fixed. Remove the
override when a fixed VitePress is released.
