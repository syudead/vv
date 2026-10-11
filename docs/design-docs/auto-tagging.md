# Auto-tagging with a Clef classifier in Ollama

VVMDM asks a decision model running in Ollama (Cloudflare's Clef or
Clef-flash) which existing tags fit a video, and adds the tags whose
probability reaches a threshold. The use case is
[`internal/app/auto_tag.go`](../../internal/app/auto_tag.go), the queue is
[`internal/store/auto_tags.go`](../../internal/store/auto_tags.go), and the
Ollama client is [`internal/clef`](../../internal/clef).

The diagram shows how a video reaches the classifier and how its tags come
back.

```mermaid
flowchart LR
  seek[Seek preview stage done] -->|feature and automatic judging on| queue[(auto_tag_queue)]
  owner[Owner] -->|Settings or video page| queue
  queue --> worker[Auto-tagging worker]
  worker --> clef[Ollama /v1/systemone]
  clef -->|probability per tag| worker
  worker -->|tags at or above threshold| tags[(video_tags)]
  tags --> screen[Video notice to screens]
```

## Classifier request

Each existing tag becomes one yes/no (`noul`) question, and a video is sent as
its library thumbnail, 8 frames spread across the whole video, and a JSON
state with its title, file name and folder.

The thumbnail shows one scene, so a video whose scenes differ would only be
judged on that one; the frames let the classifier see the rest. They are cut
from the seek preview sheet the import already made, so no ffmpeg work is
added, and at 320 px or less each frame costs fewer input tokens than the
thumbnail.

| Input | Source | When missing |
| --- | --- | --- |
| Images | The library thumbnail (640 px), then 8 frames at the middle of 8 equal parts of the seek preview | The images that can be read; text only when none can |
| `title` | The display name, else the file title | Never missing |
| `file_name`, `folder` | The representative location's path | Never missing |

Clef scores every question in one non-autoregressive pass and returns a
probability per question. A request carries 16 tags, and a library with more
tags is sent in batches of 16 with the same images. The question asks whether
the tag appears in or applies to any part of the video, names its synonyms,
and says the images come from across the video; tentative tags are not asked
about.

The wording decides whether frames help. On a 30 s video of a cat, a rocket
and a cup of coffee, with the thumbnail showing the cat:

| Images and question | cat | coffee | rocket | Unrelated tags |
| --- | --- | --- | --- | --- |
| Thumbnail, "appears in any part" | 0.96 | 0.01 | 0.00 | 0.02 or less |
| Thumbnail and 8 frames, "describes this video" | 0.60 | 0.61 | 0.17 | 0.01 or less |
| Thumbnail and 8 frames, "appears in any part" | 0.93 | 0.85 | 0.70 | 0.00 |

Asking whether a tag describes the video makes the model weigh the whole
video, so a tag that fits one scene falls below the threshold.

Ollama accepts 64 questions per request, but 64 questions and a thumbnail make
about 7,400 input tokens, and Clef-flash on a 15 GB CPU machine was killed for
lack of memory. 16 questions make about 2,000 tokens with the thumbnail alone.
The images are processed again for every batch, so time per video grows with
the number of tags. On a 4-core CPU without a GPU, Clef-flash took about 100 s
per thumbnail with 9 tags; the published GPU latency is under 0.2 s.

| Rejected | Why |
| --- | --- |
| The thumbnail alone | It shows one scene, so tags for the rest of the video are missed |
| The whole 9 × 9 seek preview sheet as one image | Up to 2,880 px wide, it costs several times the tokens of 8 frames |

## Tags it adds

The probability decides how a tag is attached: at or above "Probability to
add" (0.8 by default) as an ordinary tag, between "Probability to suggest"
(0.5 by default) and that as a suggested (unconfirmed) tag, and below it not
at all. No tag is created.

```mermaid
flowchart LR
  p[Probability] --> add{0.8 or more?}
  add -->|yes| ordinary[Ordinary tag]
  add -->|no| suggest{0.5 or more?}
  suggest -->|yes| suggested[Suggested tag]
  suggest -->|no| none[Not added]
```

