# Contract: 認証の HTTP 境界

親 Issue: #135。Plan: [plan.md](../plan.md)。

API の正本は [api/openapi.yaml](../../../api/openapi.yaml) で、この文書はこの feature が
足す経路・応答・Cookie と、既存の全経路に掛かる認証の境界だけを書く。実装では
`openapi.yaml` に足して `task generate` で生成する。

## 1. 認証の境界

既定で認証必須とし、認証なしで通すものを次に限る（要件 9）。

| 要求 | 理由 |
| --- | --- |
| `GET /api/health` | 稼働確認。応答は変えない（§5） |
| `GET /api/auth/session` | ログイン画面が状態を知るため（§3） |
| `POST /api/auth/login` | ログイン（§2） |
| `POST /api/auth/logout` | 失効済みの Cookie でも消せるように（§3） |
| `/api/` で始まらない `GET`・`HEAD` | SPA のビルド成果物（`index.html` と `/assets/*`）。利用者データを含まず、ログイン画面の静的資産を兼ねる |

それ以外の `/api/*` は、定義の無い経路も含めて、有効なセッション（[data-model.md §3](../data-model.md#3-セッションが有効である条件)）が
無ければ §4 の未認証応答を返す。動画ストリーム・サムネイル・シークプレビュー・
ホバープレビュー・ライブ変換・`/api/events`・スキャン・メディアフォルダ・再生位置・
タグもここに入る。

境界は `path.Clean` した `r.URL.Path`（復号済み）で判定する。`//api/…`・`/./api/…`・
`/%61pi/…`・`/api/../api/…` のような書き方でも、`/api/` 以下なら認証を求める。

`openapi.yaml` では、全体に `security: [{sessionCookie: []}]` を置き、上の表の API だけに
`security: []` を置く。`sessionCookie` は `in: cookie` の `apiKey` として宣言する。
Go のテストで、`security: []` の操作の集合と境界の許可リストが一致することを確かめる。

## 2. `POST /api/auth/login`

要求（`Content-Type: application/json`、本文は 8 KiB まで）:

```json
{ "username": "string", "password": "string", "next": "/videos/12?t=30" }
```

`next` は省略できる。

| 状況 | 応答 |
| --- | --- |
| 成功 | `200` `{ "redirectTo": "<安全な戻り先>" }` と `Set-Cookie`（§6） |
| ユーザー名かパスワードが違う、空、上限超え、アカウントが未設定 | `401` `{ "code": "invalid_credentials", "message": "ユーザー名またはパスワードが違います" }` |
| 同じ送信元の「照合中 + 直近 5 分の失敗」が 5 以上、または照合の空きを 5 秒待っても得られない | `429` `{ "code": "login_throttled", … }` と `Retry-After`（秒） |
| 本文が JSON でない、8 KiB を超える、同一オリジンでない | 既存どおり `400`・`403` |

- 401 の4つの原因は、状態コード・本文・ヘッダーが同じである。応答時間を揃えるため、
  どの原因でも Argon2id の照合を1回行う（未設定なら固定のダミーのハッシュと照合する）。
  ユーザー名の比較は定数時間で行う（要件 11）。
- 照合の前に、その送信元の1回分を予約する。予約中の数も制限に数えるので、同時に
  送られた要求でも照合は 5 分に 5 回を超えない（[plan.md Structural Decisions 6](../plan.md#structural-decisions)）。
- 429 のときは照合しない。429 の要求は失敗に数えない。成功すると、その送信元の
  失敗の記録を消す。
- 送信元は、信頼するプロキシを経た場合はその転送ヘッダーから求めたクライアントの
  IP アドレスである（[plan.md Structural Decisions 7](../plan.md#structural-decisions)）。IPv6 は /64 で1つの
  送信元とみなす。
- `redirectTo` は `next` を `domain` の規則で確かめた値である。規則は次のとおりで、
  外れたもの・省略は `/` にする。画面はこの値へ遷移するだけで、自分では判定しない。
  - 制御文字・空白・`\` をどこにも含まない（ブラウザは URL の解釈でタブと改行を
    取り除くので、`/\t/evil.example` が `//evil.example` になる）。
  - 固定の基底に対して URL として解釈した結果が、スキームもホストも持たず、`/` で
    始まるパスになる。
  - パスが `/login` でも `/api/` 以下でもない。
  - 返すのは解釈した結果のパスと問い合わせ文字列を組み立て直した値で、入力そのもの
    ではない。

## 3. `GET /api/auth/session`・`POST /api/auth/logout`

`GET /api/auth/session` は `200` `{ "state": "authenticated" | "unauthenticated" | "setupRequired" }`
を返す。`setupRequired` はユーザー名かパスワードが未設定の状態で、Cookie の有無に
よらずこれを返す（受け入れ条件 1）。ユーザー名は返さない。問い合わせ文字列に `next` を
付けて呼ぶと、`authenticated` のときだけ §2 と同じ規則で確かめた `redirectTo` も返す。
認証済みで `/login` を開いた画面は、この値へ遷移する。

`POST /api/auth/logout` は `204` を返し、Cookie を消す（`Max-Age=0`）。Cookie が
有効なセッションを指していれば、そのセッションを削除し、同じセッションで処理中の
応答を打ち切る（[plan.md Structural Decisions 5](../plan.md#structural-decisions)）。同一オリジンの確認は他の
状態変更と同じく掛かる。

## 4. 未認証とその他の応答

| 状況 | 応答 |
| --- | --- |
| Cookie が無い・形式が違う・該当するセッションが無い・期限切れ・資格情報の再設定後 | `401` `{ "code": "unauthenticated", "message": "ログインが必要です" }` |
| セッションの確認で DB が失敗した | `500` `{ "code": "internal", … }`（認証済みとして扱わない） |

- 401 の原因は区別しない（Edge Case「不正、期限切れ、改ざん済みの Cookie」）。
  `WWW-Authenticate` は付けない（ブラウザの Basic 認証の窓を出さない）。
- どちらも `Cache-Control: no-store` で、HTML を返さない（要件 10）。
- DB の失敗を 401 にしないのは、画面がログイン画面へ送り、そこでも失敗する往復を
  作らないためである。

## 5. `GET /api/health`

応答の形は変えない（`status`・`version`・`commit`・`builtAt`）。ライブラリの内容、設定、
ユーザー名、認証の状態、アカウントが設定済みかどうかは載せない（受け入れ条件 13）。
`version` と `commit` は稼働中のバイナリの特定に使っているので残す。

## 6. セッション Cookie

| 接続 | 名前 | 属性 |
| --- | --- | --- |
| HTTPS | `__Host-vv_session` | `HttpOnly; Secure; SameSite=Strict; Path=/; Max-Age=<期限までの秒>` |
| HTTP | `vv_session` | `HttpOnly; SameSite=Strict; Path=/; Max-Age=<期限までの秒>` |

- 値はセッション ID（32 バイトの暗号学的乱数の base64url）である（要件 5）。
- `Max-Age` を付けるので、ブラウザを閉じても期限まで残る（受け入れ条件 6）。
- HTTPS の要求では `__Host-vv_session` だけを、HTTP の要求では `vv_session` だけを読む。
  名前を分けるので、HTTP の応答が HTTPS 用の Cookie を上書きしたり、HTTPS 用の
  Cookie が HTTP で送られたりしない（Edge Case「HTTP と HTTPS」）。
- 接続が HTTPS かどうかは、TLS で受けたか、信頼するプロキシの `X-Forwarded-Proto` で
  決める（[plan.md Structural Decisions 7](../plan.md#structural-decisions)）。

## 7. 同一オリジンの確認

既存の `acceptsSameOrigin`（`internal/httpapi/media_folders.go`）を、ログインと
ログアウトを含むすべての `POST`・`PUT`・`PATCH`・`DELETE` に掛ける規則のまま使う
（要件 12）。変えるのは、期待するスキームを §6 と同じ判定から取ることだけである。
今は `r.TLS` だけを見るので、TLS を終端するプロキシの後ろでは同一オリジンの要求も
拒否している。

## 8. 記録

ログイン成功・失敗・試行制限・ログアウトを、それぞれ1行の `slog` で `info` に出す。
属性は出来事の種類と送信元だけで、送られたユーザー名・パスワード・セッション ID・
Cookie の値は出さない（要件 11）。送られたユーザー名を出さないのは、パスワードを
ユーザー名の欄に誤って入れた場合に平文が記録に残るためである。
