# Set up ComfyUI for thumbnail edits

When you finish, the connection test in Settings passes, and the video page can
send a frame to ComfyUI to edit it into a thumbnail. VVMDM brings the
workflow; you provide a ComfyUI that can run it, either your own or Comfy
Cloud. The design is in
[ai-layer.md, Image-edit provider](../design-docs/ai-layer.md#image-edit-provider).

## Prerequisites

- You are signed in to VVMDM as the owner.
- For your own ComfyUI: a ComfyUI recent enough to have the core nodes
  `Flux2Scheduler` and `GetImageSize`, on a machine VVMDM can reach, with an
  estimated 8 to 12 GB of VRAM (not measured) for the default model, or a
  24 GB GPU for Qwen-Image-Edit-2511. No custom nodes are needed.
- For Comfy Cloud: a Comfy account with credits. Each edit uses them.

## Choose the model

The "Model" choice in Settings' ComfyUI section decides which model edits the
frame. Each model needs its own files.

| Model | Choose it when |
| --- | --- |
| FLUX.2 [klein] 4B (default) | You mostly remove captions, logos or objects |
| Qwen-Image-Edit-2511 | You attach a reference image to take its colours and mood; your own ComfyUI needs a 24 GB GPU for it |

## Steps for your own ComfyUI

1. Download the model files of the model you chose into the folders under
   ComfyUI's `models` folder. For FLUX.2 [klein] 4B:

   | File | Folder under `models` | Size | Download page |
   | --- | --- | --- | --- |
   | `flux-2-klein-4b-fp8.safetensors` | `diffusion_models` | 4.07 GB | <https://huggingface.co/black-forest-labs/FLUX.2-klein-4b-fp8> |
   | `qwen_3_4b.safetensors` | `text_encoders` | 8.04 GB | <https://huggingface.co/Comfy-Org/z_image_turbo/tree/main/split_files/text_encoders> |
   | `flux2-vae.safetensors` | `vae` | 0.34 GB | <https://huggingface.co/Comfy-Org/flux2-dev/tree/main/split_files/vae> |

   For Qwen-Image-Edit-2511:

   | File | Folder under `models` | Size | Download page |
   | --- | --- | --- | --- |
   | `qwen_image_edit_2511_fp8mixed.safetensors` | `diffusion_models` | 20.53 GB | <https://huggingface.co/Comfy-Org/Qwen-Image-Edit_ComfyUI/tree/main/split_files/diffusion_models> |
   | `qwen_2.5_vl_7b_fp8_scaled.safetensors` | `text_encoders` | 9.38 GB | <https://huggingface.co/Comfy-Org/Qwen-Image_ComfyUI/tree/main/split_files/text_encoders> |
   | `qwen_image_vae.safetensors` | `vae` | 0.25 GB | <https://huggingface.co/Comfy-Org/Qwen-Image_ComfyUI/tree/main/split_files/vae> |
   | `Qwen-Image-Edit-2511-Lightning-4steps-V1.0-bf16.safetensors` | `loras` | 0.85 GB | <https://huggingface.co/lightx2v/Qwen-Image-Edit-2511-Lightning> |

2. Start ComfyUI so that it listens on an address VVMDM can reach. On another
   machine, start it with `--listen`:

   ```bash
   python main.py --listen 0.0.0.0 --port 8188
   ```

3. In VVMDM, open Settings, choose "My own ComfyUI" and the model in the
   ComfyUI section, enter the URL (for example `http://192.168.1.20:8188`),
   and select "Save and test connection".

## Steps for Comfy Cloud

1. Sign in to your Comfy account at <https://platform.comfy.org>, open
   "API Keys", and create a key. Copy it.
2. In VVMDM, open Settings, choose "Comfy Cloud" and the model in the ComfyUI
   section, paste the key, and select "Save and test connection". VVMDM never
   shows the key again; to change it, type a new one.

Comfy Cloud provides the model files and nodes for both models.

## Try a derived model

To try a fine-tune or merge of FLUX.2 [klein] 4B on your own ComfyUI, put its
`.safetensors` file in `models/diffusion_models` and start VVMDM with its file
name in `MDM_COMFYUI_DIFFUSION_MODEL`:

```bash
MDM_COMFYUI_DIFFUSION_MODEL=my-klein-4b-finetune.safetensors ./mdm
```

With FLUX.2 [klein] 4B chosen, VVMDM then loads that file in place of
`flux-2-klein-4b-fp8.safetensors` and keeps the text encoder, the VAE and the
4-step workflow, so the model must have the same architecture and be distilled
like the original. The connection test checks for that file, without a
download page. Comfy Cloud provides only its own models, so the variable is
for your own ComfyUI. The Windows desktop app does not read it, and the
Qwen-Image-Edit model ignores it.

## Check the result

The test answers "Connected to ComfyUI." or "Connected to Comfy Cloud.". If
something is missing, it lists each model file with its folder and download
page, and each missing node.

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| A model file is listed as missing | The file is not in the listed folder, or has another name | Put the file in the folder under the listed name and test again; if it is still listed, restart ComfyUI |
| A node is listed as missing | ComfyUI is older than the workflow's core nodes | Update ComfyUI and test again |
| The test says ComfyUI cannot be reached | The URL is wrong, or ComfyUI listens only on `127.0.0.1` of another machine | Check the URL, and start ComfyUI with `--listen` |
| The test says the API key was rejected | The Comfy Cloud key was deleted or mistyped | Create a new key and save it |
