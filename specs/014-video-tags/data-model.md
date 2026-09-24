# Data model: 動画のタグ

親 Issue: #193。Plan: [plan.md](plan.md)。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、
「再構築できる索引」と「作り直せない利用者データ」の区別は
[00002_core.sql](../../internal/store/migrations/00002_core.sql) の冒頭と
[ARCHITECTURE.md](../../ARCHITECTURE.md) にある。ここには、この feature が足す表と、
それを書き換える規則だけを書く。既存の表は変えない。

足す3つの表は、すべて「作り直せない利用者データ」に属する。再スキャンやメディア
フォルダの変更で消えてはならないので、`videos` への外部キーを張らず、
`playback_progress` と同じく `content_key` で動画に結び付ける（要件 9）。

## 1. マイグレーション

`00008_tags.sql` として新しく足す。`main` にある最後のマイグレーションは #195 の
`00007_location_search.sql` である。

```sql
-- タグそのもの。名前は tag_names が持つ。
-- autoincrement にするのは、削除したタグの id を別のタグに再利用させないため。
-- 古いタブや URL に残った id が、無関係の新しいタグを指さないようにする。
create table tags (
    id         integer primary key autoincrement,
    created_at integer not null
);

-- タグの名前とシノニムを1つの名前空間で持つ。
-- name の照合は既定の BINARY で、綴りが完全に一致するときだけ同じ名前になる（要件 4）。
create table tag_names (
    name      text    primary key,
    tag_id    integer not null references tags (id) on delete cascade,
    -- 1 は表示する元の名前、0 はシノニム。
    canonical integer not null check (canonical in (0, 1))
) without rowid;

-- 1つのタグに元の名前は1つだけ。
create unique index tag_names_canonical_idx on tag_names (tag_id) where canonical = 1;
create index tag_names_tag_idx on tag_names (tag_id);

-- 動画の中身とタグの対応。利用者データなので videos への外部キーを張らない。
create table video_tags (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    created_at  integer not null,
    primary key (content_key, tag_id)
) without rowid;

-- タグごとの本数と、統合・削除での付け替えに使う。
create index video_tags_tag_idx on video_tags (tag_id, content_key);
```

Down は3つの表を落とす。

不変条件（`internal/store/invariants_test.go` の検査に足す）:

- どの `tags` の行にも、`canonical = 1` の `tag_names` がちょうど1行ある。
- `video_tags.tag_id` と `tag_names.tag_id` は、どれも存在する `tags` を指す（外部キーで
  保証される。`foreign_keys(on)` は既に DSN にある）。

## 2. 名前の規則

`internal/domain` の1つの関数で、入力を名前に整える。

1. 前後の空白（Unicode の White_Space）を取り除く。内側の空白はそのまま残す。
2. 空になったら誤り（`invalid_request`）。
3. 100 符号位置を超えたら誤り（`invalid_request`）。上限を設けないと、1つの名前で
   カードの行・候補・URL の外にある本文を際限なく大きくできる。数は検索語と同じ 100 に
   そろえ、利用者が覚える上限を1つにする。
4. Unicode の正規化、大文字小文字の畳み込みは**しない**。`Anime` と `anime`、全角の
   `Ａ` と半角の `A` は別の名前である（要件 4、受け入れ条件 6）。

## 3. 名前の引き方

名前からタグを引くときは、`tag_names` を `name` で1回引く。元の名前でもシノニムでも
同じタグに着く（要件 7）。画面に出す名前は、常に `canonical = 1` の行である。

## 4. 書き換えの規則

どの操作も1つのトランザクションで行い、途中で失敗したら何も残さない（Edge Case
「一括操作・統合の途中失敗」）。

| 操作 | 書き換え |
| --- | --- |
| 作成 | `tags` に1行、`tag_names` に `canonical = 1` で1行。名前が既にあれば `ErrTagNameTaken` |
| 改名 | そのタグの `canonical = 1` の行の `name` を書き換える。今と同じ名前なら何も変えない。新しい名前が既にあれば（自分のシノニムでも）`ErrTagNameTaken` |
| 削除 | `tags` の行を消す。`tag_names` と `video_tags` は連鎖して消える。いまライブラリに無い動画の付与も消える |
| 統合（元 X → 先 Y） | `insert or ignore into video_tags select content_key, Y, … from video_tags where tag_id = X`、X のシノニムを `tag_id = Y` に付け替える、X を消す。X の元の名前は消える |
| シノニム登録（名前 n をタグ T に） | n が無ければ `canonical = 0` で1行足す。n が既に T のシノニムなら何も変えない。n が T の元の名前なら `ErrTagNameTaken`。n が別のタグ S のシノニムなら `ErrTagNameTaken`（S を伝える）。n が別のタグ S の元の名前なら、承諾された統合元の `id` が S でなければ（無い場合を含む）`ErrTagMergeRequired`、S なら S → T の統合をしてから n を T のシノニムとして足す |
| シノニム解除 | その `canonical = 0` の行を消す。n が T のシノニムでなければ何も変えない |
| 付与 | 対象の動画の `content_key` ごとに `insert or ignore`。既に付いていれば何も変わらない |
| 取り外し | 対象の `content_key` ごとに `delete`。付いていなければ何も変わらない |

- 統合では、両方が付いていた中身は `insert or ignore` で1行に収まる（受け入れ条件 12）。
- シノニム登録に伴う統合でも、S のシノニムは T へ引き継ぐ（Edge Case「統合とシノニム」）。
- 付与・取り外しの対象は、画面が送る動画の `id` を、いまライブラリにある動画の
  `content_key` に引き直したものである。引けない `id`（その間に消えた動画）は飛ばし、
  応答で反映した本数を返す。
- 付与で名前を渡したときは、§3 で引き、無ければ同じトランザクションで作る（要件 1）。

## 5. 本数の数え方

管理画面の本数は、`video_tags` を `videos.content_key` と結び、いまライブラリにある
動画（`registeredVideoCondition`）だけを数える（Edge Case「ファイルが見えなくなった動画」）。
本数 0 のタグも一覧に出す（要件 8）。

## 6. タグでの絞り込み

選んだタグ1つごとに、次の条件を一覧の動画に AND で掛ける（要件 5）。

```sql
exists (select 1 from video_tags vt where vt.content_key = videos.content_key and vt.tag_id = ?)
```

主キー `(content_key, tag_id)` がこの条件の索引になる。存在しない `tag_id` は条件から
落とし、どれを落としたかを一覧の応答で返す（[contracts/tags-api.md §5](contracts/tags-api.md#5-一覧の絞り込みとすべて選択)）。
