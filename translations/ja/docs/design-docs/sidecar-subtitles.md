---
source: docs/design-docs/sidecar-subtitles.md
sourceHash: e057b548b8a3428490dbe51f28da7b00a8830e6958dc968be38fc48a91a2f31f
---

# 隣の字幕ファイル {#sidecar-subtitle-files}

VVMDM は、動画の隣に置かれた SRT と WebVTT のファイルを字幕として表示し、リクエストのたびに WebVTT へ変換する ([`internal/httpapi/subtitles.go`](../../internal/httpapi/subtitles.go))。背景: [research.md](../../specs/028-sidecar-subtitles/research.md)。API: [contracts/subtitles-api.md](../../specs/028-sidecar-subtitles/contracts/subtitles-api.md)。

## 検出 {#discovery}

字幕はインデックスせず、リクエストのたびに動画のフォルダを読んで見つける ([`domain.SubtitleSidecars`](../../internal/domain))。

そのため、ファイルを追加・削除すると再スキャンなしで次の一覧に反映され、フォルダと同期させておくテーブルやジョブも要らない。一覧では名前とサイズだけを読むので、負荷は小さいままだ。

| 規則 | 動作 |
| --- | --- |
| 対象のフォルダ | 再生に使う所在のフォルダだけ |
| 対象のファイル | `<name>.srt`、`<name>.vtt`、`<name>.<label>.srt`、`<name>.<label>.vtt`。`<name>` は動画名から拡張子を除いたもので、NFC 正規化したうえで大文字と小文字を区別せずに比較する |
| サイズ | 4 MiB を超えるものは一覧に載せない |
| 同じラベルの `.srt` と `.vtt` | `.vtt` だけを載せる |
| 順序 | ラベルのないものを先頭に置き、続けてラベルを自然順に並べる |
| 壊れたファイル | 一覧に載る。取得すると 404 を返す |
| 開ける所在がない | 404 `file_unavailable` |
| フォルダを読めない | 空の一覧。理由はログに記録する |

| 採用しなかった案 | 理由 |
| --- | --- |
| スキャン中に字幕をインデックスする | 後から追加した字幕は、再スキャンしないと表示されない |

## 取得と変換 {#fetching-and-conversion}

ファイルは、その名前が新たに組み立てた一覧にあるときに限り返す。変換は `ffmpeg` を使わずに Go で実行する ([`internal/media`](../../internal/media))。

一覧と照合するので、リクエストの文字列がパスになることはなく、`..` でメディアフォルダの外に出ることはできない。開くときにメディアフォルダの規則を再び確認するので、一覧を作った後にシンボリックリンクに置き換えられたファイルはたどらない。

| 入力 | 動作 |
| --- | --- |
| 文字コード | BOM があれば BOM に従う。なければ UTF-8、次に Shift_JIS を試し、どちらでもなければ失敗する |
| SRT | WebVTT に書き換える。時刻を読めないキューは捨てる |
| `offsetMs` | すべてのキューから引く。終了が 0 以下になるキューは捨てる |
| 一覧にない名前、`.vtt` に隠された `.srt`、4 MiB 超、読めない | 404 `subtitle_unavailable` |
| 不正な `offsetMs` | 400 `invalid_request` |
| 一致する `If-None-Match` | 304 |

| 採用しなかった案 | 理由 |
| --- | --- |
| `ffmpeg` で変換する | 文字コードを判定しないので、いずれにせよ Go で判定する必要がある。残るのは小さなテキストの書き換えで、リクエストごとにプロセスを起動するほどの価値はない |

## アクセス制御 {#access-control}

ゲストは、再生してよい動画の字幕を、動画そのものと同じ規則で取得してよい ([`internal/httpapi/auth.go`](../../internal/httpapi/auth.go))。

ゲストが非公開の動画を求めると、存在しない動画と同じく 404 `video_not_found` が返る。動画を非公開にすると、処理中のリクエストも打ち切られる。

## ライブ変換の時刻合わせ {#live-transcoding-time-alignment}

ライブ変換中は、サーバーが字幕を `offsetMs` だけずらし、プレーヤーは開始位置が確定するたびにトラックを付け直す ([`subtitleTracks.ts`](../../web/src/player/subtitleTracks.ts))。

プレーヤーは、シーク位置を 0 として始まる変換後のストリームの時刻でキューを選ぶ。プレーヤーが自分の時計に加えるオフセットは字幕には届かないので、サーバーが字幕をずらす必要がある ([research.md R-6](../../specs/028-sidecar-subtitles/research.md))。

| 状態 | トラック |
| --- | --- |
| 直接再生 | オフセット 0 |
| 開始位置を待っている | 外す。そのため、ずれた字幕は表示されない |
| 開始位置が確定した | 新しいオフセットで付け直す。直前に表示していたラベルを引き続き表示する |

付け直しは変換が再開するときにだけ起こり、それにはどのみち数秒かかる。その間、字幕ボタンは隠れる。
