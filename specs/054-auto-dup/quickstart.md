# Quickstart: Judging duplicates with a real model

These steps prove what automated tests with a fake classifier cannot: that a
real Clef model separates real duplicates from look-alikes at the thresholds of
[R-3](research.md#r-3-the-question-wording-and-two-fixed-thresholds), and that
the four acceptance criteria hold end to end.

## Prerequisites

- VVMDM running from this branch ([running-vv.md](../../docs/how-to/running-vv.md)).
- A Clef model reachable as Ollama, set under Settings, Ollama, with model
  `clef`. Without Ollama, `go run ./scripts/clefrelay` relays to Workers AI
  (usage at the top of [`scripts/clefrelay/main.go`](../../scripts/clefrelay/main.go)).
- A media folder holding: a real video `A`; `B`, the same video at a lower
  resolution (`ffmpeg -i A.mp4 -vf scale=-2:360 B.mp4`); and `C`, a video of
  the same length whose first seconds are `A`'s and whose rest is other footage.
- Automatic duplicate judging turned on under Settings.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Scan, and wait until the import finishes | `A` and `B` show as one card with the versions mark; the video page lists 2 versions; the representative is `A` | 1 |
| Open Duplicates | `A` and `C` are not listed, and the video page of `C` shows no versions | 2 |
| On `A`'s video page, remove `B` from the versions, then scan again | `B` has its own card again; Duplicates lists `A` and `B`, and they stay listed after the scan | 3 |
| Turn the setting off, add `D`, a copy of `A` at another resolution, and scan | Duplicates lists `D` with `A`, and nothing is bundled | 4 |
| Stop the model, turn the setting on, add `E`, another copy, and scan | Duplicates lists `E`'s pair; after the model is started again and the retry time has passed, `E` joins `A`'s versions | Edge Case: AI unreachable |

Record the probability of each pair from the server log in the PR. When the
same pair falls between 0.2 and 0.8 on real files, the thresholds go back to
the Plan before the unit merges.
