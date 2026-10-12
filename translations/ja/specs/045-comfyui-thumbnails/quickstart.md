---
source: specs/045-comfyui-thumbnails/quickstart.md
sourceHash: 12964c5d1425460b3db1f47d4818b0f39122e7887c2dbf3325cc874fafa2e0f0
---

# クイックスタート: 実際の ComfyUI に対してサムネイルを編集する {#quickstart-edit-a-thumbnail-against-a-real-comfyui}

この手順は、vv が実際のセルフホストの ComfyUI と Comfy Cloud で動くことを示す。`task check` と `task test-e2e` は偽の ComfyUI しか動かさないので、プロバイダーの通信の形式、その状態、そのタイミング、モデルの出力を捉えられない。

## 前提条件 {#prerequisites}

- feature ブランチから実行し ([development.md](../../docs/how-to/development.md))、所有者としてログインし、フレームに焼き込みの字幕と隅のロゴが映るライブラリの動画を 1 つ持つ vv。
- vv のサーバーから届き、[contracts/workflow.md、Required files](contracts/workflow.md#required-files) のファイルを持つセルフホストの ComfyUI、そのうち 1 つを欠く 2 つ目のセルフホストの ComfyUI (または `flux2-vae.safetensors` を移動した同じ ComfyUI)、クレジットのある Comfy Cloud の API キー。

## 手順 {#steps}

表を、セルフホストの ComfyUI を保存した状態で 1 回、Comfy Cloud を保存した状態で 1 回実行する。

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| Settings: 接続を保存し、それからテストする | 「Connected」 | 1 |
| 誤った URL (セルフホスト) または誤ったキー (Cloud) を保存し、それからテストする | 原因を挙げる失敗 | 1 |
| セルフホストのみ: ファイルを欠く ComfyUI を保存し、それからテストする | 欠けているファイルと、そのフォルダーとダウンロードのページ | 2 |
| 動画のページ、別のブラウザーのゲストのセッション | 編集の入口がない | 9 |
| 接続を削除し、動画のページを開く | 入口は無効で、Settings に接続が必要だと示す | 要件 14 |
| 字幕のあるフレームで一時停止し、編集を始め、「Remove the captions」と入力し、実行する | 状態は経過時間付きで queued と running を経る (Cloud: preparing も)。字幕のない結果がフレームの隣に現れる | 3、5 |
| 隅のロゴの上だけを塗り、指示を空のままにし、実行する | ロゴが消え、塗った範囲の外は何も変わっていない ([research.md R-3](research.md#r-3-absent-inputs-get-a-neutral-value-or-the-variant-without-the-node)) | 4、エッジケース |
| 参照画像を添え、「Use the colours of the reference」と入力し、実行する | 結果は参照の色を取り入れる | 4 |
| 実行し、待機中か実行中にキャンセルする | 実行は cancelled として終わる。サムネイルは変わらない | 7 |
| 実行し、タブを閉じ、5 分待ち、再読み込みする | 編集は適用されない。サムネイルは変わらない | エッジケース |
| 実行し、破棄する | サムネイルは変わらない | 7 |
| 実行し、適用する | 動画のページ、そのライブラリのカード、別の動画の関連動画にあるそのエントリが、動画の縦横比で新しい画像を示す | 6 |
| 再読み込みし、全体のスキャンを始める | 新しい画像は残る | 6 |
| 「Use the frame instead」 | サムネイルは再びフレームになる | 8 |
| 再び適用し、「use current frame」で新しい位置を選ぶ | サムネイルはそのフレームになる | 8 |
