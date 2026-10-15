---
source: specs/054-auto-dup/plan.md
sourceHash: 7038d2632ab409d2803dd76661524191c4b5bfc95c4e5b0f3259dbc37555d417
---

# 実装計画: 重複の候補を自動で判定し、同じものをまとめる {#implementation-plan-judging-possible-duplicates-automatically-and-bundling-the-same-ones}

**ブランチ**: `feature/054-auto-dup` | **親 Issue**: #1141

**入力**: 親 Issue。これがこの機能の仕様である。

## 概要 {#summary}

ワーカーは重複の候補ごとに、2 つのファイルが同じ映像を映しているかを分類器のポートに問う。「同じ動画」なら所有者がまとめるのと同じようにまとめ、「別の動画」なら退け、それ以外は Duplicates のページに残る。動画のカードにはバージョンの印が付き、その印は一覧の項目が新たに持つ `versions` の要約から読む。

図は、1 つの候補が指紋から結果に至るまでを示す。

```mermaid
flowchart LR
  fp[指紋のジョブ] --> cand[(候補)]
  cand --> worker[判定のワーカー]
  worker -->|4 組のフレーム| port[分類器のポート]
  port -->|確率| worker
  worker -->|0.8 以上| bundle[まとめる]
  worker -->|0.2 以下| dismiss[別の動画]
  worker -->|その間、またはエラー| list[Duplicates に残る]
```

