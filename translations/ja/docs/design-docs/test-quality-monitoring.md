---
source: docs/design-docs/test-quality-monitoring.md
sourceHash: 4967adda5bcb6aaa6cfc28083d5c446d0285df8ea43588299489e209b7218a27
---

# テストと CI の品質監視 {#test-and-ci-quality-monitoring}

CI はカバレッジとテスト結果を Codecov に送り、`main` への各プッシュはすべての関数の複雑度と重複コードをアーティファクトとして残す。測定は [`ci.yml`](../../.github/workflows/ci.yml)、[`quality-metrics.yml`](../../.github/workflows/quality-metrics.yml)、[`Taskfile.yml`](../../Taskfile.yml) の `quality-metrics` タスクにある。

図は、各測定値の行き先と、それを読む人を示す。

```mermaid
flowchart LR
  push[main へのプッシュ] --> ci[CI のテストグループ]
  push --> qm[Quality metrics]
  pr[プルリクエスト] --> ci
  ci -->|カバレッジ、JUnit の結果| codecov[(Codecov)]
  qm -->|ツールの出力| art[(実行のアーティファクト、90 日)]
  codecov -->|変更行のカバレッジ| prc[PR コメント]
  prc --> author[PR の作成者]
```

## 測定値と保存先 {#measures-and-where-they-are-kept}

各測定値の置き場所は 1 つで、どれも既存のツールの出力だ。

| 測定値 | ツール | 測定のタイミング | 保存先 |
| --- | --- | --- | --- |
| 行カバレッジ（`go` と `web` のフラグ別、ファイル別） | Go の `-coverprofile`、Vitest v8 | `main` へのすべてのプッシュとすべてのプルリクエスト | [Codecov](https://app.codecov.io/gh/syudead/vv) |
| すべてのテストの所要時間と結果、不安定なテスト | gotestsum、Vitest、Playwright の JUnit | 同上。E2E は `main` へのプッシュのみ | Codecov Test Analytics |
| すべての Go の関数の循環的複雑度 | gocyclo | `main` へのすべてのプッシュ | `quality-metrics` アーティファクトの `go-complexity.txt` |
| すべての Web の関数の循環的複雑度 | 最大値を 0 にした ESLint の `complexity` ルール | 同上 | 同じアーティファクトの `ts-complexity.json` |
| 重複コード（場所別） | jscpd | 同上 | 同じアーティファクトの `jscpd/jscpd-report.json` |

アーティファクトは各ツールの出力を書き出されたままの形で、すべての関数と重複箇所のファイルと行とともに保持する。何も要約されず、以前のプッシュと比較されず、アーティファクトの 90 日を超えて保持されない。CI の所要時間は CI 実行の Actions のページで読む。

## カバレッジはプルリクエストに情報を示し、ブロックしない {#coverage-informs-a-pull-request-and-does-not-block-it}

Codecov はプルリクエストに変更行のカバレッジをコメントし、そのステータスは情報提供のみだ（[`.github/codecov.yml`](../../.github/codecov.yml)）。必須のカバレッジの数値は数値のために書かれたテストを招く。[Web のテストレベル](web-testing.md)は、振る舞いが壊れたときに失敗する最も低いレベルを選ぶことで、それを排除している。

## 測定は書き込みトークンなしで動く {#measures-run-with-no-write-token}

`Quality metrics` は npm の依存とリポジトリのコードを実行するので、読み取り専用のトークンを持つ。そうしなければ、同じリポジトリからの依存の更新が、リポジトリにプッシュできるトークンでそのパッケージのインストールスクリプトを実行することになる。

| 採用しなかった案 | 理由 |
| --- | --- |
| `quality-metrics` ブランチに要約した測定値の履歴を残し、前回のプッシュと比較する | 件数や丸めた平均は関数もプルリクエストも示さず、履歴を読む人はいなかった |
| 測定値がしきい値を超えたときに Issue を開く | Issue はメンテナーに判断の材料を何も与えなかった。平均を丸めたことで、誤った悪化も報告された |
| リポジトリ独自の複雑度カウンター | gocyclo は同じ方法で数え、すでに golangci-lint に同梱されている |
| GitHub API から CI の所要時間を集める | Actions のページがすでにすべての実行とジョブの所要時間を示している |
| カバレッジの低下や複雑度の上昇で CI を失敗させる | 必須の数値は数値のために書かれたコードを招く |
| qlty を唯一のツールにする | テストの所要時間も不安定なテストも測定せず、その lint 層は golangci-lint と ESLint と重複する |
