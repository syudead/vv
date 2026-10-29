# Judging possible duplicates automatically

VVMDM judges each possible duplicate, one pair at a time, first from the two
stored fingerprints and only then, for a section where the frames differ, by
asking the classifier port. Each pair's work is a `version_judge` job that the
[job scheduler](job-scheduling.md) grants. The handler is
[`internal/app/version_judge.go`](../../internal/app/version_judge.go), the
frame rule is
[`internal/domain/version_frames.go`](../../internal/domain/version_frames.go),
the jobs are
[`internal/store/version_judge_jobs.go`](../../internal/store/version_judge_jobs.go),
and the results are
[`internal/store/version_judgements.go`](../../internal/store/version_judgements.go).

The diagram shows one candidate from the claim to its result.

```mermaid
flowchart LR
  claim[Claim the next version_judge job] --> rule{Frames compared by time}
  rule -->|all match| same[Same video]
  rule -->|cannot be compared| undecided[Undecided]
  rule -->|a run differs| conn{Ollama connection saved?}
  conn -->|no| noconn[no_connection]
  conn -->|yes| ask[Ask the classifier about the run]
  ask -->|0.8 or more| same
  ask -->|0.2 or less| different[Different videos]
  ask -->|between| undecided
  ask -->|error or no answer| failed[failed, retry later]
```

## Versions the owner would not tell apart are the goal

Bundling is for files the owner sees as one video: a re-encode, black bands, a
watermark or channel logo, burned-in subtitles, or a start shifted by a few
seconds. Names, folders and other stored facts are not compared; they tell
whether two files are the same file, which is a different question.

| Difference | Where it is absorbed |
| --- | --- |
| Quality, resolution, black bands, a small logo | The fingerprint: 32×32 low-frequency luma hashes with black bands trimmed |
| A large watermark, burned-in subtitles, a shift of a few seconds | The classifier, whose question says to ignore them |
| Frames the classifier is unsure about | The files' own properties, below |

## The frame rule decides first

Frames are paired by time and flat frames are skipped, as the candidate check
pairs them, and a pair differs when its distance is over 12
(`FingerprintMatchMaxDistance`). The median the candidate check uses hides a
replaced section; the per-frame list shows it.

| Frames | Result | Frames shown to the model |
| --- | --- | --- |
| At most 5% differ, never two in a row | Same video, no request | None |
| Two or more in a row differ | Ask the model | The longest run (the earlier one on a tie) |
| Over 5% differ, none in a row | Ask the model | All differing frames |
| Fewer than 3 comparable pairs, or different fingerprint versions | Undecided, left to the owner | None |

More than 4 frames are spread across the run, so a request carries at most 4
images.

## The model sees the frames, not the names

Each image is frame A on the left and the paired frame B on the right, cut
from the two seek previews by
[`internal/artifacts`](../../internal/artifacts/seek_frames.go), with B scaled
to A's frame size. The request carries no facts: no file name, folder or
duration. The question asks whether both halves show the same scene of the
same video, ignoring quality, colour, cropping, black bands, watermarks, logos,
burned-in subtitles and a few seconds of timing; asking whether the pictures
are the same let a watermark or subtitle mark one video different. It and the
thresholds 0.8 and 0.2 are fixed in `internal/domain`.

| Rejected | Why |
| --- | --- |
| Frames from the media files with ffmpeg | Decoding both files costs far more than cutting the frames from the two seek previews, which already hold them at the paired times |
| A reply without the question's key as probability 0 | It would dismiss a pair the model never judged |

## The owner sees why

Each judgement stores its frame rule result on the pair's activity entry, in
`activity_entries.evidence` as JSON of `domain.VersionJudgeEvidence`, so the
reason survives the candidate row that bundling or dismissal removes. The
Duplicates screen reads it through `GET /api/version-candidates`, which also
returns each pair's judgement state, and the activity entry's `Why` section
reads it from the entry. A waiting pair that the claim skips for a
`video_version_unbundles` or `video_version_judging_holds` row is listed as
`held`, so the screen says it is left to the owner instead of waiting forever.

| Field | Shown to the owner as |
| --- | --- |
| `rule`, `compared`, `differing` | How many frames were compared by time and how many differ |
| `longestRun`, `runStartMs`, `runEndMs` | The section that differs throughout, on the left video's timeline |
| `versionMismatch` | Incomparable because the fingerprints come from different fingerprint versions |
| `conflict` | A same verdict left undecided because a side is already marked different from the other |
| `frames` | The frame pairs shown to the classifier, with their times on both videos |

