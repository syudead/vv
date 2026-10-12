---
source: docs/how-to/comfyui-thumbnails.md
sourceHash: 85a082b60966fd2c7d72d1577bf438ba1d6d91320ce8225dfe35eaa9c789898e
---

# サムネイルの編集のために ComfyUI を準備する {#set-up-comfyui-for-thumbnail-edits}

終えると、Settings の接続テストが通り、動画ページはフレームを ComfyUI に送ってサムネイルに編集できる。ワークフローは VVMDM が用意する。あなたは、それを実行できる ComfyUI を、自分のものか Comfy Cloud のどちらかで用意する。設計は [ai-layer.md の画像編集のプロバイダ](../design-docs/ai-layer.md#image-edit-provider) にある。

## 前提条件 {#prerequisites}

- VVMDM に所有者としてサインインしている。
- 自分の ComfyUI の場合: コアノード `Flux2Scheduler` と `GetImageSize` を持つほど新しい ComfyUI が、VVMDM から到達できるマシン上にあり、VRAM が推定 8〜12 GB (計測していない) ある。カスタムノードは不要だ。
- Comfy Cloud の場合: クレジットのある Comfy アカウント。編集のたびにクレジットを使う。

## 自分の ComfyUI の手順 {#steps-for-your-own-comfyui}

1. 3 つのモデルファイルを、ComfyUI の `models` フォルダ以下のフォルダにダウンロードする。

   | ファイル | `models` 以下のフォルダ | サイズ | ダウンロードページ |
   | --- | --- | --- | --- |
   | `flux-2-klein-4b-fp8.safetensors` | `diffusion_models` | 4.07 GB | <https://huggingface.co/black-forest-labs/FLUX.2-klein-4b-fp8> |
   | `qwen_3_4b.safetensors` | `text_encoders` | 8.04 GB | <https://huggingface.co/Comfy-Org/z_image_turbo/tree/main/split_files/text_encoders> |
   | `flux2-vae.safetensors` | `vae` | 0.34 GB | <https://huggingface.co/Comfy-Org/flux2-dev/tree/main/split_files/vae> |

2. VVMDM から到達できるアドレスで待ち受けるように ComfyUI を起動する。別のマシンでは `--listen` を付けて起動する。

   ```bash
   python main.py --listen 0.0.0.0 --port 8188
   ```

3. VVMDM で Settings を開き、ComfyUI のセクションで "My own ComfyUI" を選び、URL (例: `http://192.168.1.20:8188`) を入力して "Save and test connection" を選ぶ。

## Comfy Cloud の手順 {#steps-for-comfy-cloud}

1. <https://platform.comfy.org> で Comfy アカウントにサインインし、"API Keys" を開いてキーを作成する。キーをコピーする。
2. VVMDM で Settings を開き、ComfyUI のセクションで "Comfy Cloud" を選び、キーを貼り付けて "Save and test connection" を選ぶ。VVMDM はキーを二度と表示しない。変えるには新しいキーを入力する。

モデルファイルとノードは Comfy Cloud が用意する。

## 結果を確認する {#check-the-result}

テストは "Connected to ComfyUI." または "Connected to Comfy Cloud." と答える。何かが欠けていれば、各モデルファイルをそのフォルダとダウンロードページとともに並べ、欠けている各ノードを並べる。

## トラブルシューティング {#troubleshooting}

| 症状 | 原因 | 対処 |
| --- | --- | --- |
| モデルファイルが欠けていると表示される | ファイルが表示されたフォルダにない、または別の名前になっている | 表示された名前でファイルをそのフォルダに置き、再度テストする。まだ表示されるなら ComfyUI を再起動する |
| ノードが欠けていると表示される | ComfyUI がワークフローのコアノードより古い | ComfyUI を更新し、再度テストする |
| ComfyUI に到達できないとテストが言う | URL が誤っている、または ComfyUI が別のマシンの `127.0.0.1` だけで待ち受けている | URL を確認し、`--listen` を付けて ComfyUI を起動する |
| API キーが拒否されたとテストが言う | Comfy Cloud のキーが削除された、または入力を誤った | 新しいキーを作成して保存する |
