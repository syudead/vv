---
source: specs/030-video-versions/data-model.md
sourceHash: 21e5101839891a08880b072822314d9d840d6330376fc35afbd4334c17c4a53b
---

# データモデル: 同じ動画のバージョン {#data-model-versions-of-the-same-video}

親 Issue: #572。

モデルの残りは変わらない。既存のテーブルは [internal/store/migrations/](../../internal/store/migrations/) が定義し、データの分類は [running-vv.md、Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery) にあり、利用者のデータを内容の鍵に結び付ける先例は [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md) と [specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md)、フォルダの索引は [specs/017-folder-groups/data-model.md](../017-folder-groups/data-model.md)、ライブラリの項目は [specs/027-partial-group-search/contracts/library-api.md](../027-partial-group-search/contracts/library-api.md) にある。このファイルは、この機能が加えるテーブルと値、それを読み書きする規則だけを持つ。ここにないテーブルは変わらない (`videos`、`video_locations`、`playback_progress`、`video_tags`、`public_videos`、`video_overrides` に列は加えない)。

## マイグレーション {#migration}

`main` の最後のマイグレーションは `00021_video_overrides.sql` である。`scripts/migrations-immutable.sh` は PR のベースにあるマイグレーションの編集を禁じるので、テーブルを加える各単位は新しい番号のマイグレーションを 1 つ加え、feature ブランチに入ったマイグレーションは決して変えない。

| マイグレーション | 加えるもの | 単位 |
| --- | --- | --- |
| `00023_video_versions.sql` | `video_bundles`、`video_bundle_members`、`video_version_dismissals` | まとまりを保存し、利用者のデータの鍵を解決する |
| `00024_video_successions.sql` | `video_successions` | スキャン中にパスの内容が変わったとき値を引き継ぐ |
| `00025_video_fingerprints.sql` | `video_fingerprints`、`jobs.kind` の `fingerprint`、ジョブを積むこと | シーク用のスプライトから映像の指紋を取る取り込みの段階 |
| `00026_video_version_candidates.sql` | `video_version_candidates` | 候補を計算し、候補を一覧して退ける API |

各マイグレーションの Down は、そのマイグレーションが加えたものだけを戻す。テーブルの定義は次のとおりである (マイグレーションごとに見出しのコメントで区切る)。

```sql
-- 00023_video_versions.sql
-- Bundles of versions of the same video (specs/030-video-versions/research.md R-1). User data that
-- cannot be rebuilt; no foreign key to videos. The bundle's tags, playback position and visibility
-- sit in video_tags, playback_progress and public_videos keyed by user_key.
create table video_bundles (
    id                 integer primary key autoincrement,
    -- Key of the bundle's values. The writer builds 'bundle:' || id. Never collides with a content_key (<hex>:<size>).
    user_key           text    not null unique,
    -- content_key of the representative version. It is one of the members (invariant).
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

-- Pairs judged to be "different videos" (requirement 10). User data. One row with key_a < key_b.
create table video_version_dismissals (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- The tables below are index data. They are deleted when nothing references the content any more,
-- and scanning and ingest can rebuild them.

-- 00024_video_successions.sql
-- Succession candidates where the content at a path changed (R-5). Decided and deleted when the
-- recording scan has closed and the new content's probe has finished.
create table video_successions (
    new_key         text    primary key,
    old_key         text    not null,
    old_duration_ms integer not null,
    -- 1 once a scan closes with done after the record. While 0 the previous content may still appear at another path, so no decision is made.
    ready           integer not null default 0 check (ready in (0, 1)),
    created_at      integer not null
) without rowid;

-- 00025_video_fingerprints.sql
-- Footage fingerprint (R-6). hashes holds 9 bytes per frame (1 flag byte + 8-byte big-endian hash).
create table video_fingerprints (
    content_key text    primary key,
    version     integer not null,   -- domain.FingerprintVersion
    interval_ms integer not null,   -- sprite layout interval when built; used to align frames by time
    hashes      blob    not null,
    updated_at  integer not null
) without rowid;

-- 00026_video_version_candidates.sql
-- "Possibly the same video" candidates (R-7). key_a < key_b.
create table video_version_candidates (
    key_a      text    not null,
    key_b      text    not null,
    distance   integer not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;
```