`GET /api/activity/{id}/judgement-frames/{index}` cuts the indexed pair from
the two current seek previews, as the request to the classifier did, instead
of storing the images: the evidence is a few hundred bytes per pair, and a
stored copy would outlive the seek preview it came from. A content change at
the same path removes `frames` from the pair entries of the old key before
re-keying them, since the frame indexes point into the old content's seek
preview; the counts and the sentences stay.

A pair left on Duplicates by a judgement made before evidence was stored gets
evidence recomputed from the two stored fingerprints when it is read, on
Duplicates and in the entry's detail. The frame rule is deterministic, so the
counts and frames match what the classifier was shown while the fingerprints
are unchanged. A rule match or a same answer left `undecided` can only have
met a known difference across the two sides, so such evidence is marked
`conflict`. The screen's headline names the reason: a conflict, frames that
could not be compared, or the classifier's answer between the thresholds.
A Duplicates row shows only that headline; `Why?` opens a dialog with the
sentences and the frame pairs in one column, so up to four side-by-side
images do not stretch every row of the list.
The screen states the thresholds next to the probability, so a 55% answer
reads as between the 20% and 80% that would decide.

## The owner can judge a pair again

`POST /api/version-candidates/rejudge` returns an `undecided`, `failed` or
`no_connection` pair to waiting with a new job in the owner class, its entry to
`running`/`waiting` and its evidence cleared, and wakes the scheduler
([`internal/app/version_judge_review.go`](../../internal/app/version_judge_review.go)).
It covers the results that leave a pair on Duplicates: an owner who changed
the model or its connection, or rescanned a file, has no other way to ask
again about an undecided pair, which the claim never takes on its own.

| Refused | Why |
| --- | --- |
| Judging turned off (`409 version_judging_off`) | Nothing would claim the pair |
| A waiting or running pair (`409 version_not_rejudgeable`) | It is about to be judged already |
| A bundled or dismissed pair | It has left Duplicates; a force rescan judges automatic decisions again, as described below |

## The files' own properties settle what the frames leave open

The claim reads each side's duration and stored media information, which the
import read from the file itself, so a judgement adds no file reads
([`internal/domain/version_properties.go`](../../internal/domain/version_properties.go)).
A version may differ in resolution, codec, bit rate, audio or subtitle tracks,
so properties only ever say same, and those are not compared.

| Property | Says same when | Weight |
| --- | --- | --- |
| Chapters | Both have 3 or more, the same count, starts within a second, and an equal title, unless the titles differ only by their numbers ("Chapter 1", "Chapter 2") | Strong |
| Chapters | The same starts, with numbered or missing titles | Weak |
| Recording time (`creation_time`) | Equal; a difference says nothing, since many encoders rewrite it | Strong |
| Embedded title | Equal, folded, and not "Untitled" or a number | Weak |
| Duration | Shown only: candidates already have close durations | — |

| Frames and classifier | Strong same | Weak same |
| --- | --- | --- |
| Frames match | Same | Same |
| Frames differ | Same, without asking the classifier | Asked |
| Incomparable, no connection or failed | Same | Unchanged |
| The classifier answers different | Undecided, left to the owner | Different |
| The classifier is unsure | Same | Same when it answered 0.5 or more |

A verdict the properties decide is the rule's, keeping any probability as
evidence, and the evidence marks `byProperties`. File names, folders and work
information are not used: they are often empty at import, and they tell
whether two files are the same file.

## The work is a job per pair

Every waiting candidate has one unfinished `version_judge` job, keyed by the
pair's two content keys (`subject_key` < `subject_key_b`), in the same
transaction that makes the candidate wait. The candidate row keeps only the
judgement; the job holds the class, the attempts and the retry time. A trigger
deletes the job with its candidate row, whichever statement removes the row,
so a rebuilt candidate starts with a new job.

| Queued by | Class |
| --- | --- |
| A candidate being created or rebuilt, including a pair a force rescan lists again | Import |
| A pair returned to waiting: a restart, a saved connection, a verdict whose side left the library | Import |
| A retry from the activity log, or `Judge again` on Duplicates | Owner, ending any retry wait |

The claim takes the lowest class, then the earliest run-after time, among jobs
whose pair is judgeable now. A held, unbundled or unregistered pair keeps its
job queued and is claimed once the hold or the unbundle is undone, or its
location is registered again; `ProcessingChanged` and
`VersionCandidatesChanged` wake the scheduler for that. Each claim is one
attempt.

