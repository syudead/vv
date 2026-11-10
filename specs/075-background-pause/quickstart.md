# Quickstart: pausing real imports

These steps check the acceptance criteria that need long videos, a real
folder watcher or a server restart, which the automated checks replace with
fakes.

## Prerequisites

- A running server from [development.md](../../docs/how-to/development.md)
  with an owner account and automatic import on.
- A media folder with at least 20 videos not yet imported, one of them longer
  than 30 minutes.
- A second browser profile without a session, for the guest.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Start `Scan library`; while the long video's preview is generated, turn the switch on in Settings, `Scan`, `Background work` | The running list empties within seconds, no `ffmpeg` process of the server remains, and the counts stop falling | 1 |
| List `.tmp` under the thumbnails directory in the data directory | No file of the stopped job | 1 (edge case) |
| Turn the switch off | The counts fall again, the long video's preview is generated from the start, and every group reaches 0 | 1, 7 |
| Start a force rescan of the library; pause when half the files are read; resume | The scan continues, its progress ends at the total, and the server log shows no second content-key read for the files read before the pause | 2 |
| Pause; copy a new video into the media folder; wait one minute | The video is not in the library, and the `scan` group shows it waits | 3 |
| Resume | The video is imported | 3 |
| Pause; press `Scan library` | `Scan status` says the scan waits for the resume | 4 |
| Still paused, copy another video into the folder, then stop the server and start it again | The switch is on, nothing runs, and the counts are those from before the restart | 5 |
| Resume | The waiting scan runs and both the scan and the copied video finish | 4, 5 |
| Pause with work left; play a video and seek; play one that needs live transcoding | Both play and seek | 6 |
| Still paused, open the library and a video page | The indicator shows the paused state and the remaining total on both | 8 |
| Open the library in the guest profile; send `PUT /api/background/pause` without a session | No indicator state and no section; the request answers `401` | 9 |
