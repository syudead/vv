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
  - 次の記述は、実装の単位の中で更新する。
    - ARCHITECTURE.md「Intended dependency direction」の矢印
      （`internal/{httpapi,store,media,scanner,jobs}` に `opener` を足す）
    - `.golangci.yml` の depguard の説明文（「外部コマンドの実行は internal/media に
      閉じ込めること」）
    - `internal/media/probe.go` のコメント（「os/exec はこのパッケージの外へ漏らさない」）
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
   - 同じフォルダの中は全順序で並べる。`CompareNatural` は `1.mp4` と `01.mp4` や、
     大文字小文字だけが違う名前を同順位（0）にするので、そのときはファイル名のバイト順、
     それでも同じなら id の小さい方を先にする。後続・先行の分け方、並び、`nextId` はすべて
     この順序を使う。同順位を後続にも先行にも入れない、という抜けを作らないためである。
   - 却下: 画面がフォルダ閲覧 API（`/api/folders/{rootId}/videos`）を使って自分で並べる案。
     - フォルダ閲覧 API のページ順は追加日時か題名で、自然順ではない。そのためフォルダの
       全ページを読むことになる。
     - 代表の所在から `rootId` と相対パスへの変換も画面に要る。
   - 却下: 自然順を SQL で書く案。数字の並びを数値として比べる比較は SQLite に無い。
3. **シーク用プレビューの状態は、応答を作るときに導く。**
   - 状態は次のとおり。
     - `done`：置き場のディレクトリが存在する
     - `pending`：置き場が無く、`thumbnail_state=pending` であるか、その動画の
       サムネイルのジョブが `queued`・`running` である
     - `failed`：それ以外。失敗したジョブの行は 7 日で消える（`DeleteFinishedJobsBefore`）
       ので、行が無いことを `pending` と読まない。そう読むと、作成中の 1 行が消えず、
       取り直しも止まらなくなる。
   - 返す条件は、既存の `seekThumbnailUrl` と同じ判定にそろえる（読み取り済み・長さが正・
     映像コーデックあり・内容鍵あり）。
   - 却下: `videos` に列を足してジョブで更新する案。マイグレーションと、生成物の欠落を
     直す既存の修復処理との整合が要る。そこまでして得られるのは、画面の 1 行の表示だけである。
4. **ファイルを開く処理は `internal/opener` に置く。起動できる環境かどうかは、起動時に 1 度
   決める。**
   - 使うコマンドは OS ごとに決まっている。
     - Windows：`explorer.exe`
     - macOS：`open`
     - それ以外：`xdg-open`
   - コマンドは起動時に `exec.LookPath` で実行ファイルへ解決し、解決できたときだけ
     「開ける」とする。解決した実行ファイルのパスを持っておき、要求のたびには探さない。
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
   - さらに `Host` がループバックの名前（`localhost`・`127.0.0.1`・`[::1]`、ポートは問わない）で
     あることも条件にする。既存の POST の同一オリジン確認（`router.go` の
     `mutationBoundary`）は `Origin` と `Host` を比べるだけなので、DNS rebinding で
     `127.0.0.1` を指させた他サイトのページを通してしまう。`Host` の確認でこれを塞ぐ。
   - 開くのは、サーバーが持っているその動画の代表の所在だけである。要求からパスは受け取らない。
   - 誤りの判定順は次のとおり。
     1. 404
     2. 409 `open_unavailable`（サーバー全体で決まる事実で、要求元を問わない）
     3. 403
     4. 409 `file_missing`
   - 却下: 起動できる環境なら誰の要求でも開く案。認証が無いので、同じネットワークの誰でも
     サーバーの PC でアプリを起動できてしまう。要件 18 の「サーバーとブラウザが同じ PC」
     とも一致しない。
   - 逆プロキシを挟んだ構成では、要求がループバックに見えることがある。`X-Forwarded-For`
     は信用しない。この構成については quickstart で注意する。
