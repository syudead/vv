# Research: 動画の更新日時とファイルの作成日時

親 Issue: #630。技術スタック・境界・依存方向・索引と利用者データの区分は
[ARCHITECTURE.md](../../ARCHITECTURE.md) と
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) が正本で、
ここでは変えない。利用者データを内容の識別子に結ぶ規則は
[specs/029-video-overrides/research.md R-1](../029-video-overrides/research.md)、集まり（バージョン）の
利用者データの鍵は [specs/030-video-versions/data-model.md §3](../030-video-versions/data-model.md#3-利用者データの鍵)
にある。ここには、この feature が足す決定だけを書く。

## R-1: 更新日時は内容の識別子に結ぶ利用者データの表 `video_edits` に持ち、読み出しで追加日時に倒す

- **Decision**: `video_edits(content_key primary key, edited_at)` を足す。動画を返す読み出しは
  `coalesce(video_edits.edited_at, videos.added_at)` を `Video.EditedAt` に載せる。行が無い動画の更新日時は
  追加日時であり、既存の動画の埋め戻しは要らない（要件 2）。同じパスの中身の引き継ぎ
  （`moveUserData`）はこの表の行も付け替える（Edge Case「新しいバージョンへ引き継がれた動画」）。
- **Rationale**: 更新日時は所有者の操作の記録で、スキャンでは作り直せない利用者データである。
  `playback_progress`・`public_videos`・`video_overrides` と同じく内容の識別子に結べば、再スキャン・移動・
  改名を越えて残り、既存の引き継ぎの仕組み（`moveUserData` の表の一覧に 1 つ足す）にそのまま乗る。
- **Alternatives considered**:
  - `videos.edited_at` の列。`videos` は作り直せる索引で、中身が変わると行ごと消える。引き継ぎは消えた
    行の値を `video_successions` に写して持ち回る必要があり、ARCHITECTURE.md の区分（索引と利用者データ）
    も崩れる。却下。
  - 利用者データの鍵（集まりなら `bundle:<id>`）に結ぶ。初期値が各動画の追加日時で、集まりのメンバーは
    追加日時が違うので、集まりで 1 つの値にできない。却下。集まりの値（タグ・公開）を変えたときは、
    その鍵が指す全メンバーの内容の識別子に書く（`VisibilityStore.SetVideosPublic` が内容ごとの鍵を
    返すのと同じ広げ方、`contentKeysForUserKeys`）。

## R-2: 更新日時を進めるのは動画の情報を書く 4 種の操作だけで、タグ自体・集まり・取り込みでは進めない

- **Decision**: 進めるのは次の書き込みだけである。
  - 表示名: `OverrideStore.SetDisplayName`・`SetDisplayNames`
  - 代表サムネイルの位置: `OverrideStore.SetThumbnailPosition`（指定と解除）
  - 公開の設定: `VisibilityStore.SetVideosPublic`
  - 手で付けるタグの付け外し: `TagStore.AttachTagByID`・`AttachTagByName`・`DetachTag`・
    `ApplyVideoTags`（外部連携 API と MCP の一括）

  進めないのは、再生位置（`PlaybackStore`）、スキャン・解析・サムネイル・プレビュー・指紋の取り込み
  （`ScanIndexStore`・`IngestStore`）、タグ自体の操作（作成・改名・削除・統合・シノニム・仮の確定と却下。
  `video_tags` の行が連鎖して変わっても進めない）、フォルダ名由来のタグの変化（フォルダの索引の作り直し、
  グループのタグ化）、バージョンの束ね・代表の変更・外す（`VersionStore`）、同じパスの引き継ぎである。
- **Rationale**: 親 Issue の要件 1 は「所有者が動画の情報を変えたとき」であり、受け入れ条件 1 の 4 つが
  それである。タグの削除や統合は 1 回の操作で何千本もの動画の `video_tags` を変えるので、進めると
  「この動画を最後にいつ直したか」が分からなくなる。束ねる操作は値を複製する構造の操作で、動画の
  情報を書き換えない。フォルダ名由来のタグは走査が付け、所有者の編集ではない。
- **Alternatives considered**:
  - `video_tags`・`public_videos`・`video_overrides` の変化すべてを進める（トリガーで一律に）。上の理由で
    タグの削除・統合・引き継ぎ・束ねでも進んでしまう。却下。

## R-3: 変わらなかった編集は進めず、進める対象は書き込みが実際に変えた内容の識別子だけにする

- **Decision**: 各操作は、書き込みの中で実際に行が変わった利用者データの鍵を集め、その鍵が指す内容の
  識別子（集まりなら全メンバー）にだけ `touchEditedAt`（[data-model.md §3](data-model.md#3-更新日時を進める規則)）を
  書く。変わったかどうかは次で決める。
  - `video_tags`・`public_videos`: `insert or ignore` と `delete` の `RowsAffected`（1 鍵ずつ書く経路）、
    json_each で集合に書く `applyManualTags` は `returning content_key` で変わった鍵を受け取る
    （modernc.org/sqlite は SQLite 3.35 以降で `RETURNING` を持つ）。
  - 表示名: 取引の初めに対象の今の `display_name` を読み、取引の最後に残る名前（`SetDisplayNames` の一括で
    同じ内容の識別子を何度か書いたときは最後の名前）と同じ（未設定どうしを含む）なら進めない。1 回ずつの
    書き込みの前後で比べると、A→B→A の一括で値が変わらないのに進んでしまう。
  - 代表サムネイルの位置: 書く前に今の `thumbnail_position_ms` を読み、同じ（解除どうしを含む）なら
    進めない。画像の作り直し自体はこれまでどおり行う。

  書き込みが失敗して取引が戻れば、同じ取引に書く `video_edits` も戻る（Edge Case「編集が失敗したとき」）。
- **Rationale**: Edge Case「編集の前後で値が変わらなかったときは、更新日時を進めない」と「一括操作では
  変わった動画それぞれ」を満たすには、操作の単位ではなく行の単位で変化を知る必要がある。
- **Alternatives considered**:
  - 操作が成功すれば対象すべてを進める。既に付いているタグを付け直す・同じ表示名を保存するだけで
    進んでしまう。却下。

## R-4: ファイルの作成日時は所在の列 `video_locations.file_created_at` に持ち、取れなければ null にして読み出しで mtime に倒す

- **Decision**: `video_locations` に nullable の `file_created_at`（Unix 秒）を足す。走査が読めた作成日時を
  書き、ファイルシステムが持たなければ null にする（前の走査で入った値も、読めなくなれば null に戻す。
  [R-6](#r-6-登録済みの所在は変わっていないファイルでも作成日時が違えば次の走査で書き直す)）。値は
  `mtime` と同じく秒で持つ。動画を返す読み出しと並べ替えは、一覧に出す所在の
  `coalesce(file_created_at, mtime)` を `Video.FileCreatedAt` と `createdAsc`/`createdDesc` の値にする
  （要件 3・5、Edge Case「既に登録済みの動画」）。
- **Rationale**: 作成日時はファイルの事実なので、`mtime`・`size_bytes` と同じ所在の列である（作り直せる
  索引）。null を保てば、作成日時を持たないファイルシステムから持つものへ移したとき（コピー先が
  ext4 など）に、次の走査が本物の値で埋められる。倒す規則は読み出し側に 1 か所置く。
- **Alternatives considered**:
  - 取れないときに mtime を書く。代用と本物を後から区別できず、取れるようになっても更新されない。却下。
  - `videos` の列。所在が複数ある動画は所在ごとに作成日時が違い、代表が変われば値も変わる
    （Edge Case）。却下。

## R-5: 作成日時はファイルシステムのアダプタ `internal/scanner` が OS ごとに読み、Linux は `golang.org/x/sys/unix` の statx を使う

- **Decision**: `internal/scanner` に `fileCreatedAt(path string, info fs.FileInfo) (time.Time, bool)` を置き、
  build tag で分ける。
  - Linux: `unix.Statx` に `STATX_BTIME` を求め、`Mask` に立っているときだけ返す。
    `golang.org/x/sys` を直接依存にする（今は間接依存）。
  - darwin・freebsd・netbsd: `info.Sys().(*syscall.Stat_t)` の `Birthtimespec`。1 つのファイル
    `file_created_at_bsd.go` に `//go:build darwin || freebsd || netbsd` で置く（`_darwin.go` という名前は
    GOOS の暗黙の制約になり、freebsd・netbsd では読まれない）。
  - windows: `info.Sys().(*syscall.Win32FileAttributeData).CreationTime`。
  - それ以外（`file_created_at_other.go`、上の OS を除く build tag）: 常に取れない。

  読めなかったことは失敗にせず、作成日時無しとして登録する。`domain` はこの関数を知らない。
- **Rationale**: 標準ライブラリの `os.FileInfo` は Linux で作成日時を出さず、vv の主な配置先は
  Linux のコンテナである（ARCHITECTURE.md「Intended topology」）。`x/sys/unix` は既に間接依存に
  あり、ファイルシステムの事実を読むのはアダプタの仕事である（依存方向）。
- **Alternatives considered**:
  - 標準ライブラリだけで済ませ、Linux では常に mtime に倒す。要件 3 の「取れるときは作成日時」が
    主な配置先で成り立たない。却下。
  - `ffprobe` の `creation_time` タグ。コンテナのメタデータで、書き出しの日時とは限らず、無いことも多い。
    ファイルの作成日時という要求と違う。却下。

## R-6: 登録済みの所在は、変わっていないファイルでも作成日時が違えば次の走査で書き直す

- **Decision**: 走査は変わっていないファイル（大きさと mtime が同じ）でも作成日時を読み、索引の値
  （`IndexedVideo.FileCreatedAt`。無ければゼロ値）と違えば `Index.UpdateLocationCreatedAt(ctx, locationID,
  createdAt)` で所在の列だけを書く。中身の識別子は計算し直さず、job も積まず、`videos.updated_at` も
  イベントも動かさない。中身が変わったファイルは `UpsertVideo` が `VideoFile.FileCreatedAt` を他の
  事実と一緒に書く（Edge Case「同じパスで差し替えられて作成日時が変わった」）。比べるのは `mtime` と同じく
  秒で、読めなかったときはゼロ値として比べる（索引に値があれば null に戻す、R-4）。
- **Rationale**: 既存の動画に作成日時を入れるのは「次のスキャンで」（Edge Case）であり、メディア
  フォルダを歩くのは利用者が始めた走査だけである（ARCHITECTURE.md）。費用: darwin・BSD・Windows は
  走査が既に読んだ `entry.Info()` から取れるので増えない。Linux は `entry.Info()`（lstat）が作成日時を
  持たないので、メディアファイルごとに `statx` を 1 回足す。中身は読まないメタデータの問い合わせだが、
  ネットワークのマウントでは 1 ファイルあたりの往復が増える。要件 3 と Edge Case「既に登録済みの動画」は
  変わっていないファイルにも作成日時を求めるので、この費用を受け入れる。
- **Alternatives considered**:
  - 起動時に全所在を `stat` して埋める。利用者が始めた走査以外でメディアフォルダに触れる経路を
    1 つ足す。却下。
  - Linux で索引に値がある変わっていないファイルは `statx` を省く。読めなくなった所在を null に戻せず、
    大きさと mtime を保った差し替え（`cp -p` など）で作成日時が古いまま残る。却下。
  - 作成日時の違いを「変わった」として `UpsertVideo` に通す。中身の識別子（先頭・末尾 1 MiB の
    sha256）を全ファイルで読み直すことになる。却下。

## R-7: 応答の項目は `updatedAt`（vv 上の更新日時）と `fileCreatedAt`（所在の作成日時）で、画面と外部連携で同じ名前にする

- **Decision**: `Video`（画面）と `ExternalVideo`（外部連携）に必須の `updatedAt` と `fileCreatedAt`
  （どちらも date-time）を足す。ゲストの応答にも入れる（追加日と同じく公開の動画の事実で、所有者の
  データを漏らさない）。`VideoSort` に `createdAsc`・`createdDesc` を足し、ゲストにも許す
  （`played*` のように所有者のデータに依らない）。`modifiedAsc`・`modifiedDesc` は名前も値も変えない
  （要件 7）。`domain.Video` の項目は、既にある `UpdatedAt`（`videos.updated_at`、取り込みの行の更新）と
  取り違えないよう `EditedAt` にする。
- **Rationale**: 親 Issue の語は「更新日時」と「作成日時」で、`updatedAt` はそのまま。`createdAt` は
  API の利用者が行の作成日時と読むので、ファイルの事実であることを名前に出す。
- **Alternatives considered**:
  - `editedAt`。親 Issue と画面の語（更新日時）から離れる。却下。
  - `createdAt`。`VideoLocation.createdAt`（所在の行の作成）と同じ名前で意味が違う。却下。
  - 所在ごとの `ExternalVideoLocation.createdAt`。要件 6 は動画の情報に含めることで、一覧に出す所在の
    値で足りる。所在ごとの値は対象外のまま。却下。

## R-8: 再生画面は、タグと公開の設定を変えたあと動画を取り直し、新しいドメインイベントは足さない

- **Decision**: `VideoTags` と `VisibilitySwitch` に成功時の `onChanged` を足し、`VideoPage` が
  `useVideoDetail` の `refresh` で動画を取り直す（表示名と代表サムネイルは、応答の `Video` を `replace`
  する今の経路で `updatedAt` も入れ替わる）。`TagStore`・`VisibilityStore` はイベントを発行しない
  まま。
- **Rationale**: 受け入れ条件 1 は再生画面で更新日時が進んで見えること。進んだ値を知るのは
  その操作をした画面だけで、1 回の `GET /api/videos/{id}` で足りる。ARCHITECTURE.md は
  「タグの変更は副作用を持たないのでイベントを発行しない」と定め、`TagStore` は通知に依存しない。
- **Alternatives considered**:
  - `VideoOverrideChanged` のようなイベントを `TagStore`・`VisibilityStore` から発行し、`/api/events`
    の `video` に写す。上の設計を崩し、一覧の全カードがタグの付け外しのたびに取り直す。却下。
  - タグの付け外しの応答を `Video` に変える。`POST /api/video-tags` は複数の動画への一括操作で、
    応答の形を変える理由にならない。却下。
