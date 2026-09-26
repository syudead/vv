# Implementation Plan: ライブ変換のシークと動画配信の無駄を減らす

**Branch**: `feature/018-live-transcode-seek` | **Parent Issue**: #371

**Input**: The parent Issue. It is this feature's specification.

## Summary

変換再生（`GET /api/videos/{id}/transcode.mp4`）のシークと途中からの再開で、映像がコピーできる
形式の動画（`videoCanCopy` が真）は再エンコードせず `-c:v copy` で変換する。コピーは指定位置の
直前のキーフレームから始まるので、サーバーは実際に始まった時刻を ffmpeg の出力から読み取り、
プレイヤーはそれを新しい経路 `GET /api/videos/{id}/transcode-start` で受け取って、表示する現在
時刻と保存する再生位置を映っている内容に合わせる（親 Issue #371 要件 1〜5）。直前のキーフレームが
指定位置から 15 秒より前なら、現行どおりエンコードし直して指定位置から始める（要件 3、
[research.md R-2](research.md#r-2-コピーで許すキーフレームとの差)）。

取り込み時の解析（ffprobe）は、ライブ変換に要る stream の情報を新しい表
`video_transcode_probes` に保存し、ライブ変換はそれを使って ffprobe を起動しない。保存が無い、
保存したときとファイルの大きさか更新時刻が違う、保存値で変換が最初のデータの前に失敗した、の
どれかのときだけその場で ffprobe を実行し、結果で保存を置き換える（要件 7〜10、
[data-model.md](data-model.md)）。

要件 6（エンコード時のキーフレーム間隔 2 秒）と要件 11（直接再生の sendfile）は PR #375 で `main`
に入っており、この Plan の範囲外である。要件 12 の文書の更新は各実装単位が行う。

## Technical Context

**Canonical definitions**:

- 境界と依存方向、store の役割の型、索引と利用者データの区別: [ARCHITECTURE.md](../../ARCHITECTURE.md)・
  [internal/store/roles.go](../../internal/store/roles.go)・[.golangci.yml](../../.golangci.yml)
- ライブ変換の現行の作り（要求ごとの ffprobe と ffmpeg、引数の組み立て、MOV の二入力）:
  [internal/media/transcode.go](../../internal/media/transcode.go)・
  [internal/httpapi/transcode.go](../../internal/httpapi/transcode.go)・
  [docs/design-docs/mov-live-transcoding.md](../../docs/design-docs/mov-live-transcoding.md)
- 取り込み時の解析と結果の反映: [internal/media/probe.go](../../internal/media/probe.go)・
  `app.Ingest.Probe`（[internal/app/ingest.go](../../internal/app/ingest.go)）・
  `IngestStore.ApplyProbe`/`ApplyProbeForJob`（[internal/store/ingest_results.go](../../internal/store/ingest_results.go)）
- 再生画面のライブ変換の時間軸（source の offset を足す middleware）:
  [web/src/player/liveOffset.ts](../../web/src/player/liveOffset.ts)・
  [web/src/player/playbackAttempt.ts](../../web/src/player/playbackAttempt.ts)
- API の正本と生成: [api/openapi.yaml](../../api/openapi.yaml)（`task generate`）。経路の分類の守り:
  [internal/httpapi/openapi_routes_test.go](../../internal/httpapi/openapi_routes_test.go)
- ゲストへの配信の扱い（台帳と打ち切り）:
  [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)・
  `lookupServedVideo`（[internal/httpapi/visibility.go](../../internal/httpapi/visibility.go)）
- マイグレーションは足すだけで既存を変えない: [internal/store/migrations/](../../internal/store/migrations/)・
  [scripts/migrations-immutable.sh](../../scripts/migrations-immutable.sh)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task test-e2e`）

**Feature-specific context**:

- マイグレーションを 1 つ足す（`00013_transcode_probes.sql`、[data-model.md §1](data-model.md#1-マイグレーション)）。
  既存の表は変えない。
- 追加する依存は無い。使う ffmpeg の選択肢（`-copyts`・`-start_at_zero`・`-movflags delay_moov`）は
  mp4 muxer に以前からあるもので、開発コンテナの ffmpeg 6.1 で動作を確かめた
  （[research.md R-1](research.md#r-1-実際の開始位置をどこから知るか)）。
- 実際の開始位置を読むために、ライブ変換の出力（fragmented MP4）の先頭の `moov` をサーバーが読んで
  書き換える。書き換えるのは各 track の edit list（`elst`）だけで、`moof`/`mdat` には触れない
  （[research.md R-1](research.md#r-1-実際の開始位置をどこから知るか)）。
- コピーで途中から始めたときの最初のデータは、元動画のキーフレーム 1 区間分（`frag_keyframe` の
  区切り）を読み終えてから出る。これは現行の先頭からのコピーと同じで、この Plan では変えない
  （[research.md R-7](research.md#r-7-コピーの最初のデータが出るまでの時間)）。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。解析情報の値型
  （`domain.TranscodeProbe`）、保存値を使ってよいかの判定、コピーで許す差の判定は `internal/domain` の
  純粋関数に置く。ffprobe/ffmpeg の起動と出力の読み取りは `internal/media`、SQL は `internal/store`、
  経路と進行中の変換の台帳は `internal/httpapi`。`internal/media` が store を呼ぶ形にはしない
  （Structural Decisions 6）。
- **store の役割の型**: 合格。解析情報の書き込みは取り込みの結果の反映と同じ `IngestStore`、読み出しは
  `LibraryStore`（動画 1 件の読み出しと同じ側）。役割の型どうしは公開メソッドを呼ばない。
- **索引と利用者データの区別**: 合格。`video_transcode_probes` は `videos` に外部キーを張る索引で、
  消えても次の変換か再解析で埋まる（[data-model.md §1](data-model.md#1-マイグレーション)）。
- **API の正本**（AGENTS.md）: 合格。`transcodeVideo` の新しい引数と `getTranscodeStart` は
  `api/openapi.yaml` に足して `task generate` で生成する。本文を持たないので `requiresJSONBody` は変えない。
- **認証の境界とゲストへの応答**: 合格。`getTranscodeStart` は `transcodeVideo` と同じ `security`
  （所有者とゲスト）で、公開でない動画への要求は `transcodeVideo` と同じ 404 にする
  （[contracts/transcode-start-api.md §2](contracts/transcode-start-api.md#2-get-apivideosidtranscode-start)）。
- **利用者から見た再生の結果を変えない**（親 Issue「目的」）: 条件付きで合格。表示される時刻・向き・
  縦横比・音声は変えない。変わるのは、コピーで始めたときに再生が指定位置ではなく直前のキーフレームから
  始まることで、これは親 Issue 要件 2 が求める動作である。
- **文書は変更と同じ PR で直す**（core-beliefs.md）: 各実装単位が `stream.go` のコメント以外の対象
  （ARCHITECTURE.md、`mov-live-transcoding.md`、新しい設計文書）を自分の範囲で直す（要件 12）。

Phase 1 のあとも判定は同じである。

## Project Structure

### Documentation (this feature)

```text
specs/018-live-transcode-seek/
├── plan.md                              # This file
│                                        # No spec.md — the parent Issue is the specification
├── research.md                          # 出力の時刻の読み取り方・許す差・保存の形・報告の経路の決定
├── data-model.md                        # video_transcode_probes の表と保存値の規則
└── contracts/
    └── transcode-start-api.md           # transcodeVideo の attempt 引数と getTranscodeStart
```

`quickstart.md` は作らない。確かめ方は既存の検査（`task check`・`task test-e2e`）と、各単位の
受け入れに書いた ffmpeg を起動する統合テストで足りる。

### Source Code

**Affected boundaries**:

- `internal/domain`: 解析情報の値型 `TranscodeProbe`（`Probe` に載せる）、保存値を使ってよいかの
  判定（版とファイルの同一性）、コピーで許すキーフレームとの差の判定。
- `internal/media`: ffprobe の出力を 1 つの parser で `domain.Probe` にする（取り込みと要求時で共用）。
  `LiveTranscoder` は保存済みの解析情報を受け取り、無ければ ffprobe を実行して結果を返す。コピーの
  経路は `-copyts -start_at_zero` と `delay_moov` で出し、`moov` の edit list から実際の開始位置を
  読んで書き換える。コピー→エンコード、保存値→その場の ffprobe の切り替えも同じ要求の中で行う。
- `internal/store`: マイグレーション、`IngestStore` の解析結果の反映に解析情報の upsert を足す、
  `IngestStore.SaveTranscodeProbe`（要求時の保存）、`LibraryStore.TranscodeProbe`（読み出し）。
- `internal/httpapi`: 変換の経路が保存済みの解析情報と開いたファイルの大きさ・更新時刻を media に渡し、
  media が ffprobe を実行したら結果を保存する。進行中の変換の実際の開始位置を `attempt` ごとに覚える台帳と
  `GET /api/videos/{id}/transcode-start`。
- `web/src/player`: 変換の URL に `attempt` を付け、開始位置の報告を受けて offset を直す。
- `api/openapi.yaml` と生成物、`docs/design-docs/`、`ARCHITECTURE.md`、`web/e2e/`。

**New paths**: `internal/store/migrations/00013_transcode_probes.sql`、`internal/media/fmp4.go`
（`moov` の読み取りと書き換え）、`docs/design-docs/live-transcode-seek.md`、
`internal/httpapi/transcode_start.go`。

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。

## Structural Decisions

1. **コピーで途中から始める変換は、`-ss` を入力側に置いた `-c:v copy` に `-copyts -start_at_zero` と
   `-movflags delay_moov` を足して出し、サーバーが出力の `moov` の edit list から実際の開始位置を
   読んで、時間軸が 0 から始まるよう edit list を書き換えてから送る**
   （[research.md R-1](research.md#r-1-実際の開始位置をどこから知るか)）。実際の開始位置は、最も早く
   始まる track の時刻とし、他の track はそれとの差を edit list に残す（音ずれを起こさない）。
   エンコードの経路は今の引数のまま（`-copyts` も `delay_moov` も付けない）で、実際の開始位置は
   指定位置そのものである。音声は要件 4 のとおり今の規則（`audioCanCopy` ならコピー、でなければ
   AAC）のままで、映像をコピーするかどうかとは独立に決める。最初のキーフレームより前へのシークは、
   ffmpeg が動画の最初のキーフレームから始めるので、その時刻（先頭のフレームが 0 秒の動画では 0 秒）が
   実際の開始位置になる（Edge Case「最初のキーフレームより前へのシーク」）。
   - 却下: `-copyts` だけで元の時刻を出し、ブラウザの時間軸にそのまま出す案。Chrome と Firefox は
     progressive 再生で最初の時刻を 0 に寄せるので、プレイヤーが開始位置を知る手段にならない。
   - 却下: ffmpeg の前にキーフレームの位置を調べる案（ffprobe や ffmpeg をもう 1 本）。要件 8 が
     取り除く「要求ごとの外部プロセス」そのものになる。
   - 却下: 取り込み時にキーフレームの一覧を保存する案。動画ごとにファイル全体を読む解析が取り込みに
     加わり、要件 7 が挙げる保存対象にも無い。
   - 却下: `moof` ごとの `tfdt` を書き換えて時刻を寄せる案。先頭の `moov` を 1 回書き換えれば済む
     ところを、送る fragment 全部の書き換えにする理由が無い。
2. **指定位置と直前のキーフレームの差が 15 秒以内ならコピーで始め、超えたらエンコードし直して
   指定位置から始める。判定は `internal/domain` の純粋関数に置く**
   （[research.md R-2](research.md#r-2-コピーで許すキーフレームとの差)）。
   - 却下: 10 秒。x264 の既定（250 フレーム）は 23.976 fps で 10.4 秒になり、親 Issue が例に挙げる
     「中身が H.264 の MKV」の多くがエンコードし直しに落ちる。
   - 却下: 上限なし。要件 3 に反し、キーフレームが場面の切り替わりにしか無い動画では数十秒戻る。
3. **切り替えは media が同じ要求の中で行い、順序は「コピー → エンコード」「保存済みの解析情報 →
   その場の ffprobe」で、それぞれ 1 回だけである**
   （[research.md R-3](research.md#r-3-切り替えの順序と時間の予算)）。最初のデータが出る前の失敗
   （プロセスがデータを出さずに終わる、コピーの実際の開始位置が差の上限を超える）だけを切り替えの
   理由にし、期限切れは今までどおり失敗にする。期限は今の `transcodeStartupTimeout`（6 秒）を
   切り替え全体で共有する。
   - 却下: 試行ごとに別の期限を持つ案。コピーの遅さは I/O で、同じファイルを読むエンコードも同じだけ
     遅いので、コピーを早めに諦めても速くならない。
4. **実際の開始位置は、プレイヤーが変換の URL に付けた `attempt` を鍵に、別の経路
   `GET /api/videos/{id}/transcode-start?attempt=…` で受け取る。サーバーは進行中の変換の開始位置を
   台帳に覚え、まだ決まっていなければ決まるまで（期限まで）応答を待たせる**
   （[contracts/transcode-start-api.md](contracts/transcode-start-api.md)・
   [research.md R-4](research.md#r-4-実際の開始位置をプレイヤーへ伝える経路)）。`<video>` 要素は
   応答ヘッダーを読めないため、別の経路が要る。報告が届く前は今までどおり指定位置を表示し、届いたら
   offset を実際の開始位置に置き換える。届かなければ指定位置のまま（現行と同じ表示）にする。
   - 却下: `/api/events` の SSE で配る案。ゲストの再生でも要り、購読の有無と届く順序を再生画面が
     気にすることになる。
   - 却下: 先に開始位置を返す経路で ffmpeg を起動し、動画の要求がそれに接続する案。プロセスの寿命が
     2 つの要求にまたがり、接続されなかったプロセスの後始末とゲストの台帳の打ち切りを別に持つことになる。
   - 却下: MSE で `fetch` して応答ヘッダーを読む案。配信の仕組みの作り直しで、親 Issue の対象外。
5. **解析情報は新しい表 `video_transcode_probes` に、`domain.TranscodeProbe` を JSON にした 1 列と
   その版、解析したファイルの大きさと更新時刻（ナノ秒）で持つ**
   （[data-model.md](data-model.md)・[research.md R-5](research.md#r-5-解析情報の保存の形)）。
   読み出しは `videos` の一覧・詳細には載せず、変換の経路だけが 1 件ずつ読む。書き込みは upsert 1 文で、
   同時に書いても後に書いた行が残る（Edge Case「同時に複数の要求」）。
   - 却下: `videos` に型付きの列を 20 個ほど足す案。一覧の読み出しが使わない列で行を太らせ、変換に
     要る項目が増えるたびにマイグレーションが要る。
   - 却下: ffprobe の生の JSON を保存する案。要求時の解釈が保存時の ffprobe の版に依存し、数十 KB の
     文字列を動画ごとに持つ。
6. **ffprobe の出力の解釈は `internal/media` の 1 つの parser にし、取り込みも要求時もそれを使う。
   保存済みの解析情報を使ってよいかは httpapi の経路が `internal/domain` の判定で決め、media には
   使える解析情報か nil を渡す。media が ffprobe を実行したときは結果を返し、経路が
   `IngestStore.SaveTranscodeProbe` で保存する。** 変換の経路が store と media の両方と直接話す形は
   `stream.go` と同じである。
   - 却下: media が store を読む案。adapter が adapter に依存し、ffmpeg を持たない単体テストが
     SQLite を要することになる。
   - 却下: `internal/app` に use case を置く案。判断は「版が同じで大きさと更新時刻が一致するか」の
     純粋な比較 1 つで、組み合わせる相手が無い。
7. **直接再生から切り替えた変換（`playable = true`）は今までどおりエンコードし直す。シークだけでは
   エンコードを強制しない。** `Transcoder` に渡す `normalize` は `video.Playable` だけになる（要件 5）。

## Implementation Work

### 取り込みの解析でライブ変換に要る情報を保存する

**Scope**: `domain.TranscodeProbe` と `domain.Probe` への追加、`internal/media` の parser の統合
（取り込みと `LiveTranscoder` が同じ関数で `domain.Probe` を得る。`LiveTranscoder` 自体の引数は
まだ変えない）、`00013_transcode_probes.sql`、`IngestStore.ApplyProbe`/`ApplyProbeForJob` での upsert、
`IngestStore.SaveTranscodeProbe`、`LibraryStore.TranscodeProbe`、保存値を使ってよいかの判定
`domain.TranscodeProbeUsable`。ARCHITECTURE.md の store の役割と表の区分に追記する
（[data-model.md](data-model.md)）。

**Dependencies**: None

**Acceptance**: 取り込みの解析ジョブが終わった動画に `video_transcode_probes` の行があり、
`POST /api/videos/{id}/probe` のやり直しでも書き直される（store のテスト）。同じ ffprobe の出力から
取り込みの parser が保存する `TranscodeProbe` と、要求時の parser が作る値が等しく、同じ
`transcodeArgs` を生む（親 Issue 受け入れ条件 10）。版が違う行と JSON が読めない行は「無い」と
読まれる（`domain.TranscodeProbeUsable` のテスト）。`task check` が通る。

### ライブ変換で保存済みの解析情報を使い、無いか合わないときだけ ffprobe を実行して保存する

**Scope**: `httpapi.Transcoder` の引数を、保存済みの解析情報（nil 可）と開いたファイルの大きさ・
更新時刻を含む形にし、`LiveTranscoder.Start` は解析情報があれば ffprobe を起動せず、無ければ
ffprobe を実行して結果を返す。保存値で最初のデータが出る前に失敗したら、同じ要求の中で ffprobe を
実行して 1 回だけやり直す（Structural Decisions 3・6）。経路は結果を `SaveTranscodeProbe` で保存する。
`docs/design-docs/live-transcode-seek.md` を作り（この単位は解析情報の再利用の節）、
`docs/design-docs/index.md` に載せ、`mov-live-transcoding.md` の「request時probe」の記述を直す。

**Dependencies**: 取り込みの解析でライブ変換に要る情報を保存する

**Acceptance**: 取り込み済みの動画の変換で、1 回目もシーク後も ffprobe が起動しない（`commandContext` を
差し替えたテストで起動回数を数える。親 Issue 受け入れ条件 7）。解析情報が無い動画は 1 回目だけ
ffprobe が起動し、その結果が保存され、2 回目は起動しない（受け入れ条件 8）。大きさか更新時刻が違う
ファイルでは ffprobe が起動して保存が置き換わり、次は起動しない（受け入れ条件 9）。途中で打ち切られた
ffprobe の結果は保存されない。既存の変換のテストが通り、`task check` と `task check-docs` が通る。

### シークと途中からの再開でも映像をコピーで変換し、実際の開始位置を解決する

**Scope**: `transcodeArgs` から「途中からの開始では必ずエンコード」を外し、コピーで途中から始める
引数（`-copyts -start_at_zero`、`delay_moov`）を足す。`internal/media/fmp4.go` で出力の `moov` を
読み、edit list から実際の開始位置を得て時間軸が 0 から始まるよう書き換える。差の上限
（`domain.CopySeekAllowance`）を超えたとき、またはコピーが最初のデータを出さずに終わったときは
同じ要求の中でエンコードに切り替える（Structural Decisions 1〜3）。`LiveTranscoder.Start` は実際の
開始位置（ミリ秒）を返し、経路はまず記録だけする。`normalize` を `video.Playable` だけにする
（Structural Decisions 7）。設計文書に「コピーの経路と差の上限」の節を足し、`mov-live-transcoding.md`
の seek 時の記述を直す。

**Dependencies**: ライブ変換で保存済みの解析情報を使い、無いか合わないときだけ ffprobe を実行して保存する

**Acceptance**: H.264 の MKV を途中から変換すると ffmpeg が `-c:v copy` で起動し、出力を ffprobe で
読むと最初の映像フレームが直前のキーフレームの内容で、時間軸は 0 から始まり、音声との差が元動画と
同じである（ffmpeg を起動する統合テスト。受け入れ条件 1・4 の一部）。差の上限より長いキーフレーム
間隔の fixture ではエンコードで起動し、指定位置から始まる（受け入れ条件 3）。`videoCanCopy` が偽の
動画はエンコードで、引数がキーフレーム間隔以外は今と同じ（受け入れ条件 4）。MOV の二入力でも
音声が映像と同じ実際の開始位置から始まる。既存の変換のテスト（MOV の二入力、回転、縦長、4K 超の
縮小、途中開始、切断時の後始末）が通る（受け入れ条件 13）。PR に、1080p・30fps の H.264 を途中から
コピーで変換したときの最初のデータまでの時間を、#375 と同じ手順で計測して残す（受け入れ条件 6）。
`task check` と `task check-docs` が通る。

### 実際の開始位置をプレイヤーに伝え、表示する時刻と保存する再生位置を合わせる

**Scope**: `api/openapi.yaml` に `transcodeVideo` の `attempt` 引数と `getTranscodeStart` を足して
生成する。httpapi に進行中の変換の台帳（`attempt` → 実際の開始位置、決まるまで待たせる、終了後は
しばらく残す）と経路を足す（[contracts/transcode-start-api.md](contracts/transcode-start-api.md)）。
`web/src/api/client.ts` に取得関数、`liveOffset.ts` の middleware に報告を受けて offset を直す処理、
`VideoPlayer.tsx` の `vvOffsetChanged` 経由で再生位置の保存を実際の開始位置に合わせる。
`web/e2e/media-fixtures.mjs` にキーフレームが疎な H.264 の MKV を足し、`playback.e2e.ts` で確かめる。
設計文書に報告の経路の節を足す。

**Dependencies**: シークと途中からの再開でも映像をコピーで変換し、実際の開始位置を解決する

**Acceptance**: コピーできる MKV でシークすると、プレイヤーの現在時刻が直前のキーフレームの時刻に
なり、一時停止して再読み込みすると同じ場面から再開する（e2e。受け入れ条件 2）。報告が 404 か期限切れの
ときは指定位置を表示し続ける（middleware の単体テスト）。`getTranscodeStart` が公開でない動画への
ゲストの要求に 404 を返す（境界の分類のテストに載せる）。`openapi_routes_test` が通り、`task check`・
`task check-docs`・`task test-e2e` が通る。画面の見た目は変えないが、再生画面の現在時刻の表示が変わる
単位なので、実装では Chromium での再生を目で確かめる。