6. **読み取りのやり直しは、`probe_state=failed` の動画にだけ受け付ける。1 つの取引の中で
   状態を戻し、スキャンが新しい内容に積むのと同じジョブを積む。**
   - `RequeuePreviewRepair` と同じ形にする。`where probe_state='failed'` 付きの更新と、
     既存の終わったジョブ行の削除、`on conflict … do nothing` の挿入を、取引の中で直接書く。
     `EnqueueJob` は取引に参加できないので使わない。
   - 戻す状態と積むジョブは次のとおり。
     - `probe_state`：`pending` に戻し、`probe_error` を消す。`probe` ジョブを積む。
     - `thumbnail_state`：`done` でなければ `pending` に戻し、`thumbnail` ジョブを積む。
       スキャンの `Scanner.enqueue` と同じ組にするためである。`probeHandler` は成功後に
       プレビューしか積まない。読み取りに失敗した動画はたいていサムネイルも失敗しているので、
       これが無いと、やり直しが成功してもサムネイルとシーク用プレビューは作られない。
     - `preview_state`：`failed` なら `pending` に戻す。ジョブは、読み取りの成功後に
       `probeHandler` が積む。
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
   - 変換して再生中（`liveOffset.ts` が再生位置と速度を仲立ちする経路）でも、同じ操作が
     論理上の再生位置に対して効くようにする。
   - 却下: video.js の `userActions.hotkeys`。プレイヤーにフォーカスがあるときしか効かない。
     関連動画を眺めたあとなどに効かなくなる。
10. **関連動画から移るときは、最初の戻り先を引き継ぐ。**
    - 関連動画と「次を再生」のリンクは、`state.from` に今の戻り先を渡す。
    - これで × は、最初に開いた一覧（フォルダ画面を含む）へ戻る（Edge Case）。
11. **生成の `failed` は「終わった」として扱う。**
    - 段階表示の各行は次のように決める。
      - `done`：完了
      - `pending`：未完了の段のうち先頭が「処理中」、残りが「待機中」（ジョブは 1 本ずつ
        順に動くので、先頭だけが動いている）
      - `failed`：「作成できませんでした」
    - 作成中の 1 行（要件 11）は、`pending` が 1 つでもある間だけ出す。取り直しも同じ
      条件で止める（Structural Decisions 7）。
    - 見た目は `ui-design.md` が決める。
    - 却下: `failed` も「作成中」に含める案。再試行されない失敗のために、1 行と
      2 秒ごとの取り直しが画面を開いている限り続く。
12. **プレイヤーの上に重ねる層は、1 つの入れ物に集める。** タッチ用の中央操作・状態表示・
    再生終了は、どれもその入れ物の子として足す。重なりの順（上から、状態表示・再生終了・
    中央操作）は入れ物が決める。
    - 却下: 層ごとにプレイヤーの上へ別々に絶対配置する案。重なりの順とクリックの通り方を
      部品ごとに決めることになり、同時に出たときの振る舞いが定まらない。
13. **読み取りとサムネイルの終端失敗は、ジョブを `failed` にするのと同じ取引で動画側の状態へ
    記録する。** プレビューはすでにこの形である（`FailClaimedJob` の `JobPreview` の分岐）。
    - 今は `probeHandler` と `thumbnailHandler` が、エラーを返す前に自分で
      `probe_state`・`thumbnail_state` を `failed` にしている。そのため次の 2 つのずれが起きる。
      - ファイルの確認（`checkReadableRegularFile`）などの前段で上限まで失敗すると、
        状態は `pending` のまま、ジョブだけが `failed` になる。取り込み中の段階表示と
        2 秒ごとの取り直しが終わらない。
      - 動画側が `failed` になってからジョブが `failed` になるまでの間に「もう一度読み取る」が
        来ると、ジョブがまだ `running` なので新しいジョブの挿入が省かれる。そのあと古い
        ジョブが `failed` になり、動画はジョブの無い `pending` で止まる。
    - `FailClaimedJob` が終端（`state='failed'`）を確定した取引の中で、次のように記録する。
      - `probe`：`probe_state='failed'`、`probe_error` に理由
      - `thumbnail`：`thumbnail_state` が `done` でなければ `failed`
      - 条件は `JobPreview` の分岐と同じく、内容鍵・所在の世代・所在が一致するときに限る。
    - 2 つのハンドラが自分で状態を書く処理は外す。
    - 起動時に、`ReconcilePreviewFailures` と同じ条件で整合を取る。状態が `pending` で、
      進行中のジョブが無く、終端の `failed` のジョブがある動画を `failed` に直す。
      この変更より前に止まってしまった動画のためである。
    - これにより「状態が `failed` なら、その種類のジョブは終わっている」が成り立つ。
      読み取りのやり直し（Structural Decisions 6）はこれを前提にする。
    - 却下: 導き方の側で、終端のジョブを `pending` より優先する案。画面の取り直しは
      止まるが、`probe_state` と `thumbnail_state` が一覧・スキャン・やり直しの判定で
      食い違ったまま残る。
    - 却下: やり直しの側で、`running` のジョブが終わるのを待って積む案。待ち合わせを
      HTTP の要求の中に持ち込むことになり、ずれそのものは残る。

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
  サムネイルのジョブの状態、読み取りとサムネイルの終端失敗の記録と起動時の整合
