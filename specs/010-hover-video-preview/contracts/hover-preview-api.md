# Contract: Hover Preview API

Machine-readable source of truth は `api/openapi.yaml` とし、この文書は意図と受け入れ境界を固定する。

## Video Fields

- `previewState` は required で、`pending | done | failed` のいずれか。
- `previewUrl` は optional。current content key の `previewState=done` かつ asset が配信可能なときだけ返す。
- `previewUrl` は cache version として current content key を query に含む。
- `playable` は原本の本再生能力を表す既存 field であり、preview eligibility には使わない。

## `GET /api/videos/{id}/preview`

保存済み preview MP4 だけを配信する。

| Condition | Response |
| --- | --- |
| Range なし、asset あり | `200 OK` |
| valid Range、asset あり | `206 Partial Content` |
| video 不在 | `404 Not Found` |
| state が pending/failed | `404 Not Found` |
| state done だが file 不在・非 regular・zero-size | `404 Not Found` |
| unsatisfiable Range | `416 Range Not Satisfiable` |

成功応答は `Content-Type: video/mp4`、`Accept-Ranges: bytes` を返し、partial response は正しい `Content-Range` と length を返す。`v` が current content key と一致する URL は immutable cache、version なし・不一致・error response は `no-store` とする。

handler は resolved file を `ServeContent` 相当で配信するだけで、元動画を開く、ffmpeg を起動する、`/stream` や `/transcode.mp4` へ redirect/fallback する動作を持たない。内部破損は scan/startup reconciliation の size/SHA-256 照合が `done` から `pending` へ戻す。検証までの間に browser media error が起きた場合、UI は thumbnail へ戻り fallback 配信をしない。
