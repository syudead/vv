---
source: specs/045-comfyui-thumbnails/research.md
sourceHash: 7aeec06017baf28f23ecd0f4b2e76ad1f15fc2a737b58e211a5f099dbea982db
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

下の ComfyUI の事実は、Issue の POC とモデルの POC (2026-10-12、Comfy Cloud、37 回の実行)、Comfy Cloud の API リファレンス (`docs.comfy.org`)、ComfyUI のサーバーのルートによる。このファイルは、この機能が加える決定だけを記録する。

## R-1: `internal/app` の画像編集のポートと、両方のプロバイダーに対する 1 つの `internal/comfyui` アダプター {#r-1-an-image-edit-port-in-internalapp-one-internalcomfyui-adapter-for-both-providers}

**決定**: `internal/app` が `ImageEditor` ポート (入力をアップロードする、実行を始める、その状態を読む、それをキャンセルする、出力を取得する) を宣言し、新しいアダプターのパッケージ `internal/comfyui` が、セルフホストの ComfyUI と Comfy Cloud の両方に対してそれを実装する。`app.ComfyUIConnection` は、`app.OllamaConnection` と同じく、サムネイルの機能から分けて接続を保存し、接続のテストを実行する。

| 案 | 判定 |
| --- | --- |
| **新しいポートと、プロバイダーの切り替えを持つ 1 つのアダプター** | 採用 |
| `app.Classifier` を再利用する | 不採用: そのポートは確率付きではい/いいえの質問に答える。画像の編集は画像を返し、状態を持って数十秒実行する |
| プロバイダーごとに 1 つ、2 つのアダプターのパッケージ | 不採用: ルート、ワークフローの形式、出力の取得は同じであり、違いは 1 つの表に収まる ([R-5](#r-5-provider-differences-stay-inside-the-adapter)) |
| `internal/httpapi` から ComfyUI を呼ぶ | 不採用: アダプターは互いに依存せず、セッションの規則 (動画ごとに 1 つ、期限切れ、適用) はユースケースである |

**理由**: プロバイダーをポートの後ろに置くことは ai-layer.md に従う。そのためサムネイルの機能はどのプロバイダーが実行するかを知らず、後の 3 つ目のプロバイダーは新しいアダプターになる。

## R-2: vv はワークフローを同梱し、名前付きノードはその内部の契約である {#r-2-vv-bundles-the-workflow-named-nodes-are-its-internal-contract}

**決定**: vv は実行する ComfyUI のワークフローを、`internal/comfyui` に埋め込み、ComfyUI の API 形式で同梱する。所有者はワークフローを登録も編集もせず、モデルを選ばない (要件 3)。アダプターは入力を入れるノードをタイトル (`_meta.title`) で見つける: `vv:frame`、`vv:prompt`、`vv:mask`、`vv:reference`、`vv:output` である ([contracts/workflow.md](contracts/workflow.md))。`vv:reference` のないものとあるものの 2 つの種類があり、実行は所有者が参照画像を添えたかどうかに合うほうを使う。

| 案 | 判定 |
| --- | --- |
| **同梱するワークフロー、2 つの種類、`vv:*` のタイトル** | 採用 |
| Settings で登録する、所有者自身の API 形式のワークフロー | 不採用: ワークフローを組んだりモデルを選んだりできる所有者は少ないので、機能は使われないままになる (Issue の `背景`) |
| 1 つのワークフローで、フレームを中立の参照にする | 不採用: POC では、フレームをそれ自身の参照にすると Qwen-Image-Edit は字幕を除かなくなった。そのノードのない種類は字幕を除いた ([R-3](#r-3-absent-inputs-get-a-neutral-value-or-the-variant-without-the-node)) |
| GPU ごとに違う手法の選択肢 | 不採用: 選んだモデルは最も軽い候補であり、テキストの除去でも最も良い ([R-12](#r-12-the-model-is-flux2-klein-4b-fp8)) ので、引き換えるものがない |
| Go のコードの中のノード id | 不採用: タイトルなら、後でモデルを変えるときにワークフローのファイルを ComfyUI のエディターで編集できるままにできる |

**理由**: タイトルがあれば、メンテナーは Go に触れずにワークフローのファイルを置き換えられ、テストは埋め込んだ各ワークフローがアダプターの入れるタイトルを持つことを確かめる。

## R-3: ない入力は中立の値を得るか、そのノードのない種類を使う {#r-3-absent-inputs-get-a-neutral-value-or-the-variant-without-the-node}

**決定**: 塗りがないとき、vv はフレーム全体を覆うマスクを送る。参照画像がないとき、vv は `vv:reference` のない種類を実行する。マスクが塗られていて指示がないとき、vv は、マスクの下にあるものを除き、周りに合わせて埋めるよう求める固定の既定の指示を送る。ワークフローは結果をマスクを通してフレームに合成するので、その外のピクセルは変わらない (受け入れ条件 4)。

| 案 | 判定 |
| --- | --- |
| **フレーム全体のマスク、参照のない種類、既定の指示** | 採用 |
| 空の (全面が黒い) マスク | 不採用: そのときワークフローは何も塗り直さないので、テキストだけの編集はフレームを変えずに返す |
| フレームを中立の参照にする | 不採用: [R-2](#r-2-vv-bundles-the-workflow-named-nodes-are-its-internal-contract) を参照 |
| マスクがあり、指示が空 | 不採用: POC では、両方のモデルがマスクしたロゴを元のとおりに描き直した (エッジケース「塗った範囲があり、指示が空」) |

**理由**: POC はフレーム全体のマスクと合成を確かめた (マスクを使った 9 回の実行で、マスクの外のピクセルは 1 つも変わらなかった)。既定の指示は POC で実行しておらず、「Run a thumbnail edit」の単位が Comfy Cloud に対してその文言を決める ([quickstart.md](quickstart.md))。

## R-4: 編集は動画ごとのメモリ上のセッションであり、ポーリングで読む {#r-4-an-edit-is-an-in-memory-session-per-video-read-by-polling}

**決定**: `app.ThumbnailEdits` は動画ごとに 1 つのセッションをメモリに保持する: フレーム、実行を始めたときに取った接続のスナップショット、実行の状態、結果の画像である。ページは、実行が進んでいる間は毎秒、結果が待っている間は 60 秒ごとにセッションを読む。5 分間読まれなかったセッションはキャンセルされて捨てられ、再起動はすべてを捨てる ([data-model.md、Edit session](data-model.md#edit-session))。

| 案 | 判定 |
| --- | --- |
| **メモリ上のセッション、ポーリング** | 採用 |
| 永続的なキューのジョブの種類 | 不採用: 実行は開いているページのものであり、再起動の後や所有者が離れた後に適用されてはならない (エッジケース)。キューの再試行と `failed` の記録は、それを取り込みの問題に表示してしまう |
| `/api/events` を通じた進み具合 | 不採用: そのストリームは、ストアのコミットの後に発行したドメインイベントをすべての閲覧者に運ぶ。実行の状態は 1 つのページのものであり、決してコミットされない |
| 位置の上書きと同じく、1 つのリクエストの中で生成する | 不採用: 実行は 10–20 秒、その準備は数十秒かかり、要件 8 と 9 は実行中の状態とキャンセルを必要とする |

**理由**: 読み取りがページのハートビートを兼ねるので、ブラウザーを閉じると、別の合図なしに 5 分以内にセッションが終わる (エッジケース「ページを離れる、またはブラウザーを閉じる」)。画像はメモリにとどまるので、クラッシュの後にディスクに何も残らない。

## R-5: プロバイダーの違いはアダプターの中にとどめる {#r-5-provider-differences-stay-inside-the-adapter}

**決定**: アダプターは各プロバイダーのルートと状態を 1 つの状態の集合 (`queued`、`preparing`、`running`、そして `succeeded`、`failed`、`cancelled`) に対応付け、接続できるときは ComfyUI の WebSocket からステップの進み具合を得る ([contracts/workflow.md、Run states](contracts/workflow.md#run-states))。

| 関心事 | セルフホストの ComfyUI | Comfy Cloud |
| --- | --- | --- |
| ベース | 保存した URL | `https://cloud.comfy.org/api` |
| 認証 | なし | `X-API-Key` ヘッダー。WebSocket は `token` クエリパラメーター |
| 状態 | `GET /queue` (待機中または実行中)、次に `GET /history/{id}` | `GET /job/{id}/status` |
| 出力 | `GET /view` | `GET /view` は署名付き URL へのリダイレクトを返し、それをキーなしで取得する |
| キャンセル | 待機中は `POST /queue` `{"delete":[id]}`、実行中は `POST /interrupt` `{"prompt_id":id}` | 待機中は `POST /queue` `{"delete":[id]}`、実行中は `POST /interrupt` |
| 接続のテスト | `GET /system_stats` | 有効なキーを必要とする `GET /jobs?limit=1` |
| 必要なノードとモデル | `GET /object_info` | `GET /object_info` |

Comfy Cloud は 2 つの状態の語彙を文書にしている: API リファレンスと稼働中のサービスは `submitted`、`queued_waiting`、`preparing`、`executing`、`success`、`error`、`non_retryable_error`、`lost`、`cancel_requested`、`cancelled` を返し、公開されている OpenAPI スキーマは `waiting_to_dispatch`、`pending`、`in_progress`、`completed` を挙げ、モデルの POC はジョブの状態のルートから `preprocessing`、`queued_limited`、`allocated`、`pending_execution`、`executed` も見た。アダプターはそのすべてを対応付け ([contracts/workflow.md、Run states](contracts/workflow.md#run-states))、どちらにもない値では、既知の値が届くか時間の上限が実行を終えるまで、実行の前の状態を保つ。Cloud の `POST /interrupt` は 1 つではなくアカウントの実行中のジョブをすべて止める。vv は実行自身の状態が `running` のときだけそれを送り、アカウントのほかの実行中のジョブを再開するのは所有者である。セルフホストの ComfyUI には `preparing` の状態がなく、要件 8 はそれを許す。

| 案 | 判定 |
| --- | --- |
| **状態には HTTP のポーリング、WebSocket はステップの進み具合だけ** | 採用 |
| すべてに WebSocket | 不採用: POC は Comfy Cloud の WebSocket に届かなかったので、状態はそれに依存してはならない |
| HTTP だけで、ステップの進み具合なし | 不採用: 要件 8 はプロバイダーが報告するときにより細かい進み具合を求め、ComfyUI はステップを WebSocket でしか報告しない |

**理由**: 接続に失敗した WebSocket は進み具合をないままにし、決して実行を失敗させない。そのための Go の依存は `github.com/coder/websocket` であり、dependency-updates.md の下で加える。推移的な依存はない。

## R-6: 適用した画像は SQLite のユーザーデータであり、サムネイルのファイルはそこから作り直す {#r-6-the-applied-image-is-user-data-in-sqlite-the-thumbnail-file-is-rebuilt-from-it}

**決定**: 適用は、合わせ込んだ JPEG を内容の鍵をキーにした新しいユーザーデータのテーブル `thumbnail_images` に保存し、同じバイトを `internal/artifacts` を通じてサムネイルのファイルとして公開する。`Ingest.Thumbnail` は、行があるときはフレームを取り出す代わりに保存した画像を公開する ([data-model.md](data-model.md))。

| 案 | 判定 |
| --- | --- |
| **サムネイルのジョブが公開し直す blob のテーブル** | 採用 |
| `MDM_DATA_DIR/thumbnails` のサムネイルのファイルだけ | 不採用: その内容を持つ最後の動画がなくなると `RemoveContent` がそれを削除し、内容が戻るとジョブはフレームを取り出す (要件 12) |
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

**理由**: 2048 px なら 4K のフレームの PNG はアップロードのためにおよそ 8 MB 未満に収まる。同梱のワークフローは入力をモデルのためにおよそ 1 メガピクセルに拡大縮小し、結果をフレームの大きさに戻す。

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

## R-12: モデルは FLUX.2 [klein] 4B fp8 である {#r-12-the-model-is-flux2-klein-4b-fp8}

**決定**: 同梱のワークフローは、蒸留した FLUX.2 [klein] 4B を fp8 で、`qwen_3_4b` のテキストエンコーダーと `flux2-vae` の VAE とともに、ComfyUI のコアのノードだけで 4 ステップで実行する。手法は 1 つだけ提供し、所有者は何も選ばない (要件 3)。

| 候補 | モデルの POC による判定 |
| --- | --- |
| **FLUX.2 [klein] 4B fp8** | 採用: 焼き込まれた日本語と英語の字幕を最も確実に除き、場面に属するテキストを残した。Cloud で 1 回 8–16 秒。ダウンロードはおよそ 12.4 GB。Apache-2.0 |
| Lightning の 4 ステップの LoRA を使う Qwen-Image-Edit-2511 fp8mixed | 不採用: 参照画像の雰囲気を取り入れるのは最も良いが、日本語の指示を読み違えて台所を森に置き換え、フレームの中の画面にモザイクをかけた。ダウンロードはおよそ 31 GB で、24 GB の GPU が要る |
| FLUX.2 [klein] 9B | 不採用: 非商用のライセンス、9:16 のフレームで 2 回灰色のノイズ、編集しすぎる |
| FLUX.1 Kontext dev | 不採用: 非商用のライセンス、FLUX.2 [klein] に取って代わられた |
| 有料の API ノード (FLUX.2 Pro、Nano Banana、Seedream、GPT Image) | 不採用: セルフホストの ComfyUI には 2 つ目のアカウントと課金が要る |

**理由**: 字幕とテキストの除去は Issue の主な場合であり、最も軽い候補がそれを最も良くこなすので、小さい GPU のための手法の選択肢には提供するものがない。既知の弱点は参照画像である: 参照から物をフレームに写し込み、Issue の `背景` がそれを記録している。各場合は 1 つのシードで実行し、目で判定した。

## R-13: 接続のテストはワークフローのノードとモデルのファイルを確かめる {#r-13-the-connection-test-checks-the-workflows-nodes-and-model-files}

**決定**: 接続が応答した後、テストは `GET /object_info` を読み、同梱のワークフローのすべての `class_type` が存在すること、ローダーのノードが挙げるすべてのモデルのファイルがそのローダーの選択肢の中にあることを確かめる。確認は、欠けているノードとファイル、および各ファイルの ComfyUI のフォルダーとダウンロードのページ ([contracts/workflow.md、Required files](contracts/workflow.md#required-files)) と共に使用不可を返す。送信が欠けているノードやファイルを返した実行は、同じ一覧と共に `missing_models` として失敗する。

| 案 | 判定 |
| --- | --- |
| **接続のテストで、`/object_info` から確かめる** | 採用 |
| ComfyUI を通じてファイルをダウンロードする | 不採用: ComfyUI の API にはモデルをインストールするルートがなく、Issue の `対象外` は vv からのダウンロードを除外する |
| 実行が失敗したときだけ確かめる | 不採用: 要件 4 は接続をテストしたときに欠けているファイルを求める |

**理由**: `/object_info` は両方のプロバイダーで各ローダーが読み込めるファイルを挙げるので、1 つの確認で、モデルのないセルフホストの ComfyUI と、それを落とした Comfy Cloud を扱える。ワークフローが使うコアのノード (`Flux2Scheduler`、`GetImageSize`) より古いセルフホストの ComfyUI は、「update ComfyUI」を対処とする欠けているノードとして現れる。
