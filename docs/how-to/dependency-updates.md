# 依存の更新（Renovate）

依存の更新 PR は [Renovate](https://docs.renovatebot.com/) の GitHub App が
`renovate.json` に従って作る。設定の意図はこの文書に置き、`renovate.json`
自体には書かない。

## 何が起きるか

- 毎週月曜の早朝（JST）に、Go modules / web npm / tools npm / GitHub Actions /
  mise tools / container images の 6 グループに分けて PR が出る。脆弱性対応の
  PR は曜日を待たずに出る。
- マイナー・パッチ・lockfile 保守・ダイジェスト更新は、PR の必須チェック
  （Checks）が通れば Renovate が自動でマージする。Browser E2E と Docker image は
  マージ後の `main` への push で回る。
- メジャー更新は PR が残る。破壊的変更を読んで人がマージする。
- GitHub Actions はコミットハッシュに固定され、コメントでタグ名を併記する
  （`config:best-practices` の既定）。
- `Dockerfile` のベースイメージは `タグ@sha256:ダイジェスト` で固定する。同じ
  タグの中身が更新されると、Renovate がダイジェスト更新の PR を出す。

## 対象外にしているもの

- `Dockerfile` の `golang` / `node` イメージと `mise.toml` の `go` / `node`。
  ランタイム版は `go.mod` の `go` 行と揃える必要があるので、人が
  `go.mod` を上げるときに一緒に変える。mise manager は `go` を `golang/go`、
  `node` を `nodejs` という packageName で扱うため、除外は `matchPackageNames`
  ではなく両 manager に共通の `matchDepNames` で指定している。
  止めるのはバージョンの変更（major / minor / patch）だけで、`Dockerfile` の
  イメージのダイジェスト更新は対象内に残す。
- `mise.toml` の `task` と `jq`、`Dockerfile` の `alpine` は対象内で、それぞれ
  mise tools / container images グループに入る。
- Windows 版の zip に同梱する FFmpeg（`scripts/build/windows_app.go` の
  `ffmpegVersion` と `ffmpegSHA256`）。Renovate はこの定数を読まないので、
  人が下の [FFmpeg の版の上げ方](#ffmpeg-の版の上げ方)で上げる。
- Dependabot の security updates はリポジトリ設定で無効にしている。
  Renovate の `vulnerabilityAlerts` が同じ役割を果たす。

## FFmpeg の版の上げ方

Windows 版の zip は、`GyanD/codexffmpeg` の GitHub Releases にある Gyan.dev の
essentials（`ffmpeg-<版>-essentials_build.zip`）を版と SHA-256 で固定して同梱する
（[Windows デスクトップ版の配布](../design-docs/windows-app.md#配布)）。上げるときは次を 1 つの PR で行う。

1. 新しい版の Release に `ffmpeg-<版>-essentials_build.zip` があることを確かめ、その
   SHA-256 を Gyan.dev が公開する値（<https://www.gyan.dev/ffmpeg/builds/> の
   `.sha256`）で確かめる。
2. `scripts/build/windows_app.go` の `ffmpegVersion` と `ffmpegSHA256` を変える。
   SHA-256 が合わなければ `task build-windows-app` は期待値と実際の値を示して失敗する。
3. `task build-windows-app` で zip を組み、中身の `ffmpeg/README.txt` の版を確かめる。
4. マージ後に `Windows app` workflow を手動実行し、同梱の `ffmpeg` に `h264_nvenc` と
   `h264_qsv` があることを確かめる。

`ffmpeg` の中のエンコーダの名前や引数が変わったときは、
[hardware-encoding.md](../design-docs/hardware-encoding.md) と `internal/media` も合わせる。

## Renovate の PR に対する扱い

- 自動マージが止まっている PR は、CI の失敗か、コンフリクトか、メジャー更新
  のどれか。Renovate の Dependency Dashboard Issue に一覧が出る。
- 更新を一時的に止めたいときは、PR を閉じる（同じ版は再作成されない）か、
  `renovate.json` の `packageRules` で `enabled: false` にする。

## 設定を変えたら

`npx --package renovate renovate-config-validator` で検証してから push する。
