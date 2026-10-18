# Research: import pipelines in the activity log

Inherited decisions: the activity log
([activity.md](../../docs/design-docs/activity.md)), auto-tagging entries
([auto-tagging.md](../../docs/design-docs/auto-tagging.md#activity-entries-and-undo)),
pair entries
([duplicate-judging.md](../../docs/design-docs/duplicate-judging.md#each-pair-keeps-one-activity-entry))
and the store-role rules in [ARCHITECTURE.md](../../ARCHITECTURE.md#cross-cutting-invariants).
This file records only the decisions this feature adds.

## R-1: The parent is an activity entry of kind pipeline

**Decision**: An import pipeline is an `activity_entries` row of the new kind
`pipeline` (feature `import`, subject the video), and the existing `import`
entry becomes its member for the five ingest stages.

| Option | List, paging, filters, SSE | Retry of a failed import | Verdict |
| --- | --- | --- | --- |
| **New `pipeline` entry** | Reuses the id cursor, filters and counts of `activity_entries` | The retry's new `import` entry joins the same parent | Chosen |
| A separate `activity_pipelines` table | A second list query and cursor merged with entries | Same | Rejected: two sources for one newest-first list |
| The `import` entry as the parent | Reused | A retry opens a new `import` entry, so the failure and the retry land in different parents | Rejected: breaks tracing a retry inside one run |

**Rationale**: The parent has the same life as an entry (status, decider,
subject, time) and must page with the standalone entries, so it is one.

## R-2: Membership is a link table

**Decision**: `activity_pipeline_members(pipeline_id, entry_id)` links members;
an entry can belong to more than one pipeline.

| Option | A pair seen by two concurrent imports | Verdict |
| --- | --- | --- |
| **Link table** | One entry, linked from both pipelines | Chosen |
| `parent_id` column on `activity_entries` | Needs a copy of the pair entry, so retry and undo run twice | Rejected: the parent Issue forbids duplicate processing for display |

**Rationale**: A `version_judgement` entry is one per pair and spans two videos.

## R-3: A new import entry opens a pipeline; continuations join it

**Decision**: Writing a new `import` entry opens a new pipeline and makes the
entry its first member, except when the entry continues another one, which
joins that entry's pipelines.

| New entry | Pipeline |
| --- | --- |
| Scan, watch scan, re-probe, a move written as `done` | New |
| Retry from the activity log (`import`, `auto_tag`) | The retried entry's pipelines |
| Claim that reopens an interrupted entry (`import`, `auto_tag`) | The superseded entry's pipelines |
| Auto-tagging queued after the seek stage | The pipeline of the finishing job's `import` entry ([R-4](#r-4-after-import-auto-tagging-is-queued-in-the-seek-stage-transaction)) |
| A pair entry opened by a fingerprint result | [R-5](#r-5-a-pair-entry-joins-the-open-pipelines-of-its-two-videos) |
| Settings or video page tagging, judging turned on, anything owner-started | None: listed standalone |

**Rationale**: The parent Issue asks for one parent per run and forbids folding
a later, separately started run into an earlier import. A job that joins a
`running` import entry already joins its pipeline through that entry.
Alternative rejected: grouping by video and time window, which is the inferred
grouping the parent Issue rules out.

## R-4: After-import auto-tagging is queued in the seek stage transaction

**Decision**: The transaction that finishes the seek stage for good (done, or
failed with no retry left) queues the video for auto-tagging when
`auto_tag.feature_enabled` and `auto_tag.after_import` are on and the video has no
queue row, and links the new `auto_tag` entry to the pipeline. The
`VideoIngestChanged` subscriber only wakes the worker.

| Option | Parent between seek done and the queue write | Verdict |
| --- | --- | --- |
| **Queue in the seek transaction** | The member exists before the import member can finish | Chosen |
| Keep the event subscriber, pass the entry id along | Preview and fingerprint can finish first, so the parent shows `done` and then reopens | Rejected: the parent Issue forbids a parent that looks done while work remains |
| Record an "auto-tag expected" flag on the pipeline | Duplicates the queueable rule in a second place | Rejected |

**Rationale**: The store already reads `version_judging.enabled` inside the
fingerprint transaction to open pair entries; this follows that precedent and
the chained stages (preview, fingerprint) that are queued in their
predecessor's result transaction. A disabled feature queues nothing, so the
parent never waits for it.

## R-5: A pair entry joins the open pipelines of its two videos

**Decision**: When the fingerprint transaction opens a new `version_judgement`
entry, the entry joins every not-yet-finished pipeline of either key. Reattaching
an existing open entry adds no membership.

| Option | Verdict |
| --- | --- |
| **Open pipelines of both keys** | Chosen: two concurrent imports both see the pair |
| Only the pipeline of the fingerprint job | Rejected: the other import's parent hides a pair it is part of |
| Every pipeline of both keys, finished ones included | Rejected: reopens a past import, which the parent Issue forbids |

**Rationale**: Judging off opens no pair entry, so nothing waits; turning
judging on later opens standalone entries (R-3).

## R-6: Parent status is stored and rolled up in the member's transaction

**Decision**: Every write of a member entry re-rolls its pipelines in the same
transaction, through the package-private helper that writes entries; the
pipeline row carries the result.

| Option | Verdict |
| --- | --- |
| **Stored roll-up** | Chosen: `status` filter, counts and paging stay single SQL reads |
| Computed on read | Rejected: the status filter and counts would need the roll-up in SQL for every page |

**Rationale**: The `import` entry already rolls up its jobs this way
(`settleImportActivity`), so the same transaction rule holds one level up.
The roll-up rule is in [data-model.md](data-model.md#state-transitions).

## R-7: The list pages top-level entries

**Decision**: `GET /api/activity` lists pipelines and entries that belong to no
pipeline; a filter matches a pipeline when the pipeline or any member matches,
and counts count listed entries.

| Option | Verdict |
| --- | --- |
| **Top-level unit, member match** | Chosen: one count per run, and a matching child is reached through its parent |
| Flat list with members beside parents | Rejected: members fill the list and are counted twice |
| Filters on the parent only | Rejected: a search for a failed auto-tag would find nothing |

**Rationale**: The parent Issue asks for one display unit and a clear count
unit; the matched members are marked so the screen can point at them.

## R-8: The import member keeps its stage breakdown

**Decision**: Settling an `import` entry writes one `activity_entry_stages` row
per stage it holds, with state and the stage's message.

| Option | Verdict |
| --- | --- |
| **Persisted stage rows** | Chosen: readable after the video is deleted |
| Read `jobs` at read time | Rejected: deleting the video cascades its jobs, and a requeue deletes finished jobs |
| One entry per stage | Rejected: retry and the import roll-up work per video, so five entries would re-split both |

**Rationale**: The parent Issue requires the detail to stay readable after the
target video is deleted.

## R-9: The detail is one owner-only read

**Decision**: `GET /api/activity/{id}` returns the entry with its members (for a
pipeline), its pipelines (for a member) and its stage rows; the list carries a
short member summary only.

| Option | Verdict |
| --- | --- |
| **Separate detail read** | Chosen: the list stays one card per run |
| Everything in the list | Rejected: full messages and stage rows for 50 cards per page |

**Rationale**: The store already reads one entry (`ActivityStore.ActivityEntry`);
the endpoint exposes it.

## R-10: Earlier entries stay standalone

**Decision**: The migration creates no pipeline and links nothing; an `import`
entry still `running` at upgrade keeps its jobs and stays standalone, and the
detail of an earlier entry reports no stage rows instead of an empty breakdown.

**Rationale**: The parent Issue forbids inferring groups for history and
showing detail that was never recorded. Alternative rejected: backfilling
pipelines from `jobs.activity_id`, which only covers unfinished jobs.
