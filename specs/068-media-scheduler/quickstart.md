# Quickstart: validating the scheduler on real disks

These scenarios check the parts of the parent Issue that unit tests cannot
reach: real disks, real ffmpeg processes and playback. Build and run the
server as in [development.md](../../docs/how-to/development.md); every grant
and release is logged at debug level with its disk key and slot count.

## Prerequisites

- Two media folders on different block devices, one of them an HDD.
- A few hundred videos for the HDD folder and a few for the other.
- The current `main` build for the timing comparison.

## Two disks proceed together

1. Register both folders and start a scan.
2. Watch the debug log or `ps -o args -C ffmpeg`.

Expected: ffmpeg processes read from both folders at once; the HDD folder
never has more than one, and the small folder finishes without waiting for
the large one (acceptance criteria 1–3).

## Playback holds one slot

1. While the HDD folder imports, play a video from it.
2. Stop playback.

Expected: during playback exactly one background ffmpeg reads the HDD; after
playback the count returns to the disk's slots (acceptance criterion 4).

## An owner request goes first

1. With hundreds of import jobs queued, force a rescan of one video.

Expected: that video's jobs start before the queued import jobs (acceptance
criterion 5).

## An unplugged folder parks its jobs

1. Unmount the HDD folder mid-import, wait a minute, mount it again.

Expected: no job of that folder fails or loses an attempt, and import resumes
after the mount (acceptance criterion 8).

## HDD import time

1. Import the same HDD folder into an empty data directory with the `main`
   build and with the feature build; record the time until the import
   progress settles.

Expected: the feature build is not slower. Record both times in the PR that
lands per-disk slots (acceptance criterion 10).
