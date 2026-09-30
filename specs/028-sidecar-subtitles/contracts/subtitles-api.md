# Contract: 動画の隣の字幕ファイル

正本は `api/openapi.yaml` で、この文書は足す 2 つの経路だけを書く。既存の応答の形は変えない。
どちらの `security` も `getVideoStream` と同じ（所有者とゲスト）で、`internal/httpapi/auth.go`
の `accessRoutes` に「ゲストも」として載せる。動画は `lookupServedVideo` で引くので、ゲストが
公開でない動画を指すと存在しない動画と同じ 404 `video_not_found` になる（親 Issue の要件 11）。

## 1. `GET /api/videos/{id}/subtitles`

`operationId: listVideoSubtitles`。再生画面が動画を開くたびに呼ぶ。

- 200: `{ "subtitles": SubtitleTrack[] }`。無ければ空の配列。
  - `SubtitleTrack`（`required: [file, label, format]`、`additionalProperties: false`）
    - `file`: string。字幕ファイルの名前（フォルダを含まない）。§2 の `{file}` に使う。
    - `label`: string。ファイル名のラベル（`ja`、`en.forced`）。ラベルの無い字幕は `""`。
    - `format`: `srt` | `vtt`。元のファイルの形式。
- 並びと重複の規則は [research.md R-3](../research.md#r-3-名前の照合と重複の規則は-internaldomain-の純粋関数が持つ)
  （ラベル無しが先頭、続いてラベルの自然順。同じラベルの `.srt` と `.vtt` は `.vtt` だけ。
  4 MiB を超えるファイルは載せない）。
- 探すフォルダは、配信が開く所在のフォルダ
  （[R-2](../research.md#r-2-探すフォルダは配信が開く所在のフォルダである)）。どの所在も開けなければ
  404 `file_unavailable`（`reason`。`getVideoStream` と同じ）。フォルダが読めなければ空の配列を
  返し、理由をログに残す（字幕が無いことと区別しない。再生そのものは止めない）。
- 中身は読まない。壊れたファイルもここには載り、§2 で 404 になる
  （[R-7](../research.md#r-7-壊れたファイルは一覧には出し取得で-404-にする)）。
- 404 `video_not_found`: 動画が無い、ゲストが公開でない動画を指した。
- 応答は `Cache-Control: no-store`。開き直すたびにフォルダの今の状態を返す（要件 2）。

## 2. `GET /api/videos/{id}/subtitles/{file}`

`operationId: getVideoSubtitle`。§1 の `file` を渡す。

- 引数:
  - `file`（path）: §1 が返した名前。サーバーは §1 と同じ一覧を作り直し、`file` がその中の 1 つと
    一致するときだけ開く（一致は文字列の完全一致。一覧に無い名前、`.srt` が `.vtt` に隠れた
    名前、上限を超える名前はすべて 404）。パスの区切りや `..` の検査は要らない。一覧に無い
    ものは開かないからである。
  - `offsetMs`（query、任意、既定 0、`minimum: 0`）: 再生の時間軸の 0 が元動画のどの時刻か
    （ミリ秒）。すべての cue の時刻からこの値を引く。終了時刻が 0 以下になる cue は返さず、
    開始時刻が負になる cue は 0 から始める
    （[R-6](../research.md#r-6-ライブ変換の時刻合わせはサーバーが-offsetms-だけ時刻をずらした-webvtt-を返す)）。
    負や整数でない値は 400 `invalid_request`。
- 200: `Content-Type: text/vtt; charset=utf-8`。本文は UTF-8 の WebVTT。SRT は変換し
  （[R-8](../research.md#r-8-srt-の書式の揺れは時刻の行だけを正規化しcue-の本文はそのまま通す)）、
  WebVTT はヘッダーを確かめて `offsetMs` だけずらして返す。文字コードは
  [R-5](../research.md#r-5-文字コードは-bom--utf-8-の妥当性--shift_jis-の順で決める) の順で決める。
  応答は `Cache-Control: private, no-cache` と、変換した本文のダイジェストの `ETag` を持ち、
  `If-None-Match` が一致すれば 304（生成物の配信と同じ扱い。guest-api.md §5）。
- 404 `subtitle_unavailable`（`reason`、`code` は `not_found`）: `file` が一覧に無い、開けない、
  空、上限を超える、復号できない、SRT の cue が 1 つも読めない、WebVTT のヘッダーが無い。
  理由はサーバーのログに `Warn` で残す（[R-7](../research.md#r-7-壊れたファイルは一覧には出し取得で-404-にする)）。
- 404 `video_not_found` / `file_unavailable`: §1 と同じ。
- 400 `invalid_request`: `offsetMs` の形式が違う。

## 3. プレイヤーの使い方

- 再生画面は動画を開いたとき（`VideoPlayer` を作る前後）に §1 を呼び、結果を `VideoPlayer` に
  渡す。取得に失敗したら字幕無しとして扱い、再生は止めない。
- `VideoPlayer` は再生の時間軸の offset が決まるたびに、既存の字幕トラックを外し、§2 の URL に
  その offset を `offsetMs` として付けて付け直す。直接再生では 0。ライブ変換では、`liveOffset.ts`
  が `transcode-start` の報告を待っている間は付けず、報告が届いた（または 404 で指定位置に
  決まった）時点の offset で付ける。source を作り直すシークでも同じ。
- 付け直しの前に表示していた字幕（ラベル）は、付け直したトラックでも `showing` にする。
  付け直しは保存値（[R-9](../research.md#r-9-字幕の選択は-websrcpreferences-に音量と同じ形で保存する)）を
  書き換えない。
