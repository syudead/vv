# Implementation Plan: Control bar before playback

**Branch**: `claude/nice-hamilton-cvlyy9` | **Feature directory**: `specs/069-control-bar-before-play` | **Parent Issue**: #1762

**Input**: The parent Issue. It is this feature's specification.

## Summary

The video page shows the control bar while the poster and the central play
button are on screen, so a viewer can change the volume, read the duration,
seek, pick a quality, subtitle or speed, and switch theater mode before the
first play; those choices carry into playback because they already act on the
same player. The change is one stylesheet rule that overrides video.js's hide
before `vjs-has-started` ([R-1](research.md#r-1-a-stylesheet-rule-shows-the-bar-before-vjs-has-started)),
a row in the control bar's design document, and one browser test.

Artifacts: `research.md` holds R-1. There is no `data-model.md` (no entity
changes), no `contracts/` (no API changes) and no `quickstart.md` (the browser
test and the visual review in
[Implementation Work](#implementation-work) cover what a person would check).

## Constitution Check

Checked [ARCHITECTURE.md](../../ARCHITECTURE.md) (the change stays in the
browser SPA; the server and the API are untouched) and the
[AGENTS.md](../../AGENTS.md) rules on complexity, test levels and documents
beside the code; none is at stake, since the change adds no function and
documents the rule in [library-ui.md](../../docs/design-docs/library-ui.md).

## Implementation Work

### Show the player's control bar before playback starts

**Scope**: The stylesheet rule of
[R-1](research.md#r-1-a-stylesheet-rule-shows-the-bar-before-vjs-has-started)
in [`web/src/index.css`](../../web/src/index.css), next to the `data-loading`
rule it yields to. A `Before playback` row in the
[Control bar](../../docs/design-docs/library-ui.md#control-bar) table: the bar
shows over the poster once metadata has loaded, does not fade while paused,
and the choices made on it carry into playback. The comment on the
`player.hasStarted(true)` calls in
[`VideoPlayer.tsx`](../../web/src/player/VideoPlayer.tsx) is rewritten to say
what they still do (hide the poster under the failure and reconnect overlays),
because the bar no longer depends on them. The `data-loading` hide and the
central play button stay as they are (parent Issue, requirement 5 and `対象外`).

**Dependencies**: None

**Acceptance**:

- A browser test in [`playback.e2e.ts`](../../web/e2e/playback.e2e.ts) opens a
  direct-route video without autoplay and, before pressing play, sees the
  control bar and a non-empty duration display (criterion 1); clicks the mute
  button, then plays, and the `video` element is muted (criterion 2); clicks
  the theater button and the page enters theater mode (criterion 3, at the
  default `lg` viewport); clicks the seek bar at half its width, then plays,
  and playback starts within one second of that position (criterion 4); and
  waits for the `vjs-user-inactive` class without moving the mouse and still
  sees the bar (criterion 5). Browser level, because jsdom computes no
  stylesheet and the component tests fake video.js; the test stays with the
  Issue's criteria, which only Chromium and the real player show. Run as
  `task test-e2e -- e2e/playback.e2e.ts` locally or through the CI workflow's
  `Browser E2E` job on the PR branch.
- The existing unit test `読み込みの前に最後の失敗が起きても、操作バーを隠したままにしない`
  in [`VideoPlayer.test.tsx`](../../web/src/player/VideoPlayer.test.tsx)
  still passes: the `data-loading` hide is unchanged.
- `task check SCOPE=web WEB_FILES="src/player src/index.css" WEB_TESTS="src/player"`
  and `task check-docs` pass.
- The unit changes a screen, so it owes a visual review of the edge cases the
  parent Issue lists: toggling theater mode and full screen before play keeps
  the poster, the central play button and the bar in place; on a touch device
  a tap on the poster starts playback and a tap on the bar operates the bar;
  a video that fails to play shows the failure layer as today. The PR body
  records the widths checked (360px, 768px, 1280px) and the result.
