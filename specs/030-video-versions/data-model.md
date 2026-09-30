# Data model: 同じ動画の別バージョン

親 Issue: #572。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、データの区分は
[ARCHITECTURE.md](../../ARCHITECTURE.md)「Rebuildable and user data」、利用者データを内容の鍵に結ぶ先例は
[specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)・
[specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md)、フォルダの索引は
[specs/017-folder-groups/data-model.md](../017-folder-groups/data-model.md)、ライブラリの項目は
[specs/027-partial-group-search/contracts/library-api.md](../027-partial-group-search/contracts/library-api.md)
にある。ここには、この feature が足す表と値、それを読み書きする規則だけを書く。書いていない表は変えない
（`videos`・`video_locations`・`playback_progress`・`video_tags`・`public_videos`・`video_overrides` に列は
足さない）。

## 1. 移行 `00022_video_versions.sql`

`main` の最後は `00021_video_overrides.sql`。

```sql
-- 同じ動画の別バージョンの集まり（specs/030-video-versions/research.md R-1）。作り直せない
-- 利用者データで、videos への外部キーを張らない。集まりのタグ・再生位置・公開の設定は
-- video_tags・playback_progress・public_videos に user_key を鍵として置く。
create table video_bundles (
    id                 integer primary key autoincrement,
    -- 集まりの値の鍵。'bundle:' || id を書く側が作る。content_key（<16進>:<サイズ>）と重ならない。
    user_key           text    not null unique,
    -- 代表のバージョンの content_key。メンバーの1つである（不変条件）。
    representative_key text    not null,
    created_at         integer not null,
    updated_at         integer not null
);

create table video_bundle_members (
    content_key text    primary key,
    bundle_id   integer not null references video_bundles (id) on delete cascade,
    added_at    integer not null
) without rowid;
create index video_bundle_members_bundle_idx on video_bundle_members (bundle_id, content_key);

-- 「違う動画」と判断した組（要件 10）。利用者データ。key_a < key_b に並べて 1 行。
create table video_version_dismissals (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- 以下は索引。内容の参照が無くなるときに消し、走査と取り込みで作り直せる。

-- 同じパスの中身が変わった後継の候補（R-5）。新しい中身の解析が終わったときに判定して消す。
create table video_successions (
    new_key         text    primary key,
    old_key         text    not null,
    old_duration_ms integer not null,
    created_at      integer not null
) without rowid;

-- 映像の指紋（R-6）。hashes はコマごとの 9 バイト（印 1 バイト + ハッシュ 8 バイト big endian）。
create table video_fingerprints (
    content_key text    primary key,
    version     integer not null,   -- domain.FingerprintVersion
    interval_ms integer not null,   -- 作ったときのスプライトの配置の間隔
    hashes      blob    not null,
    updated_at  integer not null
) without rowid;

-- 「同じ動画かもしれない」候補（R-7）。key_a < key_b。
create table video_version_candidates (
    key_a      text    not null,
    key_b      text    not null,
    distance   integer not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;
```

続けて、`00014` と同じ手順で `jobs` を作り直し、`kind` の検査に `'fingerprint'` を足す。完成した
スプライトの動画（`seek_thumbnail_state = 'done'` で登録の所在がある）に `fingerprint` の job を積む
（`00015` の積み方と同じ）。Down は job を消して `jobs` を戻し、6 つの表を落とす。

不変条件（`internal/store/invariants_test.go` に足す）: `video_bundles.representative_key` はその集まりの
`video_bundle_members` にある。メンバーが 2 本未満の集まりは無い（書く側が解く）。`user_key` は
`bundle:` で始まる。`video_bundle_members.content_key` は空でない。`video_version_candidates` の組は
`video_version_dismissals` に無く、同じ集まりの 2 本でもない。

区分: `video_bundles`・`video_bundle_members`・`video_version_dismissals` は作り直せない利用者データ
（ARCHITECTURE.md の一覧に足す）。`video_successions`・`video_fingerprints`・`video_version_candidates` は
索引で、内容の参照が無くなるとき（`UpsertVideo` の付け替え、`DeleteVideos`、`deleteOrphanVideos`、
メディアフォルダの削除・置換）に、動画の行を消す同じ取引でその鍵の行を消す（`releaseContentIndex`）。
生成物の片付け（`RemoveContent`）は表に触れない。

`domain.FolderIndexVersion` を 2 にする（§4）。

## 2. `domain` に足す値

