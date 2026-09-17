# SDD ループハーネス: spec 以降の段階を GitHub イベントで自動で回す

- ステータス: 設計確定（feature branch 方式を実装済み）
- 最終更新: 2026-09-14（レビュー指摘への対応フローを追加）
- スコープ: Spec Kit の `plan → tasks → implement` を spec ごとの feature branch 上で進め、
  plan と最終マージだけを人の承認ゲートにする仕組み


## 0. 2026-09-13 改訂: feature branch を統合単位にする

実運用で、tasks と各 implement phase の PR は機械的な分割境界にすぎず、毎回人が承認する
価値より待ち時間が大きいことが分かった。また phase ごとに `main` へマージすると、未完成な
機能が既定 branch に小刻みに入り、PR を「1 機能を統合する判断」に使えない。そこで次を採用する。

```text
main
 └─ claude/sdd-NNN-feature       （spec ごとの統合 branch）
     ├─ claude/sdd-NNN-plan      ─PR→ feature（人が承認）
     ├─ claude/sdd-NNN-tasks     ─PR→ feature（自動マージ）
     └─ claude/sdd-NNN-implement-pN ─PR→ feature（検査成功時に自動マージ）

claude/sdd-NNN-feature ──────────最終 PR→ main（人が承認）
```

段階ごとに別セッション・別 PR とする性質は、コンテキスト分離、差分の観測、失敗時の停止に
まだ価値があるため残す。外すのは通常時の人手承認だけである。plan は実装方針を固定するため
早期の人間判断に価値があり、最終 PR は完成した機能を `main` に入れる唯一の統合判断なので残す。

検討した「段階 PR 自体を廃止して feature branch に直接 push」は採用しない。PR merge event が
routine の次セッションを起動する durable queue であり、open PR が並行実行の冪等キーにもなる
ためである。また Git は `refs/heads/claude/sdd-NNN` とその子 ref を同時に持てないので、
「孫 branch」は slash の名前ではなく PR の base/head 関係で表す。

この方式には、routine の GitHub event から `Base branch equals main` フィルタを外す外部設定変更が
必要である。設定変更前は plan を feature branch にマージしても次が起動しないため、
[運用手順](../references/sdd-routine.md)の移行チェックを必須とする。

## 1. 目的と前提（改訂前の背景）

これまで Spec Kit の各段階（`/speckit-specify`、`/speckit-plan`、`/speckit-tasks`、
`/speckit-implement`）は、人が web セッションを開いて 1 つずつ呼び、段階ごとに PR を
作ってマージしてきた（#4 spec+plan、#5 tasks、#6 implement、#8 spec）。本設計は
spec 以降の段階について「人が PR をマージする」以外の手作業を無くす。

前提:

| 項目 | 決定 |
| --- | --- |
| 実行環境 | Claude Code on the web のセッションのみ。手元のマシンや GitHub Actions では動かさない |
| 起動契機 | GitHub のイベント（PR のマージ）。Claude Code の routine の GitHub トリガーを使う |
| 1 セッションの仕事 | 次の 1 段階だけ。段階ごとにセッションを分けてコンテキストを区切る |
| PR の粒度 | これまでどおり段階ごとに 1 PR。implement は tasks.md のフェーズごとに 1 PR |
| 承認ゲート | PR のマージそのもの。マージされない限り次の段階は始まらない |
| 状態の真実 | リポジトリの成果物（`specs/NNN-*/` にあるファイルと tasks.md のチェック）から導出する。状態ファイルやラベルを真実にしない |
| 入口 | spec は人が作る（`/speckit-clarify` は人への質問が要るため自律実行に向かない）。spec PR に `sdd` ラベルを付けてマージした時点から自動で回り始める |

非スコープ:

- specify／clarify の自動化。機能説明のバックログ管理も持たない
- 複数機能の並行実行。ハーネスが同時に扱う機能は 1 つ
- レビューそのものの生成（code review は既存の仕組みに任せる。既に付いた指摘への修正は
  `/sdd-next` のレビュー対応フローで扱う）

## 2. 全体の流れ（改訂前。現在は 0 章を優先）

