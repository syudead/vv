# Data model: Judging possible duplicates automatically

The rest of the model is unchanged: bundles, candidates, dismissals and
fingerprints are [030 data-model](../030-video-versions/data-model.md), and the
schema is the migrations in [`internal/store/migrations`](../../internal/store/migrations).

## Migration

One migration, numbered after the last one on `main` when it is written (other
feature branches also add migrations), adds the judgement columns to
`video_version_candidates` and creates `video_version_unbundles`. Down drops
the table and the columns.

| Table | Category ([Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery)) |
| --- | --- |
| `video_version_candidates` | Rebuildable index, as today; the judgements in it are asked again after a rebuild |
| `video_version_unbundles` | User data: the owner's action, kept like `video_version_dismissals` |

## `video_version_candidates`

Columns added for the judgement
([R-4](research.md#r-4-the-judgement-is-columns-on-the-candidate-row)); every
row is rebuilt with them null or 0.

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `judgement` | text | Yes | Null while waiting, else `running`, `undecided` or `failed` |
| `attempts` | integer | No, default 0 | Failures so far; kept through a claim and a restart |
| `retry_at` | integer | Yes | When a `failed` row becomes claimable ([R-5](research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends)); null otherwise |
| `judgement_error` | text | Yes | The provider's message for a `failed` row |
| `probability` | real | Yes | The model's answer for an `undecided` row; null when it is undecided because of a conflict |

## `video_version_unbundles`

Pairs removed from a bundle, never judged by the model
([R-8](research.md#r-8-removing-a-version-records-the-pairs-and-rebuilds-its-candidates)).

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `key_a`, `key_b` | text | No | Content keys, `key_a < key_b`; primary key |
| `created_at` | integer | No | When the version was removed |

**Relationships**: none; content keys, as in `video_version_dismissals`.

## Rules

| Rule | Enforced in |
| --- | --- |
| A candidate is claimable when the feature is on, its pair is not in `video_version_unbundles`, and its `judgement` is null, or `failed` with `retry_at` passed | `domain` (the rule), `store` (the claim query) |
| The claim returns both keys' fingerprints, which the frame rule reads ([R-1](research.md#r-1-the-fingerprints-decide-first-the-model-is-asked-only-when-a-section-differs)) | `store` |
| One judgement is `running` at a time | The single worker; the claim is one statement |
| A `running` row left at startup returns to null, keeping `attempts` | `store`, at startup as `auto_tag_queue` recovers |
| "Same video" bundles both sides unless a pair across them is dismissed or unbundled; otherwise the row becomes `undecided` ([R-6](research.md#r-6-a-same-pair-joins-two-sides-unless-a-pair-across-them-is-known-to-differ)) | `domain` (the check over two member sets), `store` (re-reads both inside the bundling transaction) |
| "Same video" picks the representative by [R-7](research.md#r-7-the-representative-is-the-bundles-else-the-larger-picture), then bundles with `VersionStore.Bundle`'s steps | `domain` (the choice), `store` |
| "Different videos" writes `video_version_dismissals` and deletes the candidate, as the owner's "Different videos" does | `store` |
| A result for a candidate that is gone or no longer `running` is discarded | `store` |
| `Unbundle` writes the removed key paired with every other member, then rebuilds the removed key's candidates | `store` (`VersionStore.Unbundle`) |
| A content change at the same path re-keys `video_version_unbundles` as `moveDismissals` re-keys dismissals | `store` (successions) |
| Every result and `Unbundle` publish `VersionCandidatesChanged` after commit ([R-10](research.md#r-10-the-duplicates-page-refreshes-on-a-candidates-notification)) | `store` |

## Settings

| Key | Value | Missing means |
| --- | --- | --- |
| `version_judging.enabled` | `true` or `false` | Off |

## What does not change

`video_version_dismissals`, `video_bundles` and `video_bundle_members` keep
their columns: an automatic bundle and an automatic "different videos" are the
same rows the owner's actions write, which is what the `UI品質` line "no
distinction between automatic and manual" asks for.
