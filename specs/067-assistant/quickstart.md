# Quickstart: the assistant with a real Ollama model

These steps prove that a real generation model reads the Japanese requests
of the acceptance criteria into the intended outcomes, which the e2e stub
cannot show because it answers fixed outcomes.

## Prerequisites

- A running VVMDM with a library of at least 30 videos, the tags
  `コスプレ衣装` and `コスプレ衣裳` and no tag named `コスプレ`, one person
  with credits, and some watched and unwatched videos
  ([running-vv.md](../../docs/how-to/running-vv.md)). The two names have
  6 characters at distance 1, so the `tags` finder lists them
  ([plan.md, Review runs](plan.md#review-runs)); a shorter pair, such as
  `自撮り` and `自撮`, is never a candidate.
- Ollama 0.5.0 or later reachable from the server, with a chat model pulled;
  structured outputs work with any chat model from that version on, and a
  CPU-only host waits minutes per turn with a model over 8B.
- Settings, Connections, Ollama: the endpoint saved, the generation model
  chosen from the list, and the connection check green.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Open the panel on the library and send `去年追加した30分以上の未視聴` | The library shows the chips for last year, 30 minutes and unwatched; the save button creates a smart folder with the three conditions | 1 |
| Select three videos and send `これにコスプレ衣装タグを付けて` | A card names 3 videos and the tag; the videos' tags are unchanged | 2 |
| Close the panel, reopen it, do nothing for a minute | The card is still there and nothing changed | 3 |
| Send `長いのは除いて` | The card is replaced by one with fewer videos, the ones over the panel's stated bound left out | 5 |
| Press `Execute` twice quickly | One run; the second press shows the run's state; the tag is on the videos; the Activity screen lists one `assistant` entry | 12, edge case |
| Send `コスプレ衣裳タグをコスプレ衣装に統合して` | The card carries the irreversible mark and the number of videos that move | 4 |
| Send `コスプレのタグを消して` while both tags exist | `コスプレ` matches no tag, so the thread asks which of the two near tags, and nothing is deleted | 6 |
| Send `〇〇の出演作は何本？` with a person's name | The number equals the count on the person's page | 7 |
| Send `タグを整理して` | The thread shows the progress; when done, the review page lists the similar pair with its probability; adopting it merges the tags | 8 |
| Send `APIトークンを作って` | The thread answers that the assistant cannot do this | 11 |
| Clear the generation model and open the panel | The guidance with the link to the Ollama section | 9 |
| Sign in as a guest | No trigger in the top bar or on the playback screen | 10 |
| Stop Ollama and send a request | The thread names the connection as the cause and nothing changed | Edge case |