```
人: spec PR を作る → ラベル sdd を付けてマージ
        │ pull_request.closed（merged, base=main, label=sdd）
        ▼
routine「sdd-next」（claude.ai 側。プロンプトは /sdd-next を呼ぶだけ）
        │ 新しい web セッション（main を clone）
        ▼
/sdd-next スキル（.claude/skills/sdd-next/SKILL.md）
  1. 必要なら feature branch を復元し、sdd-state.sh → sdd-guard.sh で進めてよいか判定
  2. open な sdd PR に未解決レビュー指摘があれば、その PR の head に修正 commit を積んで終了
  3. 段階に応じて /speckit-plan | /speckit-tasks | /speckit-implement <フェーズ> を実行
  4. make check（implement のとき）→ コミット → claude/sdd-NNN-<stage> へ push
  5. ラベル sdd 付きの PR を feature branch に開き、checks green 後に自動マージして終了
        │
plan は人がレビューしてマージ、tasks / implement は checks green 後に自動マージ → 先頭に戻る
```

自然な終端は、対象機能の tasks.md に未完了 `- [ ]` が無くなった状態（`done`）。
このとき feature branch から `main` への最終 PR を 1 件だけ開く。最終 PR は人がレビューして
マージする。次の機能の spec PR が `sdd` ラベル付きでマージされれば、また回り始める。

## 3. 部品

判定ロジックはすべてリポジトリ内に置き、claude.ai 側の routine は最小限にする。
壊れたときに読む場所を `.claude/skills/sdd-next/` の 1 箇所に収めるためである。

| 部品 | 場所 | 役割 |
| --- | --- | --- |
| `sdd-state.sh` | `.claude/skills/sdd-next/scripts/` | `specs/*/` のファイルだけを読み、対象機能と次の段階を JSON で出す。git／gh には触れない。手元で実行して検算できる |
| `sdd-guard.sh` | 同上 | 組み込み GitHub ツールが保存した PR 一覧 JSON と git 履歴を見て、無限ループ対策と冪等性を判定し `go` / `stop` を返す。`gh` は手元検算の任意フォールバック |
| `sdd-ui-classify.sh` | 同上 | implement の差分が UI 変更かどうかを、spec の明示分類（`<!-- sdd-ui-change: yes/no -->`）または対象パスから判定する |
| `sdd-next` スキル | `.claude/skills/sdd-next/SKILL.md` | 上記 2 つの結果を受けて、既存 PR のレビュー対応または該当する `/speckit-*` の実行と PR 作成を行う手順書 |
| `sdd-lib.sh` | `.claude/skills/sdd-next/scripts/` | 上 2 つが `source` する共通関数。JSON のエスケープ、機能の列挙、`tasks.md` のフェーズ解析（awk） |
| `rate-limits-statusline.sh` | `.claude/hooks/` | 使用率を受け取るためだけのステータスライン。受け取った JSON を `${TMPDIR:-/tmp}/sdd-rate-limits.json` に落とし、何も表示しない（6 章の使用量ゲート、[R-004](../../specs/003-sdd-loop-harness/research.md)） |
| `.gitattributes` | リポジトリ root | `*.sh` を `eol=lf` に固定する。手元（Windows、`core.autocrlf=true`）で CRLF のスクリプトをチェックアウトすると Git Bash が落ちるため |
| routine | claude.ai（写しを [docs/references/sdd-routine.md](../references/sdd-routine.md) に置く） | GitHub トリガー・日次トリガー・最小プロンプト |

## 4. 状態判定: `sdd-state.sh [--feature specs/NNN-...]`

出力は JSON 1 行:

```json
{"feature_dir":"specs/002-core-video-library","feature":"002",
 "stage":"implement","phase":3,"phase_title":"User Story 1 - ...",
 "remaining":5,"total":12,"phases":6,"branch":"claude/sdd-002-implement-p3"}
```

段階の規則（上から順に最初に当たったもの）:

