# VVMDM visual validation

Use [the development setup](../../docs/how-to/development.md) and [Issue #414](https://github.com/syudead/vv/issues/414) for prerequisites and acceptance criteria. Compare each screen to the `ui-design.md` produced in the design stage. Run `task check` and `task check-docs` on PRs that change both application code and documentation.

Use a library with long Japanese and English titles, multiple tags, at least one folder group, a playable video with bright and dark frames, and a registered root with no videos. Check an owner session and a guest session at a narrow mobile width and a desktop width. Inspect the empty registered root as the owner; the guest cannot access a root without public videos. Where a loading or failure state is not naturally present, use the existing request controls or a temporary failed media request in local development to inspect it; restore normal requests afterward.

| Area | Observe in both widths |
| --- | --- |
| Identity | Top bar, setup/login, video header, tab icon, document title, and video title consistently identify VVMDM. Compare the symbol at 16, 24, and 32 px, and wordmark on dark and light surfaces. Refresh after an earlier tab icon was cached. |
| Setup and login | At both widths, inspect field order, primary action, connection warning, loading, disabled, and failure states. Check keyboard focus and announcement of warning and failure text. |
| Library | Normal, empty, loading, failed, and selection views keep thumbnails and titles first, with search, filters, sorting, tags, and selection actions available. Long titles and many tags do not cover controls. |
| Folders | Folder hierarchy, direct videos, grouped content, and long paths remain distinguishable and navigable for owner and guest where public videos exist. As owner, inspect the empty registered root and its empty state. |
| Tags and settings | Tag add/edit/delete/merge and settings fields/picker work; warnings and dangerous actions have readable labels and separate icon/text cues. |
| Playback | Play, pause, buffering, failure, and end/related-video states keep media and primary controls prominent. Inspect overlays over both bright and dark frames. |

For each area, tab through every visible control and verify focus is clear, labels are announced meaningfully, and disabled or selected state does not depend on color alone. Inspect text contrast against its *actual* rendered surface, including translucent overlays. Record any issue against the relevant implementation PR; visual composition is judged by the five UI-quality dimensions in Issue #414 rather than presence of components alone.
