# Implementation Plan: 動画詳細画面の情報設計を見直す

**Branch**: `claude/gifted-shannon-vl407s`（feature ブランチ） | **Parent Issue**: #171

**Input**: The parent Issue. It is this feature's specification.

## Summary

動画詳細画面（`/videos/:id`）を、プレイヤーを主役にした画面へ組み直す（親 Issue #171）。
上部バー・タブ・チップ・診断用の項目を外し、題名・属性の一列・ファイルの場所・関連動画の
列・プレイヤー内で完結する状態表示に置き換える。

サーバーには、動画 1 件の応答に所在とシーク用プレビューの状態を足し、次の 3 つの経路を新設する
（[contracts/video-detail-api.md](contracts/video-detail-api.md)）。

- 関連動画
- 読み取りのやり直し
- サーバーの PC でファイルを開く

画面側は、動画 1 件の取得を処理中だけ繰り返すフックと、プレイヤーの上に重ねる状態表示の層を
持つ。プレイヤー自体は video.js の設定で操作を足す。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- API の正本と生成: [api/openapi.yaml](../../api/openapi.yaml)（`task generate`）
- 論理動画と所在、代表の所在の選び方:
  [internal/store/videos.go](../../internal/store/videos.go)（`videoColumnsTemplate`・
  `registeredLocationCondition`・`GetVideo`）
- 取り込みのジョブと重複の防止: [internal/store/jobs.go](../../internal/store/jobs.go)
  （`EnqueueJob`）、一件だけやり直す前例は `RequeuePreviewRepair`
- フォルダ直下の動画と自然順: [internal/store/folders.go](../../internal/store/folders.go)
  （`directVideosCTE`）・[internal/domain/folder.go](../../internal/domain/folder.go)
  （`CompareNatural`）
- シーク用プレビューの生成と置き場: `cmd/mdm/library.go` の `thumbnailHandler`・
  [internal/media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go)
