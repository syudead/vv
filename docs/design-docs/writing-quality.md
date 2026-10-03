# Writing quality: typed technical English

These rules apply to every repository document: `docs/`, `specs/`, the root
documents, templates, and skills. Each document kind also has a type (a skeleton
with fixed headings), listed in [Document types](#document-types). Plans add
P-1..P-7 from [plan-quality.md](plan-quality.md); Issue specifications follow
[spec-quality.md](../product-specs/spec-quality.md).

Japanese readers use the translation published under `/ja/` on the
documentation site ([docs-site.md](../how-to/docs-site.md)). The
`doc-translator` subagent writes it from the finished English in the same pull
request ([japanese-translation.md](japanese-translation.md)). Nobody edits a
translation by hand; fix the English source and translate it again.

## Rules

### W-1: Documents are in English; Issues and PR bodies are in Japanese

Write every repository document in technical English. Write parent Issues, child
Issues, and pull request bodies in Japanese.

Japanese inside a document belongs in inline code or a code block: quoted screen
text from before the English UI, user data in examples, and Issue section names
such as `要件`. `task check-docs` fails on Japanese anywhere else.

### W-2: Start with the point

The first sentence of a document or section states what it decides or what the
reader can do. Drop preambles.

| Write | Not |
| --- | --- |
| Subtitles are found by reading the video's folder on each request. | This section describes how subtitles are found. |
| Run `task docs` and open the printed URL. | In order to view the site locally, there are a few steps you will need to follow. |

### W-3: No padding

Delete a sentence that the reader loses nothing by skipping:

- restating the heading
- a closing summary of the section above it
- a statement of importance ("It is important to note that")
- a list of what the document will cover

| Write | Not |
| --- | --- |
| The job stops after 5 hours. | It should be noted that, in order to stay within limits, the job is designed to stop after 5 hours. |

### W-4: Say each thing once

Do not restate a point in other words, in the same section or in another
section. Link to the place that says it.

| Write | Not |
| --- | --- |
| Translations live in `translations/ja/` (see the Storage section). | Translations live in `translations/ja/`. In other words, the translated files are kept in a separate directory called `translations/ja/`. |

### W-5: No emphasis without evidence

Do not use intensifiers or marketing words ("critical", "robust", "seamless",
"comprehensive", "powerful", "significantly", "simply", "just") unless a number
or a reason follows in the same sentence. Bold marks a term the reader must not
miss, not a feeling.

| Write | Not |
| --- | --- |
| A 10B model generates 3–5 tokens/s on a 4-vCPU runner, so the first run takes 40 hours. | Larger models are significantly slower and would be a critical bottleneck. |

### W-6: Concrete over abstract

Give the number, the limit, the status code, the setting. Describe present
behaviour in the present tense and the active voice.

| Write | Not |
| --- | --- |
| A subtitle file over 4 MiB is not listed. | Large files are handled appropriately. |

### W-7: Tables and diagrams for structure, prose for reasons

| Content | Form |
| --- | --- |
| Options compared on the same attributes | Table |
| States, cases, or situations and the behaviour in each | Table |
| A mapping (field → meaning, code → message, path → owner) | Table |
| A flow across components, a sequence of calls, a dependency graph, a state machine | Mermaid diagram |
| Steps the reader runs in order | Numbered list |
| Why a choice was made | Prose |

Keep a table cell to one statement. When a cell needs a paragraph, the content
is prose. Every diagram has a sentence before it that says what it shows.

### W-8: One term per concept

Use one English term for one concept across all documents. Product terms use
the words the screen shows, as defined in
[web/src/i18n/en.ts](../../web/src/i18n/en.ts). When a term is introduced,
define it once and link to the definition elsewhere. The
[translation terms](japanese-translation.md#terms) map each term to one
Japanese rendering.

### W-9: Stable headings

A heading is a noun phrase or a decision (`R-3: Glossary and product terms`),
not a question or a sentence fragment. Headings are link targets: changing one
changes its anchor, and the change has to update every inbound link
(`task check-docs` fails on a broken anchor).

### W-10: Write what the code cannot say

A document records what a reader cannot get from the code: the rule, the
reason, the rejected alternative, and the behaviour a user or caller sees. It
does not walk through the code. Name a file or function only to say where a
rule lives, once per section, not to narrate which function calls which.

| Write | Not |
| --- | --- |
| Only files next to the location used for playback are listed ([`mediafs`](../../internal/mediafs)). | `internal/httpapi/subtitles.go` tries the locations in the order `openMediaFile` does. `ListSidecarFiles` in `internal/mediafs` returns the names and sizes of regular files in that location's folder, only when the location opens under the same rules as `OpenMediaFile`. |

### W-11: Length budget

| Unit | Budget |
| --- | --- |
| A decision (`##` section of a design document, `R-N` of research) | One sentence of rule, at most three sentences of reason, one table when there are cases |
| A table cell | One statement |
| A design document | Fits on two screens; split it when it does not |
| A how-to step | One action |

Over budget means the section holds code walk-through (W-10) or a point said
twice (W-4). Cut those before splitting.

## Document types

Each kind has a skeleton. The writing skill copies it, keeps the sections that
apply, and deletes the rest (P-6 in [plan-quality.md](plan-quality.md)).

| Kind | Skeleton | Written by |
| --- | --- | --- |
| Plan | [plan-template.md](../../.agents/skills/sdd-plan/assets/plan-template.md) | `sdd-plan` |
| Research | [research-template.md](../../.agents/skills/sdd-plan/assets/research-template.md) | `sdd-plan` |
| Data model | [data-model-template.md](../../.agents/skills/sdd-plan/assets/data-model-template.md) | `sdd-plan` |
| Contract | [contract-template.md](../../.agents/skills/sdd-plan/assets/contract-template.md) | `sdd-plan` |
| Quickstart | [quickstart-template.md](../../.agents/skills/sdd-plan/assets/quickstart-template.md) | `sdd-plan` |
| UI design | [ui-design-template.md](../../.agents/skills/sdd-design/assets/ui-design-template.md) | `sdd-design` |
| Design document | [design-doc-template.md](../../.agents/skills/sdd-implement/assets/design-doc-template.md) | `sdd-implement`, or any change that adds one |
| How-to guide | [how-to-template.md](../../.agents/skills/sdd-implement/assets/how-to-template.md) | `sdd-implement`, or any change that adds one |
