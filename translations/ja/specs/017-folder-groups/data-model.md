---
source: specs/017-folder-groups/data-model.md
sourceHash: 20c27fc2b514cca497fda303d3a00f56a70201169723912d4bd8ad74ec53b483
---

# データモデル: フォルダのグループとフォルダから導くタグ {#data-model-folder-groups-and-folder-derived-tags}

この文書は、親 Issue #326 が保存するもの、導くもの、その両方の規則だけを扱う。既存のテーブル (`videos`、`video_locations`、`playback_progress`、`tags`、`tag_names`、`video_tags`、`media_folders`、`public_videos`) は変わらない。テーブルの分類 (索引と利用者のデータ) は [ARCHITECTURE.md](../../ARCHITECTURE.md) の「Rebuildable and user data」に従う。

## マイグレーション {#migration}

`internal/store/migrations/00012_folder_groups.sql` を加える。

```sql
-- User data. Must survive rescans, media folder changes and restarts.
-- No foreign key to registered folders or to videos.
create table folder_group_overrides (
    -- The folder's absolute path normalized by folderKey (see "Assignment rules").
    path       text    primary key,
    mode       text    not null check (mode in ('ungroup', 'group_direct')),
    updated_at integer not null
) without rowid;

-- The tables below are an index. The rebuild in "When the index is rebuilt" replaces them wholesale.
create table folder_groups (
    id              integer primary key,
    -- The key that identifies the folder: folderKey (see "Assignment rules"). Group identity is decided by this.
    path_key        text    not null unique,
    -- The folder's absolute path as spelled (taken from the smallest path among the locations under it).
    -- The response's VideoFolder (rootId and relative path) is built from this with domain.LocateFolder (see "Assignment rules").
    path            text    not null,
    -- The folder name (the last segment, as domain.FolderName).
    name            text    not null,
    -- Sort key for title order. domain.NaturalSortKey(name).
    title_key       text    not null
);

create table folder_group_members (
    video_id  integer primary key references videos (id) on delete cascade,
    group_id  integer not null references folder_groups (id) on delete cascade,
    -- Order within the group (0-based).
    position  integer not null,
    unique (group_id, position)
);

-- Ancestor folder names per video (see "Folder-derived tags").
create table video_folder_names (
    video_id integer not null references videos (id) on delete cascade,
    name     text    not null,
    primary key (video_id, name)
) without rowid;
create index video_folder_names_name_idx on video_folder_names (name, video_id);

-- Versions of the rules that built the index. If either differs from the current value, startup rebuilds (see "When the index is rebuilt").
-- title_key is built with domain.NaturalSortKey, so its version (SearchKeyVersion) is kept too.
create table folder_index_state (
    id             integer primary key check (id = 1),
    version        integer not null,   -- domain.FolderIndexVersion
    search_version integer not null,   -- domain.SearchKeyVersion
    -- 1 means "the last rebuild failed". A successful rebuild resets it to 0 (see "When the index is rebuilt").
    stale          integer not null default 0 check (stale in (0, 1))
);
```

