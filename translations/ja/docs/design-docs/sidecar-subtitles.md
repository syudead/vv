---
source: docs/design-docs/sidecar-subtitles.md
sourceHash: 0c83fc0510f2eb6dbda0df3a9ea0892e8df5ea4e79be40d6f68747cd10b4ba60
---

# 隣の字幕ファイル {#sidecar-subtitle-files}

- 状態: 採用
- 範囲: 動画のフォルダにある SRT と WebVTT の字幕ファイルの見つけ方、WebVTT への変換、`GET /api/videos/{id}/subtitles` と `GET /api/videos/{id}/subtitles/{file}` のアクセス制御、ライブ変換での再生中の字幕の時刻
- 背景: [specs/028-sidecar-subtitles/research.md](../../specs/028-sidecar-subtitles/research.md)。契約: [contracts/subtitles-api.md](../../specs/028-sidecar-subtitles/contracts/subtitles-api.md)

## 検出 {#discovery}

字幕はインデックスせず、リクエストのたびにフォルダを読んで見つける。SQLite のテーブルも取り込みジョブも使わない。フォルダに字幕を追加・削除すると、再スキャンなしで次の一覧に反映される。

1. `internal/httpapi/subtitles.go` は、配信 (`openMediaFile`) と同じ順で動画の所在を試す。`internal/mediafs` の `ListSidecarFiles` は、所在が `OpenMediaFile` と同じ規則で開けるときに限り、その所在のフォルダ(シンボリックリンクをたどる前のパス)にある通常ファイルの名前とサイズを返す。サブフォルダとシンボリックリンクは除く。使うのは最初に開けた所在のフォルダだけなので、動画が複数の場所にあるときは、再生に使う所在の隣にあるファイルだけが対象になる。
2. `domain.SubtitleSidecars` は、動画のファイル名とエントリから字幕一覧を組み立てる純粋関数だ。
   - `<name>` は動画ファイル名から最後の拡張子を除いたもの。候補は `<name>.srt`、`<name>.vtt`、`<name>.<label>.srt`、`<name>.<label>.vtt` で、`<name>` と拡張子は NFC 正規化したうえで大文字と小文字を区別せずに比較する。ラベルはドットを含んでよい (`en.forced`)。
   - `domain.SubtitleFileLimit`(4 MiB)より大きいエントリは候補にしない。
   - 同じラベルの `.srt` と `.vtt` があるときは、`.vtt` だけを残す。
   - ラベルのないエントリを先頭に置き、続けてラベルを自然順に並べる。
3. 一覧を作るときは内容を読まない。壊れたファイルも一覧に載り、取得すると 404 を返す。

| 状況 | 一覧の応答 |
| --- | --- |
| 開ける所在がない | 404 `file_unavailable`(配信と同じ) |
| 所在は開けるが、そのフォルダを読めない | 字幕がない場合と同じく空の一覧。理由はログに記録する |

一覧の応答には `Cache-Control: no-store` が付く。

## 取得と変換 {#fetching-and-conversion}

`GET /api/videos/{id}/subtitles/{file}` は一覧を組み立て直し、`file` が一覧の名前のいずれかと文字列として完全に一致するときに限り、`OpenSidecarFile` でそのファイルを開く。`OpenSidecarFile` も一覧にある名前しか受け付けず、開く前に `OpenMediaFile` の規則を再び適用する。そのため、一覧を作った後にファイルがシンボリックリンクに置き換えられても、登録済みのフォルダの外にあるものは開かない。パスをリクエストの文字列から組み立てることはないので、`..` や区切り文字の検査は要らない。一覧を作った後にファイルが大きくなっても、読むのは上限 (4 MiB) までだ。

`internal/media` の `SubtitleConverter` は Go だけで変換し、`ffmpeg` を起動しない。`internal/httpapi` は自身が宣言する `SubtitleConverter` インターフェース経由で変換器を受け取り、`cmd/mdm` が配線する。1 つのリクエストの中で読み込み、変換し、返すので、`internal/app` は関与しない。

