# Implementation Plan: 長尺動画の動くプレビュー生成を入力側シークにする

**Branch**: `feature/019-preview-input-seek` | **Parent Issue**: #387

**Input**: The parent Issue. It is this feature's specification.

## Summary

一覧カードの動くプレビュー（`internal/media/preview.go`）は、9 秒を超える動画から全編に等間隔の
12 区間 × 0.75 秒を選び、今は入力を 1 つ開いて 12 本の `trim` 分岐へ分けて `concat` している。
この方式は元動画をほぼ末尾までデコードし、後ろの区間の分岐にデコード済みフレームが溜まるので、
長尺では時間もメモリも動画の長さに比例して増える（要件 1・2）。

これを、同じ 12 区間をそれぞれ `-ss <start> -t 0.75 -i <input>` の入力として 1 回の ffmpeg に渡し、
`concat` フィルタで連結する方式に置き換える
（[research.md R-1](research.md#r-1-区間を読む方式)）。ffmpeg は各入力で区間の直前のキーフレームまで
シークしてそこからだけデコードするので、読む量と保持するフレームは区間の数と長さで決まり、動画の
長さには依らない。区間の選び方（`PreviewSegments`）、9 秒以下の動画を全体変換する経路、エンコード
設定（H.264・`yuv420p`・無音・fast-start・640×640 の枠）、置き場所・manifest・公開前の元動画の
確認・中断時の一時ファイル削除（`internal/artifacts`・`internal/app`）は変えない（要件 3〜5）。

改善前後を同じ入力・環境で比べられるよう、生成物 1 本の壁時計時間とピークメモリを測る Go プログラム
`scripts/previewbench` と、その手順（`docs/how-to/preview-benchmark.md`）を残す（要件 6）。

## Technical Context

**Canonical definitions**:

- 境界と依存方向（`internal/media` は渡されたパスに ffmpeg を走らせるだけで、置き場・公開・削除は
  `internal/artifacts`、生成の判断は `internal/app`）: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- 今の生成: [internal/media/preview.go](../../internal/media/preview.go)、
  公開と一時置き場: `PublishPreview`（[internal/artifacts/store.go](../../internal/artifacts/store.go)）、
  生成の判断と元動画の確認: `Ingest.Preview`（[internal/app/ingest.go](../../internal/app/ingest.go)）
- 区間の選び方と出力の検査: [internal/media/preview_test.go](../../internal/media/preview_test.go)
- 補助プログラムの置き場（Go で書き、`go run ./scripts/<name>` で呼ぶ）: [Taskfile.yml](../../Taskfile.yml)、
  [scripts/](../../scripts/)。手順書の置き場: [docs/how-to/README.md](../../docs/how-to/README.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`）

**Feature-specific context**:

- 追加する依存は無い。使う ffmpeg の機能（入力ごとの `-ss`/`-t`、複数入力の `concat` フィルタ）は
  ffmpeg 6.1（開発コンテナ・CI の Ubuntu）と Docker イメージの alpine 3.24 の ffmpeg のどちらにもある。
- 変わるのは ffmpeg の引数の組み立て（`previewArgs`）だけで、生成物の形式・置き場・状態遷移・API・
  UI は変わらない。
- 親 Issue の計測は Windows と Linux で結果の向きが違う（短尺では Windows の現行が 0.6 秒で最速）。
  短尺で「明らかに遅くならない」（受け入れ条件 2）は、実装 PR が両方の環境の数字で示す。開発コンテナ
  （Linux・ffmpeg 6.1.1・4 コア・16GB）での事前計測は [research.md R-1](research.md#r-1-区間を読む方式)
  にある。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。変更は `internal/media` の中で
  完結し、`internal/artifacts`・`internal/app` との interface（出力パスを受け取り MP4 を書く）は変えない。
  `scripts/previewbench` は `cmd/mdm` と同じ側から `internal/media` を使う（depguard の許す方向）。
- **生成物の所有**（ARCHITECTURE.md「Generated files have one owner」）: 合格。ffmpeg は渡された出力
  パスにだけ書く。中間ファイルを作る方式は採らない（Structural Decisions 1）。
- **失敗と再開**（親 Issue Edge Cases）: 合格。ffmpeg が失敗したら今までどおり誤りを返し、既存のジョブ
  再試行と失敗記録が扱う。旧方式へ戻す fallback は置かない（Structural Decisions 3）。
- **文書は変更と同じ PR で直す**（core-beliefs.md）: 合格。計測手順は `docs/how-to/` に置き、
  README から辿れるようにする。ARCHITECTURE.md の記述（ffmpeg で content-keyed hover-preview clip を作る）
  は方式の変更で変わらない。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/019-preview-input-seek/
├── plan.md          # This file
│                    # No spec.md — the parent Issue is the specification
└── research.md      # 区間を読む方式・末尾の空区間・シークの精度・計測方法の決定と計測値
```

`data-model.md` は作らない（表・状態・エンティティを足さない）。`contracts/` は作らない（API・UI・
ファイルの配置を変えない）。`quickstart.md` は作らない。確かめ方は既存の検査と実装単位の受け入れに
書いたテストで足り、計測の手順は恒久的な置き場 `docs/how-to/preview-benchmark.md` に書く（P-3）。

### Source Code

**Affected boundaries**:

- `internal/media`: `previewArgs` の 9 秒超の経路を、区間ごとの入力とその `concat` に変える。
  `PreviewSegments`、9 秒以下の経路、`GeneratePreview` の呼び方は変えない。
- `scripts/previewbench`（新規）: 入力の動画ごとに `media.GeneratePreview` を指定回数走らせ、壁時計時間と
  ffmpeg のピークメモリを表示する。
- `docs/how-to/`: 計測手順と、比較用の入力を ffmpeg で作るコマンド。

**New paths**: `scripts/previewbench/main.go`（と `main_test.go`）、`docs/how-to/preview-benchmark.md`。

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)、
[Taskfile.yml](../../Taskfile.yml) の「込み入った処理は scripts/ の Go プログラムに置く」）。

## Structural Decisions

1. **12 区間を 1 回の ffmpeg の 12 入力（各 `-ss <start> -t 0.75 -i <input>`）として渡し、`concat`
   フィルタで連結する**（[research.md R-1](research.md#r-1-区間を読む方式)）。各入力には今と同じ
   `setpts=PTS-STARTPTS`・640×640 の枠への `scale`・`format=yuv420p` を掛ける。出力側のエンコード引数は
   今のまま。区間を別々のプロセスで抽出して concat demuxer で結合する案は、プロセス起動が 13 回になり
   （親 Issue の Windows の短尺計測では現行の 0.6 秒に対し 1.5 秒）、`internal/media` が中間ファイルの
   置き場を持つことになるので採らない。旧方式を短尺だけに残す案は、要件 1 が 9 秒超の全部を置き換える
   よう求めており、2 分・720p でも現行はピーク 5GB を使うので採らない。
2. **`-ss` は入力側に置き、精度は ffmpeg の既定（accurate seek）のまま**
   （[research.md R-3](research.md#r-3-シークの精度)）。`-noaccurate_seek` は区間の開始が直前の
   キーフレームへずれ、選んだ区間と映る場面の対応（要件 3）が崩れるので採らない。
3. **入力側シークが失敗した動画に旧方式への fallback を置かない**。ffmpeg の失敗はそのまま誤りとして
   返し、既存のジョブ再試行と失敗記録が扱う（親 Issue Edge Cases）。fallback は長尺で OOM を起こす
   経路を残すことになる。区間の末尾にフレームが無い場合は失敗ではなく、その区間が無いだけの短い出力に
   なる（[research.md R-2](research.md#r-2-フレームの無い区間)）。
4. **計測は Go プログラム `scripts/previewbench` と how-to で残し、本番コードに計測の口を足さない**
   （[research.md R-4](research.md#r-4-計測の方法)）。shell の手順書だけにする案は、Windows で
   `/usr/bin/time` が無く、リポジトリの規則が補助処理を Go に置くので採らない。`go test -bench` にする
   案は、子プロセスのピークメモリを報告できないので採らない。

## Implementation Work

### プレビュー生成の壁時計時間とピークメモリを測るプログラムと手順を用意する

**Scope**: `scripts/previewbench`（`go run ./scripts/previewbench [-runs N] <video>...` で、入力ごとに
`media.GeneratePreview` を一時ディレクトリの出力へ N 回走らせ、各回の壁時計時間と ffmpeg のピーク
メモリ（Linux/macOS では `RUSAGE_CHILDREN` の maxrss、Windows では壁時計時間のみ）を表示する）と
`docs/how-to/preview-benchmark.md`（比較用の入力を ffmpeg で作るコマンド、改善前後のコミットで同じ
入力を測る手順、結果を PR に残す形）、`docs/how-to/README.md` からのリンク。詳細は
[research.md R-4](research.md#r-4-計測の方法)。生成コードは変えない。

**Dependencies**: None.

**Acceptance**: how-to のコマンドで作った 2 時間・H.264・640×360・30fps の入力と 2 分・720p の入力に
対して `go run ./scripts/previewbench` が、回ごとの壁時計時間と（Linux/macOS では）ピークメモリを表示し、
存在しない入力や ffmpeg の失敗では 0 以外の終了コードで理由を出す。`main_test.go` が引数の解釈と結果の
表示を検査する。`task check` と `task check-docs` が通る。

### 動くプレビューの 12 区間を入力側シークで読み、1 回の ffmpeg で連結する

**Scope**: `internal/media/preview.go` の `previewArgs` の 9 秒超の経路を Structural Decisions 1〜3 の
方式に変え、コメントで方式と理由（[research.md R-1](research.md#r-1-区間を読む方式)・
[R-2](research.md#r-2-フレームの無い区間)）を指す。`PreviewSegments`、9 秒以下の経路、エンコード設定、
`GeneratePreview` の呼び方は変えない。テストを足す: 引数に区間ごとの `-ss`/`-t` が 12 組あり `trim` が
無いこと、容器の長さが映像より長い入力（映像 10 秒・音声 12 秒）でも生成が成功し末尾の空区間を除いた
区間が並ぶこと、回転情報を持つ入力の出力が縦長のまま 640×640 の枠に収まること。既存のテスト
（全編からの標本、H.264/`yuv420p`/fast-start/無音、中断）はそのまま通す。

**Dependencies**: `プレビュー生成の壁時計時間とピークメモリを測るプログラムと手順を用意する`。

**Acceptance**: 上のテストと `task check` が通る。`docs/how-to/preview-benchmark.md` の手順で、変更前
（この単位の base）と変更後を同じ環境で測った 2 時間・H.264 の入力と 2 分・720p の入力の壁時計時間と
ピークメモリが PR 本文にある。2 時間の入力で生成が完了し、ピークメモリが 30 分の入力と同程度
（長さに比例しない）で、壁時計時間が変更前より短い。2 分の入力で変更前より明らかに遅くならない
（Linux と Windows の両方の数字、または Windows で測れない場合はその旨）。生成物を `task preview` の
長尺サンプルで hover 再生すると、冒頭から末尾まで分散した場面が時系列順に約 9 秒で無音・ループで流れ、
縦長の入力の縦横比が保たれる。
