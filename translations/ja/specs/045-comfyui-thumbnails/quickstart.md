---
source: specs/045-comfyui-thumbnails/quickstart.md
sourceHash: 61f5860dd329530331f742e8a62a542683aa0557a14170c87cc32656b2bf91c8
---

# クイックスタート: 実際の ComfyUI に対してサムネイルを編集する {#quickstart-edit-a-thumbnail-against-a-real-comfyui}

この手順は、vv が実際のセルフホストの ComfyUI と Comfy Cloud で動くことを示す。`task check` と `task test-e2e` は偽の ComfyUI しか動かさないので、プロバイダーの通信の形式、その状態、そのタイミングを捉えられない。

## 前提条件 {#prerequisites}

- feature ブランチから実行し ([development.md](../../docs/how-to/development.md))、所有者としてログインし、フレームにテキストが映るライブラリの動画を 1 つ持つ vv。
- vv のサーバーから届くセルフホストの ComfyUI と、クレジットのある Comfy Cloud の API キー。
- `vv:frame`、`vv:prompt`、`vv:mask`、`vv:reference`、`vv:output` というタイトルのノードを持ち、API 形式で書き出した画像編集のワークフロー ([contracts/workflow.md](contracts/workflow.md)) と、`vv:mask` と `vv:reference` のないその写し。

## 手順 {#steps}

表を、セルフホストの ComfyUI を保存した状態で 1 回、Comfy Cloud を保存した状態で 1 回実行する。

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| Settings: 接続を保存し、それからテストする | 「Connected」 | 1 |
| 誤った URL (セルフホスト) または誤ったキー (Cloud) を保存し、それからテストする | 原因を挙げる失敗 | 1 |
| 動画のページ、別のブラウザーのゲストのセッション | 編集の入口がない | 8 |
| ワークフローを削除し、動画のページを開く | 入口は無効で、Settings にワークフローが必要だと示す | 要件 13 |
| `vv:mask` のない写しを登録し、編集を開く | 塗りと参照画像が無効 | エッジケース |
| 完全なワークフローを登録し、テキストのあるフレームで一時停止し、編集を始め、「Remove the text」と入力し、実行する | 状態は経過時間付きで queued と running を経る (Cloud: preparing も)。結果がフレームの隣に現れる | 2、4 |
| テキストの上だけを塗り、再実行する | 編集は塗った範囲の中にとどまる | 3 |
| 参照画像を添え、再実行する | 結果は参照に従う | 3 |
| 実行し、待機中か実行中にキャンセルする | 実行は cancelled として終わる。サムネイルは変わらない | 6 |
| 実行し、タブを閉じ、5 分待ち、再読み込みする | 編集は適用されない。サムネイルは変わらない | エッジケース |
| 実行し、破棄する | サムネイルは変わらない | 6 |
| 実行し、適用する | 動画のページ、そのライブラリのカード、別の動画の関連動画にあるそのエントリが、動画の縦横比で新しい画像を示す | 5 |
| 再読み込みし、全体のスキャンを始める | 新しい画像は残る | 5 |
| 「Use the frame instead」 | サムネイルは再びフレームになる | 7 |
| 再び適用し、「use current frame」で新しい位置を選ぶ | サムネイルはそのフレームになる | 7 |
