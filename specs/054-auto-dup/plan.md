# Implementation Plan: Judging possible duplicates automatically and bundling the same ones

**Branch**: `feature/054-auto-dup` | **Parent Issue**: #1141

**Input**: The parent Issue. It is this feature's specification.

## Summary

A worker asks the classifier port, for each possible duplicate, whether the two
files show the same picture; "same video" bundles them as the owner's bundling
does, "different videos" dismisses them, and anything else stays on the
Duplicates page. Video cards gain a versions mark, read from the `versions`
summary that list items now carry.

The diagram shows one candidate from the fingerprint to its outcome.

```mermaid
flowchart LR
  fp[Fingerprint job] --> cand[(Candidates)]
  cand --> worker[Judging worker]
  worker -->|4 frame pairs| port[Classifier port]
  port -->|probability| worker
  worker -->|0.8 or more| bundle[Bundle]
  worker -->|0.2 or less| dismiss[Different videos]
  worker -->|between, or error| list[Stays on Duplicates]
```

| Concern | Decision |
| --- | --- |
| Model | One yes/no question through `app.Classifier` ([R-1](research.md#r-1-one-yesno-question-through-the-existing-classifier-port)) |
| Input | 4 images, each a frame of A beside the frame of B at the same position ([R-2](research.md#r-2-the-images-are-four-side-by-side-frame-pairs)) |
| Verdict | 0.8 or more same, 0.2 or less different, fixed ([R-3](research.md#r-3-the-question-wording-and-two-fixed-thresholds)) |
| Queue | Judgement columns on the candidate row; a null judgement waits ([R-4](research.md#r-4-the-judgement-is-columns-on-the-candidate-row)) |
| Failures | Retry after 10 minutes, 1 hour, 6 hours, then daily ([R-5](research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends)) |
| Conflicts | No bundle across a dismissed or unbundled pair ([R-6](research.md#r-6-a-same-pair-joins-two-sides-unless-a-pair-across-them-is-known-to-differ)) |
| Representative | The bundle's, else more pixels ([R-7](research.md#r-7-the-representative-is-the-bundles-else-the-larger-picture)) |
| Removing a version | Records the pairs, never judged again, candidates rebuilt ([R-8](research.md#r-8-removing-a-version-records-the-pairs-and-rebuilds-its-candidates)) |
| Card mark | `versions` on every screen-API `Video`, one query per page ([R-9](research.md#r-9-list-items-carry-the-existing-versions-summary-read-per-page)) |
| Duplicates page | Refetches on a new `candidates` notification ([R-10](research.md#r-10-the-duplicates-page-refreshes-on-a-candidates-notification)) |
| Model | `clef-flash` stays the default; Settings names `clef` for bundling ([R-11](research.md#r-11-the-default-model-is-left-as-it-is-and-settings-names-the-one-that-bundles)) |

## How it works

The judging runs after the existing import, beside it, and changes nothing
about how candidates are found. The sequence shows one candidate from the
scan to its outcome.

```mermaid
sequenceDiagram
  participant Scan as Scan and ingest
  participant Store as Store
  participant Judge as VersionJudge worker
  participant Art as Artifacts
  participant AI as Classifier (Ollama)
  Scan->>Store: fingerprint job writes candidate A–B
  Store-->>Judge: ProcessingChanged (wake)
  Judge->>Store: claim one waiting candidate
  Judge->>Art: 4 frames of A and of B, side by side
  Art-->>Judge: 4 JPEGs
  Judge->>AI: "same picture left and right?"
  AI-->>Judge: probability
  Judge->>Store: apply the result
  Store-->>Judge: VersionCandidatesChanged (screens refresh)
```

1. **A candidate appears as today.** The `fingerprint` ingest job compares the
   new video's fingerprint with the others and writes a row to
   `video_version_candidates` (030). The row now starts with its judgement
   null, which means "waiting".
2. **The worker wakes.** `app.VersionJudge` is one goroutine started in
   `cmd/mdm`, like `AutoTagger`. It wakes at startup, when a job finishes
   (`ProcessingChanged`), when candidates change and when the setting is
   saved. While the setting is off it does nothing.
3. **It claims one candidate.** The store picks the oldest waiting candidate,
   or a failed one whose retry time has passed, skips pairs the owner removed
   from versions, and marks it `running`.
4. **It builds the images.** `internal/artifacts` cuts 4 frames, at the middle
   of 4 equal parts, from each video's seek preview, the same sheet the
   fingerprint was built from, and puts frame `i` of A beside frame `i` of B
   in one JPEG. No ffmpeg runs and no media file is opened.
5. **It asks one question.** The 4 images and the question go to
   `app.Classifier`, the port auto-tagging uses, so the Ollama connection
   under Settings serves both. The answer is a probability.
6. **The store applies the result in one transaction.**

| Probability | What the store does | What the owner sees |
| --- | --- | --- |
| 0.8 or more | Bundles A and B with `VersionStore.Bundle`'s steps; the representative is the bundle's if one side is bundled, else the larger picture | One card with the versions mark |
| 0.2 or less | Writes a "different videos" dismissal, as the Duplicates page's button does | The pair leaves Duplicates |
| Between | Marks the candidate `undecided`; it is never asked again | The pair stays on Duplicates |
| No answer or an error | Marks it `failed` and sets a retry time: 10 minutes, 1 hour, 6 hours, then daily | The pair stays on Duplicates until a retry decides it |

A "same" that would put two videos the owner called different into one
bundle is stored as `undecided` instead.

**Removing a version.** "Remove from versions" on the video page records the
removed video paired with each remaining member in `video_version_unbundles`
and rebuilds its candidates, so the pair is back on Duplicates and the worker
skips it from then on.

**The card mark.** List responses fill the `versions` summary the video page
already gets, for the whole page in one query, and the card shows a small mark
when it is present.

**The model.** In the trial, `clef` answered 0.97 for the same footage and
0.02 for a video that only began the same way. `clef-flash`, the default,
answered 0.29 and 0.09: it removes clearly different pairs but bundles
nothing, so Settings says that bundling needs `clef`.

The reasons for each choice and the alternatives are in
[research.md](research.md); the tables and rules are in
[data-model.md](data-model.md).

## Technical Context

**Canonical definitions**:

| Topic | Source |
| --- | --- |
| Boundaries, store roles, events after commit | [ARCHITECTURE.md](../../ARCHITECTURE.md), [.golangci.yml](../../.golangci.yml) (depguard) |
| Fingerprints, candidates, dismissals, bundles, `Unbundle` | [030 data-model](../030-video-versions/data-model.md), [030 research](../030-video-versions/research.md) |
| Classifier port, Ollama provider and its settings | [ai-layer.md](../../docs/design-docs/ai-layer.md) |
| Queue and single-worker shape, seek frames as model input | [auto-tagging.md](../../docs/design-docs/auto-tagging.md) |
| Screen API and generated code | [api/openapi.yaml](../../api/openapi.yaml), `task generate` |
| Screens and Web tests | [design-system.md](../../docs/design-docs/design-system.md), [web-testing.md](../../docs/design-docs/web-testing.md), [i18n.md](../../docs/design-docs/i18n.md) |
| Checks | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`) |

**Feature-specific context**: the thresholds rest on a synthetic trial, and
the [quickstart](quickstart.md) repeats it on real files before the last unit
merges. The migration takes the next number on `main` when its unit is
written, since other feature branches add migrations too.

## Constitution Check

Checked against ARCHITECTURE.md and AGENTS.md; the rules this design leans on:

| Rule | How the design meets it |
| --- | --- |
| The domain decides, the store enforces | Thresholds, the conflict check, the representative and retry times are `internal/domain`; the store re-reads members and dismissals inside the bundling transaction |
| Only a scan reads the media folders' files | The model sees seek-preview frames from the data directory |
| User data survives a rebuild | `video_version_unbundles` is keyed by content keys, listed as user data |
| No copied code | The automatic bundle and dismissal call the steps of `VersionStore.Bundle` and `Dismiss` |

## Project Structure

### Documentation (this feature)

```text
specs/054-auto-dup/
├── plan.md
├── research.md            # R-1..R-11
├── data-model.md          # Two tables, the rules, the setting
├── quickstart.md          # The real-model check
└── contracts/
    └── screen-api.md      # Video.versions on lists, the setting resource
```

The `design` stage adds `ui-design.md` for the card mark and the Settings
section.

### Source Code

**Affected boundaries**:

| Package | Change |
| --- | --- |
| `internal/domain` | Verdict, conflict check, representative choice, retry times |
| `internal/store` | Migration, judgement claim and results, `VersionCandidatesChanged`, `Unbundle`, successions re-keying, the setting, `versions` on listed videos |
| `internal/artifacts` | Side-by-side frame pairs from two seek previews |
| `internal/app` | `VersionJudge`: settings, worker, wake-up |
| `internal/httpapi`, `api/openapi.yaml` | The settings resource; `versions` on list items; the `candidates` notification |
| `cmd/mdm` | Wiring the worker and its wake-ups in `events.go` |
| `web/src/settings`, `web/src/videoList`, `web/src/versions` | The Settings section; the card mark; the Duplicates page refetch |

**New paths**: `internal/app/version_judge.go`,
`internal/store/version_judgements.go`, and
`docs/design-docs/duplicate-judging.md`.

**Structure decision**: The worker is its own use case beside `AutoTagger`,
not a job kind in `jobs`: a judgement is about a pair, and the job queue's
claim and uniqueness rules are per video.

## Implementation Work

The diagram shows which units have to land first. The screen units also wait
for the `design` stage's `ui-design.md`. The real-model run of the
[quickstart](quickstart.md) is the last unit's, since it needs the switch and
the mark.

```mermaid
flowchart LR
  store[Store judgements] --> worker[Judging worker]
  store --> refresh[Duplicates refresh]
  mark[Versions mark on cards]
  worker --> settings[Settings switch]
  mark --> settings
```

### Store duplicate judgements and the pairs removed from versions

**Scope**: The migration with the candidate columns and
`video_version_unbundles`; the domain rules (verdict, conflict check,
representative, retry times) and `VersionCandidatesChanged`; the store's claim,
recovery and three results with the conflict check; the
`version_judging.enabled` setting's read and write; `Unbundle` recording pairs
and rebuilding candidates; successions re-keying `video_version_unbundles`;
and the user-data list in ARCHITECTURE.md and running-vv.md
([data-model.md](data-model.md),
[R-4](research.md#r-4-the-judgement-is-columns-on-the-candidate-row) to
[R-8](research.md#r-8-removing-a-version-records-the-pairs-and-rebuilds-its-candidates)).

**Dependencies**: None.

**Acceptance**: `task check` and `task check-docs` pass; store tests show: a
"same" result bundles the pair with the R-7 representative and the candidate
is gone; a "same" for a new video and a bundle member adds it to that bundle,
also after that bundle was split and bundled again by hand; "different" writes
a dismissal and removes the candidate; a "same" across a dismissed pair leaves
the candidate `undecided`; a pair in `video_version_unbundles` or `undecided`
is not claimed; a `failed` row is claimed only after its `retry_at`, and its
`attempts` survive recovery; `Unbundle` writes the pairs and the matching
candidate is listed again; each result publishes `VersionCandidatesChanged`.

### Judge possible duplicates with the classifier

**Scope**: Side-by-side frame pairs in `internal/artifacts`; `app.VersionJudge`
(claim one at a time while the setting is on, ask the R-3 question, apply the
result, wait for the next `retry_at`); a log line with each pair's
probability; wiring and wake-ups (startup, `ProcessingChanged`,
`VersionCandidatesChanged`, settings saved); `docs/design-docs/duplicate-judging.md`
linked from the index; and the AI-layer and subsystem-map entries
([R-1](research.md#r-1-one-yesno-question-through-the-existing-classifier-port)
to [R-5](research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends)).

**Dependencies**: Store duplicate judgements and the pairs removed from
versions.

**Acceptance**: `task check` and `task check-docs` pass; app tests with a fake
classifier show 0.85 bundles, 0.1 dismisses, 0.5 leaves the candidate, an error
or a reply without the question's key sets a `failed` row with its retry time, and nothing is claimed while the
setting is off; an artifacts test shows 4 JPEGs each twice the width of one
frame.

### Refresh the Duplicates page when candidates are judged

**Scope**: The `candidates` notification on `/api/events` for
`VersionCandidatesChanged`, and the Duplicates page refetching on it
([contracts/screen-api.md, `GET /api/events`](contracts/screen-api.md#get-apievents-candidates),
[R-10](research.md#r-10-the-duplicates-page-refreshes-on-a-candidates-notification)).

**Dependencies**: Store duplicate judgements and the pairs removed from
versions.

**Acceptance**: `task check` passes with no diff from `task generate`; a
handler test shows the event reaches an owner's event stream as `candidates`;
a Web test shows the open Duplicates page refetches on `candidates` and drops
a pair that is gone.

### Show a versions mark on video cards

**Scope**: `versions` filled on every screen-API `Video` for a bundle member,
read once per page; the `api/openapi.yaml` description; the mark on cards and
list rows from `ui-design.md`; and the superseded card rule in 030
ui-design.md
([contracts/screen-api.md, `Video` delta](contracts/screen-api.md#video-delta),
[R-9](research.md#r-9-list-items-carry-the-existing-versions-summary-read-per-page)).

**Dependencies**: None.

**Acceptance**: `task check` and `task check-docs` pass; store tests show the
library, video list, folder and group reads give a bundle's representative
`versions.count` 2 and an unbundled video none; the library, search, folder,
group and home cards of a bundled video show the mark and a plain video's do
not, in a visual review at the widths in `ui-design.md`.

### Turn automatic duplicate judging on and off in Settings

**Scope**: `GET` and `PUT /api/settings/version-judging`, saving that wakes the
worker, and the Settings section from `ui-design.md` with its note on the
model ([contracts/screen-api.md](contracts/screen-api.md),
[R-11](research.md#r-11-the-default-model-is-left-as-it-is-and-settings-names-the-one-that-bundles)).

**Dependencies**: Judge possible duplicates with the classifier; Show a
versions mark on video cards.

**Acceptance**: `task check` passes with no diff from `task generate`; handler
tests show `GET` answers `enabled: false` before any save and `PUT` answers the
saved value, and a request with no owner session gets
`401 unauthenticated`; a Web test shows the switch saving; every
[quickstart](quickstart.md) row passes with `clef` and the PR records each
pair's probability; the Settings section passes a visual review at the widths
in `ui-design.md`.
