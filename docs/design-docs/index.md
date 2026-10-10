# Design documents

Design documents explain consequential technical decisions and their context.
Add each new document to this index.

## Design document policy

- A document states only what is true. A statement that disagrees with the
  current implementation is not left in place; it is fixed in the change that
  altered the implementation.
- Restricting what may be shown or placed is not the job of a design document.
  Do not write constraints such as "only X", "do not show Y" or "do not add more
  Z"; write how things are now and why.

## Documents

The `ui-design.md` documents below quote Japanese screen text. After the English
conversion, the English catalog (`web/src/i18n/en.ts`) is the source of truth
for screen text ([i18n](i18n.md#wording-in-ui-designmd)).

- [Core beliefs](core-beliefs.md)
- [Writing quality: typed technical English](writing-quality.md)
- [Japanese translation of the documents](japanese-translation.md)
- [Plan quality rules: no text written only to fill a slot](plan-quality.md)
- [Technology selection: MDM (Media Data Management)](tech-stack-selection.md)
- [Separate track inputs for MOV live transcoding](mov-live-transcoding.md)
- [Live transcoding seek and probe reuse](live-transcode-seek.md)
- [Hardware encoding for live transcoding](hardware-encoding.md)
- [Windows desktop app (VVMDM.exe)](windows-app.md)
- [Process lifecycle: startup and shutdown order](process-lifecycle.md)
- [Folder watching: importing changed media folders](folder-watching.md)
- [Playback quality](playback-quality.md)
- [Seek sprite generation](seek-sprite-generation.md)
- [Sidecar subtitle files](sidecar-subtitles.md)
- [Auto-tagging with a Clef classifier in Ollama](auto-tagging.md)
- [vv design system](design-system.md)
- [Library UI: visual rules and list layout](library-ui.md)
- [Screen text and formatting (i18n)](i18n.md)
- [Web test levels](web-testing.md)
- [VVMDM brand and screen UI design](../../specs/022-vvmdm-brand/ui-design.md)
- [Thumbnail preview while seeking: UI](../../specs/009-seek-thumbnail-preview/ui-design.md)
- [Hover video preview in lists: UI](../../specs/010-hover-video-preview/ui-design.md)
- [Folder browsing screens: UI](../../specs/011-folder-browser/ui-design.md)
- [Video page: UI](../../specs/012-video-detail-ia/ui-design.md)
- [Video import progress: UI](../../specs/012-scan-progress/ui-design.md) (placement and controls only; what it shows is in 024)
- [Import progress and results: UI](../../specs/024-import-progress/ui-design.md)
- [Search on the library and folder screens: UI](../../specs/013-library-search/ui-design.md)
- [Video tags and filtering by tag: UI](../../specs/014-video-tags/ui-design.md)
- [Single-account authentication and guest viewing: UI](../../specs/016-single-account-auth/ui-design.md)
- [Folder groups and continuous playback: UI](../../specs/017-folder-groups/ui-design.md)
- [Quality menu and stall warning: UI](../../specs/027-playback-quality/ui-design.md)
- [Editing a video's display name and using the current frame as thumbnail: UI](../../specs/029-video-overrides/ui-design.md)
- [Bundling versions of the same video: UI](../../specs/030-video-versions/ui-design.md)
- [Tentative tag marker, and confirming, rejecting and rejected names: UI](../../specs/031-tentative-tags/ui-design.md)
- [Scrubbing along the bottom edge of a card thumbnail: UI](../../specs/032-card-scrub-preview/ui-design.md)
- [Modified and created dates on the video page, and the "Date created" sort: UI](../../specs/033-video-dates/ui-design.md)
- [Favorite mark, toggling, filtering and sorting for videos and groups: UI](../../specs/035-favorites/ui-design.md)
- [Tag admin screen at scale (top bar, band, tabs, sort, selection and bulk actions): UI](../../specs/036-tag-admin-scale/ui-design.md)
- [vv design system on shadcn/ui (foundations, components, page patterns): UI](../../specs/038-design-system/ui-design.md)
- [Hidden tags behind a card's `+N`, visible and pressable at any count: UI](../../specs/041-tag-overflow-list/ui-design.md)
- [Auto-import of changed media folders (setting, problem notice, quiet result notice): UI](../../specs/042-folder-watch-import/ui-design.md)
- [Watch history screen (day groups, removing one entry, clearing the history): UI](../../specs/043-watch-history/ui-design.md)
