# Quickstart: the hidden tags of a 30-tag card at two widths

These steps prove, by looking, that the list behind `+N` is readable and
reachable on a real screen at the two widths the parent Issue names;
`task test-e2e` checks the same behaviour in a headless browser but not how
the list looks against `ui-design.md`.

## Prerequisites

- A running server signed in as the owner, with a library of at least 12
  videos so that one card sits in the last row at 1280×800.
- One video with 30 hand-added tags, one of them 60 characters long. From a
  shell with an API token ([Create a token](../../docs/how-to/external-api.md#create-a-token)),
  `POST /api/v1/video-tags` with `action: "add"` adds them in one call
  ([Tag videos](../../docs/how-to/external-api.md#tag-videos)); the tag
  input on the video page adds them one by one.
- A browser window at 1280×800 with a mouse, and one at 390×844 (the device
  toolbar of the browser's developer tools, with touch emulation).

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| At 1280×800, open the library and rest the mouse on the card's `+N` | After a short rest the list opens without a click; `+N` shows its hover state from `ui-design.md` | 2 |
| Move the mouse across `+N` and off the card in one motion | The list does not open | 3 |
| Rest on `+N` again, then move the pointer down into the list | The list stays open while the pointer crosses the gap and while it is on the list | 2 |
| Look at the list | The chips wrap into several per line, in the card row's chip size and spacing; the list lies inside the window; a scroll bar appears only when the chips do not fit | 1, 4 |
| Find the 60-character tag | Its full name is shown, wrapped inside its chip, with no `…` | 5 |
| Scroll the list to its end and press the last tag | The library filters by that tag, the chip appears above the list, and the list closes | 1, 6 |
| Clear the filter, scroll so the card is in the last row near the window's bottom edge, open `+N` | The list opens above or beside `+N` and lies inside the window; every tag is reachable | Edge case |
| Move the mouse off the list | The list closes | Edge case |
| At 390×844, tap `+N` | The list opens, lies inside the screen, and every chip is reachable by scrolling the list | 1, 4 |
| Read the 60-character tag at 390×844 | Its full name is shown without a mouse | 5 |
| Tap a tag in the list | The library filters by it and the list closes | 6 |
| Open a folder that holds the video and repeat the tap | The library opens filtered by that tag (`/?tag=<id>`) | 6 |
| Back at 1280×800 with the keyboard: `Tab` to `+N`, `Enter`, `Tab`, `Enter` | The list opens on `Enter`, focus reaches its first chip, and `Enter` filters by it | 4 |
| Open the list, then press the card's checkbox on another card | Selection starts; `+N` becomes a plain chip and the list is gone | Out of scope confirmed |