| 関心事 | 方針 |
| --- | --- |
| モデル | `app.Classifier` を通した、はいかいいえの 1 つの質問 ([R-1](research.md#r-1-one-yesno-question-through-the-existing-classifier-port)) |
| 入力 | 4 枚の画像。それぞれ A のフレームの横に、同じ位置の B のフレームを置く ([R-2](research.md#r-2-the-images-are-four-side-by-side-frame-pairs)) |
| 判定 | 0.8 以上で同じ、0.2 以下で別、固定 ([R-3](research.md#r-3-the-question-wording-and-two-fixed-thresholds)) |
| キュー | 候補の行にある判定の列。判定が null の候補が待つ ([R-4](research.md#r-4-the-judgement-is-columns-on-the-candidate-row)) |
| 失敗 | 10 分、1 時間、6 時間の後に再試行し、その後は毎日 ([R-5](research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends)) |
| 衝突 | 退けた組か、まとまりから外した組をまたぐまとめはしない ([R-6](research.md#r-6-a-same-pair-joins-two-sides-unless-a-pair-across-them-is-known-to-differ)) |
| 代表 | まとまりの代表、なければ画素の多い方 ([R-7](research.md#r-7-the-representative-is-the-bundles-else-the-larger-picture)) |
| バージョンを外す | 組を記録し、二度と判定せず、候補を作り直す ([R-8](research.md#r-8-removing-a-version-records-the-pairs-and-rebuilds-its-candidates)) |
| カードの印 | 画面の API のすべての `Video` に `versions`、ページごとに 1 つのクエリ ([R-9](research.md#r-9-list-items-carry-the-existing-versions-summary-read-per-page)) |
| Duplicates のページ | 新しい `candidates` の通知で取得し直す ([R-10](research.md#r-10-the-duplicates-page-refreshes-on-a-candidates-notification)) |
| モデル | `clef-flash` を既定のままにする。まとめるための `clef` を Settings が示す ([R-11](research.md#r-11-the-default-model-is-left-as-it-is-and-settings-names-the-one-that-bundles)) |

## 技術的な文脈 {#technical-context}

**正本の定義**:

| 項目 | 出典 |
| --- | --- |
| 境界、ストアの役割、コミットの後のイベント | [ARCHITECTURE.md](../../ARCHITECTURE.md)、[.golangci.yml](../../.golangci.yml) (depguard) |
| 指紋、候補、退けた組、まとまり、`Unbundle` | [030 データモデル](../030-video-versions/data-model.md)、[030 の調査](../030-video-versions/research.md) |
| 分類器のポート、Ollama のプロバイダとその設定 | [ai-layer.md](../../docs/design-docs/ai-layer.md) |
| キューと単一のワーカーの形、モデルの入力としてのシーク用のフレーム | [auto-tagging.md](../../docs/design-docs/auto-tagging.md) |
| 画面の API と生成コード | [api/openapi.yaml](../../api/openapi.yaml)、`task generate` |
| 画面と Web のテスト | [design-system.md](../../docs/design-docs/design-system.md)、[web-testing.md](../../docs/design-docs/web-testing.md)、[i18n.md](../../docs/design-docs/i18n.md) |
| 検査 | [Taskfile.yml](../../Taskfile.yml) (`task check`、`task check-docs`) |

**この機能に固有の文脈**:

- しきい値は合成の試行に基づく。[クイックスタート](quickstart.md) は、ワーカーの単位がマージされる前に、それを実際のファイルで繰り返す。
- 既定のモデルである `clef-flash` では、同じ動画の組は 0.29 で、判定できないままである。そのため、受け入れ条件 1 は `clef` でだけ成り立つ (R-11)。
- ほかの feature ブランチはマイグレーションを加える。この機能のマイグレーションは、その単位を書く時点の `main` の次の番号を取る。

## 憲章の確認 {#constitution-check}

| 規則 | 判定 |
| --- | --- |
| 依存の方向 (ARCHITECTURE.md) | 合格: ワーカーは `internal/app` にあり、自分が宣言するインターフェースの背後にある。フレームの組は `cmd/mdm` の画像の取得元のアダプターを通して `internal/artifacts` から来る。どのアダプターもほかのアダプターを import しない |
| ドメインが決め、ストアが強制する | 合格: 判定のしきい値、衝突の確認、代表の規則、再試行の時刻は `internal/domain` にある。ストアはまとめるトランザクションの中で、メンバー、退けた組、まとまりから外した組を読み直す |
| メディアフォルダのファイルを読むのはスキャンだけである | 合格: モデルが見るのは、データディレクトリの下にあるシーク用プレビューのフレームである (R-2) |
| ユーザーデータは作り直しを越えて残る | 合格: `video_version_unbundles` は `videos` への外部キーを持たずに内容の鍵をキーとし、ユーザーデータとして一覧に載る |
| イベントはコミットの後 | 合格: まとめると、今と同じく `VideoBundleChanged` を発行する。すべての判定の結果とバージョンからの取り外しは、コミットの後に `VersionCandidatesChanged` を発行する |
| 生成ファイルを手で編集しない (AGENTS.md) | 合格: `api/openapi.yaml` を変え、続いて `task generate` を実行する |
| 複雑度は 25 以下、コードを写さない (AGENTS.md) | 設計上は合格: 自動のまとめは `VersionStore.Bundle` の手順を、退ける処理は `Dismiss` の手順を、写しではなく再利用する |
| 文書は変更とともに (core-beliefs.md) | 合格: ワーカーの単位が設計文書を加え、ai-layer.md、ARCHITECTURE.md、running-vv.md を更新する |

## プロジェクトの構成 {#project-structure}

### 文書 (この機能) {#documentation-this-feature}

```text
specs/054-auto-dup/
├── plan.md
├── research.md            # R-1..R-11
├── data-model.md          # Two tables, the rules, the setting
├── quickstart.md          # The real-model check
└── contracts/
    └── screen-api.md      # Video.versions on lists, the setting resource
```

`design` 段階が、カードの印と Settings のセクションのための `ui-design.md` を加える。

### ソースコード {#source-code}

**影響する境界**:

| パッケージ | 変更 |
| --- | --- |
| `internal/domain` | 判定、衝突の確認、代表の選択、再試行の時刻 |
| `internal/store` | マイグレーション、判定の取得と結果、`VersionCandidatesChanged`、`Unbundle`、引き継ぎでのキーの付け替え、設定、一覧の動画の `versions` |
| `internal/artifacts` | 2 つのシーク用プレビューから作る、横に並べたフレームの組 |
| `internal/app` | `VersionJudge`: 設定、ワーカー、起こす処理 |
| `internal/httpapi`、`api/openapi.yaml` | 設定のリソース。一覧の項目の `versions`。`candidates` の通知 |
| `cmd/mdm` | `events.go` でのワーカーとそれを起こす処理の配線 |
| `web/src/settings`、`web/src/videoList`、`web/src/versions` | Settings のセクション。カードの印。Duplicates のページの取得し直し |

**新しいパス**: `internal/app/version_judge.go`、`internal/store/version_judgements.go`、`docs/design-docs/duplicate-judging.md`。

**構成の判断**: ワーカーは `jobs` のジョブの種類ではなく、`AutoTagger` と並ぶ独自のユースケースである。判定は組についてのもので、ジョブのキューの取得と一意性の規則は動画ごとだからである。

## 実装作業 {#implementation-work}

図は、どの単位が先に入る必要があるかを示す。画面の単位は `design` 段階の `ui-design.md` も待つ。[クイックスタート](quickstart.md) の実際のモデルでの実行は、スイッチと印が要るので、最後の単位のものである。

```mermaid
flowchart LR
  store[判定を保存する] --> worker[判定のワーカー]
  store --> refresh[Duplicates の更新]
  mark[カードのバージョンの印]
  worker --> settings[Settings のスイッチ]
  mark --> settings
```

### 重複の判定と、バージョンから外した組を保存する {#store-duplicate-judgements-and-the-pairs-removed-from-versions}

**範囲**: 候補の列と `video_version_unbundles` を持つマイグレーション。ドメインの規則 (判定、衝突の確認、代表、再試行の時刻) と `VersionCandidatesChanged`。ストアの取得、回復、衝突の確認付きの 3 つの結果。`version_judging.enabled` の設定の読み書き。組を記録して候補を作り直す `Unbundle`。`video_version_unbundles` のキーを付け替える引き継ぎ。ARCHITECTURE.md と running-vv.md のユーザーデータの一覧 ([data-model.md](data-model.md)、[R-4](research.md#r-4-the-judgement-is-columns-on-the-candidate-row) から [R-8](research.md#r-8-removing-a-version-records-the-pairs-and-rebuilds-its-candidates))。

**依存**: なし。

**受け入れ**: `task check` と `task check-docs` が通る。ストアのテストが次を示す: 「同じ」の結果は組を R-7 の代表でまとめ、候補はなくなる。新しい動画とまとまりのメンバーについての「同じ」は、それをそのまとまりに加え、そのまとまりを分けて手作業でもう一度まとめた後でも加える。「別」は退けた組を書き、候補を取り除く。退けた組をまたぐ「同じ」は、候補を `undecided` のまま残す。`video_version_unbundles` にある組か `undecided` の組は取得されない。`failed` の行は、その `retry_at` の後にだけ取得され、その `attempts` は回復を越えて残る。`Unbundle` は組を書き、一致する候補はまた一覧に出る。各結果は `VersionCandidatesChanged` を発行する。

### 分類器で重複の候補を判定する {#judge-possible-duplicates-with-the-classifier}

**範囲**: `internal/artifacts` の横に並べたフレームの組。`app.VersionJudge` (設定がオンの間 1 つずつ取得し、R-3 の質問をし、結果を適用し、次の `retry_at` まで待つ)。各組の確率を記すログの行。配線と起こす処理 (起動時、`ProcessingChanged`、`VersionCandidatesChanged`、設定の保存)。索引からリンクする `docs/design-docs/duplicate-judging.md`。AI 層とサブシステムの地図の項目 ([R-1](research.md#r-1-one-yesno-question-through-the-existing-classifier-port) から [R-5](research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends))。

**依存**: 重複の判定と、バージョンから外した組を保存する。

**受け入れ**: `task check` と `task check-docs` が通る。偽の分類器を使うアプリのテストが次を示す: 0.85 はまとめ、0.1 は退け、0.5 は候補を残し、エラーか質問のキーのない応答は再試行の時刻付きの `failed` の行にし、設定がオフの間は何も取得しない。artifacts のテストが、それぞれ 1 フレームの 2 倍の幅を持つ 4 枚の JPEG を示す。

### 候補が判定されたら Duplicates のページを更新する {#refresh-the-duplicates-page-when-candidates-are-judged}

**範囲**: `VersionCandidatesChanged` のための `/api/events` の `candidates` の通知と、それで取得し直す Duplicates のページ ([contracts/screen-api.md、`GET /api/events`](contracts/screen-api.md#get-apievents-candidates)、[R-10](research.md#r-10-the-duplicates-page-refreshes-on-a-candidates-notification))。

**依存**: 重複の判定と、バージョンから外した組を保存する。

**受け入れ**: `task check` が通り、`task generate` で差分が出ない。ハンドラーのテストが、イベントが所有者のイベントの流れに `candidates` として届くことを示す。Web のテストが、開いている Duplicates のページが `candidates` で取得し直し、なくなった組を落とすことを示す。

### 動画のカードにバージョンの印を示す {#show-a-versions-mark-on-video-cards}

**範囲**: まとまりのメンバーについて画面の API のすべての `Video` に入れ、ページごとに 1 回読む `versions`。`api/openapi.yaml` の説明。`ui-design.md` に従ったカードと一覧の行の印。030 ui-design.md の置き換えられるカードの規則 ([contracts/screen-api.md、`Video` delta](contracts/screen-api.md#video-delta)、[R-9](research.md#r-9-list-items-carry-the-existing-versions-summary-read-per-page))。

**依存**: なし。

**受け入れ**: `task check` と `task check-docs` が通る。ストアのテストが、ライブラリ、動画の一覧、フォルダ、グループの読み取りが、まとまりの代表に `versions.count` 2 を与え、まとまっていない動画には何も与えないことを示す。`ui-design.md` の幅での見た目のレビューで、まとめた動画のライブラリ、検索、フォルダ、グループ、ホームのカードは印を示し、普通の動画のカードは示さない。

### 重複の自動判定を Settings でオンとオフにする {#turn-automatic-duplicate-judging-on-and-off-in-settings}

**範囲**: `GET` と `PUT /api/settings/version-judging`、ワーカーを起こす保存、モデルについての注記を持つ、`ui-design.md` に従った Settings のセクション ([contracts/screen-api.md](contracts/screen-api.md)、[R-11](research.md#r-11-the-default-model-is-left-as-it-is-and-settings-names-the-one-that-bundles))。

**依存**: 分類器で重複の候補を判定する。動画のカードにバージョンの印を示す。

**受け入れ**: `task check` が通り、`task generate` で差分が出ない。ハンドラーのテストが次を示す: `GET` は保存の前には `enabled: false` を応答し、`PUT` は保存した値を応答し、所有者のセッションのないリクエストは `401 unauthenticated` を受け取る。Web のテストがスイッチの保存を示す。[クイックスタート](quickstart.md) のすべての行が `clef` で通り、PR は各組の確率を記録する。Settings のセクションが、`ui-design.md` の幅での見た目のレビューに通る。
