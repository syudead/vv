# Data model: live transcode video encoder

This document covers only what is stored, and its rules, from parent Issue #370 Requirements 2, 3
and 8 and the edge case "the saved value is unknown". Existing tables do not change. The table
category follows "Rebuildable and user data" in [ARCHITECTURE.md](../../ARCHITECTURE.md): this
table is user and settings data, like `media_folders` (a scan does not restore it).

## 1. Migration

Add `internal/store/migrations/000NN_settings.sql` (the next number at implementation time).

```sql
-- +goose Up
-- Values the owner selects on the settings screen. Settings data; a scan does not restore it
-- (specs/025-hardware-encoding/data-model.md).
create table settings (
    key        text primary key,
    value      text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists settings;
```

The migration inserts no rows. A key without a row means "never selected".

## 2. Key `transcode.video_encoder`

| Column | Content |
| --- | --- |
| `key` | `transcode.video_encoder` |
| `value` | A `domain.EncoderChoice` string: `software`, `nvenc`, `qsv`, `vaapi`, `videotoolbox`, `auto` |
| `updated_at` | Time of saving (Unix seconds) |

- `SettingsStore.TranscodeEncoderChoice(ctx) (value string, found bool, err error)` returns the
  string as stored. `SettingsStore.SaveTranscodeEncoderChoice(ctx, value)` saves it with one
  upsert statement. When several tabs save at the same time, the last write wins (parent Issue
  edge case).
- The pure function `ParseEncoderChoice(value string, found bool) EncoderChoice` in
  `internal/domain` interprets the value. No row means `software` (Requirement 3). An unknown
  string is also treated as `software`, and the saved value does not change (edge case "the saved
  value is unknown"; the screen shows `software` as selected, and the owner can select again).
- When the startup check finds the selected encoder unavailable, the saved value also does not
  change (Requirement 8). The encoder actually used is not stored
  ([research.md R-3](research.md#r-3-a-pure-domain-function-decides-the-encoder-actually-used-and-app-holds-it-in-memory)).

## 3. In-memory values (not stored)

`TranscodeSettings` in `internal/app` holds these values and rebuilds them on every start.

| Value | Content |
| --- | --- |
| `domain.EncoderAvailability` | Per hardware encoder: `State` (`checking`, `available`, `unavailable`) and `Reason` (`unsupported_os`, `encoder_missing`, `check_failed`, `timed_out`; only when `unavailable`) |
| `domain.TranscodeEncoding` | `Choice` (interpreted saved value), `Effective` (the `VideoEncoder` actually used), `FallbackReason` (`selected_unavailable`, `checking`; only on fallback), `Checking` (whether the check is unfinished), `Encoders` (the list above) |

`domain.ResolveVideoEncoder(choice, availability)` decides `Effective` and `FallbackReason`
(R-3). `domain.HardwareEncoderCandidates(goos)` returns the pairs of checked encoders and OS; an
encoder outside them appears in the list as `unavailable` / `unsupported_os`.
