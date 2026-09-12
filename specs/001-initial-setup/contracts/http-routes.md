# HTTP 経路の契約（Phase 0）

**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md)

JSON API の形は [openapi.yaml](./openapi.yaml) が唯一の真実である。
本書は OpenAPI に書けない「API 以外の経路」の約束事を補う。

## 経路の分配

| パス | 応答 | 備考 |
| --- | --- | --- |
| `/api/health` | JSON | [openapi.yaml](./openapi.yaml) 参照 |
| `/api/*`（未定義） | `404` + `Error` | SPA の `index.html` を返してはならない |
| `/assets/*` | 埋め込み済みの静的ファイル | 内容ハッシュ付きの名前なので長期キャッシュ可 |
| 上記以外のすべて | `index.html`（`200`） | クライアント側ルーティングのためのフォールバック |

**なぜ `/api/*` の未定義経路を特別扱いするか**: フォールバックを無条件にすると、
綴りを誤った API 呼び出しに HTML が `200` で返り、クライアント側では
「JSON 解析の失敗」としてしか観測できなくなる。原因の切り分けが遅れるため、
`/api/` 配下だけは必ず JSON のエラーを返す。

## 応答ヘッダ

| 対象 | ヘッダ | 値 |
| --- | --- | --- |
| すべての JSON 応答 | `Content-Type` | `application/json; charset=utf-8` |
| `/api/health` | `Cache-Control` | `no-store` |
| `index.html` | `Cache-Control` | `no-cache`（更新したビルドが即座に反映されるように） |
| `/assets/*` | `Cache-Control` | `public, max-age=31536000, immutable` |

## 認証

Phase 0 では認証を掛けない（Phase 3 の範囲）。外部公開を前提にしないことを
README に明記する。

## 停止時の振る舞い

停止指示（`SIGINT` / `SIGTERM`）を受けたら新規の接続受付を止め、処理中の要求を
猶予時間（既定 10 秒）まで待ってから終了する。猶予を超えた接続は打ち切る。
終了コードは正常終了で `0`。
