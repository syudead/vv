# Quickstart: import pipelines on the Activity screen

These steps show, with real workers, media and a classifier, what the unit
tests check piece by piece: concurrent imports stay apart, follow-up work holds
the parent open, and a failure stays visible after a reload and a restart.

## Prerequisites

- A server from [Run locally](../../docs/how-to/development.md#run-locally)
  with an empty data directory and one media folder holding three short videos,
  two of them the same video re-encoded (a duplicate pair).
- An Ollama-compatible endpoint saved in Settings (Ollama, or the Workers AI
  relay in `scripts/clefrelay`), at least one tag, "Use auto-tagging" and
  "Tag new videos automatically" on, and duplicate judging on.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Scan the media folder and open Activity while it runs | One parent per video; each parent's members are its import run (with the five stages as its breakdown), its auto-tagging run and, for the two re-encodes, one duplicate judgement shown under both parents | 1 |
| Watch a parent whose ingest stages finish first | The parent stays running until its auto-tagging and judgement finish | 2 |
| Turn auto-tagging off, add a fourth video, scan again | The new parent finishes after its ingest stages; no auto-tagging member appears | 2 |
| Turn auto-tagging on and press "Suggest tags" on a finished video's page | A standalone auto-tagging entry; no earlier parent gains a member or reopens | 1 |
| With the endpoint stopped, add a fifth video and scan | The new parent needs attention and names the failed auto-tagging member, its ingest members stay done | 3 |
| Open that member's detail, read the full message, restart the endpoint and press Retry | The detail shows the failure and the retry under the same parent, and updates to done without a reload | 3, 6 |
| Filter Activity by "Needs attention" or search for the video's name before the retry | The parent is listed with the matching member marked | 5 |
| Delete one video, then open its parent and the detail of a tag entry you made | Stage rows, messages and probabilities are still readable; no Open action for the deleted video | 4 |
| Restart the server and reload the page | The same parents and members as before the restart | 6 |
| Repeat the list and a detail at 360 px width | Parent, members and separate runs are told apart and the full message wraps | 6 |
