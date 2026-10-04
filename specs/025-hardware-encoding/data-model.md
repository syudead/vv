# Data model: Live transcoding video encoder

The rest of the model is unchanged: no existing table changes. This document
covers only what is stored, and its rules, for requirements 2, 3 and 8 of parent
Issue #370 and the Edge Case "the stored value is unknown". Following
"Rebuildable and user data" in [ARCHITECTURE.md](../../ARCHITECTURE.md), this
table is user and settings data like `media_folders`, and a scan does not restore
it.

## 1. Migration

Add `internal/store/migrations/000NN_settings.sql` (the next number at
implementation time).

```sql
-- +goose Up
-- Values the owner picks on the Settings page. Settings data; a scan does not
-- restore it (specs/025-hardware-encoding/data-model.md).
create table settings (
    key        text primary key,
    value      text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists settings;
```

The migration writes no rows. A key with no row means "never chosen".

## 2. Key `transcode.video_encoder`

| Field | Content |
| --- | --- |
| `key` | `transcode.video_encoder` |
| `value` | A `domain.EncoderChoice` string: `software`, `nvenc`, `qsv`, `vaapi`, `videotoolbox`, `auto` |
| `updated_at` | The time it was saved (Unix seconds) |

- `SettingsStore.TranscodeEncoderChoice(ctx) (value string, found bool, err error)`
  returns the string as it is, and
  `SettingsStore.SaveTranscodeEncoderChoice(ctx, value)` saves it with a single
  upsert statement. When several tabs save at once, the row written last remains
  (parent Issue Edge Case).
- The pure function `ParseEncoderChoice(value string, found bool) EncoderChoice`
  in `internal/domain` interprets it. No row means `software` (requirement 3). An
  unknown string is also treated as `software`, and the stored value is not changed
  (Edge Case "the stored value is unknown"; the screen shows `software` selected
  and the owner can choose again).
- When the chosen encoder is unavailable in the startup check, the stored value
  does not change either (requirement 8). The effective encoder is not stored
  ([research.md R-3](research.md#r-3-a-pure-domain-function-decides-the-effective-encoder-and-app-holds-it-in-memory)).

## 3. In-memory values (not stored)

`TranscodeSettings` in `internal/app` holds these and rebuilds them on every
startup.

| Value | Content |
| --- | --- |
| `domain.EncoderAvailability` | Per hardware encoder: `State` (`checking`, `available`, `unavailable`) and `Reason` (`unsupported_os`, `encoder_missing`, `check_failed`, `timed_out`; only when `unavailable`) |
| `domain.TranscodeEncoding` | `Choice` (the interpreted stored value), `Effective` (the `VideoEncoder` actually used), `FallbackReason` (`selected_unavailable`, `checking`; only on fallback), `Checking` (whether the check has not finished), `Encoders` (the list above) |

`domain.ResolveVideoEncoder(choice, availability)` decides `Effective` and
`FallbackReason` (R-3). `domain.HardwareEncoderCandidates(goos)` returns the check
targets per OS; encoders outside them appear in the list as `unavailable` /
`unsupported_os`.
