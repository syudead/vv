---
source: specs/037-windows-app/research.md
sourceHash: dda2c7678fbda38e0d5342eb0ea5949128933a263e4066ec78b2fd008fb3cbed
---

# 調査: Windows アプリ {#research-windows-app}

親 Issue: #653。引き継ぐ決定: 技術スタック、境界、依存の方向は [ARCHITECTURE.md](../../ARCHITECTURE.md) と [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md) に従い、この機能はそれらを変えない (1 つの Go バイナリ、`CGO_ENABLED=0`、`modernc.org/sqlite`、埋め込んだ SPA、子プロセスとしての `ffmpeg`/`ffprobe`)。OS ごとのハードウェアエンコーダーの候補は [docs/design-docs/hardware-encoding.md](../../docs/design-docs/hardware-encoding.md) に従い、バイナリを直接動かす手順と「Open in default app」の条件は [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md) に従う。このファイルは、この機能が加える決定だけを記録する。

## R-1: デスクトップのビルドは `desktop` ビルドタグで `cmd/mdm` を Windows の GUI としてコンパイルし、サーバーを同じプロセスで動かす {#r-1-the-desktop-build-compiles-cmdmdm-as-a-windows-gui-under-the-desktop-build-tag-and-runs-the-server-in-the-same-process}

