# Quickstart: Checking subtitle display by hand

These steps cover only what depends on human eyes and on browser
implementations, which the automated tests do not reach. Setup, startup and
checks follow [Taskfile.yml](../../Taskfile.yml) (`task dev`, `task check`,
`task test-e2e`). Go tests cover encodings, name matching, time shifting and API
access control; Vitest and Playwright cover the subtitle button, the menu, the
remembered selection, the `c` key and reattachment during live transcode.

## 1. Full screen and overlap with the control bar (acceptance criterion 10)

1. Put `movie.srt` next to `movie.mp4` and turn subtitles on in the playback
   screen.
2. Go full screen (`F`). The subtitles appear over the picture.
3. Still in full screen, move the mouse to show the control bar. The subtitle
   text moves up and does not overlap the control bar (including the progress
   bar). When the control bar hides, the text returns to its original position.
4. The same holds for a portrait video (the portrait fixture in
   `web/e2e/media-fixtures.mjs`).

## 2. Safari and iOS (native track display)

video.js uses the browser's native subtitle display in Safari. Playwright
checks Chrome and Firefox, so only Safari is checked by hand.

1. In Safari on macOS and on iOS, open the video from section 1 and turn
   subtitles on. The subtitles appear.
2. On iOS, the subtitles still appear when the player is full screen.
3. With a video played through live transcode (`container-only.mkv`), seek
   partway. The subtitles match the picture's time (the Safari check for
   acceptance criterion 9).

## 3. A real Japanese SRT (the real-file check for acceptance criterion 5)

The Go tests check the four encodings with generated fixtures. Put one real
Shift_JIS SRT you have (made with a Windows tool) next to a video and check
that the Japanese text is not garbled. If it is, attach its first 16 bytes
(`xxd -l 16`) to the Issue.
