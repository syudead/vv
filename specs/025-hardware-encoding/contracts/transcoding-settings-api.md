# Contract: live transcode video encoder setting

The source of truth is `api/openapi.yaml`. This document lists only the added paths and types
and the changed arguments. The live transcode itself (`transcodeVideo`) and the shape of the
`transcode-start` response do not change. Errors follow
[specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md).

## 1. Types

```yaml
VideoEncoderChoice:        # The value the owner selects (parent Issue Requirement 1)
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
    encoder: { $ref: VideoEncoder }        # software is never listed (always available)
    state:   { type: string, enum: [checking, available, unavailable] }
    reason:  { $ref: EncoderUnavailableReason }   # only when state is unavailable
TranscodingSettings:
  type: object
  required: [videoEncoder, effectiveEncoder, checking, encoders]
  properties:
    videoEncoder:      { $ref: VideoEncoderChoice }   # Interpreted saved value. No selection and unknown values read as software
    effectiveEncoder:  { $ref: VideoEncoder }         # Encoder that live transcode requests use now (Requirement 6)
    fallbackReason:    { $ref: EncoderFallbackReason } # Only when the selected encoder is unavailable and software is used (Requirement 8)
    checking:          { type: boolean }              # The startup check has not finished
    encoders:          # Always 4 entries, in the order nvenc, qsv, vaapi, videotoolbox
      type: array
      items: { $ref: EncoderAvailability }
UpdateTranscodingSettingsRequest:
  type: object
  required: [videoEncoder]
  additionalProperties: false
  properties:
    videoEncoder: { $ref: VideoEncoderChoice }
```

When `auto` finds no available encoder and uses software, that is not a fallback, so
`fallbackReason` is absent.

## 2. `GET /api/settings/transcoding`

`operationId: getTranscodingSettings`. `security` is the default (owner only; not added to
`accessRoutes`).

- 200: `TranscodingSettings`. `Cache-Control: no-store`.
- 401 `unauthenticated` / 403: boundary handling (a guest sees neither the encoder nor the list;
  Requirement 13).

## 3. `PUT /api/settings/transcoding`

`operationId: updateTranscodingSettings`. Owner only. Requires `Content-Type: application/json`
(added to `requiresJSONBody`).

- Body: `UpdateTranscodingSettingsRequest`.
- 200: the `TranscodingSettings` after saving (the same shape as `GET`; the screen replaces its
  display with it). `Cache-Control: no-store`. The saved value applies from the next live
  transcode request that starts (Requirement 4).
- 400 `invalid_request`: `videoEncoder` is not in the enum, or the body is not JSON.
- 409 `conflict`, reason `encoder_unavailable`: the request selects a hardware encoder whose
  `state` is not `available` (including while checking). `software` and `auto` are always
  accepted. The saved value does not change.
- 500 `internal`: saving failed. The screen reverts the selection and shows the reason inside the
  section (parent Issue UI quality).

## 4. Effect on live transcode requests

For each request, `transcodeVideo` puts the encoder that matches
`TranscodingSettings.effectiveEncoder` into `domain.LiveTranscodeRequest.VideoEncoder`.

- A request whose video can be copied is copied, whatever the encoder (Requirement 12).
- A request that fails on hardware before its first data restarts with `software` inside the same
  request, and the response succeeds (Requirement 11;
  [research.md R-6](../research.md#r-6-switching-inside-a-request-tries-hardware-then-software-at-the-encode-step)).
- The response shape and headers do not change.
