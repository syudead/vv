# Contract: the assistant's outcome schema and action catalog

Source of truth once implemented: `internal/domain/assistant_outcome.go`
(the outcome types, from which the JSON schema sent to the model is built)
and `internal/domain/assistant_catalog.go` (the catalog rows). The
operations the rows name are those of [api/openapi.yaml](../../../api/openapi.yaml).

## Outcome schema

One turn yields one outcome: an object with `kind` and the one member that
kind reads. The schema is sent to Ollama as `format`, so the model cannot
answer outside it. No member is free text the owner reads; the server
resolves every `name` and composes the thread's sentences.

| `kind` | Member | Meaning |
| --- | --- | --- |
| `filter` | `filter` | Show the library with these conditions |
| `navigate` | `navigate` | Open a screen |
| `answer` | `answer` | A typed question the server answers from its data |
| `action` | `action` | One catalog operation on a target, to be confirmed |
| `organize` | `organize` | Start a review run of one kind |
| `clarify` | `clarify` | The request does not say what to do, what to do it to, or with which value |
| `unsupported` | none | Nothing in the catalog does this |

### `filter`

| Field | Type | Meaning |
| --- | --- | --- |
| `q` | string | Search terms, as the library search box takes them |
| `favorite` | boolean | Favorites only |
| `tags` | string[] | Tag names, all required |
| `tagKind` | string | A tag kind name |
| `person`, `organization` | string | A name |
| `minDurationMs`, `maxDurationMs` | integer | Duration bounds |
| `addedFrom`, `addedTo` | string | Calendar days `YYYY-MM-DD`, computed by the model from the date the prompt gives |
| `watchState` | `unwatched`, `inProgress`, `watched` | As the cards show it |
| `sort` | `VideoSort` | The library sort |

### `navigate`

| Field | Type | Meaning |
| --- | --- | --- |
| `to` | enum | `video`, `person`, `organization`, `playlist`, `group`, `smartFolder`, `tag`, `library`, `folders`, `tags`, `people`, `organizations`, `playlists`, `history`, `duplicates`, `activity`, `settings`, `resume` |
| `name` | string | The entity's name for the kinds that take one; `video` takes a title or `current` |
| `settingsPage` | enum | A Settings page slug for `settings` |

`resume` opens the most recently played video that is in progress, from the
watch history, or answers `clarify` when there is none.

### `answer`

| Field | Type | Meaning |
| --- | --- | --- |
| `question` | enum | `count`, `appearances`, `recent`, `list` |
| `filter` | `filter` | For `count` and `list`: the conditions to count or list |
| `entity` | `{ kind, name }` | For `appearances`: a person or organization |
| `limit` | integer | For `recent` and `list`: at most 20, default 5 |

| Question | Server read | The thread shows |
| --- | --- | --- |
| `count` | `VideoPage.Total` of the filter | The number and a link to the filtered library |
| `appearances` | `VideoPage.Total` with `Person` or `Organization`, the person page's own count | The number and a link to the page |
| `recent` | The watch history, newest first | The titles with links |
| `list` | The first page of the filter | The titles with links and the total |

### `action`

