# Contract: Plugin contract additions

Source of truth: the authors' guide
[docs/how-to/work-info-plugins.md](../../../docs/how-to/work-info-plugins.md),
which the implementation updates. The contract version stays `1`. A manifest
without the new fields behaves as before, but the fetch by ID is not
backward compatible: a plugin built before it reads `url` from the `fetch`
request and returns `url` alone on its candidates, so it must be rebuilt to
read `id` and to return `id` on each candidate. This file names only what
this feature changes.

## Manifest

Each entry of `settings` may carry two more fields
([R-3](../research.md#r-3-a-setting-declares-default-and-options-the-stored-value-or-the-default-is-what-the-run-receives)).

| Field | Type | Required | Rule |
| --- | --- | --- | --- |
| `default` | string | No | 1 to 2000 characters; with `options`, one of them; not allowed with `secret` |
| `options` | string array | No | 2 to 50 distinct values of 1 to 200 characters; not allowed with `secret` |

A manifest that breaks a rule is `manifest_invalid` at upload and at
`mdm plugin pack`.

## Request

`settings` carries the effective value of every declared key that has one:
the owner's stored value when it is set and, for a setting with `options`,
one of them; otherwise the `default`. A `required` key with a default never
stops a run.

A `fetch` request carries `id` and no `url`
([R-10](../research.md#r-10-a-fetch-takes-a-work-id-only-and-vvmdm-never-fetches-a-url-the-owner-typed)):
the candidate's `id` or the text the owner typed, trimmed, which the plugin
maps to the page URL. A plugin that read `url` must be rebuilt to read `id`;
VVMDM sends no `url`.

## Candidates

A `search` candidate carries `id` beside `url`, `title` and `detail`. `id` is
required, 1 to 200 characters without surrounding spaces, and VVMDM sends it
unchanged as the `id` of the fetch when the owner chooses the candidate. A
candidate without a valid `id` fails the search with `plugin_failed`, so a
plugin built before this change must be rebuilt to return it. `url` stays
required and is still the `http` or `https` page of the work: the dialog
shows its host and links to it with `Open page`, and VVMDM never fetches it
or sends it to the plugin's `fetch`.

## Standard error

What the plugin writes to standard error is shown to the owner on the settings
screen for the plugin's last run, with the first 64 KiB kept and each secret
setting value replaced by `[secret]`
([R-6](../research.md#r-6-the-runtime-returns-each-runs-standard-error-and-the-app-keeps-the-last-one-per-plugin-with-the-secret-values-masked)).
A secret the plugin encodes or splits before writing is not replaced.

## Rate limiting

The guide gains a section on `429` responses with the behaviour of
[R-9](../research.md#r-9-on-a-429-a-plugin-waits-for-the-smaller-of-retry-after-and-5-seconds-retries-once-and-otherwise-reports-an-error):
one retry after at most 5 s while the run's waits total at most 10 s, then
an `error` response.
