# Data model: フォルダのグループとフォルダ由来のタグ

親 Issue #326 の要件のうち、保存するもの・導くもの・その規則だけを書く。既存の表
（`videos`・`video_locations`・`playback_progress`・`tags`・`tag_names`・`video_tags`・`media_folders`）は
変えない。表の区分（索引と利用者データ）は [ARCHITECTURE.md](../../ARCHITECTURE.md) の「Two kinds of data」に従う。

## 1. マイグレーション

`internal/store/migrations/00010_folder_groups.sql` を足す。

```sql
-- 利用者データ。再スキャン・メディアフォルダの変更・再起動で消えてはならない。
-- 登録フォルダにも videos にも外部キーを張らない。
create table folder_group_overrides (
    -- フォルダの絶対パスを §2 の folderKey で整えたもの。
    path       text    primary key,
    mode       text    not null check (mode in ('ungroup', 'group_direct')),
    updated_at integer not null
) without rowid;

-- 以下は索引。§3 の作り直しが丸ごと置き換える。
create table folder_groups (
    id              integer primary key,
    -- フォルダを指す鍵。§2 の folderKey。グループの同一性はこれで決める。
    path_key        text    not null unique,
    -- フォルダの絶対パスの綴り（そのフォルダの下の所在のうちパスの最小のものから取る）。
    -- 応答の VideoFolder（rootId と相対パス）はこれから domain.LocateFolder で作る（§2）。
    path            text    not null,
    -- フォルダ名（domain.FolderName と同じ最後の段）。
    name            text    not null,
    -- 題名順の並べ替えの鍵。domain.NaturalSortKey(name)。
    title_key       text    not null
);

create table folder_group_members (
    video_id  integer primary key references videos (id) on delete cascade,
    group_id  integer not null references folder_groups (id) on delete cascade,
    -- グループの中の並び（0 始まり）。
    position  integer not null,
    unique (group_id, position)
);

-- 動画ごとの祖先フォルダ名（§4）。
create table video_folder_names (
    video_id integer not null references videos (id) on delete cascade,
    name     text    not null,
    primary key (video_id, name)
) without rowid;
create index video_folder_names_name_idx on video_folder_names (name, video_id);

-- 索引を作った規則の版。どちらかが今の値と違えば起動時に作り直す（§3）。
-- title_key は domain.NaturalSortKey で作るので、その版（SearchKeyVersion）も持つ。
create table folder_index_state (
    id             integer primary key check (id = 1),
    version        integer not null,   -- domain.FolderIndexVersion
    search_version integer not null,   -- domain.SearchKeyVersion
    -- 1 は「前回の作り直しが失敗した」。作り直しが成功すると 0 に戻る（§3）。
    stale          integer not null default 0 check (stale in (0, 1))
);
```

`videos` の行が消えると、メンバーとフォルダ名の行は連鎖で消える。それでメンバーが1本になった
グループは、次の作り直しまで1本のグループとして残る（§3）。カードのサムネイル（`cover`）と項目の `id` は
保存せず、残っているメンバーのうち `position` の最小のものから読み出しのたびに取るので、最初のメンバーが
消えても消えた動画を指さない。

## 2. 割り当ての規則

`internal/domain/folder_group.go` の1つの純粋関数が、次の入力から、グループ（パス・名前・並んだメンバー）と
動画ごとの祖先フォルダ名を返す。store はこの結果を表へ書くだけである。

- 入力: 登録フォルダ、登録フォルダの下にある全所在（動画の `id` とパス）、例外（`folderKey` → `mode`）。
- フォルダ `D` は、グループ分け（`direct`・`hasChild`・ルートかどうか）でも、例外の突き合わせでも、
  API の `(rootId, path)` からの引き当てでも、`folderKey(D)` で同一視する。Windows で綴りの大文字小文字が
  違う所在は同じフォルダに入る。
- **代表の所在**: 動画の所在のうちパスのバイト順で最小のもの（`videoColumnsTemplate` の `order by path limit 1` と同じ）。
- **直下の動画** `direct(D)`: 代表の所在の親フォルダが `D` である動画。同じ内容の別の所在は数えない（要件 6）。
- **子フォルダを持つ** `hasChild(D)`: どれかの所在（代表に限らない）が `D` の子フォルダの下にある。
  フォルダ画面の `folderCount` と同じく、所在から導く。空のフォルダは数えない。
- **グループになる**: `len(direct(D)) >= 2` かつ、次のどちらか。
  - `mode(D) = group_direct`
  - `mode(D)` が無く、`hasChild(D)` が偽で、`D` が登録フォルダそのものではない
