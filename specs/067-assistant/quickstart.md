# Quickstart: the assistant with a real tool-calling model

These steps show that a real model combines the tools to meet the parent
Issue's acceptance criteria. The e2e stub cannot show this, because it
answers scripted tool calls.

## Prerequisites

- A running VVMDM with at least 30 videos, among them videos with cats in
  their frames. The library also needs two tags with near names (such as
  `コスプレ衣装` and `コスプレ衣裳`), one person with face candidates,
  watched and unwatched videos, and one approved work-info plugin
  ([running-vv.md](../../docs/how-to/running-vv.md#assistant)).
- A generation model whose `/api/show` lists `tools`, on a local Ollama or
  through `scripts/clefrelay`. The steps marked vision also need a model
  that lists `vision`, such as `@cf/meta/llama-3.2-11b-vision-instruct`
  through the relay. The decision model is Clef (`clef-flash`).
- Settings, Connections, Ollama: endpoint, decision model and generation
  model saved; the check is green.

## Steps

Each step names the parent Issue's acceptance criterion it shows. Record the
progress items and the answer or card for each step in the PR.

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Open a video and send `この動画の情報を取得して` | Progress shows the plugin search and fetch; a card lists the fields to apply; the video's work info is unchanged until `Execute` | 1 |
| Send `カラオケ関連の動画` | The answer gives a count equal to the library filtered by `カラオケ` | 2 |
| Open a video and send `この動画に合いそうなタグを付けて` | A card attaches existing tags; progress may show `clef` or `view_images` on its frames | 3 |
| Select three videos and send `選択した動画の共通点は？` | Each name in the answer is in all three videos' details | 4 |
| Select videos and send `ライブ録画タグを作って選択中の動画に付けて` | One card with two steps, the second marked `created by step 1`; `Execute` runs both | 5 |
| Untick one step, then `Execute` | The unticked step is `skipped` | 6 |
| Send `重複してるタグをまとめて` | Merge steps, each irreversible with the number of videos that move | 7 |
| Send `お気に入りの使い方を教えて` | How to favorite and find favorites; no card | 8 |
| Send `先月追加した30分以上の未視聴を再生リストにして` | Create-playlist and add-items steps with the count of the library's same filter | 9 |
| Send `Show me videos I haven't watched yet` | Unwatched videos, answered in English | 10 |
| Close the panel during a request and reopen it | The same turn with its progress or its result | 11 |
| After step 9, send `長いのは除いて` | The card is replaced, without the long videos | 12 |
| Send `コスプレのタグを付けて` with videos selected | A question naming both near tags; no card | 13 |
| Send `タグを整理して` | Progress, then the review list | 15 |
| Choose a model without `tools` and open the panel | The guidance and the settings link | 16 |
| Send `APIトークンを作って` | The reason and a link to Settings, Account | 18 |
| Send `猫が映ってる動画を探して` | Progress shows a search, frame reads and `clef` (or `view_images` with a vision model); the answer lists videos marked as the judge's | Clef and vision samples |
| Open a person and send `この人物の顔候補が本人か確かめて` | Progress shows the face candidates, their crops and `clef`; the answer gives each candidate's probability | Clef sample |
| Repeat the cat request with a vision model | Progress shows `view_images` | Vision sample |
| Stop Ollama and send a request | The turn stops with `connection`; nothing changed | Edge case |
