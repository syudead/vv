# Data model: Person plugins

The rest of the model is unchanged: the plugin tables in
[`00046_work_info_plugins.sql`](../../internal/store/migrations/00046_work_info_plugins.sql),
[`00047`](../../internal/store/migrations/00047_work_info_plugin_generation.sql)
and [`00054`](../../internal/store/migrations/00054_work_info_plugin_runs.sql),
and the people tables described in
[people.md](../../docs/design-docs/people.md). No people column is added: a
lookup writes the existing profile columns, aliases, `people_external_ids` and
`person_images`.

## Migration

One migration, numbered after the last one on `main` when the unit lands
(`00069` is taken on `feature/face-recognition`), alters
`work_info_plugins`.

## `work_info_plugins`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `kind` | text | No, default `'work'` | The manifest's `kind`; `'work'` for an unreadable manifest |
| `sources` | text | Yes | JSON array of the manifest's `sources`; `NULL` for `work` and for an unreadable manifest |
| `auto_lookup` | integer | No, default `1` | `1` when the owner lets work-info fetches call this person plugin ([R-8](research.md#r-8-automatic-lookups-are-a-per-plugin-switch-on-the-row)) |

## In-memory results

Both expire after `WorkInfoFetchLifetime` (10 minutes) and sit in one store
whose images share the 52 MiB budget ([R-5](research.md#r-5-lookup-results-live-with-the-fetch-and-the-image-budget-grows-to-52-mib)).

| Entity | Held in | Fields |
| --- | --- | --- |
| Lookup | The work-info fetch entry | Plugin ID, digest and generation; `(source, id)`; the indexes of the fetched people sharing it and the person ID each matched at fetch time (0 for none); the result (person and image) once run |
| Person fetch | Its own entry in the same store | Person ID, plugin ID, digest and generation, the fetched person and image |

## Rules

| Rule | Enforced in |
| --- | --- |
| `sources` is non-empty exactly when `kind` is `person` | `internal/domain` manifest check, before the row is written |
| A replacement keeps `auto_lookup` and takes `kind` and `sources` from the new manifest | `internal/store` `SaveWorkInfoPlugin` |
| A lookup or person fetch applies only while its plugin row has the digest and generation it ran with | `internal/app`, re-read inside the apply transaction as for a work-info fetch |
| An external ID held by another person is never moved | `internal/store`, reported as `taken` |

## What does not change

Rows uploaded before this feature keep `kind` `work` and `sources` `NULL`; the
migration does not inspect stored modules again. A person module uploaded to an
older VVMDM is therefore listed as a work-info plugin and its runs fail with
`plugin_failed` until the owner uploads it again. Re-inspecting every stored
module at migration was rejected: a SQL migration cannot run `Inspect`, a
startup backfill adds a step for every owner, and person plugins do not exist
before this feature ships (the Issue's `対象外`).

`work_info_plugin_settings` and `work_info_plugin_runs` serve both kinds; a
person plugin's last run is kept like a work-info plugin's. Applied values
stay on the person when the plugin is deleted or revoked (`Edge Cases`),
because nothing records which plugin brought them.
