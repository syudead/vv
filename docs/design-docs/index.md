# Design documents

Design documents explain consequential technical decisions and their context.
Add each new document to this index.

## Policy for design documents

- Write only what is true. Fix a statement that disagrees with the
  implementation in the same change that alters the implementation.
- Describe what the system does now and why. Do not write restrictions such as
  "only X", "never show Y" or "do not add Z"; limiting what may be built is not
  the job of a design document.
- Write in technical English and follow
  [Writing style for repository documents](writing-style.md). The Japanese
  edition of the documentation site is generated
  ([Documentation translation pipeline](translation-pipeline.md)).

## Documents

### Process and writing

| Document | Covers |
| --- | --- |
| [Core beliefs](core-beliefs.md) | Principles every change follows |
| [Writing style for repository documents](writing-style.md) | Language, register, shapes and document templates |
| [Documentation translation pipeline](translation-pipeline.md) | How the Japanese site is generated from the English source |
| [Plan quality rules](plan-quality.md) | What a `plan.md` may contain |

### System

| Document | Covers |
| --- | --- |
| [Tech stack selection: MDM (Media Data Management)](tech-stack-selection.md) | The stack and the component boundaries |
| [Split-track inputs for MOV live transcoding](mov-live-transcoding.md) | Feeding ffmpeg MOV tracks separately |
| [Live transcoding seek and probe data reuse](live-transcode-seek.md) | Seeking inside a live transcode |
| [Hardware encoding for live transcoding](hardware-encoding.md) | Encoder selection and fallback |
| [Seek sprite generation](seek-sprite-generation.md) | Building the seek thumbnail sprites |
| [UI text and formatting (i18n)](i18n.md) | The message catalog and formatters |

### UI

The quoted labels in each `ui-design.md` are proposals. The catalog
`web/src/i18n/en.ts` is the source of truth for the text the UI shows
([i18n](i18n.md#labels-in-ui-designmd)).

| Document | Covers |
| --- | --- |
| [Library UI: visual rules and list layout](library-ui.md) | The design system and list layout |
| [VVMDM brand and screen UI design](../../specs/022-vvmdm-brand/ui-design.md) | Brand, colours and typography |
| [Thumbnail preview while seeking](../../specs/009-seek-thumbnail-preview/ui-design.md) | Seek bar preview |
| [Library hover preview](../../specs/010-hover-video-preview/ui-design.md) | Animated preview on hover |
| [Folder page](../../specs/011-folder-browser/ui-design.md) | Browsing by folder hierarchy |
| [Video detail page](../../specs/012-video-detail-ia/ui-design.md) | Playback page information architecture |
| [Scan progress display](../../specs/012-scan-progress/ui-design.md) | Where the progress lives and how it is operated (024 defines what it shows) |
| [Import progress and results](../../specs/024-import-progress/ui-design.md) | What the scan progress shows |
| [Search, filter and sort](../../specs/013-library-search/ui-design.md) | Library and folder toolbars |
| [Video tags and the tag filter](../../specs/014-video-tags/ui-design.md) | Tag chips, tag editing, tag page |
| [Single-account authentication](../../specs/016-single-account-auth/ui-design.md) | Sign-in and guest browsing |
| [Folder groups and continuous playback](../../specs/017-folder-groups/ui-design.md) | Group cards and up-next |
| [API tokens section](../../specs/026-external-api/ui-design.md) | Issuing and revoking tokens |
