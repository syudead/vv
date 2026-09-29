# Documentation site

The documents under `docs/` and `specs/` are published to GitHub Pages on every
push to `main`, in English at `/` and in machine-translated Japanese at `/ja/`.

| | |
| --- | --- |
| **URL** | <https://syudead.github.io/vv/> |
| **Published** | `docs/` and `specs/` (except `docs/templates/`), plus `docs-site/index.md` |
| **Not published** | Repository-root documents, `.agents/`, `.claude/` and other agent-facing files |
| **Config** | `docs-site/.vitepress/config.mts` |

## Preview locally

1. Install the site's dependencies once with `task setup`.
2. Optionally generate the Japanese edition (see below).
3. Start the server:

   ```bash
   mise exec --command "task docs"
   ```

4. Open <http://localhost:5174/vv/>. Edits to a document show immediately.

To build the site the way CI does, run `task docs-build`. The output goes to
`docs-site/.vitepress/dist/`, outside version control.

The sidebar is built from the directory layout when `task docs` starts, and
each entry is the document's H1. Restart `task docs` after adding, removing or
retitling a document.

## Generate the Japanese edition

`task docs-translate` writes `docs-site/ja/` from the English documents. The
design is in
[translation-pipeline.md](../design-docs/translation-pipeline.md).

| Situation | Command | Result |
| --- | --- | --- |
| No translation service | `task docs-translate` | Offline: segments missing from the memory stay English. |
| Local server (vLLM, Ollama, llama.cpp) running PLaMo Translate | `VV_TRANSLATE_PROVIDER=plamo VV_TRANSLATE_BASE_URL=http://localhost:8000/v1 VV_TRANSLATE_MODEL=pfnet/plamo-2-translate task docs-translate` | New segments are translated and added to the memory. |
| Hosted chat model | `VV_TRANSLATE_BASE_URL=... VV_TRANSLATE_MODEL=... VV_TRANSLATE_API_KEY=... task docs-translate` | Same, through `/chat/completions` with the glossary. |
| Cap the requests of one run | `task docs-translate -- -max-new 200` | At most 200 new segments are requested. |

The local memory is `docs-site/.translation/ja.json`. To start from what CI
has already translated:

```bash
git fetch origin translation-memory
mkdir -p docs-site/.translation
git show FETCH_HEAD:ja.json > docs-site/.translation/ja.json
```

Never edit `docs-site/ja/` by hand; the next run replaces it. Fix a bad
translation by improving the English source, or by adding the term to
[glossary.tsv](../../docs-site/i18n/glossary.tsv).

## Publishing

`.github/workflows/docs.yml` does the work.

| Event | What happens |
| --- | --- |
| Pull request touching `docs/`, `specs/`, `docs-site/` or the translator | Builds both editions offline from the stored memory. Nothing is published. |
| Push to `main` | Translates new segments, saves the memory to the `translation-memory` branch, builds and publishes. |

One-time repository settings:

| Setting | Value |
| --- | --- |
| Settings → Pages → Source | GitHub Actions |
| Actions variable `VV_TRANSLATE_BASE_URL` | OpenAI-compatible API base. Without it, `main` builds offline too. |
| Actions variable `VV_TRANSLATE_MODEL` | Model name, e.g. `pfnet/plamo-2-translate` |
| Actions variable `VV_TRANSLATE_PROVIDER` | `plamo` for PLaMo Translate, otherwise empty (`chat`) |
| Actions secret `VV_TRANSLATE_API_KEY` | Bearer token, if the service needs one |

## Writing for the site

- Link between documents with relative paths. A `README.md` becomes its
  directory's page (`/docs/how-to/`).
- A link outside the published tree (source code, config, root documents)
  points at GitHub on the site.
- A missing link target fails the site build. Links to `http://localhost:…`
  are allowed as examples.
- Anchors follow GitHub's rules (github-slugger), so `#anchor` links work on
  GitHub and on the site. Japanese pages keep the English anchors.
- Mermaid blocks render on GitHub and on the site.
- Write placeholders inside code spans (`` `<branch name>` ``). A bare
  `<branch name>` is parsed as an HTML tag and fails the build.

## Dependencies

`docs-site/package.json` overrides vite to 6.4.3. VitePress 1.6.4 depends on
vite 5 and esbuild 0.21, which have known vulnerabilities
(GHSA-4w7w-66w2-5vf9 and others) that no stable VitePress release fixes yet.
Remove the override once VitePress ships the fix.
