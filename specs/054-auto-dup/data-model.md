# Data model: Judging possible duplicates automatically

The rest of the model is unchanged: bundles, candidates, dismissals and
fingerprints are [030 data-model](../030-video-versions/data-model.md), and the
schema is the migrations in [`internal/store/migrations`](../../internal/store/migrations).

## Migration

One migration, numbered after the last one on `main` when it is written (other
feature branches also add migrations), creates `video_version_judgements` and
`video_version_unbundles`. Down drops both.

| Table | Category ([Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery)) |
| --- | --- |
| `video_version_judgements` | Index data: asking the model again rebuilds it |
| `video_version_unbundles` | User data: the owner's action, kept like `video_version_dismissals` |

## `video_version_judgements`

One row per candidate the worker has claimed; a candidate with no row is
waiting ([R-4](research.md#r-4-judgements-live-in-their-own-table-beside-the-candidates)).

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `key_a`, `key_b` | text | No | The candidate's pair, `key_a < key_b`; primary key |
| `state` | text | No | `running`, `undecided` or `failed` |
| `attempts` | integer | No | Failures so far; 0 unless `failed` |
| `retry_at` | integer | Yes | When a `failed` row becomes claimable ([R-5](research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends)); null otherwise |
| `error` | text | Yes | The provider's message for a `failed` row |
| `probability` | real | Yes | The model's answer for an `undecided` row; null when the row is undecided because of a conflict |
| `updated_at` | integer | No | Last state change |

**Relationships**: `(key_a, key_b)` references `video_version_candidates` with
`on delete cascade`, so bundling, dismissing, rebuilding a key's candidates and
releasing a content remove the judgement with its candidate. The store opens
SQLite with foreign keys on ([`sqlite.go`](../../internal/store/sqlite.go)).

## `video_version_unbundles`

Pairs removed from a bundle, never judged by the model
([R-8](research.md#r-8-removing-a-version-records-the-pairs-and-lists-them-again)).

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `key_a`, `key_b` | text | No | Content keys, `key_a < key_b`; primary key |
| `created_at` | integer | No | When the version was removed |

**Relationships**: none; content keys, as in `video_version_dismissals`.

## Rules

| Rule | Enforced in |
| --- | --- |
| A candidate is claimable when the feature is on, its pair is not in `video_version_unbundles`, and it has no judgement row or a `failed` row whose `retry_at` has passed | `domain` (the rule), `store` (the claim query) |
| One judgement is `running` at a time | The single worker; the claim is one statement |
| A `running` row left at startup is deleted, so its candidate waits again | `store`, at startup as `auto_tag_queue` recovers |
| "Same video" bundles only when no two members of the resulting bundle are a dismissed or unbundled pair; otherwise the row becomes `undecided` ([R-6](research.md#r-6-bundling-a-same-pair-is-refused-when-it-would-join-a-different-pair)) | `domain` (the check over a member set), `store` (re-reads both inside the bundling transaction) |
| "Same video" picks the representative by [R-7](research.md#r-7-the-representative-is-the-bundles-else-the-larger-picture), then bundles with `VersionStore.Bundle`'s steps | `domain` (the choice), `store` |
| "Different videos" writes `video_version_dismissals` and deletes the candidate, as the owner's "Different videos" does | `store` |
| A result for a candidate that no longer exists is discarded | `store` |
| `Unbundle` writes the removed key paired with every other member, then rebuilds the removed key's candidates | `store` (`VersionStore.Unbundle`) |
| A content change at the same path re-keys `video_version_unbundles` as `moveDismissals` re-keys dismissals | `store` (successions) |

## Settings

| Key | Value | Missing means |
| --- | --- | --- |
| `version_judging.enabled` | `true` or `false` | Off |

## What does not change

`video_version_candidates`, `video_version_dismissals`, `video_bundles` and
`video_bundle_members` keep their columns: an automatic bundle and an automatic
"different videos" are the same rows the owner's actions write, which is what
the `UI品質` line "no distinction between automatic and manual" asks for.
