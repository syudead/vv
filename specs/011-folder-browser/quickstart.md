# Quickstart: verifying the folder page

Use [README.md](../../README.md) and [Taskfile.yml](../../Taskfile.yml) for the startup and check
commands. This document records only the folder layout specific to this feature and what to
verify against it.

## Folder layout

Register 2 media folders. Name both `movies` at the end, to verify that registered folders with
the same name can be told apart (Acceptance criterion 2).

```text
<tmp>/a/movies/
├── A/
│   ├── x.mp4
│   └── B/
│       ├── y.mp4
│       └── C/
│           └── z.mp4
├── 1/  w1.mp4
├── 2/  w2.mp4
├── 10/ w10.mp4
├── many/            # 61 videos directly inside (Acceptance criterion 12)
├── five/            # 5 videos directly inside (Acceptance criterion 5)
├── only-deeper/
│   └── inner/ d.mp4 # subfolder with no direct videos (Acceptance criterion 6)
├── 100% #1 日本語/ s.mp4   # special characters (Edge case)
└── dup/
    ├── same-a.mp4   # 2 files with the same content; 1 card (Edge case)
    └── same-b.mp4
<tmp>/b/movies/
└── copy-of-x.mp4    # same content as A/x.mp4; both share the playback position
```

Every video can be a small H.264 file a few seconds long (made the same way as
`web/e2e/media-fixtures.mjs`). Register and scan from the settings page, or with
`POST /api/media-folders` and `POST /api/scans`.

## What to verify

1. `/folders` shows 2 `movies` cards, told apart by their paths.
2. Opening `movies` (a) lists the subfolders before the videos, in natural order: `1`, `2`, `10`,
   `100% #1 日本語`, `A`, `dup`, `five`, `many`, `only-deeper`.
3. Opening `A` shows only the video `x` and only the subfolder `B`. The `B` card shows 1 video and
   1 folder, and its preview is only `y`.
4. The `five` card shows at most 4 previews. The `only-deeper` card shows the folder outline and
   "0 videos".
5. Reloading the URL of `A/B/C` opens the same folder, and Back returns to `A/B`. The same holds
   for `100% #1 日本語`.
6. Scrolling to the bottom of `many` shows all 61 videos. Playing a video in the middle and going
   back returns to the same position.
7. Changing the sort order reorders only the videos.
8. After removing the `b/movies` registration in the settings page, opening that folder's URL
   shows "not found" and a link to the top level.
9. Keyboard alone can go through the sidebar "Folders" → a folder card → a video card → back
   through the breadcrumb.

`web/e2e/folders.e2e.ts` in `task test-e2e` verifies 1 to 5, 7 and 9 automatically. The E2E
folder layout has no `many/`. The unit tests in `internal/store` and `internal/httpapi` verify the
paging in 6, and `FolderPage.test.tsx` verifies the restore on going back. A person verifies 8 and
the visuals.
