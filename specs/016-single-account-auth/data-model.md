# Data model: 単一アカウント認証と公開フラグ

親 Issue: #135。Plan: [plan.md](plan.md)。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、
「再構築できる索引」と「作り直せない利用者データ」の区別は
[ARCHITECTURE.md](../../ARCHITECTURE.md) にある。ここには、この feature が足す3つの表と、
それを読み書きする規則だけを書く。既存の表は変えない。

## 1. マイグレーション

2つ足す。`main` にある最後のマイグレーションは `00009_display_aspect_ratio.sql` である。

`00010_auth.sql`:

```sql
-- 唯一のアカウント。行が無ければ「未設定」、あれば「設定済み」である（要件 2）。
-- 行を作るのは初回設定だけで、作った後は消さない。
create table account (
    id            integer primary key check (id = 1),
    -- 照合は既定の BINARY。大文字と小文字を区別した完全一致になる（要件 4）。
    username      text    not null,
    -- Argon2id の PHC 文字列（$argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>）。平文は持たない（要件 5）。
    password_hash text    not null,
    -- ユーザー名かパスワードを書き換えるたびに 1 増やす（§4）。
    version       integer not null default 1 check (version >= 1),
    updated_at    integer not null
);

-- ログインと初回設定で発行したセッション。
create table sessions (
    -- セッション ID の SHA-256（16進）。ID そのものは Cookie にだけあり、DB には無い。
    token_hash      text    primary key,
    -- 発行したときの account.version。一致しない行は無効である（§4）。
    account_version integer not null,
    created_at      integer not null,
    -- created_at + 90 日。延長しない（要件 7）。
    expires_at      integer not null
) without rowid;

create index sessions_expires_at on sessions (expires_at);
```

`00011_public_videos.sql`:

```sql
-- 公開フラグ。行があれば公開、無ければ非公開（既定）である（要件 15）。
-- 動画の行ではなく内容の識別子に結ぶので、再スキャン・移動・改名・メディアフォルダの
-- 変更で失われない。playback_progress・video_tags と同じく videos への外部キーを張らない。
create table public_videos (
    content_key  text    primary key,
    published_at integer not null
) without rowid;
```

Down はそれぞれの表を落とす。

## 2. データの区分

`account` と `public_videos` は作り直せない利用者データに属する。`account` を消すと
初回設定からやり直しになり、`public_videos` を消すとすべての動画が非公開に戻る。
`sessions` はどちらにも属さない一時的な状態で、消えても再ログインで戻る。
ARCHITECTURE.md の区分の段落にこの3つを書き足す。

## 3. 見る人と、公開の動画の条件

要求ごとに、見る人は「所有者」（有効なセッションを持つ）か「ゲスト」（持たない）の
どちらかになる。`internal/domain` に `Audience` として置き、ゼロ値をゲストにする
（付け忘れたら狭い側に倒れる）。空の `content_key`（まだ内容を読めていない動画）は
`public_videos` に入れず、ゲストにも見せない。

ゲストに見せる動画は、次の両方を満たすものだけである。

1. `videos.content_key` が空でなく、`public_videos` にある。
2. 今の一覧と同じく、登録したメディアフォルダの下に所在がある。

