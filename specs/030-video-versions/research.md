# Research: Bundle versions of the same video

Inherited decisions: the tech stack, boundaries and dependency direction, the split between index and user
data, where generated files live, and the authentication boundary follow their canonical documents
([docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[ARCHITECTURE.md](../../ARCHITECTURE.md), [internal/artifacts/store.go](../../internal/artifacts/store.go),
[specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)).
The earlier study (`docs/design-docs/video-identity.md` on branch `claude/video-detection-synonym-zgwmlw`) lists
candidate approaches; this file records only the decisions this feature settles.

## R-1: Bundles live in user-data tables, and bundle values sit in the existing tables under the bundle's own key

**Decision**: a bundle is held in two user-data tables, `video_bundles` (`user_key`, `representative_key`) and
`video_bundle_members` (`content_key` → bundle) ([data-model.md §1](data-model.md)). The bundle's tags,
playback position and visibility sit in the existing tables `video_tags`, `playback_progress` and
`public_videos`, keyed by `user_key` (`bundle:<id>`). Bundling copies the chosen representative's values to
this key. The rows under each member's `content_key` stay as they are.

**Rationale**: requirements 2, 3 and 7 and the Edge Cases require the bundle's values to belong to no member.
Changing the representative does not change the values (requirement 7), a removed member returns to its own
values (requirement 3), and the bundle and its values survive when the representative's file disappears (Edge
Case). If the values sat on the representative's `content_key`, changing the representative would require
moving them, and the values to restore on removal could not be told apart from the bundle's values. Keeping them
in the existing tables means reads only need key resolution (R-2), and the columns and rules of
`playback_progress` and the others (the completion rule, ordering by `updated_at`) are not duplicated.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Option A from the earlier study: re-point alias `content_key`s to a canonical key and merge the rows | Rejected: cannot meet requirement 3's "returns when removed", and moving the canonical key when the representative changes changes the user-data key |
| Option B: change the key of the user-data tables to an identity id | Rejected: needs a migration of three tables and rewriting the premise that user data is tied to the content identifier, and unbundled videos would need a new id too |

## R-2: User-data reads and writes resolve the user key with one expression and carry it in `Video.UserKey`

**Decision**: a video's user key is "the bundle's `user_key` if the video is a bundle member, otherwise its
`content_key`", decided by one expression in `internal/store` (`userKeyExpr`,
[data-model.md §3](data-model.md)). Every read that returns a video puts it in `domain.Video.UserKey`;
`internal/httpapi` uses this key to look up playback position and tags (`progressFor`, `tagsFor`,
`withProgress`, `withTags`) and to save the playback position. `registeredContentKeysForVideoIDs`, used by adding
and removing tags, tag summaries and toggling visibility, is replaced by `userKeysForVideoIDs`, which resolves to
this key; only display-name overrides (per content) keep resolving to `content_key`. The visibility check
(`publicVideoCondition`, `publicColumn`), tag filtering, search-box matching and the watch-state join use the
same expression.

