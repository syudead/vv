# Implementation Plan: VVMDM brand redesign

**Branch**: `feature/022-vvmdm-brand` | **Parent Issue**: #414

**Input**: [Issue #414](https://github.com/syudead/vv/issues/414) is the specification. Its `ui` label requires a separate design stage before implementation Issues are created.

## Summary

Make the supplied VVMDM mark, name, and six colors the basis of one visual system across the existing video library, while preserving search, organization, playback, authentication, and settings behavior. The design stage will define the actual compositions and logo variants; implementation will apply them through the existing theme and screen ownership boundaries, with visual and accessibility checks at narrow and wide widths.

## Technical Context

The existing ownership and dependency direction are in [ARCHITECTURE.md](../../ARCHITECTURE.md); the visual token and layout rationale is in [library-ui.md](../../docs/design-docs/library-ui.md). The current values and contrast assertions are in [`web/src/index.css`](../../web/src/index.css) and [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts). [Issue #414](https://github.com/syudead/vv/issues/414) defines the visual and accessibility acceptance criteria. [AGENTS.md](../../AGENTS.md) and [core beliefs](../../docs/design-docs/core-beliefs.md) govern validation and documentation.

The supplied originals are retained in [`logo/`](../../logo/) for design and implementation. `symbol.svg` is the standalone mark; `1.svg`, `2.svg`, and `3.svg` are cyan, white, and black wordmarks. The PNGs are reference exports. The app currently has the dot plus `vv` in the shell, credential screen, and video header, and `vv` in the document and video titles. These are display-name changes; storage keys, API names, and CSS selectors are outside the Issue's scope.

No data model or external API changes are needed, so there is no `data-model.md` or `contracts/`. The established stack and theme strategy leave no feature-specific research decision, so there is no `research.md`. The [quickstart](quickstart.md) records the manual visual checks that the existing automated checks cannot perform.

## Constitution Check

- [ARCHITECTURE.md](../../ARCHITECTURE.md) and [core beliefs](../../docs/design-docs/core-beliefs.md): keep backend, API, and persisted identifiers stable; document the visual behavior with its implementation. Pass before design; recheck after design.
- [library-ui.md](../../docs/design-docs/library-ui.md): use the role-based theme tokens and extend contrast assertions for new text/surface pairs; preserve the library's content-first hierarchy. Pass before design; recheck after design.
- [AGENTS.md](../../AGENTS.md): this Plan includes assets as well as documentation, so both `task check` and `task check-docs` apply before push; repeat the applicable checks per implementation PR.

## Project Structure

The supplied editable logo source stays in `logo/`; the design stage will specify which variants and small-size adjustments are suitable for app delivery. **Alternative rejected:** use the PNG exports as the only runtime source, because that would blur the mark at tab and dense-screen sizes and prevent controlled small-size adjustment.

App-specific branding will be delivered under `web/` by the implementation units. Brand colors and common text/surface roles belong in the existing `web/src/index.css` theme, with contrast pairs in `web/src/theme/tokens.test.ts`. **Alternative rejected:** put the six hex values directly in each screen, because the shared selection, progress, and action meanings would drift and bypass the existing color scan.

The display name will change at the visible entry points while internal `vv` identifiers stay stable. **Alternative rejected:** rename storage keys and selectors with the brand, because their compatibility and semantics differ from a user-facing name change.

The separate `ui-design.md` produced by the design stage will be the visual source of truth for logo usage, color roles, screen composition, responsive behavior, and states. The implementation units below consume that artifact rather than making one screen the de facto standard. **Alternative rejected:** redesign all screens in one PR, because it would make the cross-screen visual and keyboard review too large to verify against Issue #414.

## Implementation Work

### VVMDM theme and shared control states

**Scope**: Update `web/src/index.css` role tokens and shared UI primitives for the design's dark surfaces, six supplied colors, text hierarchy, focus, disabled, loading, and semantic status states. Extend `web/src/theme/tokens.test.ts` for the actual text/surface and control pairs. Update [library-ui.md](../../docs/design-docs/library-ui.md) where its current mint and charcoal rationale changes.

**Dependencies**: The `ui-design.md` from the design stage; no other implementation unit.

**Acceptance evidence**: Shared controls and a component sample match the design on dark and light logo-adjacent surfaces; normal text reaches 4.5:1 and key control boundaries reach 3:1 where Issue #414 calls for them. Focus, disabled, and status meanings remain distinct without color alone; `task check` and `task check-docs` pass.

### VVMDM logo and visible product name

**Scope**: Derive app-delivery logo assets from [`logo/`](../../logo/), use the approved wordmark and symbol in the top bar, setup/login, and video header, and update the tab icon, document title, video title, and user-facing product descriptions in the application and user documentation. Keep internal identifiers stable.

**Dependencies**: “VVMDM theme and shared control states”.

**Acceptance evidence**: At 16, 24, and 32 px the symbol is recognizable; dark, light, and monochrome placements follow the design. The named app entry points, browser titles, and user descriptions show VVMDM rather than the old product name, the logo has a suitable accessible name or is hidden when redundant, and related UI tests, `task check`, and `task check-docs` pass.

### Library grid and search surfaces

**Scope**: Apply the design to the library landing view, cards and group cards, search/filter/sort toolbar, selection actions, and their normal, empty, loading, and error states. Preserve the existing search and organization behavior.

**Dependencies**: “VVMDM theme and shared control states” and “VVMDM logo and visible product name”.

**Acceptance evidence**: Narrow and wide browser views retain video-image and title priority while filters and actions remain discoverable; keyboard focus, selection, thumbnail overlay text, and state messages remain readable and operable for owner and guest. Relevant UI tests, `task check`, and the [quickstart](quickstart.md) library scenarios pass.

### Folder browsing surfaces

**Scope**: Apply the design to folder navigation, folder and video cards, folder toolbars, and grouping actions, including long names and empty folders. Preserve folder access and navigation behavior.

**Dependencies**: “VVMDM theme and shared control states” and “VVMDM logo and visible product name”.

**Acceptance evidence**: Narrow and wide browser views distinguish folder hierarchy from video content without losing the dense browse view; long names wrap or truncate without covering actions; owner and guest navigation, focus, and empty states work. Relevant UI tests, `task check`, and the [quickstart](quickstart.md) folder scenarios pass.

### Tags and settings surfaces

**Scope**: Apply the design to tag lists, editing controls and dialogs, and settings sections, fields, picker, status and dangerous actions. Keep all current tag and settings operations intact.

**Dependencies**: “VVMDM theme and shared control states” and “VVMDM logo and visible product name”.

**Acceptance evidence**: Narrow and wide browser views show a clear order for primary and destructive actions; input, focus, error, loading, and disabled states have text or icon cues in addition to color. Owner operations and guest-available navigation remain usable; relevant UI tests, `task check`, and the [quickstart](quickstart.md) scenarios pass.

### Video detail and playback surfaces

**Scope**: Apply the design to video detail metadata, tags and related videos, the player controls and overlays, and play, pause, buffering, failure, and end states. Keep video and controls ahead of decorative branding.

**Dependencies**: “VVMDM theme and shared control states” and “VVMDM logo and visible product name”.

**Acceptance evidence**: Narrow and wide browser views keep media, title, and playback actions legible; overlay controls and text remain readable against representative bright and dark frames. Owner and guest can use the existing playback path by keyboard and pointer; relevant UI tests, `task check`, and the [quickstart](quickstart.md) playback scenarios pass.