| 値 | 中身 |
| --- | --- |
| `Video.UserKey` | 利用者データの鍵（§3）。集まりのメンバーなら `user_key`、そうでなければ `ContentKey` と同じ。保存層が埋め、空の `ContentKey` の動画では空 |
| `Video.Versions` | `*VideoVersionsRef{Count int, RepresentativeID int64}`。集まりのメンバーのときだけ、詳細の読み出しで埋める。`Count` は見る人に見せてよい所在を持つメンバーの本数 |
| `VideoVersions{RepresentativeID int64, Items []Video}` | 集まりの全バージョン。代表が先頭、続きは題名の自然順（同じなら id） |
| `DurationsMatch(a, b int64) bool` | `abs(a-b) <= max(1000, max(a,b)*0.005)`。0 以下の尺は一致しない |
| `FingerprintVersion = 1` | 指紋の規則の版 |
| `FrameHash{Hash uint64, Flat bool}` | コマ 1 つのハッシュと、単色（比較から外す）の印 |
| `Fingerprint{Version int, IntervalMs int64, Frames []FrameHash}` | `Encode() []byte`・`DecodeFingerprint([]byte, version, interval)`。1 コマ 9 バイト |
| `HashFrame(luma [32][32]uint8) FrameHash` | 2 次元 DCT の低周波 8×8 の直流を除く 63 係数を中央値と比べたビット。輝度の分散が `FingerprintFlatVariance` 未満なら `Flat` |
| `CompareFingerprints(a, b Fingerprint) (distance int, ok bool)` | 版と間隔が違えば `ok = false`。両方 `Flat` でない同じ番号のコマのハミング距離の中央値。比べたコマが `FingerprintMinComparableFrames`（3）未満なら `ok = false` |
| `FingerprintMatchMaxDistance = 12` | 候補にする中央値の上限 |
| `JobFingerprint JobKind = "fingerprint"` | `JobKinds` の最後。`ClaimConditionFor` は `RegisteredLocation` と新しい `SeekThumbnailFinished`（`seek_thumbnail_state = done`）。`ScanActivityKind` にも足す |
| `VideoBundleChanged{VideoIDs []int64}` | 束ねの変化。`Event` を実装する（R-9） |
| 誤りの値 | `ErrNotBundled`（集まりに属さない動画への代表の変更・解除）、`ErrRepresentativeNotSelected`（`representativeId` が `videoIds` に無い）、`ErrTooFewVersions`（引けた動画が 2 本未満） |

## 3. 利用者データの鍵

動画 `v` の利用者データの鍵は次の式で決める（`internal/store/user_keys.go` の `userKeyExpr(alias)`）。

```sql
coalesce((select b.user_key from video_bundle_members m join video_bundles b on b.id = m.bundle_id
          where m.content_key = v.content_key), v.content_key)
```

空の `content_key` は今までどおりどの利用者データにも結ばない（`content_key <> ''` の条件はそのまま）。

この式を通す読み出しと書き込み（R-2）:

| 経路 | 今 | 変更 |
| --- | --- | --- |
| `videoColumnsTemplate`・`listColumns`・外部連携の列 | `videos.content_key` | 加えて `userKeyExpr` を `Video.UserKey` に |
| `publicColumn`・`publicVideoCondition` | `public_videos.content_key = videos.content_key` | 鍵の式で結ぶ |
| `filteredFrom`・`libraryItemsCTE` の `playback_progress` の結合 | `p.content_key = videos.content_key` | 鍵の式で結ぶ |
| `videoHasTagCondition`・`tagNameMatchCondition`・タグの本数・要約 | `video_tags.content_key = v.content_key` | 鍵の式で結ぶ。フォルダ名から付く分（`video_folder_names`、`video_id`）は変えない |
| `TagsByContentKeys`・`ProgressByContentKeys` | 呼び出し側が `ContentKey` を渡す | `internal/httpapi` が `UserKey` を渡す。手で付けた分は鍵で、フォルダ名の分は `videos.content_key` から引くよう `tagsByContentKeys` の 2 つ目の出所を `videos.id` 経由にする |
| `PUT /api/videos/{id}/progress` | `SaveProgress(video.ContentKey)` | `SaveProgress(video.UserKey)`。尺は再生したバージョンのもの |
| `registeredContentKeysForVideoIDs`（タグの付け外し・要約、公開の切り替え） | `content_key` | `userKeysForVideoIDs`（同じ集まりは 1 つにまとめる）。`SetVideosPublic` は返す鍵に、影響した集まりの全メンバーの `content_key` を入れる（ゲストの配信の打ち切りは内容ごと） |
| `OverrideStore`（表示名・サムネイルの位置） | `content_key` | 変えない（内容ごと） |
| 生成物・指紋・ライブ変換の解析情報 | `content_key` | 変えない |

## 4. 見せる動画と一覧

