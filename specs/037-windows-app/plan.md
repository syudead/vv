# Implementation Plan: Windows アプリ化（zip を展開して exe を実行するだけで専用ウィンドウで使える）

**Branch**: `feature/037-windows-app` | **Parent Issue**: #653

**Input**: The parent Issue. It is this feature's specification.

## Summary

Windows の利用者が zip を展開して `VVMDM.exe` を実行するだけで、`ffmpeg` の用意も環境変数も要らずに、
コンソール窓の無い専用ウィンドウで VVMDM を使えるようにする。ウィンドウを閉じればサーバーも止まる。

- **形**: `cmd/mdm` を `desktop` ビルドタグで Windows GUI の exe として組み、サーバーを同じプロセスで動かす
  （[research.md R-1](research.md#r-1-デスクトップ版は-cmdmdm-を-desktop-ビルドタグで-windows-gui-として組みサーバーを同じプロセスで動かす)）。
  ウィンドウは自前の Win32 ウィンドウに WebView2（`github.com/wailsapp/go-webview2`）を埋め込み、
  `http://localhost:47880/` を開く（[R-2](research.md#r-2-ウィンドウは自前の-win32-ウィンドウに-githubcomwailsappgo-webview2-の-pkgedge-で-webview2-を埋め込む)、
  [R-3](research.md#r-3-ウィンドウは-httplocalhostポート-を開く)、[R-4](research.md#r-4-ポートは固定の既定値-47880-で使えないときは理由を示して終わる--port-で変えられる)）。
- **データ・ffmpeg**: データは `%LOCALAPPDATA%\VVMDM`、同梱の `ffmpeg\` を `PATH` の先頭に足し、子プロセスは
  コンソール窓を出さずジョブオブジェクトに入れる（[R-5](research.md#r-5-データは-localappdatavvmdm-に置く)、
  [R-11](research.md#r-11-ffmpeg-は-exe-の隣の-ffmpeg-を-path-の先頭に足して使い子プロセスはコンソール窓を出さずジョブオブジェクトに入れる)、
  [R-12](research.md#r-12-同梱の-ffmpeg-は-gyandev-の-windows-版essentialsを版と-sha-256-で固定する)）。
- **閉じる・二重起動・失敗**: 取り込み中なら閉じる前に確認し、二重起動は既存のウィンドウを前面に出し、
  サインアウトでは確認なしで穏当に止める。起動の失敗は全てダイアログで理由を示す
  （[R-6](research.md#r-6-二重起動は利用者ごとセッションをまたぐ名前付きミューテックスで判定し既存のウィンドウを前面に出す)、
  [R-7](research.md#r-7-閉じる確認は走査中か未完了の取り込みの仕事があるときに出し閉じると決めたらウィンドウを先に消してから停止する)、
  [R-8](research.md#r-8-サインアウトシャットダウンでは確認を出さず停止を待つ理由を-windows-に示して同じ停止手順を通す)、
  [R-10](research.md#r-10-起動の失敗は全て-windows-標準のダイアログで理由を示しログを-logs-に書く)）。
  中断した走査は、全ての起動方法で次の起動時に始め直す（[R-9](research.md#r-9-起動時に最後の走査が中断interruptedで終わっていれば走査を自動で始め直す全ての起動方法で)）。
- **LAN**: 既定はループバックだけで待ち受け、設定画面の所有者だけの「Network」の節で LAN からの接続を
  許可でき、許可は設定表に残る（[R-14](research.md#r-14-lan-からの接続の許可は設定表に保存し切り替えたら待ち受けを開き直す既定はループバックだけ)、
  [R-15](research.md#r-15-lan-の許可は設定画面の所有者だけの節に置き初期設定の前は切り替えられない)、
  [contracts/network-settings-api.md](contracts/network-settings-api.md)）。
- **配布**: GitHub Actions がタグ `v*` で `VVMDM-<版>-windows-amd64.zip` を GitHub Release に添付する
  （[R-13](research.md#r-13-配布物は-github-actions-で作りタグ-v-で-github-release-に添付する)、
  [contracts/windows-app.md](contracts/windows-app.md)）。

`ui` ラベルは無いので design 段階は無い。画面の変更は設定画面の 1 節だけで、置き場所と示す内容は R-15 が決める。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・組み立ての場所・depguard: [ARCHITECTURE.md](../../ARCHITECTURE.md#intended-dependency-direction)、
  [.golangci.yml](../../.golangci.yml)
- 技術スタックと配布の形（Docker）: [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
- OS ごとのハードウェアエンコーダ: [docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md)
- 起動・環境変数・直接起動・ネットワークの公開: [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md)
- API: [api/openapi.yaml](../../api/openapi.yaml)。検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task generate`）
- 依存の更新: [docs/how-to/dependency-updates.md](../../docs/how-to/dependency-updates.md)

**Feature-specific context**:

- 新しい実行形態: `windows/amd64` の GUI サブシステムの exe（`-tags desktop -ldflags -H=windowsgui`）。
  `CGO_ENABLED=0` のまま Linux からクロスビルドする。
- 新しい依存: `github.com/wailsapp/go-webview2`（`go.mod`、Windows のビルドだけが読む）、
  `github.com/tc-hib/go-winres`（`tools/go.mod` の `tool`）。同梱物として FFmpeg（Gyan.dev essentials、版と SHA-256 で固定）。
- 実行時の前提: WebView2 ランタイム（Windows 10/11 に既定で入る。無ければ R-10 のダイアログ）。
- 新しい保存値: 既存の `settings` 表の鍵 `desktop.lan_access`。移行は足さない。それ以外のデータモデルは
  変えない（`data-model.md` は作らない）。
- 全ての起動方法に効く振る舞いの変更: 中断した走査の自動の始め直し（R-9）と、`ffmpeg`/`ffprobe` の
  `CREATE_NO_WINDOW`（R-11、Windows のみ）。

## Constitution Check

| 規則（出典） | 判定 |
| --- | --- |
| 依存は `cmd → internal/{adapters, app} → internal/domain` の一方向、兄弟の adapter は互いに読まない（ARCHITECTURE.md、depguard） | 満たす。Win32 と WebView2 を扱う新しい adapter `internal/desktop` は `cmd/mdm` だけが読み、他の `internal/*` を読まない。depguard の兄弟の規則に足す |
| `cmd/mdm` は組み立てだけで、ユースケースを持たない（ARCHITECTURE.md） | 満たす。閉じる確認に要る「取り込み中か」は `app.Scans.Busy` に置き、`cmd/mdm` はつなぐだけ。待ち受けの開き直しは開始・停止の配線で、組み立ての役目に入る |
| `internal/app` は `os/exec`・`net/http`・DB を読まない（ARCHITECTURE.md） | 満たす。`Busy` は app が宣言する役割の型（未完了の仕事の有無）を通す |
| 生成物を手で変えない（AGENTS.md） | 満たす。`/api/settings/network` は `api/openapi.yaml` を変えて `task generate` |
| 制約は検査で守る（core-beliefs.md） | 満たす。`build-windows-check` に `-tags desktop` のビルドを足し、`task lint` に `GOOS=windows`・`desktop` タグでの lint を足して、Windows 専用のファイルも CI で検査する |
| 文書は振る舞いの変更と同じ変更で直す（core-beliefs.md、AGENTS.md） | 文書の節ごとに持ち主の実装単位を 1 つ決め（[文書の持ち分](#文書の持ち分)）、単位どうしで同じ節を書かない。新しい設計文書 `docs/design-docs/windows-app.md` を索引に載せる |
| Docker と直接起動はこれまでどおり使える（親 Issue 要件 11） | 満たす。タグなしのビルドの入口・環境変数・イメージは変えない。全起動方法に効く変更は R-9 と R-11 だけ |

設計の後に見直しても違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/037-windows-app/
├── plan.md
├── research.md                      # R-1..R-15
├── quickstart.md                    # 実機での確かめ方（CI で見られない部分）
└── contracts/
    ├── network-settings-api.md      # /api/settings/network
    └── windows-app.md               # zip の中身・起動の引数と失敗の表示・データの置き場・ウィンドウ
```

`data-model.md` は作らない。足す保存値は既存の `settings` 表の鍵 1 つで、R-14 と contracts に書いた。

### Source Code

**Affected boundaries**:

- `cmd/mdm`: `run()` を、設定・ログの書き先・待ち受けのアドレス・起動の通知・停止の指示を引数に取る
  起動・停止の手順に分け、タグなしの `main` とデスクトップ版の `main` の両方がそれを呼ぶ。
  待ち受けを開き直せる部品（R-14）と、LAN の許可の読み書きをつなぐ。
- `internal/app`: `Scans.Busy`（R-7）と、起動時の走査の始め直し（R-9）。
- `internal/store`: `desktop.lan_access` の読み書き、未完了の仕事の有無の問い合わせ。
- `internal/media`: 子プロセスの起こし方を 1 つの関数に寄せ、Windows で `CREATE_NO_WINDOW` を付ける。
- `internal/httpapi`・`api/openapi.yaml`: `/api/settings/network`。
- `web/src/settings`: 「Network」の節。
- ビルドと配布: `scripts/build`、`Taskfile.yml`、`tools/go.mod`、`.github/workflows/windows-app.yml`。

**New paths**: `internal/desktop/`（ウィンドウ・WebView2・ダイアログ・ミューテックス・ジョブオブジェクト・
データの置き場の解決。Windows の実装は `_windows.go`）、`cmd/mdm/desktop_windows.go` ほか `desktop` タグの
ファイル、`cmd/mdm/winres/`（アイコンと manifest の入力）、`.github/workflows/windows-app.yml`、
`docs/design-docs/windows-app.md`。

**Structure decision**: Win32 と WebView2 の扱いは新しい adapter `internal/desktop` に置き、`cmd/mdm` の
`desktop` タグのファイルは組み立てと配線だけを持つ。却下した案: 全てを `cmd/mdm` に置く — 組み立ての場所に
ウィンドウ手続きやダイアログの実装が入り、depguard で他の adapter から切り離せない。

### 文書の持ち分

振る舞いを変えた単位が、その節を同じ PR で書く（core-beliefs.md）。同じ節を 2 つの単位が書かないよう、
節ごとに持ち主を 1 つに決める。`docs/design-docs/windows-app.md` は「Windows デスクトップ版」の単位が
節の見出しごと作り、後の単位は自分の節の本文だけを書く。

| 文書 | 節 | 持ち主の単位 |
| --- | --- | --- |
| ARCHITECTURE.md | 起動時の回復（Intended topology の「In place today」の段落の走査の回復） | 起動時に、中断で終わった走査を自動で始め直す |
| ARCHITECTURE.md | Intended topology の配布の形、Intended dependency direction の `internal/desktop` | Windows デスクトップ版 `VVMDM.exe`: 専用ウィンドウで開き、閉じたらサーバーを止める |
| ARCHITECTURE.md | API の一覧の `/api/settings/network` | LAN からの接続の許可を保存し、`/api/settings/network` で切り替える |
| docs/design-docs/windows-app.md と索引 | 文書の作成、「プロセスとウィンドウ」「データと ffmpeg」「起動の失敗」 | Windows デスクトップ版 `VVMDM.exe`: 専用ウィンドウで開き、閉じたらサーバーを止める |
| docs/design-docs/windows-app.md | 「閉じる・二重起動・サインアウト」 | 閉じる前の確認、二重起動の前面化、サインアウト時の穏当な停止 |
| docs/design-docs/windows-app.md | 「LAN からの接続」 | LAN からの接続の許可を保存し、`/api/settings/network` で切り替える |
| docs/design-docs/windows-app.md | 「配布」 | Windows 版の zip を作るビルドと、タグで GitHub Release に添付する workflow |
| docs/how-to/running-vv.md | Data and recovery | 起動時に、中断で終わった走査を自動で始め直す |
| docs/how-to/running-vv.md | 新しい「Windows app」の節（入手・起動・SmartScreen・データの場所・更新・`--port`・LAN の許可） | Windows 版の zip を作るビルドと、タグで GitHub Release に添付する workflow |
| docs/design-docs/tech-stack-selection.md | 配布の形 | Windows 版の zip を作るビルドと、タグで GitHub Release に添付する workflow |
| docs/how-to/dependency-updates.md | FFmpeg の版の上げ方 | Windows 版の zip を作るビルドと、タグで GitHub Release に添付する workflow |

## Implementation Work

### 起動時に、中断で終わった走査を自動で始め直す

**Scope**: 起動時に最新の走査が `failed`・`interrupted` ならワーカーを動かしたあとに走査を 1 回始める
（[research.md R-9](research.md#r-9-起動時に最後の走査が中断interruptedで終わっていれば走査を自動で始め直す全ての起動方法で)）。
全ての起動方法に効く。[文書の持ち分](#文書の持ち分)のこの単位の節。

**Dependencies**: None

**Acceptance**: `task check` が通る。app の試験で、最新の走査が `interrupted` の起動では走査が 1 回始まり、
`done` や `interrupted` 以外の `failed`、走査の記録が無い場合は始まらない。cmd/mdm の試験か手元の確認で、
走査の途中で SIGTERM して起動し直すと、`GET /api/scans/current` が新しい `running` の走査を返し、終わると
止める前に見つかっていなかったファイルも取り込まれている。

### `ffmpeg`/`ffprobe` を Windows ではコンソール窓を出さずに起動する

**Scope**: `internal/media` の 6 か所の子プロセスの起こし方を 1 つの関数に寄せ、Windows では
`CREATE_NO_WINDOW` を付ける（[R-11](research.md#r-11-ffmpeg-は-exe-の隣の-ffmpeg-を-path-の先頭に足して使い子プロセスはコンソール窓を出さずジョブオブジェクトに入れる)）。

**Dependencies**: None

**Acceptance**: `task check`（`build-windows-check` を含む）が通る。`internal/media` で `exec.Command`/
`exec.CommandContext` を直接呼ぶのはその関数だけになっている。`GOOS=windows` で組んだ media の試験
（`go test -c` で組めること、Windows の試験で `SysProcAttr.CreationFlags` に `CREATE_NO_WINDOW` が入ること）。

### `cmd/mdm` の起動と停止を、設定と待ち受けを外から渡せる形に分ける

**Scope**: `run()` を、`Config`・ログの書き先・待ち受けを作る部品・待ち受け後の通知・停止の指示を引数に
取る手順に分け、タグなしの `main` は今の環境変数・標準出力・`MDM_ADDR`・SIGINT/SIGTERM を渡す。
待ち受けを作る部品は、アドレスを変えて開き直せる（[R-14](research.md#r-14-lan-からの接続の許可は設定表に保存し切り替えたら待ち受けを開き直す既定はループバックだけ)
の「待ち受け」）。`app.Scans.Busy`（[R-7](research.md#r-7-閉じる確認は走査中か未完了の取り込みの仕事があるときに出し閉じると決めたらウィンドウを先に消してから停止する)）と、
そのための store の問い合わせ。振る舞いは変えない。

**Dependencies**: None

**Acceptance**: `task check` が通る。今の `serveUntil` の試験が通る。待ち受けの部品の試験で、`127.0.0.1:<p>` から
`0.0.0.0:<p>` に開き直したあと、ループバックでない自分のアドレスから接続でき、戻すと接続が拒まれ、
開き直しの間も確立済みの接続（SSE）が切れない。開き直しに失敗すると元のアドレスで待ち受けが残る。
`Busy` の試験で、走査中・`queued`・`running` の仕事があるとき真、どれも無いとき偽。`task up` のコンテナが
今までどおり `/api/health` に応える。

### Windows デスクトップ版 `VVMDM.exe`: 専用ウィンドウで開き、閉じたらサーバーを止める

**Scope**: `internal/desktop`（ウィンドウ、WebView2 の埋め込みと全画面・描画プロセスの異常からの再読み込み、
ダイアログ、ジョブオブジェクト、データの置き場の解決）と、`cmd/mdm` の `desktop` タグの `main`: 引数
（`--port`）、R-10 の起動前の確認とダイアログ、ログのファイル、`PATH` への `ffmpeg\` の追加、
`%LOCALAPPDATA%\VVMDM` の設定、転送ヘッダを読まない設定、待ち受け後にウィンドウを出し、閉じたら
（確認なしで）停止手順を通して終わる（[R-1](research.md#r-1-デスクトップ版は-cmdmdm-を-desktop-ビルドタグで-windows-gui-として組みサーバーを同じプロセスで動かす)〜[R-5](research.md#r-5-データは-localappdatavvmdm-に置く)、
[R-10](research.md#r-10-起動の失敗は全て-windows-標準のダイアログで理由を示しログを-logs-に書く)、R-11、
[contracts/windows-app.md §2〜§4](contracts/windows-app.md#2-起動の引数と失敗時の表示)）。
depguard に `internal/desktop` を足す。`build-windows-check` に `-tags desktop` のビルド、`task lint` に
`GOOS=windows`・`desktop` タグの lint を足す。[文書の持ち分](#文書の持ち分)のこの単位の節
（`docs/design-docs/windows-app.md` の作成と索引を含む）。

**Dependencies**: 「`cmd/mdm` の起動と停止を、設定と待ち受けを外から渡せる形に分ける」、
「`ffmpeg`/`ffprobe` を Windows ではコンソール窓を出さずに起動する」

**Acceptance**: `task check` が通り、`-tags desktop` の Windows 版が組める。データの置き場の解決と引数の
読み取りの試験が通る。Windows の実機で、組んだ exe の隣に `ffmpeg\` を置いて
[quickstart.md](quickstart.md) の手順 1・2・4・5 と Edge Cases の「ポート」「zip から直接」「書けない場所」
「ネットワークドライブ」「全画面」が書いたとおりになる。画面に関わるので、ウィンドウの見た目と操作を確かめる。

### 閉じる前の確認、二重起動の前面化、サインアウト時の穏当な停止

**Scope**: `WM_CLOSE` で `Busy` を読んで確認を出す（[R-7](research.md#r-7-閉じる確認は走査中か未完了の取り込みの仕事があるときに出し閉じると決めたらウィンドウを先に消してから停止する)）、
名前付きミューテックスと既存のウィンドウの前面化（[R-6](research.md#r-6-二重起動は利用者ごとセッションをまたぐ名前付きミューテックスで判定し既存のウィンドウを前面に出す)）、
`WM_QUERYENDSESSION`/`WM_ENDSESSION`（[R-8](research.md#r-8-サインアウトシャットダウンでは確認を出さず停止を待つ理由を-windows-に示して同じ停止手順を通す)）。
[文書の持ち分](#文書の持ち分)のこの単位の節。

**Dependencies**: 「Windows デスクトップ版 `VVMDM.exe`: 専用ウィンドウで開き、閉じたらサーバーを止める」、
「起動時に、中断で終わった走査を自動で始め直す」

**Acceptance**: `task check` が通る。Windows の実機で [quickstart.md](quickstart.md) の手順 6 と Edge Cases の
「二重起動」「サインアウト」が書いたとおりになる。確認のダイアログの見た目と操作を確かめる。

### LAN からの接続の許可を保存し、`/api/settings/network` で切り替える

**Scope**: `api/openapi.yaml` と `task generate`、`internal/httpapi` の経路（デスクトップ版でなければ `404`）、
store の `desktop.lan_access`、`cmd/mdm` で起動時に保存値から待ち受けのアドレスを決め、`PUT` で待ち受けを
開き直してから保存する配線（[contracts/network-settings-api.md](contracts/network-settings-api.md)、
[R-14](research.md#r-14-lan-からの接続の許可は設定表に保存し切り替えたら待ち受けを開き直す既定はループバックだけ)）。
[文書の持ち分](#文書の持ち分)のこの単位の節。

**Dependencies**: 「`cmd/mdm` の起動と停止を、設定と待ち受けを外から渡せる形に分ける」、
「Windows デスクトップ版 `VVMDM.exe`: 専用ウィンドウで開き、閉じたらサーバーを止める」

**Acceptance**: `task check` が通る。httpapi の試験で、契約の §2・§3 の表の各行の応答が返る（デスクトップ版で
ないとき `404`、ゲスト `401`、別サイトから `403`、開き直しの失敗で `409` `listen_failed` と保存値が変わらない、
保存の失敗で `500` と待ち受けが元のアドレスに戻る）。store の試験で、
行が無いと偽、保存した値が読める。cmd/mdm の試験で、保存値が真なら `0.0.0.0` で、偽か行が無ければ
`127.0.0.1` で待ち受ける。`addresses` は許可中だけ、ループバックを含まず入る。

### 設定画面の「Network」の節で LAN からの接続を許可し、開くアドレスを示す

**Scope**: `web/src/settings` に節を足し、`404` なら出さない。スイッチ、許可中のアドレス、オンにするときの
注意（[R-15](research.md#r-15-lan-の許可は設定画面の所有者だけの節に置き初期設定の前は切り替えられない)）、
`409` `listen_failed` の表示。文言は `web/src/i18n/en.ts`。文書は書かない（running-vv.md の LAN の許可は zip の単位が書く）。

**Dependencies**: 「LAN からの接続の許可を保存し、`/api/settings/network` で切り替える」

**Acceptance**: `task check` が通る。コンポーネントの試験で、`404` のとき節が無く、`lanAccess` が偽のとき
アドレスが無く注意が出て、スイッチで `PUT` が送られ、真のとき返った `addresses` が並び、`409` で誤りが出て
スイッチが元に戻る。Windows の実機で [quickstart.md](quickstart.md) の手順 7 が書いたとおりになる。画面が
変わるので、節の見た目と操作を確かめる。

### Windows 版の zip を作るビルドと、タグで GitHub Release に添付する workflow

**Scope**: `scripts/build` に Windows 版の zip の組み立て（FFmpeg の版と SHA-256 の固定と取得、
`go-winres` によるアイコンと manifest、`README.txt`）、`task build-windows-app`、`tools/go.mod` に `go-winres`、
`.github/workflows/windows-app.yml`（タグ `v*` と手動実行、Windows のジョブでのエンコーダの確認、Release への添付）
（[R-12](research.md#r-12-同梱の-ffmpeg-は-gyandev-の-windows-版essentialsを版と-sha-256-で固定する)、
[R-13](research.md#r-13-配布物は-github-actions-で作りタグ-v-で-github-release-に添付する)、
[contracts/windows-app.md §1](contracts/windows-app.md#1-zip-の中身)）。running-vv.md に Windows 版の節（入手・
起動・SmartScreen・データの場所・更新・`--port`・LAN の許可）ほか、[文書の持ち分](#文書の持ち分)のこの単位の節。

**Dependencies**: 「Windows デスクトップ版 `VVMDM.exe`: 専用ウィンドウで開き、閉じたらサーバーを止める」、
「設定画面の「Network」の節で LAN からの接続を許可し、開くアドレスを示す」（running-vv.md に LAN の許可を書くため）

**Acceptance**: `task check` が通る。`task build-windows-app` が `dist/VVMDM-<版>-windows-amd64.zip` を作り、
中身が契約 §1 のとおりで、SHA-256 が合わない FFmpeg ではビルドが失敗する。workflow の手動実行が成功し、
Windows のジョブが `h264_nvenc` と `h264_qsv` を確かめ、成果物に zip が残る。ffmpeg の無い Windows の実機で、
その zip から [quickstart.md](quickstart.md) の手順 1〜3 と 8 が書いたとおりになる（8 は、この単位の zip と、
その後の変更で作った zip で確かめる）。