| Result | The job |
| --- | --- |
| `same`, `different`, `undecided`, `no_connection` | Ends |
| `failed` | Waits 10 minutes, 1 hour, 6 hours, then a day after each later failure, without a limit, through the kind's override of the [retry delay](job-scheduling.md#a-failed-job-retries-after-a-growing-delay) |
| Judging turned off during the judgement, including before an input that cannot be read | Snoozes: queued again at once without counting the attempt, and the pair waits |
| `no_connection` decided while the connection was being saved | Snoozes, so the pair is judged again with the saved connection |

`version_judge` is paused while `version_judging.enabled` is false, so its jobs
keep their attempts until judging is turned on, and turning it on wakes the
scheduler. Saving the Ollama connection returns `no_connection` pairs to
waiting with new jobs. A reset that fails, or whose request ends first, stays
owed: every scheduler round retries it, and a failure wakes the scheduler again
after the worker retry wait. The handler records a `no_connection` result only
after checking, under the same lock, that the connection is still unsaved. The
Duplicates list, the activity log and the API's `retryAt` read a failed pair's
retry time from the `run_after` of its queued job.

| Rejected | Why |
| --- | --- |
| Create the job only when a pair becomes judgeable | Lifting a hold or undoing an unbundle would also have to queue it; the claim already checks eligibility each time |
| Keep `attempts` and `retry_at` on the candidate | Claim order, delays and startup recovery would exist outside the one job table |

The handler asks the store whether an `ollama.endpoint` row exists before a
request, so a pair is never sent to the default address the owner never set.
Each pair's frame-rule outcome, frame counts, verdict and probability go to
the log under `component=duplicate-judging`.

## Each pair keeps one activity entry

Every judgeable candidate has one `version_judgement` entry in the activity
log ([activity.md](activity.md)), opened when the candidate appears while
judging is on, or when judging is turned on. The candidate row's
`activity_id` points to it, and its status follows the row's `judgement`, so
the same entry turns from `no_connection` or `failed` into the result.
The code is
[`internal/store/activity_judgements.go`](../../internal/store/activity_judgements.go).

| Candidate row | Entry |
| --- | --- |
| Waiting, or returned to waiting (connection saved, judging turned off mid-judgement, retry) | `running`/`waiting` |
| Claimed, including an entry left `interrupted` by a restart | `running`/`processing` |
| `undecided`, `failed` (with its retry time), `no_connection` | `attention` with that reason |
| Bundled or dismissed by the judgement | `done`/`same` or `done`/`different` |
| Rebuilt for one of its keys | Attached again to the pair's open entry, found by its two keys |
| Bundled or dismissed by the owner | `done`/`decided_by_owner` |
| Restored by undoing the drag-organize bundle that closed it | The same entry reopens with the row's state |
| Its key's content changed at the same path | Kept open until the succession is decided: re-keyed when it carries over, `done`/`gone` when it does not |
| A key left the library | `done`/`gone` |
| Swept into one bundle by another pair's automatic bundle, or dropped by a rebuild while both keys stay in the library | `done`/`superseded` |

The decider is `ai` with the probability when the model's answer decided, and
`rule` otherwise.
An automatic bundle keeps the joined side's member keys and its former
representative in `undo_keys`, the side whose representative did not become
the bundle's. The representative is the owner's pick when one side has one, else
the better file, and the bundle merges both sides' user data
([video-versions.md](video-versions.md)).