A suggested tag shows the tentative mark on cards and the video page, and the
video page offers keep and reject in place of remove. Keeping it, adding the
same tag by hand, or a later judgement at or above "Probability to add" turns
it into an ordinary tag. Rejecting it removes it and records the tag for that
video in `auto_tag_rejections`, so auto-tagging never adds it to that video
again; adding it by hand still works. Setting both probabilities to the same
value turns suggestions off.

The middle band exists because the classifier's probabilities near the
threshold are not reliable either way: in the trials, the one wrong tag and
one right tag (the rocket in a three-scene video, 0.70) both fell between 0.5
and 0.8. Suggested tags count in tag filters like ordinary ones, the same way
tentative tags do.

| Rejected | Why |
| --- | --- |
| Create tags from the model's suggestions | Clef only scores options it is given; it does not generate names |
| Mark the tag itself as tentative | Tentative is a state of a tag across all videos, and a suggestion is about one video |
| One threshold | Tags near it are either dropped or added unchecked |

## Queue and when a video is judged

Each video's content key has one row in `auto_tag_queue`, and the row stays
`done` after judging so that automatic judging never repeats on that video.

```mermaid
stateDiagram-v2
  [*] --> queued: added
  queued --> running: worker claims
  running --> done: answer recorded
  running --> failed: Ollama error
  running --> queued: restart
  failed --> queued: tag not yet tagged
  done --> queued: tag all again, or the video page
```

| Trigger | Queues |
| --- | --- |
| Seek preview stage finishes, "Tag new videos automatically" on | Videos with no row |
| Settings, "Tag videos not yet tagged" | Videos with no row or a `failed` row |
| Settings, "Tag all videos again" | Every video not `running` |
| Video page, "Suggest tags" | That video, unless `running` |

Every trigger requires "Use auto-tagging" to be on; while it is off the
Settings run buttons and the video page button are hidden, and the API refuses
to queue with `409 auto_tagging_off`.

Keeping `done` rows means a tag the owner removed is not added back by the
next import of the same content. One worker judges one video at a time, so a
slow CPU-only Ollama is not given parallel requests it cannot serve.

## Failures

A failed request marks the row `failed` with Ollama's message and the worker
moves on; nothing retries it until the owner queues it again.

Ollama being stopped makes every queued video fail within seconds instead of
blocking the queue, and the Settings page shows the count and the last
message. A row still `running` at shutdown goes back to `queued` at the next
startup.

## Settings

The Ollama connection and the auto-tagging feature are separate settings, each
with its own Settings section and API resource. The connection says where the
classifier runs; the feature says whether VVMDM uses it.

| Section | API | Holds |
| --- | --- | --- |
| Ollama | `/api/settings/ollama`, `/api/settings/ollama/check` | The Ollama URL and the model, and the connection test |
| Auto-tagging | `/api/settings/auto-tagging` | Whether the feature is used, automatic judging after import, the two probabilities, the queue counts |

Auto-tagging is optional and off by default. While "Use auto-tagging" is off:

- the worker claims no queued video and never contacts Ollama, and the queued
  rows wait until the feature is turned on again;
- nothing is queued, by import, Settings or the video page;
- the video page has no "Suggest tags" button;
- suggested tags already added can still be kept or rejected.

"Save and test connection" in the Ollama section saves the form and sends one
question to the saved URL; it works whether the feature is on or off, so the
connection can be tried before turning the feature on.

The server only sends requests to the saved URL, never to one in a request
body, and passes on only the error text of an Ollama JSON reply. Otherwise a
request could make the server fetch any reachable address and show the reply.

| Setting | Section | Default |
| --- | --- | --- |
| Ollama URL | Ollama | `http://127.0.0.1:11434` |
| Model | Ollama | `clef-flash` (`clef` is 27B and more accurate) |
| Use auto-tagging | Auto-tagging | Off |
| Tag new videos automatically | Auto-tagging | Off (has no effect while the feature is off) |
| Probability to add | Auto-tagging | 0.8 |
| Probability to suggest | Auto-tagging | 0.5 (the same value as "Probability to add" turns suggestions off) |

| Rejected | Why |
| --- | --- |
| One form for the connection and the feature switch | Testing or changing the connection would mean editing the feature's settings, and turning the feature off would look like losing the connection |
