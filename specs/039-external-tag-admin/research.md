# Research: Tag administration from the external API and MCP

Inherited decisions: the external API and MCP boundary, its compatibility
policy and its error shape
([specs/026-external-api/research.md](../026-external-api/research.md) R-3 to
R-5, R-7 and R-8,
[docs/how-to/external-api.md](../../docs/how-to/external-api.md)); the tag
tables, name rules and merge semantics
([specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)); tentative
tags, rejected names and the write serialization that settles concurrent edits
([specs/031-tentative-tags/research.md](../031-tentative-tags/research.md) R-1 to
R-6); the paged list, bulk actions and `sourceIds` merge of the tag admin screen
([specs/036-tag-admin-scale/research.md](../036-tag-admin-scale/research.md) R-1,
R-4 to R-6, R-10). This file records only the decisions this feature adds.

## R-1: `GET /api/v1/tags` takes the screen's list parameters, and only the MCP tool pages by default

**Decision**: `GET /api/v1/tags` gains `q`, `tentative`, `unused`, `sort`,
`cursor` and `limit` with the meaning of the screen's `GET /api/tags`
([036 contracts/screen-api.md §5](../036-tag-admin-scale/contracts/screen-api.md#5-get-apitags-parameters)),
and `TagList` gains `total`, `totalAll` and `nextCursor`; `Tag` gains
`createdAt`, the value the `createdDesc` and `createdAsc` sorts order by. A
request without `limit` still returns every tag. The MCP tool `list_tags` fills
`limit` with 100 when the caller leaves it out, so its default response holds
one page ([contracts/external-api.md §1](contracts/external-api.md#1-get-apiv1tags)).

| Option | v1 compatibility | A tool call without arguments at 2,048 tags | Verdict |
| --- | --- | --- | --- |
| **Add the parameters; REST keeps "no `limit`, every tag"; the tool defaults `limit` to 100** | Kept (fields and parameters added) | One page, about 10 KB | Chosen |
| Add the parameters and make the REST default a page too | Broken: a script reading `.items` as the whole list gets 100 tags | One page | Rejected: the compatibility policy puts a changed default in `v2` |
| Add the parameters; the tool keeps the REST default | Kept | Every tag, about 147 KB | Rejected: the parent Issue's background names this response as unreadable, and the acceptance criterion asks that a response fit |
| A new operation `GET /api/v1/tags/page` | Kept | Depends on which tool the agent picks | Rejected: two list operations with two shapes, and the agent has to know which one pages |

**Rationale**: The store already answers the screen's `TagListQuery`, so the
external operation and the tool reuse one query shape and one cursor format,
and the acceptance criterion "read every page of the tentative tags matching a
search term" holds by the same code the screen runs. The tool is the one
surface the parent Issue names as unreadable, and a default inside the tool
changes no REST response, so the policy that `v1` only adds is kept.

## R-2: Tag operations are literal `POST` routes that name the tag in the body

**Decision**: The new operations are `POST /api/v1/tags/merge`,
`/tags/rename`, `/tags/synonyms` and `/tags/batch`, plus `GET` and `DELETE` on
`/tags/rejected-names`, and each body carries the tag id (`id`, `targetId`,
`sourceIds`, `ids`). No operation puts an id in the path
([contracts/external-api.md](contracts/external-api.md)).

| Option | MCP input | Verdict |
| --- | --- | --- |
| **Literal routes, ids in the body** | The generated request type, as for every existing tool | Chosen |
| Mirror the screen: `PATCH /tags/{id}`, `POST /tags/{id}/merge`, … | A hand-written type per tool that joins the path id with the body | Rejected: the 026 contract says a tool's input is the operation's body, and the glue would hold the only copy of that join |

**Rationale**: Every existing external operation identifies its targets in the
body (`VideoRef`, `items[].video`), so one convention covers the API, and the
generated server has no path-parameter operation to keep company. `rename`
and `synonyms` are the only operations that act on one tag, and an agent that
tidies a library sends them as calls, not as resource edits.

## R-3: Confirm, reject and delete go only through `POST /api/v1/tags/batch`

**Decision**: There is no single-tag confirm, reject or delete operation. The
batch operation takes `action` and `ids` (1 to 20,000) and returns
`appliedIds`, `notFoundIds` and `notApplicableIds`, as the screen's
`POST /api/tags/batch` does
([036 contracts/screen-api.md §1](../036-tag-admin-scale/contracts/screen-api.md#1-post-apitagsbatch)).
A single tag is `ids: [id]`.

| Option | A confirmed tag sent to `reject` | Verdict |
| --- | --- | --- |
| **Batch only** | `200`, the id in `notApplicableIds`, nothing changes | Chosen |
| Batch plus the screen's `POST /tags/{id}/confirm`, `/reject` and `DELETE /tags/{id}` | `409 tag_not_tentative` on the single route, `notApplicableIds` on the batch | Rejected: two answers to one situation, and requirements 4 to 6 and the edge cases describe only the batch answer |

**Rationale**: The parent Issue's edge cases ("a confirmed tag sent to reject,
a tentative tag sent to delete: leave it and return it as not processed") are
the batch outcome, and requirement 6 asks that the three actions take several
tags in one call. Six fewer tools keep the MCP tool list readable.

## R-4: Name conflicts answer `409 conflict` with the conflicting tag's `tagId` and `tagName`

**Decision**: Rename and synonym add return `409 conflict` with reason
`tag_name_taken` when the name belongs to another tag, and `tag_merge_required`
when the name is another tag's original name and `mergeTagId` does not name
that tag. The external `Error` gains `tagId` and `tagName`, present only with
these two reasons, so the caller can send `mergeTagId` without reading the
list again ([contracts/external-api.md §3 and §4](contracts/external-api.md#3-post-apiv1tagsrename)).

| Option | Verdict |
| --- | --- |
| **`mergeTagId` names the tag whose merge the caller accepts, and the error carries that tag's id** | Chosen |
| A boolean `merge: true` | Rejected: between the error and the retry another client can move the name to a different tag, and the boolean would merge a tag the caller never saw ([014 contracts/tags-api.md §3](../014-video-tags/contracts/tags-api.md#3-tag-management)) |
| Merge without asking when the name is another tag's original name | Rejected: requirement 3 asks that the merge happen only when the caller says so |
| `tagName` only, as the screen's error has | Rejected: the screen rereads its list to find the id; an agent would read a page per conflict |

**Rationale**: The acceptance rule of the screen is reused unchanged, so a
synonym added by the API and by the screen produce the same state
(requirement 9), and the one addition, the id in the error, is what an
agent needs to act on the answer. `merge_same_tag` (a `sourceIds` entry equal to
`targetId`) stays `400 invalid_request`, as in the screen.

## R-5: Every operation returns a JSON body, so every tool has a result

**Decision**: Synonym removal returns the tag after the change, and forgetting a
rejected name returns `{ name, removed }`; no new operation answers `204`.
`TagStore.RemoveSynonym` returns the tag read in its transaction, and
`TagStore.ForgetRejectedTagName` returns whether a row was deleted
([plan.md "Source Code"](plan.md#source-code)).

| Option | Verdict |
| --- | --- |
| **`200` with a body on every operation** | Chosen |
| `204` as the screen answers, and teach the MCP glue to turn an empty `2xx` into `{}` | Rejected: the tool result would say nothing about what changed, and the glue would gain a case that only two operations use |

**Rationale**: The MCP glue (`mcpTools.call`) takes the operation's JSON body
as the tool result and fails on an empty one; every existing external
operation returns a body. An agent that removes a synonym wants the tag's
remaining synonyms, and one that forgets a name wants to know whether the name
was in the list.

## R-6: The external handlers call the same `Tags` methods as the screen, with no new write path

**Decision**: Each new handler in `internal/httpapi` calls the existing
`TagStore` operation the screen handler calls (`ListTags`, `MergeTags`,
`RenameTag`, `AddSynonym`, `RemoveSynonym`, `BatchTags`,
`ListRejectedTagNames`, `ForgetRejectedTagName`) through the `Tags` interface
in `router.go`. No operation is added to `internal/app`, and no SQL is added
for this feature.

| Option | Verdict |
| --- | --- |
| **Reuse the screen's store operations through the same interface** | Chosen |
| A use case in `internal/app` that both the screen and the external API call | Rejected: the screen's tag handlers already call the store directly because each operation is one transaction ([014 contracts/tags-api.md](../014-video-tags/contracts/tags-api.md)), and a new layer for the second caller would move only the screen's calls |
| Store operations of their own for the external API | Rejected: requirement 9 and acceptance criterion 5 ask that the two surfaces produce the same state, which two code paths can only promise by test |

**Rationale**: One transaction per operation, shared by both surfaces, makes
"the result is the same as from the screen" true by construction, and a
simultaneous edit from the screen and the API resolves as the parent Issue's
edge case asks because SQLite serializes write transactions
([031 R-6](../031-tentative-tags/research.md#r-6-rejection-and-a-tentative-attach-of-the-same-name-rely-on-sqlite-write-serialization)).
