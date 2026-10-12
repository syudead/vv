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
  estimated 8 to 12 GB of VRAM (not measured). No custom nodes are needed.
- For Comfy Cloud: a Comfy account with credits. Each edit uses them.

## Steps for your own ComfyUI

1. Download the three model files into the folders under ComfyUI's `models`
   folder.

   | File | Folder under `models` | Size | Download page |
   | --- | --- | --- | --- |
   | `flux-2-klein-4b-fp8.safetensors` | `diffusion_models` | 4.07 GB | <https://huggingface.co/black-forest-labs/FLUX.2-klein-4b-fp8> |
   | `qwen_3_4b.safetensors` | `text_encoders` | 8.04 GB | <https://huggingface.co/Comfy-Org/z_image_turbo/tree/main/split_files/text_encoders> |
   | `flux2-vae.safetensors` | `vae` | 0.34 GB | <https://huggingface.co/Comfy-Org/flux2-dev/tree/main/split_files/vae> |

2. Start ComfyUI so that it listens on an address VVMDM can reach. On another
   machine, start it with `--listen`:

   ```bash
   python main.py --listen 0.0.0.0 --port 8188
   ```

3. In VVMDM, open Settings, choose "My own ComfyUI" in the ComfyUI section,
   enter the URL (for example `http://192.168.1.20:8188`), and select "Save and
   test connection".

## Steps for Comfy Cloud

1. Sign in to your Comfy account at <https://platform.comfy.org>, open
   "API Keys", and create a key. Copy it.
2. In VVMDM, open Settings, choose "Comfy Cloud" in the ComfyUI section, paste
   the key, and select "Save and test connection". VVMDM never shows the key
   again; to change it, type a new one.

Comfy Cloud provides the model files and nodes.

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
