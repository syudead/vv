# Force rescan

A force rescan reads the library, one media folder or one video again and
rebuilds the metadata, thumbnail, seek sprite, hover preview and fingerprint of
every video it reaches, even when the file is unchanged
([`internal/app/scans.go`](../../internal/app/scans.go),
[`internal/scanner/rescan.go`](../../internal/scanner/rescan.go)).

The diagram shows the path from the owner's request to the rebuilt artifacts.

```mermaid
flowchart LR
  owner[Owner] --> api[POST /api/scans/rescan]
  api --> row[Scan row with scope]
  row --> pass[Force pass]
  pass --> jobs[Forced jobs]
  jobs --> stages[Import stages]
  stages --> judge[Duplicate judging]
```

## A force rescan is a manual scan with a scope

The rescan is a `scans` row of origin `manual` that records its scope, and the
scanner runs in force mode for it. The row gives the rescan the manual scan's
progress, issue list, activity and one-running-scan rule; a separate origin
would change every check that treats it as manual.

| Scope | Reads | Removes missing locations | Issues of the previous scan |
| --- | --- | --- | --- |
| `library` | Every media folder | Everywhere, as a manual scan | Cleared |
| `folder` | That media folder's tree | Under that folder only | Kept outside the folder |
| `video` | The video's registered locations | Nothing | Kept except that video's |

A `video` scope whose file is missing or unreadable records the issue a manual
scan would and keeps the video.

## Starting one

`POST /api/scans/rescan` supersedes a running watch scan, as `Scan library`
does, and refuses a running manual scan or force rescan with `409
scan_in_progress` ([`api/openapi.yaml`](../../api/openapi.yaml), `startRescan`).
A `folder` or `video` rescan reads only part of the library, so the watcher
queues the superseded batch again and keeps a lost-events problem
([folder watching](folder-watching.md)).

```mermaid
flowchart LR
  req[Start request] --> watch{Watch scan runs?}
  watch -->|yes| stop[Supersede it]
  watch -->|no| other{Other scan runs?}
  stop --> other
  other -->|yes| refuse[409 scan_in_progress]
  other -->|no| start[202 Scan with rescan]
```

Unlike `Scan library`, which answers the running scan, the rescan refuses it:
returning a scan of another scope would tell the owner a rescan started that
did not. Every `Scan` response and SSE `scan` event of a force rescan carries
`rescan` with its scope, so the screen can name it.

## Where the owner starts it

Each scope has one control where its target is already on screen. The library
and folder scopes ask first because they reread many videos; the video scope
rereads one and starts at once.

| Scope | Control | Confirmation |
| --- | --- | --- |
| `library` | `Force rescan` beside `Scan library` in Settings, `Scan status` ([`ScanStatusSection.tsx`](../../web/src/settings/ScanStatusSection.tsx)) | Dialog saying every video is read again and it takes long |
| `folder` | `Force rescan` on the folder's row in Settings, `Media folders` ([`MediaFoldersSection.tsx`](../../web/src/settings/MediaFoldersSection.tsx)) | The same dialog, naming the folder |
| `video` | The icon action after `Open file` and `Copy path` in the video page's file facts ([`RescanVideoButton.tsx`](../../web/src/player/RescanVideoButton.tsx)) | None |

Every control is unavailable, with the reason, while a manual scan or force
rescan runs or no media folder exists, as `Scan library` is; a watch scan does
not block it, because the start supersedes it. A refused start shows its
reason where the owner acted: inside the dialog, which stays open, or in the
file facts' error row. The client leaves the no-folder check to the server, so
a folder removed while the dialog is open is refused with its reason rather
than closing the dialog as if the rescan started. A failure without a 4xx
answer may hide a start whose response was lost, so when a later read or event
shows the new scan, the dialog closes or the error row clears as on a start;
a `409` or `404` never counts as a start. While the rescan runs, the `Scan status` section and
the bottom-right indicator's summary name its scope.

| Rejected | Why |
| --- | --- |
| A control on the folder page | The folder page shows folder groups; media folders are managed in Settings |
| One dialog with a scope picker | Each entry point already names its scope; a picker adds a step |

## Force travels on the job

The force pass queues `probe`, `thumbnail` and `seek_thumbnail` jobs with
`jobs.force` for every video it reaches; a forced `probe` result queues a
forced `preview`, and a forced `seek_thumbnail` result a forced `fingerprint`.
A forced job regenerates whatever the video's stage state is, and the stage
states stay as they are until the job records its result, so the screen keeps
the old thumbnail and preview while the new ones are built.

A forced `thumbnail` republishes the owner's applied image, extracts the chosen
position again, or else takes the automatic frame
([thumbnail editing](thumbnail-editing.md)). A forced sprite or preview is
built beside the published one ([forced rebuild](seek-sprite-generation.md#forced-rebuild)).
A forced job that gives up records its stage `failed` even over a `done`
state, with the import issue an ordinary failure records, so a failed rebuild
is not shown as the old success; an unforced job keeps a `done` state. The
video's `seekThumbnailState` stays `failed` while the kept sprite still serves
the seek preview. An owner's thumbnail edit while a forced `thumbnail` job runs
clears that job's force, because the edit is the newer result: the job giving
up afterwards keeps the edit's `done` state.

| Rejected | Why |
| --- | --- |
| Reset the stage states to `pending` | The screen sends a thumbnail URL only for a `done` state, so every card would lose its thumbnail during the rescan |
| Queue jobs without a scan row for the `video` scope | A missing file would need a second way to be reported, and a running manual scan would not refuse it |

A forced `seek_thumbnail` stage queues neither auto-tagging nor face analysis:
the owner re-runs tagging with `Tag all videos again`
([auto-tagging](auto-tagging.md)).

## An unchanged file is read again

In force mode the scanner recomputes the content key of a file whose size and
modification time are unchanged, because a replaced file with the same size and
time would otherwise keep stale artifacts. A file that differs takes the
ordinary path first; then every video the walk reaches is forced once per scan
and marked `forced` in the scan's video set.

## Resume after a restart

Startup resumes an interrupted force rescan like an interrupted manual scan
([process lifecycle](process-lifecycle.md)): the new scan keeps the scope and
the `forced` marks, so the walk skips the videos the interrupted run already
queued. A `folder` rescan whose media folder now points elsewhere fails instead
of reading the new tree.
