# 契約: `sdd-guard.sh`（ガード）

**Feature**: [spec.md](../spec.md) | **Data model**: [data-model.md](../data-model.md) | **Research**: [R-002](../research.md)

場所: `.claude/skills/sdd-next/scripts/sdd-guard.sh`

## 呼び出し

```bash
sdd-state.sh | sdd-guard.sh [--repo <owner/name>] [--root <repo_root>]
```

| 引数 | 既定 | 意味 |
| --- | --- | --- |
| stdin | 必須 | `sdd-state.sh` の出力（自動選択の結果） |
| `--repo` | `git remote get-url origin` から導出 | GitHub の `owner/name` |
| `--root` | カレントディレクトリ | `sdd-state.sh --feature` を呼び直すときに渡す |

## 出力

標準出力に JSON 1 行（[data-model.md 6.](../data-model.md)）。`state` は対象機能を確定し直した
後の State で、呼び出し側はこちらを以後の真実として使う。

依存: bash、`gh`（`gh api` のみ）、`jq`、同じディレクトリの `sdd-state.sh`。

## 終了コード

| コード | 条件 |
| --- | --- |
| 0 | 判定できた（`go` の真偽を問わない） |
| 2 | stdin が JSON でない、引数の誤り |

`gh` が無い・REST が失敗した場合も終了コード 0 で `{"go":false,"reason":"gh-unavailable",...}` を
返す（手元で検算できるようにするため。spec の Edge Cases）。

## 手順（この順で最初に該当したものを返す）

| # | 判定 | REST | 結果 |
| --- | --- | --- | --- |
| 0 | `gh api /rate_limit` が通るか（疎通） | `GET /rate_limit` | 失敗 → `gh-unavailable` |
| 1 | **対象機能の確定**: マージ済み `sdd` PR を更新日時の降順で 1 件取り、変更ファイルから `specs/NNN-*/` を抽出。見つかれば `sdd-state.sh --feature` で再判定 | `GET /repos/{o}/{r}/pulls?state=closed&base=main&sort=updated&direction=desc&per_page=100`（`merged_at != null` かつ label `sdd`）→ `GET /repos/{o}/{r}/pulls/{n}/files` | `state` を置き換える。見つからなければ stdin の state のまま |
| 2 | `state.stage` が `done` または `none` | — | `go:false`, `reason:"nothing-to-do"` |
| 3 | **冪等**: open PR のうち `head.ref` が `claude/sdd-NNN-` で始まるものがある | `GET /repos/{o}/{r}/pulls?state=open&base=main&per_page=100` | `go:false`, `reason:"open-pr"`, `open_prs:[...]` |
| 4 | **フェーズ別リトライ**: `stage = implement` で、`head.ref = claude/sdd-NNN-implement-pN` のマージ済み PR が 2 件以上 | 1 で取得済みの一覧を再利用 | `go:false`, `reason:"phase-retry-limit"` |
| 5 | **ホップ上限**: `head.ref` が `claude/sdd-NNN-` で始まるマージ済み PR が `2 + phases + 2` 件以上 | 同上 | `go:false`, `reason:"hop-limit"` |
| 6 | 上記に該当しない | — | `go:true` |

`phases` は `state.phases`（無ければ 0）。

## 注意

- 高水準の `gh pr list` / `gh issue list` は使わない。プロキシが GraphQL を制限するため（R-002）
- `per_page=100` を超える件数は想定しない（1 機能あたりのホップは最大でも十数件）
- 出力の `open_prs` は `head.ref` の配列。空なら `[]`

## 例

```bash
$ sdd-state.sh | sdd-guard.sh
{"go":true,"state":{"feature_dir":"specs/002-core-video-library","feature":"002","stage":"plan","branch":"claude/sdd-002-plan"},"hops":0,"phase_retries":0,"open_prs":[]}

$ sdd-state.sh | sdd-guard.sh     # 手元（gh 無し）
{"go":false,"reason":"gh-unavailable","state":{"feature_dir":"specs/002-core-video-library","feature":"002","stage":"plan","branch":"claude/sdd-002-plan"}}
```

## テスト

自動テストは「`PATH` から `gh` を外した状態で `gh-unavailable` を返し、`state` を素通しする」
ことだけを確認する。REST を伴う判定は quickstart の S3〜S6 で実機確認する。