- **並び**: `compareSiblings`（ファイル名の自然順、バイト順、`id`）と同じ全順序で、今の同じフォルダの前後と
  同じ比べ方である（要件 4）。ただし対象は `direct(D)` だけで、代表の所在が別のフォルダにある動画は、
  今の同じフォルダの前後（`DirectVideoPaths`）には入ってもグループには入らない（要件 6）。
- **名前**: `domain.FolderName` と同じく、フォルダの最後の段（要件 5）。
- **folderKey**: パスの末尾の区切りを落とし、Windows では `/` を `\` にそろえて `LowerASCII` を掛ける。
  所在と例外の突き合わせ、例外の主キーの両方に使う。登録フォルダの判定
  （`registeredLocationCondition`・`LocateVideoFolder`）と同じ区切りと大文字小文字の規則である。
- **フォルダの VideoFolder**: グループや例外の**フォルダそのもの**の絶対パスから `(rootId, path)` を
  求める関数 `domain.LocateFolder` を足す。今の `LocateVideoFolder` は所在（ファイル）のパスを受けて
  最後の段をファイル名として落とすので、フォルダのパスを渡すと一段上を指してしまう。両者は
  `pathBelowRoot` を共有し、違いは最後の段を落とすかどうかだけにする。入れ子のフォルダで、グループの
  `folder` が `GET /api/folders/{rootId}/group` と `getFolder` の引き当てに一致することをテストする。
- 例外のパスに一致するフォルダが無くても例外は消さない。同じパスのフォルダが戻れば効く（Edge Case）。

## 3. 作り直す時点

`rebuildFolderIndex(tx)` は、登録フォルダの下の全所在と例外を読み、§2 の関数を通し、
`folder_groups`・`folder_group_members`・`video_folder_names` を消して書き直し、
`folder_index_state` を今の `domain.FolderIndexVersion` と `domain.SearchKeyVersion` にする。1つの書き込み取引の中で行うので、
読み出しは作り直しの前か後のどちらかだけを見る。

| 時点 | 呼ぶ側 | 取引 |
| --- | --- | --- |
| スキャンを閉じる直前（成功でも失敗でも） | `app.Scans` → `ScanIndexStore.RebuildFolderIndex` | 作り直しだけの取引 |
| メディアフォルダの追加・置換・削除 | `SettingsStore` | その変更と同じ取引 |
| 例外の設定・解除、グループのタグ化 | `FolderGroupStore` | その変更と同じ取引 |
| 起動時、`version` か `search_version` が今の値と違うか、`stale = 1` か、行が無いとき | `cmd/mdm` → `ScanIndexStore.RefreshFolderIndex` | 作り直しだけの取引。`RefreshSearchKeys` の後で、HTTP とワーカーの開始より前 |
| 起動時、前回の停止で中断したスキャンを閉じたとき（`FailInterruptedScans` が 1 件以上） | `app.Scans.RecoverInterrupted` → `ScanIndexStore.RebuildFolderIndex` | 作り直しだけの取引 |

スキャンの途中（閉じる前）に足された動画は単体として、消えた動画のグループは残りのメンバーで出る。
スキャンを閉じる前の作り直しで正しい形になる。作り直しに失敗したときは、その取引は巻き戻り、
別の取引で `folder_index_state.stale = 1` を書いてログに残し、スキャンを閉じる。次の作り直しの時点
（次の起動を含む）まで前の索引を使う（plan の Structural Decisions 14）。`stale` の書き込みまで失敗した
ときは、次のスキャン・例外の変更・メディアフォルダの変更の作り直しで直る。作り直しの取引の途中で
プロセスが止まった場合は、スキャンが閉じられずに残るので、起動時の中断したスキャンの回復で作り直す。作り直しの前後をまたいだ
ページングでは、13 の取り込み中と同じく項目の重複や抜けが起こりうる。

## 4. フォルダ由来のタグ

- **フォルダ名**: 動画の登録フォルダの下にある**すべての**所在について、登録フォルダより下で
  ファイルより上の段（グループのフォルダを含む）を取り、`domain.NormalizeTagName` を掛ける。誤りになる名前は
  捨てる。動画ごとに重複を除いて `video_folder_names` に書く（要件 9、Edge Case「同じ内容が複数の場所にある」）。
- **付いているタグ**: `video_folder_names.name = tag_names.name` の行がある `tag_names.tag_id`。
  元の名前とシノニムのどちらにも当たる。照合は `tag_names` の主キーと同じ完全一致（014 の §2・§3）。
- **動画のタグ** = 手で付けたタグ（`video_tags` を `videos.content_key` で結ぶ）∪ フォルダ由来のタグ。
  同じタグが両方から付けば1件にまとめ、出所を両方持つ。
- これを使う読み出し:
  - `TagStore.TagsByContentKeys`（`Video.tags`）: 出所つきで返す。
  - タグでの絞り込み（014 §6）: 条件ごとの `exists` を「手で付けた行」または「フォルダ名の行」の OR にする。
  - 検索欄のタグ名の照合（014 §7）: タグ名の鍵を持つタグが動画に付いているかを、同じく2つの出所の OR にする。
  - タグごとの本数（014 §5）: どちらかの出所で付いている、いまライブラリにある動画を数える。
  - 選択の要約: 本数 `count` はどちらかの出所で、`manualCount` は手で付けた分だけで数える。
- 取り外し（`DetachTag`）は `video_tags` だけを消す。変更は要らない（要件 13）。
- 登録フォルダそのものの名前はフォルダ名に入らない（要件 9 の「ルートより下」）。そのため「直下を
  まとめる」で登録フォルダそのものがグループになっていても、タグ化はできない（409、
  [contracts/folder-groups-api.md §2](contracts/folder-groups-api.md#2-グループをタグに変える)）。
  タグを作っても要件 11 の結果（中の動画にそのタグが付く）にならないためである。
- タグ化（要件 11）は `FolderGroupStore` の1つの取引で、フォルダ名を `NormalizeTagName` に通し
  （誤りなら何も書かない）、名前かシノニムで引けたタグを使うか新しく作り、そのフォルダに `ungroup` を
  書き、§3 の作り直しを行う。

## 5. ライブラリの項目

`GET /api/library` の問い合わせ。013 の流れ（範囲と検索式 → `chosen` → 絞り込み → keyset）に、
項目へまとめる段を足す。

1. **メンバー単位の絞り込み**: `chosen`（範囲はライブラリ、検索式）に `videos` を結び、再生可否とタグの AND
   （§4 の出所の OR）を掛ける。ここを通った動画を「当たった動画」とする。1本の動画が全条件を満たす
   ことを求める（要件 19）。
2. **項目にまとめる**: 当たった動画のうち `folder_group_members` の行を持つものはそのグループへ、
   持たないものは動画の項目にする。グループは当たったメンバーが1本以上あれば1件になる。
3. **グループの集計**: 当たったかどうかに関わらず、グループの**全メンバー**から数える。

   | 値 | 動画の項目 | グループの項目 |
   | --- | --- | --- |
   | 追加日時 | `videos.added_at` | メンバーの最大 |
   | 更新日時 | `chosen` の所在の `mtime` | メンバーの代表の所在の `mtime` の最大 |
   | 最後に再生した時刻 | `playback_progress.updated_at` | メンバーの最大（無ければ NULL） |
   | 題名 | `chosen` の所在の `title_key` | `folder_groups.title_key` |
   | 長さ | `videos.duration_ms` | 分かっているメンバーの合計。1本も分かっていなければ NULL |
   | ファイルサイズ | `chosen` の所在の `size_bytes` | メンバーの代表の所在の `size_bytes` の合計 |
   | 視聴状態 | 今の `watchCondition` | §6 |

4. **項目単位の絞り込み**: 視聴状態の絞り込みを、上の視聴状態に掛ける（要件 19）。
5. **並べ替えと keyset**: 並び順の値は上の表の値。値が同じときの決着と keyset の `id` は、動画の項目は
   動画の `id`、グループの項目は残っているメンバーのうち `position` の最小のものの動画の `id` である。メンバーは重ならないので項目どうしで
   重ならない。シャッフルの鍵は `vv_shuffle_key(seed, その id)`。カーソルの形は 013 と同じ。
6. **件数**: 4 を通った項目の数（要件 20）。
7. **「すべて選択」**: 4 を通った項目の、動画の `id` とグループの全メンバーの `id`（要件 21）。

## 6. グループの視聴状態と開くメンバー

`internal/domain` の関数が、並んだメンバーの再生の記録（無い・位置・完了）から決める。1本の定義は
`domain.ClassifyWatch` と同じ。

- **視聴状態**: 見始めたメンバー（位置が 0 より大きいか完了）が無ければ `unwatched`、全メンバーが完了なら
  `watched`、それ以外は `inProgress`。**見終えた本数**は完了したメンバーの数（要件 17）。
- **開くメンバー**: 並びの順で、位置が 0 より大きく完了していない最初のメンバー。無ければ最初の未完了の
  メンバー。全部完了なら最初のメンバー（要件 23）。
- 項目の SQL の視聴状態（§5 の 4）と同じ結果になることを、同じ入力でテストする。
