# Contract: 認証の HTTP 境界

親 Issue: #135。Plan: [plan.md](../plan.md)。

API の正本は [api/openapi.yaml](../../../api/openapi.yaml) で、この文書はこの feature が
足す経路・応答・Cookie と、既存の全経路に掛かる認証の境界だけを書く。ゲストに返す
内容の差と公開フラグの API は [guest-api.md](guest-api.md) にある。実装では
`openapi.yaml` に足して `task generate` で生成する。

## 1. 3つの扱い

すべての要求は、次の3つのどれかに入る。どれにも挙がらないものは「所有者だけ」である
（既定拒否、要件 10）。

| 扱い | 要求 | `openapi.yaml` の `security` |
| --- | --- | --- |
| 誰でも | `GET /api/health`、`GET /api/auth/session`、`POST /api/auth/setup`、`POST /api/auth/login`、`POST /api/auth/logout`、`/api/` で始まらない `GET`・`HEAD`（SPA のビルド成果物） | `[]` |
| ゲストも | `listVideos`、`getVideo`、`getRelatedVideos`、`streamVideo`、`getVideoPreview`、`transcodeVideo`、`getVideoThumbnail`、`getVideoSeekThumbnail`、`listRootFolders`、`getFolder`、`listFolderVideos` | `[{sessionCookie: []}, {}]` |
| 所有者だけ | 上以外のすべての `/api/*`（定義の無い経路を含む）。`listVideoIds`、`streamEvents`、`getProcessing`、スキャン、メディアフォルダ、ディレクトリ選択、タグ、`updateVideoTags`、`putVideoProgress`、`reprobeVideo`、`openVideoFile`、`updateVideoVisibility` もここに入る | 全体の既定 `[{sessionCookie: []}]` |

- 「ゲストも」の要求は、有効なセッションがあれば所有者として、無ければゲストとして
  処理する。ゲストとして処理した応答は [guest-api.md](guest-api.md) に従う。
- 「所有者だけ」の要求に有効なセッションが無ければ、§5 の未認証応答を返す。
- アカウントが未設定の間は、「誰でも」以外のすべての要求に §5 の未認証応答を返す
  （公開フラグも効かない、要件 2）。
- 境界は `path.Clean` した `r.URL.Path`（復号済み）で判定する。`//api/…`・`/./api/…`・
  `/%61pi/…`・`/api/../api/…` のような書き方でも、`/api/` 以下として扱う。