| Field | Type | Meaning |
| --- | --- | --- |
| `operation` | enum of the catalog row ids | The row to run ([Catalog](#catalog)) |
| `target` | object | What it runs on, for rows with a target kind |
| `target.kind` | enum | `selection` (the selected videos or the open video), `current` (the open group, smart folder, person, playlist or tag), `filter` (every video matching `target.filter`), `named` (`target.names` resolved by the row's target kind) |
| `target.names` | string[] | Names for `named` |
| `target.filter` | `filter` | Conditions for `filter` |
| `arguments` | object | The fields the row reads, from the list below |

Arguments is one object; a row reads only the fields its table row names.

| Argument | Type | Used as |
| --- | --- | --- |
| `name` | string | A name to create, or the one entity to act on |
| `newName` | string | The name after a rename |
| `names` | string[] | Names of the sources of a merge, the members of a group, the items of a playlist |
| `tagNames` | string[] | Tags to attach, remove, confirm or reject |
| `tagKindName` | string | A kind to set |
| `personNames`, `organizationNames` | string[] | Credits to set, per role |
| `role` | `WorkPersonRole` | The role of `personNames` |
| `synonym` | string | A synonym to add or remove |
| `aliases` | string[] | Aliases to set on a person or organization |
| `code` | string | A work code |
| `url` | string | A link of a virtual video, or a connection URL |
| `dateKind`, `date` | enum, string | One work date |
| `enabled` | boolean | A switch |
| `number` | number | A probability, a position in a list, a thumbnail time in milliseconds |
| `public` | boolean | Visibility |
| `model`, `encoder`, `provider` | string | A model, an encoder or a provider name |
| `sort` | `VideoSort` | A smart folder's sort |
| `parentName` | string | A smart folder's parent |
| `scope` | enum | `all` or `untagged` for a run over the library |

### `organize`

| Field | Type | Meaning |
| --- | --- | --- |
| `kind` | enum | `tags`, `people`, `works` |

### `clarify`

| Field | Type | Meaning |
| --- | --- | --- |
| `about` | enum | `operation`, `target`, `value` |

The server adds its own clarifications, with candidates, when a name
resolves to several entities or to none
([plan.md, Names, context and clarification](../plan.md#names-context-and-clarification)).

## Catalog

Each row names its id, what the target is, the arguments the row reads,
whether video targets run in batches of 500 user keys, and whether the card
marks the change as one that cannot be undone. A row without a target kind
acts on the arguments alone.

A row's id is the `operationId` it calls. An operation whose request picks
one of several actions has one row per action, with the id
`operationId.action` (`updateVideoTags.remove`, `batchTags.reject`), and the
runner sends that action. The model therefore names the variant in
`operation`, and the variant decides the request and the irreversible mark.

### Videos

| Operation | Target | Arguments | Batches | Irreversible |
| --- | --- | --- | --- | --- |
| `updateVideoTags.add` | videos | `tagNames` | Yes | No |
| `updateVideoTags.remove` | videos | `tagNames` | Yes | No |
| `applyVideoEdits.add` | videos | `tagNames` and `personNames`, `role`, `organizationNames` to attach; `dateKind`, `date`, `public` to set | Yes | No |
| `applyVideoEdits.remove` | videos | `tagNames` and `personNames`, `role`, `organizationNames` to detach | Yes | No |
| `applyVideoCodeEdits` | videos | `code` | Yes | No |
| `updateVideoVisibility` | videos | `public` | Yes | No |
| `updateFavorites` | videos or groups | `enabled` | Yes | No |
| `setVideoDisplayName` | one video | `newName` (empty clears) | No | No |
| `setVideoWorkInfo` | one video | `code`, `personNames`, `role`, `organizationNames`, `dateKind`, `date`, `url` | No | No |
| `setVideoThumbnailPosition` | one video | `number` (milliseconds; absent clears) | No | No |
| `reprobeVideo` | one video | none | No | No |
| `autoTagVideo` | one video | none | No | No |
| `confirmVideoTag` | one video | `tagNames` | No | No |
| `rejectVideoTag` | one video | `tagNames` | No | Yes |
| `bundleVideos` | videos, the first as representative | none | No | No |
| `makeRepresentativeVersion` | one video | none | No | No |
| `unbundleVideo` | one video | none | No | No |
| `dismissVersionCandidate` | two videos | none | No | No |
| `rejudgeVersionCandidate` | two videos | none | No | No |
| `createVirtualVideo` | none | `name`, `url` | No | No |
| `mergeVirtualVideo` | one virtual video | `names` (the file video's title) | No | Yes |

### Tags and tag kinds

| Operation | Target | Arguments | Batches | Irreversible |
| --- | --- | --- | --- | --- |
| `createTag` | none | `name`, `tagKindName` | No | No |
| `renameTag` | one tag | `newName` | No | No |
| `deleteTag` | one tag | none | No | Yes |
| `mergeTag` | one tag (destination) | `names` (sources) | No | Yes |
| `addTagSynonym` | one tag | `synonym` | No | No |
| `removeTagSynonym` | one tag | `synonym` | No | No |
| `confirmTag` | one tag | none | No | No |
| `rejectTag` | one tag | none | No | Yes |
| `forgetRejectedTagName` | none | `name` | No | No |
| `batchTags.confirm` | tags by name or by `filter` (`tentative`, `unused`, a kind) | none | No | No |
| `batchTags.reject` | tags as for `batchTags.confirm` | none | No | Yes |
| `batchTags.delete` | tags as for `batchTags.confirm` | none | No | Yes |
| `batchTags.setKind` | tags as for `batchTags.confirm` | `tagKindName` | No | No |
| `batchTags.clearKind` | tags as for `batchTags.confirm` | none | No | Yes |
| `createTagKind` | none | `name` | No | No |
| `renameTagKind` | one tag kind | `newName` | No | No |
| `deleteTagKind` | one tag kind | none | No | Yes |
| `setTagKindCardFirst` | one tag kind | `enabled` | No | No |

### People and organizations

| Operation | Target | Arguments | Batches | Irreversible |
| --- | --- | --- | --- | --- |
| `createPerson` | none | `name`, `aliases` | No | No |
| `updatePerson` | one person | `newName`, `aliases` | No | No |
| `deletePerson` | one person | none | No | Yes |
| `mergePeople` | one person (destination) | `names` (sources) | No | Yes |
| `deletePersonImage` | one person | none | No | Yes |
| `confirmFaceCandidate` | one person | `names` (the other person) | No | Yes |
| `dismissFaceCandidate` | one person | `names` (the other person) | No | No |
| `reopenFaceCandidate` | one person | `names` (the other person) | No | No |
| `createOrganization` | none | `name`, `aliases`, `parentName` | No | No |
| `updateOrganization` | one organization | `newName`, `aliases`, `parentName` | No | No |
| `deleteOrganization` | one organization | none | No | Yes |
| `mergeOrganizations` | one organization (destination) | `names` (sources) | No | Yes |

### Playlists, groups and smart folders

| Operation | Target | Arguments | Batches | Irreversible |
| --- | --- | --- | --- | --- |
| `createPlaylist` | none | `name` | No | No |
| `renamePlaylist` | one playlist | `newName` | No | No |
| `deletePlaylist` | one playlist | none | No | Yes |
| `addPlaylistItems` | videos | `name` (the playlist) | Yes | No |
| `removePlaylistItem` | videos | `name` (the playlist) | Yes | No |
| `movePlaylistItem` | one video | `name`, `number` (the new position) | No | No |
| `createGroup` | videos | `name` | No | No |
| `renameGroup` | one group | `newName` | No | No |
| `dissolveGroup` | one group | none | No | Yes |
| `addGroupItems` | videos | `name` (the group) | Yes | No |
| `removeGroupItems` | videos | `name` (the group) | Yes | No |
| `moveGroupItem` | one video | `name`, `number` | No | No |
| `tagGroup` | one group | none | No | Yes |
| `deleteCollections` | groups and smart folders by name | none | No | Yes |
| `setFolderGrouping` | one media folder root by name | `enabled` (absent clears) | No | No |
| `createSmartFolder` | none | `name`, `sort`, `parentName`; the rule from `target.filter` | No | No |
| `changeSmartFolder` | one smart folder | `newName`, `sort`, `parentName`; the rule from `target.filter` | No | No |
| `deleteSmartFolder` | one smart folder | none | No | Yes |

### Settings and scans

| Operation | Target | Arguments | Batches | Irreversible |
| --- | --- | --- | --- | --- |
| `updateAutoTaggingSettings` | none | `enabled`, `number` with the field named by `name` (`use`, `automatic`, `add`, `suggest`) | No | No |
| `startAutoTagging` | none | `scope` | No | No |
| `updateVersionJudgingSettings` | none | `enabled` | No | No |
| `updateFaceRecognitionSettings` | none | `enabled`, `number` with the field named by `name` | No | No |
| `startFaceRecognition` | none | `scope` | No | No |
| `updateAutoImportSettings` | none | `enabled` | No | No |
| `updateTranscodingSettings` | none | `encoder` | No | No |
| `updateOllamaSettings` | none | `url`, `model` with the field named by `name` (`decision`, `generation`) | No | No |
| `updateComfyUISettings` | none | `provider`, `url`, `model` | No | No |
| `startScan` | none | none | No | No |
| `startRescan` | none | `scope` (`all` or the current folder) | No | No |

The catalog holds 74 operations in 80 rows. A settings row reads the
current settings first and sends them back with the one changed field, so
the card shows the field and its new value.

## Excluded operations

A Go test fails when a non-`GET` operation of `api/openapi.yaml` is in
neither the catalog nor one of these two lists, so a new operation is
classified when it is added.

| Excluded by the Issue | Operations |
| --- | --- |
| Account, authentication and API tokens | `setupAccount`, `login`, `logout`, `updateAccount`, `createApiToken`, `deleteApiToken` |
| Plugin upload, approval, settings and order | `uploadWorkInfoPlugin`, `deleteWorkInfoPlugin`, `approveWorkInfoPlugin`, `revokeWorkInfoPluginApproval`, `setWorkInfoPluginSettings`, `setWorkInfoPluginLinkOrder` |
| Network and proxy settings | `updateNetworkSettings`, `updatePluginProxySettings` |
| Media folders | `createMediaFolder`, `updateMediaFolder`, `deleteMediaFolder` |
| Watch history deletion | `clearWatchHistory`, `deleteWatchHistoryEntry` |
| Face runtime download | `downloadFaceRuntime` |
| Onboarding | `updateOnboarding`, `pickOnboardingInterests` |

| Excluded by this plan | Operations | Why |
| --- | --- | --- |
| Deleting every person | `clearPeople` | It sits in the Settings danger zone with the deletions the Issue excludes |
| Deleting a video | `deleteVideo` | Deleting a video belongs on the video page with its `contentVersion` check; an irreversible delete is not offered from conversation |
| Images the page sends | `uploadThumbnailImage`, `revertThumbnailImage`, `uploadPersonImage` | The input is a file, not a sentence |
| Thumbnail edit sessions | `startThumbnailEdit`, `startThumbnailEditFromImage`, `discardThumbnailEdit`, `runThumbnailEdit`, `cancelThumbnailEdit`, `applyThumbnailEdit` | The owner judges the output image on the video page |
| Plugin fetches | `searchWorkInfo`, `fetchWorkInfo`, `applyWorkInfoFetch`, `searchPeopleWithPlugin`, `fetchPersonWithPlugin`, `applyPersonPluginFetch` | The apply chooses among fetched candidates on a comparison screen |
| Face appearances | `moveFaceAppearances`, `splitFaceAppearances`, `splitFaceAppearance` | The appearances are chosen by looking at face crops |
| Drag operations | `organizeReorder`, `organizeGroup`, `organizeMove`, `organizeMerge`, `organizeBundle`, `organizeUndo` | They take drag tokens; the group, bundle and playlist rows cover the same changes |
| Activity log actions | `retryActivity`, `undoActivity` | They act on an entry chosen in the log |
| Playback and the server PC | `putVideoProgress`, `openVideoFile` | Playback state belongs to the player; opening an app is loopback only |
| Two operations in one | `setVideoDetails` | `setVideoDisplayName` and `setVideoWorkInfo` cover it |
| Reads and tests sent as `POST` | `summarizeVideoTags`, `summarizeVideoEdits`, `summarizeVideoCodeEdits`, `tagImpact`, `checkOllama`, `checkComfyUI`, `checkAutoTaggingClassifier`, `listWorkInfoFillTargets` | They write nothing |
| The assistant's own operations | `/api/assistant/*` | The assistant does not call itself |
