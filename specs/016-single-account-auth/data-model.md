# Data model: 単一アカウント認証

親 Issue: #135。Plan: [plan.md](plan.md)。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、
「再構築できる索引」と「作り直せない利用者データ」の区別は
[ARCHITECTURE.md](../../ARCHITECTURE.md) にある。ここには、この feature が足す2つの表と、
それを読み書きする規則だけを書く。既存の表は変えない。

## 1. マイグレーション

`00009_auth.sql` として新しく足す。`main` にある最後のマイグレーションは
`00008_tags.sql` である。

```sql
-- 管理者が設定した唯一のアカウント。行は多くても1つ（id = 1）。
-- 行が無い、または username か password_hash が null なら「未設定」である（要件 2）。
create table account (
    id            integer primary key check (id = 1),
    -- 照合は既定の BINARY。大文字と小文字を区別した完全一致になる（要件 3）。
    username      text,
    -- Argon2id の PHC 文字列（$argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>）。平文は持たない（要件 4）。
    password_hash text,
    -- username か password_hash を書き換えるたびに 1 増やす（§3）。
    version       integer not null default 1 check (version >= 1),
    updated_at    integer not null
);

-- ログインで発行したセッション。
create table sessions (
    -- セッション ID の SHA-256（16進）。ID そのものは Cookie にだけあり、DB には無い。
    token_hash      text    primary key,
    -- 発行したときの account.version。一致しない行は無効である（§3）。
    account_version integer not null,
    created_at      integer not null,
    -- created_at + 90 日。延長しない（要件 6）。
    expires_at      integer not null
) without rowid;

create index sessions_expires_at on sessions (expires_at);
```

Down は2つの表を落とす。

## 2. データの区分

`account` は作り直せない利用者データに属する。消すとホストのコマンドで設定し直すまで
vv は使えない。`sessions` はどちらにも属さない一時的な状態で、消えても再ログインで
戻る。ARCHITECTURE.md の区分の段落に `account` を足し、`sessions` はそのどちらでも
ないと書く。

## 3. セッションが有効である条件

セッションは、次のすべてを満たすときだけ有効である。1回の問い合わせで確かめる。

1. Cookie の値の SHA-256 が `sessions.token_hash` にある。
2. `account` の行があり、`username` と `password_hash` がどちらも null でない。
3. `sessions.account_version = account.version`。
4. `expires_at` が今より後。

3 を置くのは、ログインの照合（Argon2id、数十ミリ秒）と行の追加の間に、別のプロセスの
コマンドが資格情報を再設定しても、古い資格情報で照合したセッションが生き残らない
ようにするためである。ログインは、照合に使った `account.version` を読んだ値のまま
`account_version` に書く。

問い合わせが失敗したときは、無効として扱わず誤りとして返す。HTTP 側はこれを
500 にする（[contracts/auth-api.md §4](contracts/auth-api.md#4-未認証とその他の応答)）。

## 4. 書き換えの規則

| 操作 | 1つの取引で行うこと |
| --- | --- |
| ユーザー名を設定する | 行が無ければ作る。`username` を書き、`version` を 1 増やし、`sessions` を全件消す（要件 8） |
| パスワードを設定する | 行が無ければ作る。`password_hash` を書き、`version` を 1 増やし、`sessions` を全件消す（要件 8） |
| ログインに成功する | `expires_at <= now` の行を消し、新しい行を足す。要求が有効なセッションの Cookie を持っていれば、その行も消す（二重送信で行が増えないように） |
| ログアウトする | Cookie の値に当たる行を消す。無ければ何もしない |
| 有効性を確かめる（§3） | 読むだけ。期限切れの行に当たったら、その行を消す |
| 起動する | `expires_at <= now` の行を消す |

`sessions` の全件削除は後片付けで、無効化そのものは §3 の 3 が担う。

## 5. ユーザー名とパスワードの値

- ユーザー名: 1〜128 文字。制御文字を含まず、先頭と末尾に空白を置かない。正規化や
  大文字小文字の畳み込みはしない。この規則は `internal/domain` に置き、設定のときだけ
  当てる。ログインでは、送られた値をそのまま保存値とバイト列で比べる。
- パスワード: 1〜1024 バイト。強度の規則は親 Issue に無いので置かない。

どちらもログインの要求で空や上限超えだったときは、形式の誤りにせず、他と同じ
認証失敗として扱う（[contracts/auth-api.md §2](contracts/auth-api.md#2-post-apiauthlogin)）。
