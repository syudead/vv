---
source: docs/how-to/dependency-updates.md
sourceHash: d2e3056b92d05ea7eca1cbbd58024f0675947f8d72b0cd6a694cba7838efe90f
---

# 依存関係の更新を扱う（Renovate） {#handle-dependency-updates-renovate}

[Renovate](https://docs.renovatebot.com/) GitHub App は `renovate.json` に従って依存関係の更新 PR を開く。その設定の意図はこの文書にある。

## Renovate がすること {#what-renovate-does}

Renovate は、PR を自分でマージするかを更新ごとに決める。開いたまま残した PR は [Renovate のルーチン](#the-renovate-routine) に渡る。

```mermaid
flowchart LR
  upd[更新を検出] --> excl{除外対象?}
  excl -->|はい| none[PR なし]
  excl -->|いいえ| major{メジャー更新?}
  major -->|はい| routine[ルーチンが修正してマージ]
  major -->|いいえ| checks{チェック通過?}
  checks -->|はい| auto[Renovate がマージ]
  checks -->|いいえ| routine
  routine -->|できない| person[needs-human: 人が判断]
```

- PR は毎週月曜の早朝（JST）に 6 つのグループで開く。Go モジュール、web の npm、tools の npm、GitHub Actions、mise のツール、コンテナイメージである。脆弱性修正の PR はいつでも開く。
- マイナー、パッチ、ピン留め、ダイジェストのピン留め（`pinDigest`）、ダイジェスト、ロックファイル保守の更新は、必須の `Checks` ジョブが通ると自動でマージされる。
- Renovate の PR は Browser E2E を実行し、Docker イメージを公開せずにビルドする。その他の PR は、それらを `main` へのマージ後にだけ実行する。
- GitHub Actions はコミットハッシュに固定し、タグ名をコメントに書く（`config:best-practices` の既定）。
- `Dockerfile` のベースイメージは `tag@sha256:digest` として固定する。タグの指す内容が変わると、Renovate がダイジェスト更新の PR を開く。
- `renovate.json` は Renovate のロックファイル生成に使う npm を 11.x に制限するが、この制限はローカル開発と CI には適用されない。npm 12 は現在、ロックファイルのみの更新中に `@tailwindcss/oxide-wasm32-wasi` の tarball で失敗する（[npm issue #9800](https://github.com/npm/cli/issues/9800)）。npm がこの失敗を修正したら、この制限を見直す。

## 除外する依存関係 {#excluded-dependencies}

| 依存関係 | 扱い | 理由 |
| --- | --- | --- |
| `Dockerfile` の `golang` / `node` イメージ、`mise.toml` の `go` / `node` | バージョン更新は除外する。`Dockerfile` のイメージのダイジェスト更新は対象のまま | ランタイムのバージョンは `go.mod` の `go` 行と一致しなければならないため、人がまとめて変える |
| `mise.toml` の `task` と `jq`、`Dockerfile` の `alpine` | 対象。mise のツールとコンテナイメージのグループに入る | — |
| `compose.hosting.yaml` の `ghcr.io/syudead/vv:latest` | 除外 | `latest` は `main` から公開されたイメージに追従する。ダイジェストを固定すると、この動作が止まる |
| Windows の zip に同梱する FFmpeg（`scripts/build/windows_app.go` の `ffmpegVersion` と `ffmpegSHA256`） | 人が上げる（[同梱 FFmpeg のバージョンを上げる](#raise-the-bundled-ffmpeg-version)） | Renovate はこれらの定数を読まない |
| Dependabot のセキュリティ更新 | リポジトリの設定で無効 | Renovate の `vulnerabilityAlerts` が同じ役目を果たす |

除外には `matchPackageNames` ではなく `matchDepNames` を使う。mise マネージャーは `go` を `golang/go`、`node` を `nodejs` と名付ける一方、依存関係名は両方のマネージャーで共通だからである。

## 用途の狭い依存関係 {#dependencies-with-a-narrow-use}

次の直接の依存関係は、それぞれ 1 つのアダプタが使う。Renovate はそれぞれが属するグループと一緒に更新する。表は、どれかが変わったときに確かめることを示す。

| 依存関係 | 使う側 | 更新時に確かめること |
| --- | --- | --- |
| `github.com/coder/websocket` | `internal/comfyui`。ComfyUI の実行のステップ進捗に使う（[045 research、R-5](../../specs/045-comfyui-thumbnails/research.md#r-5-provider-differences-stay-inside-the-adapter)） | `go test ./internal/comfyui`。このモジュールは自身の依存関係を持たないので、更新で依存関係は増えない |
| `golang.org/x/image` | `internal/app` と `internal/media`。サムネイル編集の参照画像と出力を検査し、適用する WebP を収める WebP デコーダーに使う（[thumbnail-editing.md](../design-docs/thumbnail-editing.md#inputs-are-checked-before-anything-is-uploaded)） | `go test ./internal/app ./internal/media`。インポートするのは `webp` パッケージだけである |

## 同梱 FFmpeg のバージョンを上げる {#raise-the-bundled-ffmpeg-version}

Windows の zip は、`GyanD/codexffmpeg` の GitHub Releases から Gyan.dev の essentials ビルド（`ffmpeg-<version>-essentials_build.zip`）を同梱し、バージョンと SHA-256 で固定している（[Windows デスクトップアプリの配布](../design-docs/windows-app.md#distribution)）。1 つの PR で上げる。

1. 新しいリリースに `ffmpeg-<version>-essentials_build.zip` があり、その SHA-256 が <https://www.gyan.dev/ffmpeg/builds/> の `.sha256` の値と一致することを確認する。
2. `scripts/build/windows_app.go` の `ffmpegVersion` と `ffmpegSHA256` を変える。一致しないと `task build-windows-app` は失敗し、期待値と実際の値を表示する。
3. `task build-windows-app` で zip をビルドし、その `ffmpeg/README.txt` のバージョンを確認する。
4. マージ後、`main` での `Windows app` ワークフローの実行が通ることを確認する。これで同梱の `ffmpeg` に `h264_nvenc` と `h264_qsv` があることが確かめられる。

`ffmpeg` がエンコーダー名や引数を変えたときは、[hardware-encoding.md](../design-docs/hardware-encoding.md) と `internal/media` を合わせて更新する。

## 同梱した shadcn スキルを更新する {#update-the-vendored-shadcn-skill}

shadcn CLI は `web/package.json` の devDependency `shadcn` であり、Renovate が web の npm グループと一緒に更新する。`.agents/skills/shadcn/` の shadcn スキルは Renovate が読まない複製なので、1 つの PR で更新する。

1. [shadcn-ui/ui](https://github.com/shadcn-ui/ui) の新しいコミットから `skills/shadcn/` を `evals/` を除いて複製する。
2. `npx shadcn@latest`、`pnpm dlx shadcn@latest`、`bunx --bun shadcn@latest` をすべて `npm --prefix web exec shadcn --` に置き換え、`.agents/skills/shadcn/VENDORED.md` のコミットを更新する。
3. `task test-web` を実行する。`shadcn@` が残っている間、`vendoredSkill.test.ts` は失敗する。

## Renovate のルーチン {#the-renovate-routine}

Claude Code の Routine が、[renovate-triage](../../.agents/skills/renovate-triage/SKILL.md) スキルを毎日 07:47 JST に実行する。ルーチンは、Renovate が自分でマージしない開いた Renovate の PR をすべて扱い、スキルのゲートを通ったものを、メジャー更新も含めて `main` にマージする。

```mermaid
flowchart TD
  start[毎日の実行] --> watch[ルーチンのマージ後の main を確認]
  watch -->|E2E か Docker イメージが壊れた| revert[revert の PR をマージ]
  watch --> list[開いた Renovate の PR]
  list --> kind{状態}
  kind -->|チェックが緑、メジャーでない| skip[Renovate がマージ]
  kind -->|競合| rebase[Renovate がリベース、またはワーカーが main をマージ]
  kind -->|チェック失敗| fix[ワーカーが根本原因を修正]
  kind -->|メジャー| review[ワーカーが破壊的変更を確認してコードを合わせる]
  fix --> next[次の実行がゲートを適用]
  rebase --> next
  review --> gate{ゲート通過?}
  next --> gate
  gate -->|はい| merge[ルーチンがマージ]
  gate -->|いいえ、3 回試した後| human[needs-human ラベル]
```

- PR ごとに 1 つのワーカーが、PR 自身のブランチで修正し、1 回の実行で 1 回 push する。その push で CI が実行され、CI が緑なら翌日の実行が PR をマージする。
- メジャー更新は、ワーカーが破壊的変更の表を PR に投稿した後にだけマージされる。表には、それぞれがリポジトリの何に影響するか、PR がそれをどう扱うか、判定 `safe to merge` を書く。
- ルーチンがマージした更新が `main` で Browser E2E か Docker イメージを壊すと、次の実行がそれを revert する。Renovate が同じバージョンを再び開くと、ルーチンはそれに `needs-human` ラベルを付ける。
- 何かを変えた実行はそれぞれ、したことを Renovate の Dependency Dashboard Issue へのコメントに列挙する。

## 人の対応が必要な Renovate の PR {#renovate-prs-that-need-a-person}

- ルーチンは、3 回試しても緑にならなかったとき、新しいバージョンにこのリポジトリで回避策のないバグがあるとき、または失敗が `main` でも起きるときに、PR に `needs-human` ラベルを付ける。PR のコメントに理由がある。ラベルを外すと、PR はルーチンに戻る。
- 更新を止めるには、PR を閉じる（同じバージョンは再び開かれない）か、`renovate.json` の `packageRules` に `enabled: false` を設定する。

## 設定を変える {#changing-the-configuration}

push の前に `npx --package renovate renovate-config-validator` で検証する。
