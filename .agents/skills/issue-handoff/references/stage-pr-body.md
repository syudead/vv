# Stage PR body

The `plan` and `design` stages write or revise documents. Their PR body lets a
reviewer see what the documents decide without opening them. The body is in
Japanese ([W-1](../../../../docs/design-docs/writing-quality.md#w-1-documents-are-in-english-issues-and-pr-bodies-are-in-japanese)).
It replaces the sections of
[the repository PR template](../../../../.github/pull_request_template.md) for
these PRs, except `関連 Issue`, which it keeps.

## Sections

Write these sections in this order. Each one is a table; a section with nothing
to say gets one row that says so (`なし`).

| Section | Columns | Content |
| --- | --- | --- |
| `## 段階` | none (one or two lines) | The stage, and the GitHub facts that selected it ([Selecting the stage](README.md#selecting-the-stage)) |
| `## 決めたこと` | `決めたこと`, `採らなかった案`, `理由`, `詳細` | One row per decision in the artifact. `詳細` links the artifact section (`R-3`, a `ui-design.md` heading) |
| `## 範囲` | `区分`, `内容` | Rows for what this PR changes, what the feature covers, and what it leaves out (the parent Issue's `対象外`) |
| `## 検証の方法` | `検査`, `確かめること`, `実行する場所` | Each check named in the artifact: the automated checks, a visual review, a quickstart step |
| `## 決まっていないこと` | `項目`, `決める人`, `時期` | Questions still open, and who closes them when |

A `plan` PR also carries a Mermaid graph of the implementation units and their
dependencies, under `## 範囲`. A `design` PR carries the review widths and the
states it designed, as rows of `## 検証の方法`.

## Example

```markdown
## 段階

`plan`。親 Issue を `Refs` する PR も feature branch も無かった。

## 決めたこと

| 決めたこと | 採らなかった案 | 理由 | 詳細 |
| --- | --- | --- | --- |
| 字幕はフォルダを要求のたびに読む | SQLite に索引を置く | ファイルの追加をすぐ反映できる | [R-1](https://github.com/syudead/vv/blob/main/specs/028-sidecar-subtitles/research.md) |
```
