# Work with the documentation site

The English documents in `docs/` and `ARCHITECTURE.md` are published to GitHub
Pages on every merge to `main` at <https://syudead.github.io/vv/>. Agent
instructions (`.agents/`, `.claude/`), `AGENTS.md`, temporary SDD artifacts and
the root `README.md` are not published.

The site builds directly from the documents in the repository.

```mermaid
flowchart LR
  docs[English documents] --> vp[VitePress build]
  vp --> pages[GitHub Pages]
```

## Prerequisites

- `task setup` has installed the `docs-site/` dependencies.

## Preview the site locally

1. Start the development server:

   ```bash
   mise exec --command "task docs"
   ```

2. Open the printed URL (<http://localhost:5174/vv/>). Edits to a document show
   at once.

The sidebar is built when the server starts, so restart `task docs` after
adding or removing a document or changing its first heading.

`task docs-build` builds as CI does, into `docs-site/.vitepress/dist/` (not
version controlled). Run `task check-docs` to check repository document links,
anchors and writing rules before pushing.

## How publishing works

`.github/workflows/docs.yml` runs on every pull request and push to `main` that
touches published paths, `docs-site/` or the workflow itself. It builds the
site; on `main` it also deploys. Repository Settings → Pages must have Source
set to "GitHub Actions".

## Writing notes

- Maintain the English documents directly, without translated mirrors.
- Link between documents with relative paths. A `README.md` becomes its
  directory's index page (`/docs/how-to/`).
- Links to files outside the published set (code, configuration, root
  documents other than `ARCHITECTURE.md`) point at GitHub on the site.
- A broken link fails the build. `http://localhost:…` examples are allowed.
- Heading anchors follow GitHub's rules, so the same `#anchor` works on GitHub
  and on the site.
- Write documents to [writing-quality.md](../design-docs/writing-quality.md).

## Configuration

The site configuration is `docs-site/.vitepress/config.mts`.

`docs-site/package.json` pins the Vite override because VitePress 1.6.4 depends
on older Vite and esbuild versions with known vulnerabilities
(GHSA-4w7w-66w2-5vf9 and others). Remove the override when a fixed VitePress
release supplies supported dependencies.
