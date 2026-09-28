# Contract: ライブ変換の映像エンコード方式の設定

正本は `api/openapi.yaml` で、この文書は足す経路と型、変える引数だけを書く。ライブ変換の本体
（`transcodeVideo`）と `transcode-start` の応答の形は変えない。エラーの形は
[specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md) に従う。

## 1. 型

```yaml
VideoEncoderChoice:        # 所有者が選ぶ値（親 Issue 要件 1）
  type: string
  enum: [software, nvenc, qsv, vaapi, videotoolbox, auto]
VideoEncoder:              # 実際に使う方式
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
    encoder: { $ref: VideoEncoder }        # software は載らない（常に使える）
    state:   { type: string, enum: [checking, available, unavailable] }
    reason:  { $ref: EncoderUnavailableReason }   # state が unavailable のときだけ
TranscodingSettings:
  type: object
  required: [videoEncoder, effectiveEncoder, checking, encoders]
  properties:
    videoEncoder:      { $ref: VideoEncoderChoice }   # 保存値の解釈。未選択と未知の値は software
    effectiveEncoder:  { $ref: VideoEncoder }         # 今のライブ変換の要求が使う方式（要件 6）
    fallbackReason:    { $ref: EncoderFallbackReason } # 選んだ方式が使えず software のときだけ（要件 8）
    checking:          { type: boolean }              # 起動時の確認が終わっていない
    encoders:          # nvenc・qsv・vaapi・videotoolbox の順で常に 4 件
      type: array
      items: { $ref: EncoderAvailability }
UpdateTranscodingSettingsRequest:
  type: object
  required: [videoEncoder]
  additionalProperties: false
  properties:
    videoEncoder: { $ref: VideoEncoderChoice }
```

`auto` で使えるものが無く software になるのは fallback ではないので、`fallbackReason` は載らない。

## 2. `GET /api/settings/transcoding`

`operationId: getTranscodingSettings`。`security` は既定（所有者だけ。`accessRoutes` に足さない）。

- 200: `TranscodingSettings`。`Cache-Control: no-store`。
- 401 `unauthenticated`／403: 境界の扱い（ゲストは方式も一覧も見られない。要件 13）。

## 3. `PUT /api/settings/transcoding`

`operationId: updateTranscodingSettings`。所有者だけ。`Content-Type: application/json` を要求する
（`requiresJSONBody` に足す）。

- 本文: `UpdateTranscodingSettingsRequest`。
- 200: 保存後の `TranscodingSettings`（`GET` と同じ形。画面はこれで表示を置き換える）。
  `Cache-Control: no-store`。保存は次に始まるライブ変換の要求から効く（要件 4）。
- 400 `invalid_request`: `videoEncoder` が列挙に無い、本文が JSON でない。
- 409 `conflict`、reason `encoder_unavailable`: `state` が `available` でないハードウェアの方式を
  選んだ（確認中を含む）。`software` と `auto` は常に受け付ける。保存値は変えない。
- 500 `internal`: 保存に失敗した。画面は選択を元に戻し、区画内に理由を出す（親 Issue UI品質）。

## 4. ライブ変換の要求への効き方

`transcodeVideo` は要求ごとに `TranscodingSettings.effectiveEncoder` に当たる方式を
`domain.LiveTranscodeRequest.VideoEncoder` に載せる。映像をコピーできる要求は方式に依らず
コピーする（要件 12）。ハードウェアで最初のデータを出す前に失敗した要求は同じ要求の中で
`software` でやり直し、応答は成功する（要件 11。
[research.md R-6](../research.md#r-6-要求の中での切り替えはエンコードの段でハードウェア--ソフトウェアの順に試す)）。
応答の形もヘッダーも変えない。
