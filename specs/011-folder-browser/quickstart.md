# Quickstart: Verifying the folder screen

These steps check the folder screen against a feature-specific folder layout.
Startup and check commands are in [README.md](../../README.md) and
[Taskfile.yml](../../Taskfile.yml).

## Prerequisites

Register two media folders. Both end in the name `movies`, to check that
registered folders with the same name can be told apart (acceptance criterion 2).

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
├── many/            # 61 videos directly inside (acceptance criterion 12)
├── five/            # 5 videos directly inside (acceptance criterion 5)
├── only-deeper/
│   └── inner/ d.mp4 # a child folder with no direct videos (acceptance criterion 6)
├── 100% #1 日本語/ s.mp4   # special characters (Edge Case)
└── dup/
    ├── same-a.mp4   # two files with the same content; one card (Edge Case)
    └── same-b.mp4
<tmp>/b/movies/
└── copy-of-x.mp4    # same content as A/x.mp4; both share the playback position
```

Each video can be a small H.264 clip of a few seconds (made the same way as in
`web/e2e/media-fixtures.mjs`). Register and scan from the settings screen, or with
`POST /api/media-folders` and `POST /api/scans`.

## Steps

| Step | Expected result |
| --- | --- |
| 1. Open `/folders` | Two `movies` cards appear, told apart by their paths |
| 2. Open `movies` (a) | Child folders come before videos, in natural order: `1`, `2`, `10`, `100% #1 日本語`, `A`, `dup`, `five`, `many`, `only-deeper` |
| 3. Open `A` | The only video is `x` and the only child folder is `B`. The `B` card shows 1 video and 1 folder, and its only preview is `y` |
| 4. Look at the `five` and `only-deeper` cards | `five` shows at most 4 previews. `only-deeper` shows the outline and `動画 0 本` |
| 5. Reload the URL that opened `A/B/C`, then press Back | The same folder opens, and Back returns to `A/B`. The same holds for `100% #1 日本語` |
| 6. Scroll to the bottom of `many`, play a video in the middle, and go back | All 61 videos appear, and going back returns to the same position |
| 7. Change the sort | Only the videos are re-sorted |
| 8. Remove the `b/movies` registration in settings, then open that folder's URL | `見つかりません` and a link to the top level appear |
| 9. Use only the keyboard | The route sidebar `フォルダ` → folder card → video card → back through the breadcrumb can be completed |

`web/e2e/folders.e2e.ts` in `task test-e2e` checks steps 1 to 5, 7 and 9
automatically. The E2E folder layout has no `many/`. Paging for step 6 is covered
by unit tests in `internal/store` and `internal/httpapi`, and restoring the
position on return by `FolderPage.test.tsx`. Step 8 and the visual appearance are
checked by a person.
