# Research: 外部連携 API と MCP

親 Issue: #493。

受け継ぐ技術の決定は [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
と [ARCHITECTURE.md](../../ARCHITECTURE.md) にある（Go の単一バイナリ、SQLite、`api/openapi.yaml` を
正本にした生成、認証の境界 `internal/httpapi/auth.go`、単一アカウントのセッション
[specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md)）。
ここには、この feature が足す決定だけを書く。

## R-1: トークンは接頭辞付きの 256 ビットの乱数にし、SHA-256 だけを保存する

- **Decision**: 平文は `vvt_` と 32 バイトの乱数の base64url（パディング無し、43 文字）をつないだ
  47 文字にする。DB には平文の SHA-256（16 進）だけを置き、要求のたびにそれで引く。形式が合わない
  値は DB を引かずに 401 にする。
- **Rationale**: セッション（`sessions.token_hash`）と同じ作りなので、`internal/store` と
  `internal/app` の既存の関数と試験の形をそのまま使える。256 ビットの乱数は総当たりできないので、
  パスワードのような遅いハッシュは要らない。接頭辞は、利用者が設定ファイルやログの中で vv の
  トークンと見分けるためと、漏洩検査の規則を書けるようにするためである。
- **Alternatives considered**:
  - Argon2id で保存する: 要求のたびに数十ミリ秒かかる。高エントロピーの値には効果が無い。
  - 識別子と秘密の 2 つに分けて識別子で引く: SHA-256 の主キーで 1 回で引けるので、分ける利点が無い。

## R-2: アカウントの変更はトークンの行を消し、版の一致でも確かめる

- **Decision**: `api_tokens` の各行は、発行したときの `account.version` を持つ。有効なのは
  `api_tokens.account_version = account.version` のときだけにする。ユーザー名・パスワードの変更
  （`AuthStore.changeCredentials`）は、`sessions` と同じトランザクションで `api_tokens` も全件消す。
  `mdm account` の出力に「API トークンもすべて失効した」旨を足す。
- **Rationale**: 要件 8。行を消すので、設定ページの一覧からも消え、使えないトークンが一覧に
  残らない。版の一致は、画面での発行（古い版を読んだセッション）とホストのコマンドが競ったときに、
  古い資格情報のもとで作ったトークンが生き残らないためで、セッションの §4 の 3 と同じ理由である。
- **Alternatives considered**: 行を残して「無効」と表示する: 使い道の無い行が一覧に残り、利用者は
  結局作り直す。要件にも無い。

## R-3: 外部連携 API は `/api/v1/` の下に置き、境界に「Bearer」の分類を足す

- **Decision**: 外部連携 API は `/api/v1/…`、MCP は `/mcp`（要件 9 で固定）に置く。
  `classifyRequest` に 4 つ目の分類 `accessBearer` を足し、`path.Clean` した経路が `/api/v1/` の下か
  `/mcp` なら、この分類にする。
  - Bearer の分類では Cookie を読まず、`Authorization: Bearer <token>` だけで所有者を決める。
    無い・形式が違う・無効・失効済みは `401 unauthenticated` と `WWW-Authenticate: Bearer` を返す
    （要件 7、受け入れ条件 7）。
  - それ以外の `/api/*` では、今どおり Cookie だけを読み、`Authorization` は見ない。Bearer だけの
    要求はゲストになる（受け入れ条件 7 の後半）。トークンの管理 API もここにあるので、Bearer では
    扱えない（要件 4、受け入れ条件 8）。
  - `/api/v1/` の下の未定義の経路も Bearer の分類になり、認証の後に JSON の 404 を返す。
  - 符号化した区切り（`%2F` など）の扱いは今の規則のまま、どちらかの形が `/api/` の下なら
    所有者だけに倒す。Bearer の分類は、符号化の前後が一致したときだけ選ぶ。
- **Rationale**: 画面の API と外部連携 API の認証を経路の接頭辞で分けると、1 つの要求がどちらの
  資格情報で扱われるかが経路だけで決まり、今の `accessRoutes` と `openapi_routes_test.go` の作りに
  そのまま載る。`/api/` の下に置けば、SPA の経路の名前空間を取らず、未定義の経路の JSON 404 も
  今の仕組みで返る。
- **Alternatives considered**:
  - 最上位の `/v1/`: SPA の経路（`/api/` の外はすべて SPA）から名前を取り、SPA の後始末と JSON の
    404 を別に足す必要がある。
  - 同じ経路で Cookie と Bearer の両方を受ける: 要件 5 が Cookie での利用を禁じている。画面の API の
    挙動も変わる。

## R-4: Bearer の要求には同一オリジンの検査をかけない

- **Decision**: `mutationBoundary` の同一オリジンの検査（`acceptsSameOrigin`）は Cookie の分類の
  要求だけにかけ、Bearer の分類の要求には `Origin` を問わない。JSON の本文を要求する規則は外部連携 API
  にもかける。
- **Rationale**: 同一オリジンの検査は、ブラウザが自動で送る Cookie を使った CSRF を防ぐためにある。
  `Authorization` はブラウザが自動では付けないので、この攻撃は成り立たない。スクレイパーや MCP
  クライアントは `Origin` を送らないか、vv と違うオリジンを送る。
- **Alternatives considered**: 検査をそのまま当てる: `Origin` を送るクライアント（ブラウザ上の
  ツールなど）が理由無く拒まれる。守るものが無い。

## R-5: 外部連携 API の契約は別の OpenAPI の文書にし、Go だけを生成する

- **Decision**: `api/external-v1.yaml` を外部連携 API の正本にし、
  `api/oapi-codegen-external.yaml` で `internal/httpapi/extgen/` に Go の型とハンドラの口を生成する。
  TypeScript は生成しない。`scripts/generate` がこれも生成し、`task generate-check` が差分を見る。
  公開はこの文書そのもので、`docs/how-to/external-api.md` から案内する。サーバーからは配らない。
  互換の方針: `v1` の中では項目と操作の追加だけを行い、既存の項目の意味・型・必須を変えるときは
  `v2` を足す。
- **Rationale**: 画面の API は画面の都合で変わり（背景）、外部連携 API は変えない約束である。
  1 つの文書に混ぜると、ある変更が外部の約束を破るかが文書を見ても分からない。別の文書なら、
  `api/external-v1.yaml` の差分が約束の変更そのものになる。画面は外部連携 API を呼ばないので
  TypeScript の型は要らない。
- **Alternatives considered**:
  - `api/openapi.yaml` に足してタグで分ける: 上の理由に加え、画面向けの TypeScript の生成物に外部の
    型が混ざる。
  - サーバーから `GET /api/v1/openapi.yaml` で配る: `api/` をバイナリへ埋め込む例外（`web/embed.go`
    と同じ種類）が要り、要件はリポジトリで公開すれば満たせる。

## R-6: 動画の一覧は、単調に増える列でカーソルを作る

- **Decision**: `videos` に `added_seq` と `changed_seq` を足し、1 つのカウンタ
  （`video_seq`、1 行の表）から取った値を入れる。
  - 動画の行を作るとき、両方に次の値を入れる。
  - 外部連携 API が返す動画の事実が変わったとき（登録フォルダの下の所在の追加・削除・パスの変更、題名、
    長さ）、その動画の `changed_seq` に次の値を入れる。所在が別の動画へ付け替わるときは、所在を失って残る
    動画も変わっている。メディアフォルダの追加・置き換え・削除で所在が登録の下に入る・外れるときも、
    その動画は変わっている（外部連携 API は登録外の所在を返さない）。書く箇所の一覧は
    [data-model.md §2](data-model.md#2-videosadded_seqvideoschanged_seqr-6)。タグ・取り込みの段の状態・
    公開フラグの変更では変えない。
  - 一覧は `order=added|changed` の昇順で読む。カーソルを持たない最初の要求で、その時点の
    `video_seq.value` を上限として固定する。カーソルは「順序・最後に返した値・上限」を符号化した不透明な
    文字列にし、1 回の読み通しは「最後に返した値 < 番号 ≤ 上限」だけを返す。上限まで読み終えたら
    `hasMore` を `false` にし、`nextCursor` には上限を下限にした新しい読み通しの始まり（上限は次の要求で
    固定する）を返す。後でそこから呼ぶと、その後に足された（変わった）動画だけが返る（受け入れ条件 3）。
  - 一覧は登録フォルダの下に所在を持つ動画だけを返す。登録外の所在だけで残った動画は、行が消えた動画と
    同じく返らない。消えたことを知らせる墓標は持たない（Issue は消えた動画の通知を求めていない）。
  - 移行は、既存の行に `(added_at, id)` の順で番号を振る。
- **Rationale**: SQLite の書き込みは 1 本ずつ直列に確定するので、後で確定した取引の値は必ず大きい。
  カーソルより大きい値だけを読めば、ページングの途中で動画が増えても消えても取りこぼさない。
  `changed_seq` は書き換わるので、読み通しの途中で移動した動画は上限より大きい値になる。上限で
  切るので、同じ読み通しの中で二度は返らず（重複しない、Edge Cases）、次の読み通しで新しいパスとして
  一度だけ返る。内容キーで同じ動画と分かる。タグの変更で進めないのは、スクレイパーが自分で付けたタグの変更を
  次の読み込みでもう一度受け取らないためである。
- **Alternatives considered**:
  - `(added_at, id)` のキーセット: `videos.id` は `autoincrement` の無い `integer primary key` なので、
    最大の id の行を消した後に同じ id が再利用され、カーソルより前の値になって取りこぼす。
    `added_at` は秒の精度で時計にも依る。
  - `videos.updated_at` を「更新日時」に使う: 取り込みの段が進むたびに変わる一方で、移動では
    変わらない（`scan_index.go` は所在の行と `location_generation` だけを変える）。
  - 上限を持たないカーソル: 読み通しの途中で移動した動画が、同じ読み通しの後のページでもう一度返る。
  - 不変の変更履歴の表（1 変更 1 行）: 重複は防げるが、行が増え続け、片付けの規則が要る。上限で
    足りる。
  - タグの変更でも `changed_seq` を進める: 上の理由で、利用者が自分の書き込みを読み直すことになる。

## R-7: タグの操作は厳格な一括操作として `TagStore` に足す

- **Decision**: `TagStore.ApplyVideoTags(ctx, videos []domain.VideoRef, action, names)` を足す。
  1 つのトランザクションで、動画の指定（id・内容キー・所在のパスのいずれか）を今ライブラリにある動画へ
  引き当て、1 つでも引けなければ動画が無い誤り（`domain.ErrNotFound` を包み、何番目の指定かを持つ値）で全体を失敗させる。
  - `add`: 名前をシノニムを含めて引き、無ければ作る（`findOrCreateTag`、画面の付与と同じ）。
  - `remove`: 名前をシノニムを含めて引き、あるタグだけを外す。どのタグにも当たらない名前は何もしない。
  - `replace`: 名前ごとに `add` と同じく引く・作り、各動画の手で付けたタグ（`video_tags` の行）を
    ちょうどその集合にする。空の集合は手で付けたタグをすべて外す。
  - 3 つとも書き換えるのは手で付けたタグだけで、祖先のフォルダ名から付くタグ
    （[017 data-model.md §4](../017-folder-groups/data-model.md#4-フォルダ由来のタグ)）は変えない。
    画面の取り外し（`DetachTag`）と同じ規則である。応答のタグは出所（`manual`・`fromFolder`、
    `domain.VideoTag`）を持つので、利用者はフォルダ由来で残ったタグを見分けられる。
  - 同じタグに当たる名前は 1 つにまとめる。結果として、各動画の操作後のタグを返す。
  - 名前の検証は `domain.NormalizeTagName`、動画の件数の上限は画面と同じ `maxVideoTagsIDs`
    （20000）。名前の件数の上限は 100 とする。
  `internal/app` は通さない。
- **Rationale**: 要件 6.4・10。今の付与（`AttachTagByName`）は 1 つのトランザクションで済むので
  `internal/app` を通していない（`httpapi.Tags` のコメント）。同じ理由でここも store に置く。画面の
  付与は消えた動画を飛ばすが、外部連携 API は Edge Case が要求全体の失敗を求めるので、引き当ての規則は
  別にする。同じ付与・除去の繰り返しは状態を変えずに成功する（受け入れ条件 5）。付与と除去は
  `(content_key, tag_id)` の行ごとの挿入・削除なので、画面と同時に付けても両方が残る。
- **Alternatives considered**:
  - `replace` でフォルダ由来のタグも消す: 付き方がフォルダ名から毎回導かれるので、動画ごとに打ち消す
    新しい仕組み（除外の表）が要る。親 Issue にその要求は無く、画面にも無い。
  - 画面の `POST /api/video-tags` と同じく 1 回に 1 タグ: スクレイパーは 1 本の動画に複数のタグを
    付けるので要求が増え、複数のタグの間でトランザクションが分かれる。
  - `internal/app` に新しい use case を置く: 副作用もイベントも無く、1 トランザクションで閉じる。
    今のタグの操作の置き場所と揃わなくなる。

## R-8: MCP は公式の Go SDK を stateless で `internal/httpapi` の中に置く

- **Decision**: `github.com/modelcontextprotocol/go-sdk`（v1.8.0 時点）の
  `mcp.NewStreamableHTTPHandler` を `Stateless: true`・`JSONResponse: true` で使い、`/mcp` に載せる。
  - ハンドラは `internal/httpapi` の中に置き、境界（R-3 の Bearer の分類）の内側に入れる。
  - ツールは外部連携 API の操作と 1 対 1 にし、入力と出力は `api/external-v1.yaml` と同じ JSON にする。
    実装は REST のハンドラと同じ関数を呼ぶ。
- **Rationale**: 要件 9。stateless ならサーバーがクライアントごとの会話の状態を持たず、要求ごとに
  Bearer を確かめるので、失効したトークンの会話が残らない。ツールは要求と応答だけで、サーバーから
  始める通知は要らない。`internal/httpapi` に置けば、動画の応答への変換と認証の境界を REST と共有
  でき、兄弟のパッケージ同士の import の規則（depguard）にも触れない。
- **Alternatives considered**:
  - `github.com/mark3labs/mcp-go`: 利用者は多いが、公式の SDK が v1 の互換を約束して保守されている。
  - JSON-RPC を自前で書く: 版の交渉や Streamable HTTP の細部を持ち続けることになる。
  - stateful（セッション ID と GET のストリーム）: サーバーが会話の状態を持ち、トークンの失効と別に
    寿命を管理する必要がある。
  - `internal/mcpapi` という別のパッケージ: 動画の応答への変換を重複して持ち、境界の配線も別に要る。

## R-9: 最終使用日時は 1 分に 1 回だけ書く。失効は実行中の要求も止める

- **Decision**:
  - 有効なトークンの要求で、保存した `last_used_at` が空か 60 秒以上前なら、その場で書き換える
    （条件付きの `update`）。書き込みの失敗はログに残し、要求は続ける。
  - 境界の `sessionLedger` に Bearer の要求も載せる。画面で失効したら、そのトークンの実行中の応答を
    すぐに打ち切る。ホストのコマンドによる失効は、今と同じ 30 秒ごとの再確認で打ち切る。
- **Rationale**: Edge Cases（最終使用日時は間引いてよい、長く続く要求は失効で打ち切る）。
  条件付きの `update` なら、1 分に何回呼ばれても書くのは 1 回で、メモリに溜めて後で書く仕組みが要らない。
  ログアウトとホストのコマンドでセッションの要求を止める今の仕組みがそのまま使える。
- **Alternatives considered**: メモリに溜めて定期的に書く: 停止時に失われ、書き出しの goroutine と
  停止の順序が増える。

## R-10: トークンの名前の規則はタグ名と同じ形にする

- **Decision**: 名前は前後の空白を除いて 1〜100 文字（Unicode のコードポイント）で、制御文字を
  含まない。`domain.NormalizeAPITokenName` に置き、誤りは画面の API の `invalid_request` に新しい
  `reason`（`api_token_name_empty`・`api_token_name_control_characters`・`api_token_name_too_long`、
  上限は `limit`）で返す。同じ名前は重複してよい。
- **Rationale**: Edge Cases（空・長すぎる名前は発行しない、同じ名前は複数作れる）。画面の
  エラーの作り（[023 error-api.md](../023-english-i18n/contracts/error-api.md)）に載せると、画面は
  `reason` から文言を作れる。
- **Alternatives considered**: タグ名の `reason` を流用する: 画面の文言が「タグ名」になり、
  利用者に誤った対象を示す。
