---
source: docs/design-docs/work-info-plugins.md
sourceHash: a3e97f2b06da8c0b7af6cf2bd34bd6268d476f59065c8bf02b4073a0594b7221
---

# 作品情報プラグイン: サンドボックス、制限、ネットワーク {#work-info-plugins-sandbox-limits-and-network}

VVMDM は、外部のサイトから作品情報を読み取る WebAssembly のモジュールである作品情報プラグインを、自分のプロセスの中で wazero により実行する。アダプタは [`internal/wasmplugin`](../../internal/wasmplugin) で、[`internal/app/work_info_plugin_runtime.go`](../../internal/app/work_info_plugin_runtime.go) の `WorkInfoPluginRuntime` ポートの背後にある。プラグインの契約は [作品情報プラグインを書く](../how-to/work-info-plugins.md) で、各選択の理由はこの機能の [research.md](../../specs/049-work-info-plugins/research.md) (R-1 から R-5) にある。

図は、モジュールがアップロードから実行の結果までにたどる過程を示す。

```mermaid
flowchart LR
  upload[モジュールのバイト列] --> inspect[検査: コードは実行しない]
  inspect -->|問題| refused[マニフェストは返さず、問題を返す]
  inspect -->|マニフェスト| approved[所有者が承認]
  approved --> slot[1 回分の実行枠、最大 10 秒待つ]
  slot --> run[新しいインスタンス: 標準入力に要求、標準出力に応答]
  run --> limits{最初に達した制限}
  limits -->|なし| result[候補または作品情報]
  limits -->|時間、メモリ、出力| failure[実行の失敗]
```

## 検査はモジュールを決して実行しない {#inspection-never-runs-the-module}

