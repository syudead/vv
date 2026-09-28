# Data model: 外部連携 API と MCP

親 Issue: #493。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、
データの区分は [ARCHITECTURE.md](../../ARCHITECTURE.md)「Rebuildable and user data」、`account` と
`sessions` の規則は [016 data-model.md](../016-single-account-auth/data-model.md) にある。ここには、
この feature が足す表と列、それを読み書きする規則だけを書く。書いていない表は変えない。

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

## 2. `videos.added_seq`・`videos.changed_seq`（R-6）

```sql
-- 次に振る値。1 行だけ。
create table video_seq (
    id    integer primary key check (id = 1),
    value integer not null
);

alter table videos add column added_seq   integer not null default 0;
alter table videos add column changed_seq integer not null default 0;
create unique index videos_added_seq_idx   on videos (added_seq);
create unique index videos_changed_seq_idx on videos (changed_seq);
```

移行は、既存の行に `(added_at, id)` の順で 1 から番号を振り、両方の列に同じ値を入れ、
`video_seq.value` を最大値にする。一意の索引は番号を振った後に作る。

番号を取るのは package-private の 1 つの関数（例: `nextVideoSeq(tx)`）だけで、同じトランザクションの
中で `video_seq` を 1 増やして返す。

外部連携 API が返す動画の事実は、登録フォルダの下の所在（[contracts/external-api.md §2](contracts/external-api.md#2-動画)）と
題名・長さである。登録外の所在は返さないので、所在が登録の下に入る・登録の下から外れることも事実の変更である。
番号はどの場合も、事実を変える書き込みと同じトランザクションの中で進める。

| 事実の変更（今の箇所） | 書く列 |
| --- | --- |
| 動画の行を作る（`ScanIndexStore.UpsertVideo` の挿入） | `added_seq` と `changed_seq` |
| 所在を足す・パスや題名を変える（`UpsertVideo` の所在の挿入と更新） | 所在を持つ動画の `changed_seq` |
| 所在が別の動画へ付け替わる（`UpsertVideo` で内容が変わり `oldVideoID != videoID` のとき） | 所在を得た動画と、所在を失って残る前の動画の両方の `changed_seq`。前の動画が孤立して消えるなら、その動画には書かない |
| 所在を消して動画が残る（`DeleteVideoLocations`、メディアフォルダの削除・置き換えの `removeLocationsUnder`） | 残る動画の `changed_seq`。登録の下に所在が残らない動画にも書くが、一覧は登録の下に所在を持つ動画だけを返すので返らない |
| 登録外だった所在が登録の下に入る（`AddMediaFolder`、`ReplaceMediaFolder` の新しいパスの `syncLocationsUnder`） | その所在を持つ動画の `changed_seq`。`added_seq` は変えない（行を作った順のまま） |
| 解析の結果で長さが変わる（`IngestStore` の probe の結果の書き込み） | その動画の `changed_seq` |

タグ・公開フラグ・取り込みの段の状態・再生位置の変更では書かない。動画の行が消えるときは何も残さない
（一覧は消えた動画を返さない）。登録外の所在だけで残った動画も、一覧と `lookup` からは消えた動画と同じに
見える（[contracts/external-api.md §2](contracts/external-api.md#2-動画)）。走査が同じ所在を同じ内容で見つけたとき（`UpsertVideo` の `OutcomeUnchanged`）も
書かないので、フォルダの追加のあとの走査は、フォルダの追加で進めた番号を二重に進めない。

区分: `videos` の一部なので再構築できる索引に属する。データベースを作り直すと番号は振り直しになり、
利用者は先頭から読み直す（`docs/how-to/external-api.md` に書く）。

## 3. `internal/domain` に足す値

| 値 | 中身 |
| --- | --- |
| `APIToken` | `ID`・`Name`・`CreatedAt`・`LastUsedAt`（未使用はゼロ値）。平文とハッシュは持たない |
| `NormalizeAPITokenName` と `APITokenNameMaxLength = 100` | R-10 の規則と、その誤りの値（タグ名の `InvalidTagNameError` と同じ形） |
| `VideoRef` | `ID`・`ContentKey`・`Path` のちょうど 1 つ。`Path` は正規化せず、所在の `path` とバイト列で比べる |
| `VideoChangeOrder` | `added`・`changed` |
| `VideoChangeQuery`・`VideoChangePage` | 順序・カーソル・件数と、項目・`NextCursor`・`HasMore` |
| `VideoTagsAction` | `add`・`remove`・`replace` |
| 引けない動画の誤り | `ErrNotFound` を包み、`VideoRef` の位置（0 から）を持つ |