| 条件 | stage | branch |
| --- | --- | --- |
| `spec.md` が無い | `none`（対象外） | — |
| `plan.md` が無い | `plan` | `claude/sdd-NNN-plan` |
| `tasks.md` が無い | `tasks` | `claude/sdd-NNN-tasks` |
| `tasks.md` に `- [ ]` が残る | `implement`。phase は未完了を含む最初の `## Phase N:` 節。`remaining` / `total` はその節内の `- [ ]` / `- [x]` の数 | `claude/sdd-NNN-implement-pN` |
| 残りなし | `done` | — |

`--feature` 省略時の対象機能: `specs/[0-9][0-9][0-9]-*/` を昇順に走査し、`done` でも
`none` でもない最初の機能。`--feature` は `sdd-guard.sh` が直近のマージ済み `sdd` PR
から対象機能を確定したときに使う。

`stage` が `implement` のときは `remaining` を出力に含める。前進チェック（5. #1）で
「同じフェーズだが一部進んだ」回を前進として扱うためである。

## 5. 無限ループ対策と冪等性: `sdd-guard.sh <state.json>`

1 ホップごとに必ず人のマージが挟まることが第一の歯止めである。その上で、人が機械的に
マージし続けても暴走しないよう、以下を置く。判定の根拠はすべて導出で、状態ファイルは
持たない。

| # | 経路 | 対策 | 根拠 |
| --- | --- | --- | --- |
| 1 | 同じ段階を延々やり直す（例: implement が何も進まないまま PR が出てマージされる） | **前進チェック**: 作業前後の `sdd-state.sh` の出力を比較し、変化がなければ PR を開かず終了。`git diff` が空の場合も同じ | state.sh の JSON 差分（スキル側で判定） |
| 2 | 同じフェーズのやり直しが積み重なる | **フェーズ別リトライ上限（2 回）**: `git log --merges` 中の `claude/sdd-NNN-implement-pN` のマージ数が 2 以上なら停止 | git のマージ履歴 |
| 3 | 機能単位で回数が膨らむ | **機能別ホップ上限**: `claude/sdd-NNN-*` のマージ数が `2（plan, tasks）+ フェーズ数 + 余裕 2` 以上なら停止 | git のマージ履歴 + tasks.md のフェーズ数 |
| 4 | 二重発火（同じイベントで 2 セッション、人が手で回した直後に routine も走る、日次トリガーとイベントが重なる） | **冪等ガード**: 対象機能の open PR（head `claude/sdd-NNN-*`、base `claude/sdd-NNN-feature`）が既にあれば、label 付与に失敗していても新しい段階 PR は作らない。同 prefix で `base.ref` が無い場合は fail-closed。未解決レビューがある場合だけレビュー対応へ進む | 組み込み GitHub ツールで取得した open PR 一覧と review threads |
| 5 | 緊急停止 | routine の一時停止（claude.ai のトグル）。加えて `sdd` ラベルを付けなければ発火しない（トリガーのフィルタ条件） | routine 設定 / PR ラベル |

補助的な歯止め:

- トリガーの購読は `pull_request.closed` のみ。routine が開く PR には `sdd` ラベルが
  付くが、`opened` / `labeled` は購読しないので自分の PR で自分が起きることはない。
  マージされずに閉じた PR は `is merged = true` のフィルタで弾く
- routine のアカウント単位の日次実行上限が外側の天井になる

`sdd-guard.sh` の手順:

1. **対象機能の確定**: 組み込み GitHub ツールで取得した closed PR 一覧のうち `sdd` ラベル付きで、
   git の first-parent 履歴に現れる直近 PR を採る。その merge/squash commit が触った
   `specs/NNN-*/` を `--feature` として `sdd-state.sh` を呼び直す。取れなければ state.sh の
   既定（昇順最初）を使う。これで「ラベル無しで寝かせている spec」を誤って拾わない
2. 冪等ガード（#4）。open PR に未解決レビュー指摘がある場合は通常の停止ではなくレビュー対応へ渡す。
   指摘が無い tasks / implement の non-draft PR は checks を再評価し、green なら既存 PR を
   マージする
3. フェーズ別リトライ上限（#2）
4. ホップ上限（#3）

