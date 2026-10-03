# Work with the documentation site

The documents in `docs/`, `specs/` and `ARCHITECTURE.md` are published to GitHub
Pages on every merge to `main`, in English at <https://syudead.github.io/vv/>
and in Japanese under <https://syudead.github.io/vv/ja/>. The Japanese pages are
machine translations; the design behind them is in
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
`docs-site/translate/site.mjs` into `ja/` (not version controlled). They come
from a checkout of the `docs-ja` branch in `.docs-ja/`; without one, every
Japanese page shows the English text under an "untranslated" notice. To see
the published translations locally:

```bash
git fetch origin docs-ja && git worktree add --detach .docs-ja FETCH_HEAD
```

## How publishing works

| Workflow | Trigger | What it does |
| --- | --- | --- |
| `.github/workflows/docs.yml` | Pull request touching published paths | Runs the translator's tests and builds the site; publishes nothing |
| `.github/workflows/docs.yml` | Push to `main`, or a run started by the translation workflow | Builds the site from `main` and the current `docs-ja`, then deploys it |
| `.github/workflows/docs-translate.yml` | Push to `main` touching published paths, nightly at 18:17 UTC, or manual | Translates out-of-date documents, commits them to `docs-ja`, and starts a deploy |

The English site never waits for translation. Repository Settings → Pages must
have Source set to "GitHub Actions".

## Translations

| Situation | What the reader sees | What fixes it |
| --- | --- | --- |
| The English document changed | The old translation with a notice that it is out of date | The next translation run |
| A new document | The English text with a notice that it is not translated yet | The next translation run |
| The translation run failed | The previous translation, with the out-of-date notice | The next push or nightly run, or a manual run of `Docs translation` |
| The English document was deleted or renamed | No Japanese page at the old path | The next build |

Never edit a translation. Fix the English source on `main`, or the glossary.

### Change a term's translation

1. Edit `docs-site/translate/glossary.tsv` (`english<TAB>japanese`). A UI label
   maps to itself, because the screen is English.
2. Merge the change. The next run retranslates only the segments that contain
   the term.

### Change the model

1. Edit `docs-site/translate/model.json`: the repository, revision, file name
   and sha256 of the GGUF file, the prompt `family`, and the `llama.cpp` release
   tag.
2. Merge the change. The next runs retranslate every document, a few hours at a
   time.

### Read a run's results

Open the `Docs translation` run in the Actions tab. Each shard's summary lists
translated files, failed files, segments kept in English (the model dropped a
code or link marker twice), glossary misses, and files left for the next run.

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
