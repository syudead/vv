# Contract: Live transcoding video encoder setting

Source of truth: `api/openapi.yaml`. This document covers only the routes and
types added and the request field that changes. The live transcode itself
(`transcodeVideo`) and the `transcode-start` response shape do not change. Errors
follow [specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md).

## Types

```yaml
VideoEncoderChoice:        # The value the owner picks (parent Issue requirement 1)
  type: string
  enum: [software, nvenc, qsv, vaapi, videotoolbox, auto]
VideoEncoder:              # The encoder actually used
  type: string
  enum: [software, nvenc, qsv, vaapi, videotoolbox]
EncoderUnavailableReason:
  type: string
  enum: [unsupported_os, encoder_missing, check_failed, timed_out]
EncoderFallbackReason:
  type: string
  enum: [selected_unavailable, checking]
EncoderAvailability:
  type: object
  required: [encoder, state]
  properties:
    encoder: { $ref: VideoEncoder }        # software is not listed (always available)
    state:   { type: string, enum: [checking, available, unavailable] }
    reason:  { $ref: EncoderUnavailableReason }   # only when state is unavailable
TranscodingSettings:
  type: object
  required: [videoEncoder, effectiveEncoder, checking, encoders]
  properties:
    videoEncoder:      { $ref: VideoEncoderChoice }   # the interpreted stored value; not chosen and unknown values are software
    effectiveEncoder:  { $ref: VideoEncoder }         # the encoder current live transcode requests use (requirement 6)
    fallbackReason:    { $ref: EncoderFallbackReason } # only when the chosen encoder is unavailable and software is used (requirement 8)
    checking:          { type: boolean }              # the startup check has not finished
    encoders:          # always 4 entries, in the order nvenc, qsv, vaapi, videotoolbox
      type: array
      items: { $ref: EncoderAvailability }
UpdateTranscodingSettingsRequest:
  type: object
  required: [videoEncoder]
  additionalProperties: false
  properties:
    videoEncoder: { $ref: VideoEncoderChoice }
```

When `auto` finds nothing available and uses software, that is not a fallback, so
`fallbackReason` is absent.

## `GET /api/settings/transcoding`

`operationId: getTranscodingSettings`. `security` is the default (owner only; not
added to `accessRoutes`).

| Status | `code` | When |
| --- | --- | --- |
| 200 | | `TranscodingSettings`, with `Cache-Control: no-store` |
| 401 | `unauthenticated` | Boundary handling; a guest sees neither the encoder nor the list (requirement 13) |
| 403 | | Same |

## `PUT /api/settings/transcoding`

`operationId: updateTranscodingSettings`. Owner only. Requires
`Content-Type: application/json` (added to `requiresJSONBody`).

**Request**: `UpdateTranscodingSettingsRequest`.

**Response**: 200 with the saved `TranscodingSettings` (the same shape as `GET`;
the screen replaces its display with it) and `Cache-Control: no-store`. The saved
value applies from the next live transcode request that starts (requirement 4).

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_request` | `videoEncoder` is not in the enum, or the body is not JSON |
| 409 | `conflict`, reason `encoder_unavailable` | A hardware encoder whose `state` is not `available` was chosen (including while checking). `software` and `auto` are always accepted. The stored value does not change |
| 500 | `internal` | Saving failed. The screen reverts the selection and shows the reason inside the section (parent Issue `UI品質`) |

## Effect on live transcode requests

`transcodeVideo` puts the encoder that corresponds to
`TranscodingSettings.effectiveEncoder` into
`domain.LiveTranscodeRequest.VideoEncoder` on every request. A request whose video
can be copied is copied whatever the encoder (requirement 12). A request where
hardware fails before producing initial data retries with `software` within the
same request, and the response succeeds (requirement 11;
[research.md R-6](../research.md#r-6-in-request-fallback-tries-hardware-then-software-at-the-encode-step)).
Neither the response shape nor its headers change.