出力は `{"go":true,"state":{...}}` か `{"go":false,"reason":"...","state":{...}}`。
`gh` が使えない環境では理由付きで `go:false` を返す。

停止（#1〜#3）した場合は、PR を開かない代わりに Issue「`sdd-next 停止: NNN <reason>`」を
1 件立てて人に知らせる。同じタイトルの open Issue があれば作らない。`done` / `none` は
正常終了なので Issue は作らない。

## 5.5 レビュー対応フロー

日次トリガーまたは手動実行で、open な `sdd` PR に未解決レビュー指摘があれば、新しい段階を
始めずにその PR だけを直す。対象は `sdd` ラベル付きで、段階 PR（base =
`claude/sdd-NNN-feature`）または最終 PR（base = `main`）に限る。

1 セッションで扱う PR は 1 件だけ。review thread が未解決、または最新 commit 後に
`REQUEST_CHANGES` / 修正依頼コメントがあるものを対象にし、head branch を checkout して
最小差分を commit、同じ PR に push する。修正後は該当 thread に対応内容と検査結果を返信し、
解決できた thread を resolve する。仕様判断・権限・外部情報が必要なコメントは、修正せず
block 理由を PR コメントに残して終了する。

このフローは「レビューコメントを作る」仕組みではない。既存レビューを SDD ハーネスの durable
queue に載せ、通常の段階生成と混線させずに返すための入口である。
修正後の tasks / implement PR は checks が green ならその場でマージしてよい。checks が pending
または読めない場合も、次回の日次実行が同じ open PR を再評価する。

## 6. スキルの手順: `/sdd-next [--dry-run]`

routine のプロンプトは「`/sdd-next` を実行する。それ以外の作業はしない」だけにし、手順は
すべて SKILL.md に置く。人が手元の web セッションで同じコマンドを打っても同じ動きになる。

```
0.   前提確認      作業ツリーが clean。必要なら feature branch を復元する。ダメなら理由を出して終了
0.5  使用量ゲート  7 日窓 ≥ 80% または 5 時間窓 ≥ 70% なら「今回は見送り」と書いて終了
                   （PR も Issue も作らない。再開は日次トリガーか次のマージに任せる）
1.   判定          before = sdd-state.sh → sdd-guard.sh。before は guard.state に置き換える。
                   go:false なら
                   open-pr はレビュー対応または checks 再評価へ渡す。feature branch 上の
                   nothing-to-do は最終 PR のレビュー確認へ進む。stage が done/none 以外の
                   上限停止なら Issue を立てて終了
1.5  レビュー対応  open な sdd PR に未解決レビュー指摘があれば、その head に修正 commit を
                   push し、返信・resolve して終了
     --dry-run     ここまでで「何をするつもりか」を表示して終了
2.   準備          git switch -c <state.branch>、export SPECIFY_FEATURE_DIRECTORY=<feature_dir>
3.   段階の実行（下表）
4.   前進確認      after = sdd-state.sh --feature <dir>。before と同じ、または git diff が
                   空なら PR を開かず Issue を立てて終了
5.   PR            コミット（既存の慣習どおり日本語、Co-Authored-By 付き）→ push →
                   ラベル sdd 付きで state.base_branch へ PR。本文に before/after の JSON、実行した検査、
                   残課題、セッションへのリンク（CLAUDE_CODE_REMOTE_SESSION_ID）を書く。
                   tasks / implement は checks green を確認してから自動マージする。
                   自動マージ成功後は remote の段階 branch を削除する
6.   報告          最後に「段階・PR URL・次に起きること」を 3 行で出す
```

段階ごとの実行内容:

| stage | 呼ぶもの | 追加の作業 | PR の題名例 |
| --- | --- | --- | --- |
| `plan` | `/speckit-plan` | なし | `docs: 002 の実装計画と設計成果物を追加する` |
| `tasks` | `/speckit-tasks` → `/speckit-analyze` | analyze の結果を PR 本文に載せる。CRITICAL が tasks.md 側の直しで解消できるものだけ直す（spec／plan は触らない） | `docs: 002 の実装タスクを分解する` |
| `implement` | `/speckit-implement "Phase N（<title>）のタスクだけを対象にする"` | 完了タスクを `[X]` にする。`make check` を通す。通らなければ直し、それでも通らないときは draft PR として開き本文に失敗内容を書く（人が判断してから ready にする）。文書の同時更新や `docs/exec-plans/` の扱いは `AGENTS.md` に従う | `feat: 002 Phase 3（US1 置いた動画が自動で一覧に並ぶ）を実装する` |
| `done` / `none` | 何もしない | — | — |

