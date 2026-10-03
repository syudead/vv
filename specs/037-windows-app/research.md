# Research: Windows アプリ化

親 Issue: #653。技術スタック・境界・依存方向は
[ARCHITECTURE.md](../../ARCHITECTURE.md) と
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) が正本で、
ここでは変えない（Go 1 バイナリ、`CGO_ENABLED=0`、`modernc.org/sqlite`、SPA の埋め込み、`ffmpeg`/`ffprobe` の
子プロセス）。ハードウェアエンコードの OS ごとの候補は
[docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md)、直接起動の手順と
「既定のアプリで開く」の条件は [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md) が正本である。
ここには、この feature が足す決定だけを書く。

## R-1: デスクトップ版は `cmd/mdm` を `desktop` ビルドタグで Windows GUI として組み、サーバーを同じプロセスで動かす

- **Decision**: `cmd/mdm` に `//go:build windows && desktop` のファイル群を足し、
  `go build -tags desktop -ldflags "-H=windowsgui"` で `VVMDM.exe` を作る。デスクトップ版の `main` は
  設定を環境変数でなく自分で組み立て（R-4・R-5・R-9）、今の `run()` から切り出した起動・停止の手順
  （[plan.md の構造](plan.md#source-code)）を同じプロセスで呼び、閉じるときは同じ停止手順を通す。
  タグなしのビルド（Docker・`task build`・`mdm account`）は今のまま。
- **Rationale**: `cmd/mdm` は唯一の組み立て場所で（ARCHITECTURE.md「Intended dependency direction」）、
  `internal/eventbus` を読めるのも `cmd/mdm` だけである。同じパッケージの別エントリにすれば組み立てを
  二重に持たず、depguard の規則も変えない。同じプロセスなら、閉じる操作をそのまま今の段階的な停止
  （HTTP の停止→ワーカーの取り消し→走査の終わりを待つ→DB を閉じる）へつなげられ、閉じる確認に要る
  「取り込み中か」を DB から直接読める（R-7）。
- **Alternatives considered**:
  - 新しい `cmd/vvmdm` と、サーバーの組み立てを `internal/server` へ移す。3,500 行の組み立てを動かし、
    「組み立ては `cmd/mdm` だけ」「`eventbus` は `cmd/mdm` だけが読む」の規則を書き換えることになる。却下。
  - ランチャーの exe が今の `mdm.exe` を子プロセスとして起動する。Windows では GUI の親からコンソールの子へ
    SIGTERM 相当を送る手段が無く（`CTRL_BREAK_EVENT` は共有コンソールが要る）、穏当な停止を保証できない。
    閉じる確認の「取り込み中か」も、所有者の認証を通して HTTP で聞くことになる。却下。

## R-2: ウィンドウは自前の Win32 ウィンドウに `github.com/wailsapp/go-webview2` の `pkg/edge` で WebView2 を埋め込む

- **Decision**: `github.com/wailsapp/go-webview2`（タグ付き版、`CGO_ENABLED=0` で組める）の `edge.Chromium`
  を、デスクトップ版が自分で作って自分のウィンドウ手続きを持つ Win32 ウィンドウに埋め込む。
  ウィンドウ手続きで `WM_CLOSE`（R-7）・`WM_QUERYENDSESSION`/`WM_ENDSESSION`（R-8）・2 つ目の起動からの
  前面化（R-6）を扱い、`ContainsFullScreenElementChanged` で動画の全画面をウィンドウの全画面
  （枠なし・画面いっぱい）に切り替え、`ProcessFailed` で描画プロセスが落ちたら再読み込みする。
  WebView2 ランタイムは Windows 10/11 に既に入っているものを使い、無いときは起動前に判定して入手先を
  示すダイアログを出す（R-10）。
- **Rationale**: 要件 6 の「閉じるか続けるかを選べる」には `WM_CLOSE` を自分で受ける必要がある。動画の
  アプリとして、`<video>` の全画面がウィンドウの枠内に留まるのは使えない。CGO を使わないので、今の
  `CGO_ENABLED=0` のクロスビルドと `build-windows-check` がそのまま使える。
- **Alternatives considered**:
  - `github.com/jchv/go-webview2` の高水準 API。`WM_CLOSE` で即座にウィンドウを壊す手続きが固定で確認を
    挟めず、全画面のイベントの扱いが無く、初期化の失敗で `log.Fatal`（GUI では黙って落ちる）し、タグ付きの
    版が無い。却下。
  - Wails（v2/v3）。資産を `wails://` などの独自のオリジンで配るので、同じオリジンの確認
    （`acceptsSameOrigin`）と「既定のアプリで開く」の `Host` の確認に合わず、専用の CLI とプロジェクト構成も
    要る。却下。
  - Edge を `--app=` で起動する。閉じる確認を挟めず、閉じたことも確実には分からない（要件 5・6）。却下。
  - Electron・Tauri。Node か Rust のツールチェーンと、数十〜百 MB の実行環境が増える。却下。

## R-3: ウィンドウは `http://localhost:<ポート>` を開く

- **Decision**: WebView2 は `http://localhost:<ポート>/` を開く。SPA・API・動画の配信は今の HTTP サーバーを
  そのまま通す。
- **Rationale**: 「既定のアプリで開く」（受け入れ条件 4）は、接続元がループバックで `Host` が `localhost` 等で
  あることを求める（`internal/httpapi/open.go` の `loopbackRequest`）。同じオリジンの確認も `http://` と
  `Host` の一致で通る。セッションの Cookie は HTTP では `vv_session` で `Secure` が付かず、そのまま使える。
- **Alternatives considered**: WebView2 の `WebResourceRequested` でリクエストを横取りしてハンドラへ直接渡す。
  HTTP サーバーは LAN 接続（要件 9）とブラウザからの確認（受け入れ条件 5）のためにどのみち要り、
  経路が 2 つになる。却下。

## R-4: ポートは固定の既定値 `47880` で、使えないときは理由を示して終わる。`--port` で変えられる

- **Decision**: 既定のポートは `47880`。`VVMDM.exe --port <番号>` で変えられる（ショートカットの引数で
  指定する）。待ち受けに失敗したら、ポート番号と「他のプログラムが使っている可能性」と `--port` での
  変え方をダイアログで示して終了する（[contracts/windows-app.md §2](contracts/windows-app.md#2-起動の引数と失敗時の表示)）。
- **Rationale**: Edge Case「使おうとしたポートが他のプログラムに使われている」は、ポートが決まっていて、
  使えなければ理由を示すことを前提にしている。オリジン（`http://localhost:<ポート>`）が起動ごとに
  変わらないので、WebView2 の `localStorage`（表示の好み）と、LAN の端末で開いたアドレス（要件 9）が
  次回も使える。`8080` は開発用のサーバーや Docker 版の既定と重なりやすいので避ける。
- **Alternatives considered**:
  - 起動ごとに空いているポートを選ぶ。オリジンが変わって `localStorage` が毎回消え、LAN の端末に
    覚えさせたアドレスも使えなくなる。却下。
  - 塞がっていたら次の番号を自動で試す。LAN の端末に示したアドレスが黙って変わる。却下。
  - 設定画面でポートを変える。変えた値を待ち受け前に読む必要があり、塞がっていて起動できないときは
    画面そのものに届かない。却下。

## R-5: データは `%LOCALAPPDATA%\VVMDM` に置く

- **Decision**: `MDM_DATA_DIR` に当たるデータの置き場を `%LOCALAPPDATA%\VVMDM\data`（`mdm.db` と
  `thumbnails\`）、WebView2 の利用者データを `%LOCALAPPDATA%\VVMDM\webview2`、ログを
  `%LOCALAPPDATA%\VVMDM\logs` にする（[contracts/windows-app.md §3](contracts/windows-app.md#3-データの置き場)）。
  環境変数 `MDM_*` は読まない。
- **Rationale**: 要件 4（利用者ごと・書き込み可能・指定不要）と要件 10（zip を置き換えてもデータが残る）を
  満たす。生成物（サムネイル・シーク用スプライト・プレビュー）は大きいので、移動プロファイルに乗る
  Roaming でなく Local に置く。WebView2 の既定の利用者データの場所は exe の隣で、書き込めない場所に
  展開すると落ちるので明示する（Edge Case）。
- **Alternatives considered**:
  - exe の隣（ポータブル）。zip の置き換え（要件 10）で消えやすく、`Program Files` などでは書けない。却下。
  - `%APPDATA%`（Roaming）。生成物がサインインのたびに同期される。却下。

## R-6: 二重起動は名前付きミューテックスで判定し、既存のウィンドウを前面に出す

- **Decision**: 起動の最初に `Local\VVMDM-<データの置き場のハッシュ>` の名前付きミューテックスを取る。
  取れなければ、既存のウィンドウ（固有のウィンドウクラス名で探す）を元のサイズに戻して前面に出し、
  何も起動せずに終わる。ミューテックスはプロセスの終わりまで持つ。前のプロセスが停止の途中で
  ウィンドウが既に無いときは、最大 30 秒ミューテックスを待ってから起動する。
- **Rationale**: 要件 7。ポートの衝突で判定すると、他のプログラムが使っている場合（R-4）と区別できない。
  同じ DB を 2 つのプロセスが開くことも防ぐ。停止の途中（R-7 の段階的な停止は最大で数十秒かかる）に
  もう一度起動したときに、何も出ずに終わらないようにする。
- **Alternatives considered**: ロックファイル。プロセスが強制終了されたときに残り、次の起動を誤って
  止める。却下。

## R-7: 閉じる確認は「走査中か、未完了の取り込みの仕事がある」ときに出し、閉じると決めたらウィンドウを先に消してから停止する

- **Decision**: `WM_CLOSE` を受けたら、`app.Scans` に足す `Busy(ctx)`（走っている走査がある、または
  `queued`/`running` の仕事が 1 件以上ある）を読む。偽なら確認なしで閉じる。真なら Windows 標準の
  確認ダイアログ（TaskDialog。「取り込みの途中です。閉じると中断され、次に起動したときに続きから再開します。」、
  ボタンは「閉じる」「続ける」）を出し、「続ける」なら何もしない。閉じるときはウィンドウを隠してから、
  今の停止手順を最後まで通し、DB を閉じて終了する。ライブ変換の配信（要求ごとの `ffmpeg`）は判定に
  含めない。
- **Rationale**: 要件 6 の「次回起動時に続きから再開される」が成り立つのは、走査（R-9）と取り込みの仕事
  （`RequeueRunningJobs`）である。ライブ変換は視聴中の要求に結び付いていて再開の対象でなく、動画を
  見ている途中に閉じるたびに確認が出ることになる。ウィンドウを先に消すので、閉じた操作への反応は
  すぐに見え、受け入れ条件 5（閉じたあと応答しない）はプロセスの終了で満たす。
- **Alternatives considered**:
  - HTML の確認（`beforeunload`）。ウィンドウを閉じる操作では WebView2 に届かない。却下。
  - SPA の中に独自の確認ダイアログを出す。ウィンドウ手続きから SPA へ問い合わせて答えを待つ往復が要り、
    SPA の読み込み前や描画プロセスの異常時に閉じられなくなる。却下。

## R-8: サインアウト・シャットダウンでは確認を出さず、停止を待つ理由を Windows に示して同じ停止手順を通す

- **Decision**: `WM_QUERYENDSESSION` には常に「終了してよい」と答え、`ShutdownBlockReasonCreate` で
  「VVMDM を停止しています」を登録する。`WM_ENDSESSION`（終了が確定）で今の停止手順を同期で通し、
  終わってから手続きを返す。確認は出さない。
- **Rationale**: Edge Case「サインアウト・シャットダウン」。SQLite は WAL で、途中で強制終了されても
  DB は壊れないが、穏当に止めれば走査の取り消しと仕事の状態が記録される。止めきれずに終了されても、
  次回の起動で R-9 と `RequeueRunningJobs` が続きを始める。
- **Alternatives considered**: `WM_QUERYENDSESSION` で終了を拒み、閉じる確認を出す。Windows は確認を
  待たずに強制終了の画面へ進むので、確認は意味を持たない。却下。

## R-9: 起動時に、最後の走査が中断（`interrupted`）で終わっていれば走査を自動で始め直す（全ての起動方法で）

- **Decision**: 起動時、`RecoverInterrupted` のあと、ワーカーを動かしてから、最新の走査が `failed` で理由が
  `interrupted` なら新しい走査を 1 回始める。`interrupted` は、停止の指示で打ち切った走査
  （`Scans` の寿命の取り消し）と、running のまま残って起動時に閉じた走査の両方に付く
  （`internal/app/scans.go`、`internal/store/scans.go`）。利用者が走査を取り消す操作は無いので、
  `interrupted` は必ずプロセスの停止による。デスクトップ版に限らず、Docker と直接起動でも同じにする。
- **Rationale**: 今は中断した走査は `failed` で閉じるだけで、仕事（`RequeueRunningJobs`）は続くが、
  ファイルを探す段は続かない。受け入れ条件 6「閉じるを選んで再起動するとスキャンが続きから進む」と
  Edge Case のシャットダウンにはこれが要る。走査はサイズと mtime が変わらないファイルを何もしないで
  通る（`internal/scanner/scanner.go`）ので、始め直しは「続きから」と同じ結果になる。コンテナの再起動で
  止まった走査も同じ理由で続ける方がよく、起動方法で振る舞いを分ける理由が無い。
- **Alternatives considered**:
  - デスクトップ版だけで始め直す。分岐を足すだけで、Docker の利用者には中断した走査を手で始め直す
    手間が残る。却下。
  - 走査の途中の位置（どのフォルダまで見たか）を記録して、そこから再開する。始め直しが変わっていない
    ファイルを読み飛ばすので、記録と再開の仕組みを足すほどの差が無い。却下。
  - `RecoverInterrupted` が閉じた走査があるときだけ始め直す。穏当に止めた走査は停止の時点で
    `interrupted` として閉じているので、閉じる確認で「閉じる」を選んだ場合（受け入れ条件 6）に続かない。却下。

## R-10: 起動の失敗は全て Windows 標準のダイアログで理由を示し、ログを `logs` に書く

- **Decision**: ウィンドウを出す前に、次を順に確かめ、失敗したら理由と対処を示すダイアログを出して
  終わる（文言は [contracts/windows-app.md §2](contracts/windows-app.md#2-起動の引数と失敗時の表示)）:
  zip の中から直接実行していない（exe が一時フォルダの下にない、`ffmpeg\ffmpeg.exe` と
  `ffmpeg\ffprobe.exe` が exe の隣にある）→ データの置き場に書ける → WebView2 ランタイムがある →
  DB を開いて移行できる → ポートで待ち受けられる。サーバーは待ち受けまで済ませてから
  ウィンドウを出すので、ウィンドウが空白のまま残ることはない。ログは JSON のまま
  `logs\vvmdm.log` に書き、起動のたびに前回分を `vvmdm.1.log` に移す。ダイアログはログの場所も示す。
- **Rationale**: Edge Case「ウィンドウは空白のままにならず、起動できない理由が分かる」「黙って落ちず、
  どうすればよいかが分かる」。GUI の exe には標準出力が無いので、ログをファイルに書かないと原因を
  追えない。エクスプローラーで zip の中の exe を実行すると、その exe だけが一時フォルダへ展開されるので、
  同梱の `ffmpeg` が隣に無いことで判定できる。
- **Alternatives considered**: WebView2 に失敗のページを出す。WebView2 ランタイムが無い・初期化に失敗した
  場合に出せない。却下。

## R-11: `ffmpeg` は exe の隣の `ffmpeg\` を `PATH` の先頭に足して使い、子プロセスはコンソール窓を出さずジョブオブジェクトに入れる

- **Decision**:
  - デスクトップ版は起動時に `<exe のフォルダ>\ffmpeg` を自分の `PATH` の先頭に足す。`internal/media` の
    コマンド名（`ffmpeg`/`ffprobe`）と `exec.LookPath` の確認はそのまま使う。
  - `internal/media` が起こす全ての `ffmpeg`/`ffprobe` に、Windows では `CREATE_NO_WINDOW` を付ける
    （`_windows.go` の 1 つの関数で `exec.Cmd` に設定し、他の OS では何もしない）。
  - デスクトップ版は起動の最初に自分を `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` のジョブオブジェクトへ入れ、
    子プロセスも同じジョブに入るようにする。
- **Rationale**: 要件 2・3。GUI サブシステムの親からコンソールの子を起こすと、子ごとにコンソール窓が
  一瞬出る（要件 3）。プロセスが強制終了されたときに、ライブ変換の `ffmpeg` が残って動き続けない
  （要件 5「常駐はしない」）。`PATH` の先頭に足せば、利用者が別の `ffmpeg` を入れていても同梱の版が
  使われ、`internal/media` の 6 か所のコマンド名を設定可能にしなくて済む。
- **Alternatives considered**:
  - `ffmpeg`/`ffprobe` のパスを設定として `internal/media` の各所へ渡す。呼び出し元はデスクトップ版
    1 つだけで、6 か所の定数と試験を変えることになる。却下。
  - `CREATE_NO_WINDOW` をデスクトップ版のビルドだけに付ける。直接起動をタスクスケジューラなどから
    ウィンドウなしで動かす場合にも同じ問題があり、付けて困る場面が無い。却下。

## R-12: 同梱の `ffmpeg` は Gyan.dev の Windows 版「essentials」を版と SHA-256 で固定する

- **Decision**: `GyanD/codexffmpeg` の GitHub Releases にある `ffmpeg-<版>-essentials_build.zip` を、
  版と SHA-256 をビルドスクリプトに固定して取得し、`ffmpeg.exe`・`ffprobe.exe` と、FFmpeg の LICENSE とソースの入手先だけを zip に入れる
  （[contracts/windows-app.md §1](contracts/windows-app.md#1-zip-の中身)）。ビルドは Windows のランナーで
  `ffmpeg -hide_banner -encoders` に `h264_nvenc` と `h264_qsv` があることを確かめ、無ければ失敗する。
  対応する Windows のハードウェアエンコーダは
  [hardware-encoding.md](../../docs/design-docs/hardware-encoding.md) の NVENC と QSV のままで、AMF は足さない。
- **Rationale**: 要件 2・3、受け入れ条件 3。Gyan.dev の Release は版ごとに URL が変わらず残るので、
  固定した版を後から再現できる。essentials は NVENC・QSV（oneVPL）を含み、full より小さい。
  受け入れ条件 3 の「対応 GPU」は、今の設計で Windows の候補にしている NVENC と QSV のことで、AMF を
  足すのはエンコーダの選択肢を増やす別の変更である。
- **Alternatives considered**:
  - BtbN/FFmpeg-Builds。版ごとの固定の Release が残らず（日付つきの自動ビルドは古いものが消える）、
    固定した版を後から取れない。却下。
  - 利用者に `ffmpeg` を入れてもらう。要件 2 に反する。却下。

## R-13: 配布物は GitHub Actions で作り、タグ `v*` で GitHub Release に添付する

- **Decision**: 新しい workflow `.github/workflows/windows-app.yml` を、`v*` のタグの push と手動実行で
  動かす。Linux のジョブで SPA を組み、`GOOS=windows GOARCH=amd64 CGO_ENABLED=0` と `-tags desktop
  -ldflags "-H=windowsgui -X main.version=<版>"` で `VVMDM.exe` を作り、R-12 の `ffmpeg` と合わせて zip に
  する。Windows のジョブで同梱の `ffmpeg` のエンコーダを確かめ（R-12）、タグのときは GitHub Release に
  zip を添付する。手動実行では workflow の成果物として残す。手元では `task build-windows-app` で同じ
  zip を作る。exe のアイコンと manifest（DPI 対応・Common Controls v6）は
  `github.com/tc-hib/go-winres` で `.syso` を生成して埋め込む。対象は `windows/amd64` だけ。
- **Rationale**: 要件 1（zip を配布する）。今の CI は `main` への push で Docker イメージを出すだけで、
  利用者が取れる配布物の置き場が無い。manifest が無いと、高 DPI の画面で WebView2 の表示がぼやける。
- **Alternatives considered**:
  - `main` への push ごとに zip を作る。利用者向けの版の区切りが無く、どれを取ればよいか分からない。却下。
  - `windows/arm64` も作る。固定できる `ffmpeg` の arm64 版が無く、Windows on ARM は x64 の exe を
    エミュレーションで動かせる。却下。

## R-14: LAN からの接続の許可は設定表に保存し、切り替えたら待ち受けを開き直す。既定はループバックだけ

- **Decision**:
  - 保存: 既存の `settings` 表の鍵 `desktop.lan_access`（値 `true`/`false`、行が無ければ偽）。移行は足さない。
  - 待ち受け: 偽なら `127.0.0.1:<ポート>`、真なら `0.0.0.0:<ポート>`。起動時は DB を開いたあと、待ち受けの
    前にこの値を読む。切り替えの要求では、新しいアドレスで待ち受けを開き直す（今の待ち受けを閉じて、
    同じ `http.Server` で新しい待ち受けを `Serve` する。確立済みの接続はそのまま続く）。開き直しに失敗
    したら元のアドレスへ戻し、保存値も変えずに `409` を返す。
  - 表示: 許可中は、上がっている非ループバックの IPv4 アドレスごとに `http://<アドレス>:<ポート>/` を返す。
  - 経路: 所有者だけの `GET`/`PUT /api/settings/network`。デスクトップ版でないときは `404`
    （[contracts/network-settings-api.md](contracts/network-settings-api.md)）。
  - 転送ヘッダ: デスクトップ版は `MDM_TRUSTED_PROXIES=none` 相当で動き、転送ヘッダを読まない。
- **Rationale**: 要件 8・9、受け入れ条件 7。ループバックだけで待ち受ければ、許可していない間は LAN から
  TCP の接続そのものができず、最初の起動で Windows ファイアウォールの許可ダイアログも出ない。
  ダイアログは利用者が許可をオンにしたときに出るので、出る理由が分かる。設定表は今の映像エンコードの
  選択と同じ置き場で、新しい保存の仕組みが要らない。デスクトップ版の前に逆プロキシは無いので、
  既定の「私設アドレスの転送ヘッダを信じる」は、LAN の端末に接続元を偽ってログインの試行の制限を
  逃れる道を開くだけになる。
- **Alternatives considered**:
  - 常に `0.0.0.0` で待ち受け、許可していない間は非ループバックの接続を受けてすぐ切る。最初の起動で
    ファイアウォールのダイアログが出て、許可していないのに「ネットワークで許可しますか」と聞かれる。却下。
  - 許可したときに `0.0.0.0` の待ち受けを `127.0.0.1` と並べて足す。Windows で同じポートのワイルドカードと
    特定アドレスの同時の待ち受けはソケットの設定に依存し、確実でない。却下。
  - 設定をデータの置き場のファイルに持つ。保存の仕組みが 2 つになる。DB は待ち受けの前に開いているので、
    表で足りる。却下。

## R-15: LAN の許可は設定画面の所有者だけの節に置き、初期設定の前は切り替えられない

- **Decision**: 設定画面に「Network」の節を足し、`GET /api/settings/network` が `404` のとき（デスクトップ版
  でない）は出さない。節には、許可のスイッチ、許可中に開くアドレス、オンにするときの注意（同じ LAN の
  端末から開けるようになること、Windows のファイアウォールが許可を求めたら「プライベート ネットワーク」で
  許可すること、HTTP なのでパスワードが暗号化されずに流れること）を出す。設定画面と API は所有者だけなので、
  アカウントを作る前（`setupRequired`）には切り替えられない。
- **Rationale**: 要件 9（アプリの中から許可でき、許可中はアドレスが分かる）。Edge Case「初期設定の前に
  LAN 接続を許可しようとする」は、許可を所有者だけにすることで起こりえない形にする。LAN の別端末が
  先にアカウントを作る危険は、許可した時点で必ずアカウントがあることで無くなる。
- **Alternatives considered**:
  - ウィンドウのメニュー（ネイティブ）に置く。初期設定の前にも押せて警告が要り、SPA の外に設定の
    画面がもう 1 つできる。却下。
  - 初期設定の画面に許可のスイッチを置き、警告を出す。アカウントを作る前に LAN へ開く理由が無い。却下。
