# Data model: 一覧とフォルダ画面の検索

親 Issue: #195。Plan: [plan.md](plan.md)。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、
索引と利用者データの区別は [ARCHITECTURE.md](../../ARCHITECTURE.md) にある。ここには
この feature が足す列と表、それを埋める規則だけを書く。`videos`・`media_folders`・
`playback_progress`・`jobs` は変えない。足すものはすべて「再構築できる索引」側に属する。

## 1. `video_locations` に足す列

| 列 | 型 | 意味 |
| --- | --- | --- |
| `search_key` | `text not null default ''` | 所在1件の照合用の文字列。§3 の規則で Go が作る |
| `title_key` | `text not null default ''` | 題名の自然順の並べ替え用の鍵。§4 の規則で Go が作る |
| `search_version` | `integer not null default 0` | `search_key` と `title_key` を作った規則の版。§5 |

鍵を所在ごとに持つのは、要件 9（語ごとに別の所在で満たしても当たりにしない）を
所在1行の中で判定するためである。

## 2. 全文索引の付け替え

`videos_fts`（`title`・`path` の trigram）と、それを同期するトリガ
`video_locations_ai`・`video_locations_ad`・`video_locations_au` を落とす。代わりに、
`search_key` の1列だけを持つ次の索引を作る。

```sql
create virtual table location_search_fts using fts5(
    search_key, content='video_locations', content_rowid='id', tokenize='trigram'
);
```

同期トリガは `search_key` だけを写す（挿入・削除、`search_key` の更新）。題名や
パスの変更は、Go が同じ書き込みの中で `search_key` を作り直すことで索引に届く。

マイグレーションは `00007_location_search.sql` として新しく足す。既存のマイグレーションは
変えない（`scripts/migrations-immutable.sh`）。SQL だけでは §3 の正規化を書けないため、
マイグレーションは列・索引・トリガを作るところまでとし、鍵の値は §5 で埋める。Down は
`videos_fts` と3つのトリガを 00003 の定義どおりに作り直し、足した列を落とす。

## 3. `search_key` の規則

```text
search_key = fold(title) + "\n" + fold(relative_path)
```

- `relative_path` は、所在を含む登録メディアフォルダより下の相対パスで、区切りは `/`、
  拡張子を含む。登録メディアフォルダ自身のパスは入れない（要件 8）。
- どの登録メディアフォルダにも含まれない所在は `search_key = ''` とする。その所在は
  もともと一覧に出ない（`registeredLocationCondition`）。
- `fold` は `internal/domain` の照合形への変換で、検索語にも同じものを掛ける
  （[contracts/list-api.md §1](contracts/list-api.md#1-検索語の書き方)）。
  NFKC 正規化、Unicode の小文字化、ひらがなからカタカナへの置き換えの順に掛ける。

## 4. `title_key` の規則

`internal/domain` の関数が題名から作る、バイト順に比べると自然順になる文字列である。
`fold` を掛けたうえで、数字の連続を「桁数を表す固定長の接頭辞 + 先頭の 0 を除いた数字」に
置き換える。並べ替えは `title_key` が同じなら `id` で決着させる。

自然順の正本は `domain.CompareNatural` である。`title_key` のバイト順は、異なる2つの
題名について `CompareNatural` と同じ向きを返す（大文字小文字、全角半角、かなの違いだけの
題名は同じ鍵になり、`id` で決着する）。

## 5. 鍵を作る時点と `search_version`

`domain.SearchKeyVersion`（初版は 1）を §3・§4 の規則の版とする。鍵は次の時点で、
その所在について作り直し、`search_version` に現在の版を書く。

| 時点 | 対象 |
| --- | --- |
| 起動時、マイグレーションの直後で HTTP を受け付ける前 | `search_version` が現在の版より小さい所在すべて |
| 取り込みで所在を追加・更新したとき（`UpsertVideo`） | その所在 |
| メディアフォルダを追加・変更したとき（`AddMediaFolder`・`ReplaceMediaFolder`） | 新しい登録フォルダの下にある所在 |

起動時の埋め直しによって、既存のライブラリは再取り込みなしで要件 7 と 8 を満たす
（受け入れ条件 8）。規則を変えるときは版を上げ、同じ経路で全件を作り直す。削除された
所在は行ごと消えるので、作り直しの対象にならない。

## 6. 視聴状態の導き方

保存するものは無い。`playback_progress` を `content_key` で引き、次の3値に畳む。
一覧画面の `watchState`（`web/src/lib/format.ts`）と同じ定義で、サーバー側は
`internal/domain` の関数とそれに対応する SQL の条件で持つ。

| 状態 | 条件 |
| --- | --- |
| 未視聴 `unwatched` | 記録が無い、または `completed = 0` かつ `position_ms = 0` |
| 視聴済み `watched` | `completed = 1` |
| 視聴途中 `inProgress` | それ以外 |

「最近再生した順」は `playback_progress.updated_at` を使い、記録の無い動画は向きに
関係なく末尾に置く（受け入れ条件 14）。
