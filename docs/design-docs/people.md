# People: profiles and faces

A person carries a fixed profile and at most one face next to the names,
aliases and external IDs that people and organizations share. The profile
rules live in [`person_profile.go`](../../internal/domain/person_profile.go)
and the storage in
[`person_profiles.go`](../../internal/store/person_profiles.go) and
[`person_images.go`](../../internal/store/person_images.go).

The diagram shows where a profile is checked and written on the way in.

```mermaid
flowchart LR
  screen[PATCH /api/people/id] --> http[creditPatchOf]
  external[POST /api/v1/people/update] --> http
  http --> store[CreditStore.Update]
  store --> domain[NormalizePersonProfile]
  store --> columns[(people columns + people_links)]
```

## Profile fields are columns on `people`

Each field is a nullable column on `people`, and links are rows in
`people_links` ordered by position and deleted with their person. The field
set is fixed, so typed columns keep each check in one function and a list or a
person page reads the profile without a join. Organizations have no profile;
`profile` sent for an organization is `400 invalid_request`.

| Rejected                                      | Why                                                            |
| --------------------------------------------- | -------------------------------------------------------------- |
| A `person_profiles` table, one row per person | One more join on every read and a missing-row case for nothing |
| Key-value rows per field                      | Pivoting on read, and validation spread over the rows          |

## What a valid profile is

`NormalizePersonProfile` trims every text field and stores an empty value as
`null`. Errors carry a `work_info_*` reason and the field `profile.<name>`.

| Field                              | Accepted                                                                                 | Error                                                                |
| ---------------------------------- | ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `gender`                           | `female`, `male`, `other`                                                                | `work_info_out_of_range`                                             |
| `birthDate`, `deathDate`           | `YYYY`, `YYYY-MM` or `YYYY-MM-DD`, the work-date format                                  | `work_info_invalid_date`                                             |
| `birthDate`, `deathDate`           | Not after today in UTC                                                                   | `work_info_out_of_range`                                             |
| `deathDate`                        | Not before `birthDate` at the precision both share, so `1990` and `1990-05` fit together | `work_info_out_of_range`                                             |
| `birthplace`                       | 1–200 code points                                                                        | `work_info_too_long`                                                 |
| `heightCm`                         | 50–300                                                                                   | `work_info_out_of_range`                                             |
| `bustCm`, `waistCm`, `hipsCm`      | 30–300 each                                                                              | `work_info_out_of_range`                                             |
| `cup`                              | 1–3 Latin letters, stored upper case                                                     | `work_info_too_long`, `work_info_out_of_range`                       |
| `bloodType`                        | `A`, `B`, `O`, `AB`, in any case                                                         | `work_info_out_of_range`                                             |
| `careerStartYear`, `careerEndYear` | 1900–9999, the end not before the start                                                  | `work_info_out_of_range`                                             |
| `bio`                              | 1–5,000 code points; line breaks kept                                                    | `work_info_too_long`                                                 |
| `links`                            | Up to 20 unique HTTP(S) URLs, as work URLs                                               | `work_info_invalid_url`, `work_info_duplicate`, `work_info_too_many` |

Owners and plugins often know a date only to the year, so a date keeps its own
precision instead of being padded to a full date.

## A patch changes one field at a time

In `PATCH /api/people/{id}` and `POST /api/v1/people/update`, each `profile`
property present replaces its field, `null` clears it, and `links` replaces the
whole list. A patch that leaves the profile as it was writes nothing. Profile
writes skip the smart-folder and collection refresh that name changes trigger,
because no smart rule or collection value reads a profile.

## A merge fills the target's empty fields

The merge target keeps every field it has. Each field it lacks takes the value
of the first source, in `sourceIds` order, that has one; an empty link list
takes the first source's list. Merging a duplicate that a plugin created keeps
the profile that the plugin brought.

Fields filled from different people can contradict each other, such as a
death date from one source before the target's birth date. A source value that
would make the profile invalid is skipped, and the field takes the next
source's value or stays empty, so a merge always stores a valid profile.

| Rejected                       | Why                                                                              |
| ------------------------------ | -------------------------------------------------------------------------------- |
| Keep only the target's profile | The duplicate's profile would be lost                                            |
| Reject the merge with `400`    | A merge cannot be fixed by editing its request, only by editing the people first |
| Let the owner choose per field | The merge stays one step                                                         |

## A person without a canonical name is provisional

