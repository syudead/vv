# 動画の隣に置いた字幕ファイル

- ステータス: 採用
- スコープ: 動画と同じフォルダにある SRT・WebVTT の字幕ファイルの見つけ方、WebVTT への変換、
  `GET /api/videos/{id}/subtitles` と `GET /api/videos/{id}/subtitles/{file}` のアクセス制御
- 経緯: [specs/028-sidecar-subtitles/research.md](../../specs/028-sidecar-subtitles/research.md)、
  契約: [contracts/subtitles-api.md](../../specs/028-sidecar-subtitles/contracts/subtitles-api.md)

## 見つけ方

字幕は索引に持たず、要求のたびにフォルダを読んで見つける。SQLite の表も、取り込みの job も
関わらない。フォルダに字幕を足したり消したりすると、再スキャンなしに次の一覧から反映される。

1. `internal/httpapi/subtitles.go` が、配信（`openMediaFile`）と同じ順で動画の所在を試す。
   `internal/mediafs` の `ListSidecarFiles` は、所在が `OpenMediaFile` と同じ規則で開けるときだけ、
   その所在（symlink を辿る前のパス）のフォルダにある通常ファイルの名前と大きさを返す。
   サブフォルダと symlink は含めない。最初に開けた所在のフォルダだけを使うので、動画が複数の
   場所にあっても、再生に使う場所の隣だけが対象になる。
2. `domain.SubtitleSidecars` が、動画のファイル名と項目から字幕の一覧を作る純粋関数である。
   - `<名前>` は動画のファイル名から最後の拡張子だけを除いたもの。候補は `<名前>.srt`・
     `<名前>.vtt`・`<名前>.<ラベル>.srt`・`<名前>.<ラベル>.vtt` で、`<名前>` と拡張子は NFC
     正規化のうえ大文字・小文字を区別せずに照合する。ラベルはドットを含んでよい（`en.forced`）。
   - `domain.SubtitleFileLimit`（4 MiB）を超える項目は候補にしない。
   - 同じラベルの `.srt` と `.vtt` は `.vtt` だけを残す。
   - 並びはラベルの無いものが先頭で、続いてラベルの自然順。
3. 一覧は中身を読まない。壊れたファイルも一覧には載り、取得で 404 になる。

どの所在も開けなければ一覧は 404 `file_unavailable`（配信と同じ）。所在は開けてもフォルダを
読めなければ、字幕が無いのと同じく空の一覧を返し、理由をログに残す。一覧の応答は
`Cache-Control: no-store` である。

## 取得と変換

`GET /api/videos/{id}/subtitles/{file}` は一覧を作り直し、`file` がその中の 1 つと文字列で完全に
一致するときだけ `OpenSidecarFile` で開く。`OpenSidecarFile` も一覧に出た名前しか受けず、開く前に
`OpenMediaFile` の規則を通し直すので、一覧のあとに symlink へ差し替えられても登録フォルダの外は
開かない。要求の文字列からパスを組み立てないので、`..` や区切りの検査は要らない。
読むのは上限（4 MiB）までで、一覧のあとに大きくなったファイルも上限を超えては読まない。

変換は `internal/media` の `SubtitleConverter` が Go だけで行い、`ffmpeg` は起動しない。
`internal/httpapi` はこれを自分が宣言する `SubtitleConverter` interface で受け取り、`cmd/mdm` が
配線する。要求 1 回で読んで変換して返すので、`internal/app` は通さない。

- 文字コード: BOM（UTF-8・UTF-16 LE/BE）があればそれで決める。無ければ、字幕に使われる
  文字体系の文字だけから成る妥当な UTF-8、次に `U+FFFD` も C1 制御文字も出ない Shift_JIS、
  次に妥当な UTF-8 の順で決め、どれでもなければ読めないファイルとする。
- SRT は、番号行を除き、時刻の行を WebVTT の形に揃え、cue の本文はそのまま通す。時刻の行が
  読めない cue は落とす。WebVTT はヘッダーを確かめて通す。
- `offsetMs`（既定 0）だけすべての cue の時刻を引く。終わりが 0 以下になる cue は返さず、
  始まりが負になる cue は 0 から始める。ライブ変換の出力の時間軸に字幕を合わせるためで、
  プレイヤーが実際の開始位置を渡す。

応答は `Content-Type: text/vtt; charset=utf-8`、`Cache-Control: private, no-cache` と、変換した
本文のダイジェストの `ETag` を持ち、`If-None-Match` が一致すれば 304 を返す。一覧に無い名前、
`.vtt` に隠れた `.srt`、上限を超える・開けない・読めないファイルは 404 `subtitle_unavailable` で、
理由はサーバーのログに `Warn` で残す。`offsetMs` が負や整数でなければ 400 `invalid_request`。

## アクセス制御

2 つの経路はどちらも「ゲストも」の経路で（`api/openapi.yaml` の `security` が所有者とゲスト、
`internal/httpapi/auth.go` の `accessRoutes` がその写し）、動画を `lookupServedVideo` で引く。
ゲストが公開でない動画を指すと、存在しない動画と同じ 404 `video_not_found` になる。ゲストの
要求は配信と同じ台帳に載るので、動画を非公開にすると処理中の要求も打ち切られる。
