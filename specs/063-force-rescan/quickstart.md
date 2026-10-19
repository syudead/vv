# Quickstart: a force rescan rebuilds unchanged videos

These steps show, on real media and `ffmpeg`, that artifacts are rebuilt for
files the scanner would otherwise skip and that the owner's choices survive,
which the unit tests prove only with fakes.

## Prerequisites

- A running server ([development.md](../../docs/how-to/development.md)) with
  two media folders, each holding at least one video, imported by one scan.
- One video with a chosen thumbnail position and one with an uploaded image;
  one hand-added tag; one pair bundled by the owner.
- Note the modification time of each file under `MDM_DATA_DIR/thumbnails`.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Settings, `Force rescan` on the scan status section, cancel the dialog | No scan starts; `GET /api/scans/current` keeps its `id` | 9 |
| Confirm the dialog | The indicator shows a force rescan of the library; every thumbnail, sprite and preview file has a new modification time | 1 |
| During the rebuild, open the library | Cards of videos not yet rebuilt show their previous thumbnail | 8 |
| `Force rescan` on one media folder's row, confirm | Only that folder's artifact files change | 2 |
| Video page, `Force rescan` | Only that video's artifact files change, with no dialog | 3 |
| On a video whose import issue list shows a failed stage, `Force rescan` | The stage runs again, and its issue disappears when it succeeds | 4 |
| Compare the chosen-position video and the uploaded-image video | The thumbnail shows the same frame; the uploaded image is unchanged | 5 |
| Open the owner-bundled pair and the hand-tagged video | Bundle and tag are unchanged; the activity log shows no `auto_tag` entry from the rescan | 6, 7 |
| Start `Scan library`, then `Force rescan` | The force rescan button is disabled with the reason | Edge case |