`shownVideosCTE(audience)` は、見る人に見せる動画 `shown(video_id, bundle_id)` を返す
（`internal/store/user_keys.go`）。

- 集まりに属さない動画で、見る人に見せてよい所在（`visibleLocationCondition`）を持つもの。
- 各集まりの**実効の代表**: `representative_key` の動画に見せてよい所在があればそれ、無ければ見せてよい
  所在を持つメンバーのうち `videos.id` の最小のもの。1 本も無ければ集まりは出ない（R-3）。

これを通す読み出し（R-3、R-4）:

| 読み出し | 変更 |
| --- | --- |
| `chosenLocationsCTE` | `shown` の動画の所在に範囲を掛け、検索式は `exists (同じ集まりのどれかの動画の登録の所在で式が真)`（集まりに属さなければ自分の所在）に変える。`chosen.path` は今までどおり範囲にある自分の所在のパスの最小 |
| `libraryItemsCTE`・`LibraryIDs`・`ListVideos`・`ListFolderVideos`・`CountVideos` | `chosen` を経由するので追随する。タグ・視聴状態・並び順・`total` は見せる動画の値 |
| フォルダの直下の動画とフォルダの本数（`folders.go` の `DirectVideoPaths`・フォルダの一覧の本数） | `shown` の動画の所在だけを数える。代表以外だけのフォルダは動画の無いフォルダ（Edge Case） |
| `VideosAddedNear`・`VideosByIDs`（関連） | `shown` の動画に限る |
| `folderIndexLocations` | 所有者から見た `shown` の動画の所在だけ。`FolderIndexVersion = 2` |
| `GetVideo`・`VideoLocations`・配信・字幕・`versions` | 変えない。代表以外のバージョンも返す |

ゲストには公開が集まりの鍵で決まるので（§3）、公開した集まりは全バージョンが見せてよい所在を持つ。

## 5. 同じパスの中身の引き継ぎ

`UpsertVideo`（`internal/store/scan_index.go`）で、`locationExists && oldKey != file.ContentKey` かつ
`newVideo` かつ前の動画の行が所在を失って消えるとき:

1. 前の行を消す前に `duration_ms` を読む。null なら記録しない（解析が終わっていない・失敗した動画は
   対象外。Edge Case）。
2. `video_successions` に `(new_key = file.ContentKey, old_key, old_duration_ms)` を書く（`new_key` の
   既存の行は置き換える）。
3. `newVideo` で作る鍵が `video_successions.old_key` に一致すれば、その行を消す（前の中身が別のパスに
   移った。Edge Case「ファイルの入れ替え」）。

`applySuccession(tx, contentKey, durationMs)` を `ApplyProbe`・`ApplyProbeForJob` が、解析の結果を書いた
あと同じ取引で呼ぶ:

1. `new_key = contentKey` の行を読み、あれば消す。無ければ終わり。
2. `DurationsMatch(old_duration_ms, durationMs)` でなければ終わり（別の動画で上書きされた。要件 8）。
3. `old_key` を参照する動画があれば終わり。
4. 引き継ぐ。`old_key` が `video_bundle_members` にあれば `content_key` を `new_key` に付け替え、
   `representative_key = old_key` なら `new_key` にする。それ以外は次の行を付け替える:
   `playback_progress`（`new_key` の行があれば `old_key` の行で置き換える）、`public_videos`（同じ）、
   `video_tags`（和）、`video_version_dismissals`（`old_key` を `new_key` に、並び替えて重複は 1 つ）。
   集まりのメンバーの場合も、`old_key` 自身のこれらの行（集まりに入る前の値）を同じく付け替える。
5. `VideoBundleChanged{VideoIDs: [その動画]}` を確定後に発行する。

解析が上限まで失敗しても行は残す。やり直し（`RetryProbe`）が成功したときに判定する。行は `new_key` の
内容の参照が無くなるときに消える（§1）。

## 6. 指紋

`fingerprint` の job（R-6）:

1. `ClaimJob` は `seek_thumbnail_state = done` の動画だけ取り出す。
2. `app.Ingest.Fingerprint` は `JobIdentityCurrent` を確かめ、`ArtifactStore.SeekSprite(contentKey)` で
   配置を読み、各シートを `SeekSpriteSheet` で読んで `media.SpriteFingerprint(sprite, sheets)` に渡す。
   スプライトが無ければ誤りを返す（job は再試行し、上限で `failed`。問題の記録は他の段階と同じ）。
3. `media.SpriteFingerprint` は各コマを切り出し、上下左右の黒い帯（平均輝度 16 未満の行と列）を落として
   32×32 の輝度に縮め、`domain.HashFrame` を並べて `Fingerprint{Version, IntervalMs, Frames}` を返す。