**Rationale**: since bundle values sit in the existing tables (R-1), a single place on either the read or the
write side left on `content_key` produces a mismatch such as "the tag shows but filtering by it does not find the
video" or "made public but guests cannot see it". With one expression carried in `Video`, the code that builds
responses never chooses a key. Display names and thumbnail positions are close to facts about the file
(requirement 7 makes the title the new representative's), so they stay per content.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Look up the bundle in `internal/httpapi` and swap the key | Rejected: has no effect on the tag, watch-state and visibility conditions the listing SQL reads directly |
| Make `Video.ContentKey` itself the bundle key | Rejected: generated files, fingerprints and overrides are per content; mixing keys breaks finding where they are stored |

## R-3: The listing picks shown videos by representative; the search expression applies to all of a bundle's locations, the scope to the representative's

**Decision**: the library, search, folders, related videos and the folder index cover only **shown videos**:
videos in no bundle, and the effective representative of each bundle ([data-model.md §4](data-model.md)). The
effective representative is the `representative_key` video if it has a location the viewer may see; otherwise it
is the member with the smallest `videos.id` among those with such a location. `chosenLocationsCTE` applies the
scope (registered, public, folder) to the shown videos' locations, and changes the search expression to "some
registered location of the same bundle matches". Tags, watch state and sort order are judged on the shown
video's `UserKey` values. `GET /api/videos/{id}`, `versions`, playback and streaming also return
non-representative versions.

**Rationale**: requirement 5 says no list shows a non-representative version, a search term that matches any
version returns the one item, and the folder screen shows the item only in the representative's folder.
Splitting scope (representative's locations) from the search expression (the bundle's locations) meets all three
with one CTE. For the Edge Case "the bundle survives when the representative's file disappears, and a remaining
version is shown instead", deciding the effective representative on every read avoids adding state to either the
index or user data.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Drop non-representative versions' locations from `video_locations` | Rejected: locations are scan facts and come back on every scan |
| Apply the search expression to the representative's locations only | Rejected: violates requirement 5's "matching the title or path of any version" |
| Write the effective representative to `video_bundles` | Rejected: it would be rewritten every time a scan loses or regains a location, updating a user-data table for the index's sake |

## R-4: The folder index is built from shown videos' locations only and rebuilt in the same transaction as a bundle change

**Decision**: `folderIndexLocations` returns only the locations of shown videos (the effective representative as
the owner sees it, R-3); `domain.BuildFolderIndex` does not change. `VersionStore`'s bundle, change-representative
and remove operations call `rebuildFolderIndex` in the same transaction, as `SettingsStore` and
`FolderGroupStore` do. `domain.FolderIndexVersion` goes to 2 and the index is rebuilt at startup.

**Rationale**: requirement 5 says non-representative versions do not join folder groups, and an Edge Case says a
folder holding only non-representative versions becomes a folder with no videos. Dropping those locations from
the index input changes both the group assignment (the count of direct videos, whether child folders exist) and
the ancestor folder names consistently. A bundle operation that moves the representative to another folder
changes the direct counts, so that transaction rebuilds the index (the same treatment as "setting and clearing
an exception" in 017 §3).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Keep the index as is and exclude members when reading | Rejected: the group condition "2 or more direct videos" counts non-representative versions, leaving one-video groups |
| Rebuild only when a scan closes | Rejected: between bundling and the next scan, a removed version does not return to its group |

## R-5: Scan-time carry-over records a succession in `UpsertVideo` and decides by duration in the transaction that writes the probe result

**Decision**: when `UpsertVideo` creates a new video row because the content at a path changed, and the previous
video row loses its last location and is deleted, it records `video_successions (new_key, old_key,
old_duration_ms)` if the previous duration is known ([data-model.md §5](data-model.md)). If the previous key's
video appears at another path in the same scan (the key `newVideo` creates equals `old_key`), the record is
deleted. `ApplyProbe` and `ApplyProbeForJob` delete the record for the new key if there is one, and carry over
only when `domain.DurationsMatch(old, new)` (`max(1 s, 0.5% of the duration)`) holds and no video references
`old_key`. If the previous key is a bundle member, carry-over re-keys `video_bundle_members` and
`representative_key`; otherwise it replaces the `playback_progress` and `public_videos` rows and unions
`video_tags`. The previous key's own rows (values from before it joined a bundle, and dismissal records) are
re-keyed the same way.

**Rationale**: requirement 8 asks for carry-over only when the durations are nearly equal, and treats a large
difference as a different video. The new content's duration is unknown until its probe finishes, so the scan
cannot decide; it leaves a record and the transaction that writes the probe result decides. The previous
duration can only be read before the previous row is deleted, so it is copied into the record. The Edge Case
"files swapped" is met by not carrying over when the previous key remains as a video at another path. Probe jobs
progress while the scan is still running, so the decision waits until the recording scan closes with `done` (in
the transaction that closes the scan or in a later probe transaction); this keeps the record cancellable when the
previous key is found at another path afterwards. Carrying over a bundle member's place (Edge Case) is only a
key re-keying and does not touch the bundle's values.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Call `ffprobe` during the scan and compare durations | Rejected: the scan only reads, and heavy work is queued as jobs (ARCHITECTURE.md) |
| Keep the previous video row instead of deleting it | Rejected: more places would have to exclude videos with no location from listing conditions, and the content-reference check would change |
| Also require a fingerprint match | Rejected: the fingerprint comes after the sprite, long after the probe; requirement 8 says the decision is by duration only |

## R-6: The fingerprint is the pHash of seek-sprite frames, built by an ingest stage `fingerprint`

**Decision**: an ingest stage `fingerprint` is added last in `JobKinds`. Its claim condition is a registered
location and a finished seek thumbnail (`seek_thumbnail_state = done`); the transaction that writes the
finished seek thumbnail and the migration (for videos with a finished sprite) queue the job.
`app.Ingest.Fingerprint` reads the sprite layout and sheets from the generated-file store; `internal/media`
shrinks each frame to 32×32 luma (after cropping black bars on all four sides); `internal/domain` builds a 64-bit
hash by comparing the low-frequency 8×8 of a 2D DCT (excluding DC) with its median. A frame with small luma
variance is marked "flat". The fingerprint is stored in `video_fingerprints (content_key, version, interval_ms,
hashes)` ([data-model.md §6](data-model.md)). Comparing two (`domain.CompareFingerprints`) requires the same
`version`, pairs frames by time (the `b` frame that covers the midpoint of an `a` frame's interval;
`interval_ms` need not match), takes the median Hamming distance over pairs where neither frame is flat, and does
not compare when fewer than 3 pairs remain. The threshold is a median of 12 or less. The values are constants in
`internal/domain` tied to `FingerprintVersion`; changing them bumps the version and a migration rebuilds the
fingerprints.

**Rationale**: the targets of requirement 9 (re-encodes, different resolutions, different containers) have the
same duration and the same sequence of images, so pairing frames by time compares the same scenes. The sprite
interval depends on the duration: above 405 seconds it is `ceil(duration / 81)`, so a few milliseconds of
difference in duration changes the interval. Pairing by frame index would keep re-encodes of long videos from
becoming candidates. Ingest already builds a sprite for each video, so ffmpeg is not started even once more and
files on network drives are not read again. pHash is more robust than dHash to differences in brightness,
contrast and blur (earlier study §1.2). Flat frames are excluded because fades and black screens match in any
video. The threshold still has to be tuned on real data, so it is kept as constants with a version. Making it a
stage reuses scan progress, remaining work, failure records and the wake-up mechanism as they are.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A dedicated extraction stage (`ffmpeg` grabs small grey images at a fixed interval) | Rejected: reads every file once more per video; partially identical videos are out of scope, so an interval independent of duration brings no benefit |
| Audio fingerprints | Rejected: the bundled ffmpeg has no Chromaprint; out of scope |
| Build the fingerprint inside the seek-thumbnail job | Rejected: leaves no path to rebuild only the fingerprint when the sprite is complete, so bumping the threshold version would rebuild the sprites too |

## R-7: Candidates are computed with an SQLite function in the transaction that writes the fingerprint and stored in a table; dismissals are kept as user data

**Decision**: `ApplyFingerprintForJob` rebuilds the candidates for that content in the same transaction that
writes the fingerprint. The other side is content with a fingerprint of the same version and a duration within
the `DurationsMatch` tolerance, excluding dismissed pairs and pairs in the same bundle; pairs for which
`vv_fingerprint_distance(a, b)` is at or below the threshold go into `video_version_candidates (key_a, key_b,
distance)` ([data-model.md §7](data-model.md)). `vv_fingerprint_distance` is a deterministic function registered
with `modernc.org/sqlite` like `vv_shuffle_key`; it calls `CompareFingerprints` and returns -1 when the two
cannot be compared. "Different videos" is kept in `video_version_dismissals (key_a, key_b)` and excluded both
when computing candidates and when `Candidates` reads them. Bundling deletes candidates for pairs that ended up
in the same bundle, and when nothing references a content any more, its candidates, fingerprint and successions
are deleted.

**Rationale**: requirement 10 says a dismissed pair is never offered again; tying it to content keys as user data
keeps it effective when a rescan recreates video rows. Storing candidates in a table means the candidate list
does not read every fingerprint combination, and the Edge Case "the candidate disappears when one side
disappears" is met by deleting rows. Making the comparison an SQL function turns "compare, with the Go function,
only the partners narrowed by the duration index" into a single statement, and the candidate set and the
fingerprint are committed in the same transaction.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Compute candidates from fingerprints on every read | Rejected: every list compares all pairs with close durations; handling dismissals and deleted sides is the same, and a table is cheaper |
| Make candidates user data | Rejected: they are index data rebuildable from fingerprints; only dismissals cannot be rebuilt |

## R-8: Bundles are addressed by video id; new routes for listing versions, bundling, the representative, removal and candidates

**Decision**: the screen API never exposes a bundle id and operates on video ids
([contracts/screen-api.md](contracts/screen-api.md)).

| Route | Purpose |
| --- | --- |
| `GET /api/videos/{id}/versions` (guests allowed) | Every version of the bundle, representative first |
| `POST /api/video-bundles` (`videoIds` and `representativeId`) | Bundle |
| `POST /api/videos/{id}/make-representative` | Change the representative |
| `POST /api/videos/{id}/unbundle` | Remove from the bundle |
| `GET /api/version-candidates` | List candidates |
| `POST /api/version-candidates/dismiss` | Dismiss a candidate |

"Same video" on a candidate uses the same route, `POST /api/video-bundles`, and bundling removes the candidate.
The detail `Video` gains `versions {count, representativeId}`; list items do not.

**Rationale**: every screen holds video ids (the video page, the library selection, the two videos of a
candidate), and no screen carries a bundle id around. This follows the same idea as `VideoGroupRef` not exposing
a group id. The candidate decision is not split from the bundle route because the decision's result is bundling
itself; separate routes would hold the same rule in two places.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| The `PUT /api/video-bundles/{bundleId}` shape | Rejected: puts the bundle id in responses, and the id disappears when removal dissolves the bundle (Edge Case) |
| One `decide` route for candidate decisions | Rejected: the "same" response is `VideoVersions` and the "different" response is empty, so the shapes do not match |

## R-9: A bundle change publishes `domain.VideoBundleChanged`, mapped to the screen's `video` notification

**Decision**: `VersionStore` operations and carry-over (R-5) publish `VideoBundleChanged{VideoIDs}` (the video ids
of every affected member) after commit, and the screen subscription in `cmd/mdm/events.go` turns each id into a
`video` notification. It is not tied to worker wake-ups. Candidates change through the success or failure of
`fingerprint` jobs, for which `ProcessingChanged` becomes a `scan` notification, so the candidate screen
refetches on `scan`.

**Rationale**: the library and the video page already refetch items on the `video` notification
(`useItemRefresh`, `useVideoDetail`). Bundling removes items from the list or changes titles, so sending it for
every member lets the existing refetch bring the screen up to date. This is the same shape as
`VideoOverrideChanged`.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A new notification kind `versions` | Rejected: needs a new receiver on the screen, and `video` is enough |
| A notification for each candidate change | Rejected: adds a kind only for the candidate screen; refetching on `scan` delays by one job at most |

## R-10: The external API does not collapse versions; only the values become the bundle's

**Decision**: `api/external-v1.yaml` does not change. `GET /api/v1/videos` and `lookup` return every video with a
registered location as before, and `tags` and `POST /api/v1/video-tags` read and write by `UserKey` (R-2). No
bundle or remove operation and no MCP tool is added.

**Rationale**: the external API is a contract for tools that look up videos by file and content key (the 026
contract); hiding non-representative versions would stop a tool holding such a file from finding it with
`lookup`. The parent Issue's requirements are about the screen's lists and do not mention the external API.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Collapse to the representative in the external API too | Rejected: breaks compatibility and is not required |
| Add bundle fields to `ExternalVideo` | Rejected: not required, and no tool-side use is settled; adding the fields when needed is a compatible change |

## R-11: When the bundle's playback position is at or past that version's duration, the screen plays from the beginning

**Decision**: the server returns the bundle's playback position unchanged. When the video page (a pure decision
in `web/src/player`) decides where to resume, it plays from 0 if the position is at or past that version's
`durationMs`. `PUT /api/videos/{id}/progress` judges completion by the duration of the version that played.

**Rationale**: the Edge Case says "when versions differ in length and the bundle's playback position is past the
duration of the version being played, play from the beginning". The position is a fact about the bundle and the
library's watch state is derived from it, so rounding it in the response would make the list and the detail
disagree. The screen already owns the resume-position decision.

**Alternatives considered**: rounding `progress` in the response to the version's duration (rejected: drifts from
the list's "in progress" judgement).