`Inspect` はモジュールをコンパイルし、その `vv.plugin` マニフェスト、インポート、エクスポート、テーブルを読み、最初に見つけた [data-model.md](../../specs/049-work-info-plugins/data-model.md#work_info_plugins) の `problem` を返す ([`runtime.go`](../../internal/wasmplugin/runtime.go))。

所有者はプラグインをその名前、バージョン、設定から承認するので、これらは所有者がまだ承認していないコードからではなく、カスタムセクションから得る。wazero のコンパイル済みモジュールはテーブルも関数以外のインポートも見せないので、[`binary.go`](../../internal/wasmplugin/binary.go) がインポートとテーブルのセクションをバイト列から読む。

| 確認する内容 | 満たさないときの問題 |
| --- | --- |
| バイト列が、メモリの初期サイズが 128 MiB 以下の WebAssembly モジュールとしてコンパイルできる | `not_wasm` |
| `vv.plugin` セクションがある | `manifest_missing` |
| マニフェストが、マニフェストの規則に従う JSON オブジェクトである | `manifest_invalid` |
| `contract` が `1` である。他のフィールドより先に確認する | `contract_unsupported` |
| すべてのインポートが、vv のものと同じシグネチャを持つ `wasi_snapshot_preview1` または `vv` の関数である | `import_unsupported` |
| `_start` が何も受け取らず何も返さず、`memory` がエクスポートされている | `export_invalid` |
| すべてのテーブルの最小値と、宣言された最大値があればその最大値が、100,000 要素以下であり、各テーブルの宣言された最大値 (ないテーブルは 100,000) を全テーブルで合計した値も 100,000 要素以下である | `table_too_large` |

## テーブルにはコンパイルの前に最大値を与える {#tables-get-a-maximum-before-compiling}

Go の `wasip1` リンカが関数テーブルを書き出すときのように最大値を宣言していないテーブルには、検査時と実行前の両方で、コンパイルの前にモジュールのバイト列へ 100,000 要素の最大値を書き込む ([`binary.go`](../../internal/wasmplugin/binary.go) の `capTables`)。

テーブルの要素は線形メモリの上限の外にあるホストのメモリなので、上限のない `table.grow` はサーバーのメモリを際限なく使えてしまう。そのようなテーブルを拒否すると、Go でビルドしたプラグインをすべて拒否することになる。同じ理由で、上限はモジュール全体にも適用する。そうしないと、それぞれ 100,000 要素以内の多数のテーブルが合計で上限を超えうる。

## 実行にはファイルも環境もなく、制限は固定である {#a-run-has-no-files-no-environment-and-fixed-limits}

各実行は新しいインスタンスを開始し、要求の JSON を標準入力に書き、応答の JSON を標準出力から読む ([`run.go`](../../internal/wasmplugin/run.go))。インスタンスには時計と乱数源を与え、事前に開いたディレクトリ、環境変数、引数は与えない。ネットワークへの唯一の経路は `vv` のホスト関数で、[プラグインは公開インターネットのアドレスにだけ到達する](#plugins-reach-only-public-internet-addresses) で説明する。

| 制限 | 値 | 達したとき |
| --- | --- | --- |
| モジュールのサイズ | 16 MiB | `Inspect` が `domain.ErrWorkInfoPluginTooLarge` を返す |
| 線形メモリ | 128 MiB (2048 ページ) | `memory.grow` が `-1` を返し、モジュールを閉じ、プラグインがその後正常に終了した場合でも、実行は `memory_exceeded` で失敗する |
| 実行時間 | 実行枠を取ってから 30 秒、コンパイルと HTTP を含む | コンパイルまたはモジュールを止め、実行は `timed_out` で失敗する |
| 標準出力 | 1 MiB | 実行を `output_too_large` で止める |
| 標準エラー出力 | 64 KiB を保持し、残りは捨てる | サーバーのログにだけ書く |
| 同時に行う実行 | 1。2 つ目の実行は最大 10 秒待つ | 2 つ目の実行は `busy` で失敗する |

メモリの上限は wazero のページ数の上限ではなく vv のメモリアロケータで強制する。wazero は上限を超える `memory.grow` をアロケータが見る前に拒否するので、vv はメモリの失敗を他の 0 以外の終了と区別できなくなるからである。実行が出力とメモリの両方の上限に達したときは、先に達した方が失敗を決める。応答がプラグインのエラーである実行、0 以外で終了した実行、契約から外れた出力をした実行は `plugin_failed` で失敗する。

実行時間は実行が実行枠を取った時点から始まるので、まだキャッシュされていないモジュールのコンパイルもこれに含まれる。wazero は複数のワーカーでコンパイルするときにだけ関数の間でコンテキストを確認するので、実行は 2 つのワーカーでコンパイルする。

| 採用しなかった案 | 理由 |
| --- | --- |
| 実行ごとに別のプロセス | コンテナと Windows アプリで別々の OS のサンドボックスが必要になる |
| アップロード時に `describe` 操作を実行する | 承認のために表示する名前とバージョンが、承認されていないコードから来ることになる |
| 複数の実行を同時に行う | 各実行は 128 MiB を使いうるので、1 つずつにするとプラグインのメモリが 1 モジュール分に収まる |

## プラグインは公開インターネットのアドレスにだけ到達する {#plugins-reach-only-public-internet-addresses}

プラグインはソケットを持たない。vv がプラグインの各 HTTP 要求を、`vv` のホスト関数 `http_send`、`http_meta`、`http_body` ([`host.go`](../../internal/wasmplugin/host.go)) を通じて、自分の HTTP クライアント ([`netclient.go`](../../internal/wasmplugin/netclient.go)) で実行する。クライアントは URL のホストを名前解決し、解決したすべてのアドレスを、各接続の直前にダイアラの制御フックで確認するので、リダイレクトと DNS の応答は最初の要求と同じ確認を通る。

図は、プラグインの要求からそのハンドルまたはエラーまでの、1 回の `http_send` を示す。

```mermaid
flowchart LR
  send[http_send] --> valid{有効な要求で、送信が 16 回未満か}
  valid -->|いいえ| err1[-1 または -5]
  valid -->|はい| resolve[ホストを名前解決する]
  resolve --> dial{接続時に各アドレスが許可されるか}
  dial -->|いいえ| refuse[-2、ホストを記録]
  dial -->|はい| site[インターネットのサイト]
  site -->|リダイレクト、最大 5 回| resolve
  site --> body{本文が 4 MiB 以内かつ 10 秒以内か}
  body -->|はい| handle[http_meta と http_body 用のハンドル]
  body -->|いいえ| err2[-6 または -4]
  refuse --> fail[実行の失敗: blocked_address]
```

アドレスが許可されるのは、IPv4 射影 IPv6 アドレスを IPv4 に戻したうえで、グローバルユニキャスト (IPv6 では `2000::/3` の中) であり、`refusedPrefixes` のどの範囲にも含まれず、サーバー自身のインターフェースのアドレスでもないときだけである ([`address.go`](../../internal/wasmplugin/address.go))。`refusedPrefixes` は 1 つの表で、IANA の IPv4 と IPv6 の Special-Purpose Address Registry のうち "Globally Reachable" が `True` でないすべての範囲に、`fec0::/10` と IPv4 アドレスを埋め込むプレフィックス (`64:ff9b::/96`、`64:ff9b:1::/48`、`2001::/32`、`2002::/16`) を加えたものである。サーバーのインターフェースは vv の実行中に変わりうるので、確認のたびに読む。

拒否した要求はプラグインに `-2` を返し、プラグインがその後に何を書いても、実行全体を `blocked_address` と拒否したホストで失敗させる。そのため、プラグインは vv や LAN に到達しようとしたことを所有者から隠せない。接続に失敗した要求は `-3` を返す。その後プラグインが失敗したときは、実行は `network_error` と到達できなかった最初のホストで失敗する。所有者はホストには対処できるが、プラグインのメッセージだけでは対処できないからである。

| 制限 | 値 | 達したとき |
| --- | --- | --- |
| 要求 | 実行ごとに 16 回、`GET` と `POST`、`http` と `https` | 17 回目の要求は `-5` を返す。他のメソッドやスキームは `-1` を返す |
| リダイレクト | 要求ごとに 5 回 | 6 回目のリダイレクトの応答はそのままプラグインに渡す |
| 時間 | 要求ごとに 10 秒、送信から本文の最後のバイトまで | 要求は `-4` を返す |
| 応答の本文 | 4 MiB | 要求は `-6` を返す |

プロキシを使うとダイアラの確認がプロキシのアドレスしか見なくなるので、クライアントは `HTTP_PROXY` などのプロキシの変数を無視する。プラグインが設定した `Host` とホップバイホップのヘッダーは捨て、プラグインが `User-Agent` を設定しないときは自分のものを送る。本文は応答のヘッダーとともにバイト列としてプラグインに届くので、ページの文字コードはプラグイン自身がデコードする。

アドレスの確認はこのアダプタにだけある。ComfyUI と Ollama のクライアントは、それらのサービスが動く LAN に引き続き到達する。

| 採用しなかった案 | 理由 |
| --- | --- |
| 要求の前にホスト名を名前解決して確認する | 接続が名前を再び解決して別の応答を得ることがあり、リダイレクトは確認を通らない |
| プラグインごとのホスト名の許可リスト | 公開の名前が LAN のアドレスに解決されうる |
| 拒否をプラグインにだけ返す | プラグインがそれを無視でき、所有者は到達しようとしたことを知れない |
| 接続、TLS、ヘッダーで別々のタイムアウト | 遅いサイトが、要求の 10 秒の制限より前に `-4` ではなく `-3` を返しうる |