- 文字コード: BOM(UTF-8、UTF-16 LE/BE)があれば BOM で決める。なければ次の順に判定する: 字幕で使われる文字体系の文字だけから成る妥当な UTF-8、`U+FFFD` も C1 制御文字も生じない Shift_JIS、妥当な UTF-8。どれにも当てはまらなければ、ファイルは読めないものとする。
- SRT: 連番の行を捨て、時刻行を WebVTT の形式に書き換え、キューのテキストはそのまま通す。時刻行を読めないキューは捨てる。WebVTT はヘッダーを確認したうえでそのまま通す。
- `offsetMs`(既定値 0)をすべてのキューの時刻から引く。終了が 0 以下になるキューは返さず、開始が負になるキューは 0 から始める。これで字幕をライブ変換の出力の時間軸に合わせる。プレーヤーは実際の開始位置を渡す。

応答には `Content-Type: text/vtt; charset=utf-8`、`Cache-Control: private, no-cache`、変換後の本文のダイジェストである `ETag` が付く。一致する `If-None-Match` には 304 を返す。

| 場合 | 応答 |
| --- | --- |
| 一覧にない名前、`.vtt` に隠された `.srt`、上限を超えるファイル、開けない・読めないファイル | 404 `subtitle_unavailable`。理由は `Warn` でログに記録する |
| `offsetMs` が負、または整数でない | 400 `invalid_request` |

## アクセス制御 {#access-control}

どちらのルートも「ゲストも可」のルートで(`api/openapi.yaml` の `security` が所有者とゲストを許可し、`internal/httpapi/auth.go` の `accessRoutes` がそれと揃えてある)、`lookupServedVideo` で動画を引く。ゲストが公開されていない動画を求めると、存在しない動画と同じ 404 `video_not_found` が返る。ゲストのリクエストは配信と同じ台帳に記録するので、動画を非公開にすると処理中のリクエストも打ち切られる。

## ライブ変換の時刻合わせ {#live-transcoding-time-alignment}

Video.js は、エミュレーションでもブラウザーのネイティブトラックでも、`<video>` 要素の `currentTime`、つまり変換出力の時間軸で字幕のキューを選ぶ。`liveOffset.ts` の仲介役が現在時刻に加えるオフセットは字幕には効かない。そのため、サーバーは `offsetMs` だけずらした WebVTT を返し、プレーヤーはオフセットが確定するたびにトラックを付け直す ([research.md R-6](../../specs/028-sidecar-subtitles/research.md))。

- `liveSource` のソースは、元の動画のどの時刻を再生時間軸の 0 とするかが確定するたびに、`vvOffsetSettled(seconds)` を 1 回呼ぶ。`attempt` のないソース(先頭から)は要求した位置ですぐに呼ぶ。`attempt` のあるソースは、`transcode-start` の報告が 200 を返したときは実際の開始位置で、404 またはエラーのときは要求した位置で呼ぶ。報告を待ち始めるときには `vvOffsetPending()` を呼ぶ。バッファーにない位置へのシークによる作り直し (`reloadAt`) も同じ経路を通り、ソースが置き換えられた後に届いた古い報告では何も呼ばない ([live-transcode-seek.md の開始位置の報告](live-transcode-seek.md#start-position-report))。
- `VideoPlayer.tsx` は両方を `subtitleTracks.ts` の `setOffset` に渡す。未確定 (`null`) の間は、ずれた字幕が一瞬でも表示されないようにトラックを外す。確定したら `subtitleUrl(id, file, offsetMs)` でトラックを付け直し、直前に表示していたラベルを `showing` にする。付け直しは利用者の選択ではないので、保存値を上書きしない。何を表示するかを保存値で決めるのは、一覧が変わった後の最初の取り付けだけだ。
- 直接再生ではオフセットを 0 のままにする。直接再生から変換への切り替え (`fallbackToTranscode`) も `liveSource` を通るので、同じ経路で時刻を合わせる。
- 付け直しは、変換が再開するとき(どのみち数秒かかる)にだけ起こる。その間はトラックがないので、字幕ボタンも隠れる。
