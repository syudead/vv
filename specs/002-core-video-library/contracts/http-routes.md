# HTTP 経路の契約（コア機能）

**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md)

JSON API の形は [openapi.yaml](./openapi.yaml) が唯一の真実である。本書は OpenAPI に
書けない約束事（経路の分配、Range 配信、キャッシュ、安全性）を補う。
001 の[同名の契約](../../001-initial-setup/contracts/http-routes.md)を引き継ぎ、差分を書く。

## 経路の分配

| パス                                             | 応答                       | 備考                                                |
| ------------------------------------------------ | -------------------------- | --------------------------------------------------- |
| `/api/health`                                    | JSON                       | 001 から変更なし                                    |
| `/api/videos`・`/api/videos/{id}`・`/api/scans*` | JSON                       | [openapi.yaml](./openapi.yaml)                      |
| `/api/videos/{id}/stream`                        | 動画本体（`200` / `206`）  | 下記「Range 配信」                                  |
| `/api/videos/{id}/thumbnail`                     | `image/jpeg`               | 下記「キャッシュ」                                  |
| `/api/*`（未定義）                               | `404` + `Error`            | SPA の `index.html` を返してはならない              |
| `/assets/*`                                      | 埋め込み済みの静的ファイル | 内容ハッシュ付きの名前なので長期キャッシュ可        |
| 上記以外のすべて                                 | `index.html`（`200`）      | クライアント側ルーティング（`/videos/{id}` を含む） |

## Range 配信（`/api/videos/{id}/stream`）

- `http.ServeContent` に開いたファイルと `ModTime` を渡す。`Range` の解釈・`206`・
  `Content-Range`・`Accept-Ranges: bytes`・`416`・`If-Range` は標準実装に任せ、自前で
  組み立てない
- `Content-Type` は拡張子から決める（`.mp4`/`.m4v` → `video/mp4`、`.webm` → `video/webm`、
  それ以外 → `application/octet-stream`）
- `Cache-Control: private, max-age=0, must-revalidate`。内容が同じでも別の利用者に
  共有キャッシュさせない
- 再生できない形式（`playable = false`）でも配信自体は行う。ブラウザが再生できるか
  どうかと、ファイルを取得できるかは別の話である
- 転送中に接続が切れた場合（利用者がシークした、タブを閉じた）は、記録を `info` では
  なく `debug` に落とす。通常運用で常時発生するため

### 安全性（必須）

DB に入っているパスをそのまま開かない。次を満たさない行は `404`（存在を漏らさないため
`403` にはしない）として扱う。

1. `filepath.Clean` 後のpathが、現在登録済みのいずれかのMediaFolder + 区切り文字で始まること
2. symlinkを含まない保存済みlocationであり、1が成り立つこと
3. 通常ファイルであること（ディレクトリ・デバイスファイルを開かない）

## キャッシュ

| 対象                                             | `Cache-Control`                       |
| ------------------------------------------------ | ------------------------------------- |
| `/api/health`                                    | `no-store`                            |
| `/api/videos`・`/api/videos/{id}`・`/api/scans*` | `no-store`                            |
| `/api/videos/{id}/stream`                        | `private, max-age=0, must-revalidate` |
| `/api/videos/{id}/thumbnail`（`v` 付き）         | `public, max-age=31536000, immutable` |
| `index.html`                                     | `no-cache`                            |
| `/assets/*`                                      | `public, max-age=31536000, immutable` |

サムネイルの `v` は内容由来の識別子である。内容が変われば URL が変わるので、
長期キャッシュしても古い画像が残らない。`v` の無い要求には長期キャッシュを付けない。

## エラー表現

すべての JSON エラーは `Error`（`code` と `message`）で返す。`code` は機械可読で、
`message` は利用者にそのまま提示してよい日本語にする。

| 状況                                              | 状態コード | `code`            |
| ------------------------------------------------- | ---------- | ----------------- |
| 動画が存在しない／実体を開けない                  | `404`      | `not_found`       |
| 経路の綴り誤り（`/api/` 配下）                    | `404`      | `not_found`       |
| パラメータの形式が不正（`limit`、`cursor`、本文） | `400`      | `invalid_request` |
| 予期しない失敗                                    | `500`      | `internal`        |

`cursor` が壊れている場合は `400` にする。黙って先頭から返すと、無限スクロールが
巻き戻って同じ内容を延々と表示することになる。

## 進捗の記録（`PUT /api/videos/{id}/progress`）

- 呼び出し間隔はクライアントの責務（再生中 5 秒ごと、一時停止・離脱時）。サーバー側で
  頻度制限はしない
- 視聴済みの判定はサーバー側で行い、応答で返す。クライアントの申告は採らない
- `navigator.sendBeacon` からの送信も受け付けるため、本文の `Content-Type` は
  `application/json` と `text/plain;charset=UTF-8` の双方を許容する

## 認証

掛けない（Phase 3 の範囲）。ストリーム経路も同様であり、外部公開を前提にしないことを
README に明記した状態を維持する。
