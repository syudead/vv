---
source: specs/016-single-account-auth/data-model.md
sourceHash: 660e88c6d4d9996f06b3e99356caac8918143b6f1428976b25e86707ff6b2f99
---

# データモデル: 単一アカウントの認証と公開フラグ {#data-model-single-account-authentication-and-the-public-flag}

親 Issue: #135。

既存のテーブルは [internal/store/migrations/](../../internal/store/migrations/) で定義している。作り直せる索引と作り直せない利用者のデータの区別は [ARCHITECTURE.md](../../ARCHITECTURE.md) にある。この文書は、この機能が加える 3 つのテーブルと、それを読み書きする規則だけを扱う。既存のテーブルは変わらない。

## マイグレーション {#migration}

マイグレーションを 2 つ加える。`main` の最後のマイグレーションは `00009_display_aspect_ratio.sql` である。

`00010_auth.sql`:

```sql
-- The only account. No row means "not configured", a row means "configured" (requirement 2).
-- Only first-time setup creates the row, and it is never deleted.
create table account (
    id            integer primary key check (id = 1),
    -- Default BINARY collation: an exact, case-sensitive match (requirement 4).
    username      text    not null,
    -- Argon2id PHC string ($argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>). No plain text is kept (requirement 5).
    password_hash text    not null,
    -- Incremented on every username or password change (see "Session validity conditions").
    version       integer not null default 1 check (version >= 1),
    updated_at    integer not null
);

-- Sessions issued by login and first-time setup.
create table sessions (
    -- SHA-256 (hex) of the session ID. The ID itself exists only in the cookie, not in the DB.
    token_hash      text    primary key,
    -- account.version at issue time. A row that does not match is invalid (see "Session validity conditions").
    account_version integer not null,
    created_at      integer not null,
    -- created_at + 90 days. Not extended (requirement 7).
    expires_at      integer not null
) without rowid;

create index sessions_expires_at on sessions (expires_at);
```

`00011_public_videos.sql`:

```sql
-- Public flag. A row means public, no row means private (the default) (requirement 15).
-- Keyed by the content identifier rather than the video row, so rescans, moves,
-- renames and media folder changes do not lose it. Like playback_progress and
-- video_tags, there is no foreign key to videos.
create table public_videos (
    content_key  text    primary key,
    published_at integer not null
) without rowid;
```

Down はそれぞれのテーブルを削除する。

## データの分類 {#data-classification}

| テーブル | 分類 | 失われたとき |
| --- | --- | --- |
| `account` | 作り直せない利用者のデータ | 設定を初回設定からやり直す |
| `public_videos` | 作り直せない利用者のデータ | すべての動画が非公開に戻る |
| `sessions` | 一時的な状態で、どちらの分類でもない | もう一度ログインすれば戻る |

ARCHITECTURE.md の分類の段落に、この 3 つのテーブルを加える。

## 閲覧者と公開された動画の条件 {#audience-and-the-public-video-condition}

要求ごとに、閲覧者は所有者 (有効なセッションを持つ) かゲスト (持たない) のどちらかである。これは `internal/domain` に `Audience` として置き、そのゼロ値はゲストなので、設定し忘れると狭い側に倒れる。空の `content_key` (内容をまだ読んでいない動画) は `public_videos` に決して入れず、ゲストに決して見せない。

ゲストには、次の両方を満たす動画だけが見える:

1. `videos.content_key` が空でなく、`public_videos` にある。
2. 今の一覧と同じく、ある場所が登録したメディアフォルダの下にある。

この条件は、`Audience` を受け取って場所の条件を返す 1 つの関数に置く (たとえば `visibleLocationCondition(alias, audience)`)。所有者には今の「登録したフォルダの下」の条件をそのまま返し、ゲストには 1 を加える。今は各自で条件を組み立てている下のすべての読み取りが、この関数を通る。

| 読み取り | 今の条件の場所 |
| --- | --- |
| `ListVideos`、`ListFolderVideos` | `listing.go` の `locationScope.condition` |
| `GetVideo` | `videos.go` の `registeredVideoCondition` と、`videoColumns` の中のサブクエリ |
| `RelatedCandidates`、`VideosByIDs` | `related.go` |
| `FolderLocations`、`HasFolderLocations` | `folders.go` |