- `internal/httpapi/`: 動画 1 件の応答の拡張と 3 つの経路
- `internal/opener/`（新設）: OS の既定アプリの起動と、起動できる環境かどうかの判定
- `cmd/mdm/`: `opener` と新しい store の操作を `httpapi` へ渡す。`probeHandler`・
  `thumbnailHandler` が自分で失敗の状態を書く処理を外す
- `web/src/api/`: 動画 1 件の取得と取り直しのフック、新しい経路の呼び出し
- `web/src/player/`: 画面の構成・プレイヤーの操作・状態表示・関連動画・再生終了
- `web/e2e/playback.e2e.ts`: 新しい構成・キーボード・関連動画の実ブラウザ検証
- `ARCHITECTURE.md`・`.golangci.yml`・`internal/media/probe.go` のコメント・
  `docs/design-docs/library-ui.md`: 外部コマンドの置き場、依存の矢印、公開する API、
  再生画面の構成

**New paths**:

- `internal/opener/`（`opener.go` と対応 test）
- `internal/domain/related.go` と対応 test
- `internal/httpapi/related.go`・`open.go`・`reprobe.go` と対応 test
- `web/src/api/useVideoDetail.ts` と対応 test
- `web/src/player/` の新しい部品（属性の一列、関連動画の列、状態表示の層、再生終了）と
  対応 test

**削除**: `web/src/player/VideoHeader.tsx`、`web/src/player/FileDetails.tsx`、
`web/src/player/FileDetails.test.tsx`。

## Implementation Work

実装は 1 つの単位・1 つの PR にまとめる（要求者の判断）。下の太字の区切りは、作業と確認の
まとまりを示す見出しで、子 Issue や PR の単位ではない。

- 却下: API 4・画面 5 の 9 単位に分けて、それぞれを子 Issue と PR にする案。要求者が、この
  機能は PR 1 つで見ることを選んだ。

### 動画詳細画面の情報設計を実装する

**Scope**:

- **動画 1 件の応答に所在とシーク用プレビューの状態を足す**
  - [contracts/video-detail-api.md](contracts/video-detail-api.md)「`Video` の追加項目」の
    うち、`location.path` と `seekThumbnailState` を実装する（Structural Decisions 1・3）。
  - `api/openapi.yaml` を更新して生成する。
- **サーバーの PC で動画ファイルを開く API を足す**
  - `internal/opener` を新設する（Structural Decisions 4）。
  - `POST /api/videos/{id}/open` と `location.openable` を実装する
    （Structural Decisions 5、[contracts](contracts/video-detail-api.md)「ファイルを開く」）。
  - `cmd/mdm` で組み立てる。
  - ARCHITECTURE.md の依存の矢印と API の記述、`.golangci.yml` の depguard の説明文、
    `internal/media/probe.go` のコメントを更新する（Constitution Check「外部プロセスの閉じ込め」）。
- **関連動画 API を足す**
  - 関連動画の並べ方を `internal/domain` に置く（Structural Decisions 2）。
  - store の読み出しを足す。
  - `GET /api/videos/{id}/related` を実装する（[contracts](contracts/video-detail-api.md)
    「関連動画」）。
- **読み取りに失敗した動画を読み取り直す API を足す**
  - 読み取りとサムネイルの終端失敗を、ジョブの失敗と同じ取引で記録するように直す。
    ハンドラが自分で状態を書く処理を外し、起動時の整合を足す（Structural Decisions 13）。
  - `POST /api/videos/{id}/probe` を実装する（Structural Decisions 6、
    [contracts](contracts/video-detail-api.md)「読み取りのやり直し」）。
  - store に、状態を戻すことと積むことを 1 つの取引で行う操作を足す。
- **動画詳細画面を題名・属性・場所・閉じる操作の構成に組み直す**
  - 親 Issue の要件 1〜5・17・18 と `ui-design.md` に従い、次を行う。
    - 上部バー・チップ・タブ・`VideoHeader`・`FileDetails` を外す。
    - 2 列と縦積みの構成、題名、区切り線の下の属性の一列、ファイルの場所
      （開けるときだけリンク）、右上の ×、Esc で閉じる操作を実装する。
    - 長いパスは、広い画面では 1 行で末尾を省略してポイントで全体を示し、狭い画面では
      折り返す（Edge Case「パスが長い」、見た目は `ui-design.md`）。
    - 開こうとして `file_missing` などで失敗したときは、場所の行のすぐ下に開けなかった
      ことを出す。帯やトーストは使わない（Edge Case「ファイルが移動・削除されている」）。
  - `docs/design-docs/library-ui.md` に再生画面の構成の節を足し、ARCHITECTURE.md の Web layer の
    再生画面の記述を更新する。
