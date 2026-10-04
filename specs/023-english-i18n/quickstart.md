# Quickstart: Checking the English translation

These steps cover only this feature's checks; starting, importing and the
general checks are in [Running vv](../../docs/how-to/running-vv.md) and
[Taskfile.yml](../../Taskfile.yml) (`task check`, `task test-e2e`).

## No Japanese strings remain

```sh
task check          # includes the screen lint (R-3's no-restricted-syntax), gosmopolitan and type checking
```

Expected: everything passes. After all units, `web/eslint.config.js` has no
exclusion list of untranslated directories left
([research.md R-3](research.md#r-3-missing-translations-caught-by-lint-and-types)).

## Server output is in English

Start the server with an empty `MDM_DATA_DIR`, then perform initial setup,
register a media folder, import, register a nonexistent path, and log in with a
wrong password. Search the JSON logs on standard output and the bodies of
failure responses for Japanese.

```sh
grep -P '[\p{Hiragana}\p{Katakana}\p{Han}]' server.log   # prints nothing
curl -s -X POST localhost:8080/api/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"x","password":"y"}'                    # code and status as before the change, message in English
```

Japanese names the user gave (media folder paths, video names) appearing in the
logs is correct.

## Import and probe failures

- Importing an unreadable media folder (`chmod 000` after registering it, or
  unplugging it) shows an English explanation that includes the path, on the
  settings screen and in the progress display at the top
  (`Scan.errorCode = media_folder_unreadable`).
- Importing a non-video file named `.mp4` makes the probe failure display on
  the playback screen an English explanation, and no ffprobe output appears on
  screen (`Video.probeErrorCode = probe_failed`).

## Japanese failure reasons from before the upgrade

Open, with the translated version, a data directory in which the failures of
step 3 were created by the pre-translation version (or insert
`update videos set probe_state='failed', probe_error='ffprobe が失敗しました' where id=…`
and
`update scans set state='failed', error='取り込みの途中でアプリケーションが停止しました' where id=…`
with SQLite).

Expected: the playback screen and the settings screen show a general English
failure explanation, and no Japanese appears on screen. `probeError` in
`GET /api/videos/{id}` and `error` in `GET /api/scans/current` remain the stored
Japanese, and `probeErrorCode` and `errorCode` are absent.

## Formatting and assistive technology

- Showing results of one video and of several in the list gives counts of the
  form `1 video` / `2 videos`.
- The date-time of the last import on the settings screen, the date added on
  the playback screen, and relative times in the list appear in English format.
- With a screen reader (or the DevTools Accessibility tree), the accessible
  names of the player's control bar, the search field, the tag controls and the
  import start button are English and match the visible text.
  `document.documentElement.lang` is `en`.