取り込み、ジョブ、タグの数が使う条件の関数 (`videos.go`、`jobs.go`、`tags.go`) は所有者専用のままで、ゲストの条件を受け取らない。

動画、場所、フォルダを返す `LibraryStore` のすべての読み取りは `Audience` の引数を受け取る。フォルダは引き続き場所のパスから導くので、ゲストには公開された動画の場所から導いたフォルダだけが見え、数には公開された動画だけが入る。

公開フラグの読み取りに失敗すると、要求はエラーを返し、動画を公開として扱わない (Edge Case `公開フラグの DB の失敗`)。

## セッションが有効な条件 {#session-validity-conditions}

セッションが有効なのは、次のすべてを満たすときだけで、1 つのクエリで確かめる:

1. クッキーの値の SHA-256 が `sessions.token_hash` にある。
2. `account` の行がある。
3. `sessions.account_version = account.version`。
4. `expires_at` が今より後である。

条件 3 は、ログインの確認 (Argon2id、数十ミリ秒) と行の挿入の間に別のプロセスのコマンドが資格情報を設定し直しても、古い資格情報で確かめたセッションが生き残らないようにするためにある。ログインは、確認のために読んだ `account.version` をそのまま `account_version` に書く。

クエリが失敗すると、結果は「無効」ではなくエラーである。HTTP 側はそれを 500 にする ([contracts/auth-api.md、Unauthenticated and other responses](contracts/auth-api.md#unauthenticated-and-other-responses))。

## 書き込みの規則 {#write-rules}

| 操作 | 1 つのトランザクションがすること |
| --- | --- |
| 初回設定 | `id = 1` の `account` の行と、セッションの行を 1 つ挿入する。行がすでにあれば、主キーの衝突で書かずに失敗する (同時の初回設定は 1 つだけ成功する、要件 2) |
| ユーザー名を変える (ホストのコマンド) | 行がなければ失敗する。`username` を書き、`version` を増やし、`sessions` のすべての行を削除する (要件 9) |
| パスワードを変える (ホストのコマンド) | 行がなければ失敗する。`password_hash` を書き、`version` を増やし、`sessions` のすべての行を削除する (要件 9) |
| ログインに成功する | `expires_at <= now` の行を削除し、新しい行を挿入する。要求が有効なセッションのクッキーを持っていれば、その行も削除する (そのため、もう一度ログインしても古い行は残らない。クッキーのない最初のログインの二重送信は画面が防ぎ、すり抜けた行は期限で切れる) |
| ログアウトする | クッキーの値に合う行を削除する。なければ何もしない |
| 有効か確かめる ([セッションが有効な条件](#session-validity-conditions)) | 判定は読み取りだけでする。期限切れの行に当たったら削除を試みる。削除に失敗しても判定は変わらない |
| 起動する | `expires_at <= now` の行を削除する |
| 公開または非公開にする | 与えられた動画の id を、今ライブラリにある動画の `content_key` に解決する。公開なら `public_videos` に挿入し (あれば何もしない)、非公開なら削除する。すべてに適用するか、何も適用しない (タグの追加と削除と同じ、[014 tags-api.md、Adding and removing tags on videos](../014-video-tags/contracts/tags-api.md#adding-and-removing-tags-on-videos)) |

`sessions` のすべての行を削除するのは後片付けである。無効にすること自体は [セッションが有効な条件](#session-validity-conditions) の条件 3 がする。

## ユーザー名とパスワードの値 {#username-and-password-values}

| 値 | 規則 |
| --- | --- |
| ユーザー名 | 1 から 128 文字。制御文字を含まず、先頭と末尾に空白を置かない。正規化も大文字小文字の畳み込みもしない。規則は `internal/domain` にあり、初回設定とホストのコマンドに適用する。ログインは、送られた値を保存した値とバイト単位で比べる |
| パスワード | 1 から 1024 バイト。親 Issue に強さの規則はないので、設けない |

ログインの要求の空の値や上限を超える値は形式のエラーではなく、他と同じ認証の失敗である ([contracts/auth-api.md、`POST /api/auth/login`](contracts/auth-api.md#post-apiauthlogin))。初回設定では形式のエラーで、何も設定しない。
