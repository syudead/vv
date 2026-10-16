# Research: Smart folders whose contents follow a saved condition

Parent Issue: #1139. Groups, their play order, visibility and the library item
query are inherited from [048 research](../048-groups/research.md) and
[048 data-model](../048-groups/data-model.md); partly matching groups from
[027 library-api.md](../027-partial-group-search/contracts/library-api.md);
tag kinds from #1136 (`specs/051-tag-kinds/`, in review when this plan was
written). The library filter is `domain.VideoQuery` and its SQL in
`internal/store/listing.go` and `internal/store/library_items.go`. This file
records only the decisions this feature adds.

## R-1: A smart folder is its own entity, not a group

**Decision**: Smart folders live in a new user-data table `smart_folders`
with their own ids, a rule, a sort and a parent
([data-model.md, Migration](data-model.md#migration)). Favorites, playlists,
the library and the video page address one by a `smartFolder` kind beside
`video` and `group`.

| Option | Verdict |
| --- | --- |
| **Own table and kind** | Chosen |
| A `groups` row with a rule | Rejected: a group holds items, takes its videos off the library's top level and nests through `group_items`; a smart folder does none of these (requirements 3, 5, 7), so every group read and write would branch on the kind |

**Rationale**: The Issue makes a smart folder a virtual view that never moves a
video, while a group is where a video actually is. Keeping them apart leaves
048's group rules untouched.

## R-2: Contents are computed when read, not stored

**Decision**: A smart folder's contents are the library item query run with
the folder's effective rule; nothing about membership is written.

| Option | Verdict |
| --- | --- |
| **Evaluate on read** | Chosen |
| A membership table refreshed after each change | Rejected: every write that can change a match (tags, credits, progress, favorites, bundling, imports) would have to update it, and one missed write leaves a wrong member until a full refresh |
| A background job that rebuilds memberships | Rejected: the folder lags its condition until the job runs, against acceptance criterion 1 |

**Rationale**: Requirement 3 then holds on every read whatever wrote the facts,
and an import never waits on a rule (Edge Case on tens of thousands of
videos). Telling open screens to read again is a separate notice
([R-12](#r-12-writes-that-can-change-a-match-publish-smartfolderschanged)).

## R-3: A rule is the library filter's fields; contents are the filtered library

**Decision**: A rule holds the library filter's fields (search text, watch
state, playable only, favorites only, tags, tag kind, person, organization,
added within, length), every one of which must hold. A folder's contents are
exactly what `ListLibrary` returns for that filter: videos in no group, a
group as one card when all its visible videos match, and the matching videos
of a partly matching group (027, 048 R-6).

| Option | Verdict |
| --- | --- |
| **The library filter and the library item query** | Chosen |
| An expression tree with OR, NOT and nested clauses | Rejected: the condition UI must use the library filter's words and parts (`UI品質`), and search text already accepts `OR` and `-` |
| Show a group when any of its videos matches | Rejected by the maintainer (2026-10-15): the folder shows what the filtered library shows (requirement 4, acceptance criterion 3) |

**Rationale**: Requirement 4 and acceptance criterion 5 ask the folder to equal
the filtered library. Calling the same query makes that true by construction
and follows the rule against copied code (AGENTS.md).

## R-4: The library filter gains added within and length

**Decision**: `VideoQuery` and the library URL gain `addedWithin` (days, 1 to
36,500: added at or after now minus that many days) and `minDuration` and
`maxDuration` (seconds, inclusive; a video with no known length matches
neither). Guests may use them, as they reveal nothing the guest's list does
not.

| Option | Verdict |
| --- | --- |
| **Add them to the shared filter** | Chosen |
| Smart folder only | Rejected: the condition editor would need parts the library lacks, and a saved filter could not carry them back (requirement 10) |
| An absolute date range for the added date | Rejected: a fixed range never gains a new video, so it is a one-off filter rather than a folder that keeps filling |

**Rationale**: Requirement 2 names the added date and the length as facts a
rule uses.

## R-5: A child's effective rule is every rule on its path, ANDed

**Decision**: A smart folder has at most one parent. Its effective rule is its
own rule and every ancestor's rule; the store applies each rule's filter
conditions in turn, so two levels may both name a person or a watch state.
A folder's page lists its children first, by name, then its contents.

```mermaid
flowchart LR
  root["猫 (tag)"] --> child["未視聴 (watch)"]
  child --> eff["Contents: tag 猫 AND unwatched"]
```

| Option | Verdict |
| --- | --- |
| **AND along the path, applied per level** | Chosen |
| Merge the rules into one `VideoQuery` | Rejected: single-valued fields (person, organization, tag kind, watch) from two levels cannot be merged, and a conflict would need a rule of its own |
| Children by hand, as group items | Rejected: the Issue asks the contents to follow conditions only (`対象外`) |

**Rationale**: Requirement 7 defines a child as matching both conditions.
Applying conditions per level keeps "the parent's contents filtered again"
exact for every field.

## R-6: Videos follow the folder's sort; random is refused

**Decision**: A folder stores one of the library's sorts other than `random`
(default `addedDesc`), and its contents are listed and played in it. A group
card sorts by its values as in the library.

| Option | Verdict |
| --- | --- |
| **A stored library sort** | Chosen |
| `random` | Rejected: the order would change on every read, so `Up next` and the member window would disagree between two requests |

## R-7: Root smart folders are cards in the unfiltered library

**Decision**: The library's top level lists every smart folder with no
parent as a card among the groups and videos, sorted by its values as a group
card is; its videos stay listed too. With a search or a filter, smart folder
cards are left out, except that favorites only lists the favorited ones. The
page computes only the sort value of each folder; the card's other values
are read when the card is on screen
([R-14](#r-14-folder-cards-carry-a-summary-their-values-are-read-on-screen-without-group-collapsing)).

| Option | Verdict |
| --- | --- |
| **Cards in the unfiltered library** | Chosen |
| Cards with a filter too, when all contents match | Rejected: a folder's contents rarely all match another filter, and checking it costs one evaluation per folder on every filtered read |
| Only on a separate list | Rejected by the maintainer (2026-10-15): requirement 6 mixes them with groups and videos |

**Rationale**: Requirement 5 keeps videos where they are, and requirement 6
puts the folders beside them. A filtered library is a search for videos, which
the folders' videos already answer.

## R-8: An empty folder stays and is shown to the owner only

**Decision**: A folder with no visible content is listed and opened for the
owner, and is not shown to a guest (`404` when opened), as 048 Visibility.

| Option | Verdict |
| --- | --- |
| **Owner sees it, guests do not** | Chosen |
| Hide it from everyone, as an empty group | Rejected: the owner could not open it to change its condition (Edge Case on zero matches) |

## R-9: Merges rewrite rules; a deleted reference makes the rule match nothing

**Decision**: Merging tags, people or organizations rewrites every rule that
names a source id to the target id, in the merge's transaction. A rule that
names a tag, tag kind, person or organization that no longer exists matches
nothing, and so do its descendants; the owner's reads name the missing
references.

| Option | Verdict |
| --- | --- |
| **Rewrite on merge; match nothing on a missing reference** | Chosen |
| Drop the missing field, as the library drops a missing tag (`MissingTagIDs`) | Rejected: a folder `タグ: 猫` whose tag is deleted would list every video, unlike what the Edge Case asks (`効かないと分かる`) |
| A foreign key per reference | Rejected: the references sit in a JSON rule ([data-model.md](data-model.md#smart_foldersrule)) that is always read whole |

## R-10: `owner_kept` records the owner's folders

**Decision**: `smart_folders.owner_kept` is 1 for a folder the owner created,
and is set to 1 by every owner write to its name, rule, sort or parent. Only
the write path the AI organizing Issue adds will create a folder without setting it.

| Option | Verdict |
| --- | --- |
| **A flag set by owner writes** | Chosen |
| A `created_by` value | Rejected: requirement 11 also asks an AI folder whose condition the owner changed to count as the owner's |

## R-11: The rule reads the owner's facts; visibility is the viewer's

**Decision**: A rule is evaluated with the owner's tags, progress, favorites
and full search, whoever views the folder; the result is then cut to what the
viewer may see (048 Visibility).

| Option | Verdict |
| --- | --- |
| **Owner's facts, viewer's visibility** | Chosen |
| Evaluate the rule as the viewer | Rejected: the guest filter refuses tags, credits, watch state and favorites (`guest_filter_not_allowed`), so a guest would see a different folder |

## R-12: Writes that can change a match publish `SmartFoldersChanged`

**Decision**: A new domain event `SmartFoldersChanged` (screen event kind
`smartFolder`) names folders whose contents or card may have changed. Folder
edits name the folder and its descendants; a committed write that can change
which videos match any rule names every folder, and nothing is published when
no folder exists ([data-model.md, Notices](data-model.md#notices)). A
progress save publishes only when it changes the video's watch state.

| Option | Verdict |
| --- | --- |
| **One notice naming every folder** | Chosen |
| Edits only | Rejected: a newly tagged video would not appear in an open folder or in the library's folder card until a reload (acceptance criterion 1) |
| Work out which folders the write changed | Rejected: it evaluates every rule before and after each write, the cost [R-2](#r-2-contents-are-computed-when-read-not-stored) keeps off the write path |
| Reuse the `group` event | Rejected: group and smart folder ids are separate spaces |
| Every progress save | Rejected: progress is saved every few seconds during playback, and each notice makes the library re-read its loaded pages |

## R-13: A read budget at 20,000 videos and 50 smart folders

**Decision**: With 20,000 videos, measured by a store benchmark on the
development host, each of these reads returns within 1 second: the first
unfiltered library page with 50 root folders, a folder's page three levels
deep, and the card values of 24 folders in one
[cards read](contracts/screen-api.md#routes-added). The benchmark's 50 root
folders mix tag, watch state, length, added-within and search text rules, as
`BenchmarkLibraryWithSmartFolders` does.

| Option | Verdict |
| --- | --- |
| **A benchmark with a stated budget** | Chosen |
| A time limit inside a unit test | Rejected: CI runners vary, and a timing assertion fails without a code change |
| 500 ms for the library page (the budget before 2026-10-16) | Rejected by the maintainer (2026-10-16): the library page with no folder takes about 0.6 s on the development host, so 500 ms left the folders nothing |

**Rationale**: The library page computes one sort value per root folder and
no other card value
([R-14](#r-14-folder-cards-carry-a-summary-their-values-are-read-on-screen-without-group-collapsing)),
so the folders share the part of the second that the no-folder page leaves.
A folder's page evaluates its own rule once and lists its children as
summaries, so the same budget holds three levels deep. The cards read
evaluates each folder once per video, without the group step that made a
folder cost about 0.5 s in PR #1204. `BenchmarkRebuildFolderIndex` sets the
same kind of budget for the folder index.

## R-14: Folder cards carry a summary; their values are read on screen, without group collapsing

**Decision**: A folder item in the library and a child folder item on a
folder's page carry only the folder's summary: id, parent id, name and
whether it is favorited. The library's first page places a root folder by
one sort value over its matching videos
([data-model.md, Library items](data-model.md#library-items)). The screen
reads the count, cover, length, size, dates and watch state of the cards it
shows through `GET /api/smart-folders/cards`
([contracts/screen-api.md](contracts/screen-api.md#routes-added)), which
evaluates the folder's rules on each video and collapses no group: the count
is the number of matching videos, and the cover is the first of them in the
folder's sort ([data-model.md, Contents](data-model.md#contents)). The
folder's own page keeps the exact values over the play order.

```mermaid
sequenceDiagram
  participant S as Library screen
  participant A as GET /api/library
  participant C as GET /api/smart-folders/cards
  S->>A: first page
  A-->>S: videos, groups, folder summaries in sort order
  S->>C: ids of the folder cards on screen
  C-->>S: count, cover, sums, dates per folder
```

| Option | Verdict |
| --- | --- |
| **Summaries on the page, card values on demand, evaluated per video** | Chosen |
| Every card's values on the first page | Rejected by the maintainer (2026-10-16): PR #1204 measured about 25 s for 50 root folders over 20,000 videos, about 0.5 s per folder |
| Card values on demand, exact over the play order | Rejected: the group step is most of the 0.5 s per folder, so a screen of 24 cards would take seconds, and the read repeats on every `smartFolder` event |
| Stored card values refreshed on write | Rejected: every match-changing write would evaluate every folder, the cost [R-2](#r-2-contents-are-computed-when-read-not-stored) keeps off the write path |
| Children with full values on the folder page | Rejected: a folder with 50 children would take the 25 s the library did |

**Rationale**: A card needs its sort value to be placed and its other values
only when it is seen. The per-video values differ from the page's only for a
rule with a watch state, playable only or favorites only over a group whose
videos are mixed, which the item query takes or leaves as one item (048
Library items) while the cards read counts its matching videos; the folder's
page, where the count is read beside the items, keeps the exact values.
