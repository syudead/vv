# Quickstart: verifying the English migration

Startup, scanning and checks are described in [Running vv](../../docs/how-to/running-vv.md) and
[Taskfile.yml](../../Taskfile.yml) (`task check`, `task test-e2e`). This page covers only the
checks specific to this feature.

## 1. No Japanese strings remain

```sh
task check          # includes the UI lint (no-restricted-syntax from R-3), gosmopolitan and type checks
```

Expected: everything passes. After all units, `web/eslint.config.js` no longer has the list of
directories excluded until their English migration
([research.md R-3](research.md#r-3-lint-and-types-detect-untranslated-text)).

## 2. Server output is English

1. Start the server with an empty `MDM_DATA_DIR`.
2. Run the initial setup, add a media folder, run a scan, add a path that does not exist, and log
   in with a wrong password.
3. Search the JSON log on stdout and the failure response bodies for Japanese.

```sh
grep -P '[\p{Hiragana}\p{Katakana}\p{Han}]' server.log   # prints nothing
curl -s -X POST localhost:8080/api/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"x","password":"y"}'                    # code and status as before the change; message in English
```

Japanese names that users gave (media folder paths, video names) may appear in the log; that is
correct.

## 3. Scan and probe failures

- Scan with an unreadable media folder (`chmod 000` it after adding it, or unmount it). The
  Settings page and the progress indicator at the top show an English explanation that includes
  the path (`Scan.errorCode = media_folder_unreadable`).
- Place a non-video file named `.mp4` and scan. The probe-failure message on the playback page is
  an English explanation, and the ffprobe output does not appear on screen
  (`Video.probeErrorCode = probe_failed`).

## 4. Japanese failure reasons from before the upgrade

Open a data directory where a pre-migration version produced the failures from section 3, with
the post-migration version. Alternatively, insert them in SQLite:
`update videos set probe_state='failed', probe_error='ffprobe が失敗しました' where id=…` and
`update scans set state='failed', error='取り込みの途中でアプリケーションが停止しました' where id=…`.

Expected:

- The playback page and the Settings page show a generic English failure explanation; no Japanese
  appears on screen.
- `probeError` in `GET /api/videos/{id}` and `error` in `GET /api/scans/current` keep the stored
  Japanese, and `probeErrorCode` and `errorCode` are absent.

## 5. Formatting and assistive technology

- Show one result and several results in a list; the count reads `1 video` / `2 videos`.
- The last scan time on the Settings page, the date added on the playback page, and the relative
  times in lists use English formats.
- In a screen reader (or the DevTools Accessibility tree), the accessible names of the player
  control bar, the search field, the tag controls and the scan start button are English and match
  the visible text. `document.documentElement.lang` is `en`.
