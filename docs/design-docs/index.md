# Design documents

Design documents explain consequential technical decisions and their context.
Add each new document to this index. Completed SDD artifacts are not
reference documents; retain lasting decisions here before deleting them.

## Design document policy

- A document states only what is true. A statement that disagrees with the
  current implementation is not left in place; it is fixed in the change that
  altered the implementation.
- Restricting what may be shown or placed is not the job of a design document.
  Do not write constraints such as "only X", "do not show Y" or "do not add more
  Z"; write how things are now and why.

## Documents

- [Core beliefs](core-beliefs.md)
- [Writing quality: typed technical English](writing-quality.md)
- [Plan quality rules: no text written only to fill a slot](plan-quality.md)
- [Technology selection: MDM (Media Data Management)](tech-stack-selection.md)
- [Separate track inputs for MOV live transcoding](mov-live-transcoding.md)
- [Live transcoding seek and probe reuse](live-transcode-seek.md)
- [Media information from the import probe](media-information.md)
- [Hardware encoding for live transcoding](hardware-encoding.md)
- [Windows desktop app (VVMDM.exe)](windows-app.md)
- [Process lifecycle: startup and shutdown order](process-lifecycle.md)
- [Folder watching: importing changed media folders](folder-watching.md)
- [Playback quality](playback-quality.md)
- [Seek sprite generation](seek-sprite-generation.md)
- [Sidecar subtitle files](sidecar-subtitles.md)
- [Auto-tagging with a decision model](auto-tagging.md)
- [AI layer: classifier port and providers](ai-layer.md)
- [Judging possible duplicates automatically](duplicate-judging.md)
- [Activity log](activity.md)
- [Thumbnail editing sessions](thumbnail-editing.md)
- [Work-info plugins: sandbox, limits and network](work-info-plugins.md)
- [vv design system](design-system.md)
- [Library UI: visual rules and list layout](library-ui.md)
- [Related videos and the previous and next video](related-videos.md)
- [Home library and setup](home.md)
- [Screen text and formatting (i18n)](i18n.md)
- [Testing policy: shared rules](testing.md)
- [Time and synchronization in tests](testing-time.md)
- [Web test levels](web-testing.md)
- [Test and CI quality monitoring](test-quality-monitoring.md)
- [Complexity and duplication](complexity-and-duplication.md)
- [Authentication and visibility](authentication.md)
- [Catalog organization and persistent identity](catalog-organization.md)
- [Import progress and recovery](import-pipeline.md)