- SPA のビルド成果物は利用者データを含まない。画面の出し分けは SPA が §4 の状態で行う
  （[plan.md Structural Decisions 2](../plan.md#structural-decisions)）。
- Go のテストで、`openapi.yaml` の各操作の `security` と境界の分類が一致することを
  確かめる。`sessionCookie` は `in: cookie` の `apiKey` として宣言する。

## 2. `POST /api/auth/setup`

要求（`Content-Type: application/json`、本文は 8 KiB まで）:

```json
{ "username": "string", "password": "string" }
```

| 状況 | 応答 |
| --- | --- |
| 未設定で、値が [data-model.md §6](../data-model.md#6-ユーザー名とパスワードの値) を満たす | `200` `{ "redirectTo": "/" }` と `Set-Cookie`（§7）。そのままログイン済みになる |
| 値が規則を外れる | `400` `invalid_request`（どの欄かを `message` で示してよい。まだアカウントが無いので隠す値が無い） |
| 既に設定済み（同時の初回設定で負けた場合を含む） | `409` `{ "code": "account_already_configured", "message": "アカウントは既に設定されています" }`。何も書かない |
| 同一オリジンでない、JSON でない | 既存どおり `403`・`400` |

- 確認用のパスワードは画面で照らし合わせ、一致しなければ送らない（要件 5）。サーバーは
  1つのパスワードだけを受け取る。
- 成立は [data-model.md §5](../data-model.md#5-書き換えの規則) の主キーの衝突で1つに決まる。

## 3. `POST /api/auth/login`

要求（`Content-Type: application/json`、本文は 8 KiB まで）:

```json
{ "username": "string", "password": "string", "next": "/videos/12?t=30" }
```

`next` は省略できる。

| 状況 | 応答 |
| --- | --- |
| 成功 | `200` `{ "redirectTo": "<安全な戻り先>" }` と `Set-Cookie`（§7） |
| ユーザー名かパスワードが違う、空、上限超え、アカウントが未設定 | `401` `{ "code": "invalid_credentials", "message": "ユーザー名またはパスワードが違います" }` |
| 同じ送信元の「照合中 + 直近 5 分の失敗」が 5 以上、または照合の空きを 5 秒待っても得られない | `429` `{ "code": "login_throttled", … }` と `Retry-After`（秒） |
| 本文が JSON でない、8 KiB を超える、同一オリジンでない | 既存どおり `400`・`403` |

- 401 の4つの原因は、状態コード・本文・ヘッダーが同じである。応答時間を揃えるため、
  どの原因でも Argon2id の照合を1回行う（未設定なら固定のダミーのハッシュと照合する）。
  ユーザー名の比較は定数時間で行う（要件 12）。
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
  - パスが `/login`・`/setup` でも `/api/` 以下でもない。
  - 返すのは解釈した結果のパスと問い合わせ文字列を組み立て直した値で、入力そのもの
    ではない。

## 4. `GET /api/auth/session`・`POST /api/auth/logout`

`GET /api/auth/session` は `200` `{ "state": "owner" | "guest" | "setupRequired" }` を返す。

- `setupRequired` は未設定の状態で、Cookie の有無によらずこれを返す。
- ユーザー名は返さない。
- 問い合わせ文字列に `next` を付けて呼ぶと、`owner` のときだけ §3 と同じ規則で
  確かめた `redirectTo` も返す。ログイン済みで `/login` を開いた画面は、この値へ遷移する。

`POST /api/auth/logout` は `204` を返し、要求に付いていた認証の Cookie をすべて消す
（`Max-Age=0`）。§7 の読み分けと違い、ログアウトは届いた両方の名前の Cookie を見る。
HTTPS の要求には、同じホストの HTTP でログインした `vv_session`（`Secure` なし）も
届くので、HTTPS でのログアウトは HTTP のセッションも終わらせる。HTTP の要求には
`__Host-vv_session` が届かないので、HTTP でのログアウトは HTTP のセッションだけを
終わらせる。そのセッションは HTTPS でしか使えず、HTTP の経路からは読めない。
届いた Cookie が有効なセッションを指していれば、そのセッションを削除し、同じセッションで
処理中の応答を打ち切る（[plan.md Structural Decisions 5](../plan.md#structural-decisions)）。
同一オリジンの確認は他の状態変更と同じく掛かる。

## 5. 未認証とその他の応答

| 状況 | 応答 |
| --- | --- |
| 「所有者だけ」の要求で、Cookie が無い・形式が違う・該当するセッションが無い・期限切れ・資格情報の再設定後、または未設定 | `401` `{ "code": "unauthenticated", "message": "ログインが必要です" }` |
| 「ゲストも」の要求で、ゲストに見せない動画を指した | 既存の「その動画はありません」と同じ `404`（[guest-api.md §2](guest-api.md#2-ゲストに見せない動画)） |
| セッションか公開フラグの確認で DB が失敗した | `500` `{ "code": "internal", … }`（所有者とも公開ともみなさない） |

- 401 の原因は区別しない（Edge Case「不正、期限切れ、改ざん済みの Cookie」）。
  `WWW-Authenticate` は付けない（ブラウザの Basic 認証の窓を出さない）。
- 「ゲストも」の要求に不正・期限切れの Cookie が付いていたら、401 にせずゲストとして
  処理する。
- どれも `Cache-Control: no-store` で、HTML を返さない（要件 11）。
- DB の失敗を 401 にしないのは、画面がログイン画面へ送り、そこでも失敗する往復を
  作らないためである。

## 6. `GET /api/health`

応答の形は変えない（`status`・`version`・`commit`・`builtAt`）。ライブラリの内容、設定、
ユーザー名、認証の状態、初回設定の要否は載せない（受け入れ条件 16）。
`version` と `commit` は稼働中のバイナリの特定に使っているので残す。

## 7. セッション Cookie

| 接続 | 名前 | 属性 |
| --- | --- | --- |
| HTTPS | `__Host-vv_session` | `HttpOnly; Secure; SameSite=Strict; Path=/; Max-Age=<期限までの秒>` |
| HTTP | `vv_session` | `HttpOnly; SameSite=Strict; Path=/; Max-Age=<期限までの秒>` |

- 値はセッション ID（32 バイトの暗号学的乱数の base64url）である（要件 6）。
- `Max-Age` を付けるので、ブラウザを閉じても期限まで残る（受け入れ条件 10）。
- HTTP でも同じ仕組みで動き、HTTPS を前提にする属性・API は使わない（要件 14）。
- 認証では、HTTPS の要求は `__Host-vv_session` だけを、HTTP の要求は `vv_session` だけを
  読む（ログアウトは例外で、§4 のとおり届いた両方を見る）。
  名前を分けるので、HTTP の応答が HTTPS 用の Cookie を上書きしたり、HTTPS 用の
  Cookie が HTTP で送られたりしない（Edge Case「HTTP と HTTPS」）。
- 接続が HTTPS かどうかは、TLS で受けたか、信頼するプロキシの `X-Forwarded-Proto` で
  決める（[plan.md Structural Decisions 7](../plan.md#structural-decisions)）。

## 8. 同一オリジンの確認

既存の `acceptsSameOrigin`（`internal/httpapi/media_folders.go`）を、初回設定・ログイン・
ログアウトを含むすべての `POST`・`PUT`・`PATCH`・`DELETE` に掛ける規則のまま使う
（要件 13）。変えるのは、期待するスキームを §7 と同じ判定から取ることだけである。
今は `r.TLS` だけを見るので、TLS を終端するプロキシの後ろでは同一オリジンの要求も
拒否している。

## 9. 記録

初回設定・ログイン成功・失敗・試行制限・ログアウトを、それぞれ1行の `slog` で `info` に
出す。属性は出来事の種類と送信元だけで、送られたユーザー名・パスワード・セッション ID・
Cookie の値は出さない（要件 12）。送られたユーザー名を出さないのは、パスワードを
ユーザー名の欄に誤って入れた場合に平文が記録に残るためである。未設定のまま起動した
ときは、初回設定を促す警告を1行出す。
