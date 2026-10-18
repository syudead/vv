# Data model: faces and profiles for people

The rest of the model is unchanged: [`00042_work_info.sql`](../../internal/store/migrations/00042_work_info.sql)
and the later migrations in [`internal/store/migrations`](../../internal/store/migrations).
Organizations gain nothing.

## Migration

`00064_person_profiles.sql` adds the profile columns to `people` and the tables `people_links`, `person_images` and `person_favorites`. Its Down
section drops them. Existing people get every column `null`.

## `people` (added columns)

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `gender` | text | yes | `female`, `male` or `other` |
| `birth_date` | text | yes | `YYYY`, `YYYY-MM` or `YYYY-MM-DD` ([R-2](research.md#r-2-partial-dates-reuse-the-work-date-format)) |
| `birth_key` | integer | yes | `YYYYMMDD` of `birth_date` with missing parts as `00`; `null` with it ([R-4](research.md#r-4-age-sort-is-a-birth-date-sort-with-the-unknown-last)) |
| `death_date` | text | yes | Same format as `birth_date` |
| `birthplace` | text | yes | Free text, 1–200 code points |
| `height_cm` | integer | yes | 50–300 |
| `bust_cm`, `waist_cm`, `hips_cm` | integer | yes | 30–300 each, independent of one another |
| `cup` | text | yes | 1–3 Latin capitals, stored upper case |
| `blood_type` | text | yes | `A`, `B`, `O` or `AB` |
| `career_start_year`, `career_end_year` | integer | yes | 1900–9999 |
| `bio` | text | yes | 1–5,000 code points; line breaks kept |
| `image_revision` | integer | no | Default `0`; increases on every change of the face (upload, removal, merge or plugin fill), so a revision is never reused for a different face ([R-6](research.md#r-6-the-image-is-addressed-by-person-and-revision)) |

## `people_links`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `person_id` | integer | no | `references people(id) on delete cascade` |
| `position` | integer | no | 0-based order; primary key with `person_id` |
| `url` | text | no | `http` or `https`, at most 2,048 characters, unique per person |

## `person_images`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `person_id` | integer | no | Primary key, `references people(id) on delete cascade` |
| `image` | blob | no | JPEG, at most 800 px on its long side ([R-5](research.md#r-5-a-face-is-one-jpeg-blob-in-sqlite-fitted-on-the-server-in-pure-go)) |
| `width`, `height` | integer | no | Pixel size of `image` |
| `updated_at` | integer | no | Unix seconds |

## `person_favorites`

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `person_id` | integer | no | Primary key, `references people(id) on delete cascade` |
| `favorited_at` | integer | no | Unix seconds; kept when the favorite is set again |

## Rules

| Rule | Enforced in |
| --- | --- |
| Each text field is trimmed; an empty value is `null` | `domain.NormalizePersonProfile` |
| A value outside its range or format is `400` with `work_info_*` reason and `field` `profile.<name>` | `domain.NormalizePersonProfile` |
| `death_date` is not before `birth_date`, compared at the precision both share | `domain.NormalizePersonProfile` |
| Neither date is after today in UTC | `domain.NormalizePersonProfile` |
| `career_end_year` is not before `career_start_year` | `domain.NormalizePersonProfile` |
| At most 20 links | `domain.NormalizePersonProfile` |
| `birth_key` is written with `birth_date` in the same statement | `internal/store` |
| A merge fills only the target's `null` fields, an empty link list and a missing image ([R-8](research.md#r-8-merge-fills-the-targets-empty-fields-from-the-sources)) | `mergeCreditsInto` |
| A merge target becomes a favorite when any source is one ([R-13](research.md#r-13-person-favorites-join-the-existing-favorites)) | `mergeCreditsInto` |
| A plugin value fills only a `null` field or an empty link list; a plugin image is stored only when the person has none ([R-10](research.md#r-10-plugin-data-only-fills-what-is-empty-after-the-apply-commits)) | `ApplyFetchedWorkInfo`, the post-commit image step |

## What does not change

- `people_names`, `people_external_ids` and `video_work_people`: names,
  aliases, external IDs and roles keep their tables and rules.
- Profile and image changes do not touch smart-folder notices or collection
  values, which depend on names and links to videos only, so they commit
  without the match-change refresh.
- Person edits stay out of the activity log, as they are today.