- **動画詳細画面に関連動画の列を足す**
  - 親 Issue の要件 15・16 に従い、関連動画の列（右の列・狭い画面では下）を実装する。
    - 各項目はサムネイル・長さ・題名・途中までの進捗バーとする。
    - 関連動画が無いときは見出しごと出さない。
    - リンクで戻り先を引き継ぐ（Structural Decisions 10）。
  - `playback.e2e.ts` に、関連動画から移って × で最初の一覧へ戻る検証を足す。
- **プレイヤーに再生速度・10 秒送り・キーボード操作を足す**
  - 親 Issue の要件 6〜9 に従い、次を行う。
    - video.js に再生速度と 10 秒送りを足し、残り時間を外す。
    - 現在時刻/長さを出す。
    - 画面全体のキーボード操作を足す（Structural Decisions 9）。
    - タッチ端末向けの中央操作を足す（Structural Decisions 8）。
    - プレイヤーの上に重ねる層の入れ物を作る（Structural Decisions 12）。
  - `playback.e2e.ts` にキーボード操作の検証を足す。
- **プレイヤーの中に読み込み・変換・失敗・取り込み中の状態を出す**
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
  - 表示中の動画が消えた（取り直しや再生が 404 になった）ときは、プレイヤーの中に
    「開けません」を出し、× で戻れるようにする（Edge Case）。
  - 段階表示と 1 行の出し分けは Structural Decisions 11 に従う。
- **再生終了時に次の動画を提示する**
  - 親 Issue の要件 14 に従い、再生終了の層を実装する。
    - 関連動画の応答の `nextId` が指す動画を「次の動画」として出す。
    - 「次を再生」「もう一度見る」を置き、自動では再生しない。
    - 次が無いときは「もう一度見る」だけにする。
  - 「次を再生」は戻り先を引き継ぎ、移った先で再生を始める。

**Dependencies**: なし。

**Acceptance**:

- **動画 1 件の応答に所在とシーク用プレビューの状態を足す**
  - httpapi の test で次を確かめる。
    - `GET /api/videos/{id}` が代表の所在の絶対パスを返す。
    - `GET /api/videos` の項目には `location` が無い。
    - シーク用プレビューの状態が次のとおりになる。
      - 置き場あり：`done`
      - ジョブが `queued`・`running`：`pending`
      - ジョブが `failed`：`failed`
      - ジョブの行が消えた：`failed`
    - 読み取り前の動画と映像の無い動画には `seekThumbnailState` が無い。
- **サーバーの PC で動画ファイルを開く API を足す**
  - opener の test で、OS・環境変数・コマンドの有無の組ごとに「開ける／開けない」と実行ファイルが
    決まる。画面の環境変数があってもコマンドが見つからなければ「開けない」になる。
  - httpapi の test で次を確かめる。
    - ループバックから、ループバックの `Host` で来た要求だけが 204 と `openable: true` を得る。
    - ループバック以外の要求と、`Host` がループバックの名前でない要求は 403 になる。
    - 開けない環境では、要求元を問わず 409 `open_unavailable` になる。
    - ファイルが無いときは 409 `file_missing` になる。
    - 知らない id は 404 になる。
    - opener には、要求の内容に関わらず代表の所在だけが渡る。
- **関連動画 API を足す**
  - domain の test で次を確かめる。
    - `#2`・`#10`・`#9` の並びが自然順になる。
    - `CompareNatural` が同順位にする名前も全順序で並ぶ。
      - `01.mp4` と `1.mp4` だけのフォルダでは、`01.mp4` の `nextId` が `1.mp4` を指し、
        `1.mp4` では `nextId` が省かれて `01.mp4` が先行に入る。
      - 大文字小文字だけが違う組（`A.mp4` と `a.mp4`）も同じ形で並ぶ。
    - 後続が先行より前に来る。
    - 同じフォルダで足りない分を、追加日時の差の小さい順で補う。
    - 自分自身は含めない。
    - 最大 20 件である。
  - store・httpapi の test で次を確かめる。
    - 別フォルダの動画が「同じフォルダ」に混ざらない。
    - 同じ動画が同じフォルダに 2 つの所在を持っても 1 件になる。
    - 知らない id は 404 になる。
    - `nextId` が同じフォルダの後続の先頭を指し、後続が無いときは省かれる。