### UI 変更の実装・視覚評価ループ

implement の差分が UI 変更を含む場合は、通常の「部品実装 → `make check` → PR」だけでは
完了扱いにしない。`sdd-ui-classify.sh` が、対象機能の spec / plan / tasks にある
`<!-- sdd-ui-change: yes/no -->` を優先し、明示が無ければ変更パスで判定する。パスだけで
拾えない API 変更が画面表示を変える場合は、spec に明示分類を書く。

UI 変更では次を必須にする。

1. 実装前に、ユーザー要求、仕様、参照画像、変更前画面を確認する
2. 「一覧画面を完成」「再生画面を完成」のように、1 画面を端から端まで評価できる単位で実装する
3. 実ブラウザで 360px、768px、1280px のスクリーンショットを生成する
4. 参照画像がある場合、参照画像と変更後スクリーンショットを並べた比較画像を生成する
5. 実装中の判断とは別に、撮影後の画面成果物を入力にした visual review を行う
6. visual review の指摘を修正して再撮影する。指摘なしの場合も、その評価を PR 本文に残す
7. interaction / accessibility を確認してから PR を non-draft にする

PR 本文には、変更前後、確認した viewport、比較画像、visual review の指摘と修正、視覚上の
残課題を載せる。UI 変更でない場合は `UI 変更なし` と書く。

GitHub 操作の使い分け:

- シェルスクリプト（`sdd-guard.sh`）は cloud の組み込み GitHub ツールを直接呼べないため、
  スキルが PR 一覧を JSON ファイルに保存して `--github-dir` で渡す
- スキルの手順（PR 作成・Issue 作成・label 付与・マージ）は cloud セッション組み込みの
  GitHub ツールを通常経路にする。`gh` は cloud に無い実機ケースがあるため、使える場合の
  手元検算・保守用フォールバックに留める
- PR 一覧は 100 件単位でページを連結してから `sdd-guard.sh` へ渡す。通常 PR が多い時期でも
  対象 feature の古い段階 PR が欠落しないようにする

Spec Kit との接続: `SPECIFY_FEATURE_DIRECTORY` を export しておけば既存の
`.specify/scripts/bash/common.sh` が `.specify/feature.json`（gitignore 済み）を書き、
以後の `/speckit-*` は routine のセッション（`claude/...` ブランチ）でもそのまま動く。

### 使用量ゲートの取得方法

5 時間窓・7 日窓の `used_percentage` は、Claude Code が**ステータスライン用スクリプトへの
JSON**（`rate_limits.five_hour.used_percentage` 等）にだけ渡していて、フックや環境変数には
出ない。cloud セッションでステータスラインスクリプトが実行されるかは文書に明記が無いので、
導入時のプローブで決める:

- 案 a) リポジトリの `.claude/settings.json` に `statusLine` コマンドを置き、受け取った
  JSON を `/tmp/sdd-rate-limits.json` に書き出す。cloud セッションでも動けばスキルが読む
- 案 b) `api.anthropic.com` の OAuth usage エンドポイントをプロキシ経由で叩けるか試す
- 両方ダメなら**ゲートは無効のまま**にし、`docs/exec-plans/tech-debt.md` に記録する

しきい値は SKILL.md の先頭に定数として書き、routine 側には持たせない。

## 7. routine の設定

