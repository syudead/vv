# 依存の更新（Renovate）

依存の更新 PR は [Renovate](https://docs.renovatebot.com/) の GitHub App が
`renovate.json` に従って作る。設定の意図はこの文書に置き、`renovate.json`
自体には書かない。

## 何が起きるか

- 毎週月曜の早朝（JST）に、Go modules / web npm / tools npm / GitHub Actions /
  mise tools / container images の 6 グループに分けて PR が出る。脆弱性対応の
  PR は曜日を待たずに出る。
- マイナー・パッチ・lockfile 保守・ダイジェスト更新は、`main` の必須チェック
  （CI の 3 ジョブ）が通れば Renovate が自動でマージする。
- メジャー更新は PR が残る。破壊的変更を読んで人がマージする。
- GitHub Actions はコミットハッシュに固定され、コメントでタグ名を併記する
  （`config:best-practices` の既定）。

## 対象外にしているもの

- `Dockerfile` の `golang` / `node` イメージと `mise.toml` の `go` / `node`。
  ランタイム版は `go.mod` の `go` 行と揃える必要があるので、人が
  `go.mod` を上げるときに一緒に変える。mise manager は `go` を `golang/go`、
  `node` を `nodejs` という packageName で扱うため、除外は `matchPackageNames`
  ではなく両 manager に共通の `matchDepNames` で指定している。
- `mise.toml` の `task` と `jq`、`Dockerfile` の `alpine` は対象内で、それぞれ
  mise tools / container images グループに入る。
- Dependabot の security updates はリポジトリ設定で無効にしている。
  Renovate の `vulnerabilityAlerts` が同じ役割を果たす。

## Renovate の PR に対する扱い

- Bot の PR には「UI 変更なし」の記載や画像添付を求めない。
- 自動マージが止まっている PR は、CI の失敗か、コンフリクトか、メジャー更新
  のどれか。Renovate の Dependency Dashboard Issue に一覧が出る。
- 更新を一時的に止めたいときは、PR を閉じる（同じ版は再作成されない）か、
  `renovate.json` の `packageRules` で `enabled: false` にする。

## 設定を変えたら

`npx --package renovate renovate-config-validator` で検証してから push する。