- **読み取りに失敗した動画を読み取り直す API を足す**
  - store・httpapi の test で次を確かめる。
    - 失敗した動画への要求で `probeState` が `pending` になり、読み取りのジョブがちょうど
      1 件積まれる。
    - `thumbnail_state=failed` の動画では、`thumbnailState` も `pending` に戻り、サムネイルの
      ジョブが 1 件積まれる。`preview_state=failed` なら `previewState` が `pending` に戻る。
    - 続けて 2 回送ると、2 回目は 409 `probe_not_failed` で、ジョブは増えない。
  - ファイルの確認で上限まで失敗した読み取り・サムネイルのジョブは、同じ取引で
    `probe_state`・`thumbnail_state` を `failed` にする。
  - 最後の試行が失敗した直後（ジョブの失敗が記録される前）には、動画はまだ `failed` に
    ならない。この間の要求は 409 になり、ジョブの無い `pending` は生まれない。
  - 起動時の整合で、`pending` のまま終端の失敗ジョブを持つ動画が `failed` になる。
    - 読み取り済みの動画は 409 になる。
    - 知らない id は 404 になる。
- **動画詳細画面を題名・属性・場所・閉じる操作の構成に組み直す**
  - 単体 test で次を確かめる。
    - 廃止した項目が出ない。
    - 属性の英語ラベルと値の形が正しい。
      - コーデックが `H.264` のように大文字になる。
      - 未再生の LAST PLAYED が `—` になる。
      - 読み取り前の項目が「読み取り中」になる。
      - 読み取り失敗の動画では技術情報が「読み取れませんでした」の 1 項目になる。
    - `openable` が偽のとき場所がリンクにならない。
    - 開く要求が 409 `file_missing` を返したとき、場所の行の下に開けなかったことが出て、
      画面の他の部分は変わらない。
    - × と Esc で `state.from` へ戻る。
    - 全画面中の Esc では閉じない。
- **動画詳細画面に関連動画の列を足す**
  - 単体 test で次を確かめる。
    - 項目に追加日時などの文字が出ない。
    - 途中まで見た動画だけに進捗バーが出る。
    - 関連動画から移ったあとの × が最初の一覧へ戻る。
    - 関連動画が 0 件のとき、見出しは出ず、× は広い画面でも残る。
- **プレイヤーに再生速度・10 秒送り・キーボード操作を足す**
  - 単体 test で次を確かめる。
    - Space・←/→・F・M・0 がそれぞれの操作を起こす。
    - ボタン上の Space や入力欄では起きない。
    - タッチ用の中央の 10 秒戻る/進むを押すと、再生位置が 10 秒動く。
    - 変換して再生する経路でも、→ と再生速度の変更が論理上の再生位置と速度に効く。
  - e2e で次を確かめる。
    - → で再生位置が 10 秒進む。
    - 再生速度の変更が反映される。
- **プレイヤーの中に読み込み・変換・失敗・取り込み中の状態を出す**
  - 単体 test で次を確かめる。
    - 各 `probeState`・`thumbnailState`・`seekThumbnailState`・`previewState` の組で、
      Structural Decisions 11 どおりの段階と 1 行の表示が出る。`failed` だけが残ると 1 行は
      消え、取り直しも止まる。
    - `pending` から `done` への変化で再生できるようになる。
    - 取り直しで `probeState` が `pending` から `failed` に変わると、段階表示から読み取り
      失敗の表示へ切り替わる。
    - 変換して再生する動画で、操作バーに「変換して再生中」が出る。
    - 取り直しが 404 を返すと、プレイヤーの中に「開けません」が出て、× が残る。
    - 取り直しで `thumbnailState` だけが変わってもプレイヤーが作り直されない。
    - 「もう一度読み取る」の 409 でも段階表示へ移る。
    - 再生失敗の再試行が失敗した位置から始まる。
- **再生終了時に次の動画を提示する**
  - 単体 test で次を確かめる。
    - `ended` で層が出る。
    - 同じフォルダの後続が無いとき「次を再生」が出ない。
    - 「もう一度見る」で先頭から再生される。
    - 「次を再生」で次の動画の画面へ戻り先付きで移り、移った先で再生が始まる。
- 全体として次を満たす。
  - `task check` と `task test-e2e` が成功する。
  - 画面が変わるので、PR に 360px・768px・1280px の画像（各状態・タッチ用の中央操作・
    再生終了を含む）を添える。あわせて、`ui-design.md` の観点による見た目・キーボード操作・
    支援技術の確認結果を添える。
