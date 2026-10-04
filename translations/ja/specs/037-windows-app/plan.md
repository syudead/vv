---
source: specs/037-windows-app/plan.md
sourceHash: 4cb1ef4b936ac58f25e3ff4bc2ea3e01c94706d362f2d25661f03a849a544a1d
---

# 実装計画: Windows アプリ (zip を展開し、exe を実行し、VVMDM を専用のウィンドウで使う) {#implementation-plan-windows-app-extract-the-zip-run-the-exe-and-use-vvmdm-in-its-own-window}

**ブランチ**: `feature/037-windows-app` | **親 Issue**: #653

**入力**: 親 Issue。これがこの機能の仕様である。

## 概要 {#summary}

Windows のユーザーは zip を展開して `VVMDM.exe` を実行し、VVMDM はコンソールウィンドウなしで専用のウィンドウに開く。インストールする `ffmpeg` も、設定する環境変数もない。ウィンドウを閉じるとサーバーも止まる。

| 関心事 | 方針 |
| --- | --- |
| 形 | `cmd/mdm` を `desktop` ビルドタグ付きで Windows の GUI の exe としてビルドし、サーバーは同じプロセスで動く ([research.md R-1](research.md#r-1-the-desktop-build-compiles-cmdmdm-as-a-windows-gui-under-the-desktop-build-tag-and-runs-the-server-in-the-same-process))。ウィンドウは WebView2 (`github.com/wailsapp/go-webview2`) を埋め込んだ独自の Win32 ウィンドウで、`http://localhost:47880/` を開く ([R-2](research.md#r-2-the-window-embeds-webview2-with-pkgedge-from-githubcomwailsappgo-webview2-in-vvmdms-own-win32-window)、[R-3](research.md#r-3-the-window-opens-httplocalhost-on-the-listening-port)、[R-4](research.md#r-4-a-fixed-default-port-47880-changed-with---port-an-unavailable-port-ends-startup-with-the-reason))。 |
| データと ffmpeg | データは `%LOCALAPPDATA%\VVMDM` に置く。同梱の `ffmpeg\` を `PATH` の先頭に加え、子プロセスはコンソールウィンドウを開かず、ジョブオブジェクトの中で動く ([R-5](research.md#r-5-data-lives-in-localappdatavvmdm)、[R-11](research.md#r-11-ffmpeg-comes-from-the-ffmpeg-folder-next-to-the-exe-added-first-to-path-and-child-processes-run-without-a-console-window-inside-a-job-object)、[R-12](research.md#r-12-the-bundled-ffmpeg-is-gyandevs-windows-essentials-build-pinned-by-version-and-sha-256))。 |
| 終了、2 回目の起動、失敗 | 取り込み中に閉じると先に確認する。2 回目の起動は既存のウィンドウを前面に出す。サインアウトでは確認せずに正常に止まる。起動時のどの失敗もダイアログに理由を示す ([R-6](research.md#r-6-a-second-launch-is-detected-by-a-per-user-named-mutex-across-sessions-and-the-existing-window-comes-to-the-front)、[R-7](research.md#r-7-the-close-confirmation-appears-while-a-scan-runs-or-import-jobs-are-unfinished-and-closing-hides-the-window-before-the-shutdown)、[R-8](research.md#r-8-sign-out-and-shutdown-show-no-confirmation-give-windows-a-reason-to-wait-and-run-the-same-shutdown-steps)、[R-10](research.md#r-10-every-startup-failure-shows-its-reason-in-a-standard-windows-dialog-and-is-logged-under-logs))。中断したスキャンは、VVMDM のどの起動方法でも、次の起動で再び始まる ([R-9](research.md#r-9-an-interrupted-last-scan-restarts-automatically-at-startup-for-every-way-of-starting))。 |
| LAN | 既定では VVMDM はループバックだけで待ち受ける。Settings の所有者だけの「Network」の節が LAN からの接続を許可し、その許可は settings テーブルに保持する ([R-14](research.md#r-14-lan-access-is-stored-in-the-settings-table-switching-it-reopens-the-listener-and-the-default-is-loopback-only)、[R-15](research.md#r-15-the-lan-access-switch-sits-in-an-owner-only-section-of-settings-and-cannot-be-changed-before-account-setup)、[contracts/network-settings-api.md](contracts/network-settings-api.md))。 |
| 配布 | GitHub Actions は、`v*` タグで `VVMDM-<version>-windows-amd64.zip` を GitHub Release に添付し、後の改訂により、`main` へのすべての push で `nightly` プレリリースに添付する ([R-13](research.md#r-13-github-actions-builds-the-distribution-and-attaches-it-to-a-github-release-on-a-v-tag)、[contracts/windows-app.md](contracts/windows-app.md))。 |

Issue に `ui` ラベルはないので、design ステージはない。画面の変更は Settings の 1 つの節だけで、その配置と内容は R-15 が決める。

## 技術的な文脈 {#technical-context}

**正本の定義**:

| 項目 | 出典 |
| --- | --- |
| 境界、依存の方向、組み立てを行う場所、depguard | [ARCHITECTURE.md](../../ARCHITECTURE.md#intended-dependency-direction)、[.golangci.yml](../../.golangci.yml) |
| 技術スタックと配布の形 (Docker) | [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) |
| OS ごとのハードウェアエンコーダー | [docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md) |
| 起動、環境変数、直接の起動、ネットワークへの公開 | [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md) |
| API | [api/openapi.yaml](../../api/openapi.yaml) |
| 検査の入口 | [Taskfile.yml](../../Taskfile.yml) (`task check`、`task generate`) |
| 依存の更新 | [docs/how-to/dependency-updates.md](../../docs/how-to/dependency-updates.md) |

**この機能に固有の文脈**:

- 新しい実行形態: GUI サブシステムの `windows/amd64` の exe (`-tags desktop -ldflags -H=windowsgui`)。`CGO_ENABLED=0` を保ったまま Linux からクロスビルドする。
- 新しい依存: `github.com/wailsapp/go-webview2` (`go.mod`。読むのは Windows のビルドだけ) と `github.com/tc-hib/go-winres` (`tools/go.mod` の `tool`)。FFmpeg を同梱する (Gyan.dev の essentials。バージョンと SHA-256 で固定)。
- 実行時の前提: WebView2 Runtime (Windows 10/11 には既定で入っている。ないときは R-10 のダイアログが出る)。
- 新しく保存する値: 既存の `settings` テーブルのキー `desktop.lan_access`。マイグレーションは加えない。データモデルのほかの部分は変わらない (`data-model.md` はない)。
- どの起動方法でも変わる振る舞い: 中断したスキャンの自動的な再開始 (R-9) と、`ffmpeg`/`ffprobe` への `CREATE_NO_WINDOW` (R-11、Windows だけ)。

## Constitution Check {#constitution-check}

| 関門 | 判定 |
| --- | --- |
| 依存は `cmd → internal/{adapters, app} → internal/domain` の一方向に流れ、兄弟のアダプターは互いを読まない (ARCHITECTURE.md、depguard) | 合格。Win32 と WebView2 を扱う新しいアダプター `internal/desktop` を読むのは `cmd/mdm` だけで、ほかの `internal/*` を読まない。depguard の兄弟の規則に加える。 |
| `cmd/mdm` は組み立てるだけで、ユースケースを持たない (ARCHITECTURE.md) | 合格。終了の確認が必要とする「取り込み中か」は `app.Scans.Busy` にあり、`cmd/mdm` はつなぐだけである。リスナーの開き直しは起動と停止のつなぎ込みで、組み立ての一部である。 |
| `internal/app` は `os/exec`、`net/http`、DB を読まない (ARCHITECTURE.md) | 合格。`Busy` は app が宣言する役割の型 (未完了のジョブがあるか) を通る。 |
| 生成ファイルは手で編集しない (AGENTS.md) | 合格。`/api/settings/network` は `api/openapi.yaml` を変更し、`task generate` を実行する。 |
| 制約は検査で強制する (core-beliefs.md) | 合格。`build-windows-check` に `-tags desktop` のビルドを、`task lint` に `GOOS=windows` と `desktop` タグでの lint を加えるので、CI は Windows だけのファイルも検査する。 |
| 文書は振る舞いと同じ変更で直す (core-beliefs.md、AGENTS.md) | 各文書の節には担当の単位がちょうど 1 つあり ([文書の担当](#documentation-ownership))、2 つの単位が同じ節を書くことはない。新しい設計文書 `docs/design-docs/windows-app.md` を索引に加える。 |
| Docker と直接の起動は動き続ける (親 Issue の要件 11) | 合格。タグなしのビルドの入口、環境変数、イメージは変わらない。どの起動方法にも影響する変更は R-9 と R-11 だけである。 |

design の後の再確認で違反は見つからなかった。

## プロジェクトの構成 {#project-structure}

### 文書 (この機能) {#documentation-this-feature}

```text
specs/037-windows-app/
├── plan.md
├── research.md                      # R-1..R-15
├── quickstart.md                    # Checks on real hardware (the parts CI cannot see)
└── contracts/
    ├── network-settings-api.md      # /api/settings/network
    └── windows-app.md               # Zip contents, startup arguments and failure messages, data location, window
```

`data-model.md` はない。加える保存値は既存の `settings` テーブルの 1 つのキーだけで、R-14 と契約に書いてある。

### ソースコード {#source-code}

**影響する境界**:

| 境界 | 変わるもの |
| --- | --- |
| `cmd/mdm` | `run()` を、設定、ログの出力先、待ち受けるアドレス、待ち受け開始後の通知、停止のシグナルを引数に取る起動と停止の手順に分ける。タグなしの `main` とデスクトップの `main` の両方がこれを呼ぶ。開き直せるリスナー (R-14) と、LAN の許可の読み書きをつなぐ。 |
| `internal/app` | `Scans.Busy` (R-7) と、起動時のスキャンの再開始 (R-9) |
| `internal/store` | `desktop.lan_access` の読み書きと、未完了のジョブがあるかの問い合わせ |
| `internal/media` | 子プロセスの作成を 1 つの関数に移し、その関数が Windows で `CREATE_NO_WINDOW` を加える |
| `internal/httpapi`、`api/openapi.yaml` | `/api/settings/network` |
| `web/src/settings` | 「Network」の節 |
| ビルドと配布 | `scripts/build`、`Taskfile.yml`、`tools/go.mod`、`.github/workflows/windows-app.yml` |

**新しいパス**:

| パス | 目的 |
| --- | --- |
| `internal/desktop/` | ウィンドウ、WebView2、ダイアログ、ミューテックス、ジョブオブジェクト、データの場所の解決。Windows の実装は `_windows.go` のファイルにある |
| `cmd/mdm/desktop_windows.go` とほかの `desktop` タグ付きのファイル | デスクトップの組み立てとつなぎ込み |
| `cmd/mdm/winres/` | アイコンとマニフェストの入力 |
| `.github/workflows/windows-app.yml` | ビルドとリリースのワークフロー |
| `docs/design-docs/windows-app.md` | 設計文書 |

**構成の決定**: Win32 と WebView2 の扱いは新しいアダプター `internal/desktop` に置き、`cmd/mdm` の `desktop` タグ付きのファイルは組み立てとつなぎ込みだけを持つ。採用しなかった案: すべてを `cmd/mdm` に置く。ウィンドウプロシージャとダイアログの実装が組み立てのパッケージに入り、depguard でほかのアダプターから切り離せなくなる。

### 文書の担当 {#documentation-ownership}

振る舞いを変える単位が、同じ PR でその節を書く (core-beliefs.md)。各節には担当の単位がちょうど 1 つあるので、2 つの単位が同じ節を書くことはない。「Windows デスクトップアプリ」の単位がすべての節の見出しを付けて `docs/design-docs/windows-app.md` を作り、後の単位は自分の節の本文だけを書く。

| 文書 | 節 | 担当の単位 |
| --- | --- | --- |
| ARCHITECTURE.md | 起動時の復旧 (Intended topology の「In place today」の段落にあるスキャンの復旧) | 中断したスキャンを起動時に自動で再開始する |
| ARCHITECTURE.md | Intended topology の配布の形と、Intended dependency direction の `internal/desktop` | Windows デスクトップアプリ `VVMDM.exe`: 専用のウィンドウで開き、閉じたらサーバーを止める |
| ARCHITECTURE.md | API の一覧の `/api/settings/network` | LAN アクセスの許可を保存し、`/api/settings/network` で切り替える |
| docs/design-docs/windows-app.md と索引 | 文書の作成、「プロセスとウィンドウ」、「データと ffmpeg」、「起動時の失敗」 | Windows デスクトップアプリ `VVMDM.exe`: 専用のウィンドウで開き、閉じたらサーバーを止める |
| docs/design-docs/windows-app.md | 「終了、2 回目の起動、サインアウト」 | 閉じる前に確認し、2 回目の起動で動いているウィンドウを前面に出し、サインアウトで正常に止まる |
| docs/design-docs/windows-app.md | 「LAN からの接続」 | LAN アクセスの許可を保存し、`/api/settings/network` で切り替える |
| docs/design-docs/windows-app.md | 「配布」 | Windows の zip をビルドし、ワークフローでタグの GitHub Release に添付する |
| docs/how-to/running-vv.md | データと復旧 | 中断したスキャンを起動時に自動で再開始する |
| docs/how-to/running-vv.md | 新しい「Windows アプリ」の節 (入手、起動、SmartScreen、データの場所、更新、`--port`、LAN の許可) | Windows の zip をビルドし、ワークフローでタグの GitHub Release に添付する |
| docs/design-docs/tech-stack-selection.md | 配布の形 | Windows の zip をビルドし、ワークフローでタグの GitHub Release に添付する |
| docs/how-to/dependency-updates.md | FFmpeg のバージョンを更新する方法 | Windows の zip をビルドし、ワークフローでタグの GitHub Release に添付する |

## 実装作業 {#implementation-work}

### 中断したスキャンを起動時に自動で再開始する {#restart-an-interrupted-scan-automatically-at-startup}

**範囲**: 起動時、最新のスキャンが理由 `interrupted` の `failed` なら、ワーカーの開始後にスキャンを 1 つ始める ([research.md R-9](research.md#r-9-an-interrupted-last-scan-restarts-automatically-at-startup-for-every-way-of-starting))。VVMDM のどの起動方法にも影響する。[文書の担当](#documentation-ownership)にあるこの単位の節。

**依存**: なし

**受け入れ**: `task check` が通る。app のテストで、最新のスキャンが `interrupted` の起動はスキャンを 1 つ始め、最新のスキャンが `done` のとき、`interrupted` 以外の理由の `failed` のとき、スキャンの記録がないときはスキャンを始めない。cmd/mdm のテストかローカルでの確認で、スキャン中に SIGTERM を送って再び起動すると、`GET /api/scans/current` が新しい `running` のスキャンを返し、それが終わると、停止の前に見つかっていなかったファイルも取り込まれる。

### Windows で `ffmpeg`/`ffprobe` をコンソールウィンドウなしで起動する {#start-ffmpegffprobe-without-a-console-window-on-windows}

**範囲**: `internal/media` で子プロセスを作る 6 か所を 1 つの関数に移し、その関数が Windows で `CREATE_NO_WINDOW` を加える ([R-11](research.md#r-11-ffmpeg-comes-from-the-ffmpeg-folder-next-to-the-exe-added-first-to-path-and-child-processes-run-without-a-console-window-inside-a-job-object))。

**依存**: なし

**受け入れ**: `task check` (`build-windows-check` を含む) が通る。`internal/media` で `exec.Command` か `exec.CommandContext` を直接呼ぶのはその関数だけである。media のテストは `GOOS=windows` でビルドでき (`go test -c` が成功する)、Windows のテストが `SysProcAttr.CreationFlags` に `CREATE_NO_WINDOW` が含まれることを確かめる。

### 設定とリスナーを渡せるように `cmd/mdm` の起動と停止を分ける {#split-cmdmdm-startup-and-shutdown-so-the-configuration-and-the-listener-are-passed-in}

**範囲**: `run()` を、`Config`、ログの出力先、リスナーを作る部品、待ち受け開始後の通知、停止のシグナルを引数に取る手順に分ける。タグなしの `main` は、今の環境変数、標準出力、`MDM_ADDR`、SIGINT/SIGTERM を渡す。リスナーの部品は別のアドレスで開き直せる ([R-14](research.md#r-14-lan-access-is-stored-in-the-settings-table-switching-it-reopens-the-listener-and-the-default-is-loopback-only) のリスナー)。`app.Scans.Busy` ([R-7](research.md#r-7-the-close-confirmation-appears-while-a-scan-runs-or-import-jobs-are-unfinished-and-closing-hides-the-window-before-the-shutdown)) と、それが必要とするストアの問い合わせも含む。振る舞いは変わらない。

**依存**: なし

**受け入れ**: `task check` が通る。既存の `serveUntil` のテストが通る。リスナーの部品のテストで、`127.0.0.1:<p>` から `0.0.0.0:<p>` に開き直した後は、そのマシン自身のループバックでないアドレスからの接続が成功し、元に戻した後は拒否され、確立済みの接続 (SSE) は開き直しの後も残る。開き直しに失敗したときは、リスナーは元のアドレスに留まる。`Busy` のテストで、スキャンが動いているか `queued` か `running` のジョブがある間は true で、どれもないときは false である。`task up` のコンテナは以前と同じく `/api/health` に応答する。

### Windows デスクトップアプリ `VVMDM.exe`: 専用のウィンドウで開き、閉じたらサーバーを止める {#windows-desktop-app-vvmdmexe-open-in-its-own-window-and-stop-the-server-when-it-closes}

**範囲**: `internal/desktop` (ウィンドウ、全画面とレンダラープロセスの失敗後の再読み込みを伴う WebView2 の埋め込み、ダイアログ、ジョブオブジェクト、データの場所の解決) と、`cmd/mdm` の `desktop` タグ付きの `main`。引数 (`--port`)、R-10 の起動前の確認とダイアログ、ログファイル、`PATH` の先頭への `ffmpeg\` の追加、`%LOCALAPPDATA%\VVMDM` の設定、転送ヘッダーを読まないサーバーの設定、待ち受け開始後のウィンドウの表示、閉じたときの停止の手順の実行 (確認なし) と終了 ([R-1](research.md#r-1-the-desktop-build-compiles-cmdmdm-as-a-windows-gui-under-the-desktop-build-tag-and-runs-the-server-in-the-same-process) から [R-5](research.md#r-5-data-lives-in-localappdatavvmdm)、[R-10](research.md#r-10-every-startup-failure-shows-its-reason-in-a-standard-windows-dialog-and-is-logged-under-logs)、R-11、[contracts/windows-app.md、起動の引数と失敗のメッセージ](contracts/windows-app.md#startup-arguments-and-failure-messages) から [ウィンドウ](contracts/windows-app.md#window) まで)。depguard に `internal/desktop` を加える。`build-windows-check` に `-tags desktop` のビルドを、`task lint` に `GOOS=windows` と `desktop` タグでの lint を加える。[文書の担当](#documentation-ownership)にあるこの単位の節 (`docs/design-docs/windows-app.md` とその索引の項目の作成を含む)。

**依存**: 「設定とリスナーを渡せるように `cmd/mdm` の起動と停止を分ける」、「Windows で `ffmpeg`/`ffprobe` をコンソールウィンドウなしで起動する」

**受け入れ**: `task check` が通り、`-tags desktop` の Windows のビルドが成功する。データの場所の解決と引数の解析のテストが通る。Windows の実機で、ビルドした exe の隣に `ffmpeg\` を置き、[quickstart.md](quickstart.md) の手順 1、2、4、5 と、Edge Cases の「Port」、「Running from inside the zip」、「Unwritable location」、「Network drive」、「Full screen」が書かれたとおりに振る舞う。この単位は画面に影響するので、ウィンドウの見た目と操作を確認する。

### 閉じる前に確認し、2 回目の起動で動いているウィンドウを前面に出し、サインアウトで正常に止まる {#confirm-before-closing-bring-the-running-window-to-the-front-on-a-second-launch-and-stop-gracefully-on-sign-out}

**範囲**: `WM_CLOSE` で `Busy` を読んで確認を出す ([R-7](research.md#r-7-the-close-confirmation-appears-while-a-scan-runs-or-import-jobs-are-unfinished-and-closing-hides-the-window-before-the-shutdown))。名前付きミューテックスと既存のウィンドウを前面に出すこと ([R-6](research.md#r-6-a-second-launch-is-detected-by-a-per-user-named-mutex-across-sessions-and-the-existing-window-comes-to-the-front))。`WM_QUERYENDSESSION`/`WM_ENDSESSION` ([R-8](research.md#r-8-sign-out-and-shutdown-show-no-confirmation-give-windows-a-reason-to-wait-and-run-the-same-shutdown-steps))。[文書の担当](#documentation-ownership)にあるこの単位の節。

**依存**: 「Windows デスクトップアプリ `VVMDM.exe`: 専用のウィンドウで開き、閉じたらサーバーを止める」、「中断したスキャンを起動時に自動で再開始する」

**受け入れ**: `task check` が通る。Windows の実機で、[quickstart.md](quickstart.md) の手順 6 と、Edge Cases の「Second launch」と「Sign-out」が書かれたとおりに振る舞う。確認ダイアログの見た目と操作を確認する。

### LAN アクセスの許可を保存し、`/api/settings/network` で切り替える {#store-the-lan-access-permission-and-switch-it-with-apisettingsnetwork}

**範囲**: `api/openapi.yaml` と `task generate`、`internal/httpapi` のルート (デスクトップアプリとして動いていないときは `404`)、ストアの `desktop.lan_access`、そして起動時に保存値から待ち受けるアドレスを選び、`PUT` では保存の前にリスナーを開き直す `cmd/mdm` のつなぎ込み ([contracts/network-settings-api.md](contracts/network-settings-api.md)、[R-14](research.md#r-14-lan-access-is-stored-in-the-settings-table-switching-it-reopens-the-listener-and-the-default-is-loopback-only))。[文書の担当](#documentation-ownership)にあるこの単位の節。

**依存**: 「設定とリスナーを渡せるように `cmd/mdm` の起動と停止を分ける」、「Windows デスクトップアプリ `VVMDM.exe`: 専用のウィンドウで開き、閉じたらサーバーを止める」

**受け入れ**: `task check` が通る。httpapi のテストで、契約の [`GET /api/settings/network`](contracts/network-settings-api.md#get-apisettingsnetwork) と [`PUT /api/settings/network`](contracts/network-settings-api.md#put-apisettingsnetwork) の表のすべての行が、その応答を返す。デスクトップアプリでないときは `404`、ゲストには `401`、別のサイトからは `403`、開き直しに失敗したときは保存値を変えずに `409` `listen_failed`、保存に失敗したときはリスナーを元のアドレスに戻して `500`。ストアのテストで、行がないと false と読め、保存した値を読み戻せる。cmd/mdm のテストで、保存値が true なら VVMDM は `0.0.0.0` で待ち受け、false か行がないときは `127.0.0.1` で待ち受ける。`addresses` はアクセスを許可している間だけ埋まり、ループバックを含まない。

### Settings の「Network」の節で LAN からの接続を許可し、開くアドレスを示す {#allow-lan-connections-and-show-the-addresses-to-open-in-the-network-section-of-settings}

**範囲**: `web/src/settings` に節を加え、`404` では隠す。スイッチ、アクセスを許可している間のアドレス、オンにするときに示す注意 ([R-15](research.md#r-15-the-lan-access-switch-sits-in-an-owner-only-section-of-settings-and-cannot-be-changed-before-account-setup))、`409` `listen_failed` のメッセージ。文字列は `web/src/i18n/en.ts` に置く。この単位は文書を書かない (running-vv.md の LAN の許可は zip の単位が書く)。

**依存**: 「LAN アクセスの許可を保存し、`/api/settings/network` で切り替える」

**受け入れ**: `task check` が通る。コンポーネントのテストで、`404` では節がない。`lanAccess` が false のときはアドレスが出ず、注意が出る。スイッチは `PUT` を送る。true のときは返された `addresses` を一覧する。`409` ではエラーが出て、スイッチが元に戻る。Windows の実機で、[quickstart.md](quickstart.md) の手順 7 が書かれたとおりに振る舞う。画面が変わるので、節の見た目と操作を確認する。

### Windows の zip をビルドし、ワークフローでタグの GitHub Release に添付する {#build-the-windows-zip-and-attach-it-to-a-github-release-on-a-tag-with-a-workflow}

**範囲**: `scripts/build` での Windows の zip の組み立て (バージョンと SHA-256 による FFmpeg の固定と取得、`go-winres` によるアイコンとマニフェスト、`README.txt`)、`task build-windows-app`、`tools/go.mod` の `go-winres`、`.github/workflows/windows-app.yml` (`v*` タグと手動実行、Windows のジョブでのエンコーダーの確認、Release への添付) ([R-12](research.md#r-12-the-bundled-ffmpeg-is-gyandevs-windows-essentials-build-pinned-by-version-and-sha-256)、[R-13](research.md#r-13-github-actions-builds-the-distribution-and-attaches-it-to-a-github-release-on-a-v-tag)、[contracts/windows-app.md、zip の中身](contracts/windows-app.md#zip-contents))。running-vv.md の Windows アプリの節 (入手、起動、SmartScreen、データの場所、更新、`--port`、LAN の許可) と、[文書の担当](#documentation-ownership)にあるこの単位のほかの節。

**依存**: 「Windows デスクトップアプリ `VVMDM.exe`: 専用のウィンドウで開き、閉じたらサーバーを止める」、「Settings の「Network」の節で LAN からの接続を許可し、開くアドレスを示す」(running-vv.md が LAN の許可を説明できるように)

**受け入れ**: `task check` が通る。`task build-windows-app` が `dist/VVMDM-<version>-windows-amd64.zip` を作り、その中身は契約の [zip の中身](contracts/windows-app.md#zip-contents) と一致し、SHA-256 が一致しない FFmpeg ではビルドが失敗する。ワークフローの手動実行が成功し、Windows のジョブが `h264_nvenc` と `h264_qsv` を確認し、zip がアーティファクトに残る。ffmpeg のない Windows の実機で、その zip で [quickstart.md](quickstart.md) の手順 1 から 3 と 8 が書かれたとおりに振る舞う (手順 8 は、この単位の zip と後の変更でビルドした zip で確認する)。
