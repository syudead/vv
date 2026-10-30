# Contract: the assistant's tools

Source of truth once implemented: the generated
`internal/httpapi/gen/assistant_tools.gen.json` (each operation's method,
path, summary, description and schemas, from
[api/openapi.yaml](../../../api/openapi.yaml)), the classification and the
write rows in
[`internal/domain/assistant_catalog.go`](../../../internal/domain/assistant_catalog.go),
and the built-in tools in `internal/app`. This contract lists what the
revision adds or changes; the name-based write rows already on the branch
keep their targets, arguments, batching and irreversible marks.

## Tool definitions sent to Ollama

Each tool is `{ type: "function", function: { name, description, parameters } }`.

| Tool kind | `name` | `description` | `parameters` |
| --- | --- | --- | --- |
| Read operation | `operationId` | The operation's `summary` and `description` | One object whose properties are the path and query parameters, or the JSON body, with the spec's schemas and descriptions |
| Name-based write row | The row id (`updateVideoTags.add`) | The operation's `summary` and `description`, plus `Proposed for the owner's approval; it does not run now.` | `target` (when the row has a target kind) and the row's arguments, as defined below |
| Schema write row | `operationId` | As for a name-based row | The operation's path parameters and JSON body, as for a read |
| Built-in tool | As listed below | As listed below | As listed below |

A tool whose result is an image answers
`{ "image": "<operationId>?<query-encoded parameters>", "contentType": "image/jpeg" }`.
The string is the reference that `clef` and `view_images` take.

### Arguments of name-based rows

The target object and the arguments keep their shipped meaning. A name
argument also accepts an `id` from a read result. That id is a pin and wins
only when it is among the candidates its name finds.

| Field | Schema |
| --- | --- |
| `target.kind` | `selection`, `current`, `named`, `filter` |
| `target.names` | string array; each item may be `{ name, id }` |
| `target.filter` | The criteria of `show_in_library` |
| Each argument of [the shipped list](../../../internal/domain/assistant_catalog.go) | Its type, with one shared description per argument |

## Core tools

These are sent on every model call. Every other tool is sent after
`find_tools` returns it.

| Tool | Kind | Arguments | Result |
| --- | --- | --- | --- |
| `screen_context` | Built-in read | none | The route and its entity's id and name, the selected videos (id, title), groups and smart folders (id, name), and the library filter |
| `find_tools` | Built-in | `query` (words) | Up to 15 tools, each with name, kind (`read`, `write`, `excluded`), summary and, when excluded, the reason and page; the read and write tools found are added to later calls |
| `app_help` | Built-in read | `query` | Matching pages (route, title, purpose) and operations (summary) |
| `listVideos` | Read operation | As the spec | The page with `total` |
| `getVideo` | Read operation | As the spec | The video |
| `listTags`, `listPeople`, `listOrganizations` | Read operations | As the spec | The pages |
| `show_in_library` | Built-in directive | The shipped filter fields | The resolved criteria with names; the panel opens the library with them |
| `open_page` | Built-in directive | The shipped navigation fields | The destination |
| `start_review` | Built-in | `kind` (`tags`, `people`, `works`) | The run with its id |
| `clef` | Built-in | See [clef](#clef) | Clef's answer |
| `view_images` | Built-in, for a `vision` model | See [view_images](#view_images) | `attached: n` |

### `clef`

| Field | Schema | Meaning |
| --- | --- | --- |
| `state` | object of strings | Facts, as the classifier's `state` |
| `images` | string array of image references | Replaced by the images' bytes, base64, before the call |
| `questions` | object of `{ type: "noul", instructions: string }` keyed by the caller's keys | As the classifier's `questions` |

The server adds `model` (the connection's decision model) and `keep_alive`,
and posts to `/v1/systemone` at the saved endpoint. The result is the
endpoint's JSON answer (`answers` keyed by question, each with `noul`), or
the error text. Images over the endpoint's image limit are sent as one tiled
grid.

### `view_images`

| Field | Schema | Meaning |
| --- | --- | --- |
| `images` | string array of image references | Read, downscaled to at most 1,024 px on the long side, and attached to the next `user` message as `images` |

## Write tools

The name-based rows already on the branch stay. The revision adds the rows
below; the operations they call come from the spec.

| Row | Kind | Card | Irreversible |
| --- | --- | --- | --- |
| `applyWorkInfoFetch` | Name-based; target one video; `fetchId` and the field names to apply | The fields it sets with their fetched values | No |
| `applyPersonPluginFetch` | Name-based; target one person; `fetchId` and the field names | As above | No |
| `editVideoThumbnail` | Composite; target one video; `atMs`, `instruction` | The video, the frame time and the instruction; runs `startThumbnailEdit`, `runThumbnailEdit`, `applyThumbnailEdit` | No (`revertThumbnailImage` undoes it) |
| `revertThumbnailImage` | Schema | The video | No |
| `setVideoDetails` | Schema | The video and the fields in the body | No |
| `putVideoProgress` | Schema | The video and the position | No |
| `undoActivity`, `retryActivity` | Schema | The entry's label from `getActivity` | No |
| `moveFaceAppearances`, `splitFaceAppearances`, `splitFaceAppearance` | Schema | The people the path and body ids name, and the appearance count | Yes |

`editVideoThumbnail` classifies `startThumbnailEdit`, `runThumbnailEdit`,
`applyThumbnailEdit`, and `discardThumbnailEdit` and `cancelThumbnailEdit`,
which it calls when a stage fails.

A schema row's card labels each id in its path parameters and its body with
the entity the id names. The generated file records, per operation, each id
field's location (`path` or a JSON pointer into the body) and its entity
kind, outside the schemas sent to the model. The generator takes the kind
from one of two sources in the spec, and the annotation wins:

| Source | Example | Kind |
| --- | --- | --- |
| The path parameter's `$ref` name in `components/parameters` | `PersonId`, `VideoId`, `ActivityId`, `FaceAppearanceId` | `person`, `video`, `activity`, `faceAppearance` |
| `x-vv-entity` on the parameter or property schema, or on an array's `items` | `FaceAppearanceMoveRequest.targetId`: `x-vv-entity: person`; its `appearanceIds` items: `faceAppearance`; `ActivityUndoRequest.tagId`: `tag` | The annotation's value |

The annotation goes on every id the `$ref` name does not type, which covers
every body id, because the generator inlines body `$ref`s. A test fails when
an id-like integer field (named `id` or ending in `Id` or `Ids`) of a write
tool's path parameters or body has no kind, or has a kind the card cannot
label. `moveFaceAppearances` thus shows the source person from the path, the
target person from `targetId`, and the count of `appearanceIds`. An id the
store does not hold makes the step `stale` before it runs.

## Read tools

Every owner `GET` is a read tool, except the excluded ones below. These
`POST` operations, which write nothing, are read tools too: `tagImpact`,
`summarizeVideoTags`, `summarizeVideoEdits`, `summarizeVideoCodeEdits`,
`listWorkInfoFillTargets`, `searchWorkInfo`, `fetchWorkInfo`,
`searchPeopleWithPlugin`, `fetchPersonWithPlugin`, `checkOllama`,
`checkComfyUI` and `checkAutoTaggingClassifier`.

A read whose response carries a credential is excluded, not a read tool.
The spec marks such a response property with `x-vv-secret: true`;
`PluginProxySettings.url` carries it, because the saved proxy URL may hold
`user:pass@`. The generator records, per operation, whether its response
reaches a property that has `x-vv-secret: true` or `format: password`, or a
string property named `password`, `secret`, `token`, `apiKey` or
`credential`. A test fails when such an operation is classified as a read
tool. The other owner `GET`s that could return credentials are already
excluded (`getAccount`, `listApiTokens`, `getAuthSession`). Plugin settings
answer no secret values, and ComfyUI answers only `apiKeySet`.

As a second guard, the runner replaces the userinfo of every URL in a read
result with `***` before the result reaches the model
(`socks5://user:pass@host:1080` becomes `socks5://***@host:1080`). This also
covers the Ollama endpoint and ComfyUI URLs.

