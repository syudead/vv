# Quickstart: Judging duplicates with a real model

These steps prove what automated tests with a fake classifier cannot: that on
real files the frame rule of
[R-1](research.md#r-1-the-fingerprints-decide-first-the-model-is-asked-only-when-a-section-differs)
bundles re-encodes by itself, that the default Clef model dismisses a copy
with a replaced section at the thresholds of
[R-3](research.md#r-3-the-question-wording-and-two-fixed-thresholds), and that
the four acceptance criteria hold end to end.

## Prerequisites

- VVMDM running from this branch ([running-vv.md](../../docs/how-to/running-vv.md)).
- A Clef model reachable as Ollama, set under Settings, Ollama, with the
  default model `clef-flash`. Without Ollama, `go run ./scripts/clefrelay`
  relays to Workers AI (usage at the top of
  [`scripts/clefrelay/main.go`](../../scripts/clefrelay/main.go)).
- A media folder holding a real video `A` of a few minutes, and:
  - `B`, `A` at a lower resolution (`ffmpeg -i A.mp4 -vf scale=-2:360 B.mp4`);
  - `C`, `A` with about a tenth of its middle replaced by other footage of
    the same length, so that its median frame distance still makes it a
    candidate.
- Automatic duplicate judging turned on under Settings.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Scan, and wait until the import finishes | `A` and `B` show as one card with the versions mark; the video page lists 2 versions; the representative is `A`; the log shows the frame rule decided with no request | 1 |
| Open Duplicates | `A` and `C` are not listed; the log shows one request for `C` with a probability of 0.2 or less, and `C` has its own card | 2 |
| On `A`'s video page, remove `B` from the versions, then scan again | `B` has its own card again; Duplicates lists `A` and `B`, and they stay listed after the scan | 3 |
| Turn the setting off, add `D`, another copy of `A` at another resolution, and scan | Duplicates lists `D` with `A`, and nothing is bundled | 4 |
| Stop the model, turn the setting on, add `E`, made like `C` with another section replaced, and scan | `D` joins `A`'s versions with no request; Duplicates keeps `E`'s pair; after the model is started again and the retry time has passed, the pair leaves Duplicates | Edge Case: AI unreachable |

Record each pair's rule outcome and probability from the server log in the
PR. When a pair the rule sent to the model falls between 0.2 and 0.8, or a
real re-encode is sent to the model at all, the rule or the thresholds go back
to the Plan before the unit merges.