この条件は、`Audience` を受け取って所在の条件を返す1つの関数（例:
`visibleLocationCondition(alias, audience)`）に置く（[plan.md Structural Decisions 11](plan.md#structural-decisions)）。
所有者では今の「登録フォルダの下」の条件をそのまま返し、ゲストでは 1 を足す。
今それぞれの場所で条件を組み立てている次の読み出しを、すべてこの関数に通す。

| 読み出し | 今の条件の場所 |
| --- | --- |
| `ListVideos`・`ListFolderVideos`・`DirectVideoPaths` | `listing.go` の `locationScope.condition` |
| `GetVideo` | `videos.go` の `registeredVideoCondition` と `videoColumns` の副問い合わせ |
| `VideosAddedNear`・`VideosByIDs` | `related.go` |
| `FolderLocations`・`HasFolderLocations` | `folders.go` |

取り込み・ジョブ・タグの本数（`videos.go`・`jobs.go`・`tags.go`）が使う条件の関数は
所有者のものとして残し、ゲストの条件を入れない。

動画・所在・フォルダを返す `LibraryStore` の読み出しは、すべて `Audience` を引数に取る。
フォルダは今どおり所在のパスから導くので、ゲストには公開の動画の所在から導いた
フォルダだけが現れ、件数も公開の動画だけを数える。

公開フラグの読み出しが失敗したときは、その要求を誤りとして返し、公開とみなさない
（Edge Case「公開フラグの DB の失敗」）。

## 4. セッションが有効である条件

セッションは、次のすべてを満たすときだけ有効である。1回の問い合わせで確かめる。

1. Cookie の値の SHA-256 が `sessions.token_hash` にある。
2. `account` の行がある。
3. `sessions.account_version = account.version`。
4. `expires_at` が今より後。

3 を置くのは、ログインの照合（Argon2id、数十ミリ秒）と行の追加の間に、別のプロセスの
コマンドが資格情報を再設定しても、古い資格情報で照合したセッションが生き残らない
ようにするためである。ログインは、照合に使った `account.version` を読んだ値のまま
`account_version` に書く。

問い合わせが失敗したときは、無効として扱わず誤りとして返す。HTTP 側はこれを
500 にする（[contracts/auth-api.md §5](contracts/auth-api.md#5-未認証とその他の応答)）。

## 5. 書き換えの規則

| 操作 | 1つの取引で行うこと |
| --- | --- |
| 初回設定 | `account` の行を `id = 1` で足し、セッションを1行足す。行が既にあれば主キーの衝突で何も書かずに失敗する（同時の初回設定は一方だけが成立する、要件 2） |
| ユーザー名を変える（ホストのコマンド） | 行が無ければ失敗する。`username` を書き、`version` を 1 増やし、`sessions` を全件消す（要件 9） |
| パスワードを変える（ホストのコマンド） | 行が無ければ失敗する。`password_hash` を書き、`version` を 1 増やし、`sessions` を全件消す（要件 9） |
| ログインに成功する | `expires_at <= now` の行を消し、新しい行を足す。要求が有効なセッションの Cookie を持っていれば、その行も消す（ログインし直しで古い行を残さないため。Cookie を持たない最初のログインの二重送信は画面が防ぎ、すり抜けた行は期限で消える） |
| ログアウトする | Cookie の値に当たる行を消す。無ければ何もしない |
| 有効性を確かめる（§4） | 判定は読むだけで決める。期限切れの行に当たったら、その行の削除を試みる。削除の失敗は判定を変えない |
| 起動する | `expires_at <= now` の行を消す |
| 公開・非公開を切り替える | 指定した動画 id を、いまライブラリにある動画の `content_key` へ引き当て、公開なら `public_videos` に足し（既にあれば何もしない）、非公開なら消す。全部に反映するか1つも反映しない（タグの付け外しと同じ、[014 tags-api.md §4](../014-video-tags/contracts/tags-api.md#4-付与と取り外し)） |

`sessions` の全件削除は後片付けで、無効化そのものは §4 の 3 が担う。

## 6. ユーザー名とパスワードの値

- ユーザー名: 1〜128 文字。制御文字を含まず、先頭と末尾に空白を置かない。正規化や
  大文字小文字の畳み込みはしない。この規則は `internal/domain` に置き、初回設定と
  ホストのコマンドで当てる。ログインでは、送られた値をそのまま保存値とバイト列で比べる。
- パスワード: 1〜1024 バイト。強度の規則は親 Issue に無いので置かない。

ログインの要求で空や上限超えだったときは、形式の誤りにせず、他と同じ認証失敗として
扱う（[contracts/auth-api.md §3](contracts/auth-api.md#3-post-apiauthlogin)）。初回設定で
外れたときは、形式の誤りとして設定しない。
