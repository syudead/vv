# Quickstart: Edit a thumbnail against a real ComfyUI

These steps prove that vv works with a real self-hosted ComfyUI and with Comfy
Cloud; `task check` and `task test-e2e` drive a fake ComfyUI only, so they
cannot catch a provider's wire format, its states or its timing.

## Prerequisites

- vv running from the feature branch ([development.md](../../docs/how-to/development.md)),
  logged in as the owner, with one library video that shows text in a frame.
- A self-hosted ComfyUI reachable from the vv server, and a Comfy Cloud API
  key with credits.
- An image-edit workflow exported in API format, with nodes titled
  `vv:frame`, `vv:prompt`, `vv:mask`, `vv:reference` and `vv:output`
  ([contracts/workflow.md](contracts/workflow.md)), and a copy without
  `vv:mask` and `vv:reference`.

## Steps

Run the table once with the self-hosted ComfyUI saved and once with Comfy
Cloud saved.

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Settings: save the connection, then test it | "Connected" | 1 |
| Save a wrong URL (self-hosted) or a wrong key (Cloud), then test it | A failure that names the cause | 1 |
| Video page, guest session in another browser | No edit entry | 8 |
| Remove the workflow; open the video page | The entry is disabled and says Settings needs a workflow | Requirement 13 |
| Register the copy without `vv:mask`; open the edit | Painting and the reference image are disabled | Edge Case |
| Register the full workflow; pause on the frame with text; start an edit; type "Remove the text"; run | The state moves through queued and running (Cloud: also preparing) with the elapsed time; the result appears next to the frame | 2, 4 |
| Paint over the text only; run again | The edit stays inside the painted area | 3 |
| Attach a reference image; run again | The result follows the reference | 3 |
| Run, then cancel while queued or running | The run ends as cancelled; the thumbnail is unchanged | 6 |
| Run, then close the tab; wait 5 minutes; reload | No edit is applied; the thumbnail is unchanged | Edge Case |
| Run; discard | The thumbnail is unchanged | 6 |
| Run; apply | The video page, its library card and its entry in another video's related videos show the new image, at the video's aspect | 5 |
| Reload; start a full scan | The new image stays | 5 |
| "Use the frame instead" | The thumbnail is the frame again | 7 |
| Apply again; choose a new position with "use current frame" | The thumbnail is that frame | 7 |
