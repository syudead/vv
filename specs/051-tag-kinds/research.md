# Research: Tag kinds

Inherited decisions: tags, names and the tag filter
([014 data-model](../014-video-tags/data-model.md)), tentative tags
([031](../031-tentative-tags/contracts/screen-api.md)), paged tag lists and bulk
actions ([036](../036-tag-admin-scale/contracts/screen-api.md)), the external tag
administration ([039](../039-external-tag-admin/contracts/external-api.md)) and
the person and organization list filters
([047](../047-video-work-info/contracts/screen-api.md#list-parameters-person-and-organization)).
This file records only the decisions this feature adds.

## R-1: A kind is a row that tags point to

**Decision**: Kinds live in a new table `tag_kinds`, and each tag points to at
most one of them through a nullable `tags.kind_id`.

| Option | Rename or delete a kind | Same name twice | Verdict |
| --- | --- | --- | --- |
| **`tag_kinds` row, `tags.kind_id`** | One row changes; delete sets `kind_id` to null by foreign key | One unique column | Chosen |
| Kind name as text on `tags` | Rewrites every tag of that kind | Spelling variants become separate kinds | Rejected: requirement 4 renames and deletes kinds as units |
| A join table `tag_kind_links` | One row changes | One unique column | Rejected: allows several kinds per tag, which the Issue's `対象外` excludes |

**Rationale**: One column on `tags` makes "one kind or none" a property of the
schema, and `on delete set null` gives requirement 4's "the tag stays, without
a kind" without a store rule.

## R-2: Preset kinds are seeded rows with no special status

**Decision**: The migration inserts four kinds, `Genre`, `Work`, `Place` and
`Character`; after that they are ordinary kinds the owner may rename or delete,
and a deleted preset does not come back.

| Option | Verdict |
| --- | --- |
| **Seeded rows, no flag** | Chosen |
| A `preset` flag that forbids rename and delete | Rejected: requirement 4 lets the owner rename and delete kinds, and a fixed kind the owner does not use would stay in every kind menu |
| Presets defined in the catalog by key, not stored | Rejected: the screen is English only ([i18n.md](../../docs/design-docs/i18n.md)), and the server could not check a new name against a preset it does not store (Edge Case "same name twice") |

**Rationale**: `Character` is added to the Issue's three examples because a
work's characters are tagged beside the work itself; more presets would fill
the kind menu with kinds most owners never use.

## R-3: A tag's kind is set and cleared through the tag batch

**Decision**: `POST /api/tags/batch` gains the actions `setKind` (with
`kindId`) and `clearKind`; one tag or 20,000 tags take the same route, and
neither action confirms a tentative tag.

| Option | Verdict |
| --- | --- |
| **`setKind` and `clearKind` in the batch** | Chosen |
| `kindId` on `PATCH /api/tags/{id}` | Rejected: ten tags would be ten requests and ten partial failures (acceptance criterion 2) |
| A new route `PUT /api/tags/kind` | Rejected: repeats the batch's id limit, deduplication and three result arrays for one more action |
| One `setKind` where a null `kindId` clears | Rejected: the generated Go type cannot tell an absent `kindId` from a null one, so a client that forgot the field would clear kinds |
| Setting a kind confirms a tentative tag, as a rename does | Rejected: the Edge Case keeps a kind on a tentative tag, and a bulk kind change over suggestions would accept them unseen |

**Rationale**: The tag management screen already selects tags and applies a
batch action (requirement 3's "same flow"), and the external API reaches the
same store operation through its own batch
([039 R-3](../039-external-tag-admin/research.md#r-3-confirm-reject-and-delete-go-only-through-post-apiv1tagsbatch)).

## R-4: Every tag shape carries the kind's id and name

**Decision**: `Tag`, `TagRef` and `VideoTag` in the screen API, and `Tag` in
the external API, gain `kind`, an object `{ id, name }` or null.

| Option | Verdict |
| --- | --- |
| **`kind: { id, name } \| null`** | Chosen |
| `kindId` only, names from `GET /api/tag-kinds` | Rejected: every screen and external client that shows a tag would need the kind list loaded as well |

**Rationale**: A kind rename then changes the cached tag list's text, so the
screen reloads its tag list after a kind rename or delete (the rename is rare;
the reload already exists as `refreshTags`).

## R-5: The library filters videos by one tag kind

**Decision**: The list routes that take `person` gain `tagKind`, one kind id;
a video matches when any of its tags, manual or from its folder, has that kind.

| Option | Verdict |
| --- | --- |
| **`tagKind`: videos with any tag of the kind** | Chosen |
| Kind only narrows a tag picker in the library | Rejected: the library has no tag picker; a tag enters the filter by pressing its chip ([014 ui-design](../014-video-tags/ui-design.md#active-tag-filters)) |
| Several kinds at once | Rejected: "any Genre tag and any Work tag" is not a question the Issue asks, and one value keeps the filter chip row short |

**Rationale**: "Videos that have a Work tag" separates catalogued videos from
the rest, which the smart-folder work this Issue prepares for needs. The
parameter follows `person`: an id with no kind gives an empty list, and a
guest gets `guest_filter_not_allowed`, because the kind list is owner only.

## R-6: Kind names follow the tag name rules in their own namespace

**Decision**: A kind name is checked by `domain.NormalizeTagName` (trimmed, no
control characters, 1 to 100 code points, exact match) and is unique among
kinds only; a tag and a kind may share a name.

| Option | Verdict |
| --- | --- |
| **Tag rules, own namespace** | Chosen |
| One namespace with tag names | Rejected: a kind `Work` would block a tag `Work`, and the two are never shown in the same list |
| Case-insensitive uniqueness | Rejected: tag names are case-sensitive ([014 data-model, Name rules](../014-video-tags/data-model.md#name-rules)), and two rules for two kinds of name would surprise the owner |

**Rationale**: Reusing the rule reuses its three `reason` values, so the
screen and external clients handle a bad kind name the way they handle a bad
tag name.

## R-7: A kind has a name and nothing else

**Decision**: A kind stores no colour or icon; the `design` stage tells kinds
apart with the existing tokens and the kind's name.

| Option | Verdict |
| --- | --- |
| **Name only** | Chosen |
| An owner-chosen colour per kind | Rejected: needs a picker and a contrast check in both themes, and the Issue's `UI品質` keeps the kind secondary to the tag name |

## R-8: The tag list filters by one kind or by no kind

**Decision**: `GET /api/tags` takes `kind`, either a kind id or `none`; it
ANDs with `q`, `tentative` and `unused`.

| Option | Verdict |
| --- | --- |
| **One value, id or `none`** | Chosen |
| Several kinds | Rejected: acceptance criterion 1 asks for one kind and for no kind; a multi-value filter adds chips and a menu state for no requirement |
