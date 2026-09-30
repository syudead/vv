# Contract: 外部連携 API v1 と MCP の仮の付与

正本は `api/external-v1.yaml` で、この文書は足す項目の差分だけを書く。共通の規則（Bearer、誤りの形、
`VideoRef`、`index`、件数の上限）は
[specs/026-external-api/contracts/external-api.md](../../026-external-api/contracts/external-api.md)
のまま。決定は [research.md R-7](../research.md#r-7-外部連携-api-は-tentative-を要求の-1-項目飛ばした名前を応答の-1-項目として足す)。
互換の方針どおり、項目の追加だけを行う。新しい操作は足さない。

## 0. スキーマの差分

| スキーマ | 足す項目 | 規則 |
| --- | --- | --- |
| `Tag`（`GET /api/v1/tags`） | `tentative: boolean`（`required`） | 画面の `Tag` と同じ |
| `ExternalVideoTag` | `tentative: boolean`（`required`） | 動画に付いたタグの状態 |
| `VideoTagsRequest` | `tentative: boolean`（任意、省略時は `false`） | §1 |
| `VideoTagsResponse` | `skippedTags: string[]`（`required`） | §1。飛ばした名前が無ければ空の配列 |

## 1. `POST /api/v1/video-tags` の差分

```json
{ "videos": [{ "path": "/media/a.mp4" }],
  "action": "add",
  "tags": ["猫", "高画質"],
  "tentative": true }
```

- `tentative` が `false` か省略: 今と同じ。無い名前は確定したタグとして作り、その名前が却下した名前なら
  一覧から消える（要件 3・15）。
- `tentative` が `true` で `add`・`replace`:
  - 既存のタグの名前かシノニムに当たる名前は、今と同じくそのタグを付ける。そのタグが仮でも確定でも状態は
    変えない（要件 2）。
  - どのタグにも当たらない名前は、却下した名前の一覧に無ければ**仮のタグ**として作って付ける。
    1 つの要求で同じ名前が複数の動画に付くときも作るのは 1 つ（Edge Case）。
  - 却下した名前（綴りの完全一致。要件 13）は飛ばす。タグを作らず、付けず、`skippedTags` に整えた名前を
    `tags` の順で 1 回ずつ返す。残りの名前は処理し、要求は `200` で成功する（要件 11・12）。
  - `replace` では、飛ばした名前は最初から送られなかったものとして扱い、置き換え後の集合に入らない。
    `tags` がすべて却下した名前なら空の集合で置き換えたのと同じになる（Edge Case）。
- `tentative` が `true` で `remove`: 今と同じ（どのタグにも当たらない名前は何もしない）。`skippedTags` は
  空の配列。
- 応答:

```json
{ "items": [{ "video": { "id": 1, "contentKey": "…" },
              "tags": [{ "id": 3, "name": "猫", "manual": true, "fromFolder": false, "tentative": false }] }],
  "skippedTags": ["高画質"] }
```

- 誤りの `code`・`reason` は変えない。`tentative` が真偽値でなければ `400 invalid_request`（本文の形の
  誤りとして、今の `readJSONBody` の扱い）。

## 2. MCP

`update_video_tags` の入力は `VideoTagsRequest` の型から導いているので、`tentative` はそのまま入る。
ツールの説明に「`tentative: true` で、新しく作るタグを仮のタグにし、却下した名前を飛ばして
`skippedTags` に返す」を足す。`list_tags`・`get_video`・`list_videos` の出力は §0 の差分をそのまま含む。
ツールの数と注釈は変えない（[specs/026-external-api/contracts/mcp.md](../../026-external-api/contracts/mcp.md)）。

## 3. `docs/how-to/external-api.md`

「動画にタグを付ける」に `tentative` と `skippedTags` の説明と、スクレイパーの例（LLM が出した名前は
`tentative: true` で付け、利用者が管理画面で片付ける）を足す。仮のタグの確定・却下・却下した名前の
一覧は画面の操作で、この API にはない。