- Web の所有境界と視覚規則: [ARCHITECTURE.md#web-layer](../../ARCHITECTURE.md#web-layer)・
  [library-ui.md](../../docs/design-docs/library-ui.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task test-e2e`）

**Feature-specific context**:

- シーク用プレビューには DB 上の状態が無い。サムネイルのジョブが代表サムネイルを書いて
  `thumbnail_state=done` にしたあと、同じジョブの中で作る。完成は置き場のディレクトリが
  あることで判る。この差は Structural Decisions 3 で扱う。
- 読み取りに失敗した動画は、スキャンでは再試行されない（`ensurePendingJobs` は `pending`
  だけを見る）。そのため要件 13 の「もう一度読み取る」には新しい入口が要る。
- コンテナで動かすとき、サーバーが見るパスはコンテナ内のパス（`/media/...`）である。
  ホスト側のパスは知らない（`MDM_MEDIA_HOST_DIR` は compose だけが読む）。画面に出す場所は
  サーバーが見ているパスのままにする。
- 規模の前提は既存と同じ 1 万件。1 つのフォルダに数千本あっても、関連動画は 1 回の要求で
  直下の所在を 1 度読むだけで済む形にする（Structural Decisions 2）。
- マイグレーション・新しい依存・新しい設定値は無い。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - 関連動画の並べ方（同じフォルダの自然順での後続・先行、追加日時の近さで補う）は、
    `internal/domain` の純粋な関数に置く。
  - `internal/store` は行を読むだけにする。`internal/httpapi` は自分が使う操作を
    インターフェースで宣言する。
  - 新しいパッケージ `internal/opener` も、`cmd/mdm` から `internal/httpapi` へ
    インターフェースとして渡す。
- **外部プロセスの閉じ込め**（ARCHITECTURE.md・`internal/media/probe.go` の「os/exec はこの
  パッケージの外へ漏らさない」）: 条件付き合格。
  - OS の既定アプリを起動する `os/exec` は ffmpeg とは別の責務なので、`internal/media` には
    置かず、新しいパッケージ `internal/opener` に閉じ込める（Structural Decisions 4）。
  - ARCHITECTURE.md の「`os/exec` を使う場所」の記述を同じ単位で更新する。
- **API の正本**（AGENTS.md）: 合格。経路と型は `api/openapi.yaml` に足して `task generate` で
  生成する。生成物は手で編集しない。
- **データの扱い**（ARCHITECTURE.md の、再構築できる索引と利用者データの区別）: 合格。
  - 読み取りのやり直しが触るのは、再構築できる `videos.probe_state`・`probe_error` と
    `jobs` だけである。
  - `playback_progress` には触れない。テーブル・マイグレーションは足さない。
- **UI の正本**（library-ui.md 1・4・5）: 合格。
  - 色と大きさは `web/src/index.css` のトークンだけを使う。
  - 幅とポインターの種類による出し分けは CSS だけで行う（タッチ用の中央操作は
    `pointer: coarse`）。
  - 構図は人が実機で確かめ、PR に画像を添える。
- **裏側の無い入口を増やさない**（library-ui.md 7）: 合格。
  - ファイルの場所は、開けない環境では押せる見た目にしない（要件 18）。
  - 「もう一度読み取る」は裏側の経路と同時に足す。

Phase 1 のあとも判定は同じで、正当化の要る違反は無い。

## Structural Decisions

1. **所在とシーク用プレビューの状態は、既存の `Video` に任意項目として足す。一覧の応答には
   入れず、動画 1 件の応答にだけ入れる。**
   - `location`（`path`・`openable`）と `seekThumbnailState` を足す。
   - 却下: `VideoDetail` を別の型にする案。画面の控え・関連動画・一覧はどれも `Video` を
     使っており、型の変換が Go と TypeScript の両方に増える。
   - 却下: 一覧の各項目にも所在を載せる案。1 ページ 60 件ぶん、画面が使わない絶対パスを
     毎回送ることになる。
2. **関連動画はサーバーで並べ、1 回の要求で返す。**
   - store は次の 3 つを読む。
     - 代表の所在のディレクトリ直下にある動画（`directVideosCTE` と同じ、動画ごとに最小の
       パス）を id とパスだけで全件
     - 追加日時の前後それぞれ最大 20 件（`videos_added_at_desc_idx` を両方向に使う）
     - 選ばれた動画の本体
   - 並べ方は `internal/domain` の関数が決める。ファイル名の `CompareNatural` で、後続・
     先行・追加日時の差の小さい順にする。追加日時の差が同じときは id の大きい方を先にする。
   - 却下: 画面がフォルダ閲覧 API（`/api/folders/{rootId}/videos`）を使って自分で並べる案。
     - フォルダ閲覧 API のページ順は追加日時か題名で、自然順ではない。そのためフォルダの
       全ページを読むことになる。
     - 代表の所在から `rootId` と相対パスへの変換も画面に要る。
   - 却下: 自然順を SQL で書く案。数字の並びを数値として比べる比較は SQLite に無い。
3. **シーク用プレビューの状態は、応答を作るときに導く。**
   - 状態は次のとおり。
     - `done`：置き場のディレクトリが存在する
     - `failed`：その動画のサムネイルのジョブが `failed` である
     - `pending`：それ以外
   - 読み取りが終わっていて、長さが正の動画にだけ返す。
   - 却下: `videos` に列を足してジョブで更新する案。マイグレーションと、生成物の欠落を
     直す既存の修復処理との整合が要る。そこまでして得られるのは、画面の 1 行の表示だけである。
4. **ファイルを開く処理は `internal/opener` に置く。起動できる環境かどうかは、起動時に 1 度
   決める。**
   - 使うコマンドは OS ごとに決まっている。
     - Windows：`explorer.exe`
     - macOS：`open`
     - それ以外：`xdg-open`
   - Linux 等では、それに加えて `DISPLAY` か `WAYLAND_DISPLAY` があるときだけ
     「開ける」とする。これでコンテナや画面の無いサーバーでは自動的に開けない扱いになる。
   - パスはシェルを通さず、引数として渡す。起動した子プロセスは待ち合わせて回収し、
     応答は起動に成功した時点で返す。
   - 却下: `internal/media` に置く案。`media` は ffmpeg と ffprobe の入口で、起動時の
     preflight もそれを前提にしている。
   - 却下: 設定値（`MDM_*`）で有効にする案。要件 18 は「開けない環境では押せる見た目に
     しない」ことを求めている。既定値を誤ると、押しても何も起きないリンクが出る。
5. **ファイルを開けるのは、要求がループバックから来たときだけにする。**
   - `openable` と `POST /api/videos/{id}/open` は、`RemoteAddr` がループバックのときだけ
     真・成功になる。
   - 開くのは、サーバーが持っているその動画の代表の所在だけである。要求からパスは受け取らない。
   - 却下: 起動できる環境なら誰の要求でも開く案。認証が無いので、同じネットワークの誰でも
     サーバーの PC でアプリを起動できてしまう。要件 18 の「サーバーとブラウザが同じ PC」
     とも一致しない。
   - 逆プロキシを挟んだ構成では、要求がループバックに見えることがある。`X-Forwarded-For`
     は信用しない。この構成については quickstart で注意する。
6. **読み取りのやり直しは、`probe_state=failed` の動画にだけ受け付ける。1 つの取引の中で
   状態を `pending` に戻し、ジョブを積む。**
   - `RequeuePreviewRepair` と同じ形にする。
   - 失敗していない動画への要求は 409 `probe_not_failed` とする。連打や別タブからの二度目は
     これになり、ジョブは重複しない（Edge Case）。
   - 却下: 状態を問わず積み直す案。読み取り済みの動画を押すたびに、サムネイルとプレビューの
     作り直しまで連鎖する。
7. **画面は動画 1 件を `web/src/api/` のフックで取り、処理中だけ 2 秒ごとに取り直す。**
   - 「処理中」とは、`probeState`・`thumbnailState`・`seekThumbnailState`・`previewState` の
     どれかが `pending` のときである。
   - 間隔は `ScanProvider` と同じ 2 秒にする。ページが隠れている間は止める。
   - プレイヤーは、再生に関わる値（id・`probeState`・`durationMs`・`playable`）が変わった
     ときだけ作り直す。取り直しのたびに再生が途切れないようにするためである。
   - 却下: サーバーからの push（SSE 等）。この仕組みはリポジトリにまだ無く、他の画面も
     ポーリングである。
8. **状態表示・再生終了・タッチ用の中央操作は、React の層としてプレイヤーの上に重ねる。
   video.js の中に入れるのは、操作バーの部品だけにする。**
   - 操作バーには、10 秒送りのボタン（`controlBar.skipButtons`）・再生速度
     （`playbackRates`）・「変換して再生中」の表示を入れる。
   - 変換中の表示は、シーク位置サムネイルと同じく DOM を操作バーへ差し込む。
   - 却下: 状態表示を video.js の部品として書く案。取り込みの段階表示や関連動画を
     video.js の部品の中で描くことになり、画面の他の部分と React の状態を共有できない。
9. **キーボード操作は、画面全体の `keydown` で受ける。**
   - 次のときは処理しない。
     - 入力欄が選ばれているとき
     - 修飾キーが押されているとき
     - ボタンやリンクの上で Space が押されたとき（そのボタンやリンク自身の操作になる）
   - Esc は、全画面中なら何もしない。全画面の解除はブラウザが行う。
   - 却下: video.js の `userActions.hotkeys`。プレイヤーにフォーカスがあるときしか効かない。
     関連動画を眺めたあとなどに効かなくなる。
10. **関連動画から移るときは、最初の戻り先を引き継ぐ。**
    - 関連動画と「次を再生」のリンクは、`state.from` に今の戻り先を渡す。
    - これで × は、最初に開いた一覧（フォルダ画面を含む）へ戻る（Edge Case）。

## Project Structure

### Documentation (this feature)

```text
specs/012-video-detail-ia/
├── plan.md
├── quickstart.md
└── contracts/
    └── video-detail-api.md
```

- `data-model.md` は作らない。新しく保存するエンティティが無いからである。
- `research.md` も作らない。判断は上の Structural Decisions で閉じている。
- `ui-design.md` は次の Design 工程で足す。
- この workflow は `tasks.md` を作らず、下の Implementation Work を子 Issue にする。

### Source Code

**Affected boundaries**:

- `api/openapi.yaml` と生成物: `Video` の任意項目 2 つと、3 つの経路・`RelatedVideos` 型・
  `Error.code` の 3 つの種別
- `internal/domain/`: 関連動画の並べ方
- `internal/store/`: フォルダ直下の動画の id とパス、追加日時の前後、読み取りのやり直し、
  サムネイルのジョブの状態
- `internal/httpapi/`: 動画 1 件の応答の拡張と 3 つの経路
- `internal/opener/`（新設）: OS の既定アプリの起動と、起動できる環境かどうかの判定
- `cmd/mdm/`: `opener` と新しい store の操作を `httpapi` へ渡す
- `web/src/api/`: 動画 1 件の取得と取り直しのフック、新しい経路の呼び出し
- `web/src/player/`: 画面の構成・プレイヤーの操作・状態表示・関連動画・再生終了
- `web/e2e/playback.e2e.ts`: 新しい構成・キーボード・関連動画の実ブラウザ検証
- `ARCHITECTURE.md`・`docs/design-docs/library-ui.md`: `os/exec` の置き場、公開する API、
  再生画面の構成

**New paths**:

- `internal/opener/`（`opener.go` と対応 test）
- `internal/domain/related.go` と対応 test
- `internal/httpapi/related.go`・`open.go`・`reprobe.go` と対応 test
- `web/src/api/useVideoDetail.ts` と対応 test
- `web/src/player/` の新しい部品（属性の一列、関連動画の列、状態表示の層、再生終了）と
  対応 test

**削除**: `web/src/player/VideoHeader.tsx`、`web/src/player/FileDetails.tsx` とその test。

## Implementation Work

### 動画 1 件の応答に所在とシーク用プレビューの状態を足す

**Scope**:
- [contracts/video-detail-api.md](contracts/video-detail-api.md)「`Video` の追加項目」の
  うち、`location.path` と `seekThumbnailState` を実装する（Structural Decisions 1・3）。
- この単位では `location.openable` は常に `false` とする。
- `api/openapi.yaml` を更新して生成する。
- 画面は変えない。

**Dependencies**: なし。

**Acceptance**:
- httpapi の test で次を確かめる。
  - `GET /api/videos/{id}` が代表の所在の絶対パスを返す。
  - `GET /api/videos` の項目には `location` が無い。
  - シーク用プレビューの状態が、置き場あり・ジョブ失敗・それ以外でそれぞれ `done`・`failed`・
    `pending` になる。
  - 読み取り前の動画には `seekThumbnailState` が無い。
- `task check` が成功する。

### サーバーの PC で動画ファイルを開く API を足す

**Scope**:
- `internal/opener` を新設する（Structural Decisions 4）。
- `POST /api/videos/{id}/open` と `location.openable` を実装する
  （Structural Decisions 5、[contracts](contracts/video-detail-api.md)「ファイルを開く」）。
- `cmd/mdm` で組み立てる。
- ARCHITECTURE.md の `os/exec` の置き場と API の記述を更新する。

**Dependencies**: 動画 1 件の応答に所在とシーク用プレビューの状態を足す。

**Acceptance**:
- opener の test で、OS と環境変数の組ごとに「開ける／開けない」とコマンドが決まる。
- httpapi の test で次を確かめる。
  - ループバックの要求だけが 204 と `openable: true` を得る。
  - ループバック以外の要求は 403 になる。
  - 開けない環境では 409 `open_unavailable` になる。
  - ファイルが無いときは 409 `file_missing` になる。
  - 知らない id は 404 になる。
  - opener には、要求の内容に関わらず代表の所在だけが渡る。
- `task check` が成功する。

### 関連動画 API を足す

**Scope**:
- 関連動画の並べ方を `internal/domain` に置く（Structural Decisions 2）。
- store の読み出しを足す。
- `GET /api/videos/{id}/related` を実装する（[contracts](contracts/video-detail-api.md)
  「関連動画」）。

**Dependencies**: なし。

**Acceptance**:
- domain の test で次を確かめる。
  - `#2`・`#10`・`#9` の並びが自然順になる。
  - 後続が先行より前に来る。
  - 同じフォルダで足りない分を、追加日時の差の小さい順で補う。
  - 自分自身は含めない。
  - 最大 20 件である。
- store・httpapi の test で次を確かめる。
  - 別フォルダの動画が「同じフォルダ」に混ざらない。
  - 同じ動画が同じフォルダに 2 つの所在を持っても 1 件になる。
  - 知らない id は 404 になる。
  - `nextId` が同じフォルダの後続の先頭を指し、後続が無いときは省かれる。
- `task check` が成功する。

### 読み取りに失敗した動画を読み取り直す API を足す

**Scope**:
- `POST /api/videos/{id}/probe` を実装する（Structural Decisions 6、
  [contracts](contracts/video-detail-api.md)「読み取りのやり直し」）。
- store に、状態を戻すことと積むことを 1 つの取引で行う操作を足す。

**Dependencies**: なし。

**Acceptance**:
- store・httpapi の test で次を確かめる。
  - 失敗した動画への要求で `probeState` が `pending` になり、読み取りのジョブがちょうど
    1 件積まれる。
  - 続けて 2 回送ると、2 回目は 409 `probe_not_failed` で、ジョブは増えない。
  - 読み取り済みの動画は 409 になる。
  - 知らない id は 404 になる。
- `task check` が成功する。

### 動画詳細画面を題名・属性・場所・閉じる操作の構成に組み直す

**Scope**:
- 親 Issue の要件 1〜5・17・18 と `ui-design.md` に従い、次を行う。
  - 上部バー・チップ・タブ・`VideoHeader`・`FileDetails` を外す。
  - 2 列と縦積みの構成、題名、区切り線の下の属性の一列、ファイルの場所
    （開けるときだけリンク）、右上の ×、Esc で閉じる操作を実装する。
- 動画 1 件の取得を `web/src/api/useVideoDetail.ts` に移す（この単位では取り直しをしない）。
- `docs/design-docs/library-ui.md` に再生画面の構成の節を足し、ARCHITECTURE.md の Web layer の
  再生画面の記述を更新する。

**Dependencies**: サーバーの PC で動画ファイルを開く API を足す。

**Acceptance**:
- 単体 test で次を確かめる。
  - 廃止した項目が出ない。
  - 属性の英語ラベルと値の形（`H.264`・未再生の `—`）が正しい。
  - `openable` が偽のとき場所がリンクにならない。
  - × と Esc で `state.from` へ戻る。
  - 全画面中の Esc では閉じない。
- `task check` が成功する。
- 画面が変わる単位なので、PR に 360px・768px・1280px の画像と、`ui-design.md` の観点による
  見た目・キーボード操作・支援技術の確認結果を添える。

### 動画詳細画面に関連動画の列を足す

**Scope**:
- 親 Issue の要件 15・16 に従い、関連動画の列（右の列・狭い画面では下）を実装する。
  - 各項目はサムネイル・長さ・題名・途中までの進捗バーとする。
  - 関連動画が無いときは見出しごと出さない。
  - リンクで戻り先を引き継ぐ（Structural Decisions 10）。
- `playback.e2e.ts` に、関連動画から移って × で最初の一覧へ戻る検証を足す。

**Dependencies**: 関連動画 API を足す、動画詳細画面を題名・属性・場所・閉じる操作の構成に
組み直す。

**Acceptance**:
- 単体 test で次を確かめる。
  - 項目に追加日時などの文字が出ない。
  - 途中まで見た動画だけに進捗バーが出る。
  - 関連動画から移ったあとの × が最初の一覧へ戻る。
- e2e が成功する。
- `task check` が成功する。
- 画面が変わる単位なので、PR に画像と確認結果を添える。

### プレイヤーに再生速度・10 秒送り・キーボード操作を足す

**Scope**:
- 親 Issue の要件 6〜9 に従い、次を行う。
  - video.js に再生速度と 10 秒送りを足し、残り時間を外す。
  - 現在時刻/長さを出す。
  - 画面全体のキーボード操作を足す（Structural Decisions 9）。
  - タッチ端末向けの中央操作を足す（Structural Decisions 8）。
- `playback.e2e.ts` にキーボード操作の検証を足す。

**Dependencies**: 動画詳細画面を題名・属性・場所・閉じる操作の構成に組み直す。

**Acceptance**:
- 単体 test で次を確かめる。
  - Space・←/→・F・M・0 がそれぞれの操作を起こす。
  - ボタン上の Space や入力欄では起きない。
- e2e で次を確かめる。
  - → で再生位置が 10 秒進む。
  - 再生速度の変更が反映される。
- `task check` が成功する。
- 画面が変わる単位なので、PR に画像（タッチ用の中央操作を含む）と確認結果を添える。

### プレイヤーの中に読み込み・変換・失敗・取り込み中の状態を出す

**Scope**:
- 親 Issue の要件 10〜13 に従い、次を行う。
  - プレイヤーの上に状態表示の層を置く。
    - 読み込み中
    - 変換して再生中（操作バーの中）
    - 再生失敗と、その位置からの再試行
    - 取り込み中の段階表示
    - 読み取り失敗と「もう一度読み取る」「ファイルを開く」
  - プレイヤー直下に作成中の 1 行を出す。
- `useVideoDetail` に処理中だけの取り直しを足し、プレイヤーを作り直す条件を限る
  （Structural Decisions 7）。
- 既存のプレイヤー下の赤帯と `Blocked` を置き換える。

**Dependencies**:
- 動画 1 件の応答に所在とシーク用プレビューの状態を足す
- 読み取りに失敗した動画を読み取り直す API を足す
- 動画詳細画面を題名・属性・場所・閉じる操作の構成に組み直す

**Acceptance**:
- 単体 test で次を確かめる。
  - 各 `probeState`・`thumbnailState`・`seekThumbnailState`・`previewState` の組で、正しい
    段階と 1 行の表示が出る。
  - `pending` から `done` への変化で再生できるようになる。
  - 取り直しで `thumbnailState` だけが変わってもプレイヤーが作り直されない。
  - 「もう一度読み取る」の 409 でも段階表示へ移る。
  - 再生失敗の再試行が失敗した位置から始まる。
- `task check` が成功する。
- 画面が変わる単位なので、PR に各状態の画像と確認結果を添える。

### 再生終了時に次の動画を提示する

**Scope**:
- 親 Issue の要件 14 に従い、再生終了の層を実装する。
  - 関連動画の応答の `nextId` が指す動画を「次の動画」として出す。
  - 「次を再生」「もう一度見る」を置き、自動では再生しない。
  - 次が無いときは「もう一度見る」だけにする。
- 「次を再生」は戻り先を引き継ぎ、移った先で再生を始める。

**Dependencies**: 動画詳細画面に関連動画の列を足す。

**Acceptance**:
- 単体 test で次を確かめる。
  - `ended` で層が出る。
  - 同じフォルダの後続が無いとき「次を再生」が出ない。
  - 「もう一度見る」で先頭から再生される。
  - 「次を再生」で次の動画の画面へ戻り先付きで移る。
- `task check` が成功する。
- 画面が変わる単位なので、PR に画像と確認結果を添える。
