# Windows デスクトップ版（VVMDM.exe）

- ステータス: 採用
- スコープ: Windows で zip を展開して `VVMDM.exe` を実行するだけで使える形（専用ウィンドウ・同じプロセスの
  サーバー・データの置き場・同梱の `ffmpeg`・起動の失敗の示し方・閉じ方・LAN からの接続・配布）
- 経緯: [specs/037-windows-app/](../../specs/037-windows-app/plan.md)（親 Issue #653）

Docker と直接起動の形（`MDM_*`、`mdm account`、イメージ）は
[docs/how-to/running-vv.md](../how-to/running-vv.md) が正本で、この文書は扱わない。
zip の中身・起動の引数とダイアログの文の意味・データの置き場の一覧は
[contracts/windows-app.md](../../specs/037-windows-app/contracts/windows-app.md) にある。

## プロセスとウィンドウ

### Context

`cmd/mdm` は唯一の組み立ての場所で、HTTP サーバー・走査・取り込みのワーカーを 1 つのプロセスで動かし、
停止の指示で段階的に止める（HTTP の停止 → ワーカーの取り消し → 走査の終わりを待つ → DB を閉じる）。
Windows の利用者には、コンソール窓を出さず、ブラウザでなく専用のウィンドウで使えて、ウィンドウを
閉じればサーバーも止まる形が要る。

### Decision

- `cmd/mdm` を `desktop` ビルドタグで Windows GUI の exe として組む
  （`GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop -ldflags "-H=windowsgui" ./cmd/mdm`）。
  入口は `cmd/mdm/desktop_windows.go`（`//go:build windows && desktop`）で、タグなしの入口
  （`cmd/mdm/main_server.go`、環境変数と `mdm account`）は組み込まれない。どちらの入口も同じ起動・停止の
  手順 `run` を呼ぶので、組み立ては 1 つだけである。
- デスクトップ版の入口は組み立てと配線だけを持つ。Win32 のウィンドウ、WebView2 の埋め込み、ダイアログ、
  ジョブオブジェクト、データの置き場の解決は adapter `internal/desktop` が持つ。`internal/desktop` を読むのは
  `cmd/mdm` だけで、`internal/desktop` は他の `internal/*` を読まない（depguard の兄弟の規則）。
