# Contract: Windows 版の zip・起動・データの置き場

利用者と配布の手順から見える Windows 版の形。決定の理由は [research.md](../research.md) の各節にある。
直接起動・Docker の形（`MDM_*`、`mdm account`、イメージ）は
[docs/how-to/running-vv.md](../../../docs/how-to/running-vv.md) が正本で、この feature では変えない。

## 1. zip の中身

名前は `VVMDM-<版>-windows-amd64.zip`。`<版>` はタグ（`v` を除く）か、手動実行なら `sha-<12 桁>`。

```text
VVMDM-<版>-windows-amd64/
├── VVMDM.exe            # -tags desktop、-H=windowsgui、アイコンと manifest を埋め込み
├── ffmpeg/
│   ├── ffmpeg.exe       # Gyan.dev essentials（research.md R-12）
│   ├── ffprobe.exe
│   ├── LICENSE.txt      # FFmpeg の同梱物の LICENSE
│   └── README.txt       # FFmpeg の版とソースの入手先
└── README.txt           # 展開して VVMDM.exe を実行すること、SmartScreen の通し方、データの場所、更新の仕方
```

## 2. 起動の引数と失敗時の表示

| 引数 | 意味 |
| --- | --- |
| （なし） | ポート `47880` で起動する |
| `--port <1-65535>` | そのポートで起動する |
| それ以外 | 「使えない引数」のダイアログを出して終わる |

起動時の確認は上から順に行い、最初に失敗したものだけを Windows 標準のダイアログ（題名 `VVMDM`、
OK だけ）で示して終わる。どの文にもログの場所（`%LOCALAPPDATA%\VVMDM\logs\vvmdm.log`）を添える
（ログを書けない 2 番目を除く）。文言は英語で、意味は次のとおり。

| # | 確認 | 失敗したときに示すこと |
| --- | --- | --- |
| 1 | `VVMDM.exe` が一時フォルダの下になく、隣に `ffmpeg\ffmpeg.exe` と `ffmpeg\ffprobe.exe` がある | zip を「すべて展開」してから、展開したフォルダの `VVMDM.exe` を実行すること |
| 2 | `%LOCALAPPDATA%\VVMDM` の下に書ける | 書けなかったフォルダのパス |
| 3 | WebView2 ランタイムがある | WebView2 ランタイムが要ることと、入手先の URL |
| 4 | DB を開いて移行できる | 開けなかったこと（エラーの要約） |
| 5 | ポートで待ち受けられる | ポート番号、他のプログラムが使っている可能性、`--port` で変えられること |

二重起動（[research.md R-6](../research.md#r-6-二重起動は名前付きミューテックスで判定し既存のウィンドウを前面に出す)）は
失敗として扱わず、ダイアログを出さない。

## 3. データの置き場

| パス | 中身 |
| --- | --- |
| `%LOCALAPPDATA%\VVMDM\data\mdm.db`（と `-wal`/`-shm`） | Docker・直接起動の `MDM_DATA_DIR` と同じ中身 |
| `%LOCALAPPDATA%\VVMDM\data\thumbnails\` | 生成物 |
| `%LOCALAPPDATA%\VVMDM\webview2\` | WebView2 の利用者データ（Cookie・`localStorage`） |
| `%LOCALAPPDATA%\VVMDM\logs\vvmdm.log`、`vvmdm.1.log` | 今回と前回の JSON ログ |

zip を新しい版で置き換えても、ここは残る（要件 10）。新しい版は起動時に今の移行を当てる。

## 4. ウィンドウ

- 題名 `VVMDM`、ウィンドウクラス名 `VVMDMWindow`（二重起動の検出に使う）、初期サイズ 1280×800（画面より
  大きければ画面に収める）、`http://localhost:<ポート>/` を開く。
- 動画の全画面は、ウィンドウを枠なしで画面いっぱいにする。全画面を抜けると元の位置とサイズに戻る。
- 閉じる操作の確認（[research.md R-7](../research.md#r-7-閉じる確認は走査中か未完了の取り込みの仕事があるときに出し閉じると決めたらウィンドウを先に消してから停止する)）は
  Windows 標準のダイアログで、ボタンは「閉じる」「続ける」に当たる 2 つ（既定は「続ける」）。