`00025_video_fingerprints.sql` は続けて、`00014` と同じ手順で `jobs` を作り直し、`kind` の check に `'fingerprint'` を加える。スプライトのできた動画 (`seek_thumbnail_state = 'done'` で登録した場所がある) に、`00015` がジョブを積むのと同じ方法で `fingerprint` ジョブを積む。その Down はそのジョブを削除し、`jobs` を戻し、`video_fingerprints` を削除する。

不変条件 (`internal/store/invariants_test.go` に加える):

- `video_bundles.representative_key` は、そのまとまりの `video_bundle_members` にある。
- メンバーが 2 未満のまとまりはない (書き手が解散する)。
- `user_key` は `bundle:` で始まる。
- `video_bundle_members.content_key` は空でない。
- `video_version_candidates` の組は `video_version_dismissals` になく、同じまとまりの 2 本でもない。

分類: `video_bundles`、`video_bundle_members`、`video_version_dismissals` は作り直せない利用者のデータである (ARCHITECTURE.md の一覧に加える)。`video_successions`、`video_fingerprints`、`video_version_candidates` は索引のデータである。内容をどこも参照しなくなったとき (`UpsertVideo` での鍵の付け替え、`DeleteVideos`、`deleteOrphanVideos`、メディアフォルダの削除や置き換え)、動画の行を削除するのと同じトランザクションが、これらのテーブルのその鍵の行を削除する (`releaseContentIndex`)。生成したファイルの後片付け (`RemoveContent`) はテーブルに触れない。

