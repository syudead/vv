# Data model: 外部連携 API と MCP

親 Issue: #493。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、
データの区分は [ARCHITECTURE.md](../../ARCHITECTURE.md)「Rebuildable and user data」、`account` と
`sessions` の規則は [016 data-model.md](../016-single-account-auth/data-model.md) にある。ここには、
この feature が足す表と、それを読み書きする規則だけを書く。書いていない表は変えない（`videos` にも
列を足さない。動画の一覧は今の列だけで読む。[R-6](research.md#r-6-動画の一覧は-added_at-id-の-keyset-のカーソルで読む)）。

## 1. `api_tokens`（R-1・R-2・R-9・R-10）

```sql
create table api_tokens (
    id              integer primary key autoincrement,
    -- 用途の名前。domain.NormalizeAPITokenName を通した値。重複してよい。
    name            text    not null,
    -- 平文の SHA-256（16 進）。平文はどこにも保存しない。
    token_hash      text    not null unique,
    -- 発行したときの account.version。一致しない行は無効（R-2）。
    account_version integer not null,
    created_at      integer not null,
    -- 最後に使った時刻（Unix 秒）。未使用なら null。60 秒より細かくは書かない（R-9）。
    last_used_at    integer
);
```

`id` に `autoincrement` を付けるのは、失効した行の id を新しいトークンに再利用しないためである
（画面の失効の要求が、同じ id の別のトークンを消さない）。

区分: 作り直せない設定のデータ（スキャンで戻らず、失えば発行し直す）。ARCHITECTURE.md の一覧に足す。

| 操作 | 1 つのトランザクションで行うこと |
| --- | --- |
| 発行（Cookie の所有者） | 読んだ `account.version` を `account_version` に書いて 1 行足し、平文を返す。`account` の行が無ければ失敗する |
| 一覧 | `id`・`name`・`created_at`・`last_used_at` を `created_at` の降順で返す。版の合わない行は無い（下の変更で消える）が、あれば返さない |
| 失効 | `id` の行を消す。無ければ何もしない |
| 確かめる（Bearer） | 平文の SHA-256 で引き、`account` があり版が一致する行だけを有効とする。問い合わせの失敗は無効とせず誤りにする |
| 使った時刻 | `last_used_at is null or last_used_at <= now - 60` のときだけ `now` を書く |
| ユーザー名・パスワードの変更 | 今の `changeCredentials` に `delete from api_tokens` を足す |

## 2. `internal/domain` に足す値

| 値 | 中身 |
| --- | --- |
| `APIToken` | `ID`・`Name`・`CreatedAt`・`LastUsedAt`（未使用はゼロ値）。平文とハッシュは持たない |
| `NormalizeAPITokenName` と `APITokenNameMaxLength = 100` | R-10 の規則と、その誤りの値（タグ名の `InvalidTagNameError` と同じ形） |
| `VideoRef` | `ID`・`ContentKey`・`Path` のちょうど 1 つ。`Path` は正規化せず、所在の `path` とバイト列で比べる |
| `ExternalVideoQuery`・`ExternalVideoPage` | カーソル・件数と、項目・`NextCursor`（[R-6](research.md#r-6-動画の一覧は-added_at-id-の-keyset-のカーソルで読む)）。並びは `(added_at, id)` の昇順だけで、順序の指定は持たない |
| `VideoTagsAction` | `add`・`remove`・`replace` |
| 引けない動画の誤り | `ErrNotFound` を包み、`VideoRef` の位置（0 から）を持つ |
