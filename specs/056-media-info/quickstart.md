# Quickstart: Media information on real files

These steps prove what the fixture-based tests cannot: that real HDR, phone,
broadcast and chaptered files produce the items the parent Issue's acceptance
criteria name, and that a library imported before this feature is left as
it was (existing videos are not re-analysed, R-5).

## Prerequisites

- VVMDM running from this branch ([running-vv.md](../../docs/how-to/running-vv.md)).
- A media folder holding:
  - `hdr10.mp4`, `hlg.mp4` and `dv.mp4`: short HDR10, HLG and Dolby Vision
    encodes (sample clips from a camera or a public test set), and
    `sdr.mp4`, any SDR file;
  - `dual.mkv`: a file with two audio tracks tagged `jpn` and `eng` and an
    embedded SRT subtitle track tagged `jpn`;
  - `phone.mov`: a clip shot on a smartphone with location services on;
  - `rec.ts`: a Japanese terrestrial or BS recording that starts a few
    minutes before the programme, whose second audio or caption stream
    appears after the start;
  - `chapters.mkv`: a file with at least three titled chapters, one of them
    ending past the file's duration (`ffmpeg -i in.mp4 -i meta.txt
    -map_metadata 1 -map_chapters 1 -c copy chapters.mkv` with an `END` past
    the duration in `meta.txt`).
- For the last row: a database imported with the release before this
  branch, holding the same files.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Scan, wait for the import to finish, open each HDR file's video page | The Video group shows HDR10, HLG and Dolby Vision respectively; `sdr.mp4` shows SDR | 1 |
| Open `dual.mkv` | The Audio group lists both tracks with Japanese and English; the Subtitles group lists the Japanese track; `sdr.mp4` has no Subtitles group | 2, 3 |
| Open `phone.mov` | The Embedded group shows the shooting time, the device and the location; the Created and Added facts and the title are the same as before the feature | 4 |
| Open `rec.ts` | The Broadcast group shows the station name and the recorded programme's name, not the name of the programme before it; the Audio and Subtitles groups list the stream that appears after the start | 5 (requirement), 7 |
| Play `chapters.mkv` | The seek bar shows one mark per chapter inside the duration; hovering a mark shows its title; choosing a chapter in the chapters menu plays from its start; the last chapter ends at the file's end; `sdr.mp4` shows no mark and no chapters menu | 5 |
| Start the branch on the old database and open a video imported before it | No new groups and no queued `metadata` job; after the file changes and a scan, the groups appear ([R-5](research.md#r-5-media-information-is-read-only-by-the-import-probe-of-new-or-changed-files)) | 6 (re-analysis removed by the maintainer) |

Record each HDR file's `ffprobe` `color_transfer`, and the TS recording's
decoded names, its TOT time and the time `Probe` took on it, in the PR of the
unit that runs these rows. When the TOT is not inside the first 32 MiB or the
probe of `rec.ts` comes near the 30-second `probeTimeout`, the limits go back
to [R-2](research.md#r-2-the-same-ffprobe-run-reads-chapters-and-reads-longer-for-mpeg-ts)
and R-4 before the unit merges. When a name of
`rec.ts` decodes with a wrong or missing character, the ARIB symbol table
goes back to [R-4](research.md#r-4-station-and-programme-names-of-a-ts-recording-come-from-a-go-si-reader-with-an-arib-std-b24-decoder)
before the unit merges.
