# Quickstart: Measuring long lists at scale

These steps measure the acceptance criteria of #675 in headless Chromium
against the production build, which `task check` and `task test-e2e` do not:
jsdom has no layout, and the e2e data is a few videos.

## Prerequisites

- `task build` passes, and Playwright's Chromium for `web/` is installed, as
  for [the tag admin benchmark](../../docs/how-to/tags-admin-benchmark.md#prerequisites).
- `scripts/listsbench` exists ([R-8](research.md#r-8-a-list-benchmark-shares-the-tag-benchmarks-runner)).
  Run it twice, with `-videos 10000` and `-videos 30000`; both seeds have 3,000
  tags and one group of 6,000 videos.
- The viewport is 1280×800 and the session is the owner's.

## Steps

`web/bench/lists.bench.ts` runs each scene; a long frame is a frame over 50 ms
from `PerformanceObserver` (`long-animation-frame`), and "elements" is
`document.getElementsByTagName("*").length`.

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Library grid at the default zoom: scroll down until 900 or more cards have loaded | No two consecutive long frames | 1 |
| The same at the smallest zoom | No two consecutive long frames | 1 |
| The same in list view | No two consecutive long frames | 1 |
| Folder screen of the 3,000-video folder at the smallest zoom: scroll to 1,380 cards | No two consecutive long frames | 1 |
| Library: count elements, scroll until 1,000 cards have loaded, count again | The second count is within 10% of the first | 5 |
| Library: scroll to card 900, open its video, press Back | The card that was at the top is visible within 0.5 s of Back | 2 |
| Open a member of the 6,000-video group | The first member row is visible within 1 s, with no task over 200 ms meanwhile | 3 |
| In that member list, scroll to the end of the loaded members, then press the position | The current member's row is visible | 3 |
| Video page `Add tag`: focus the input | The suggestion list is visible within 0.2 s | 4 |
| Type one character at a time, five characters | Each keystroke's suggestions are drawn within 0.1 s | 4 |
| Selection bar `Add tag`: open it and type, as above | Same as the two rows above | 4 |
