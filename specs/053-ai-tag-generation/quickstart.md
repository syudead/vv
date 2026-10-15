# Quickstart: Tag creation with a real model

These steps prove that the default generation model creates usable tags from a
real video in acceptable time; the automated checks use a fake generator and
cannot.

## Prerequisites

- A build of the feature branch, set up as in
  [running-vv.md](../../docs/how-to/running-vv.md).
- Ollama with the judging model and `gemma4:e4b`, or, in a container without
  Ollama, `go run ./scripts/clefrelay` with its generation relay
  ([R-2](research.md#r-2-the-ollama-provider-uses-apichat-with-a-vision-model)).
- A media folder with three short videos of clearly different subjects, one of
  them a cat.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Settings, Ollama: "Save and test connection" | Available; with `gemma4:e4b` removed from Ollama, unavailable and the message names it | — |
| Settings, Auto-tagging: turn on "Use auto-tagging" and "Tag new videos automatically"; leave "Create new tags" on. Scan the empty library | Each video gets tentative tags with kinds; the tag screen's tentative filter lists them. Note the seconds per video in the log | 1 |
| Create tag `猫`, open the cat video, "Suggest tags" | `猫` is on the video and no tag such as `ねこ` or `ネコ` is created | 2 |
| Reject one created tag on the tag screen, then "Suggest tags" on its video | No tag of that name comes back | 3 |
| Turn off "Create new tags", add a fourth video | No new tag is created; existing tags are judged | 4 |
| Stop Ollama, add a fifth video, start Ollama | The video's row is `failed`, then judged after about 10 minutes without any action | Edge Case |