| 項目 | 値 |
| --- | --- |
| 名前 | `sdd-next (syudead/vv)` |
| リポジトリ / 環境 | `syudead/vv` / 既存の Default。依存取得は既存の SessionStart フック（`CLAUDE_CODE_REMOTE=true` なら `make setup`）に任せ、routine で走らなければ環境の setup script に `make setup` を置く |
| プロンプト | 「リポジトリの `/sdd-next` スキルを実行する。それ以外の作業はしない。`routine-fire-payload` に PR 情報があれば対象機能の特定に使ってよい。」 |
| トリガー 1 | GitHub `pull_request.closed`。フィルタ: labels に `sdd` を含む、is merged = true。base は限定しない |
| トリガー 2 | スケジュール: 毎日 1 回（深夜）。見送り・取りこぼし・open PR のレビュー対応と checks 再評価用。状態は導出・ガードは冪等なので、待機状態なら無害 |
| コネクタ | 無し（GitHub は組み込みツールとプロキシで足りる） |

設定の写しは [docs/references/sdd-routine.md](../references/sdd-routine.md) に置き、
claude.ai 側の設定が消えても再現できるようにする。

リポジトリには `sdd` ラベルを 1 つ作る（説明: 「マージすると /sdd-next が次の段階を回す」）。

## 8. テスト

- `sdd-state.sh`: `.claude/skills/sdd-next/tests/` にフィクスチャ（plan 前 / tasks 前 /
  implement 途中 / done / spec 無し / 複数機能）と bash のテストランナーを置く。
  `make test-sdd` を追加し、CI の Go ジョブに 1 ステップ足す。**ランナーの依存は bash だけで、
  `jq` は要らない**（保守者の手元の Git Bash に `jq` が無いため。4 章の判定を手元で
  検算できることが US3 の要件である）
- フィクスチャの構成: 各ディレクトリが `specs/` の書式を縮小したファイル群と、
  `--root` だけを渡したときの出力を書いた `expected.json` を持つ。`--feature` の検証が
  要るものは、渡す相対パスを書いた `feature.txt` と `expected-feature.json` を足す。
  `done` の機能は自動選択で飛ばされるので、`done` の形は `--feature` 経由でしか
  検証できない
- `sdd-guard.sh`: `gh` 無しの環境で `go:false` と理由を返すことを自動テストする。
  ランナーは一時ディレクトリに `gh` の代役（即座に終了コード 1 を返す実行可能ファイル）を
  置いて `PATH` の先頭に足し、`{"go":false,"reason":"gh-unavailable","state":<stdin そのまま>}`
  が返ることを確かめる。`jq` と git がある環境では `--github-dir` で PR 一覧を渡す判定も
  自動テストする
- スキル全体: `/sdd-next --dry-run` を導入時のプローブと日常の検算に使う
- `sdd-ui-classify.sh`: spec の明示 `yes` / `no`、UI 対象パス、非 UI パスを
  `.claude/skills/sdd-next/tests/run.sh` で検証する

## 9. 導入手順

1. スクリプト・スキル・テスト・文書を 1 つの PR で入れてマージする（この PR には `sdd`
   ラベルを付けない）
2. routine を作り、Run now でプローブする: `/sdd-next --dry-run` を走らせ、SessionStart
   フックの実行・使用量の取得方法・fire payload の中身を確認する。結果で使用量ゲートの
   実装を決めて小さな追従 PR を出す
3. Run now で本番 1 回目: 次の未完了 spec の feature branch が作られ、`plan` 段階の PR が開く
4. plan PR をレビューして `sdd` ラベル付きで feature branch にマージする。以後は自動で
   tasks → implement（フェーズごと）→ 最終 PR まで進む

## 10. 検討した代替案

| 案 | 見送った理由 |
| --- | --- |
| 段階ごとに routine を分け、`sdd:plan-next` 等のラベルでフィルタする | ラベルが事実上の状態になる。routine が増えるほど claude.ai 側の設定が散らばる |
| GitHub Actions が状態を計算して routine の API トリガーを叩く | トークン管理と Actions の追加が要る。動くものが 2 系統になり追いにくい |
| 状態ファイル（`specs/NNN/state.yml`）を持つ | 成果物との二重管理になる。人が手で段階を進めたときにずれる |
| implement 全体で 1 PR にし、同じ PR に続きのセッションが push する | 002 は 1 セッションで終わる大きさではない。トリガーもマージ以外の契機が要る |