- 起動の順序: ジョブオブジェクトに入る → 引数を読む → データの置き場とログを用意する → 起動前の確認
  （[起動の失敗](#起動の失敗)）→ `run` を別の goroutine で始め、待ち受けを開いたらウィンドウを出す。
  ウィンドウは待ち受けの後にしか出ないので、空白のまま残らない。
- 待ち受けはループバックの `127.0.0.1:<ポート>`。ポートは既定 `47880`、`--port <1-65535>` で変えられ、
  それ以外の引数は「使えない引数」のダイアログで終わる。デスクトップ版の前に逆プロキシは無いので、
  転送ヘッダを読まない（`MDM_TRUSTED_PROXIES=none` 相当）。
- ウィンドウは自前の Win32 ウィンドウ（題名 `VVMDM`、クラス名 `VVMDMWindow`）に
  `github.com/wailsapp/go-webview2` の `pkg/edge`（`edge.Chromium`）を埋め込み、`http://localhost:<ポート>/` を
  開く。SPA・API・動画の配信は HTTP サーバーをそのまま通るので、「既定のアプリで開く」のループバックと
  `Host` の確認、同じオリジンの確認、`vv_session` の Cookie は直接起動と同じに働く。
  - 初期の大きさは 1280×800（画面の DPI に合わせて広げる）で、主画面の作業領域より大きければ収めて
    中央に置く。
  - 動画の全画面（`ContainsFullScreenElementChanged`）では、ウィンドウを枠なし（`WS_POPUP`）でそのモニターの
    全体に広げ、抜けたら元のスタイル・位置・大きさ（`WINDOWPLACEMENT`）に戻す。
  - 描画プロセスが落ちたか応答しなくなったら（`ProcessFailed`）、今のページを開き直す。ブラウザの
    プロセスが終わると WebView2 は使えないので、続けられない誤りとしてダイアログを出し、停止手順を
    通して終わる。
- ウィンドウを閉じると（`WM_CLOSE`）、先にウィンドウを隠してから停止の指示を出し、`run` が停止手順を
  終えたらウィンドウを壊してプロセスを終える。待ち受けを失うなど、`run` が誤りで戻ったときも同じ道で
  ウィンドウを壊し、理由をダイアログで示す。

### Alternatives considered

- 新しい `cmd/vvmdm` と、組み立てを `internal/server` へ移す。組み立ての場所が 2 つになるか、3,500 行の
  組み立てを動かすことになる。却下（research.md R-1）。
- ランチャーの exe が今の `mdm.exe` を子プロセスとして起こす。GUI の親からコンソールの子へ穏当な停止を
  送る手段が無い。却下（R-1）。
- `github.com/jchv/go-webview2` の高水準 API、Wails、Edge の `--app=`、Electron・Tauri。閉じる操作を自分で
  受けられないか、資産のオリジンが変わるか、実行環境が増える。却下（R-2）。

## データと ffmpeg

### Context

利用者は環境変数を設定せず、`ffmpeg` も入れない。展開した zip は新しい版で置き換えられ、
`Program Files` や読み取り専用の共有フォルダに置かれることもある。

### Decision

- データは利用者ごとの `%LOCALAPPDATA%\VVMDM` に置く（既知のフォルダ `FOLDERID_LocalAppData` として読む）。
  `data\`（`MDM_DATA_DIR` と同じ中身: `mdm.db` と `thumbnails\`）、`webview2\`（WebView2 の利用者データ）、
  `logs\` を作る。環境変数 `MDM_*` は読まない。exe の隣には何も書かないので、zip の置き換えでデータは
  消えず、書けない場所に展開しても動く。
- ログは今の JSON のまま `logs\vvmdm.log` に書き、起動のたびに前回分を `vvmdm.1.log` へ移す（前々回は消える）。
- 起動時に `<exe のフォルダ>\ffmpeg` を `PATH` の先頭に足す。`internal/media` のコマンド名と起動前の
  `exec.LookPath` の確認はそのままで、利用者が別の `ffmpeg` を入れていても同梱の版が使われる。
- `internal/media` が起こす `ffmpeg`/`ffprobe` には、Windows では `CREATE_NO_WINDOW` が付く（全ての起動方法）。
- 起動の最初に自分を `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` のジョブオブジェクトへ入れる。子プロセスも同じ
  ジョブに入るので、プロセスが強制終了されてもライブ変換の `ffmpeg` が残らない。ジョブに入れなくても
  起動は続け、ログに残す。

### Alternatives considered

- exe の隣（ポータブル）や `%APPDATA%`（Roaming）にデータを置く。zip の置き換えで消えやすいか、大きな
  生成物がサインインのたびに同期される。却下（R-5）。
- `ffmpeg`/`ffprobe` のパスを設定として `internal/media` の各所へ渡す。呼び出し元はデスクトップ版だけで、
  変更が広がる。却下（R-11）。

## 起動の失敗

### Context

GUI の exe には標準出力も標準エラーも無い。起動に失敗して黙って終わると、利用者には何も起きないように
見える。

### Decision

ウィンドウを出す前に次を順に確かめ、最初に失敗したものだけを Windows 標準のダイアログ（`MessageBox`、
題名 `VVMDM`、OK だけ、文は英語）で示して終わる。どの文にもログの場所を添える（ログを書けない 2 番目を除く）。
文は `internal/desktop` の `Message*` が組み立てる。

1. `VVMDM.exe` が一時フォルダ（長い形のパス）の下になく、隣に `ffmpeg\ffmpeg.exe` と `ffmpeg\ffprobe.exe`
   がある。エクスプローラーで zip の中の exe を開くと exe だけが一時フォルダへ展開されるので、これで
   判定できる。失敗したら、zip を「すべて展開」して展開先の `VVMDM.exe` を実行するよう示す。
2. `%LOCALAPPDATA%\VVMDM` の下のフォルダを作って書ける。失敗したら、書けなかったフォルダを示す。
3. WebView2 ランタイムがある。無ければ、要ることと入手先の URL を示す。
4. DB を開いて移行できる。
5. ポートで待ち受けられる。失敗したら、ポート番号、他のプログラムが使っている可能性、`--port` での
   変え方を示す。

4 と 5 は `run` の中で起こるので、`run` は失敗の段階（`startupStage`: DB・待ち受け・その他）を誤りに添えて
返し、入口はそれで文を選ぶ。誤りの文はそのままなので、タグなしの入口の出力は変わらない。
どれにも当たらない失敗（`ffmpeg` が `PATH` から見つからないなど）と、ウィンドウを出した後の失敗
（WebView2 の続けられない誤り、待ち受けを失った）は、要約とログの場所を示す。

### Alternatives considered

- WebView2 に失敗のページを出す。WebView2 ランタイムが無いか初期化に失敗したときに出せない。却下（R-10）。

## 閉じる・二重起動・サインアウト

### Context

閉じると取り込みが中断されることを、利用者は知らずに閉じうる（親 Issue 要件 6）。同じ利用者が 2 つ目を
起動すると、同じ `%LOCALAPPDATA%\VVMDM` の DB を 2 つのプロセスが開くことになる（要件 7）。サインアウトや
シャットダウンでは、確認を待たずに Windows がプロセスを終わらせる。

### Decision

- **閉じる前の確認**: `WM_CLOSE` で `app.Scans.Busy`（走っている走査がある、または `queued`/`running` の仕事が
  1 件以上ある）を読む。`run` は走査を用意したあと、この問いを `runOptions.OnBusyProbe` で入口へ渡す。偽なら
  確認なしで閉じる。真か読めなければ、Windows 標準の TaskDialog（題名 `VVMDM`、「Import is in progress」、
  閉じると中断され次の起動で続きから再開すること、ボタンは `Close` と `Keep running`、既定は `Keep running`）
  を出す。`Keep running`・Esc・ダイアログを閉じる操作では何もしない。`Close` なら、先にウィンドウを隠して
  から停止の指示を出し、停止手順を最後まで通して DB を閉じ、終了する。ライブ変換の配信は再開の対象で
  ないので判定に含めない。TaskDialog は Common Controls v6（exe の manifest で選ぶ）にしか無いので、
  manifest の無いビルドでは同じ文の「はい・いいえ」の `MessageBox`（既定は「いいえ」）で問う。
- **二重起動**: 起動の最初、ログと DB を開く前に、`Global\VVMDM-<利用者の SID>-<データの置き場のハッシュ>` の
  名前付きミューテックスを、作った利用者だけが開ける DACL（`D:P(A;;GA;;;<SID>)`）で取り、プロセスの
  終わりまで持つ。取れたら、同じ名前の `Local\`（セッションごと）のミューテックスも持つ。
  - `Global\` を取れず、`Local\` がある（持ち主が同じセッションにいる）なら、`VVMDMWindow` のウィンドウが
    表に出ていれば最小化を戻して前面に出し、ダイアログなしで終わる。表に出ていなければ（前のプロセスが
    ウィンドウを隠して停止の途中か、まだウィンドウを出す前）、表に出るかミューテックスが放されるのを
    最大 30 秒待つ。放されたら起動を続け、30 秒で決まらなければ起動中であることを示して終わる。
  - `Local\` が無い（同じ利用者が別のセッションで起動中）なら、待たずにそのことを示すダイアログを出して
    終わる。
- **サインアウト・シャットダウン**: `WM_QUERYENDSESSION` には常に「終了してよい」と答え、
  `ShutdownBlockReasonCreate` で「VVMDM is stopping」を登録する。`WM_ENDSESSION`（終了が確定）で、確認を
  出さずに同じ停止手順を同期で通し（上限 30 秒）、終わってから手続きを返す。終了が取り消されたら理由を
  消す。止めきれずに終了されても、次の起動で走査の始め直しと `RequeueRunningJobs` が続きを始める。

### Alternatives considered

- HTML の `beforeunload` や SPA の中の確認。ウィンドウを閉じる操作は WebView2 に届かないか、SPA の読み込み
  前や描画プロセスの異常時に閉じられなくなる。却下（R-7）。
- `Local\` だけのミューテックス、ロックファイル、ポートの衝突で判定する。セッションをまたいで同じ DB を
  開けるか、強制終了で残るか、他のプログラムとの衝突と区別できない。却下（R-6）。
- `WM_QUERYENDSESSION` で終了を拒んで確認を出す。Windows は確認を待たずに先へ進む。却下（R-8）。

## LAN からの接続

## 配布