A pair entry that a fingerprint result opens joins the import pipelines of
that run ([activity.md](activity.md#a-pipeline-groups-one-import-run)), so
the owner reaches it from either video's import.

| Pipeline | Joined |
| --- | --- |
| Every pipeline of the finishing fingerprint job's `import` entry | Always |
| The other video's newest pipeline | When it is not `done`, or when the same scan opened it and the triggering pipeline |
| An earlier, finished pipeline of either video from another scan or a re-probe | Never, so a later run does not reopen a past import |

Only a new entry joins. An entry that a rebuild attaches again, or that a
retry returns to waiting, keeps the pipelines it has, which return to
`running` with it. Judging turned off opens no entry, so no pipeline waits for
one; an entry that turning judging on opens later belongs to no pipeline.

## Undo of an automatic decision is held

The owner undoes an automatic `same` or `different` from the activity log in
one `ActivityStore` transaction that re-reads the state first.
When the state has changed, nothing else changes and the `undo` entry says
`already_reverted`. Whether the owner changed it since is read from the log,
not from the rows' timestamps: entry ids are monotonic, so an owner entry on
the same subject with an id greater than the undone entry's `decided_mark`
(the greatest entry id when the judgement wrote its result) means the owner
decided again after the result, even within the same second; an owner change
made while the pair waited or was being judged does not count. For `same`, a joined member with
a later owner `bundle`, `unbundle` or `organize_undo` entry no longer counts
as part of the automatic bundle and the undo leaves it where the owner put
it; when no member is left to split, the undo is `already_reverted`. A
`bundle` entry is written only for members whose membership changed, so the
owner adding a video to the automatic bundle does not hold its undo. For
`different`, the dismissal row does not say who wrote it, so a later owner
`dismiss` entry on the same pair means the owner decided the pair again, and
the undo keeps the dismissal.

| Undone | State change | Keeps automatic judging from deciding it again |
| --- | --- | --- |
| `same` | The joined side leaves the bundle as `Unbundle` removes one video; a side of two or more leaves as one bundle with its former representative and a copy of the merged bundle's user data | `video_version_unbundles`, only for pairs across the two sides |
| `different` | The dismissal is deleted and both keys' candidates are rebuilt, so the pair is listed on Duplicates again | `video_version_judging_holds` |

The claim skips a held pair as it skips an unbundled one, and a content change
at the same path re-keys both tables and every key the entries hold, finished
entries included, so an undo after the change still finds the bundle member
or the tag under the new key.

## Judging again after a force rescan

A forced fingerprint job
([`internal/store/version_rejudge.go`](../../internal/store/version_rejudge.go))
lists its key's automatically decided pairs as candidates again, in the
transaction that stores the fingerprint. A pair is listed when its latest
`version_judgement` decision is a `done` `same` or `different` entry by `rule`
or `ai`, not undone, and still in force under the undo rules above: the
owner has not unbundled, held or decided the pair again since the entry's
`decided_mark`. An owner's own bundle or dismissal has no automatic entry, so
it is never listed. With judging off nothing is listed, since there is no
automatic judgement to redo; an unforced fingerprint lists nothing either.

The listed row carries `rejudge_of`, the id of that entry. The claim takes it
although the pair is bundled or dismissed, and still skips a pair with a
`video_version_unbundles` or `video_version_judging_holds` row. The prior
decision stays in force until the verdict, and the verdict gets a new entry,
so a bundle never dissolves while the pair waits or the model is unreachable.

| Verdict | Prior `same` (bundled) | Prior `different` (dismissed) |
| --- | --- | --- |
| `same` | Nothing changes; the entry takes over the prior's undo keys | The dismissal is deleted, then the pair is bundled; a conflict restores the dismissal and the entry is `undecided` |
| `different` | The pair is dismissed and the joined side leaves as the undo splits it | Nothing changes |
| `undecided`, `failed`, `no_connection` | The bundle stays; the entry shows the reason | The dismissal stays; the entry shows the reason |

The row is removed once the verdict is applied, so a `failed` or
`no_connection` result is not retried; the next force rescan judges the pair
again. The split for `different` writes no `video_version_unbundles` row. That
row protects an owner's `Unbundle` from automatic bundling, and the claim
skips pairs that have it, so writing it here would keep every later force
rescan from judging the pair; the dismissal already keeps the pair apart.
When the owner changes the pair after it was listed, the prior is no longer
in force when the verdict arrives: the row is removed, nothing changes and
the entry closes as `superseded`. Bundling and succession prune ordinary
candidates of bundled or dismissed pairs but keep listed rows, which are
bundled or dismissed by definition. A split rebuilds the leaving keys'
candidates the same way: a listed row stays while its pair is still bundled or
dismissed, and one whose pair the split separated returns as an ordinary
candidate. For the same reason, an automatic bundle
does not close the entry of a listed pair among its members as `superseded`;
that entry closes with its own verdict. A pair whose two keys are both rescanned
is listed again by each fingerprint, and the later listing replaces the
earlier row, so the judgement sees both new fingerprints.

A `same` or `different` verdict replaces the prior entry: only the latest
`same` or `different` entry of a pair offers the undo
([`internal/store/activity.go`](../../internal/store/activity.go)). Otherwise
undoing the prior would split or un-dismiss a pair that the new verdict
confirmed, leaving the new entry's undo `already_reverted`.
