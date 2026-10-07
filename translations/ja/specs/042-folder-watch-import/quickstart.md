---
source: specs/042-folder-watch-import/quickstart.md
sourceHash: f312e3ad34faa4064b8980ad840adf5cd534aabd007d571af844eabfb4a2962e
---

# クイックスタート: メディアフォルダに加えたファイルがスキャンなしで現れる {#quickstart-files-added-to-a-media-folder-appear-without-a-scan}

この手順は、実際のファイルシステムと出荷するコンテナで、自動取り込みが変更に反応し、コピーが終わるのを待ち、何もしていない間は何も読まないことを示す。`task check` と `task test-e2e` は偽の監視でまとめる規則と削除の規則を確かめるが、ディスクへのアクセスは観察しない。

## 前提条件 {#prerequisites}

- ローカルのメディアフォルダを持つ、Linux の Docker で動く VVMDM ([Run with Docker](../../docs/how-to/running-vv.md))。アカウントを設定し、フォルダを Settings で加え、手動のスキャンを 1 回終えている。
- ホストの `strace` と、`docker inspect -f '{{.State.Pid}}' <container>` で得たコンテナのプロセス id。
- Windows の行には、`nightly` リリースの `VVMDM.exe` と、ローカルの NTFS ドライブ上のフォルダ。

## 手順 {#steps}

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| メディアフォルダのサブフォルダに動画をコピーする | 約 15 秒以内にサムネイルが生成された状態でライブラリに現れる。右下には進み具合が出ない | 1, 5 |
| その動画を別のサブフォルダに移動し、次に名前を変える | 同じ動画のままで、移動の前に設定したタグと再生位置を保つ | 2 |
| 動画を削除する | ライブラリから消える | 2 |
| 数 GB のファイルをコピーし、コピー中にライブラリを開く | コピーが終わるまでなく、その後に現れる | 3 |
| `strace -f -e trace=%file -p <pid>` を実行し、5 分間何も変えない | メディアフォルダの下のパスへの `openat`、`newfstatat`、`getdents64` がない | 4 |
| 内容が動画ではなく、動画の拡張子を持つファイルをコピーする | 失敗の通知が出る。Settings がそのファイルの問題を挙げる | 5 |
| Settings で自動取り込みをオフにし、動画をコピーする | 現れない。自動取り込みをオンに戻してもスキャンは始まらず、その動画は加わらない | 6 |
| Settings から手動のスキャンを始める | 完全なスキャンが今までどおり動き、前の行の動画を加える | 7 |
| Windows の `VVMDM.exe` で最初の 3 行を繰り返す | 同じ結果 | 1, 2 |