Face recognition finds people before anyone knows their names, so a person may
have no `people_names` row at all. Such a person is provisional: `Person`,
`CreditRef` and `WorkPerson` carry `provisional: true` and `name` is `""`, so
a client that ignores the flag still parses the response. The flag is read
from the missing canonical row by every person query, never stored, so a
named provisional person cannot exist. Organizations always have a name and
read `provisional: false`.

| Operation                       | Effect on a provisional person                                                                                            |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/people?provisional=…` | `true` keeps only provisional people, `false` only named ones; an organization list answers `400 invalid_request`         |
| Natural-name order              | Provisional people come after every named person, by ID; the other sorts break ties the same way                          |
| `PATCH` with `name`             | The person becomes an ordinary person in the same write                                                                   |
| `PATCH` without `name`          | Profile and external-ID changes keep the person provisional; aliases need a name and answer `work_info_empty` on `name`   |
| Merge into a provisional target | The target takes the canonical name of the first source in `sourceIds` order that has one; the other names become aliases |

The name order keeps one keyset cursor by prefixing each listed sort key with
`0` for a named person and making it `1` for a provisional one, so the cursor
needs no null branch.

| Rejected                                       | Why                                                                                                |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| A `provisional` column beside the name         | Two facts that can disagree                                                                        |
| A stored placeholder name such as `Unknown 12` | It cannot follow the screen's language, it matches name searches, and a merge keeps it as an alias |

## People sort by age, unknown birth dates last

People lists take two sorts that tags and organizations do not: `ageAsc`
puts the newest birth date first and `ageDesc` the oldest. An organization
list answers either with `400 invalid_request`.

The store keeps `birth_key` next to `birth_date` as `YYYYMMDD` with missing
parts as `00`, and writes both in the same statement, so a partial date sorts
as its first possible day. Each sort reads an unknown date as the value that
sorts last in its direction, `0` for `ageAsc` and `99999999` for `ageDesc`, so
the keyset cursor keeps an integer value and its comparison has no null
branch. Ties use natural-name order, then ID, as every other sort does.

| Rejected                                  | Why                                                                   |
| ----------------------------------------- | --------------------------------------------------------------------- |
| Add the sorts to `TagSort` for every list | Tags and organizations have no birth date                             |
| Sort the loaded rows in the browser       | The list is paged by keyset, so the browser holds only the first page |

## Favorites join the shared favorites

A favorite person is a row in `person_favorites`, set and cleared through
`personIds` on `PUT /api/favorites` together with videos, groups and smart
folders. The IDs count toward the same 1–20,000 limit, missing people are
ignored, and setting a favorite again keeps its time. `Person.favorite` is
always present, and `GET /api/people?favorite=true` keeps only favorites, with
`total` counting them; an organization list answers `favorite` with
`400 invalid_request`. No smart rule reads a person favorite, so setting one
queues no smart-folder refresh.

A merge makes the target a favorite when any source is one, so merging a
duplicate never drops the mark. Deleting a person deletes its row.

| Rejected                        | Why                                                                                                                     |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| A `favorite` column on `people` | Every other favorite is a table of its own with its time, and the shared endpoint already sets several kinds atomically |
| `PUT /api/people/{id}/favorite` | A second way to set favorites beside the shared endpoint                                                                |

## A face is one fitted JPEG in SQLite

A person has at most one face, a row in `person_images` deleted with its
person. `PUT /api/people/{id}/image` decodes the upload, fits it to 800 px on
its long side without enlarging it, and stores it as JPEG
([`person_images.go`](../../internal/app/person_images.go)). The upload uses the
thumbnail upload's body budget, byte and pixel limits, decode gate and
animated-WebP refusal, described in
[thumbnail-editing.md](thumbnail-editing.md#uploaded-images); the two kinds of
upload share one gate, so they never decode at the same time.

| Problem                                                        | Response                                  |
| -------------------------------------------------------------- | ----------------------------------------- |
| Body over 64 MiB                                               | `413 person_image_too_large` with `limit` |
| Not a readable PNG, JPEG or still WebP, or over 120 megapixels | `400 person_image_invalid`                |
| No usable face, while the face runtime is ready                | `400 person_image_no_face`                |
| More than one usable face, while the face runtime is ready     | `400 person_image_many_faces`             |
| No face on `GET`                                               | `404 person_image_not_found`              |

Face recognition checks the face
([face-recognition.md](face-recognition.md#a-face-image-merges-only-one-provisional-person)).

A face is user data, so it is not a file under the rebuildable thumbnails
directory, and an 800 px JPEG of about 200 KiB costs little as a blob.

| Rejected                               | Why                                                                                                |
| -------------------------------------- | -------------------------------------------------------------------------------------------------- |
| A file managed by `internal/artifacts` | The thumbnails directory is rebuildable, and artifacts are removed by video content, not by person |
| Fitting through ffmpeg                 | Needs the media adapter for a still image, while `x/image/draw` scales without cgo                 |
| Keeping the original upload            | An upload may be 64 MiB, and the screens show at most a portrait card                              |

## A face can be framed from a video

The camera menu on the person's face offers `Choose from videos…` beside
`Upload an image…`, with or without a face. It opens an `EditorDialog`
([`FacePickerDialog.tsx`](../../web/src/credits/person/FacePickerDialog.tsx)):

| Part    | Shows                                                                                                                                                                            |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Faces   | Up to 48 of the person's crops from `GET /api/people/{id}/appearances?sort=mostSimilar`, most like the person first; a face whose video is gone or that has no track is left out |
| Stage   | The original frame from `GET /api/videos/{id}/frame?positionMs=`, at the middle of the face's longest track, with a 2:3 portrait outline that drags and takes arrow keys         |
| Frame   | The moment, with buttons one second earlier and later                                                                                                                            |
| Size    | A slider for the outline's width, from 5% of the frame's long side to the widest that fits                                                                                       |
| Preview | The portrait as the profile shows it and the round crop the cards take from it                                                                                                   |

The outline starts at 2.2 times the face's mean width with the face above its
centre ([`faceFraming.ts`](../../web/src/credits/person/faceFraming.ts)), from
the appearance's `focus`: the longest track's middle, mean centre and width as
fractions of the frame's long side, as face recognition samples them.
`Set as face` cuts the outline out of the frame at the frame's own pixels and
sends it to `PUT /api/people/{id}/image` as a JPEG, so the face goes through
the same fitting and face check as an upload and the dialog shows their
refusals. The frame endpoint is owner-only, answers a PNG at most 2048 px on its
long side with `private, max-age=3600`, and reports a position outside the
video, an unreadable file and a frame that does not decode as the thumbnail
position endpoint does.

| Rejected                           | Why                                                                                                  |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Choosing a stored crop as the face | A crop is cut from a frame scaled to 1280 px for analysis, so it is coarse, and its framing is fixed |
| Cutting the face on the server     | The page already holds the frame it shows, and the upload path fits and checks any image             |
| A free-form crop shape             | Every screen shows the face as a portrait or a circle inside it, so another shape is cut again       |

## The image revision never repeats

`people.image_revision` increases on every change of the face: an upload, a
removal and a merge that brings a face. `Person` and the video page's
`WorkPerson` carry it as `imageRevision` only while the person has a face, a
client loads the face from `/api/people/{id}/image?v={imageRevision}`, and
`GET` answers `private, no-cache` with the revision as `ETag`. The counter lives on `people`,
not on the image row, so removing a face does not reset it and a later face
never answers `304` to the removed face's `ETag`.

| Rejected                              | Why                                                  |
| ------------------------------------- | ---------------------------------------------------- |
| A revision on the `person_images` row | Removing the row resets it                           |
| A boolean `hasImage`                  | A replaced face would stay cached under the same URL |
| Data URLs in list responses           | A page of 100 people would carry 100 faces           |

A merge keeps the target's face; a target without one takes the face of the
first source in `sourceIds` order that has one. Deleting or merging a person
also deletes or moves their face recognition data
([face-recognition.md](face-recognition.md#face-data-follows-its-person)).

## The people grid

`/people` is a `ListPage` of round face cards
([patterns.md, Face grid](../../web/registry/rules/patterns.md#face-grid)),
because people are found by face. A card is one link to the person page with
the face, the name and `34 · 12 videos`, the age computed as on the person
page ([Ages are computed in the browser](#ages-are-computed-in-the-browser)),
and the favorite heart outside the link. Renaming, merging and deleting moved
to the person page, so a card has no other control.

The grid reads `GET /api/people` 100 at a time with the shared paging of the
admin lists ([library-ui.md](library-ui.md)), and asks for the next page when
the end of the grid comes into view. The search, `favorite=true` and the sort,
including `ageAsc` and `ageDesc`, are sent to the server and kept in the URL;
the sort is also remembered per browser. `New person` asks for the name in a
dialog and opens the new person's page. Organizations keep the table.

| Rejected                             | Why                                                                        |
| ------------------------------------ | -------------------------------------------------------------------------- |
| Cards inside `AdminTablePage`        | The design system keeps cards out of that skeleton                         |
| A virtualized grid                   | Library card grids are not virtualized, and the list pages 100 at a time   |
| Row actions on a card                | A card is one link; the person page holds the actions with room to confirm |
| Portrait, square or horizontal cards | A round face shows more people per row and reads as a person, not a video  |

## The person page

`/people/:id` shows one person in a `ProfilePage`
([design-system.md](design-system.md#page-patterns)): a profile spread across
the page, then the person's videos. The spread holds the face; the name and
aliases; the key figures (age, height, measurements with the cup, career) as
large numbers; the bio; the actions; and a card at the end with the remaining
facts (dates, gender, birthplace, blood type), links and external IDs. From
`lg` that card stops at the face's height and scrolls inside, so a person with
many links does not push the videos down. The actions are `secondary` buttons
with icons (`Edit`, `Fetch info…`, `Show in library`, `Faces`), with merging
and deleting in the `More` menu. The person is seen before their videos, as in
other media libraries' person pages.

The videos follow as one `Shelf` per role the person has videos in, in the
roles' order: the role and its count over a grid of every video in that role,
which wraps like the library and reads the next page as its end comes into
view. The page reads the person with `GET /api/people/{id}` and the videos with
`GET /api/videos?person={id}`, in the library's default order; each shelf adds
`personRole`, so totals and paging stay the server's
([library-ui.md](library-ui.md)). A role without videos draws no shelf, and a
person without videos shows one empty state. The page is owner-only, as the
rest of `/people`.

| Rejected                                   | Why                                                                                     |
| ------------------------------------------ | --------------------------------------------------------------------------------------- |
| A sticky profile column beside the videos  | The facts were squeezed under a large face and scrolled out of view inside their column |
| One grid with a role filter as the default | It hides that a person both performs and directs until each role is pressed             |
| A sideways row per role with `See all`     | Most of a row was off the screen, and reaching the rest took another press              |

The face, its upload and removal, the favorite heart, merging and deleting
happen on the page. Each request that answers `404 person_not_found` turns the
page into its gone state; a merge opens the target's page and a delete the
people list, each with a toast. The person's own fields are edited on
`/people/:id/edit`.

A person plugin fills or replaces the person's fields from the page: the apply
writes the profile values and face the owner selects, appends missing aliases
and external IDs, and never merges the person with the holder of a fetched ID
([person-plugins.md](person-plugins.md#a-fetched-person-applies-per-item-and-a-profile-value-may-replace)).
`Fetch info…` after `Edit` opens that fetch, shown only while an approved
person plugin exists
([person-plugins.md](person-plugins.md#the-person-page-compares-a-fetched-person-before-applying-it)).

### The edit page

`/people/:id/edit` is a `FormPage` outside the shell that edits the name,
aliases, external IDs, links and every profile field together, built and left
as the video details editor is
([library-ui.md](library-ui.md#details-editor)): the same band, the same
`Discard changes?` on `Cancel`, the aside's link and the browser's Back, and
the same return along the history when the person page opened it. Organizations
keep the details dialog.

`Save` sends one `PATCH /api/people/{id}` with only the properties that differ
from the person as loaded, so a change made elsewhere to another property
survives; an edited list replaces the whole list, and an emptied field is sent
as `null`. With nothing changed, `Save` returns without a request. The
response goes back to the person page in memory, so it shows the saved person
without reading it again; opened by its URL, the page reads the person first.
A reason with a `field` stays under that input with every other input kept.
Number inputs keep only digits, so a value the server reads is always a whole
number or empty.

| Rejected                         | Why                                                                                |
| -------------------------------- | ---------------------------------------------------------------------------------- |
| A dialog per group of fields     | The owner had to reopen the editor for each change                                 |
| One `FormDialog` for every field | More than a dozen fields break that skeleton's rule of a few fields for one action |

### Ages are computed in the browser

The API returns dates, not ages, because an age changes at midnight in the
viewer's time zone and a cached response would show yesterday's age. The page
computes the age on the viewer's local date, or on the death date when there
is one ([`personFacts.ts`](../../web/src/credits/person/personFacts.ts)). A
partial date stands for every day it allows, so the page shows the youngest
and oldest ages the two dates allow, such as `1990 (age 34–35)`, and one age
when both are equal.

| Rejected                    | Why                                              |
| --------------------------- | ------------------------------------------------ |
| `age` in the response       | Stale after midnight in a cached response        |
| No age at partial precision | A year alone still narrows the age to two values |

### Candidates and faces in videos

A person's face-recognition results have their own page,
`/people/:id/faces`, a `RecordPage` with `Back to {name}`, titled `Faces` with
the person's name under it. On the person page they were long lists that
pushed the profile and the videos aside, while they are work the owner does
now and then. The person page keeps only a `Faces` action after
`Show in library`, which counts the pairs waiting for a decision
(`Faces, 2 to review`) from `GET /api/people/{id}/face-candidates`. The action
shows only while `GET /api/settings/face-recognition` reports face recognition
on; while it is off, or the settings cannot be read, the page neither shows it
nor reads the candidates.

The faces page puts its two lists in `line` tabs, `Same person?` and
`Faces in videos`, each with its count, `Same person?` counting only the pairs
not yet decided on the page, and keeps the tab in the address
(`?tab=candidates` or `?tab=faces`); without one it opens `Same person?`
unless no pair waits at the first read, a choice a later `Refresh` never
changes. A `Refresh` button after the tabs reads both lists again. Pairs that
cannot be read say so with `Retry` in place of the rows.
Nothing else reads them again: a decided pair and a split face stay where they
are, dimmed with the result and `Undo` where one applies, so the next row never
moves under the owner's press. Before, every decision read the lists again,
which put the faces back to their first page and reordered the pairs.

`Same person?` is one row per pair: the shown person's `PersonFace judge` and
the other person's side by side, each over the name, the face count and up to
four of their crops, with the similarity between them (under them below `sm`),
then `Not the same` and `Same person`. One face alone is a poor basis for the
decision, so the crops are the first four with a crop among each person's
first twelve appearances (`GET /api/people/{id}/appearances?limit=12`). A confirm keeps the shown person
([The owner settles a candidate pair](face-recognition.md#the-owner-settles-a-candidate-pair))
and the row reads `Merged into {name}`, with `Undo` when the merged person had
no name, which takes the merge back and offers the pair again; a retry after
the merge was undone but the pair not reopened only reopens it. Only an unnamed
person confirmed with a named one is merged away, and the page then opens the
named person's faces page with a `Merged into "{name}"` toast whose `Undo`
takes the merge back and returns to the unnamed person's page. A dismissed row reads `Not the same` with `Undo`, which reopens the pair.
A pressed row disables only its own buttons.

`Faces in videos` is a `CardGrid size="face"` of `PersonFace scene` crops,
each over the video's title and the start of each span, each a link that plays
the video from that time (`?t=`); the crop and the title open the video at the
first span. A select above the grid orders it `Least like this person first`
(the default) or `Newest video first`, and each crop shows its similarity to
the person in its `PersonFaceMark` `bottom-end`, in `warning` when the
appearance is `unlike`: below its recognizer's candidate threshold, where a
face is as unlike the person as a stranger usually is. The default order puts the faces most likely linked by mistake
first, so the owner finds them without paging through the person's own
faces. `Show more` reads the next page; with none it says so. A split or its
`Undo` rescores the person's other faces, so the cursor no longer marks where
the shown faces end; the next `Show more` reads the rest whole and adds only
the faces not yet shown. Changing the order clears the selection, since the
new order starts from its first page, and keeps the faces split on the page
first in the grid with their `Undo` until a `Refresh`. An appearance
whose video left the library reads `Not in the library` without links. A grid
shows many faces at once, so a face that is not the person stands out, where a
list of rows led with titles.

Each crop carries a `Not this person` icon button in its `PersonFaceMark`
`top-end`, for a face the analysis linked to the wrong person. It splits the
face into a new unnamed person with no confirmation: the tile stays, dimmed,
with `Split into #12` and `Undo`, which merges the new person back into the
shown one (`POST /api/people/{id}/merge`); a person who had no face is left
without one, rather than keep the face the merge brings, and an `Undo` whose
split person is gone reports it and keeps the tile split. `Select` replaces
each button with a checkbox; with faces checked, a `SelectionBar` offers
`Not this person`, which splits them all into one new person
(`POST /api/people/{id}/appearances/split`), since faces wrongly linked together
are usually one other person, and the `Undo` of any of them takes them all
back. A selection holds at most 200 faces, what one split or move takes; past
that the other checkboxes are disabled and the bar says so. `Move to person…`
in the same bar opens a dialog that picks where the checked faces belong:
before a name is typed it lists, as a `RadioGroup`, the people whose faces look
most like them (`GET /api/people/{id}/similar-people`), each with a
`PersonFace suggestion`, up to four more crops from `sm`, the face count, the
video they were first seen in and the similarity, so even people without a name
are told apart; typing searches by name as the merge dialog does. `Move` hands
the faces and their credits to that person
(`POST /api/people/{id}/appearances/move`), and the tiles read
`Moved to {name}` with an `Undo` that moves back the faces of that move alone,
even when another move went to the same person. A pressed tile disables only
its own button, a second press while it runs sends nothing, and a failure shows
under the grid.

