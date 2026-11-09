# Quickstart: Image search on a real library

These steps check the parent Issue's acceptance criteria with real encodes,
keyframe spacing and clipboard behaviour, which the generated fixtures of
`task check` and `task test-e2e` do not reproduce.

## Prerequisites

- A server built from the feature branch and run as in
  [Run locally](../../docs/how-to/development.md#run-locally), signed in as
  the owner, with a media folder of at least ten videos, one of them over
  30 minutes with the encoder's default keyframe interval.
- The `queue_frame_index` backfill has finished: the jobs table holds no
  queued or running `frame_index` row.
- `ffmpeg` on the tester's machine, to make the query files.
- A second browser session signed out (guest).

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Pause the long video at a moving scene, take a screenshot of the frame and drop the file on the search field | The video is first with its rate; choosing its frame plays from within a few seconds of the paused position | 1 |
| Copy the same screenshot to the clipboard, focus the search field and paste | The same first result and rate as the drop | 2 |
| Copy a word, focus the search field and paste | The word is typed into the field and no search starts | 2 |
| `ffmpeg -i <video> -vf scale=-2:360 -crf 30 small.mp4`, then drop `small.mp4` | The original is first and marked as a whole match | 3 |
| `ffmpeg -ss 600 -i <long video> -t 120 -c:v libx264 cut.mp4`, then drop `cut.mp4` | The long video is listed as a partial match with a section of about 10:00 to 12:00; choosing it plays from about 10:00 | 4 |
| Drop the screenshot on the video grid, outside the search field | No search starts and the page stays | 5 |
| As the guest, look at the search field and send `curl -F file=@shot.png <server>/api/image-search` without a session | No file control in the field; the request answers 401 `unauthenticated` | 6 |
| After the searches, list `MDM_DATA_DIR/search-uploads/` and search the library for the query file names | The directory is empty and no video named after a query exists | 7 |
| Switch off the media folder holding the long video and repeat the screenshot search | The long video is not listed | Requirement 8 |
