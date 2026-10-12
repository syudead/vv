# Research: Edit a video frame with ComfyUI and use it as the thumbnail

Parent Issue: #942. Inherited decisions:

| Topic | Source of truth |
| --- | --- |
| Boundaries, user data versus rebuildable files, the authentication boundary | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| The AI layer: a port in `internal/app`, a provider adapter wired in `cmd/mdm`, connection settings apart from features, no request to a URL from a request body | [ai-layer.md](../../docs/design-docs/ai-layer.md) |
| The thumbnail position override, the generation lock, the `thumbnailUrl` revision | [029 research, R-4](../029-video-overrides/research.md#r-4-a-thumbnail-position-is-generated-within-the-request-then-recorded), [R-5](../029-video-overrides/research.md#r-5-the-import-thumbnail-job-also-uses-the-chosen-position-the-automatic-rule-stays-in-internalmedia), [R-6](../029-video-overrides/research.md#r-6-the-thumbnailurl-version-includes-the-position-revision) |
| The error shape and reason codes | [023 error-api](../023-english-i18n/contracts/error-api.md) |
| Adding a dependency | [dependency-updates.md](../../docs/how-to/dependency-updates.md) |

The ComfyUI facts below come from the Issue's POC (2026-10-12, Comfy Cloud),
the Comfy Cloud API reference (`docs.comfy.org`) and ComfyUI's server routes.
This file records only the decisions this feature adds.

## R-1: An image-edit port in `internal/app`, one `internal/comfyui` adapter for both providers

**Decision**: `internal/app` declares an `ImageEditor` port (upload the
inputs, start a run, read its state, cancel it, fetch the output), and the new
adapter package `internal/comfyui` implements it for both a self-hosted
ComfyUI and Comfy Cloud. `app.ComfyUIConnection` saves the connection and runs
the connection test, apart from the thumbnail feature, as
`app.OllamaConnection` does.

| Option | Verdict |
| --- | --- |
| **New port and one adapter with a provider switch** | Chosen |
| Reuse `app.Classifier` | Rejected: that port answers yes/no questions with probabilities; an image edit returns an image and runs for tens of seconds with states |
| Two adapter packages, one per provider | Rejected: the routes, the workflow format and the output fetch are the same; the differences fit in one table ([R-5](#r-5-provider-differences-stay-inside-the-adapter)) |
| Call ComfyUI from `internal/httpapi` | Rejected: adapters do not import each other, and the session rules (one per video, expiry, apply) are a use case |

**Rationale**: Keeping the provider behind a port follows ai-layer.md, so the
thumbnail feature does not know which provider runs, and a third provider
later is a new adapter.

## R-2: Named nodes in the owner's API-format workflow are the whole contract

**Decision**: The owner registers one workflow exported in ComfyUI's API
format. vv finds its inputs and output by node title (`_meta.title`):
`vv:frame` and `vv:output` are required; `vv:prompt`, `vv:mask` and
`vv:reference` are optional, and each one present enables that input on the
video page ([contracts/workflow.md](contracts/workflow.md)). vv rewrites only
those nodes' inputs and never edits another node.

| Option | Verdict |
| --- | --- |
| **Titles `vv:*` on the owner's nodes** | Chosen: the POC ran this way |
| Node ids entered in Settings | Rejected: ids change when the owner rebuilds the workflow, and the owner has to read the JSON to find them |
| Find inputs by class (`LoadImage`, `CLIPTextEncode`) | Rejected: a workflow with two `LoadImage` nodes is ambiguous, and requirement 3 rules out assuming nodes |
| A bundled default workflow | Rejected: the Issue's `対象外` excludes shipping workflows and models |
| vv randomizes the sampler's seed on each run | Rejected: it assumes a sampler node; the owner changes the prompt to get a different result (requirement 9), and a workflow that wants a fresh seed uses a random-seed node |

**Rationale**: Titles are visible and editable in ComfyUI's editor and survive
an API-format export, so the owner prepares the workflow where it runs.
Validation at registration answers the Edge Case "an input the workflow cannot
take" before any run: the page disables a control whose node is absent.

## R-3: Absent optional inputs get a neutral value

**Decision**: When the workflow has `vv:mask` and the owner painted nothing,
vv sends a mask covering the whole frame; when it has `vv:reference` and the
owner attached nothing, vv sends the frame; when it has `vv:prompt` and the
owner typed nothing, vv sends an empty string.

| Option | Verdict |
| --- | --- |
| **Neutral values** | Chosen |
| An empty (all-black) mask | Rejected: an inpainting workflow then repaints nothing, so a text-only edit returns the frame unchanged |
| Remove the unused node before submitting | Rejected: the nodes downstream of it lose their input and the run fails validation |
| Ask for one workflow per input combination | Rejected: requirement 3 asks for one registered workflow, and the owner would keep up to eight |

**Rationale**: "Edit the whole frame" and "the frame is its own reference" are
what an edit without that input means, so one workflow serves every
combination requirement 5 allows.

## R-4: An edit is an in-memory session per video, read by polling

**Decision**: `app.ThumbnailEdits` holds one session per video in memory: the
frame, the snapshot of the connection and workflow taken when a run starts,
the run's state and the result image. The page reads the session every second
while a run is in progress and every 60 seconds while a result waits. A session
not read for 5 minutes is cancelled and dropped, and a restart drops all of
them ([data-model.md, Edit session](data-model.md#edit-session)).

| Option | Verdict |
| --- | --- |
| **In-memory session, polling** | Chosen |
| A job kind in the persistent queue | Rejected: a run belongs to an open page and must not be applied after a restart or after the owner leaves (Edge Cases); the queue's retry and `failed` records would show it in the import issues |
| Progress through `/api/events` | Rejected: that stream carries domain events published after a store commit to every viewer; a run's state is one page's and is never committed |
| Generate within one request, as the position override does | Rejected: a run takes 10–20 s and its preparation tens of seconds, and requirements 7 and 8 need the state and a cancel while it runs |

**Rationale**: Reads double as the page's heartbeat, so closing the browser
ends the session within 5 minutes without a separate signal (Edge Case "leave
the page or close the browser"). Images stay in memory, so nothing is left on
disk after a crash.

## R-5: Provider differences stay inside the adapter

**Decision**: The adapter maps each provider's routes and states to one state
set (`queued`, `preparing`, `running`, then `succeeded`, `failed` or
`cancelled`), and takes step progress from the ComfyUI WebSocket where it
connects ([contracts/workflow.md, Run states](contracts/workflow.md#run-states)).

| Concern | Self-hosted ComfyUI | Comfy Cloud |
| --- | --- | --- |
| Base | The saved URL | `https://cloud.comfy.org/api` |
| Authentication | None | `X-API-Key` header; WebSocket `token` query parameter |
| State | `GET /queue` (pending or running), then `GET /history/{id}` | `GET /job/{id}/status` |
| Output | `GET /view` | `GET /view` answers a redirect to a signed URL, fetched without the key |
| Cancel | `POST /queue` `{"delete":[id]}` while queued; `POST /interrupt` `{"prompt_id":id}` while running | `POST /queue` `{"delete":[id]}` while queued; `POST /interrupt` while running |
| Connection test | `GET /system_stats` | `GET /jobs?limit=1`, which needs a valid key |

Comfy Cloud documents two state vocabularies: the API reference and the live
service answer `submitted`, `queued_waiting`, `preparing`, `executing`,
`success`, `error`, `non_retryable_error`, `lost`, `cancel_requested` and
`cancelled`, while the published OpenAPI schema lists `waiting_to_dispatch`,
`pending`, `in_progress` and `completed`. The adapter maps both
([contracts/workflow.md, Run states](contracts/workflow.md#run-states)), and a
value in neither keeps the run's previous state until a known one arrives or
the time limit ends it. Cloud's `POST /interrupt` stops every running job of
the account, not one; vv sends it only when the run's own state is `running`,
and the account's other running jobs are the owner's to restart. A self-hosted ComfyUI has no `preparing` state, which
requirement 7 allows.

| Option | Verdict |
| --- | --- |
| **HTTP polling for the state, WebSocket for step progress only** | Chosen |
| WebSocket for everything | Rejected: the POC could not reach Comfy Cloud's WebSocket, so the state must not depend on it |
| HTTP only, no step progress | Rejected: requirement 7 asks for finer progress when the provider reports it, and ComfyUI reports steps only on the WebSocket |

**Rationale**: A WebSocket that fails to connect leaves the progress absent,
never fails the run. The Go dependency for it is `github.com/coder/websocket`,
added under dependency-updates.md; it has no transitive dependencies.

## R-6: The applied image is user data in SQLite; the thumbnail file is rebuilt from it

**Decision**: Applying stores the fitted JPEG in a new user-data table
`thumbnail_images` keyed by content key, and publishes the same bytes as the
thumbnail file through `internal/artifacts`. `Ingest.Thumbnail` publishes the
stored image instead of extracting a frame when a row exists
([data-model.md](data-model.md)).

| Option | Verdict |
| --- | --- |
| **A blob table, republished by the thumbnail job** | Chosen |
| Only the thumbnail file in `MDM_DATA_DIR/thumbnails` | Rejected: `RemoveContent` deletes it when the last video with the content goes, and the job would extract a frame when the content returns (requirement 11) |
| A second file area for user images under `MDM_DATA_DIR` | Rejected: backup and recovery cover SQLite and treat the data directory's files as rebuildable ([running-vv.md, Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery)); a new area changes that rule |

**Rationale**: A fitted image is 50–150 KB, so one row per edited video keeps
SQLite small, and the image is backed up with the rest of the owner's data.
`internal/artifacts` stays the only owner of thumbnail files, which remain
rebuildable.

## R-7: The generated image is fitted to the video's frame before it is stored

**Decision**: `internal/media` scales and centre-crops the result to the
video's display aspect (from `Video.Width`, `Video.Height`) with the long side
640 px, as a JPEG with the same quality as an extracted thumbnail. When the
video's size is unknown, the image keeps its own aspect.

| Option | Verdict |
| --- | --- |
| **Fit to the video's aspect** | Chosen |
| Keep the result's size and aspect | Rejected: cards place the card scrub preview and the narrow-video backdrop on the video's aspect (`isNarrowVideo`, `ScrubFrame`), so a different aspect shifts them against the thumbnail |
| Letterbox to the aspect | Rejected: the bars show inside the card's `object-contain` frame as a second border |

**Rationale**: The Edge Case "a different size or aspect does not break the
display" holds on every screen without changing any of them, and the workflow
may output any size.

## R-8: The frame is extracted by the server at up to 2048 px

**Decision**: Starting an edit extracts the frame at the given position with
`ffmpeg` as a PNG whose long side is the video's, capped at 2048 px. The page
shows that frame for painting, and the mask is a PNG of the same pixel size.

| Option | Verdict |
| --- | --- |
| **Server extraction at the source size, capped** | Chosen |
| Capture the `<video>` element on a canvas | Rejected: a live-transcoded stream is a lower-quality encode of the file, and the image differs from what a later position override extracts |
| The 640 px thumbnail extraction | Rejected: an edit model loses the text it is asked to remove at that size |

**Rationale**: 2048 px keeps a 4K frame's PNG under about 8 MB for the
upload; the owner's workflow scales the input to its model's size, as the
POC's `ImageScaleToTotalPixels` did.

## R-9: The API key is write-only

**Decision**: The key is saved in `settings` as `comfyui.api_key`. Reads return
`apiKeySet` and never the key; a save without `apiKey` keeps it, and an empty
string clears it.

| Option | Verdict |
| --- | --- |
| **Plain value, never returned** | Chosen |
| Encrypt it in SQLite | Rejected: the encryption key would live in the same data directory, so it protects nothing a reader of the database cannot undo |
| Hash it, as API tokens are | Rejected: vv sends the key to Comfy Cloud, so it must be readable by the server |

**Rationale**: Requirement 2 is about the screen; the response never carries
the key, so neither the page nor a cached response can show it.

## R-10: Apply, revert and the position override are ordered by the generation lock

**Decision**: Applying, reverting and setting a position each run inside the
content's thumbnail generation lock, so the one confirmed last wins. Applying
checks that the video still has the content key the session started with,
setting a position deletes the `thumbnail_images` row, and reverting deletes it
and rebuilds the thumbnail from the recorded position or the automatic rule.

| Option | Verdict |
| --- | --- |
| **One lock for all three** | Chosen |
| Cancel the edit session when a position is set | Rejected: the owner may still apply afterwards, and applying is the later confirmed operation |
| Keep the position column empty while an image is applied | Rejected: reverting would lose the position the owner chose before the edit |

**Rationale**: This is the Edge Case "the last confirmed operation wins" with
the mechanism 029 already uses ([029 R-4](../029-video-overrides/research.md#r-4-a-thumbnail-position-is-generated-within-the-request-then-recorded)).
A moved or replaced file changes the content key, so the apply check answers
the Edge Case "the video is gone or now holds other content".

## R-11: A new session replaces the previous one; a run is limited to 10 minutes

**Decision**: Starting an edit for a video that has a session cancels its run
and drops it; starting a run in a session cancels its running run. A run with
no terminal state 10 minutes after it was submitted is cancelled and fails as
`timed_out`.

| Option | Verdict |
| --- | --- |
| **Replace and limit** | Chosen |
| Refuse a second edit while one runs | Rejected: a closed tab would block the video for up to 5 minutes |
| No time limit | Rejected: the Edge Case "a run that never ends" has to end in a failure the owner can read |

**Rationale**: Ten minutes covers the POC's tens of seconds of preparation
many times over, and a queued Comfy Cloud job past it is cancelled so it does
not spend credits after the owner has gone.
