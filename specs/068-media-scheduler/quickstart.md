# Quickstart: validating the scheduler on real disks

These scenarios check the parts of the parent Issue that unit tests cannot
reach: real disks, real ffmpeg processes and playback. Build and run the
server as in [development.md](../../docs/how-to/development.md); every grant
and release is logged at debug level with its disk key and slot count.

## Prerequisites

- Two media folders on different block devices: one on an HDD, one on an SSD.
- A few hundred videos for the HDD folder and a few dozen for the SSD folder.
- The current `main` build for the timing comparison.

## Two disks proceed together

1. Register both folders and start a scan.
2. Watch the debug log or `ps -o args -C ffmpeg`.

Expected: ffmpeg processes read from both folders at once; the HDD folder
never has more than one, and the small folder finishes without waiting for
the large one (acceptance criteria 1–3).

## Playback holds one slot

1. While the SSD folder imports, play a video from it.
2. Stop playback.

Expected: once the jobs running when playback started have finished, exactly
one background ffmpeg reads the SSD, and never none while work is queued.
After playback, the count returns to up to 4 (acceptance criterion 4).

## An owner request goes first

1. With hundreds of import jobs queued, force a rescan of one video.

Expected: that video's jobs start before the queued import jobs (acceptance
criterion 5).

## An unplugged folder parks its jobs

1. Unmount the HDD folder mid-import, so that its mount point is an empty
   directory, wait two minutes, mount it again.

Expected: no job of that folder that reads its files fails or loses an
attempt, fingerprints of its videos whose sprites are saved still complete,
the other folder keeps importing, and import of the HDD folder resumes within
a minute of the mount (acceptance criterion 8). Starting the server while the
folder is unmounted parks its jobs the same way.

## HDD import time

1. Import the same HDD folder into an empty data directory with the `main`
   build and with the feature build; record the time until the import
   progress settles.

Expected: the feature build is not slower. Record both times in the PR that
lands per-disk slots (acceptance criterion 10).

## Unknown-disk import time

1. Repeat the HDD timing for a folder whose disk resolves as unknown: a
   Btrfs or ZFS volume, or a network share.

Expected: the debug log shows the root with 1 read slot. Record both times in
the same PR. A slowdown is the cost of
[R-5](research.md#r-5-read-slots-per-disk-follow-the-disk-type) and is
reported, not fixed in this feature.
