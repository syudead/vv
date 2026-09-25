# Implementation Plan: ユーザー名とパスワードによる単一アカウント認証でライブラリ全体を保護する

**Branch**: `feature/016-single-account-auth` | **Parent Issue**: #135

**Input**: The parent Issue. It is this feature's specification.

## Summary

無認証の vv を、管理者がホストのコマンドで設定する1つのアカウント（ユーザー名と
パスワード）で保護する。パスワードは Argon2id で保存し、ログインで発行した不透明な
セッション ID を HttpOnly Cookie に入れ、SQLite に置いたセッションを要求ごとに確かめる。
`internal/httpapi` の一番外側に既定拒否の境界を置き、`GET /api/health` とログインに
要る経路と SPA の静的資産だけを通す。画面は、セッションの確認が済むまでシェルを
描かず、未認証ならログイン画面へ送る（親 Issue #135）。

HTTP の差分は [contracts/auth-api.md](contracts/auth-api.md)、ホストのコマンドは
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
- マイグレーションを1つ足す（`00009_auth.sql`、[data-model.md §1](data-model.md#1-マイグレーション)）。
- 環境変数を1つ足す: `MDM_TRUSTED_PROXIES`（Structural Decisions 7）。
- `ui` Issue なので、ログイン画面・未設定の案内・ログアウトの置き場所の見た目と
  操作は、この Plan のあとの design 工程で `ui-design.md` に決める。この Plan は、画面が
  守る API と遷移の契約までを決める。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。戻り先の判定・
  ユーザー名の規則・セッションの寿命は `internal/domain` の純粋な関数と値にする。ログインの
  照合・試行制限・発行・失効は `internal/app` に置き、保存とハッシュは自身の interface
  越しに使う。Argon2id は新しいアダプタ `internal/password` に置き、depguard の兄弟
  パッケージの規則に足す。SQL は `internal/store` の新しい役割の型に閉じる。
- **API の正本**（AGENTS.md・ARCHITECTURE.md）: 合格。経路・`security`・誤りの種別は
  `api/openapi.yaml` に足して生成する。生成物は手で編集しない。
- **索引と利用者データの区別**（ARCHITECTURE.md）: 合格。`account` は利用者データ、
  `sessions` は一時的な状態として ARCHITECTURE.md に書く（[data-model.md §2](data-model.md#2-データの区分)）。
  マイグレーションは新しいファイルとして足す。
- **サーバーと話す場所**（ARCHITECTURE.md「Web layer」）: 合格。ログイン・ログアウト・
  セッションの確認と、401 を受けたときのログイン画面への遷移は `web/src/api/` に置き、
  画面は `fetch` しない。
- **UI の正本**（library-ui.md・`web/src/theme/tokens.test.ts`）: 合格の見込み。色と
  大きさはトークンだけを使う。構図は design 工程で決め、実装 PR に画像を添える。
- **利用者に見える設定は環境変数だけ**（cmd/mdm/config.go の方針）: 合格。足すのは
  `MDM_TRUSTED_PROXIES` だけで、アカウントは設定項目ではなく DB の利用者データとして
  コマンドで書く（Structural Decisions 3）。

Phase 1 のあとも判定は同じで、正当化の要る違反は無い。

## Structural Decisions

1. **認証の境界は `internal/httpapi` の一番外側に1つ置き、許可リスト以外を既定で拒否する。**
   `NewRouter` が返すハンドラの外側で、[contracts/auth-api.md §1](contracts/auth-api.md#1-認証の境界) の
   許可リストに無い要求を、生成した経路に渡す前に確かめる。許可リストと
   `openapi.yaml` の `security: []` の一致は Go のテストで確かめる。
   - 却下: oapi-codegen の生成した操作ごとの middleware（`security` の値を見る）で守る案。
     定義の無い `/api/*` と SPA の経路を通らないので、既定拒否が構造にならない。
   - 却下: ハンドラごとに確かめる案。経路を足すたびに付け忘れうる。
2. **画面の認証は SPA のゲートで行い、`index.html` と `/assets/*` は認証なしで配る。**
   SPA は、`GET /api/auth/session` の答えが出るまでシェルもプロバイダも描かず、
   `unauthenticated`・`setupRequired` なら `/login?next=<今の場所>` へ置き換える。確認が
   失敗した（500・通信の失敗）ときは、シェルを描かず、ログイン画面へも送らず、
   再試行の操作を持つ誤りの表示にとどめる（Edge Case「セッション DB の失敗」）。`/login` は
   `ScanProvider` などの外に置き、`/api/events` を開かない。ビルド成果物は利用者データを
   含まず、ログイン画面の静的資産を兼ねる（要件 9 の例外）。
   - 却下: 未認証の画面遷移をサーバーが `/login` へ 303 で送る案。e2e と `task dev` は
     Vite の開発サーバーが `index.html` を配るので、そこでは効かず、受け入れ条件 2 を
     自動で確かめられない。使用中の失効は SPA が扱う必要があり、同じ判断が2か所になる。
   - 却下: ログイン画面を別の HTML の入口にし、その資産だけを公開する案。`/assets/*` の
     どれがログイン画面の分かをサーバーがビルドの目録から知る必要があり、隠すべき
     データも無い。
3. **アカウントは、ホストで実行する `mdm account` の下位コマンドが DB に書く。**
   ([contracts/account-cli.md](contracts/account-cli.md))。未設定の間、サーバーは起動して
   未設定の案内だけを出す（受け入れ条件 1）。
   - 却下: ユーザー名とパスワードのハッシュを環境変数で渡す案。変えるたびに Compose の
     ファイルを編集して再起動する必要があり、PHC 文字列の `$` を Compose 用に `$$` と
     書く必要もある。パスワードを忘れたときの手順が重い。
   - 却下: 平文のパスワードを環境変数やファイルで渡す案。`docker inspect` や設定ファイルに
     平文が残る（要件 4 の趣旨）。
   - 却下: 未設定のサーバーで最初にブラウザから設定した人に決めさせる案。要件 2 が
     明示的に禁じている。
4. **セッションは ID の SHA-256 を SQLite に置き、要求ごとに DB で確かめる。無効化は
   `account.version` の一致で決める**（[data-model.md §3](data-model.md#3-セッションが有効である条件)）。
   メモリには持たない。
   - 却下: 署名付きの Cookie に状態を入れる案。要件 5 はサーバー側の状態と期限の管理を
     求め、ログアウトで直ちに無効にできない。
   - 却下: セッションをメモリだけに置く案。再起動で消え、受け入れ条件 6 を満たさない。
   - 却下: DB の結果をメモリに一時的に覚える案。別のプロセスのコマンドによる再設定が、
     覚えている間は効かない。1回の問い合わせは主キーの引き当てで、サムネイルが並ぶ
     一覧でも足りる。
   - 却下: セッション ID をそのまま保存する案。DB を読まれると、そのまま使える。
5. **失効したセッションの処理中の応答を打ち切る。** 境界は認証した要求の context に
   セッションの期限を締め切りとして付け、同じセッションの処理中の要求をメモリの台帳に
   載せる。ログアウトは台帳から該当の要求を打ち切る。別のプロセスのコマンドによる
   再設定は、30 秒を超えて続いている要求だけを 30 秒ごとに確かめ直して見つける。
   再設定から打ち切りまでは最長で約 30 秒かかり、その間の配信を許すのはこの経路だけで
   ある（再設定の後に始まる要求はすべて拒む）。間隔と時計はテストで差し替えられるようにする。
   打ち切るときは context を取り消し、`http.ResponseController` で書き込みの締め切りを
   今にする（Edge Case「一覧表示中、動画再生中、ライブ変換中にセッションが失効」）。
   - 却下: context の取り消しだけにする案。`http.ServeContent` は context を見ずに
     書き続けるので、長い Range 応答が止まらない。
   - 却下: すべての要求を定期的に確かめ直す案。ほとんどの要求は数秒で終わるので、
     長く続くもの（`/api/events`・ライブ変換・大きな Range 応答）だけで足りる。
6. **ログインの試行制限は、`internal/app` のメモリ上で送信元ごとに数える。** 直近 5 分の
   失敗が 5 回に達した送信元は、最も古い失敗が 5 分を過ぎるまで 429 にする。照合の前に、
   制限の錠の中で1回分を予約し、「照合中の予約 + 記録した失敗」が 5 以上なら照合せずに
   429 にする。失敗で予約を失敗の記録に変え、成功で予約と記録を消す。こうして同時に
   送られた要求でも、1つの送信元の照合は 5 分に 5 回を超えない。429 は数えない。IPv6 は
   /64 を1つの送信元とする。記録は古いものから捨て、送信元の数に上限を置く（要件 11）。
   Argon2id の照合は同時に2つまでにし、空きを 5 秒待っても得られない要求は、失敗に
   数えずに `Retry-After` 付きの 429 にする（待ち行列を際限なく伸ばさない）。
   - 却下: 失敗を記録した後で数えるだけにする案。同時に送った要求はどれも「まだ 5 回
     未満」を通って照合まで進み、制限を回避できる。
   - 却下: DB に記録する案。再起動で消えても、再起動できるのはホストの管理者だけで、
     攻撃者は制限を解けない。ログインのたびの書き込みが増えるだけである。
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
     同一オリジンの確認は `Host` と比べるので、`Host` を渡す前提で足りる。
8. **セッション Cookie の名前を、HTTPS では `__Host-vv_session`、HTTP では `vv_session` に
   分ける**（[contracts/auth-api.md §6](contracts/auth-api.md#6-セッション-cookie)）。
   - 却下: 1つの名前で、HTTPS のときだけ `Secure` を付ける案。同じホストを HTTP と
     HTTPS の両方で使うと、HTTP でのログインの `Set-Cookie` がブラウザに捨てられ、
     ログインしてもログイン画面に戻る。
9. **Argon2id は新しいアダプタ `internal/password` に置き、PHC 文字列で保存する。**
   パラメータは m=19456 KiB、t=2、p=1、ソルト 16 バイト、鍵 32 バイトとする。
   照合は文字列に書かれたパラメータで行うので、後で強めても既存のハッシュは読める。
   - 却下: m=64 MiB・t=3（RFC 9106 の第2推奨）。NAS や小型機で、同時の照合が
     メモリを圧迫する。19 MiB・t=2 は OWASP の最低推奨を満たす。
   - 却下: `internal/domain` に置く案。ソルトの乱数と外部の暗号実装は domain の
     「純粋な規則」に入らない。`internal/app` に置くと、単体テストが本物の Argon2id の
     時間を払う。
10. **ログイン後の戻り先は、サーバーが `domain` の規則で確かめて `redirectTo` として返す。**
    ログインの応答と、認証済みのときの `GET /api/auth/session?next=…` の応答が返し、
    画面はその値へ遷移するだけにする（[contracts/auth-api.md §2](contracts/auth-api.md#2-post-apiauthlogin)・
    [§3](contracts/auth-api.md#3-get-apiauthsessionpost-apiauthlogout)）。
    - 却下: 画面で `next` を確かめる案。オープンリダイレクトの判定が TypeScript と Go の
      2か所に分かれる。

## Project Structure

### Documentation (this feature)

```text
specs/016-single-account-auth/
├── plan.md
├── data-model.md
└── contracts/
    ├── auth-api.md
    └── account-cli.md
```

`ui-design.md` は、この Plan のあとの design 工程で足す。判断は上の Structural Decisions で
閉じており、別に調べて決めることが無いので `research.md` は作らない。検証は各実装単位の
自動テストと e2e で行い、それ以外に手で走らせる手順が無いので `quickstart.md` も作らない。
HTTPS で公開するための手順は、実装で運用文書（`docs/how-to/running-vv.md`）に書く。

### Source Code

**Affected boundaries**:

- `internal/domain/`: ユーザー名の規則、セッションの寿命、戻り先の判定、認証の誤り
- `internal/password/`（新規）: Argon2id のハッシュ化と照合、PHC 文字列
- `internal/store/`: マイグレーション、新しい役割の型 `AuthStore`
- `internal/app/`: ログイン・試行制限・セッションの確認と失効（`Auth`）
- `api/openapi.yaml` と生成物: 認証の経路、`security`、誤りの種別
- `internal/httpapi/`: 認証の境界、Cookie、送信元と HTTPS の判定、処理中の応答の打ち切り、
  同一オリジンとループバックの確認の判定元の差し替え
- `cmd/mdm/`: `account` の下位コマンド、`MDM_TRUSTED_PROXIES`、組み立て、起動時の期限切れの掃除
- `web/src/api/`: ログイン・ログアウト・セッションの確認、401 での遷移、`/api/events` の再接続の停止
- `web/src/auth/`（新規）: ゲート、ログイン画面、未設定の案内
- `web/src/app/App.tsx`・`web/src/shell/`・`web/src/player/`: ルートの組み替え、ログアウト、再生失敗時のセッション確認
- `web/e2e/`: e2e のアカウント作成とログイン
- `.golangci.yml`・`ARCHITECTURE.md`・`README.md`・`docs/how-to/running-vv.md`・
  `docs/design-docs/tech-stack-selection.md`・`compose.yaml`

**New paths**: `internal/domain/auth.go`・`internal/password/`・`internal/store/auth.go`・
`internal/store/migrations/00009_auth.sql`・`internal/app/auth.go`・`internal/httpapi/auth.go`・
`internal/httpapi/client_origin.go`・`cmd/mdm/account.go`・`web/src/api/auth.ts`・`web/src/auth/`・
`web/e2e/auth.e2e.ts`（それぞれ対応する test を含む）

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。
新しいパッケージは `internal/password` だけで、理由は Structural Decisions 9 のとおり。

## Implementation Work

### パスワードを Argon2id でハッシュ化し、ユーザー名と戻り先の規則を domain に置く

**Scope**: `internal/password` に Argon2id のハッシュ化と照合（PHC 文字列）を置き、
`.golangci.yml` の兄弟パッケージの規則と `ARCHITECTURE.md` の依存方向に足す
（Structural Decisions 9）。`internal/domain/auth.go` に、ユーザー名とパスワードの値の規則
（[data-model.md §5](data-model.md#5-ユーザー名とパスワードの値)）、セッションの寿命 90 日、
戻り先の判定（[contracts/auth-api.md §2](contracts/auth-api.md#2-post-apiauthlogin)）、認証の誤りを置く。
まだどこからも呼ばない。

**Dependencies**: None

**Acceptance**: `go test ./internal/password/... ./internal/domain/...` で次が通る。同じ
パスワードを2回ハッシュ化すると別の文字列になり、どちらも照合に通る。1文字違いと
大文字小文字違いは通らない。別のパラメータで作った PHC 文字列も照合できる。壊れた
PHC 文字列は誤りになる。戻り先は `/videos/1?t=2` を保ち、`//evil.example`・`/\evil`・
`/\t/evil.example`・`/\n/evil.example`・`/a\\b`・`https://evil.example/`・`/login`・`/api/videos`・空を
`/` にする。ユーザー名の規則は空・
制御文字・前後の空白・129 文字を拒む。`task check` が通る。

### 単一アカウントとログインセッションを SQLite に保存する

**Scope**: マイグレーション `00009_auth.sql` と役割の型 `AuthStore` を足す
（[data-model.md](data-model.md) の §1・§3・§4）。ユーザー名とパスワードの設定、アカウントの読み出し、
セッションの追加・有効性の確認・削除・期限切れの掃除を持つ。`ARCHITECTURE.md` の
役割の型の一覧とデータの区分を更新する。

**Dependencies**: パスワードを Argon2id でハッシュ化し、ユーザー名と戻り先の規則を domain に置く

**Acceptance**: `internal/store` のテストで次が通る。ユーザー名だけ、パスワードだけの
状態は未設定として読める。資格情報を書き換えると既存のセッションが無効になり、
書き換え前に読んだ版で足したセッションも無効になる。期限を過ぎたセッションは無効で、
確かめたときと掃除で行が消える。ユーザー名は大文字小文字を区別して保存される。
Down で2つの表が消える。`scripts/migrations-immutable.sh` と `task check` が通る。

### サーバー管理者がホストのコマンドでユーザー名とパスワードを設定できるようにする

**Scope**: `cmd/mdm/account.go` に `mdm account set-username`・`set-password` を足す
（[contracts/account-cli.md](contracts/account-cli.md)）。引数なしの起動は変えない。
`golang.org/x/term` を足す。`docs/how-to/running-vv.md` に初期設定と再設定の手順を書く。

**Dependencies**: 単一アカウントとログインセッションを SQLite に保存する

**Acceptance**: `cmd/mdm` のテストで次が通る。一時的なデータディレクトリに対して
2つのコマンドを実行すると、アカウントが設定済みとして読める。標準入力から渡した
パスワードが照合に通り、DB と標準出力・標準エラーに平文が現れない。設定し直すと
既存のセッションが無効になる。空のユーザー名、未知の下位コマンドは終了コード 2 で
何も書かない。`task check` が通る。

### ログインの照合・試行制限・セッションの発行と失効をアプリケーション層に置く

**Scope**: `internal/app/auth.go` に `Auth` を置く。ログイン（常に1回の照合、定数時間の
ユーザー名比較、未設定時のダミーの照合、照合の同時実行の上限）、送信元ごとの試行制限
（Structural Decisions 6）、セッションの発行・確認・ログアウト、未設定かどうかの状態を持つ。
保存・ハッシュ・時計・乱数は自身の interface 越しに使う。

**Dependencies**: 単一アカウントとログインセッションを SQLite に保存する

**Acceptance**: `go test ./internal/app/...` で次が通る。誤ったユーザー名、誤った
パスワード、両方誤り、空欄、未設定は同じ誤りになり、どれも照合を1回呼ぶ。大文字小文字
だけ違うユーザー名は失敗する。同じ送信元の5回の失敗のあとは、ユーザー名を変えても
照合を呼ばずに制限の誤りになり、時計を 5 分進めると正しい資格情報で成功する。同じ
送信元から誤った資格情報で 20 件を同時に送っても、照合は 5 回までしか呼ばれない。照合の
空きを待ち切れない要求は、失敗に数えずに制限の誤りになる。別の
IPv6 アドレスでも同じ /64 なら制限が掛かる。発行した ID は32バイトで、保存されるのは
そのハッシュだけである。有効なセッションの Cookie を持ったままログインすると、古い
セッションは消える。`task check` が通る。

### すべての API と動画配信を既定で認証必須にし、ログイン API を公開する

**Scope**: `api/openapi.yaml` に [contracts/auth-api.md](contracts/auth-api.md) の §1〜§6 の差分を
足し、`task generate` で生成する。`internal/httpapi/auth.go` に既定拒否の境界
（Structural Decisions 1）、ログイン・ログアウト・セッションの経路、Cookie、処理中の応答の
打ち切り（Structural Decisions 5）、認証の記録（§8）を置く。この単位では接続元のアドレスと
`r.TLS` をそのまま送信元と HTTPS の判定に使う。`/api/auth/login` を `requiresJSONBody` に足す。
境界は `path.Clean` した経路で判定する。処理中の応答を確かめ直す間隔と時計は差し替え
られるようにする。未設定のまま起動したときは、設定のコマンドを示す警告を1行記録する。`cmd/mdm` で組み立て、起動時に期限切れの
セッションを消す。e2e は `run-e2e.mjs` がコマンドでアカウントを作り、Playwright の
セットアップが `POST /api/auth/login` でログインした状態を全テストに渡す。e2e の失敗の
試行は、同じ送信元の試行制限に掛からない回数に収める。
`ARCHITECTURE.md` に認証の境界を書き、「Not built yet」から認証を外す。

**Dependencies**: サーバー管理者がホストのコマンドでユーザー名とパスワードを設定できるようにする。
ログインの照合・試行制限・セッションの発行と失効をアプリケーション層に置く

**Acceptance**: `internal/httpapi` のテストで次が通る。Cookie なしで一覧・設定・スキャン・
Range 再生・サムネイル・シークプレビュー・ライブ変換・再生位置・`/api/events`・定義の無い
`/api/x` を要求すると、401 `unauthenticated` の JSON が返り、動画データも HTML も返らない。
`//api/videos`・`/./api/videos`・`/%61pi/videos`・`/api/../api/videos` も保護対象を返さない。
アカウントが未設定なら、残っていた Cookie でも保護対象は 401 で、`GET /api/auth/session` は
`setupRequired` を返す。
ログインした Cookie ではそれらが既存どおり返る。ログアウト後、パスワードの再設定後、
ユーザー名の変更後は、同じ Cookie が 401 になり、処理中の `/api/events` と Range 応答が
終わる。HTTPS で `__Host-vv_session` と `vv_session` を両方付けてログアウトすると、
どちらのセッションも 401 になる。同じデータディレクトリで組み立て直したサーバーでも、期限内の Cookie は通る。
90 日を過ぎたセッションは 401 になる。セッションの確認で DB が失敗すると 500 になり、
保護対象を返さない。3通りの誤りのログインが同じ状態・本文になる。同じ送信元の
6 回目の誤りは 429 `login_throttled` と `Retry-After` になる。HTTP のログインの
`Set-Cookie` は `vv_session` に `HttpOnly`・`SameSite=Strict`・`Path=/`・`Max-Age` を持つ。
JSON でない本文のログインは 400 になる。別オリジンの
`Origin` の `POST` は 403 になる。`GET /api/health` は未認証で既存の形を返す。
`security: []` の操作と許可リストが一致する。記録にパスワードとセッション ID が出ない。
既存の e2e を含む `task check` と `task test-e2e` が通る。

### 信頼するリバースプロキシの転送ヘッダーから送信元と HTTPS を判定する

**Scope**: `MDM_TRUSTED_PROXIES` を `cmd/mdm/config.go` に足し、`internal/httpapi/client_origin.go`
で送信元と HTTPS を判定する（Structural Decisions 7）。その判定を、試行制限、Cookie の名前と
`Secure`、同一オリジンの確認（[contracts/auth-api.md §7](contracts/auth-api.md#7-同一オリジンの確認)）、
既定アプリで開く操作のループバックの確認、記録に使う。`docs/how-to/running-vv.md` に
HTTPS の逆プロキシで公開するための要件（HTTPS 必須、`Host` を渡す、転送ヘッダー、
`MDM_TRUSTED_PROXIES`）と Caddy の設定例を書く。プロキシを `MDM_TRUSTED_PROXIES` に
書き忘れると、全員がプロキシのアドレスとして試行制限を共有し、HTTPS の `Origin` の
`POST` が 403 になることも書く。`compose.yaml` で環境変数を渡せるようにする。

**Dependencies**: すべての API と動画配信を既定で認証必須にし、ログイン API を公開する

**Acceptance**: `internal/httpapi` と `cmd/mdm` のテストで次が通る。信頼しない接続元が
付けた `X-Forwarded-For` と `X-Forwarded-Proto: https` は無視され、試行制限は接続元の
アドレスで掛かり、Cookie は `vv_session` で `Secure` が付かない。信頼する接続元からの
`X-Forwarded-Proto: https` では `__Host-vv_session` に `Secure` が付き、`Origin: https://<Host>` の
`POST` が通る。HTTP の要求は `__Host-vv_session` を読まない。信頼するプロキシが同じ PC に
あっても、転送元が外部なら既定アプリで開く操作は 403 になる。不正な CIDR は起動時に
他の設定の誤りと一緒に報告される。`task check` が通る。

### ログイン画面と未設定の案内を表示し、認証の確認前にシェルを描かない

**Scope**: `web/src/api/auth.ts` にセッションの確認・ログイン・ログアウトを置く。
`web/src/auth/` に、確認が済むまで何も描かないゲート、ログイン画面（ユーザー名と
パスワードの欄、HTTP の警告、失敗と試行制限の表示）、未設定の案内を置く。
`App.tsx` で `/login` をプロバイダの外に出し、他の経路をゲートの内側に入れる
（Structural Decisions 2）。成功したら `redirectTo` へページごと遷移する。
配置・文言・見た目は `ui-design.md` による。

**Dependencies**: すべての API と動画配信を既定で認証必須にし、ログイン API を公開する。
design 工程の `ui-design.md` が feature ブランチに入っていること

**Acceptance**: Vitest で、確認中はシェルも一覧も描かれず、`unauthenticated` で
`/login?next=…` へ、`setupRequired` で案内へ、`authenticated` で `/login` からサーバーが
返した `redirectTo` へ移ることが確かめられる。セッションの確認が 500 や通信の失敗の
ときは、シェルを描かず、ログイン画面へ遷移せず、確認を自動で繰り返さない。`web/e2e/auth.e2e.ts` で、未認証で `/`・`/settings`・`/videos/{id}` を
直接開くとシェルと動画情報が一度も出ずにログイン画面になり、ログインすると開いた URL に
戻ること、送信中は再送信できないこと、失敗後にパスワード欄だけが空になりユーザー名が
残ること、キーボードだけで
入力から送信まで進められること、HTTP で警告が出ること、欄に `autocomplete` の
`username`・`current-password` が付くことが確かめられる。`localStorage` と
`sessionStorage` にパスワードとセッション ID が無い。`task check` と `task test-e2e` が
通る。画面が変わるので、実装 PR に 360・768・1280 px の画像と、視覚・操作・支援技術の
確認を添える。

### 使用中のセッション失効を検知してログイン画面へ戻し、シェルからログアウトできるようにする

**Scope**: `web/src/api/client.ts` で `401 unauthenticated` を受けたら、1度だけ
`/login?next=<今の場所>` へ置き換える。`web/src/api/serverEvents.ts` は、接続が閉じたら
張り直す前にセッションを確かめ、未認証なら張り直さない。再生画面は、動画・変換の
読み込みの失敗でセッションを確かめる。シェルにログアウトの操作を置く（置き場所は
`ui-design.md`）。`README.md` の「認証が無い」警告を、HTTPS の逆プロキシを必須とする
案内に改め、`docs/design-docs/tech-stack-selection.md` の認証の行を更新する。

**Dependencies**: ログイン画面と未設定の案内を表示し、認証の確認前にシェルを描かない

**Acceptance**: Vitest で、401 を返す取得が何度あっても遷移は1回で、`/api/events` が
閉じた後に未認証なら張り直さないことが確かめられる。`web/e2e/auth.e2e.ts` で、
ログアウトするとログイン画面になり、ログアウト前の Cookie を付け直しても保護対象が
401 になること、別タブは次の操作でログイン画面になること、再生中にサーバー側で
セッションを消すと再生画面がログイン画面へ戻ることが確かめられる。`task check` と
`task test-e2e` が通る。画面が変わるので、実装 PR に画像と、視覚・操作・支援技術の
確認を添える。
