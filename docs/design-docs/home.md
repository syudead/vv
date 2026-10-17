# Home shelves and the rule organizer

The home is a list of shelves, and each shelf is a smart folder with a home
position; the rule organizer adds shelves for well-used tags, people and
organizations without AI. The organizer is
[`internal/app/home_organizer.go`](../../internal/app/home_organizer.go), its
choice of shelves is
[`internal/domain/home_rules.go`](../../internal/domain/home_rules.go), and
the apply transaction is [`internal/store/home.go`](../../internal/store/home.go).
The decisions behind this document are in the
[057 research](../../specs/057-home/research.md).

The diagram shows what starts a run and what a run reads and writes.

```mermaid
flowchart LR
  owner[Reorganize] -->|rebuild| pending[(home.organize.pending)]
  scan[Scan done] --> drain{Queues empty 2 min?}
  drain -->|first run| pending
  drain -->|20+ unshelved| pending
  pending --> run[Organizer run]
  run --> rules[Rule organizer]
  rules --> apply[Apply proposal]
  apply --> shelves[(smart_folders)]
  run --> last[(home.organize.last)]
  run --> event[HomeChanged]
```

## Shelves from tags, people and organizations

The rule organizer proposes one shelf per tag, person or organization on at
least 5 shown videos, named after it, at most 24 in a run
([R-2](../../specs/057-home/research.md#r-2-the-rule-organizer-makes-shelves-from-tags-people-and-organizations)).
Five videos keep a shelf from being one card, and a tentative tag is left out
because it is the auto-tagger's guess, not a tag the owner accepted.

| Source | Shelf rule | Order |
| --- | --- | --- |
| Tag with a kind | `{"tags": [id]}` | First, by kind id, then by video count |
| Tag without a kind | `{"tags": [id]}` | Second, by video count |
| Person | `{"person": id}` | Third, by video count |
| Organization | `{"organization": id}` | Last, by video count |

A count is the number of videos the shelf's rule would show the owner, so a
tag counts both tags added by hand and tags from folder names.

## Rebuild and extend

A rebuild proposes from every shown video and replaces the organizer's
shelves; an extend proposes only from unshelved videos and only adds
([R-8](../../specs/057-home/research.md#r-8-a-rebuild-replaces-the-organizers-folders-in-one-transaction-an-extend-only-adds)).
Neither run changes a shelf the owner added, moved or kept, or a shelf
onboarding made from a picked interest, because both have `owner_kept` 1.

| Run | Started by | Proposes from |
| --- | --- | --- |
| `rebuild` | `Reorganize` (`POST /api/home/organize`), or the first scan when no run has finished | Every shown video |
| `extend` | A scan that ends when 20 or more shown videos are unshelved | Unshelved videos only |

## One run at a time

At most one run waits, in the setting `home.organize.pending`, and a rebuild
absorbs a waiting extend
([R-9](../../specs/057-home/research.md#r-9-one-run-at-a-time-queued-in-settings-triggered-by-events)).
The setting keeps the waiting run across a restart, and a run stopped by
shutdown stays in it, so the next start runs it again.

The diagram shows the organizer's states.

```mermaid
stateDiagram-v2
  [*] --> Idle
  Idle --> Pending: Reorganize or scan end
  Pending --> Running: no run going
  Running --> Idle: finished or failed
  Running --> Pending: rebuild asked meanwhile
```

A finished run writes `home.organize.last` with its outcome (`done`,
`failed`, or `unchanged` when the proposal changed nothing) and the numbers
of shelves added and removed. Each change of state publishes `HomeChanged`,
so open screens read the home again.

A storage error does not drop a run. When the organizer cannot read the
waiting run at startup, or cannot write `home.organize.last` after a run, it
reads `home.organize.pending` again at each queue poll (10 seconds) and runs
what it finds, so a run whose result was not recorded runs again. The setting
is cleared only after the result is written.

## Waiting for the queues after a scan

A scan-triggered run waits until the job queue and the auto-tag queue have
both stayed empty for 2 minutes after a scan ends as `done`
([R-10](../../specs/057-home/research.md#r-10-an-extend-runs-when-20-or-more-shown-videos-are-unshelved)).
New videos get their tags from auto-tagging after the scan, so an earlier run
would count every new video as unshelved before it had a tag. A failed or
cancelled scan starts nothing.

A check of the queues that fails counts as not empty, so the 2 minutes start
again after the next successful check. When the decision or the queueing
after the 2 minutes fails, the organizer keeps waiting for that scan and
tries again at the next poll.

| Rejected | Why |
| --- | --- |
| A kind in the `jobs` table | `jobs` rows belong to one video and its location |
| A timer every few hours | Requirement 8 ties adding shelves to the end of a scan |
| A share of the library for the extend threshold | At 20,000 videos it waits for 2,000 unshelved videos |

## The owner's fixed shelves

After the folder shelves, the owner's home lists continue watching, recently
added, unshelved and recommended; a guest gets only recently added and
unshelved, because the watch history is the owner's only
([R-13](../../specs/057-home/research.md#r-13-four-fixed-shelves-two-of-them-the-owners-only)).

- **Continue watching** is the library's `watch=inProgress` list sorted by
  the latest play, read by [`internal/store/home_reads.go`](../../internal/store/home_reads.go).
  Its held items are dropped on every saved playback position, because a play
  that keeps a video in progress changes the order without any notice.
- **Recommended** is listed only when a seed exists: up to 10 videos played
  most in the last 90 days and up to 10 latest favorites
  ([`internal/store/home_recommend.go`](../../internal/store/home_recommend.go)).
  [`internal/app/home.go`](../../internal/app/home.go) ranks the shown videos
  that are neither a seed nor completed by their highest closeness to any
  seed, with the related videos' `RelatedScorer`, keeps the first 24, and
  holds that order for 10 minutes. Scoring 20,000 videos against 20 seeds
  takes seconds on one core, so the candidates are scored in parallel, and a
  shelf read computes the ranking alongside the store's read of the other
  shelves.

| Rejected | Why |
| --- | --- |
| The sum of closeness to all seeds | Videos near one large cluster of seeds fill the whole shelf |
| Ranking on every read | One pass reads every video's clues, which does not fit the home's 1 second |
| Continue watching held until the next notice | Playing a video in progress publishes nothing, so the shelf would keep the old order for 10 minutes |
