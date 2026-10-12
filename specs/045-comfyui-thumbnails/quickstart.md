# Quickstart: Edit a thumbnail against a real ComfyUI

These steps prove that vv works with a real self-hosted ComfyUI and with Comfy
Cloud; `task check` and `task test-e2e` drive a fake ComfyUI only, so they
cannot catch a provider's wire format, its states, its timing or the model's
output.

## Prerequisites

- vv running from the feature branch ([development.md](../../docs/how-to/development.md)),
  logged in as the owner, with one library video whose frames show burned-in
  captions and a logo in a corner.
- A self-hosted ComfyUI reachable from the vv server with the files of
  [contracts/workflow.md, Required files](contracts/workflow.md#required-files),
  a second self-hosted ComfyUI (or the same one with `flux2-vae.safetensors`
  moved away) that lacks one of them, and a Comfy Cloud API key with credits.

## Steps

Run the table once with the self-hosted ComfyUI saved and once with Comfy
Cloud saved.

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Settings: save the connection, then test it | "Connected" | 1 |
| Save a wrong URL (self-hosted) or a wrong key (Cloud), then test it | A failure that names the cause | 1 |
| Self-hosted only: save the ComfyUI that lacks a file, then test it | The missing file with its folder and download page | 2 |
| Video page, guest session in another browser | No edit entry | 9 |
| Remove the connection; open the video page | The entry is disabled and says Settings needs a connection | Requirement 14 |
| Pause on a frame with captions; start an edit; type "Remove the captions"; run | The state moves through queued and running (Cloud: also preparing) with the elapsed time; the result appears next to the frame without the captions | 3, 5 |
| Paint over the corner logo only; leave the instruction empty; run | The logo is gone and nothing outside the painted area changed; this settles the default instruction ([research.md R-3](research.md#r-3-absent-inputs-get-a-neutral-value-or-the-variant-without-the-node)) | 4, Edge Case |
| Attach a reference image; type "Use the colours of the reference"; run | The result takes the reference's colours | 4 |
| Run, then cancel while queued or running | The run ends as cancelled; the thumbnail is unchanged | 7 |
| Run, then close the tab; wait 5 minutes; reload | No edit is applied; the thumbnail is unchanged | Edge Case |
| Run; discard | The thumbnail is unchanged | 7 |
| Run; apply | The video page, its library card and its entry in another video's related videos show the new image, at the video's aspect | 6 |
| Reload; start a full scan | The new image stays | 6 |
| "Use the frame instead" | The thumbnail is the frame again | 8 |
| Apply again; choose a new position with "use current frame" | The thumbnail is that frame | 8 |