| Rejected                                   | Why                                                                               |
| ------------------------------------------ | --------------------------------------------------------------------------------- |
| Candidates and faces on the person page    | They crowded out the profile and the videos, which the page is for                |
| A strip of crops alone                     | Reaching a scene would take a second press, and a crop does not say which video   |
| Confirming in the people list              | A pair is judged with both faces and the similarity in view, one person at a time |
| Removing a decided row or a split tile     | The rows after it moved up under the pointer, and a misclick had no way back      |
| A name search alone to pick where faces go | An unnamed person has no name to search, and a name says nothing about the face   |

## Faces elsewhere on the screens

On the video page, each person in the work details row is a `PersonFace chip`
and the name inside one link to `/people/:id`, named `Open {name}`, so the
cast opens the person page instead of the filtered library; the chip sits on
the name's line and keeps the row's height. Organizations keep their text links
to the filtered library, and the fold after three names is unchanged.

Wherever a person is chosen, the face comes with the name: a picker's
candidate rows lead with `PersonFace option` (the video details editor, the
merge dialog and a smart folder's `Person` condition), and a chosen person's
chip or filter leads with `PersonFace token` (the details editor's credit chips,
the smart folder condition and the library's active person filter). A person
without a face shows the placeholder in the same frame, so the rows keep one
height. The chip carries the person's `imageRevision` from the candidate or the
video's `WorkPerson`, so a face picked in the editor shows before the save.

In the comparison of a work-info fetch
([work-info-plugins.md](work-info-plugins.md#peoples-images-are-fetched-like-the-cover-and-become-faces-after-the-commit)),
a fetched person with a kept image shows `PersonFace compare` before the name,
and each fetched person with a profile adds one muted line `Fills only empty
fields: …` under the chips, because the apply never replaces a stored value.
Three lines show at first and the rest fold behind `+N more`, which shows them
all when expanded, so the chips stay the primary content of the row.

## Provisional people on the screens

A provisional person
([A person without a canonical name is provisional](#a-person-without-a-canonical-name-is-provisional))
is drawn as any person, with their id as `#1042` in `muted-foreground` where
the name goes: the grid card, the person page's `h1` and document title, the
video page's cast chip, the details editor's chips and the library's person
filter chip. Every unnamed person read `Unnamed person` before, which a card
truncated and which told no two of them apart; the id is short, unique and
does not change when other people are merged or deleted. The quieter name line alone separates an unnamed card from a named one,
so the faces stay what the eye scans; a badge on every unnamed card was
rejected for competing with them. The screens map the flag to the words in one
place, [`displayName.ts`](../../web/src/credits/displayName.ts), so the empty
`name` never reaches a screen.

The grid's filter adds a `Name` select (`Any`, `Named`, `Unnamed`) that sends
`provisional=false` or `true` and is kept in the URL as `named=1` or `named=0`.
A provisional person's page offers `Name…` in place of `Edit`; the edit page
shows the empty name with the person's number as its placeholder, and `Save` with the
name still empty sends no `name`, so the person stays provisional.

The `Performers` picker of the details editor lists provisional people under
`Unnamed people` (`GET /api/people?provisional=true` with the picker's limit).
Choosing one while a name is typed adds a chip with that name, and `Save`
sends that name by `PATCH /api/people/{id}` before it creates new people or
writes the credits, so a refused name stops the save before the video changes.