**決定**: `cmd/mdm` に `//go:build windows && desktop` のファイル群を加え、`go build -tags desktop -ldflags "-H=windowsgui"` が `VVMDM.exe` を作る。デスクトップの `main` は、設定を環境変数からではなく自分で組み立て (R-4、R-5、R-9)、今の `run()` から分けた起動と停止の手順 ([plan.md の構成](plan.md#source-code)) を同じプロセスで呼び、ウィンドウが閉じたときに同じ停止の手順を実行する。タグなしのビルド (Docker、`task build`、`mdm account`) はそのままである。

**理由**: `cmd/mdm` は唯一の組み立ての起点であり (ARCHITECTURE.md "Intended dependency direction")、`internal/eventbus` を読んでよいのは `cmd/mdm` だけである。同じパッケージに 2 つ目の入口を置けば、組み立ては 1 つのままで、depguard の規則も変わらない。同じプロセスなら、閉じる操作が今の段階的な停止 (HTTP を止める → ワーカーを取り消す → スキャンの終了を待つ → DB を閉じる) に直接つながり、終了の確認は「取り込み中か」を DB から直接読む (R-7)。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| 新しい `cmd/vvmdm` を作り、サーバーの組み立てを `internal/server` に移す | 不採用: 3,500 行の組み立てを移し、「組み立ては `cmd/mdm` でだけ行う」と「`eventbus` を読むのは `cmd/mdm` だけ」の規則を書き換えることになる。 |
| 今の `mdm.exe` を子プロセスとして起動するランチャーの exe | 不採用: Windows では GUI の親がコンソールの子に SIGTERM に相当するものを送る方法がない (`CTRL_BREAK_EVENT` はコンソールの共有を要する) ので、正常な停止を保証できない。終了の確認も、「取り込み中か」を所有者の認証を通して HTTP で問い合わせなければならない。 |

## R-2: ウィンドウは VVMDM 自身の Win32 ウィンドウに、`github.com/wailsapp/go-webview2` の `pkg/edge` で WebView2 を埋め込む {#r-2-the-window-embeds-webview2-with-pkgedge-from-githubcomwailsappgo-webview2-in-vvmdms-own-win32-window}

**決定**: デスクトップのビルドは、独自のウィンドウプロシージャを持つ独自の Win32 ウィンドウを作り、そこに `github.com/wailsapp/go-webview2` (`CGO_ENABLED=0` でビルドできるタグ付きのリリース) の `edge.Chromium` を埋め込む。ウィンドウプロシージャは `WM_CLOSE` (R-7)、`WM_QUERYENDSESSION`/`WM_ENDSESSION` (R-8)、2 回目の起動のためにウィンドウを前面に出すこと (R-6) を扱う。`ContainsFullScreenElementChanged` は動画の全画面をウィンドウの全画面 (枠なしで画面を埋める) に切り替え、`ProcessFailed` はレンダラープロセスがクラッシュしたときにページを再読み込みする。アプリは Windows 10/11 にすでに入っている WebView2 Runtime を使う。それがないときは、起動前の確認が入手先を示すダイアログを出す (R-10)。

**理由**: 要件 6 の「閉じるか動かし続けるかを選ぶ」には、アプリが自分で `WM_CLOSE` を受け取る必要がある。動画のアプリにとって、ウィンドウの枠に閉じ込められた `<video>` の全画面は使い物にならない。CGO がないので、今の `CGO_ENABLED=0` のクロスビルドと `build-windows-check` が変更なしで動く。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| `github.com/jchv/go-webview2` の高水準 API | 不採用: その固定のウィンドウプロシージャは `WM_CLOSE` でウィンドウを破棄し確認の余地がない。全画面のイベントを扱わない。初期化の失敗で `log.Fatal` を呼ぶ (GUI アプリは黙って終了する)。タグ付きのリリースがない。 |
| Wails (v2/v3) | 不採用: `wails://` のような独自のオリジンからアセットを配信するので、「Open in default app」の同一オリジンの確認 (`acceptsSameOrigin`) と `Host` の確認に通らない。独自の CLI とプロジェクトの構成も要る。 |
| `--app=` で Edge を起動する | 不採用: 終了の確認の余地がなく、閉じたことを確実には検出できない (要件 5 と 6)。 |
| Electron か Tauri | 不採用: Node か Rust のツールチェーンと、数十から百 MB のランタイムが加わる。 |

## R-3: ウィンドウは待ち受けているポートの `http://localhost` を開く {#r-3-the-window-opens-httplocalhost-on-the-listening-port}

**決定**: WebView2 は `http://localhost:<port>/` を開く。SPA、API、動画の配信は、今の HTTP サーバーを変更なしで通る。

**理由**: 「Open in default app」(受け入れ条件 4) は、ループバックのクライアントと `localhost` のような `Host` を要する (`internal/httpapi/open.go` の `loopbackRequest`)。同一オリジンの確認も、`http://` と一致する `Host` で通る。HTTP ではセッションの cookie は `Secure` なしの `vv_session` で、そのままで動く。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| WebView2 の `WebResourceRequested` で要求を横取りし、ハンドラに直接渡す | 不採用: LAN からの接続 (要件 9) とブラウザからの確認 (受け入れ条件 5) のために HTTP サーバーはどのみち要るので、経路が 2 つになる。 |

## R-4: 既定のポートは固定の `47880` で、`--port` で変える。使えないポートは理由を示して起動を終える {#r-4-a-fixed-default-port-47880-changed-with---port-an-unavailable-port-ends-startup-with-the-reason}

**決定**: 既定のポートは `47880` である。`VVMDM.exe --port <number>` がそれを変える (ショートカットに引数として与える)。待ち受けに失敗したときは、ダイアログがポート番号、別のプログラムがそれを使っているかもしれないこと、`--port` で変える方法を示し、アプリは終了する ([contracts/windows-app.md、起動の引数と失敗のメッセージ](contracts/windows-app.md#startup-arguments-and-failure-messages))。

**理由**: エッジケース「VVMDM が使おうとするポートを別のプログラムが使っている」は、固定のポートと、使えないときに示す理由を前提にしている。オリジン (`http://localhost:<port>`) が起動の間で変わらないので、WebView2 の `localStorage` (表示の設定) と、LAN の端末で開くアドレス (要件 9) が次回も使える。`8080` は開発サーバーや Docker の既定とよくぶつかるので避ける。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| 起動ごとに空いているポートを選ぶ | 不採用: オリジンが変わるので、毎回 `localStorage` が失われ、LAN の端末で覚えたアドレスが使えなくなる。 |
| ポートが使われているときに次の番号を自動で試す | 不採用: LAN の端末に伝えたアドレスが黙って変わる。 |
| Settings の画面でポートを変える | 不採用: 値は待ち受けの前に読まなければならず、ポートが使われていて起動に失敗したときは画面そのものに届かない。 |

## R-5: データは `%LOCALAPPDATA%\VVMDM` に置く {#r-5-data-lives-in-localappdatavvmdm}

**決定**: `MDM_DATA_DIR` に当たるデータの場所は `%LOCALAPPDATA%\VVMDM\data` (`mdm.db` と `thumbnails\`)、WebView2 のユーザーデータは `%LOCALAPPDATA%\VVMDM\webview2`、ログは `%LOCALAPPDATA%\VVMDM\logs` である ([contracts/windows-app.md、データの場所](contracts/windows-app.md#data-locations))。`MDM_*` の環境変数は読まない。

**理由**: 要件 4 (ユーザーごと、書き込める、指定するものがない) と要件 10 (zip を置き換えてもデータが残る) を満たす。生成したメディア (サムネイル、シーク用サムネイルのスプライト、プレビュー) は大きいので、移動プロファイルとともに移る Roaming ではなく Local に置く。WebView2 の既定のユーザーデータの場所は exe の隣で、書き込めない場所に展開するとアプリがクラッシュするので、場所を明示的に設定する (エッジケース)。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| exe の隣 (ポータブル) | 不採用: zip を置き換えるときに失いやすく (要件 10)、`Program Files` のような場所では書き込めない。 |
| `%APPDATA%` (Roaming) | 不採用: 生成したメディアがサインインのたびに同期される。 |

## R-6: 2 回目の起動はセッションをまたぐユーザーごとの名前付きミューテックスで検出し、既存のウィンドウが前面に出る {#r-6-a-second-launch-is-detected-by-a-per-user-named-mutex-across-sessions-and-the-existing-window-comes-to-the-front}

**決定**: 起動時の最初、DB を開く前に、アプリは作成したユーザーだけが開ける DACL 付きで、名前付きミューテックス `Global\VVMDM-<hash of the user's SID and the data location>` を取る。ミューテックスを取れず、既存のウィンドウが同じセッションにある (固有のウィンドウクラス名で見つける) ときは、そのウィンドウを通常の大きさに戻して前面に出し、終了する。同じセッションにウィンドウがない (同じユーザーが別のセッションで VVMDM を動かしている) ときは、VVMDM がすでに別のサインインで動いていると伝えるダイアログを出して終了する。プロセスは終わるまでミューテックスを持つ。前のプロセスがまだ停止中で、そのウィンドウがすでにないときは、アプリは起動する前に最大 30 秒ミューテックスを待つ。

**理由**: 要件 7。同じユーザーはどのセッションでも `%LOCALAPPDATA%\VVMDM` の同じ DB を使うので、`Local\` (セッションごと) では、別のセッション (たとえばリモートデスクトップ) で別の `--port` を使うと 2 つのプロセスが同じ DB を開けてしまう。`Global\` の名前に SID を入れ、DACL を作成したユーザーに限ると、ほかのユーザーは自分のものを自由に起動できる。ポートの衝突で検出すると、別のプログラムがポートを使っている場合と区別できない (R-4)。前のプロセスの停止中 (R-7 の段階的な停止は最大で数十秒かかる) に再び起動したときに、何も表示されずに終わってはならない。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| `Local\` のミューテックス | 不採用: 上のとおり、別のセッションの 2 つのプロセスが同じ DB を開ける。 |
| ロックファイル | 不採用: プロセスが強制終了されると残り、次の起動を誤って妨げる。 |

## R-7: 終了の確認はスキャンが動いているか取り込みのジョブが未完了の間に出て、閉じるとまずウィンドウを隠してから停止する {#r-7-the-close-confirmation-appears-while-a-scan-runs-or-import-jobs-are-unfinished-and-closing-hides-the-window-before-the-shutdown}

**決定**: `WM_CLOSE` で、アプリは `app.Scans` に加えた `Busy(ctx)` を読む (スキャンが動いているか、`queued` か `running` のジョブが 1 つ以上ある)。false なら、ウィンドウは確認なしで閉じる。true なら、Windows の標準の確認ダイアログ (TaskDialog: "Import is in progress. If you close VVMDM now, the import is interrupted. It continues from where it left off the next time you start VVMDM."、ボタンは "Close" と "Keep running") が出て、"Keep running" は何もしない。閉じるときは、アプリはウィンドウを隠し、今の停止の手順を最後まで実行し、DB を閉じて終了する。ライブ変換 (要求ごとの `ffmpeg`) は数えない。

**理由**: 要件 6 の「次の起動で止まったところから続く」は、スキャン (R-9) と取り込みのジョブ (`RequeueRunningJobs`) に当てはまる。ライブ変換は視聴者からの要求に結び付いていて再開されず、それを数えると再生中に閉じるたびに確認を求めることになる。先にウィンドウを隠すと閉じる操作への反応がすぐに見え、受け入れ条件 5 (閉じた後は応答しない) はプロセスの終了で満たされる。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| HTML の確認 (`beforeunload`) | 不採用: ウィンドウを閉じることは WebView2 に届かない。 |
| SPA の中の独自の確認ダイアログ | 不採用: ウィンドウプロシージャから SPA へ行って戻る往復が要り、SPA の読み込み前やレンダラープロセスが壊れているときにはウィンドウを閉じられない。 |

## R-8: サインアウトとシャットダウンでは確認を出さず、Windows に待つ理由を伝え、同じ停止の手順を実行する {#r-8-sign-out-and-shutdown-show-no-confirmation-give-windows-a-reason-to-wait-and-run-the-same-shutdown-steps}

**決定**: アプリは `WM_QUERYENDSESSION` に常に「終了してよい」と答え、`ShutdownBlockReasonCreate` で "VVMDM is stopping" を登録する。`WM_ENDSESSION` (終了が確定した) では、今の停止の手順を同期的に実行し、それが終わるとプロシージャから戻る。確認は出ない。

**理由**: エッジケース「サインアウトかシャットダウン」。SQLite は WAL モードで動くので強制終了で DB は壊れないが、正常に止まればスキャンの取り消しとジョブの状態が記録される。停止が終わる前に Windows がプロセスを終わらせたときは、R-9 と `RequeueRunningJobs` が次の起動で作業を続ける。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| `WM_QUERYENDSESSION` で終了を拒み、終了の確認を出す | 不採用: Windows は確認を待たずに強制終了の画面に進むので、確認は意味をなさない。 |

## R-9: 中断した最後のスキャンは、どの起動方法でも、起動時に自動で再開始する {#r-9-an-interrupted-last-scan-restarts-automatically-at-startup-for-every-way-of-starting}

**決定**: 起動時、`RecoverInterrupted` の後でワーカーの開始後に、最新のスキャンが理由 `interrupted` の `failed` なら、アプリは新しいスキャンを 1 つ始める。`interrupted` は、停止の要求 (`Scans` の生存期間の取り消し) で打ち切られたスキャンと、`running` のまま残り起動時に閉じられたスキャンの両方に付く (`internal/app/scans.go`、`internal/store/scans.go`)。ユーザーにはスキャンを取り消す操作がないので、`interrupted` は常にプロセスの停止から来る。振る舞いはデスクトップのビルドだけでなく、Docker でもバイナリを直接動かすときでも同じである。

**理由**: 今は中断したスキャンは `failed` として閉じるだけである。ジョブは続く (`RequeueRunningJobs`) が、ファイルを見つける段階は続かない。受け入れ条件 6 (「Close を選んで再起動した後、スキャンは止まったところから続く」) と停止のエッジケースがこれを必要とする。スキャンは大きさと mtime が変わっていないファイルを何もせずに通り過ぎる (`internal/scanner/scanner.go`) ので、最初からやり直しても続けるのと同じ結果になる。コンテナの再起動で止まったスキャンも同じ理由で続けるほうがよく、VVMDM の起動方法によって振る舞いを変える理由はない。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| デスクトップのビルドでだけ再開始する | 不採用: 分岐が増えるだけで、Docker のユーザーは中断したスキャンを手で再開始しなければならないままになる。 |
| スキャンの位置 (フォルダのどこまで進んだか) を記録し、そこから再開する | 不採用: やり直しは変わっていないファイルを飛ばすので、得られるものが記録と再開の仕組みに見合わない。 |
| `RecoverInterrupted` がスキャンを閉じたときだけ再開始する | 不採用: 正常に止まったスキャンは停止時にすでに `interrupted` として閉じられているので、終了の確認で「Close」を選んでも (受け入れ条件 6) 続かない。 |

## R-10: 起動時のどの失敗も Windows の標準のダイアログに理由を示し、`logs` の下に記録する {#r-10-every-startup-failure-shows-its-reason-in-a-standard-windows-dialog-and-is-logged-under-logs}

**決定**: ウィンドウを表示する前に、アプリは次のことを順に確認し、失敗したら理由とすべきことをダイアログに示して終了する (文言は [contracts/windows-app.md、起動の引数と失敗のメッセージ](contracts/windows-app.md#startup-arguments-and-failure-messages)):

1. zip の中から直接動かしていない (exe が一時フォルダの下になく、`ffmpeg\ffmpeg.exe` と `ffmpeg\ffprobe.exe` が exe の隣にある)。
2. データの場所に書き込める。
3. WebView2 Runtime が入っている。
4. DB が開き、マイグレーションが通る。
5. ポートで待ち受けられる。

ウィンドウが現れる前にサーバーは待ち受けを終えているので、ウィンドウが空白のままになることはない。ログは JSON のままで `logs\vvmdm.log` に出る。起動ごとに前回の実行のログを `vvmdm.1.log` に移す。ダイアログはログの場所も示す。

**理由**: エッジケース「ウィンドウが空白のままにならず、起動できない理由がわかる」と「黙って終了せず、すべきことがわかる」。GUI の exe には標準出力がないので、ログファイルがなければ原因を追えない。エクスプローラーが zip の中の exe を実行すると、その exe だけを一時フォルダに展開するので、隣に同梱の `ffmpeg` がないことでその場合を検出できる。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| WebView2 に失敗のページを表示する | 不採用: WebView2 Runtime がないときや初期化に失敗したときには表示できない。 |

## R-11: `ffmpeg` は exe の隣の `ffmpeg\` フォルダから来て `PATH` の先頭に加え、子プロセスはジョブオブジェクトの中でコンソールウィンドウなしで動く {#r-11-ffmpeg-comes-from-the-ffmpeg-folder-next-to-the-exe-added-first-to-path-and-child-processes-run-without-a-console-window-inside-a-job-object}

**決定**:

- 起動時、デスクトップのビルドは自分の `PATH` の先頭に `<exe folder>\ffmpeg` を加える。`internal/media` のコマンド名 (`ffmpeg`/`ffprobe`) と `exec.LookPath` の確認はそのままである。
- `internal/media` が起動するすべての `ffmpeg`/`ffprobe` は、Windows で `CREATE_NO_WINDOW` を得る (`_windows.go` のファイルの 1 つの関数が `exec.Cmd` にそれを設定する。ほかの OS では何もしない)。
- 起動時の最初に、デスクトップのビルドは自身を `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` 付きのジョブオブジェクトに入れるので、子プロセスは同じジョブに入る。

**理由**: 要件 2 と 3。GUI サブシステムの親から起動したコンソールの子は、子ごとにコンソールウィンドウを一瞬表示する (要件 3)。プロセスが強制終了されたとき、ライブ変換の `ffmpeg` が残って動き続けることはない (要件 5 の「常駐しない」)。フォルダを `PATH` の先頭に加えると、ユーザーが別の `ffmpeg` を入れていても同梱のバージョンが優先され、`internal/media` の 6 か所のコマンド名を設定可能にしなくて済む。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| `ffmpeg`/`ffprobe` のパスを設定として `internal/media` の各箇所に渡す | 不採用: 呼び出し側はデスクトップのビルドだけで、6 つの定数とそのテストを変えることになる。 |
| `CREATE_NO_WINDOW` をデスクトップのビルドでだけ設定する | 不採用: たとえばタスクスケジューラーからウィンドウなしでバイナリを直接動かす場合にも同じ問題があり、それを設定してもどこにも害はない。 |

## R-12: 同梱の `ffmpeg` は Gyan.dev の Windows 用の「essentials」ビルドで、バージョンと SHA-256 で固定する {#r-12-the-bundled-ffmpeg-is-gyandevs-windows-essentials-build-pinned-by-version-and-sha-256}

**決定**: ビルドスクリプトは、`GyanD/codexffmpeg` の GitHub Releases にある `ffmpeg-<version>-essentials_build.zip` のバージョンと SHA-256 を固定してダウンロードし、`ffmpeg.exe`、`ffprobe.exe`、FFmpeg の LICENSE、ソースの場所だけを zip に入れる ([contracts/windows-app.md、zip の中身](contracts/windows-app.md#zip-contents))。Windows のランナーで、ビルドは `ffmpeg -hide_banner -encoders` が `h264_nvenc` と `h264_qsv` を挙げることを確かめ、そうでなければ失敗する。対応する Windows のハードウェアエンコーダーは [hardware-encoding.md](../../docs/design-docs/hardware-encoding.md) の NVENC と QSV のままで、AMF は加えない。

**理由**: 要件 2 と 3、受け入れ条件 3。Gyan.dev の Releases はバージョンごとに安定した URL を保つので、固定したバージョンを後で再現できる。essentials は NVENC と QSV (oneVPL) を含み、full より小さい。受け入れ条件 3 の「Supported GPU」は、今の設計の Windows の候補である NVENC と QSV を意味する。AMF を加えるのは、エンコーダーの選択肢を加える別の変更である。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| BtbN/FFmpeg-Builds | 不採用: バージョンごとの固定された Release を保たない (古い日付の自動ビルドは消える) ので、固定したバージョンを後で取得できない。 |
| ユーザーに `ffmpeg` を入れてもらう | 不採用: 要件 2 に反する。 |

## R-13: GitHub Actions は `v*` タグで配布物をビルドし、GitHub Release に添付する {#r-13-github-actions-builds-the-distribution-and-attaches-it-to-a-github-release-on-a-v-tag}

**決定**: 新しいワークフロー `.github/workflows/windows-app.yml` は、`v*` タグの push と手動の dispatch で動く。Linux のジョブが SPA をビルドし、`GOOS=windows GOARCH=amd64 CGO_ENABLED=0` と `-tags desktop -ldflags "-H=windowsgui -X main.version=<version>"` で `VVMDM.exe` をビルドし、R-12 の `ffmpeg` とともに zip にする。Windows のジョブが同梱の `ffmpeg` のエンコーダーを確認し (R-12)、タグなら zip を GitHub Release に添付する。手動実行ではワークフローのアーティファクトとして残す。ローカルでは `task build-windows-app` が同じ zip をビルドする。exe のアイコンとマニフェスト (DPI 対応、Common Controls v6) は、`github.com/tc-hib/go-winres` で生成した `.syso` として埋め込む。対象は `windows/amd64` だけである。

**後の改訂**: `main` へのすべての push でも zip をビルドし、1 つだけの `nightly` プレリリースの zip を置き換える。そのタグはそのコミットに移るので、タグを切らずに最新のビルドをダウンロードできる。`v*` タグは引き続きユーザーにとってのバージョンの区切りを示す。今の規則は [docs/design-docs/windows-app.md](../../docs/design-docs/windows-app.md#distribution) にある。

**理由**: 要件 1 (zip を配布する)。今の CI は `main` への push で Docker イメージを公開するだけで、ユーザーがダウンロードできる配布物の置き場がない。マニフェストがないと、WebView2 は高 DPI の画面でぼやけて描画される。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| `windows/arm64` もビルドする | 不採用: 固定できる arm64 の `ffmpeg` のビルドがなく、Windows on ARM は x64 の exe をエミュレーションで動かす。 |

## R-14: LAN アクセスは settings テーブルに保存し、切り替えるとリスナーを開き直し、既定はループバックだけである {#r-14-lan-access-is-stored-in-the-settings-table-switching-it-reopens-the-listener-and-the-default-is-loopback-only}

**決定**:

| 観点 | 決定 |
| --- | --- |
| 保存 | 既存の `settings` テーブルのキー `desktop.lan_access` (値は `true`/`false`。行がないときは false)。マイグレーションは加えない。 |
| 待ち受け | false なら `127.0.0.1:<port>`、true なら `0.0.0.0:<port>`。起動時、値は DB を開いた後、待ち受けの前に読む。切り替えの要求は新しいアドレスでリスナーを開き直す。今のリスナーを閉じ、同じ `http.Server` が新しいリスナーで `Serve` を呼び、確立済みの接続は続く。開き直しに失敗したときは、リスナーは古いアドレスに戻り、保存値は変わらず、応答は `409` である。 |
| 表示 | 許可している間、応答は起動しているループバックでない IPv4 アドレスごとに `http://<address>:<port>/` を持つ。 |
| ルート | 所有者だけの `GET`/`PUT /api/settings/network`。デスクトップのビルドの外では `404`。リスナーを開き直した後に保存に失敗したときは、リスナーは古いアドレスに戻り、応答は `500` なので、リスナーと保存値が食い違うことはない ([contracts/network-settings-api.md](contracts/network-settings-api.md))。 |
| 転送ヘッダー | デスクトップのビルドは `MDM_TRUSTED_PROXIES=none` として動き、転送ヘッダーを読まない。 |

**理由**: 要件 8 と 9、受け入れ条件 7。ループバックだけで待ち受けるので、LAN アクセスを許可していない間は LAN からの TCP 接続すらできず、最初の起動で Windows ファイアウォールの許可のダイアログが出ない。ダイアログはユーザーがアクセスをオンにしたときに出るので、その理由がわかる。settings テーブルは今の動画のエンコードの選択を保存している場所なので、新しい保存の仕組みは要らない。デスクトップのビルドの前にはリバースプロキシとして何も置かれないので、既定の「プライベートアドレスからの転送ヘッダーを信頼する」は、LAN の端末が自分のクライアントアドレスを偽ってサインインの試行回数の上限を逃れられるようにするだけである。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| 常に `0.0.0.0` で待ち受け、アクセスを許可していない間はループバックでない接続をすぐに切る | 不採用: 最初の起動でファイアウォールのダイアログが出て、ユーザーが何も許可していないのに「ネットワークで許可するか」を尋ねる。 |
| アクセスを許可したら、`127.0.0.1` のリスナーの隣に `0.0.0.0` のリスナーを加える | 不採用: Windows では、同じポートでワイルドカードと特定のアドレスを同時に待ち受けられるかはソケットのオプションに依存し、確実ではない。 |
| 設定をデータの場所のファイルに保持する | 不採用: 保存の仕組みが 2 つになる。DB は待ち受けの前に開いているので、テーブルで足りる。 |

## R-15: LAN アクセスのスイッチは Settings の所有者だけの節にあり、アカウントのセットアップ前には変えられない {#r-15-the-lan-access-switch-sits-in-an-owner-only-section-of-settings-and-cannot-be-changed-before-account-setup}

**決定**: Settings の画面に「Network」の節を加え、`GET /api/settings/network` が `404` を返す (デスクトップのビルドでない) ときは隠す。節は、スイッチ、アクセスを許可している間に開くアドレス、オンにするときの注意 (同じ LAN の端末が VVMDM を開けるようになる。Windows ファイアウォールが尋ねたら"private networks" で許可する。接続は HTTP なので、パスワードは暗号化されずに流れる) を示す。Settings の画面と API は所有者だけのものなので、アカウントが存在する前 (`setupRequired`) にはスイッチを変えられない。

**理由**: 要件 9 (アプリの中からアクセスを許可でき、許可している間はアドレスが見える)。スイッチを所有者だけのものにすると、エッジケース「アカウントのセットアップ前に LAN からの接続を許可しようとする」が起こりえなくなる。アクセスを許可する時点で必ずアカウントが存在するので、LAN の別の端末が先にアカウントを作る危険がなくなる。

**検討した案**:

| 案 | 判定 |
| --- | --- |
| ウィンドウの (ネイティブの) メニューに置く | 不採用: アカウントのセットアップ前に押せてしまい警告が要り、SPA の外に 2 つ目の設定画面が加わる。 |
| 警告付きのスイッチをアカウントのセットアップ画面に置く | 不採用: アカウントが存在する前に VVMDM を LAN に開く理由はない。 |