## Excluded operations

| Excluded | Operations | Page | Why |
| --- | --- | --- | --- |
| Account, authentication and API tokens (Issue) | `setupAccount`, `login`, `logout`, `updateAccount`, `createApiToken`, `deleteApiToken`, `getAuthSession`, `getAccount`, `listApiTokens` | Settings, Account | Out of scope in the Issue |
| Plugin upload, approval, settings and order (Issue) | `uploadWorkInfoPlugin`, `deleteWorkInfoPlugin`, `approveWorkInfoPlugin`, `revokeWorkInfoPluginApproval`, `setWorkInfoPluginSettings`, `setWorkInfoPluginLinkOrder` | Settings, Plugins | Out of scope in the Issue |
| Network and proxy settings (Issue) | `updateNetworkSettings`, `updatePluginProxySettings` | Settings, Network | Out of scope in the Issue |
| Credentials in the response | `getPluginProxySettings` | Settings, Network | Its `url` may hold the proxy's user and password (`x-vv-secret`), which would reach the model |
| Media folders (Issue) | `createMediaFolder`, `updateMediaFolder`, `deleteMediaFolder` | Settings, Media folders | Out of scope in the Issue |
| Watch history deletion (Issue) | `clearWatchHistory`, `deleteWatchHistoryEntry` | History | Out of scope in the Issue |
| Face runtime download (Issue) | `downloadFaceRuntime` | Settings, Face recognition | Out of scope in the Issue |
| Onboarding (Issue) | `updateOnboarding`, `pickOnboardingInterests` | Onboarding | Out of scope in the Issue |
| Pairing (Issue) and the peer's own calls | `issuePairingCode`, `pairPeer`, `updatePeerAddress`, `confirmPeerCertificate`, `unpairPeer`, `forgetPeer`, `revokePeerGrant`, `applyPeerVocabulary`, `matchPeerCredits`, `applyPeerFetchedWorkInfo`, `applyPeerFetchedPerson`, `peerHandshake`, `getPeerRelatedClues` | Settings, Pairing | Trust decisions made with the other node's code; the peer's calls carry its credential |
| Irreversible deletion (Issue) | `deleteVideo`, `clearPeople` | The video page; Settings, danger zone | Out of scope in the Issue |
| Files the browser sends | `uploadThumbnailImage`, `uploadPersonImage`, `startThumbnailEditFromImage` | The video page; the person page | The server holds no such file; the fetch-apply rows cover plugin images |
| Drag operations | `organizeReorder`, `organizeGroup`, `organizeMove`, `organizeMerge`, `organizeBundle`, `organizeUndo` | Library | They take drag tokens; the group, bundle and playlist rows make the same changes |
| The server PC and streams | `openVideoFile`, `streamVideo`, `transcodeVideo`, `getTranscodeStart`, `getVideoSubtitle`, `streamEvents`, `getHealth` | The player | Loopback-only, or a media stream rather than data |
| The assistant itself | `/api/assistant/*` | | The assistant does not call itself |
