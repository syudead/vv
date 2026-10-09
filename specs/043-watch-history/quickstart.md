# Quickstart: existing playback records appear as history, and viewings stay separate

These steps prove, on a database written before this feature and with two
browser tabs, what `task check` and the browser test cannot set up: the
backfill of existing records and two playbacks of one video at the same time.

## Prerequisites

- A VVMDM data directory from a build before this feature, with at least two
  videos that have been played (their cards show a progress bar or a watched
  mark), and one more played video whose file was moved out of the media
  folders and rescanned, so it left the library; backed up
  ([Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery)).
- A build with this feature, started on that data directory, signed in as the
  owner.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Open **History** from the sidebar | Every video that had a playback record is listed once, at the time its card's "Last played" sort uses | 8 |
| Find the entry of the video that left the library, then move its file back and rescan | The entry is listed and marked as not playable; after the rescan it opens the video | 8, Edge Case: file gone and back |
| Open one listed video and go back without pressing play | The list is unchanged | 2 |
| Open one video in two tabs, play both for a few seconds, then open the history | Two entries for that video, one per tab | Edge Case: two tabs |
| Play a short video to the end, press **Replay**, then open the history | Two entries for that video | 4 |
| While a video plays in one tab, delete its entry in another, then keep watching 10 s | Playback continues; the history shows a new entry for it | Edge Case: deleted while playing |
| Delete one entry, then open that video's card in the library | The card's progress bar, watched mark and place under "Last played" are as before | 6 |
