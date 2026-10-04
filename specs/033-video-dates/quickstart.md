# Quickstart: Checking creation times on a real file system

These steps check, on a running server, the acceptance criteria of parent Issue
#630 that automated tests cannot verify because they depend on real file-system
creation times (acceptance criteria 4 and 5). The other acceptance criteria are
verified by the store, scanner, httpapi and Vitest tests that `task check` runs.
Record the results in the body of the implementation PR for the scan unit ("Read
file creation times during the scan and record them on locations").

## Prerequisites

- The server is running with `task dev` as in
  [docs/how-to/development.md](../../docs/how-to/development.md), you are logged
  in as the owner, and one media folder is registered. Below,
  `BASE=http://localhost:8080`.
- The media folder is on a file system that records creation times (ext4, xfs or
  btrfs on Linux, APFS on macOS, NTFS on Windows).

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| 1. Copy one video into the media folder and note its OS creation time (Linux: `stat -c %w <path>`; macOS: `stat -f %SB <path>`; Windows: the file's Properties in Explorer) | — | — |
| 2. Run a scan, open the video page, and run `curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos/lookup?path=<path>"` | The creation time in the facts row matches the value from step 1, and so does `fileCreatedAt` | 4, 7 |
| 3. Register a media folder on a file system without creation times (`tmpfs`, or a place where `stat -c %w` returns `-`) and repeat the steps | The creation time matches the file's modification time (`stat -c %y`) | 5 |
| 4. For a video registered before this feature (a database from before the migration), compare before and after a scan | Before the scan the creation time equals the modification time; after the scan it is the OS creation time | Edge case "videos already registered" |