`videos` の行を削除すると、そのメンバーの行とフォルダ名の行も連鎖して消える。メンバーが 1 本だけ残ったグループは、次の作り直し ([索引を作り直すとき](#when-the-index-is-rebuilt)) まで 1 本のグループのままである。カードの絵柄のサムネイル (`previews`) と項目の `id` は保存しない。毎回残ったメンバーから読むので、削除した動画を指すことはない。

## 割り当ての規則 {#assignment-rules}

`internal/domain/folder_group.go` の 1 つの純粋な関数が、下の入力からグループ (パス、名前、順序付きのメンバー) と各動画の祖先のフォルダ名を返す。store は結果をテーブルに書くだけである。

- 入力: 登録したフォルダ、その下のすべての場所 (動画の `id` とパス)、上書き (`folderKey` → `mode`)。
- フォルダ `D` は、グループ分け (`direct`、`hasChild`、ルートかどうか)、上書きとの照合、API の `(rootId, path)` からの検索で、`folderKey(D)` によって識別する。Windows では、綴りが大文字小文字だけ違う場所は同じフォルダに入る。

| 用語 | 定義 |
| --- | --- |
| **代表の場所** | 動画の場所のうち、バイト順でパスが最も小さい場所 (`videoColumnsTemplate` の `order by path limit 1` と同じ) |
| **直下の動画** `direct(D)` | 代表の場所の親フォルダが `D` である動画。同じ内容の他の場所は数えない (要件 6) |
| **子フォルダがある** `hasChild(D)` | ある場所 (代表の場所に限らない) が `D` の子フォルダの下にある。フォルダ画面の `folderCount` と同じく場所から導く。空のフォルダは数えない |
| **グループになる** | `len(direct(D)) >= 2` で、`mode(D) = group_direct` であるか、`mode(D)` がなく `hasChild(D)` が偽で `D` 自体が登録したフォルダでない |
| **順序** | 全順序 `compareSiblings`: ファイル名の自然順、次にバイト順、次に `id` (要件 4)。順序を付けるのは `direct(D)` だけである: 代表の場所が別のフォルダにある動画はグループに入らない (要件 6) |
| **名前** | フォルダの最後の要素。`domain.FolderName` と同じ (要件 5) |
| **folderKey** | パスの末尾の区切り文字を落とす。Windows では `/` を `\` にし、`LowerASCII` を適用する。場所を上書きに照合するのにも、上書きの主キーにも使う。登録したフォルダの確認 (`registeredLocationCondition`、`LocateVideoFolder`) と同じ区切り文字と大文字小文字の規則である |

- **フォルダの VideoFolder**: 新しい関数 `domain.LocateFolder` が、グループまたは上書きの**フォルダそのもの**の絶対パスから `(rootId, path)` を計算する。今の `LocateVideoFolder` は場所 (ファイル) のパスを受け取り、最後の要素をファイル名として落とすので、フォルダのパスを渡すと 1 つ上を指す。2 つは `pathBelowRoot` を共有し、最後の要素を落とすかどうかだけが違う。テストは、入れ子のフォルダで、グループの `folder` が `GET /api/folders/{rootId}/group` と `getFolder` の検索に一致することを確かめる。
- 上書きは、そのパスに合うフォルダがなくても残す。同じパスのフォルダが戻れば、また効く (Edge Case)。

## 索引を作り直すとき {#when-the-index-is-rebuilt}

`rebuildFolderIndex(tx)` は、登録したフォルダの下のすべての場所と上書きを読み、[割り当ての規則](#assignment-rules) の関数に通し、`folder_groups`、`folder_group_members`、`video_folder_names` を削除して書き直し、`folder_index_state` を今の `domain.FolderIndexVersion` と `domain.SearchKeyVersion` にする。1 つの書き込みトランザクションの中で動くので、読み取りには作り直しの前か後の状態が見える。

| いつ | 呼び出し元 | トランザクション |
| --- | --- | --- |
| スキャンが閉じる直前 (成功でも失敗でも) | `app.Scans` → `ScanIndexStore.RebuildFolderIndex` | 作り直しだけのトランザクション |
| メディアフォルダを加える、置き換える、外す | `SettingsStore` | その変更と同じトランザクション |
| 上書きを設定するか外す、またはグループをタグにする | `FolderGroupStore` | その変更と同じトランザクション |
| 起動時、`version` か `search_version` が今の値と違う、`stale = 1`、または行がないとき | `cmd/mdm` → `ScanIndexStore.RefreshFolderIndex` | 作り直しだけのトランザクション。`RefreshSearchKeys` の後、HTTP とワーカーが始まる前 |
| 起動時、前回の終了で中断したスキャンを閉じたとき (`FailInterruptedScans` が 1 つ以上閉じる) | `app.Scans.RecoverInterrupted` → `ScanIndexStore.RebuildFolderIndex` | 作り直しだけのトランザクション |

スキャン中 (閉じる前) は、加わった動画は単独の動画として現れ、動画が消えたグループは残ったメンバーで現れる。スキャンが閉じる前の作り直しが正しい形を作る。作り直しが失敗すると、そのトランザクションはロールバックする。別のトランザクションが `folder_index_state.stale = 1` を書き、失敗をログに残し、スキャンは閉じる。次の作り直しの時点 (次の起動を含む) までは前の索引を使う (plan の Structural Decisions 14)。`stale` の書き込みも失敗したら、次のスキャン、上書きの変更、メディアフォルダの変更による作り直しが直す。作り直しのトランザクションの途中でプロセスが止まると、スキャンは閉じないまま残るので、起動時の中断したスキャンの回復が作り直す。作り直しをまたぐページ送りは、013 の取り込み中と同じく、項目が重複したり飛ばされたりしうる。

## フォルダから導くタグ {#folder-derived-tags}

- **フォルダ名**: 動画の登録したフォルダの下の**すべての**場所について、登録したフォルダより下でファイルより上の要素 (グループのフォルダを含む) を取り、`domain.NormalizeTagName` を適用する。エラーになる名前は落とす。各動画の名前は、重複なしで `video_folder_names` に書く (要件 9、Edge Case `同じ内容が複数の場所にある`)。
- **付くタグ**: `video_folder_names.name = tag_names.name` の行の `tag_names.tag_id`。元の名前と同義語の両方が一致する。照合は `tag_names` の主キーと同じく完全一致である ([014 data-model.md、Name rules](../014-video-tags/data-model.md#name-rules) と [Name lookup](../014-video-tags/data-model.md#name-lookup))。
- **動画のタグ** = 手で付けたタグ (`videos.content_key` で結合した `video_tags`) ∪ フォルダから導くタグ。両方から付いたタグは、両方の出どころを持つ 1 つの項目になる。
- これを使う読み取り:

| 読み取り | 使い方 |
| --- | --- |
| `TagStore.TagsByContentKeys` (`Video.tags`) | タグを出どころとともに返す |
| タグの絞り込み ([014 data-model.md、Tag filter](../014-video-tags/data-model.md#tag-filter)) | 各条件の `exists` が、「手で付けた行」か「フォルダ名の行」の OR になる |
| 検索欄でのタグ名の照合 ([014 data-model.md、Matching tag names in the search box](../014-video-tags/data-model.md#matching-tag-names-in-the-search-box)) | 名前の鍵を持つタグが動画に付いているかも、同じく 2 つの出どころの OR である |
| タグごとの数 ([014 data-model.md、Video counts](../014-video-tags/data-model.md#video-counts)) | 今ライブラリにあり、どちらかの出どころからタグが付いた動画を数える |
| 選択の要約 | `count` はどちらの出どころも数え、`manualCount` は手で付けたものだけを数える |

- 外す操作 (`DetachTag`) は `video_tags` から削除するだけで、変更はいらない (要件 13)。
- 登録したフォルダ自体の名前はフォルダ名ではない (要件 9、「ルートの下」)。そのため、`直下をまとめる` で登録したフォルダ自体がグループになっても、タグにはできない (409、[contracts/folder-groups-api.md、Turning a group into a tag](contracts/folder-groups-api.md#turning-a-group-into-a-tag)): タグを作っても要件 11 の結果 (中の動画にタグが付く) にならないからである。
- グループをタグにする (要件 11) のは `FolderGroupStore` の 1 つのトランザクションである: フォルダ名を `NormalizeTagName` に通し (エラーなら何も書かない)、名前か同義語で見つかったタグを使うか新しく作り、フォルダに `ungroup` を書き、[索引を作り直すとき](#when-the-index-is-rebuilt) の作り直しを実行する。

## ライブラリの項目 {#library-items}

`GET /api/library` のクエリ。013 の流れ (範囲と検索式 → `chosen` → 絞り込み → キーセット) に、動画を項目にまとめる段階を加える。

1. **メンバーごとの絞り込み**: `videos` を `chosen` (範囲: ライブラリ、検索式) と結合し、再生できるかとタグの AND ([フォルダから導くタグ](#folder-derived-tags) の出どころの OR) を適用する。通った動画が「一致した動画」である。単独の動画はすべての条件を満たさなければならない (要件 19)。
2. **項目にまとめる**: `folder_group_members` の行を持つ一致した動画はそのグループに入り、残りは動画の項目になる。グループは、メンバーの少なくとも 1 本が一致したとき 1 つの項目になる。
3. **グループの集計**: 一致したかどうかにかかわらず、グループの**すべてのメンバー**から計算する。

   | 値 | 動画の項目 | グループの項目 |
   | --- | --- | --- |
   | 追加日 | `videos.added_at` | メンバーの最大 |
   | 更新日 | `chosen` の場所の `mtime` | メンバーの代表の場所の `mtime` の最大 |
   | 最後の再生 | `playback_progress.updated_at` | メンバーの最大 (なければ NULL) |
   | タイトル | `chosen` の場所の `title_key` | `folder_groups.title_key` |
   | 長さ | `videos.duration_ms` | 長さがわかっているメンバーの合計。どれもわからなければ NULL |
   | ファイルサイズ | `chosen` の場所の `size_bytes` | メンバーの代表の場所の `size_bytes` の合計 |
   | 視聴の状態 | 今の `watchCondition` | [グループの視聴の状態と開くメンバー](#group-watch-state-and-the-member-to-open) |

4. **項目ごとの絞り込み**: 視聴の状態の絞り込みを、上の視聴の状態に適用する (要件 19)。
5. **並べ替えとキーセット**: 並べ替えの値は上の表の値である。同順位の決め手とキーセットの `id` は、動画の項目では動画の `id`、グループの項目では残ったメンバーのうち `position` が最も小さいものの `id` である。メンバーは重ならないので、項目も重ならない。シャッフルの鍵は `vv_shuffle_key(seed, that id)` である。カーソルの形は 013 と同じである。
6. **数**: 段階 4 を通った項目の数 (要件 20)。
7. **すべて選択**: 段階 4 を通った項目の動画の `id` と、それらのグループのすべてのメンバーの `id` (要件 21)。

## グループの視聴の状態と開くメンバー {#group-watch-state-and-the-member-to-open}

`internal/domain` の関数が、順序付きのメンバーの再生記録 (なし、位置、完了) から両方を決める。単独の動画は `domain.ClassifyWatch` と同じく分類する。

| 値 | 規則 |
| --- | --- |
| **視聴の状態** | どのメンバーも始まっていない (位置が 0 より大きいか完了) なら `unwatched`、すべてのメンバーが完了なら `watched`、それ以外は `inProgress`。**見た数**は完了したメンバーの数である (要件 17) |
| **開くメンバー** | 順に、位置が 0 より大きく完了していない最初のメンバー。なければ完了していない最初のメンバー。すべて完了なら最初のメンバー (要件 23) |

- テストは、同じ入力で、これが項目の SQL の視聴の状態 ([ライブラリの項目](#library-items) の段階 4) と一致することを確かめる。

## 閲覧者ごとの見え方 {#visibility-per-audience}

索引 ([マイグレーション](#migration) から [索引を作り直すとき](#when-the-index-is-rebuilt) まで) は閲覧者に依存しない: 所有者から見たすべての場所から 1 つの索引を作る。ゲストへの応答は、読むときにメンバーに 016 の公開の条件 (`visibleLocationCondition`、[016 data-model.md、Audience and the public video condition](../016-single-account-auth/data-model.md#audience-and-the-public-video-condition)) を適用する。

- **グループのメンバー**: ゲストには公開されたメンバーだけを数える。`videoCount`、`videoIds`、`previews`、長さとサイズの合計、追加日と更新日の最大、`Video.group` の `position` と `count`、関連動画の `group.items`、グループ内の前後は、すべて公開されたメンバーだけから作る。公開されたメンバーがちょうど 1 本のグループは、ゲストには動画の項目として見せる (1 本のグループのカードはない)。1 本もないグループは見せない。
- **視聴とタグ**: ゲストは再生位置もタグも受け取らない ([016 guest-api.md、What guests see](../016-single-account-auth/contracts/guest-api.md#what-guests-see))。`LibraryGroup` の `watchedCount`、`watchState`、`lastPlayedAt` は省き、`tags` は空である。`openVideoId` は順序で最初の公開されたメンバーである。フォルダから導くタグも同じく、ゲストへの応答、絞り込み、検索には使わない (`tag` の条件はゲストには 400 で、検索はタグ名に一致しない)。
- **上書きとグループ分けの判断**: どのフォルダがグループかは、所有者のすべての場所から決める ([割り当ての規則](#assignment-rules))。ゲストはその結果を公開されたメンバーに絞ったものだけを受け取り、公開されていない動画の数や名前が漏れることは決してない。
- `GET /api/folders/{rootId}/group`: 少なくとも 2 本のメンバーが公開されていなければ、ゲストには 404 である。