4. `IngestStore.ApplyFingerprintForJob(job, fingerprint)` は `video_fingerprints` を置き換え、同じ取引で
   §7 の候補を作り直す。

シーク用サムネイルの完了（`SetSeekThumbnailStateForJob` の `done`）と `RequeueMissingSeekThumbnails` の
作り直しの完了で `fingerprint` の job を `requeueJob` で積む。旧い 6 シートのスプライトから作った指紋は
`interval_ms` が今の配置と違うので、今の配置の指紋とは比べない。

## 7. 候補

`ApplyFingerprintForJob` は、その `content_key`（`K`）の候補をいったん消し、次で作り直す
（`vv_fingerprint_distance(a, b)` は `DecodeFingerprint` して `CompareFingerprints` を呼び、比べられなければ
-1 を返す決定的な関数）:

```sql
insert or ignore into video_version_candidates (key_a, key_b, distance, created_at)
select min(?, f.content_key), max(?, f.content_key), vv_fingerprint_distance(?, f.hashes), ?
  from video_fingerprints f
  join videos v on v.content_key = f.content_key
 where f.content_key <> ? and f.version = ? and f.interval_ms = ?
   and v.duration_ms is not null and abs(v.duration_ms - ?) <= max(1000, max(v.duration_ms, ?) * 5 / 1000)
   and not exists (select 1 from video_version_dismissals d
                   where d.key_a = min(?, f.content_key) and d.key_b = max(?, f.content_key))
   and not exists (select 1 from video_bundle_members ma join video_bundle_members mb on mb.bundle_id = ma.bundle_id
                   where ma.content_key = ? and mb.content_key = f.content_key)
   and vv_fingerprint_distance(?, f.hashes) between 0 and ?
```

- 束ねる操作は、同じ集まりになった 2 本の候補を消す。
- 却下（`Dismiss`）は `video_version_dismissals` に書き、その組の候補を消す。
- 内容の参照が無くなるときは、その鍵の候補を消す（§1）。
- `Candidates(audience = owner)` は、両方の鍵に登録の所在を持つ動画がある組を `created_at` の新しい順に
  返す（上限 200 件、`total` は全件）。応答の `Video` は各鍵の動画（[contracts/screen-api.md §5](contracts/screen-api.md)）。

## 8. 保存層の操作（`VersionStore`）

`*DB` を持つ役割（確定後の発行とフォルダの索引の作り直しのため）。どれも 1 つの取引で、途中で失敗したら
何も残さない。

| 操作 | 規則 |
| --- | --- |
| `Bundle(videoIDs, representativeID) (VideoVersions, error)` | id をいまライブラリにある動画の `content_key` に引き直す（引けない id は `ErrNotFound`）。2 本未満は `ErrTooFewVersions`、代表が含まれなければ `ErrRepresentativeNotSelected`。新しい集まりを作り、各動画（既に集まりに属していればその集まりの全メンバー）を移す。値は代表の `UserKey`（代表が集まりに属していればその集まりの鍵）の `playback_progress`・`video_tags`・`public_videos` の行を新しい `user_key` へ写す。吸収した集まりの行と値は消さない（Edge Case「集まり同士を束ねる」。値はその鍵に残る）。同じ集まりになった組の候補を消し、`rebuildFolderIndex` を呼び、`VideoBundleChanged{全メンバー}` を発行する |
| `MakeRepresentative(videoID) (VideoVersions, error)` | メンバーでなければ `ErrNotBundled`。`representative_key` を替える。値は触らない。`rebuildFolderIndex`、`VideoBundleChanged{全メンバー}` |
| `Unbundle(videoID) (Video, error)` | メンバーでなければ `ErrNotBundled`。行を消す（その動画は自分の `content_key` の値に戻る）。代表だったなら、残りのうち実効の代表の規則（§4）で選んだ 1 本を代表にする。残りが 1 本なら集まりを解く: 集まりの鍵の 3 つの表の行を残った 1 本の `content_key` へ写し（既存の行は置き換える）、集まりの行を消す（メンバーは連鎖）。`rebuildFolderIndex`、`VideoBundleChanged{元の全メンバー}` |
| `Versions(audience, videoID) (VideoVersions, error)` | 動画が見せられなければ `ErrNotFound`。メンバーでなければ自分 1 本。見せてよい所在を持つメンバーを代表を先頭に返す |
| `Candidates() (CandidatePage, error)`・`Dismiss(videoIDs [2]int64) error` | §7 |

`getVideo`（詳細）は `Video.Versions` を埋める。一覧の列には入れない。
