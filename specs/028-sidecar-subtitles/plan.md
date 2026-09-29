# Implementation Plan: 動画の隣に置いた字幕ファイル（SRT / WebVTT）を再生画面で表示する

**Branch**: `feature/028-sidecar-subtitles` | **Parent Issue**: #522

**Input**: The parent Issue. It is this feature's specification.

## Summary

動画ファイルと同じフォルダにある同じ名前の字幕ファイル（`<名前>.srt`・`<名前>.vtt`・
`<名前>.<ラベル>.srt`・`<名前>.<ラベル>.vtt`）を、設定や再スキャンなしに再生画面で選んで表示できる
ようにする。直接再生でもライブ変換でも、字幕は元動画の時刻に合って出る。

- **見つけ方**: 再生画面が呼ぶ `GET /api/videos/{id}/subtitles` が、配信が開く所在のフォルダを
  そのたびに読む。SQLite には何も足さない
  （[research.md R-1](research.md#r-1-字幕ファイルは要求のたびにフォルダを読んでsqlite-には置かない)、
  [R-2](research.md#r-2-探すフォルダは配信が開く所在のフォルダである)）。名前の照合・重複・上限
  （4 MiB）の規則は `internal/domain` の純粋関数が持つ
  （[R-3](research.md#r-3-名前の照合と重複の規則は-internaldomain-の純粋関数が持つ)）。
- **返し方**: `GET /api/videos/{id}/subtitles/{file}` が、`internal/media` の Go だけの変換で
  文字コード（UTF-8・BOM 付き UTF-16・Shift_JIS）を判定し、SRT を WebVTT にし、`offsetMs` だけ
  時刻をずらして返す。ffmpeg は使わない
  （[R-4](research.md#r-4-変換は-go-で行いffmpeg-は使わない)、
  [R-5](research.md#r-5-文字コードは-bom--utf-8-の妥当性--shift_jis-の順で決める)、
  [R-8](research.md#r-8-srt-の書式の揺れは時刻の行だけを正規化しcue-の本文はそのまま通す)）。
  契約は [contracts/subtitles-api.md](contracts/subtitles-api.md)。
- **時刻合わせ**: ライブ変換では、プレイヤーが再生の時間軸の offset（`transcode-start` の報告）が
  決まるたびに、その値を `offsetMs` に付けてトラックを付け直す。決まるまでは付けない
  （[R-6](research.md#r-6-ライブ変換の時刻合わせはサーバーが-offsetms-だけ時刻をずらした-webvtt-を返す)）。
- **画面**: video.js の `SubsCapsButton` を再生速度の隣に置き、選択（オン・オフとラベル）を
  音量と同じ形でブラウザに記憶し、`c` キーで切り替える
  （[R-9](research.md#r-9-字幕の選択は-websrcpreferences-に音量と同じ形で保存する)、
  [R-10](research.md#r-10-字幕ボタンとメニューは-videojs-の-subscapsbutton-を使う)、
  [R-11](research.md#r-11-c-キーは-keyboardts-に足し切り替えの判断はプレイヤーが持つ)）。
  見た目と操作の基準は親 Issue の「UI品質」がそのまま仕様である（`ui` ラベルは無く、design
  段階は無い）。文言は既存の英語のカタログに足す（要件 12）。

## Technical Context

**Canonical definitions**:

- 境界と依存方向、ファイルを開いてよいかの規則、認証の境界、生成物のキャッシュの扱い:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)、[internal/mediafs/media_file.go](../../internal/mediafs/media_file.go)、
  [internal/httpapi/auth.go](../../internal/httpapi/auth.go)（`accessRoutes`）、
  [internal/httpapi/artifact_cache.go](../../internal/httpapi/artifact_cache.go)、
  [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)
- 配信とライブ変換の経路: [internal/httpapi/stream.go](../../internal/httpapi/stream.go)
  （`openMediaFile`）、[internal/httpapi/transcode.go](../../internal/httpapi/transcode.go)、
  [internal/httpapi/visibility.go](../../internal/httpapi/visibility.go)（`lookupServedVideo`）
- ライブ変換の時間軸と実際の開始位置の報告:
  [docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md)、
  [specs/018-live-transcode-seek/contracts/transcode-start-api.md](../018-live-transcode-seek/contracts/transcode-start-api.md)、
  [web/src/player/liveOffset.ts](../../web/src/player/liveOffset.ts)
- 再生画面と操作バー: [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md)
  （「Control bar」）、[web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx)、
  [web/src/player/keyboard.ts](../../web/src/player/keyboard.ts)、
  [web/src/player/playerControls.ts](../../web/src/player/playerControls.ts)、
  [web/src/preferences/playbackVolume.ts](../../web/src/preferences/playbackVolume.ts)、
  [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
- 画面の文言と video.js の文言の置き換え: [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)、
  [web/src/i18n/en.ts](../../web/src/i18n/en.ts)（`player.controls`、`playerDictionary()`）
- API の正本とエラーの形: [api/openapi.yaml](../../api/openapi.yaml)、
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`・
  `task test-e2e`）、[web/e2e/playback.e2e.ts](../../web/e2e/playback.e2e.ts)、
  [web/e2e/media-fixtures.mjs](../../web/e2e/media-fixtures.mjs)

**Feature-specific context**:

- Go と npm の依存は足さない。文字コードの復号は既に依存にある `golang.org/x/text`
  （`encoding/unicode`・`encoding/japanese`）で行う（R-5）。字幕の表示・メニューは video.js 8 に
  ある `SubsCapsButton`・`addRemoteTextTrack`・`vjs-text-track-display` を使う（R-10）。
- SQLite の表・列・移行・ドメインイベントは足さない（R-1）。`data-model.md` は無い。
- 字幕ファイルの上限は `domain.SubtitleFileLimit = 4 MiB`（R-3。親 Issue の Edge Case が plan に
  委ねた値）。
- ライブ変換の時間軸（`liveOffset.ts` の offset、`transcode-start` の報告）は変えない。字幕は
  その offset を読む側に立つ（R-6）。
- `ui-design.md` は作らない（`ui` ラベルが無い）。画面の基準は親 Issue の「UI品質」にある。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: 名前の照合・重複・並び・上限の純粋関数と値（`SubtitleSidecar`）。`os` も
    `x/text/encoding` も持たない。
  - `internal/mediafs`: フォルダの項目の列挙と、一覧にある名前を開く判定（読んでよいかの規則の
    一元の置き場。R-2）。
  - `internal/media`: 文字コードの判定と WebVTT への変換・時刻のずらし（`fmp4.go` と同じ、形式の
    扱い。R-4）。store も logger も持たず、誤りは値で返す。
  - `internal/httpapi`: 要求の解釈、`lookupServedVideo`、`MediaFiles`・`SubtitleConverter` の
    interface を通した呼び出し、`gen` の型への変換だけ。
  - `cmd/mdm`: 配線だけ。兄弟のパッケージを互いに import しない（depguard の規則は変えない）。
  - `internal/app`: 触らない。要求 1 回で完結し、状態も判断の持ち越しも無い（配信と同じ）。
- **ファイルを開いてよいかの規則**（ARCHITECTURE.md の `internal/mediafs` の段落）: 合格。
  `httpapi` は自分で `ReadDir` も `Open` もせず、所在と登録フォルダを `mediafs` に渡す。開くのは
  一覧にある名前だけで、要求の文字列からパスを組み立てない
  （[contracts §2](contracts/subtitles-api.md#2-get-apivideosidsubtitlesfile)）。
- **認証の境界**（ARCHITECTURE.md の認証の段落、要件 11）: 合格。2 つの経路を `accessRoutes` に
  「ゲストも」として足し、`openapi.yaml` の `security` と `openapi_routes_test.go` で一致を確かめる。
  ゲストには `lookupServedVideo` が公開の動画だけを返す。
- **API の正本**（ARCHITECTURE.md）: 合格。`api/openapi.yaml` を変えて `task generate` し、生成物は
  手で直さない（AGENTS.md）。
- **索引と利用者データの区別**（ARCHITECTURE.md「Rebuildable and user data」）: 該当なし。
  保存するものが無い。
- **画面の文言はカタログから**（docs/design-docs/i18n.md、要件 12）: 合格。ボタン・メニュー・
  「オフ」・「既定」の文言は `en.ts` に足し、video.js の文言は `playerDictionary()` で置き換える。
  疑似ロケールの検査で漏れを捕まえる。
- **サーバーの出力は英語**（`.golangci.yml` の gosmopolitan）: 合格。ログと `message` は英語。
- **文書は変更と同じ PR で直す**（core-beliefs.md、AGENTS.md）: 合格。各単位が ARCHITECTURE.md・
  設計文書・how-to を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/028-sidecar-subtitles/
├── plan.md                     # This file
│                               # No spec.md — the parent Issue is the specification
├── research.md                 # R-1〜R-11: 見つけ方、探すフォルダ、照合の規則、変換、文字コード、
│                               #   時刻合わせ、壊れたファイル、SRT の揺れ、記憶、ボタン、c キー
├── quickstart.md               # 全画面・Safari・実在の Shift_JIS の人による確認
└── contracts/
    └── subtitles-api.md        # GET /api/videos/{id}/subtitles, GET …/subtitles/{file}
```

`data-model.md` は作らない。表も列も足さず、エンティティを変えない（R-1）。

### Source Code

**Affected boundaries**:

- `internal/domain`: `SubtitleSidecar`・`SidecarEntry`・`SubtitleFileLimit`・`SubtitleSidecars`
  （R-3）
- `internal/mediafs`: `ListSidecarFiles`・`OpenSidecarFile`（R-2）
- `internal/media`: `SubtitleConverter`（文字コード、SRT → WebVTT、WebVTT の確認、`offsetMs`。
  R-4〜R-8）
- `api/openapi.yaml`・`internal/httpapi`（2 つの経路、`accessRoutes`、`MediaFiles` と新しい
  `SubtitleConverter` の interface、`reason` の追加）・`cmd/mdm/main.go`（配線）
- `web/src/api/client.ts`・`web/src/player`（`VideoPlayer.tsx`、`VideoPage.tsx`、`keyboard.ts`、
  `playerControls.ts`、`liveOffset.ts`）・`web/src/preferences`・`web/src/i18n/en.ts`・
  `web/src/index.css`
- `web/e2e`（字幕の fixture と再生の検査）
- `ARCHITECTURE.md`・`docs/design-docs/`・`docs/how-to/running-vv.md`

**New paths**:

- `internal/domain/subtitle.go`
- `internal/mediafs/sidecar.go`
- `internal/media/subtitle.go`
- `internal/httpapi/subtitles.go`
- `web/src/player/subtitleTracks.ts`（トラックの付け直し、選択、`c` キーの判断。`VideoPlayer.tsx`
  から切り出す）
- `web/src/preferences/subtitlePreference.ts`
- `docs/design-docs/sidecar-subtitles.md`（見つけ方、変換、時刻合わせの現行設計。
  `docs/design-docs/index.md` に載せる）

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。変換を
`internal/media` に置き、`internal/httpapi` からは interface で受け取る（R-4）。`internal/app` を
通さないのは、配信とライブ変換の経路と同じく、要求 1 回で完結して app が持つ状態も判断も無い
からである。app に「動画の字幕」の使用例を作る案は、interface を 1 つ余分に挟むだけで判断が
増えないので採らない。

## Implementation Work

### 動画の隣の字幕ファイルを見つけ、文字コードを判定して WebVTT に変換する

**Scope**: HTTP より下の 3 つの部品。
- `internal/domain/subtitle.go`: `SidecarEntry`（名前・大きさ）、`SubtitleSidecar`（ファイル名・
  ラベル・形式）、`SubtitleFileLimit`、`SubtitleSidecars`
  （[R-3](research.md#r-3-名前の照合と重複の規則は-internaldomain-の純粋関数が持つ)）。
- `internal/mediafs/sidecar.go`: `ListSidecarFiles(roots, videoPath)`（動画の所在が
  `OpenMediaFile` と同じ規則で開けるときだけ、そのフォルダの通常ファイル（symlink は除く）の
  名前と大きさを返す）と `OpenSidecarFile(roots, videoPath, name)`（同じフォルダの `name` を
  開く。`name` はフォルダの項目の名前と一致するものだけを受ける。
  [R-2](research.md#r-2-探すフォルダは配信が開く所在のフォルダである)）。
- `internal/media/subtitle.go`: `SubtitleConverter.Convert(src []byte, format domain.SubtitleFormat, offsetMs int64) ([]byte, error)`。
  文字コード（[R-5](research.md#r-5-文字コードは-bom--utf-8-の妥当性--shift_jis-の順で決める)）、
  SRT → WebVTT（[R-8](research.md#r-8-srt-の書式の揺れは時刻の行だけを正規化しcue-の本文はそのまま通す)）、
  WebVTT のヘッダーの確認、`offsetMs` のずらし
  （[R-6](research.md#r-6-ライブ変換の時刻合わせはサーバーが-offsetms-だけ時刻をずらした-webvtt-を返す)）。
  読めない入力は `domain.ErrSubtitleUnreadable` を包んで返す
  （[R-7](research.md#r-7-壊れたファイルは一覧には出し取得で-404-にする)）。

**Dependencies**: None.

**Acceptance**: 次の検査があり、`task check` が通る。
- `internal/domain` のテスト（表）: `movie.mp4` に対し `movie.srt`・`Movie.SRT`・`movie.ja.srt`・
  `movie.en.forced.vtt` が候補になり、`movie2.srt`・`movie.txt`・`other.srt` はならない。
  `my.movie.2024.mp4` の `my.movie.2024.ja.srt` のラベルが `ja`。同じラベルの `.srt` と `.vtt` は
  `.vtt` だけ。並びはラベル無し → ラベルの自然順。4 MiB を超える項目は入らない。
- `internal/mediafs` のテスト（一時ディレクトリ）: 動画の隣のファイルが列挙され、サブフォルダと
  symlink は入らない。登録フォルダの外の動画は列挙も開くこともできない。一覧に無い名前
  （`../x.srt`、別のフォルダの名前）は `OpenSidecarFile` が断る。
- `internal/media` のテスト（表）: UTF-8（BOM あり・なし）、BOM 付き UTF-16 LE・BE、Shift_JIS の
  同じ日本語 SRT が同じ WebVTT になる。`,` と `.` の小数点、番号行の無い cue、CRLF、桁の少ない
  時刻、`<i>` と `{\an8}` を含む本文（そのまま）、時刻の行が読めない cue（落ちる）、cue が
  1 つも無い SRT・`WEBVTT` の無い VTT・空（`ErrSubtitleUnreadable`）。`offsetMs = 8000` で
  0〜5 秒の cue が消え、3〜10 秒の cue が 0〜2 秒になり、10〜12 秒の cue が 2〜4 秒になる。
  WebVTT の `NOTE`・`STYLE` のブロックはずらしても残る。

### 字幕の一覧と取得の API を足し、ゲストには公開の動画の字幕だけを返す

**Scope**: [contracts/subtitles-api.md](contracts/subtitles-api.md) の 2 つの経路。
- `api/openapi.yaml`: `listVideoSubtitles`・`getVideoSubtitle`、`SubtitleTrack`、`reason` に
  `subtitle_unavailable`。`task generate`。
- `internal/httpapi/subtitles.go`: `lookupServedVideo` で動画を引き、`openMediaFile` と同じ順で
  所在を試して `MediaFiles.ListSidecarFiles` を呼び、`domain.SubtitleSidecars` で一覧にする。
  取得は一覧との完全一致で開き、`SubtitleConverter.Convert` の結果を `text/vtt`、
  `private, no-cache`、ダイジェストの `ETag`（`hashETag`）で返し、`If-None-Match` に 304。
  失敗は 404 `subtitle_unavailable` と `Warn` のログ。`accessRoutes` に「ゲストも」で足す。
- `cmd/mdm/main.go`: `media.SubtitleConverter` を配線する。
- `web/src/api/client.ts`: `getVideoSubtitles(id)`、`subtitleUrl(id, file, offsetMs)`。
- 文書: ARCHITECTURE.md（「Not built yet: subtitles」を外し、経路と `internal/mediafs`・
  `internal/media` の説明に足す。冒頭の「subtitle conversion」を ffmpeg の役割から外す）、
  `docs/design-docs/sidecar-subtitles.md`（見つけ方・変換・アクセス制御）と `index.md`。

**Dependencies**: `動画の隣の字幕ファイルを見つけ、文字コードを判定して WebVTT に変換する`。

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通り、`task generate` で差分が
出ない。
- `internal/httpapi` のテスト（一時ディレクトリと本物の `mediafs`）: `movie.srt` と
  `movie.ja.srt` を置くと一覧に 2 件が契約の順と形で返り、無ければ空の配列。開いたあとに
  `movie.en.vtt` を足して呼び直すと 3 件になる（受け入れ条件 4）。取得が `text/vtt` の WebVTT を
  返し、`offsetMs` でずれる。一覧に無い名前・`.vtt` に隠れた `.srt`・壊れたファイルは 404
  `subtitle_unavailable`。`offsetMs=-1` は 400。`ETag` が一致すれば 304。
- `internal/httpapi/guest_test.go`・`openapi_routes_test.go`: ゲストは公開の動画の一覧と字幕を
  取得でき、非公開の動画は両方の経路で 404 `video_not_found`（受け入れ条件 11）。境界の扱いが
  `openapi.yaml` の `security` と一致する。

### 再生画面に字幕ボタンとメニューを足し、選択をブラウザに記憶して c キーで切り替える

**Scope**: 直接再生の動画での字幕。基準は親 Issue の「UI品質」と要件 5〜8・10・12。
- `web/src/player/VideoPage.tsx`: 動画を開くたびに `getVideoSubtitles` を呼び、結果を
  `VideoPlayer` に渡す。失敗は字幕無しとして扱う。前後の動画へ移ったときは新しい動画の一覧で
  作り直す。
- `web/src/player/VideoPlayer.tsx`・`subtitleTracks.ts`: `subsCapsButton` を再生速度の前に置き、
  `textTrackSettings: false`。`addRemoteTextTrack` でトラックを付け（`offsetMs = 0`）、
  保存値と一致するラベルがあればそれを `showing` にする（[R-9](research.md#r-9-字幕の選択は-websrcpreferences-に音量と同じ形で保存する)、
  [R-10](research.md#r-10-字幕ボタンとメニューは-videojs-の-subscapsbutton-を使う)）。
  利用者がメニューで変えたときだけ保存する。`playerDictionary()` に video.js の字幕の文言を足し、
  ボタンに `aria-keyshortcuts="C"` と `withKey(…, "C")`。
- `web/src/player/keyboard.ts`・`playerControls.ts`: `c` → `toggleSubtitles()`
  （[R-11](research.md#r-11-c-キーは-keyboardts-に足し切り替えの判断はプレイヤーが持つ)）。
  `menuOpen()` は字幕のメニューが開いているときも真にする（`rateMenuOpen` の判定は
  `.vjs-menu` 全般なので、字幕のメニューも含まれることを確かめる）。
- `web/src/preferences/subtitlePreference.ts`: 読み書きの総関数。
- `web/src/index.css`: 操作バーが見えている間の `vjs-text-track-display` の下端を、操作バーと
  再生バーの高さに合わせる（要件 10）。
- `web/src/i18n/en.ts`: `player.subtitles`（ボタン名、オフ、既定の字幕名）。
- 文書: `docs/design-docs/library-ui.md` の再生画面の構成に字幕の層を足し、
  `docs/how-to/running-vv.md` に字幕ファイルの置き方（名前の形、対応する形式と文字コード）を書く。

**Dependencies**: `字幕の一覧と取得の API を足し、ゲストには公開の動画の字幕だけを返す`。

**Acceptance**: 画面が変わる単位なので、360px・768px・1280px 幅と全画面で見た目と操作を確認する
（[quickstart.md §1](quickstart.md#1-全画面と操作バーとの重なり受け入れ条件-10)）。次の検査があり、
`task check` と `task test-e2e` が通る。
- `web/src/player` の単体テスト: 字幕が無い動画では字幕ボタンが無い（受け入れ条件 3）。2 件あると
  ボタンとメニューに `ja`・`en` と「オフ」が出て、「字幕の設定」の項目は無い（受け入れ条件 2）。
  ラベルの無い字幕はカタログの既定の名前で出る（受け入れ条件 12）。保存値が無ければオフで始まる
  （受け入れ条件 7）。`{ enabled: true, label: "ja" }` の保存値で `ja` のある動画は `ja` が
  `showing`、無い動画はオフで、保存値は変わらない（受け入れ条件 6）。メニューで選ぶと保存値が
  変わる。`c` で表示中ならオフ、オフなら保存済みのラベル、無ければ最初のトラック、字幕の無い
  動画では何も起きない（受け入れ条件 8）。疑似ロケールの検査（`expectCatalogTextOnly`）を通る。
- `web/src/preferences` の単体テスト: 壊れた値・読めない領域で既定に戻り、書けなくても例外に
  ならない。
- `web/e2e/playback.e2e.ts`: fixture の `direct.mp4` の隣に `direct.srt` と `direct.ja.srt` を
  置いて、ボタンが出て、選ぶと cue の時刻に字幕の文字が `vjs-text-track-display` に出て、「オフ」で
  消える（受け入れ条件 1・2）。`ja` を選んでから再読み込みすると `ja` がオンのまま
  （受け入れ条件 6）。

### ライブ変換の再生でも字幕を元動画の時刻に合わせる

**Scope**: 再生の時間軸の offset に合わせたトラックの付け直し
（[R-6](research.md#r-6-ライブ変換の時刻合わせはサーバーが-offsetms-だけ時刻をずらした-webvtt-を返す)、
[contracts §3](contracts/subtitles-api.md#3-プレイヤーの使い方)）。
- `web/src/player/liveOffset.ts`: `LiveSource` に、offset が決まった（報告の 200、404・誤り、
  `attempt` の無い source）ことを知らせる `vvOffsetSettled(seconds)` を足す。source を作り直す
  シーク（`reloadAt`）でも同じ経路で知らせる。
- `web/src/player/VideoPlayer.tsx`・`subtitleTracks.ts`: offset が未決になったらトラックを外し、
  決まったら `offsetMs` を付けた URL で付け直し、直前に表示していたラベルを `showing` にする。
  保存値は書き換えない。直接再生からライブ変換への切り替え（`fallbackToTranscode`）も同じ。
- 文書: `docs/design-docs/sidecar-subtitles.md` に時刻合わせの節を足し、
  `docs/design-docs/live-transcode-seek.md` の「報告の経路」から字幕が offset を読むことを指す。

**Dependencies**: `再生画面に字幕ボタンとメニューを足し、選択をブラウザに記憶して c キーで切り替える`。

**Acceptance**: 次の検査があり、`task check`・`task check-docs`・`task test-e2e` が通る。
- `web/src/player/liveOffset.test.ts`: 報告の 200・404・`attempt` 無しのそれぞれで
  `vvOffsetSettled` が 1 回呼ばれ、値は 200 なら実際の開始位置、それ以外は指定位置。source の
  差し替え後に届いた古い報告では呼ばれない。
- `web/src/player` の単体テスト: 報告を待つ間はトラックが無く、届くとその offset の URL で
  付き、表示中だったラベルが `showing` のまま、保存値は変わらない。
- `web/e2e/playback.e2e.ts`: キーフレームが 0・8・16 秒の H.264 の MKV（既存の fixture）の隣に
  6〜7 秒・9〜10 秒・14〜15 秒の cue を持つ SRT を置き、字幕をオンにして 14 秒付近へシークすると、
  報告（8 秒）が届いたあとに字幕の URL が `offsetMs=8000` で取得され、9〜10 秒の cue が表示の
  9〜10 秒台に出て、6〜7 秒の cue は出ない。再読み込みで再開位置から始めたときも同じ
  （受け入れ条件 9）。