`domain.FolderIndexVersion` は 2 になる ([表示する動画と一覧](#shown-videos-and-listing))。

## `domain` に加える値 {#values-added-to-domain}

| 値 | 内容 |
| --- | --- |
| `Video.UserKey` | 利用者の鍵 ([利用者の鍵](#user-key))。まとまりのメンバーではまとまりの `user_key`、それ以外は `ContentKey` と等しい。store が埋める。`ContentKey` が空の動画では空 |
| `Video.Versions` | `*VideoVersionsRef{Count int, RepresentativeID int64}`。詳細の読み取りだけが、まとまりのメンバーにだけ埋める。`Count` は、閲覧者が見てよい場所を持つメンバーの数 |
| `VideoVersions{RepresentativeID int64, Items []Video}` | まとまりのすべてのバージョン。代表が先で、次にタイトルの自然順 (同じなら id 順) |
| `DurationsMatch(a, b int64) bool` | `abs(a-b) <= max(1000, max(a,b)*0.005)`。長さが 0 以下なら決して一致しない |
| `FingerprintVersion = 1` | 指紋の規則のバージョン |
| `FrameHash{Hash uint64, Flat bool}` | 1 フレームのハッシュと平坦の印 (比較から除く) |
| `Fingerprint{Version int, IntervalMs int64, Frames []FrameHash}` | `Encode() []byte` と `DecodeFingerprint([]byte, version, interval)`。1 フレームあたり 9 バイト |
| `HashFrame(luma [32][32]uint8) FrameHash` | 2 次元 DCT の 8×8 ブロックの 63 個の低周波係数 (DC を除く) をその中央値と比べたビット。輝度の分散が `FingerprintFlatVariance` 未満なら `Flat` |
| `CompareFingerprints(a, b Fingerprint) (distance int, ok bool)` | 下を参照 |
| `FingerprintMatchMaxDistance = 12` | 候補になる中央値の上限 |
| `JobFingerprint JobKind = "fingerprint"` | `JobKinds` の最後。`ClaimConditionFor` は `RegisteredLocation` と新しい `SeekThumbnailFinished` (`seek_thumbnail_state = done`)。`ScanActivityKind` にも加える |
| `VideoBundleChanged{VideoIDs []int64}` | まとまりの変更。`Event` を実装する (R-9) |
| エラー | `ErrNotBundled` (どのまとまりにもない動画の代表を変える、または外す)、`ErrRepresentativeNotSelected` (`representativeId` が `videoIds` にない)、`ErrTooFewVersions` (解決した動画が 2 未満) |

`CompareFingerprints` は、バージョンが違うとき `ok = false` を返す。間隔は一致しなくてよい: `a` のフレーム `i` は、その間隔の中点 (`i*a.IntervalMs + a.IntervalMs/2`) を覆う `b` のフレームと組にする。規則は `b` のレイアウトに対する `FrameAt` と同じである。`b` のフレーム数を超える中点は組にしない。結果は、どちらのフレームも `Flat` でない組のハミング距離の中央値である。405 秒より長い動画の間隔は `ceil(duration / 81)` なので、長さが数ミリ秒違うと間隔が変わる。そのため、フレームは添字ではなく時刻で揃える。比べた組が `FingerprintMinComparableFrames` (3) 未満なら `ok = false` である。

## 利用者の鍵 {#user-key}

動画 `v` の利用者の鍵は、次の式で決まる (`internal/store/user_keys.go` の `userKeyExpr(alias)`)。

```sql
coalesce((select b.user_key from video_bundle_members m join video_bundles b on b.id = m.bundle_id
          where m.content_key = v.content_key), v.content_key)
```

空の `content_key` は、今までどおりどの利用者のデータにも結び付かない (`content_key <> ''` の条件は残る)。

この式を通る読み書き (R-2):

| 経路 | 今 | 変更 |
| --- | --- | --- |
| `videoColumnsTemplate`、`listColumns`、外部 API の列 | `videos.content_key` | `userKeyExpr` も `Video.UserKey` に入れる |
| `publicColumn`、`publicVideoCondition` | `public_videos.content_key = videos.content_key` | 鍵の式で結合する |
| `filteredFrom` と `libraryItemsCTE` の `playback_progress` の結合 | `p.content_key = videos.content_key` | 鍵の式で結合する |
| `videoHasTagCondition`、`tagNameMatchCondition`、タグの数と要約 | `video_tags.content_key = v.content_key` | 鍵の式で結合する。フォルダ名からのタグ (`video_folder_names`、`video_id`) は変わらない |
| `TagsByContentKeys`、`ProgressByContentKeys` | 呼び出し元が `ContentKey` を渡す | `internal/httpapi` が `UserKey` を渡す。手で付けたタグはその鍵で読む。`tagsByContentKeys` の 2 つ目の出どころは `videos.id` を通るので、フォルダ名のタグは引き続き `videos.content_key` から読む |
| `PUT /api/videos/{id}/progress` | `SaveProgress(video.ContentKey)` | `SaveProgress(video.UserKey)`。長さは再生したバージョンのもの |
| `registeredContentKeysForVideoIDs` (タグの追加と削除、タグの要約、公開の切り替え) | `content_key` | `userKeysForVideoIDs` (まとまりごとに 1 つ)。`SetVideosPublic` は、影響したまとまりのすべてのメンバーの `content_key` を返す鍵に含める (ゲストのストリームを切るのは内容ごとだから) |
| `OverrideStore` (表示名、サムネイルの位置) | `content_key` | 変わらない (内容ごと) |
| 生成したファイル、指紋、ライブ変換のプローブのデータ | `content_key` | 変わらない |

## 表示する動画と一覧 {#shown-videos-and-listing}

`shownVideosCTE(audience)` は、閲覧者に表示する動画を `shown(video_id, bundle_id)` として返す (`internal/store/user_keys.go`):

- どのまとまりにもなく、閲覧者が見てよい場所 (`visibleLocationCondition`) を持つ動画。
- 各まとまりの**実効の代表**: `representative_key` の動画が閲覧者の見てよい場所を持てばその動画。そうでなければ、そうした場所を持つメンバーのうち `videos.id` が最も小さいもの。どのメンバーも持たなければ、まとまりは現れない (R-3)。

これを通る読み取り (R-3、R-4):

| 読み取り | 変更 |
| --- | --- |
| `chosenLocationsCTE` | `shown` の動画の場所に範囲を適用し、検索式を `exists (the expression is true on a registered location of some video in the same bundle)` に変える (どのまとまりにもなければ動画自身の場所)。`chosen.path` は、範囲内の動画自身の場所のうち最も小さいパスのままである |
| `libraryItemsCTE`、`LibraryIDs`、`ListVideos`、`ListFolderVideos`、`CountVideos` | `chosen` を通るので従う。タグ、視聴の状態、並び順、`total` は表示する動画の値である |
| フォルダの直下の動画とフォルダの数 (`folders.go` のフォルダ一覧の数) | `shown` の動画の場所だけを数える。代表でないバージョンだけを持つフォルダは、動画のないフォルダである (Edge Case) |
| `RelatedCandidates`、`VideosByIDs` (関連) | `shown` の動画に限る |
| `folderIndexLocations` | 所有者から見た `shown` の動画の場所だけ。`FolderIndexVersion = 2` |
| `GetVideo`、`VideoLocations`、ストリーミング、字幕、`versions` | 変わらない。代表でないバージョンも返す |

ゲストへの見え方はまとまりの鍵 ([利用者の鍵](#user-key)) で決まるので、公開されたまとまりのすべてのバージョンが、ゲストの見てよい場所を持つ。

## 同じパスの内容の引き継ぎ {#carry-over-of-content-at-the-same-path}

`UpsertVideo` (`internal/store/scan_index.go`) で、`locationExists && oldKey != file.ContentKey` のとき、`newVideo` が成り立ち、前の動画の行が最後の場所を失って削除される:

1. 前の行を削除する前に `duration_ms` を読む。null なら何も記録しない (プローブが終わっていないか失敗した動画は対象外。Edge Case)。
2. `(new_key = file.ContentKey, old_key, old_duration_ms, ready = 0)` を `video_successions` に書く (`new_key` の既存の行は置き換える)。
3. `newVideo` が作る鍵が `video_successions.old_key` と等しければ、その行を削除する (前の内容が別のパスへ移った。Edge Case「ファイルの入れ替え」)。

判定は、記録したスキャンがすべてのパスを見るまで待つ。スキャンはパスを順に訪れ、ワーカーはスキャンがまだ動いている間に新しい内容のプローブのジョブを処理するので、前の内容が別のパスで見つかる (手順 3) 前にプローブが終わりうる。その時点で判定すると取り消す記録が残らず、Edge Case「ファイルの入れ替え」の結果が順序に左右される。そのため、2 か所が同じ `applySuccession` を呼び、`ready = 1` の行だけを判定する:

| 呼び出し元 | いつ |
| --- | --- |
| `ScanStore.FinishScan` | スキャンを `state = done` で閉じるトランザクションの中: `video_successions` のすべての行を `ready = 1` にし、`new_key` の動画の `duration_ms` がわかっている各行を判定する。`failed` で閉じるスキャンはすべてのパスを見ていないので、`ready` を変えない (次に `done` で閉じるスキャンが判定する) |
| `ApplyProbe`、`ApplyProbeForJob` | 同じトランザクションの中で、プローブの結果を書いた後 (スキャンが閉じた後にプローブが終わったとき) |

`applySuccession(tx, contentKey, durationMs)`:

1. `new_key = contentKey` で `ready = 1` の行を読み、あれば削除する。なければ止める (`ready = 0` の行は残す)。
2. `DurationsMatch(old_duration_ms, durationMs)` でなければ止める (別の動画で上書きされた。要件 8)。
3. `old_key` を参照する動画があれば止める。
4. 引き継ぐ。`old_key` が `video_bundle_members` にあれば、その `content_key` を `new_key` に付け替え、`representative_key = old_key` なら `new_key` にする。そうでなければ次の行を付け替える: `playback_progress` (`new_key` の行があれば `old_key` の行で置き換える)、`public_videos` (同じ)、`video_tags` (和集合)、`video_version_dismissals` (`old_key` を `new_key` にし、並べ直し、重複は 1 つにまとめる)。まとまりのメンバーでも、これらのテーブルにある `old_key` 自身の行 (まとまりに入る前の値) は同じように付け替える。
5. コミットの後に `VideoBundleChanged` を発行する。`VideoIDs` は引き継ぎを受けた動画と、`old_key` がまとまりのメンバーだったなら、そのまとまりのすべてのメンバーの動画 id (付け替えの後に調べる) である。残りのメンバーの `versions` (代表と数) も変わるので、すべてのメンバーの動画ページが取得し直す ([contracts/screen-api.md、`/api/events`](contracts/screen-api.md#apievents))。

プローブが上限まで失敗すると、行は残り、やり直し (`RetryProbe`) が成功したときに判定する。`new_key` の内容をどこも参照しなくなると、行は削除する ([マイグレーション](#migration))。

## 指紋 {#fingerprints}

`fingerprint` ジョブ (R-6):

1. `ClaimJob` は `seek_thumbnail_state = done` の動画だけを取る。
2. `app.Ingest.Fingerprint` は `JobIdentityCurrent` を確かめ、`ArtifactStore.SeekSprite(contentKey)` でレイアウトを読み、`SeekSpriteSheet` で各シートを読み、それらを `media.SpriteFingerprint(sprite, sheets)` に渡す。スプライトがなければエラーを返す (ジョブはやり直し、上限で `failed` になる。問題は他の段階と同じく記録する)。
3. `media.SpriteFingerprint` は各フレームを切り出し、四辺の黒帯 (平均輝度が 16 未満の行と列) を切り落とし、フレームを 32×32 の輝度に縮め、`domain.HashFrame` の結果から作った `Fingerprint{Version, IntervalMs, Frames}` を返す。
4. `IngestStore.ApplyFingerprintForJob(job, fingerprint)` は `video_fingerprints` の行を置き換え、同じトランザクションで [候補](#candidates) の候補を作り直す。

シーク用サムネイルの完了 (`SetSeekThumbnailStateForJob` の `done`) と、`RequeueMissingSeekThumbnails` での作り直しの完了は、`requeueJob` で `fingerprint` ジョブを積む。指紋は `videos` に状態の列を持たないので、上限まで失敗したジョブ (たとえば生成したファイルを読む一時的な失敗) は、その 2 か所から二度と積まれない。そのため、スキャンが整合を取り戻す: `domain.IndexedVideo` に `FingerprintMissing` (`seek_thumbnail_state = done` で、今の `FingerprintVersion` の `video_fingerprints` の行がない) を加え、それが真なら `Scanner.ensurePendingJobs` が `EnsureJob(JobFingerprint)` を呼ぶ。`fingerprint` では、`EnsureJob` は状態の列の `pending` の代わりにこの条件を使い、`failed` の行を捨ててもう一度積む (積まれたか実行中の行があれば積まない)。こうして失敗した指紋は次のスキャンで作り直され、バージョンを上げてもマイグレーションを待たずに追いつく。古い 6 シートのスプライトから作った指紋もフレームを時刻で揃える ([`domain` に加える値](#values-added-to-domain) の `CompareFingerprints`) ので、今のレイアウトの指紋と比べられる。

## 候補 {#candidates}

`ApplyFingerprintForJob` は、その `content_key` (`K`) の候補を削除し、下の文で作り直す。`vv_fingerprint_distance(a, b)` は、`DecodeFingerprint` を実行して `CompareFingerprints` を呼び、2 つを比べられなければ -1 を返す決定的な関数である。

```sql
insert or ignore into video_version_candidates (key_a, key_b, distance, created_at)
select min(?, f.content_key), max(?, f.content_key), vv_fingerprint_distance(?, f.hashes), ?
  from video_fingerprints f
  join videos v on v.content_key = f.content_key
 where f.content_key <> ? and f.version = ?
   and v.duration_ms is not null and abs(v.duration_ms - ?) <= max(1000, max(v.duration_ms, ?) * 5 / 1000)
   and not exists (select 1 from video_version_dismissals d
                   where d.key_a = min(?, f.content_key) and d.key_b = max(?, f.content_key))
   and not exists (select 1 from video_bundle_members ma join video_bundle_members mb on mb.bundle_id = ma.bundle_id
                   where ma.content_key = ? and mb.content_key = f.content_key)
   and vv_fingerprint_distance(?, f.hashes) between 0 and ?
```

| 出来事 | 候補への影響 |
| --- | --- |
| まとめる | 同じまとまりに入った組の候補を削除する |
| 退ける (`Dismiss`) | `video_version_dismissals` を書き、その組の候補を削除する |
| 内容をどこも参照しなくなる | その鍵の候補を削除する ([マイグレーション](#migration)) |

`Candidates(audience = owner)` は、両方の鍵が登録した場所を持つ動画を持つ組を、`created_at` の新しい順に返す (最大 200。`total` はすべてを数える)。応答の `Video` は、それぞれの鍵の動画である ([contracts/screen-api.md、Candidates](contracts/screen-api.md#candidates))。

## store の操作 (`VersionStore`) {#store-operations-versionstore}

`*DB` を持つ役割 (コミットの後に発行し、フォルダの索引を作り直すため)。各操作は 1 つのトランザクションで動き、途中で失敗しても何も残さない。

| 操作 | 規則 |
| --- | --- |
| `Bundle(videoIDs, representativeID) (VideoVersions, error)` | 下を参照 |
| `MakeRepresentative(videoID) (VideoVersions, error)` | メンバーでなければ `ErrNotBundled`。`representative_key` を変える。値には触れない。`rebuildFolderIndex`、`VideoBundleChanged{every member}` |
| `Unbundle(videoID) (Video, error)` | メンバーでなければ `ErrNotBundled`。メンバーの行を削除する (動画は自分の `content_key` の下の値に戻る)。代表だったなら、残りから実効の代表の規則 ([表示する動画と一覧](#shown-videos-and-listing)) で選んだ 1 本が代表になる。メンバーが 1 本だけ残るなら、まとまりを解散する: まとまりの鍵の下の 3 つのテーブルの行を残った動画の `content_key` に写し (既存の行は置き換える)、まとまりの行を削除する (メンバーは連鎖して消える)。`rebuildFolderIndex`、`VideoBundleChanged{every former member}` |
| `Versions(audience, videoID) (VideoVersions, error)` | 動画を表示できなければ `ErrNotFound`。どのまとまりにもない動画はそれ自身だけを返す。閲覧者が見てよい場所を持つメンバーを、代表を先にして返す |
| `Candidates() (CandidatePage, error)`、`Dismiss(videoIDs [2]int64) error` | [候補](#candidates) |

`Bundle`:

1. id を、今ライブラリにある動画の `content_key` に解決する (解決しない id は `ErrNotFound`)。2 未満なら `ErrTooFewVersions`、代表がその中になければ `ErrRepresentativeNotSelected`。
2. 新しいまとまりを作り、各動画をそこへ移す (すでにまとまりにある動画なら、そのまとまりのすべてのメンバー)。
3. 代表の `UserKey` (代表がまとまりにあればそのまとまりの鍵) の下の `playback_progress`、`video_tags`、`public_videos` の行を、新しい `user_key` に写す。
4. メンバーを移し終えたら、吸収した各まとまりの `video_bundles` の行を削除する (メンバーのないまとまりは残らない。[マイグレーション](#migration) の不変条件)。3 つのテーブルで吸収したまとまりの鍵の下にある値は削除しない (Edge Case「まとまり同士をまとめる」: 値はその鍵の下に残る)。
5. 同じまとまりになった組の候補を削除し、`rebuildFolderIndex` を呼び、`VideoBundleChanged{every member}` を発行する。

`getVideo` (詳細) は `Video.Versions` を埋める。一覧の列にはない。
