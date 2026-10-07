# Quickstart: files added to a media folder appear without a scan

These steps prove, against a real filesystem and the shipped container, that
auto-import reacts to changes, waits for copies to finish, and reads nothing
while idle. `task check` and `task test-e2e` cover the batching and removal
rules with a fake watcher; they do not observe disk access.

## Prerequisites

- VVMDM running in Docker on Linux with a local media folder
  ([Run with Docker](../../docs/how-to/running-vv.md)), the account set up, the
  folder added in Settings and one manual scan finished.
- `strace` on the host, and the container's process id from
  `docker inspect -f '{{.State.Pid}}' <container>`.
- For the Windows rows: `VVMDM.exe` from the `nightly` release with a folder on
  a local NTFS drive.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Copy a video into a subfolder of the media folder | It appears in the library within about 15 s, with its thumbnail generated; the bottom-right shows no progress | 1, 5 |
| Move that video to another subfolder, then rename it | It stays the same video, with the tags and playback position set before the move | 2 |
| Delete the video | It leaves the library | 2 |
| Copy a file of several GB and open the library during the copy | It is absent until the copy ends, then appears | 3 |
| Run `strace -f -e trace=%file -p <pid>` and change nothing for 5 minutes | No `openat`, `newfstatat` or `getdents64` on a path under the media folder | 4 |
| Copy a file whose content is not a video, with a video extension | A failure notice appears; Settings lists the file's issue | 5 |
| Turn auto-import off in Settings and copy a video | It does not appear; turning auto-import back on starts no scan and does not add it | 6 |
| Start a manual scan from Settings | The full scan runs as before and adds the video from the previous row | 7 |
| Repeat the first three rows with `VVMDM.exe` on Windows | Same results | 1, 2 |
