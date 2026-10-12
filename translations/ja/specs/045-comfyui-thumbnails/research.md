---
source: specs/045-comfyui-thumbnails/research.md
sourceHash: 4110c68b8446c4e757e3c109ba1b94410badf1bc262c16922d17f89225b3707e
---

# 調査: 動画のフレームを ComfyUI で編集し、サムネイルとして使う {#research-edit-a-video-frame-with-comfyui-and-use-it-as-the-thumbnail}

親 Issue: #942。引き継ぐ決定:

| 項目 | 正本 |
| --- | --- |
| 境界、ユーザーデータと作り直せるファイル、認証の境界 | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| AI 層: `internal/app` のポート、`cmd/mdm` で配線するプロバイダーのアダプター、機能から分けた接続の設定、リクエストの本文の URL にリクエストを送らない | [ai-layer.md](../../docs/design-docs/ai-layer.md) |
| サムネイルの位置の上書き、生成のロック、`thumbnailUrl` のリビジョン | [029 調査、R-4](../029-video-overrides/research.md#r-4-a-thumbnail-position-is-generated-within-the-request-then-recorded)、[R-5](../029-video-overrides/research.md#r-5-the-import-thumbnail-job-also-uses-the-chosen-position-the-automatic-rule-stays-in-internalmedia)、[R-6](../029-video-overrides/research.md#r-6-the-thumbnailurl-version-includes-the-position-revision) |
| エラーの形と理由のコード | [023 error-api](../023-english-i18n/contracts/error-api.md) |
| 依存の追加 | [dependency-updates.md](../../docs/how-to/dependency-updates.md) |

下の ComfyUI の事実は、Issue の POC (2026-10-12、Comfy Cloud)、Comfy Cloud の API リファレンス (`docs.comfy.org`)、ComfyUI のサーバーのルートによる。このファイルは、この機能が加える決定だけを記録する。

## R-1: `internal/app` の画像編集のポートと、両方のプロバイダーに対する 1 つの `internal/comfyui` アダプター {#r-1-an-image-edit-port-in-internalapp-one-internalcomfyui-adapter-for-both-providers}

**決定**: `internal/app` が `ImageEditor` ポート (入力をアップロードする、実行を始める、その状態を読む、それをキャンセルする、出力を取得する) を宣言し、新しいアダプターのパッケージ `internal/comfyui` が、セルフホストの ComfyUI と Comfy Cloud の両方に対してそれを実装する。`app.ComfyUIConnection` は、`app.OllamaConnection` と同じく、サムネイルの機能から分けて接続を保存し、接続のテストを実行する。

| 案 | 判定 |
| --- | --- |
| **新しいポートと、プロバイダーの切り替えを持つ 1 つのアダプター** | 採用 |
| `app.Classifier` を再利用する | 不採用: そのポートは確率付きではい/いいえの質問に答える。画像の編集は画像を返し、状態を持って数十秒実行する |
| プロバイダーごとに 1 つ、2 つのアダプターのパッケージ | 不採用: ルート、ワークフローの形式、出力の取得は同じであり、違いは 1 つの表に収まる ([R-5](#r-5-provider-differences-stay-inside-the-adapter)) |
| `internal/httpapi` から ComfyUI を呼ぶ | 不採用: アダプターは互いに依存せず、セッションの規則 (動画ごとに 1 つ、期限切れ、適用) はユースケースである |

**理由**: プロバイダーをポートの後ろに置くことは ai-layer.md に従う。そのためサムネイルの機能はどのプロバイダーが実行するかを知らず、後の 3 つ目のプロバイダーは新しいアダプターになる。

## R-2: 所有者の API 形式のワークフローの名前付きノードが契約のすべてである {#r-2-named-nodes-in-the-owners-api-format-workflow-are-the-whole-contract}

**決定**: 所有者は、ComfyUI の API 形式で書き出したワークフローを 1 つ登録する。vv はその入力と出力をノードのタイトル (`_meta.title`) で見つける: `vv:frame` と `vv:output` は必須である。`vv:prompt`、`vv:mask`、`vv:reference` は省略でき、あるものはそれぞれ動画のページでその入力を有効にする ([contracts/workflow.md](contracts/workflow.md))。vv はそれらのノードの入力だけを書き換え、ほかのノードは決して編集しない。

| 案 | 判定 |
| --- | --- |
| **所有者のノードの `vv:*` のタイトル** | 採用: POC はこの方法で動いた |
| Settings で入力するノード id | 不採用: 所有者がワークフローを組み直すと id は変わり、所有者は id を見つけるために JSON を読まなければならない |
| クラス (`LoadImage`、`CLIPTextEncode`) で入力を見つける | 不採用: `LoadImage` ノードを 2 つ持つワークフローはあいまいであり、要件 3 はノードを仮定することを除外する |
| 同梱する既定のワークフロー | 不採用: Issue の `対象外` はワークフローとモデルの同梱を除外する |
| vv が実行ごとにサンプラーのシードを無作為にする | 不採用: サンプラーのノードを仮定する。所有者は違う結果を得るためにプロンプトを変え (要件 9)、新しいシードを求めるワークフローは無作為なシードのノードを使う |

**理由**: タイトルは ComfyUI のエディターで見えて編集でき、API 形式の書き出しでも残るので、所有者はワークフローを実行する場所でそれを用意する。登録時の検証は、どの実行よりも前にエッジケース「ワークフローが受け取れない入力」に答える: ページはノードのない操作部品を無効にする。

## R-3: ない省略可能な入力は中立の値を得る {#r-3-absent-optional-inputs-get-a-neutral-value}

**決定**: ワークフローが `vv:mask` を持ち、所有者が何も塗っていないとき、vv はフレーム全体を覆うマスクを送る。`vv:reference` を持ち、所有者が何も添えていないとき、vv はフレームを送る。`vv:prompt` を持ち、所有者が何も入力していないとき、vv は空の文字列を送る。

| 案 | 判定 |
| --- | --- |
| **中立の値** | 採用 |
| 空の (全面が黒い) マスク | 不採用: インペインティングのワークフローは何も塗り直さないので、テキストだけの編集はフレームを変えずに返す |
| 送信の前に使わないノードを除く | 不採用: その下流のノードが入力を失い、実行は検証で失敗する |
| 入力の組み合わせごとに 1 つのワークフローを求める | 不採用: 要件 3 は登録するワークフローを 1 つとしており、所有者は最大 8 つを保つことになる |

**理由**: 「フレーム全体を編集する」と「フレームがそれ自身の参照である」が、その入力のない編集の意味であるので、1 つのワークフローが要件 5 の許すすべての組み合わせに使える。

## R-4: 編集は動画ごとのメモリ上のセッションであり、ポーリングで読む {#r-4-an-edit-is-an-in-memory-session-per-video-read-by-polling}

**決定**: `app.ThumbnailEdits` は動画ごとに 1 つのセッションをメモリに保持する: フレーム、実行を始めたときに取った接続とワークフローのスナップショット、実行の状態、結果の画像である。ページは、実行が進んでいる間は毎秒、結果が待っている間は 60 秒ごとにセッションを読む。5 分間読まれなかったセッションはキャンセルされて捨てられ、再起動はすべてを捨てる ([data-model.md、Edit session](data-model.md#edit-session))。

| 案 | 判定 |
| --- | --- |
| **メモリ上のセッション、ポーリング** | 採用 |
| 永続的なキューのジョブの種類 | 不採用: 実行は開いているページのものであり、再起動の後や所有者が離れた後に適用されてはならない (エッジケース)。キューの再試行と `failed` の記録は、それを取り込みの問題に表示してしまう |
| `/api/events` を通じた進み具合 | 不採用: そのストリームは、ストアのコミットの後に発行したドメインイベントをすべての閲覧者に運ぶ。実行の状態は 1 つのページのものであり、決してコミットされない |
| 位置の上書きと同じく、1 つのリクエストの中で生成する | 不採用: 実行は 10–20 秒、その準備は数十秒かかり、要件 7 と 8 は実行中の状態とキャンセルを必要とする |

**理由**: 読み取りがページのハートビートを兼ねるので、ブラウザーを閉じると、別の合図なしに 5 分以内にセッションが終わる (エッジケース「ページを離れる、またはブラウザーを閉じる」)。画像はメモリにとどまるので、クラッシュの後にディスクに何も残らない。

## R-5: プロバイダーの違いはアダプターの中にとどめる {#r-5-provider-differences-stay-inside-the-adapter}

**決定**: アダプターは各プロバイダーのルートと状態を 1 つの状態の集合 (`queued`、`preparing`、`running`、そして `succeeded`、`failed`、`cancelled`) に対応付け、接続できるときは ComfyUI の WebSocket からステップの進み具合を得る ([contracts/workflow.md、Run states](contracts/workflow.md#run-states))。

| 関心事 | セルフホストの ComfyUI | Comfy Cloud |
| --- | --- | --- |
| ベース | 保存した URL | `https://cloud.comfy.org/api` |
| 認証 | なし | `X-API-Key` ヘッダー。WebSocket は `token` クエリパラメーター |
| 状態 | `GET /queue` (待機中または実行中)、次に `GET /history/{id}` | `GET /job/{id}/status` |
| 出力 | `GET /view` | `GET /view` は署名付き URL へのリダイレクトを返し、それをキーなしで取得する |
| キャンセル | 待機中は `POST /queue` `{"delete":[id]}`、実行中は `POST /interrupt` `{"prompt_id":id}` | `POST /queue` `{"delete":[id]}` |
| 接続のテスト | `GET /system_stats` | 有効なキーを必要とする `GET /jobs?limit=1` |

Cloud の状態は次のように対応する: `submitted`、`queued_waiting` → `queued`。`preparing` → `preparing`。`executing` → `running`。`success` → `succeeded`。`error`、`non_retryable_error`、`lost` → `failed`。`cancel_requested`、`cancelled` → `cancelled`。セルフホストの ComfyUI には `preparing` の状態がなく、要件 7 はそれを許す。

| 案 | 判定 |
| --- | --- |
| **状態には HTTP のポーリング、WebSocket はステップの進み具合だけ** | 採用 |
| すべてに WebSocket | 不採用: POC は Comfy Cloud の WebSocket に届かなかったので、状態はそれに依存してはならない |
| HTTP だけで、ステップの進み具合なし | 不採用: 要件 7 はプロバイダーが報告するときにより細かい進み具合を求め、ComfyUI はステップを WebSocket でしか報告しない |

**理由**: 接続に失敗した WebSocket は進み具合をないままにし、決して実行を失敗させない。そのための Go の依存は `github.com/coder/websocket` であり、dependency-updates.md の下で加える。推移的な依存はない。

## R-6: 適用した画像は SQLite のユーザーデータであり、サムネイルのファイルはそこから作り直す {#r-6-the-applied-image-is-user-data-in-sqlite-the-thumbnail-file-is-rebuilt-from-it}

**決定**: 適用は、合わせ込んだ JPEG を内容の鍵をキーにした新しいユーザーデータのテーブル `thumbnail_images` に保存し、同じバイトを `internal/artifacts` を通じてサムネイルのファイルとして公開する。`Ingest.Thumbnail` は、行があるときはフレームを取り出す代わりに保存した画像を公開する ([data-model.md](data-model.md))。

| 案 | 判定 |
| --- | --- |
| **サムネイルのジョブが公開し直す blob のテーブル** | 採用 |
| `MDM_DATA_DIR/thumbnails` のサムネイルのファイルだけ | 不採用: その内容を持つ最後の動画がなくなると `RemoveContent` がそれを削除し、内容が戻るとジョブはフレームを取り出す (要件 11) |
| `MDM_DATA_DIR` の下のユーザーの画像のための 2 つ目のファイルの場所 | 不採用: バックアップと復旧は SQLite を対象とし、データディレクトリのファイルを作り直せるものとして扱う ([running-vv.md、Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery))。新しい場所はその規則を変える |

**理由**: 合わせ込んだ画像は 50–150 KB なので、編集した動画ごとに 1 行でも SQLite は小さいままであり、画像は所有者のほかのデータとともにバックアップされる。`internal/artifacts` がサムネイルのファイルの唯一の所有者のままであり、ファイルは作り直せるままである。

## R-7: 生成した画像は保存の前に動画のフレームに合わせる {#r-7-the-generated-image-is-fitted-to-the-videos-frame-before-it-is-stored}

**決定**: `internal/media` は結果を、長辺を 640 px として、動画の表示の縦横比 (`Video.Width`、`Video.Height` から) に拡大縮小して中央で切り抜き、取り出したサムネイルと同じ品質の JPEG にする。動画の大きさがわからないときは、画像は自身の縦横比を保つ。

| 案 | 判定 |
| --- | --- |
| **動画の縦横比に合わせる** | 採用 |
| 結果の大きさと縦横比を保つ | 不採用: カードは、カードのスクラブのプレビューと細長い動画の背景を動画の縦横比で配置する (`isNarrowVideo`、`ScrubFrame`) ので、違う縦横比はそれらをサムネイルに対してずらす |
| 縦横比にレターボックスで合わせる | 不採用: 帯がカードの `object-contain` の枠の中に 2 つ目の境界として見える |

**理由**: エッジケース「違う大きさや縦横比が表示を壊さない」が、どの画面も変えずにすべての画面で成り立ち、ワークフローはどの大きさでも出力できる。

## R-8: フレームはサーバーが最大 2048 px で取り出す {#r-8-the-frame-is-extracted-by-the-server-at-up-to-2048-px}

**決定**: 編集を始めると、与えた位置のフレームを、長辺を動画のものとして 2048 px を上限とする PNG として `ffmpeg` で取り出す。ページはそのフレームを塗るために表示し、マスクは同じピクセルの大きさの PNG である。

| 案 | 判定 |
| --- | --- |
| **ソースの大きさでのサーバーでの取り出し、上限付き** | 採用 |
| `<video>` 要素をキャンバスに取り込む | 不採用: ライブ変換したストリームはファイルのより低品質なエンコードであり、画像は後の位置の上書きが取り出すものと異なる |
| 640 px のサムネイルの取り出し | 不採用: その大きさでは、編集のモデルは除くよう求められたテキストを見失う |

**理由**: 2048 px なら 4K のフレームの PNG はアップロードのためにおよそ 8 MB 未満に収まる。POC の `ImageScaleToTotalPixels` がしたように、所有者のワークフローが入力をそのモデルの大きさに拡大縮小する。

## R-9: API キーは書き込み専用である {#r-9-the-api-key-is-write-only}

**決定**: キーは `settings` に `comfyui.api_key` として保存する。読み取りは `apiKeySet` を返し、決してキーを返さない。`apiKey` のない保存はキーを保ち、空の文字列はキーを消す。

| 案 | 判定 |
| --- | --- |
| **平文の値、決して返さない** | 採用 |
| SQLite の中で暗号化する | 不採用: 暗号化の鍵は同じデータディレクトリに置かれるので、データベースを読める者が元に戻せないものは何も守らない |
| API トークンと同じくハッシュにする | 不採用: vv はキーを Comfy Cloud に送るので、サーバーが読めなければならない |

**理由**: 要件 2 は画面についてのものである。応答は決してキーを運ばないので、ページもキャッシュされた応答もそれを表示できない。

## R-10: 適用、元に戻す操作、位置の上書きは生成のロックで順序付ける {#r-10-apply-revert-and-the-position-override-are-ordered-by-the-generation-lock}

**決定**: 適用、元に戻す操作、位置の設定はそれぞれ内容のサムネイルの生成のロックの中で実行するので、最後に確定したものが勝つ。適用は、動画がセッションの開始時の内容の鍵をまだ持つことを確かめ、位置の設定は `thumbnail_images` の行を削除し、元に戻す操作はそれを削除して、記録した位置または自動の規則からサムネイルを作り直す。

| 案 | 判定 |
| --- | --- |
| **3 つすべてに 1 つのロック** | 採用 |
| 位置を設定したときに編集のセッションをキャンセルする | 不採用: 所有者はその後でも適用でき、適用が後に確定した操作である |
| 画像を適用している間は位置の列を空のままにする | 不採用: 元に戻す操作は、所有者が編集の前に選んだ位置を失う |

**理由**: これは、029 がすでに使う仕組み ([029 R-4](../029-video-overrides/research.md#r-4-a-thumbnail-position-is-generated-within-the-request-then-recorded)) によるエッジケース「最後に確定した操作が勝つ」である。移動したファイルや置き換えたファイルは内容の鍵を変えるので、適用の確認がエッジケース「動画がなくなった、または今は別の内容を持つ」に答える。

## R-11: 新しいセッションは前のものを置き換え、実行は 10 分に制限する {#r-11-a-new-session-replaces-the-previous-one-a-run-is-limited-to-10-minutes}

**決定**: セッションを持つ動画の編集を始めると、その実行をキャンセルしてセッションを捨てる。セッションで実行を始めると、実行中の実行をキャンセルする。送信から 10 分後に終わりの状態にない実行はキャンセルされ、`timed_out` として失敗する。

| 案 | 判定 |
| --- | --- |
| **置き換えと制限** | 採用 |
| 実行中は 2 つ目の編集を拒む | 不採用: 閉じたタブが最大 5 分間その動画をふさぐ |
| 時間の制限なし | 不採用: エッジケース「終わらない実行」は、所有者が読める失敗で終わらなければならない |

**理由**: 10 分は POC の数十秒の準備を何倍も覆う。それを過ぎた待機中の Comfy Cloud のジョブはキャンセルされるので、所有者が去った後にクレジットを使わない。
