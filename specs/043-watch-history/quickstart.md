# Quickstart: existing playback records appear as history, and viewings stay separate

These steps prove, on a database written before this feature and with two
browser tabs, what `task check` and the browser test cannot set up: the
backfill of existing records, two playbacks of one video at the same time, and
(revision) the search keys of entries written before the revision and the date
list of a browser in another time zone.

## Prerequisites

- A VVMDM data directory from a build before this feature, with at least two
  videos that have been played (their cards show a progress bar or a watched
  mark), and one more played video whose file was moved out of the media
  folders and rescanned, so it left the library; backed up
  ([Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery)).
- A build with this feature, started on that data directory, signed in as the
  owner.
- For the revision steps: a data directory written by the build before the
  revision (the one with `00034_watch_history.sql` and no `title_key`), with
  entries on at least two calendar months; and a browser whose time zone
  differs from the server's (a phone, or the desktop browser's zone override in
  its developer tools).

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

## Revision: filter, search and date jump

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Start the revised build on the pre-revision data directory, open **History** and type part of the title of the entry whose video left the library | The entry is listed: its `title_key` was filled at startup | 11, Edge Case: not in the library |
| Rename a listed video with a display name, then search for a word in the new name, and for a word in the old file title | Both searches list its entries, shown with the new name ([R-10](research.md#r-10-title-search-uses-the-librarys-query-syntax-on-the-title-alone)) | 11 |
| In the browser whose zone differs from the server's, open the date list at 00:30 local time after playing a video at 23:50 the day before | The two days are listed as the browser's calendar says, not the server's | 13 |
| Choose a month from the date list, then scroll down | The list starts with the newest entry of that month and continues into older months | 13 |
| Choose a date, then open an entry and come back with `×` | The list is at the same date with the same filter and search | 13, [R-12](research.md#r-12-the-filter-the-search-and-the-date-live-in-the-screens-url) |
