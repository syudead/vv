# Implementation Plan: 単一アカウントでログインした人だけがライブラリ全体を使い、未ログインでは公開動画だけを閲覧できるようにする

**Branch**: `feature/016-single-account-auth` | **Parent Issue**: #135

**Input**: The parent Issue. It is this feature's specification.

## Summary

無認証の vv を、1つのアカウント（ユーザー名とパスワード）で保護する。アカウントは
未設定のサーバーへの最初のアクセスで画面から作り、その後の変更と再設定はホストの
コマンドで行う。パスワードは Argon2id で保存し、ログインで発行した不透明なセッション ID を
HttpOnly Cookie に入れ、SQLite に置いたセッションを要求ごとに確かめる。HTTP でも
すべて同じように動く。

要求は「誰でも」「ゲストも」「所有者だけ」の3つに分け、`internal/httpapi` の一番外側で
振り分ける。ゲスト（未ログイン）には、動画ごとの公開フラグが付いた動画だけを、同じ画面で
縮退させて見せる。公開の条件は保存層の一覧の問い合わせに1か所で入れ、ゲストの応答からは
絶対パス・タグ・再生位置を外す（親 Issue #135）。

HTTP の差分は [contracts/auth-api.md](contracts/auth-api.md)、ゲストへの応答と公開フラグの API は
[contracts/guest-api.md](contracts/guest-api.md)、ホストのコマンドは
[contracts/account-cli.md](contracts/account-cli.md)、足す表は [data-model.md](data-model.md) にある。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)・[.golangci.yml](../../.golangci.yml)（depguard）
- 認証の技術選定（Argon2id・HttpOnly Cookie・SQLite のセッション）:
  [tech-stack-selection.md §3](../../docs/design-docs/tech-stack-selection.md#3-決定事項)
- API の正本と生成: [api/openapi.yaml](../../api/openapi.yaml)（`task generate`）
- 今の同一オリジンの確認と JSON 本文の確認: `mutationBoundary`
  （[internal/httpapi/router.go](../../internal/httpapi/router.go)）・`acceptsSameOrigin`
  （[internal/httpapi/media_folders.go](../../internal/httpapi/media_folders.go)）
- 一覧の問い合わせと検索: [internal/store/listing.go](../../internal/store/listing.go)・
  [013 list-api.md](../013-library-search/contracts/list-api.md)
- 内容の識別子に結ぶ利用者データと一括の付け外し:
  [014 data-model.md](../014-video-tags/data-model.md)・[014 tags-api.md](../014-video-tags/contracts/tags-api.md)
- 起動設定: [cmd/mdm/config.go](../../cmd/mdm/config.go)・
  [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md)
- スキーマ: [internal/store/migrations/](../../internal/store/migrations/)。既存ファイルは
  変えない（[scripts/migrations-immutable.sh](../../scripts/migrations-immutable.sh)）
- Web の所有境界と視覚規則: [ARCHITECTURE.md#web-layer](../../ARCHITECTURE.md#web-layer)・
  [library-ui.md](../../docs/design-docs/library-ui.md)
- e2e の起動: [web/e2e/run-e2e.mjs](../../web/e2e/run-e2e.mjs)・
  [web/playwright.config.ts](../../web/playwright.config.ts)（Vite の開発サーバー越しに Go を叩く）
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task test-e2e`）

**Feature-specific context**:

- 技術選定からの差分: 選定表の「パスワード1つ」を「ユーザー名とパスワードの組」に
  改め、外部公開の前提を「HTTPS の逆プロキシを必須」に改める（要件 14）。
  tech-stack-selection.md のその行を実装で直す。
- 追加する依存: `golang.org/x/crypto`（`argon2`。選定済み）と `golang.org/x/term`
  （コマンドでパスワードをエコーせずに読む）。Web の依存は足さない。
- マイグレーションを2つ足す（`00009_auth.sql`・`00010_public_videos.sql`、
  [data-model.md §1](data-model.md#1-マイグレーション)）。
- 環境変数を1つ足す: `MDM_TRUSTED_PROXIES`（Structural Decisions 7）。
- HTTP でも全機能が動くよう、HTTPS を前提にする仕組み（Secure だけの Cookie、
  secure context でしか使えないブラウザの API）には頼らない（要件 14）。
- `ui` Issue なので、初回設定画面・ログイン画面・ゲストの画面での省き方・ログインと
  ログアウトの入口・公開の切り替えの見た目と操作は、この Plan のあとの design 工程で
  `ui-design.md` に決める。この Plan は、画面が守る API と遷移の契約までを決める。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。戻り先の判定・
  ユーザー名の規則・セッションの寿命・見る人（`Audience`）・ゲストの条件は `internal/domain`
  の純粋な関数と値にする。初回設定・ログインの照合・試行制限・発行・失効は `internal/app` に
  置き、保存とハッシュは自身の interface 越しに使う。Argon2id は新しいアダプタ
  `internal/password` に置き、depguard の兄弟パッケージの規則に足す。SQL は `internal/store` に閉じる。
- **API の正本**（AGENTS.md・ARCHITECTURE.md）: 合格。経路・`security`・`Video.public`・
  誤りの種別は `api/openapi.yaml` に足して生成する。生成物は手で編集しない。
- **索引と利用者データの区別**（ARCHITECTURE.md）: 合格。`account` と `public_videos` は
  利用者データ、`sessions` は一時的な状態として書く（[data-model.md §2](data-model.md#2-データの区分)）。
  `public_videos` は `content_key` に結び、`videos` に外部キーを張らない。
- **サーバーと話す場所**（ARCHITECTURE.md「Web layer」）: 合格。初回設定・ログイン・
  ログアウト・状態の確認・公開の切り替えと、401 を受けたときの扱いは `web/src/api/` に置き、
  画面は `fetch` しない。
- **UI の正本**（library-ui.md・`web/src/theme/tokens.test.ts`）: 合格の見込み。色と
  大きさはトークンだけを使う。構図は design 工程で決め、実装 PR に画像を添える。
- **利用者に見える設定は環境変数だけ**（cmd/mdm/config.go の方針）: 合格。足すのは
  `MDM_TRUSTED_PROXIES` だけで、アカウントと公開フラグは設定項目ではなく DB の利用者データである。

Phase 1 のあとも判定は同じで、正当化の要る違反は無い。

## Structural Decisions

1. **要求を「誰でも」「ゲストも」「所有者だけ」に分け、`internal/httpapi` の一番外側で
   振り分ける。どれにも挙がらないものは「所有者だけ」にする。**
   分類は `openapi.yaml` の `security` で表し（`[]`・`[{sessionCookie: []}, {}]`・既定）、
   境界の分類との一致を Go のテストで確かめる（[contracts/auth-api.md §1](contracts/auth-api.md#1-3つの扱い)）。
   境界は見る人（`domain.Audience`）を決めて要求の context に載せ、ハンドラはそれを読む。
   - 却下: oapi-codegen の生成した操作ごとの middleware で振り分ける案。定義の無い
     `/api/*` と SPA の経路を通らないので、既定拒否が構造にならない。
   - 却下: ハンドラごとに確かめる案。経路を足すたびに付け忘れうる。
2. **画面の出し分けは SPA のゲートで行い、`index.html` と `/assets/*` は誰にでも配る。**
   SPA は `GET /api/auth/session` の答え（`owner`・`guest`・`setupRequired`）が出るまで
   何も描かない。`setupRequired` ならどの経路でも初回設定画面、`guest` なら同じシェルを
   ゲスト向けに縮退させて描き、所有者だけの画面（設定・タグ）は `/login?next=…` へ送る。
   確認が失敗したときは、何も描かずに再試行の操作を持つ誤りの表示にとどめる。
   - 却下: 画面遷移をサーバーが 303 で振り分ける案。e2e と `task dev` は Vite の開発
     サーバーが `index.html` を配るので、そこでは効かない。使用中の失効は SPA が扱う必要が
     あり、同じ判断が2か所になる。
3. **最初のアカウントは画面の初回設定（`POST /api/auth/setup`）で作り、ホストの
   コマンドは作った後の変更と再設定だけを受け持つ**（[contracts/auth-api.md §2](contracts/auth-api.md#2-post-apiauthsetup)・
   [contracts/account-cli.md](contracts/account-cli.md)）。同時の初回設定は、`account` の
   主キーの衝突で1つに決める。
   - 却下: コマンドでもアカウントを作れるようにする案。作り方が2つになり、ユーザー名だけ
     設定した中間の状態が生まれる。初回設定の先着の危険は要求者が受け入れている。
   - 却下: ユーザー名とパスワードのハッシュを環境変数で渡す案。変えるたびに Compose の
     ファイルを編集して再起動する必要があり、パスワードを忘れたときの手順が重い。
   - 却下: 画面から後で変更・再設定できるようにする案。親 Issue が対象外にしている。
4. **セッションは ID の SHA-256 を SQLite に置き、要求ごとに DB で確かめる。無効化は
   `account.version` の一致で決める**（[data-model.md §4](data-model.md#4-セッションが有効である条件)）。
   メモリには持たない。
   - 却下: 署名付きの Cookie に状態を入れる案。要件 6 はサーバー側の状態と期限の管理を
     求め、ログアウトで直ちに無効にできない。
   - 却下: セッションをメモリだけに置く案。再起動で消え、受け入れ条件 10 を満たさない。
   - 却下: DB の結果をメモリに一時的に覚える案。別のプロセスのコマンドによる再設定が、
     覚えている間は効かない。1回の問い合わせは主キーの引き当てで、サムネイルが並ぶ
     一覧でも足りる。
   - 却下: セッション ID をそのまま保存する案。DB を読まれると、そのまま使える。
5. **資格を失った要求の、処理中の応答を打ち切る。** 境界は、所有者として処理する要求の
   context にセッションの期限を締め切りとして付け、ゲストとして処理する動画の配信は
   その動画の `content_key` と一緒に、メモリの台帳に載せる。ログアウトは同じセッションの
   要求を、非公開への切り替えはその `content_key` のゲストの要求を、台帳から打ち切る。
   別のプロセスのコマンドによる再設定は、30 秒を超えて続いている要求だけを 30 秒ごとに
   確かめ直して見つける（最長で約 30 秒。再設定の後に始まる要求はすべて拒む）。間隔と
   時計はテストで差し替えられるようにする。打ち切るときは context を取り消し、
   `http.ResponseController` で書き込みの締め切りを今にする。
   - 却下: context の取り消しだけにする案。`http.ServeContent` は context を見ずに
     書き続けるので、長い Range 応答が止まらない。
   - 却下: すべての要求を定期的に確かめ直す案。ほとんどの要求は数秒で終わるので、
     長く続くもの（`/api/events`・ライブ変換・大きな Range 応答）だけで足りる。
6. **ログインの試行制限は、`internal/app` のメモリ上で送信元ごとに数える。** 直近 5 分の
   失敗が 5 回に達した送信元は、最も古い失敗が 5 分を過ぎるまで 429 にする。照合の前に、
   制限の錠の中で1回分を予約し、「照合中の予約 + 記録した失敗」が 5 以上なら照合せずに
   429 にする。失敗で予約を失敗の記録に変え、成功で予約と記録を消す。429 は数えない。
   IPv6 は /64 を1つの送信元とする。記録は古いものから捨て、送信元の数に上限を置く
   （要件 12）。Argon2id の照合は同時に2つまでにし、空きを 5 秒待っても得られない要求は、
   失敗に数えずに `Retry-After` 付きの 429 にする。
   - 却下: 失敗を記録した後で数えるだけにする案。同時に送った要求はどれも「まだ 5 回
     未満」を通って照合まで進み、制限を回避できる。
   - 却下: DB に記録する案。再起動できるのはホストの管理者だけで、攻撃者は制限を
     解けない。ログインのたびの書き込みが増えるだけである。
   - 却下: アカウント単位で締める案。誰でも単一アカウントをロックできる（Edge Case
     「恒久的にロックせず」）。
   - 却下: IPv6 をアドレス単位で数える案。1つの /64 を持つ攻撃者が制限を回避できる。
7. **送信元と HTTPS の判定は、`MDM_TRUSTED_PROXIES` に書いたプロキシからの要求でだけ
   転送ヘッダーを読む。** 直接の接続元がそこに含まれるときだけ、`X-Forwarded-For` を
   右から辿って最初の信頼しないアドレスを送信元とし、`X-Forwarded-Proto` の最後の値で
   HTTPS かを決める。既定は空で、ヘッダーを読まない。この判定を、試行制限、Cookie の
   名前と `Secure`、同一オリジンの確認、既定アプリで開く操作のループバックの確認、
   記録の送信元に使う。プロキシは `Host` をそのまま渡す前提とし、運用文書に書く。
   - 却下: 転送ヘッダーを常に信じる案。直接つないだ利用者が偽って、試行制限と
     `Secure` の判定を回避できる（Edge Case「リバースプロキシ配下」）。
   - 却下: `Forwarded`（RFC 7239）や `X-Forwarded-Host` も読む案。主な逆プロキシの
     既定は `X-Forwarded-For`/`-Proto` で、読むヘッダーを増やすほど偽装の面が増える。
8. **セッション Cookie の名前を、HTTPS では `__Host-vv_session`、HTTP では `vv_session` に
   分ける**（[contracts/auth-api.md §7](contracts/auth-api.md#7-セッション-cookie)）。
   - 却下: 1つの名前で、HTTPS のときだけ `Secure` を付ける案。同じホストを HTTP と
     HTTPS の両方で使うと、HTTP でのログインの `Set-Cookie` がブラウザに捨てられ、
     ログインしてもゲストのままになる。
9. **Argon2id は新しいアダプタ `internal/password` に置き、PHC 文字列で保存する。**
   パラメータは m=19456 KiB、t=2、p=1、ソルト 16 バイト、鍵 32 バイトとする。
   照合は文字列に書かれたパラメータで行うので、後で強めても既存のハッシュは読める。
   - 却下: m=64 MiB・t=3（RFC 9106 の第2推奨）。NAS や小型機で、同時の照合が
     メモリを圧迫する。19 MiB・t=2 は OWASP の最低推奨を満たす。
   - 却下: `internal/domain` に置く案。ソルトの乱数と外部の暗号実装は domain の
     「純粋な規則」に入らない。`internal/app` に置くと、単体テストが本物の Argon2id の
     時間を払う。
10. **ログイン後の戻り先は、サーバーが `domain` の規則で確かめて `redirectTo` として返す。**
    ログインの応答と、ログイン済みのときの `GET /api/auth/session?next=…` の応答が返し、
    画面はその値へ遷移するだけにする（[contracts/auth-api.md §3](contracts/auth-api.md#3-post-apiauthlogin)・
    [§4](contracts/auth-api.md#4-get-apiauthsessionpost-apiauthlogout)）。
    - 却下: 画面で `next` を確かめる案。オープンリダイレクトの判定が TypeScript と Go の
      2か所に分かれる。
11. **動画・所在・フォルダを返す `LibraryStore` の読み出しは、すべて `domain.Audience` を
    引数に取り、ゲストでは公開の条件を一覧の問い合わせの組み立てに1か所で足す**
    （[data-model.md §3](data-model.md#3-見る人と公開の動画の条件)）。引数にするので、
    呼び出し側は1つ残らずどちらで読むかをコンパイル時に決めることになる。`Audience` の
    ゼロ値はゲストにする。
    - 却下: 問い合わせの構造体に「非公開も含める」欄を足す案。書き忘れても黙って
      コンパイルが通る。
    - 却下: ゲスト用の役割の型を別に作り、同じ読み出しを並べる案。同じ SQL を2つ保守し、
      片方だけ直す誤りを生む。
    - 却下: `internal/httpapi` で結果から非公開を取り除く案。`total`・ページの区切り・
      フォルダの件数が非公開の分だけずれ、数から存在が漏れる（要件 16）。
12. **公開フラグは `content_key` に結ぶ表 `public_videos` に置き、行の有無で表す**
    （[data-model.md §1](data-model.md#1-マイグレーション)）。切り替えの API は
    タグの付け外しと同じ形の `PUT /api/video-visibility` にする（[contracts/guest-api.md §4](contracts/guest-api.md#4-公開フラグの切り替え)）。
    - 却下: `videos` に列を足す案。`videos` は再構築できる索引で、内容が変わったときや
      フォルダを外したときに行ごと消え、要件 15 の「失われない」を満たさない。
    - 却下: 公開をタグの1つとして表す案。タグの改名・統合・削除で公開が意図せず変わり、
      ゲストに隠すべきタグの仕組みと混ざる。
13. **ゲストの一覧では、所有者のデータに依る条件を受け付けない**
    （[contracts/guest-api.md §3](contracts/guest-api.md#3-ゲストが使えない条件)）。`watch`・
    再生日時の並べ替え・`tag` は 400 にし、検索はタグの名前に照合しない。
    - 却下: そのまま受け付ける案。結果の件数や順序から、非公開の再生位置とタグの名前が
      漏れる（要件 18）。
14. **見る人が変わるとき（ログイン・ログアウト・失効）は、画面をページごと読み直す。**
    `web/src/api/client.ts` が所有者として送った要求に 401 を受けたら、1度だけ今の URL を
    読み直す。読み直した画面はゲートで状態を取り直し、ゲストとして描く（所有者だけの画面は
    ログイン画面へ送る）。
    - 却下: 読み直さずに画面の状態だけを切り替える案。一覧の控え（`listSnapshot`）や
      タグの控えなど、所有者として読んだ非公開のデータがメモリに残り、ゲストの画面に
      出うる（UI 品質「一瞬表示してから切り替える挙動は認めない」）。

## Project Structure

### Documentation (this feature)

```text
specs/016-single-account-auth/
├── plan.md
├── data-model.md
└── contracts/
    ├── auth-api.md
    ├── guest-api.md
    └── account-cli.md
```

`ui-design.md` は、この Plan のあとの design 工程で足す。判断は上の Structural Decisions で
閉じており、別に調べて決めることが無いので `research.md` は作らない。検証は各実装単位の
自動テストと e2e で行い、それ以外に手で走らせる手順が無いので `quickstart.md` も作らない。
HTTPS で公開するための手順は、実装で運用文書（`docs/how-to/running-vv.md`）に書く。

### Source Code

**Affected boundaries**:

- `internal/domain/`: ユーザー名の規則、セッションの寿命、戻り先の判定、`Audience`、ゲストの条件、認証の誤り
- `internal/password/`（新規）: Argon2id のハッシュ化と照合、PHC 文字列
- `internal/store/`: マイグレーション2つ、役割の型 `AuthStore`、公開フラグの読み書き、
  `LibraryStore` の読み出しへの `Audience`
- `internal/app/`: 初回設定・ログイン・試行制限・セッションの確認と失効（`Auth`）、
  関連動画の組み立てへの `Audience`（`Catalog`）
- `api/openapi.yaml` と生成物: 認証の経路、`security`、`Video.public`、公開の切り替え、
  `FolderSummary.rootPath` の任意化、誤りの種別
- `internal/httpapi/`: 3つの扱いの境界、Cookie、ゲストの応答の形、送信元と HTTPS の判定、
  処理中の応答の打ち切り、同一オリジンとループバックの確認の判定元の差し替え
- `cmd/mdm/`: `account` の下位コマンド、`MDM_TRUSTED_PROXIES`、組み立て、起動時の期限切れの掃除
- `web/src/api/`: 状態の確認・初回設定・ログイン・ログアウト・公開の切り替え、401 での読み直し
- `web/src/auth/`（新規）: ゲート、見る人の文脈、初回設定画面、ログイン画面
- `web/src/app/`・`web/src/shell/`・`web/src/library/`・`web/src/folders/`・`web/src/videoList/`・
  `web/src/player/`: ゲストでの省き方、ログインとログアウトの入口、公開の切り替え
- `web/e2e/`: e2e の初回設定とログイン、ゲストの閲覧
- `.golangci.yml`・`ARCHITECTURE.md`・`README.md`・`docs/how-to/running-vv.md`・
  `docs/design-docs/tech-stack-selection.md`・`compose.yaml`

**New paths**: `internal/domain/auth.go`・`internal/password/`・`internal/store/auth.go`・
`internal/store/visibility.go`・`internal/store/migrations/00009_auth.sql`・
`internal/store/migrations/00010_public_videos.sql`・`internal/app/auth.go`・
`internal/httpapi/auth.go`・`internal/httpapi/visibility.go`・`internal/httpapi/client_origin.go`・
`cmd/mdm/account.go`・`web/src/api/auth.ts`・`web/src/auth/`・`web/e2e/auth.e2e.ts`・
`web/e2e/guest.e2e.ts`（それぞれ対応する test を含む）

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。
新しいパッケージは `internal/password` だけで、理由は Structural Decisions 9 のとおり。

## Implementation Work

### パスワードを Argon2id でハッシュ化し、認証と見る人の規則を domain に置く

**Scope**: `internal/password` に Argon2id のハッシュ化と照合（PHC 文字列）を置き、
`.golangci.yml` の兄弟パッケージの規則と `ARCHITECTURE.md` の依存方向に足す
（Structural Decisions 9）。`internal/domain/auth.go` に、ユーザー名とパスワードの値の規則
（[data-model.md §6](data-model.md#6-ユーザー名とパスワードの値)）、セッションの寿命 90 日、
戻り先の判定（[contracts/auth-api.md §3](contracts/auth-api.md#3-post-apiauthlogin)）、`Audience`、
ゲストの条件の判定（[contracts/guest-api.md §3](contracts/guest-api.md#3-ゲストが使えない条件)）、
認証の誤りを置く。まだどこからも呼ばない。

**Dependencies**: None

**Acceptance**: `go test ./internal/password/... ./internal/domain/...` で次が通る。同じ
パスワードを2回ハッシュ化すると別の文字列になり、どちらも照合に通る。1文字違いと
大文字小文字違いは通らない。別のパラメータで作った PHC 文字列も照合できる。壊れた
PHC 文字列は誤りになる。戻り先は `/videos/1?t=2` を保ち、`//evil.example`・`/\evil`・
`/\t/evil.example`・`/\n/evil.example`・`/a\\b`・`https://evil.example/`・`/login`・`/setup`・
`/api/videos`・空を `/` にする。ユーザー名の規則は空・制御文字・前後の空白・129 文字を
拒む。`Audience` のゼロ値はゲストである。ゲストの条件の判定は `watch=unwatched`・
`sort=playedDesc`・`tag` を拒み、既定の条件を通す。`task check` が通る。

### アカウントとログインセッションを SQLite に保存する

**Scope**: マイグレーション `00009_auth.sql` と役割の型 `AuthStore` を足す
（[data-model.md](data-model.md) の §1・§4・§5）。初回設定（アカウントと最初のセッションを
1つの取引で作る）、ユーザー名とパスワードの変更、アカウントの読み出し、セッションの追加・
有効性の確認・削除・期限切れの掃除を持つ。`ARCHITECTURE.md` の役割の型の一覧とデータの
区分を更新する。

**Dependencies**: パスワードを Argon2id でハッシュ化し、認証と見る人の規則を domain に置く

**Acceptance**: `internal/store` のテストで次が通る。行が無ければ未設定として読める。
初回設定は1回だけ成功し、2回目と同時に走らせたもう一方は「設定済み」の誤りで何も
書かない。未設定でのユーザー名とパスワードの変更は誤りになる。資格情報を書き換えると
既存のセッションが無効になり、書き換え前に読んだ版で足したセッションも無効になる。
期限を過ぎたセッションは無効で、確かめたときと掃除で行が消える。ユーザー名は大文字
小文字を区別して保存される。Down で2つの表が消える。`scripts/migrations-immutable.sh` と
`task check` が通る。

### 公開フラグを保存し、ゲストには公開の動画だけを返す問い合わせにする

**Scope**: マイグレーション `00010_public_videos.sql` と、公開・非公開の一括の切り替えを足す
（[data-model.md §1・§5](data-model.md#5-書き換えの規則)）。動画・所在・フォルダを返す
`LibraryStore` の読み出しに `domain.Audience` を足し、ゲストでは公開の条件を一覧の
問い合わせの組み立てに入れる（Structural Decisions 11、[data-model.md §3](data-model.md#3-見る人と公開の動画の条件)）。
ゲストの検索はタグの名前に照合しない。`internal/app` の関連動画の組み立ても `Audience` を
受け取る。今の呼び出し側はすべて所有者として読む（振る舞いは変えない）。`Video` に
公開かどうかを載せる。`ARCHITECTURE.md` のデータの区分と `LibraryStore` の記述を更新する。

**Dependencies**: パスワードを Argon2id でハッシュ化し、認証と見る人の規則を domain に置く

**Acceptance**: `internal/store` と `internal/app` のテストで次が通る。公開の動画と非公開の
動画を混ぜた索引で、ゲストの一覧・`total`・検索・並べ替えの全ページ・フォルダの一覧と
件数・フォルダの動画・関連動画・1本の読み出しに公開の動画だけが現れ、所有者では全件が
現れる。非公開の動画だけを含むフォルダはゲストに現れない。ゲストの検索は、タグの名前にだけ
当たる動画を返さない。公開にした動画は、所在を移しても、内容が同じ別の所在を足しても
公開のままで、最後の所在が消えるとゲストに現れない。切り替えは、ライブラリに無い id を
数えず、全部に反映するか何も反映しない。既存の一覧・フォルダ・関連動画のテストが通る。
`task check` が通る。

### サーバー管理者がホストのコマンドでユーザー名とパスワードを変えられるようにする

**Scope**: `cmd/mdm/account.go` に `mdm account set-username`・`set-password` を足す
（[contracts/account-cli.md](contracts/account-cli.md)）。引数なしの起動は変えない。
`golang.org/x/term` を足す。`docs/how-to/running-vv.md` に、導入直後に画面で初回設定する
こと（先着の危険）と、コマンドでの変更・再設定の手順を書く。

**Dependencies**: アカウントとログインセッションを SQLite に保存する

**Acceptance**: `cmd/mdm` のテストで次が通る。設定済みのデータディレクトリに対して
2つのコマンドを実行すると、ユーザー名とパスワードが変わり、既存のセッションが無効になる。
標準入力から渡したパスワードが照合に通り、DB と標準出力・標準エラーに平文が現れない。
未設定のデータディレクトリ、空のユーザー名、未知の下位コマンドは終了コード 2 で何も
書かない。`ffmpeg` を `PATH` から外しても動く。`task check` が通る。

### 初回設定・ログインの照合・試行制限・セッションの発行と失効をアプリケーション層に置く

**Scope**: `internal/app/auth.go` に `Auth` を置く。初回設定、ログイン（常に1回の照合、定数
時間のユーザー名比較、未設定時のダミーの照合、照合の同時実行の上限）、送信元ごとの試行制限
（Structural Decisions 6）、セッションの発行・確認・ログアウト、状態（`owner`・`guest`・
`setupRequired`）を持つ。保存・ハッシュ・時計・乱数は自身の interface 越しに使う。

**Dependencies**: アカウントとログインセッションを SQLite に保存する

**Acceptance**: `go test ./internal/app/...` で次が通る。未設定では初回設定がセッションを
返し、設定済みでは「設定済み」の誤りになる。誤ったユーザー名、誤ったパスワード、両方誤り、
空欄、未設定は同じ誤りになり、どれも照合を1回呼ぶ。大文字小文字だけ違うユーザー名は
失敗する。同じ送信元の5回の失敗のあとは、ユーザー名を変えても照合を呼ばずに制限の誤りに
なり、時計を 5 分進めると正しい資格情報で成功する。同じ送信元から誤った資格情報で 20 件を
同時に送っても、照合は 5 回までしか呼ばれない。照合の空きを待ち切れない要求は、失敗に
数えずに制限の誤りになる。別の IPv6 アドレスでも同じ /64 なら制限が掛かる。発行した ID は
32バイトで、保存されるのはそのハッシュだけである。有効なセッションの Cookie を持ったまま
ログインすると、古いセッションは消える。`task check` が通る。

### 要求を3つの扱いに振り分け、初回設定・ログインの API とゲストの応答を公開する

**Scope**: `api/openapi.yaml` に [contracts/auth-api.md](contracts/auth-api.md) の §1〜§7 と
[contracts/guest-api.md](contracts/guest-api.md) の §1〜§3 の差分を足し、`task generate` で生成する。
`internal/httpapi/auth.go` に3つの扱いの境界（Structural Decisions 1）、初回設定・ログイン・
ログアウト・状態の経路、Cookie、処理中の応答の打ち切りのうちセッションの分（Structural
Decisions 5）、認証の記録（auth-api §9）を置く。ゲストの応答から絶対パス・タグ・再生位置を
外し、ゲストの条件を 400 にする。この単位では接続元のアドレスと `r.TLS` をそのまま送信元と
HTTPS の判定に使う。`/api/auth/setup`・`/api/auth/login` を `requiresJSONBody` に足す。境界は
`path.Clean` した経路で判定する。`cmd/mdm` で組み立て、起動時に期限切れのセッションを消し、
未設定なら警告を記録する。e2e は Playwright のセットアップが `POST /api/auth/setup` で
アカウントを作り、そのログインした状態を既存の全テストに渡す。e2e の失敗の試行は、同じ
送信元の試行制限に掛からない回数に収める。`ARCHITECTURE.md` に認証の境界を書き、
「Not built yet」から認証を外す。

**Dependencies**: 公開フラグを保存し、ゲストには公開の動画だけを返す問い合わせにする。
初回設定・ログインの照合・試行制限・セッションの発行と失効をアプリケーション層に置く

**Acceptance**: `internal/httpapi` のテストで次が通る。
- 未設定では、`/api/auth/session` が `setupRequired` を返し、それ以外の保護対象は
  「ゲストも」の経路を含めて 401 になる。初回設定が 200 と Cookie を返し、その Cookie で
  所有者になる。2回目の初回設定は 409 `account_already_configured` になる。
- Cookie なしで、スキャン・設定・タグ・再生位置・`/api/events`・`/api/videos/ids`・読み取りの
  やり直し・既定アプリで開く・定義の無い `/api/x` を要求すると 401 `unauthenticated` の JSON が
  返る。`//api/videos`・`/./api/scans`・`/%61pi/scans`・`/api/../api/scans` も同じになる。
- Cookie なしの一覧・フォルダ・関連動画には公開の動画だけが出て、`location`・`progress`・
  `rootPath` が無く `tags` が空である。非公開の動画の詳細・Range 再生・サムネイル・シーク
  プレビュー・ホバープレビュー・ライブ変換は、存在しない動画と同じ 404 になる。ゲストの
  `watch=watched`・`sort=playedAsc`・`tag` は 400 になる。
- ログインした Cookie ではすべてが既存どおり返る。ログアウト後、パスワードの再設定後、
  ユーザー名の変更後は、同じ Cookie が所有者として扱われず、処理中の `/api/events` と Range
  応答が終わる。HTTPS で `__Host-vv_session` と `vv_session` を両方付けてログアウトすると、
  どちらのセッションも所有者として扱われない。同じデータディレクトリで組み立て直した
  サーバーでも、期限内の Cookie は通る。90 日を過ぎたセッションは所有者として扱われない。
- セッションの確認で DB が失敗すると 500 になり、保護対象を返さない。
- 3通りの誤りのログインが同じ状態・本文になる。同じ送信元の6回目の誤りは 429
  `login_throttled` と `Retry-After` になる。HTTP のログインの `Set-Cookie` は `vv_session` に
  `HttpOnly`・`SameSite=Strict`・`Path=/`・`Max-Age` を持つ。JSON でない本文の初回設定と
  ログインは 400、別オリジンの `Origin` の `POST` は 403 になる。
- `GET /api/health` は未認証で既存の形を返し、未設定かどうかで応答が変わらない。
- `security` の分類と境界が一致する。記録にパスワードとセッション ID が出ない。

既存の e2e を含む `task check` と `task test-e2e` が通る。

### 公開・非公開を切り替える API を足し、非公開にした動画のゲストへの配信を止める

**Scope**: `PUT /api/video-visibility` と `Video.public` を `openapi.yaml` に足して生成し
（[contracts/guest-api.md §4](contracts/guest-api.md#4-公開フラグの切り替え)）、`internal/httpapi/visibility.go` に
置く。非公開にしたとき、ゲストとして処理中のその動画の応答を台帳から打ち切る（Structural
Decisions 5、[contracts/guest-api.md §5](contracts/guest-api.md#5-公開をやめたときの配信)）。
`web/src/api/client.ts` に切り替えの関数を足す（画面はまだ使わない）。

**Dependencies**: 要求を3つの扱いに振り分け、初回設定・ログインの API とゲストの応答を公開する

**Acceptance**: `internal/httpapi` のテストで次が通る。所有者が `PUT /api/video-visibility` で
公開にすると、Cookie なしの一覧と詳細にその動画が現れ、`Video.public` が `true` になる。
非公開に戻すと、ゲストとして処理中のその動画の Range 応答とライブ変換が終わり、次の要求は
404 になる。所有者として処理中の応答は終わらない。Cookie なしの切り替えは 401 になる。
`task check` が通る。

### 信頼するリバースプロキシの転送ヘッダーから送信元と HTTPS を判定する

**Scope**: `MDM_TRUSTED_PROXIES` を `cmd/mdm/config.go` に足し、`internal/httpapi/client_origin.go`
で送信元と HTTPS を判定する（Structural Decisions 7）。その判定を、試行制限、Cookie の名前と
`Secure`、同一オリジンの確認（[contracts/auth-api.md §8](contracts/auth-api.md#8-同一オリジンの確認)）、
既定アプリで開く操作のループバックの確認、記録に使う。`docs/how-to/running-vv.md` に
HTTPS の逆プロキシで公開するための要件（HTTPS 必須、`Host` を渡す、転送ヘッダー、
`MDM_TRUSTED_PROXIES`）と Caddy の設定例を書く。プロキシを `MDM_TRUSTED_PROXIES` に
書き忘れると、全員がプロキシのアドレスとして試行制限を共有し、HTTPS の `Origin` の
`POST` が 403 になることも書く。`compose.yaml` で環境変数を渡せるようにする。

**Dependencies**: 要求を3つの扱いに振り分け、初回設定・ログインの API とゲストの応答を公開する

**Acceptance**: `internal/httpapi` と `cmd/mdm` のテストで次が通る。信頼しない接続元が
付けた `X-Forwarded-For` と `X-Forwarded-Proto: https` は無視され、試行制限は接続元の
アドレスで掛かり、Cookie は `vv_session` で `Secure` が付かない。信頼する接続元からの
`X-Forwarded-Proto: https` では `__Host-vv_session` に `Secure` が付き、`Origin: https://<Host>` の
`POST` が通る。HTTP の要求は認証に `__Host-vv_session` を読まない。信頼するプロキシが同じ PC に
あっても、転送元が外部なら既定アプリで開く操作は 403 になる。不正な CIDR は起動時に
他の設定の誤りと一緒に報告される。`task check` が通る。

### 初回設定画面とログイン画面を表示し、見る人に応じて画面を出し分ける

**Scope**: `web/src/api/auth.ts` に状態の確認・初回設定・ログイン・ログアウトを置く。
`web/src/auth/` に、確認が済むまで何も描かないゲート、見る人の文脈、初回設定画面（ユーザー名・
パスワード・確認用パスワード、HTTP の警告）、ログイン画面（HTTP の警告、失敗と試行制限の
表示）を置く。`App.tsx` で `/setup`・`/login` をシェルとプロバイダの外に出し、他の経路を
ゲートの内側に入れる（Structural Decisions 2）。ゲストでは所有者だけの画面（設定・タグ）を
`/login?next=…` へ送る。初回設定とログインの成功後、ログアウトの後は、返った `redirectTo` か
今の URL へページごと遷移する（Structural Decisions 14）。シェルにログインとログアウトの
入口を置く。配置・文言・見た目は `ui-design.md` による。

**Dependencies**: 要求を3つの扱いに振り分け、初回設定・ログインの API とゲストの応答を公開する。
design 工程の `ui-design.md` が feature ブランチに入っていること

**Acceptance**: Vitest で、確認中は何も描かれず、`setupRequired` でどの経路も初回設定画面に、
`guest` で `/settings`・`/tags` が `/login?next=…` に、`owner` で `/login` からサーバーが返した
`redirectTo` へ移ることが確かめられる。確認が 500 や通信の失敗のときは、シェルを描かず、
遷移せず、確認を自動で繰り返さない。確認用パスワードが一致しなければ初回設定を送らない。
`web/e2e/auth.e2e.ts` で次が確かめられる。
- 未設定のサーバーでどの URL も初回設定画面になり、設定するとログイン済みの一覧になる。
- ゲストで `/settings` を開くとログイン画面になり、ログインすると `/settings` に戻る。
- ログインの送信中は再送信できず、失敗後にパスワード欄だけが空になりユーザー名が残る。
- キーボードだけで入力から送信まで進められる。HTTP で両画面に警告が出る。
- 欄に `autocomplete` の `username`・`current-password`（初回設定は `new-password`）が付く。
- ログアウトするとゲストの画面になり、ログアウト前の Cookie を付け直しても所有者として
  扱われない。
- `localStorage` と `sessionStorage` にパスワードとセッション ID が無い。

`task check` と `task test-e2e` が通る。画面が変わるので、実装 PR に 360・768・1280 px の
画像と、視覚・操作・支援技術の確認を添える。

### ゲストの画面で公開の動画だけを見せ、所有者だけの操作を省く

**Scope**: ゲストのとき、シェル・ライブラリ・フォルダ・再生画面から、設定・スキャンと
取り込みの進捗・タグ（表示・絞り込み・管理）・複数選択・公開の切り替え・既定アプリで開く・
再生位置の表示と保存・視聴状態の絞り込み・再生日時の並べ替えを出さない。
`ScanProvider` などの所有者だけのプロバイダを付けず、`/api/events` を開かない。URL に残った
ゲストで使えない条件は既定に丸めてから要求する（[contracts/guest-api.md §3](contracts/guest-api.md#3-ゲストが使えない条件)）。
`client.ts` は所有者として送った要求の 401 で、1度だけページを読み直す（Structural Decisions 14）。
再生画面は、動画・変換の読み込みの失敗で状態を確かめ、見る人が変わっていれば読み直す。
`README.md` の「認証が無い」警告を、初回設定と HTTPS の案内に改め、
`docs/design-docs/tech-stack-selection.md` の認証の行を更新する。省き方と見た目は `ui-design.md` による。

**Dependencies**: 初回設定画面とログイン画面を表示し、見る人に応じて画面を出し分ける

**Acceptance**: Vitest で、ゲストの文脈ではシェル・一覧・再生画面に上の操作が描かれず、
所有者の文脈では描かれることが確かめられる。401 を返す取得が何度あっても読み直しは1回で、
ゲストでは `/api/events` を開かない。`web/e2e/guest.e2e.ts` で次が確かめられる。
- 所有者が公開にした動画だけが、ゲストの一覧・検索・フォルダ・関連動画に出て、件数も
  それに合う。非公開の動画だけのフォルダは出ない。
- ゲストで公開の動画を再生でき、非公開の動画の再生 URL は「見つからない」表示になる。
- ゲストの画面に、設定・スキャン・タグ・選択・公開の切り替え・再生位置が一度も出ない。
- 再生中に別のブラウザでログアウトしたセッションの画面は、次の操作でゲストの画面になる。

`task check` と `task test-e2e` が通る。画面が変わるので、実装 PR に 360・768・1280 px の
画像と、視覚・操作・支援技術の確認を添える。

### ログイン中に動画ごとと一括で公開・非公開を切り替えられるようにする

**Scope**: 再生画面に1本の公開・非公開の切り替えを、ライブラリの選択バーに選んだ動画の
一括の切り替え（公開にする・非公開にする）を置く。一覧のカードと再生画面で、公開かどうかを
色だけに頼らず示す。切り替えの後は、一覧の該当の項目の `public` を差し替える（読み直さない、
タグの付け外しと同じ扱い）。置き場所と見た目は `ui-design.md` による。

**Dependencies**: 公開・非公開を切り替える API を足し、非公開にした動画のゲストへの配信を止める。
ゲストの画面で公開の動画だけを見せ、所有者だけの操作を省く

**Acceptance**: Vitest で、切り替えが `PUT /api/video-visibility` を1回だけ送り、応答の後に
該当の項目の表示が変わることが確かめられる。`web/e2e/guest.e2e.ts` で、再生画面から1本を、
選択バーから複数本を公開にすると、別のブラウザのゲストの一覧に現れ、非公開に戻すと消える
ことが確かめられる。再スキャンの後も公開のままである。`task check` と `task test-e2e` が
通る。画面が変わるので、実装 PR に画像と、視覚・操作・支援技術の確認を添える。
