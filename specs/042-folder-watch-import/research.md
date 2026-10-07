# Research: Auto-import changed media folders

Inherited decisions: the stack in
[tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md), the
boundaries and the scanner's role in [ARCHITECTURE.md](../../ARCHITECTURE.md),
and the import progress and issues in
[024 research](../024-import-progress/research.md). This file
records only the decisions this feature adds.

## R-1: fsnotify watches every directory on both platforms

**Decision**: The watcher uses `github.com/fsnotify/fsnotify` (v1.10.1) with one
watch per directory below each media folder, on Linux and on Windows.

| Option | Startup cost | Tested in CI | Verdict |
| --- | --- | --- | --- |
| **fsnotify, one watch per directory** | Reads the directory tree once (directory entries only, no file content) | Linux backend runs in CI; the library tests its Windows backend upstream | Chosen |
| fsnotify on Linux, own recursive `ReadDirectoryChangesW` backend on Windows | No walk on Windows | The own backend has no CI runner: this repository builds Windows only by cross-compiling (`task build-windows-check`) | Rejected: untested platform code for a startup saving the Issue does not ask for |
| Periodic re-read of the tree | None at startup, a full read on every tick | n/a | Rejected: `要件 2` forbids it |
| fanotify filesystem marks (Linux) | No walk | n/a | Rejected: needs `CAP_SYS_ADMIN`, which the hosting compose file does not grant |

**Rationale**: fsnotify v1.10.1 has no public recursive watch, so a new
directory is watched when its create event arrives, and the startup walk reads
directories only. Change notifications do not cross NFS or SMB mounts made
inside the container; the Issue puts those environments out of scope.

## R-2: A change marks a directory dirty, and only dirty directories are re-read

**Decision**: An event on a file marks its parent directory dirty
(non-recursive); an event on a directory (create, remove, rename) marks that
directory dirty with its whole subtree. A batch re-reads exactly the dirty
directories.

| Option | Directory moved or deleted | Verdict |
| --- | --- | --- |
| **Dirty directories** | One event on the directory covers every file below it | Chosen |
| One event per file | Removing a directory reports only the directory, so the files below are never seen as gone | Rejected |
| Re-read the whole media folder | n/a | Rejected: that is a full scan, which `要件 3` keeps manual |

**Rationale**: Removal is decided per dirty directory: a location the index
holds directly in a dirty directory (or anywhere under a dirty subtree) and that
the batch does not find loses that location. Locations outside the dirty set are
never touched, which today's `removeMissing` cannot guarantee because it
assumes a walk of every root.

## R-3: A batch waits for quiet, imports settled files, then removes

**Decision**: A batch starts 2 s after the last event. A file is imported only
when its size and modification time have not changed for 10 s; an unsettled
file keeps its directory dirty for the next batch. In each batch every addition
is applied before any removal.

| Option | A file still being copied | Verdict |
| --- | --- | --- |
| **Size and time unchanged for 10 s** | Skipped and retried until it settles | Chosen |
| Close-write events | fsnotify marks them unportable, and Windows has none | Rejected |
| Import on the first event | Hashes a partial file, then reports `IssueChangedDuringImport` | Rejected |

**Rationale**: Applying additions first means a file moved between two
directories gains its new location before the old one is removed, so the video
(and its tags, playback position and generated files) is never orphaned. The
10 s window is long enough for a local or LAN copy that pauses between buffers;
a copy that stalls longer is caught by the scanner's existing before-and-after
check and imported once it settles.

## R-4: A watch batch is a scan row with origin `watch`

**Decision**: Each batch runs as a scan with `origin = 'watch'` through the
existing scan machinery; the screens hide its progress and notify only a
`partial` or `failed` result.

| Option | Progress, issues, jobs, events | Verdict |
| --- | --- | --- |
| **Scan row with an origin** | Reused unchanged; one running scan stays a database guarantee | Chosen |
| Imports outside any scan row | Needs a second issue list, tally and exclusion rule, and jobs would attach to the last manual scan and turn its indicator back on | Rejected |

**Rationale**: The `Scan` the bottom-right indicator and Settings read already
carries everything a failure notice needs; the origin is the only fact missing.

## R-5: A manual scan or a media folder change supersedes a watch batch

**Decision**: Starting a manual scan, or adding, replacing or deleting a media
folder, stops a running watch batch at the next file, skips its removals and
closes it `done`. Directories dirtied while a manual scan runs are re-read after
it finishes; after a folder change, the batch's dirty set is queued again.

| Option | What the owner sees | Verdict |
| --- | --- | --- |
| **Supersede the batch** | The manual action starts at once | Chosen |
| Answer `scan running` as today | An error caused by an import the owner cannot see | Rejected |
| Wait for the batch to finish | A delay of up to the batch's length with no explanation | Rejected |

**Rationale**: A full scan re-reads everything the batch would have, so dropping
the rest of the batch loses nothing; skipping removals keeps a half-read
directory from losing locations it did not reach.

## R-6: Lost events and watch limits are reported, never repaired by a scan

**Decision**: When notifications overflow, a watch cannot be added (the inotify
watch limit, a permission error) or a media folder becomes unreachable, the
watcher records a watch problem that Settings shows with the next step (run a
manual scan, or raise the limit); it never starts a full scan.

**Rationale**: `要件 3` keeps every full scan manual, and after an overflow only a
full read can say what changed. The inotify limit on the CI host is 130,054
watches and modern kernels scale it with memory; a library with more
directories than that is told so instead of silently missing changes.

## R-7: The setting is a stored key that defaults to on

**Decision**: `settings.library.auto_import` holds `true` or `false`; an absent
key reads as on. Turning it on arms the watches (the directory walk of R-1) and
starts no scan; turning it off removes every watch and drops the dirty set.

| Option | Verdict |
| --- | --- |
| **Key in `settings`, absent means on** | Chosen: follows `desktop.lan_access`, and existing installs get the Issue's default without a migration |
| Environment variable | Rejected: the owner toggles it in Settings (`要件 6`) |

## R-8: An interrupted watch batch is closed, not resumed

**Decision**: At startup an interrupted watch batch is closed like any
interrupted scan, but `ResumeInterrupted` restarts only a manual one; a running
watch batch does not make the desktop app ask before closing.

**Rationale**: Resuming would re-read directories at startup, which the Issue
puts out of scope; changes made while VVMDM was stopped belong to the manual
scan. Additions-first ordering (R-3) leaves no orphaned video when a batch stops
part-way.

## R-9: A finished watch batch applies same-path content changes

**Decision**: A watch batch that closes `done` (and was not superseded) applies
the pending same-path successions, as a `done` full scan does
([030 data-model, Carry-over of content at the same path](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path)).

**Rationale**: The batch contains every change notified up to its start, which
is what the full scan's "all paths seen" gate stands for. Waiting for the next
manual scan would leave a replaced file without its tags and playback position
in the meantime.
