# Implementation Plan: シーク用サムネイルをスプライトシートにし、枚数とファイル数に上限を設ける

> 生成方式は、その後の[長尺動画のシーク用スプライト生成](../../docs/design-docs/seek-sprite-generation.md)で更新した。
> 以下の全編デコードの記述は初期実装の判断で、現在も短尺・低解像度・区間抽出失敗時に使う。

**Branch**: `feature/021-seek-thumbnail-sprite` | **Parent Issue**: #389

**Input**: The parent Issue. It is this feature's specification.

## Summary

今のシーク用サムネイルは 5 秒ごとの JPEG を 1 枚ずつ個別ファイルとして保存し、プレイヤーは
位置が変わるたびに 1 枚ずつ取得する。枚数は動画の長さに比例して上限が無い（2 時間で 1,440
ファイル）。これを、1 本の動画につき最大 6 枚のスプライトシート（10 列 × 10 行、最大 600 コマ）と、
その並びを表す配置情報に置き換える。

- コマの間隔・コマ数・シートの枚数は `internal/domain` の純粋関数が動画の長さから決める。
  50 分までは今の 5 秒間隔のままで、それを超える動画だけ間隔を広げる
  （[research.md R-1](research.md#r-1-上限と間隔の規則)）。
- 生成は今と同じ 1 回の全編デコードだが、`select` の代わりに `fps` フィルタで間隔ごとに 1 コマを
  取り、`tile` フィルタで格子に並べる。コマの無い区間は前のコマで埋め、末尾に映像が無ければ最後の
  コマを複製するので、コマの番号と再生位置の対応が崩れない（[R-2](research.md#r-2-コマの選び方)）。
- 置き場は今の `seek/<p>/<s>/` のまま、シートの JPEG と配置情報 `sprite.json` を置く。完成の印は
  `sprite.json` の有無で、これが無い置き場（旧形式の個別 JPEG を含む）は未完成として作り直し、
  公開のときに旧形式のファイルを回収する（[R-3](research.md#r-3-置き場と完成の印)）。
- `GET /api/videos/{id}/seek-thumbnail` は `positionMs` の JPEG ではなく配置情報（JSON）を返し、
  シートは `GET /api/videos/{id}/seek-thumbnail/{sheet}` で返す
  （[contracts/seek-sprite-api.md](contracts/seek-sprite-api.md)、[R-4](research.md#r-4-api-の形)）。
- プレイヤーは配置情報を 1 回、シートを必要になったときに 1 回だけ取得し、同じ動画の間は
  取得し直さずにコマを切り替える。コマは配置情報の大きさで切り出し、隣のコマを見せない
  （[R-5](research.md#r-5-プレイヤーの取得と切り出し)）。
- 既存の動画は移行で `seek_thumbnail` ジョブを積み直し、再取り込みなしにスプライトへ作り直す。
  作り直しが終わるまでは今の `pending` と同じで、時刻の表示・再生・シークは使える
  （[R-6](research.md#r-6-既存の個別-jpeg-からの移行)）。
- 生成物の再利用（同じ内容の動画は 1 回だけ生成）、元動画の差し替え・削除時の無効化、中断時の
  一時ファイルの削除、直接配信とライブ変換で同じ論理時刻を使う規則は変えない。

## Technical Context

**Canonical definitions**:

- 境界と依存方向、生成物の所有、段階ごとのワーカー: [ARCHITECTURE.md](../../ARCHITECTURE.md)
  （「Generated files have one owner」「Intended dependency direction」の段落）
- 今の生成: [internal/media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go)
  （`select` と `scale` の式、30 分の上限）、置き場と読み出し:
  [internal/artifacts/store.go](../../internal/artifacts/store.go)
  （`PublishSeekThumbnails`・`SeekThumbnail`・`SeekThumbnailsAvailable`・`RemoveContent`）、
  間隔の定数: `domain.SeekThumbnailInterval`（[internal/domain/video.go](../../internal/domain/video.go)）
- ジョブと状態: [specs/020-seek-thumbnail-stage/data-model.md](../020-seek-thumbnail-stage/data-model.md)
  （`seek_thumbnail` ジョブ、`videos.seek_thumbnail_state`、置き場を失った `done` の積み直し
  `RequeueMissingSeekThumbnails`）、`Ingest.SeekThumbnails` と `Catalog.SeekThumbnailState`
  （[internal/app/ingest.go](../../internal/app/ingest.go)、[internal/app/catalog.go](../../internal/app/catalog.go)）
- 今の HTTP 契約: [api/openapi.yaml](../../api/openapi.yaml)（`getVideoSeekThumbnail`、
  `Video.seekThumbnailUrl`・`seekThumbnailState`）、
  [internal/httpapi/seek_thumbnail.go](../../internal/httpapi/seek_thumbnail.go)、
  ゲストにも返す経路の表（[internal/httpapi/auth.go](../../internal/httpapi/auth.go)）、
  生成物のキャッシュ指示（[specs/016-single-account-auth/contracts/guest-api.md §5](../016-single-account-auth/contracts/guest-api.md#5-生成物のキャッシュ)）
- 今のプレイヤー側: [web/src/player/seekPreview.ts](../../web/src/player/seekPreview.ts)
  （5 秒の bucket、要求の中断、5 秒の再試行の保留、`fetchSeekThumbnail`）、
  [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx) の取り付け、
  表示の規則: [specs/009-seek-thumbnail-preview/ui-design.md](../009-seek-thumbnail-preview/ui-design.md)、
  [web/src/index.css](../../web/src/index.css) の `.vv-seek-preview`
- ライブ変換でも元動画の論理時刻を使う判断:
  `specs/009-seek-thumbnail-preview/plan.md`（Structural Decisions。完了した feature の
  Plan として `main` から外され、git の履歴にだけ残る）
- 前例: ホバープレビューの manifest（`preview/<p>/<s>.mp4.sha256`、`internal/artifacts`）、
  既存動画への積み直しの移行
  [00014_seek_thumbnail_stage.sql](../../internal/store/migrations/00014_seek_thumbnail_stage.sql)、
  生成方式の変更を測る手順 [docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md)
  と [scripts/previewbench](../../scripts/previewbench/main.go)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`・
  `task test-e2e`）

**Feature-specific context**:

- 追加する依存は無い。ffmpeg の `fps`・`tpad`・`trim`・`tile` は同梱の ffmpeg（6.1）で動くことを
  確かめた（[research.md R-2](research.md#r-2-コマの選び方)）。
- SQLite の schema は変えない。移行 `00015` は既存の動画の `seek_thumbnail_state` を `pending` に
  戻してジョブを積むだけである（[R-6](research.md#r-6-既存の個別-jpeg-からの移行)）。
- 上限は「1 本あたり最大 600 コマ・最大 6 シート」とし、2 時間の動画は 12 秒間隔の 600 コマになる。
  数の根拠は [R-1](research.md#r-1-上限と間隔の規則)。
- `GET /api/videos/{id}/seek-thumbnail` の応答の種類が JPEG から JSON に変わる。SPA はバイナリに
  同梱されて一緒に更新されるので、旧 SPA との互換は保たない（これまでの契約変更と同じ扱い）。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。配置の規則は
  `internal/domain` の純粋関数で、`internal/media` はそれを ffmpeg の引数に写すだけ、
  `internal/artifacts` は置き場・manifest・読み出しを持ち、`internal/app` と `internal/httpapi` は
  自分が宣言した interface 越しに使う。
- **生成物の所有**（ARCHITECTURE.md「Generated files have one owner」）: 合格。置き場のディレクトリは
  `seek/<p>/<s>/` のまま。中身の形式は変わるが、旧形式のファイルは `internal/artifacts` だけが
  見分けて回収する。生成途中のものは今と同じく `.tmp` の下にあり、ディレクトリごと改名するので
  配信されない（要件 8）。
- **索引と利用者データの区別**（ARCHITECTURE.md「Two kinds of data」）: 合格。移行が触るのは
  作り直せる `videos.seek_thumbnail_state` と `jobs` だけである。
- **API の正本**（ARCHITECTURE.md）: 合格。配置情報の schema と経路は `api/openapi.yaml` に足し、
  Go と TypeScript は `task generate` で作る。
- **ゲストの応答とキャッシュ**（guest-api.md §5）: 合格。配置情報とシートはどちらも
  `private, no-cache` と `ETag` で返し、ゲストにも返す経路の表に足す。
- **文書は変更と同じ PR で直す**（core-beliefs.md）: 合格。ARCHITECTURE.md の生成物の段落は
  保存と配信を切り替える単位が、技術選定と `specs/009` の記述は移行の単位が直す（要件 10）。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/021-seek-thumbnail-sprite/
├── plan.md                        # This file
│                                  # No spec.md — the parent Issue is the specification
├── research.md                    # 上限と間隔、コマの選び方、置き場、API の形、切り出し、移行の決定
├── quickstart.md                  # 2 時間・縦長・ライブ変換の入力と、改善前後の計測の手順
└── contracts/
    └── seek-sprite-api.md         # 配置情報とシートの経路、Video.seekThumbnailUrl の意味の変更
```

`data-model.md` は作らない。SQLite に足す列も表も無く、配置情報は生成物と一緒に置くファイルで、
その形は [research.md R-3](research.md#r-3-置き場と完成の印) が持つ。`ui-design.md` は作らない。
プレビューの見た目（大きさ・位置・時刻表示）は変えず、切り出しの規則は
[R-5](research.md#r-5-プレイヤーの取得と切り出し) にある。

### Source Code

**Affected boundaries**:

- `internal/domain`: スプライトの配置（間隔・コマ数・列・行・シート数）を動画の長さから決める
  純粋関数と定数。`SeekThumbnailInterval` は最小の間隔になる。
- `internal/media`: `fps`・`tpad`・`trim`・`scale`・`tile` による 1 回の ffmpeg でシートを書く。
  計測の境界 `GenerateSeekThumbnailSet`。
- `internal/artifacts`: シートと `sprite.json` の公開（コマの大きさをシートの JPEG から読む）、
  完成の判定、配置情報とシートの読み出し、旧形式の置き場の回収。
- `internal/app`: `Ingest.SeekThumbnails` が動画の長さから配置を決めて生成に渡す。
  `Catalog.SeekThumbnailState` は完成の判定の変更をそのまま受ける。
- `api/openapi.yaml`・`internal/httpapi`: 配置情報の応答、シートの経路、ゲストにも返す経路の表。
- `web/src/player`・`web/src/api`: 配置情報とシートの取得、コマの切り出し、`web/e2e` の待ち合わせ。
- `internal/store/migrations`: 既存の動画の積み直し。
- `scripts/previewbench`・`docs/how-to/preview-benchmark.md`: シーク用サムネイルの生成の計測。
- `ARCHITECTURE.md`・`docs/design-docs/tech-stack-selection.md`・`specs/009-seek-thumbnail-preview`:
  方式の記述。

**New paths**: `internal/store/migrations/00015_seek_thumbnail_sprite.sql`。

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。

## Implementation Work

### previewbench でシーク用サムネイルの生成も測れるようにする

**Scope**: `scripts/previewbench` に測る対象の種類（動くプレビュー・シーク用サムネイル）を足す。
シーク用の計測の境界として、`internal/media` に
`GenerateSeekThumbnailSet(ctx, videoPath, outputDir string, durationMs int64) error` を足す。
今はこれが今の生成で個別 JPEG を `outputDir` に書き、previewbench のシーク用はこの関数だけを呼んで、
壁時計時間とピークメモリに加えて `outputDir` のファイル数と合計バイト数を出す。後の単位はこの
関数の中身だけを変え、`scripts/previewbench` には触れない。
[docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md) にシーク用の測り方と
PR に残す表の形を足す（[quickstart.md](quickstart.md) §1）。生成そのものは変えない。

**Dependencies**: None.

**Acceptance**: `go run ./scripts/previewbench -kind seek <video>` が、今の生成で 2 時間の入力に対して
壁時計時間、ピークメモリ、ファイル数、合計バイト数を表示し、終了後に一時出力が残らない。
`scripts/previewbench` のテストが、種類の解釈とシーク用の集計を検査する。`task check` と
`task check-docs` が通る。

### シーク用サムネイルの配置の規則とスプライトシートの生成を作る

**Scope**: [research.md R-1](research.md#r-1-上限と間隔の規則) の配置の規則を `internal/domain` に
置き、[R-2](research.md#r-2-コマの選び方) の ffmpeg の引数でシートを書く生成関数を
`internal/media` に足す（配置を受け取り、1 回の ffmpeg でシートを書く。`Assets` からも呼べる形に
する）。`GenerateSeekThumbnailSet` の中身を、動画の長さから配置を決めてこの生成関数を呼ぶ形に
変える。保存・配信・プレイヤーは変えないので、再生画面の挙動はこの単位では変わらない。

**Dependencies**: `previewbench でシーク用サムネイルの生成も測れるようにする`。

**Acceptance**: `internal/domain` のテストが、50 分以下で 5 秒間隔のままであること、2 時間で
12 秒間隔・600 コマ・6 シートになること、どの長さでも 600 コマ・6 シートを超えないこと、1 コマだけの
短い動画を検査する。`internal/media` のテストが（ffmpeg があるとき）、時刻を描いたテスト入力で
先頭・中間・末尾のコマの時刻がそれぞれの区間の中にあること、映像が容器の長さより短い入力で
末尾のコマが最後の場面になること、間隔の半分より短い（1 秒の）入力で 1 コマのシートができること、
縦長の入力でコマが縦横比を保ち 1 本の中で同じ大きさになることを検査する。PR に、同じ 2 時間と
2 分の入力での改善前後の生成時間・ピークメモリ・ファイル数・合計サイズの表
（[quickstart.md](quickstart.md) §1）が載る。`task check` と `task check-docs` が通る。

### シーク用サムネイルをスプライトシートで保存・配信し、プレイヤーで切り出して表示する

**Scope**: `internal/artifacts` を [R-3](research.md#r-3-置き場と完成の印) の置き場・`sprite.json`・
完成の判定・旧形式の回収にし、`internal/app` の `Ingest.SeekThumbnails` が動画の長さから配置を
決めて前の単位の生成関数に渡すようにする。呼ばれなくなった `internal/media` の個別 JPEG の生成
（`GenerateSeekThumbnails` と `Assets.SeekThumbnails`）を消す。
[contracts/seek-sprite-api.md](contracts/seek-sprite-api.md) に従い `api/openapi.yaml` を変えて
`task generate`、`internal/httpapi` の配置情報とシートの応答と `auth.go` の表を直す。
[R-5](research.md#r-5-プレイヤーの取得と切り出し) に従い `web/src/player/seekPreview.ts` を、
配置情報を 1 回取得し、必要になったシートを 1 回だけ取得して object URL で保持し、コマを配置情報の
大きさで切り出す形にする。5 秒の bucket と `fetchSeekThumbnail` の 1 枚ずつの取得はやめ、
`web/src/api/client.ts` の取得関数を配置情報とシートに合わせる。`VideoPlayer.tsx` は
`seekThumbnailState` が変わったら取り付け直す。表示の見た目（大きさ・位置・時刻表示）は変えない。
`web/e2e/playback.e2e.ts`（待ち合わせとシークプレビューのテスト）はこの単位だけが直す。
ARCHITECTURE.md の生成物とワーカーの段落を直す。API とプレイヤーは同じ単位で切り替える。今の
プレイヤーは新しい応答を読めず、分けると間の feature branch でシークプレビューが壊れるため。

**Dependencies**: `シーク用サムネイルの配置の規則とスプライトシートの生成を作る`。

**Acceptance**: `internal/artifacts` のテストが、`sprite.json` が無い置き場を未完成と判定すること、
公開で旧形式の個別 JPEG が消えてシートと `sprite.json` に置き換わること、中断で一時置き場に何も
残らないこと、`RemoveContent` が消すことを検査する。`internal/httpapi` のテストが、配置情報が
契約の形で `private, no-cache` と `ETag` 付きで返り `If-None-Match` で 304 になること、シートの
番号が範囲外なら 404、生成中は 409 になることを検査する。web の単体テストが、位置からコマとシートを
決める規則が配置情報の間隔を使い 5 秒を前提にしないこと、同じシートの中の移動で取得が起きない
こと、シートをまたぐ移動で次のシートを 1 回だけ取得すること、取得が終わる前に離れたシートへ
戻っても 2 回目の取得が起きないこと、配置情報やシートの取得に失敗しても時刻の表示が続き 5 秒後に
再試行すること、取り外しで進行中の取得が中断され object URL が解放されることを検査する。
`task test-e2e` の再生のテストが、直接配信とライブ変換の両方で同じ位置に同じコマが出ることを
検査する。画面が変わるので、時刻を描いた 2 時間の入力と縦長の入力（[quickstart.md](quickstart.md)
§2）で、先頭・中間・末尾近くのコマの時刻が区間の中にあること、縦長でも隣のコマが見えないこと、
シークバー上でポインターを連続して動かしたときにブラウザのネットワーク記録に同じ動画のシートの
追加要求が無いことを 360px・768px・1280px で確かめ、結果を PR に残す。支援技術ではプレビューが
読み上げの対象にならないままであることを確かめる。`task check` と `task check-docs` が通る。

### 既存のシーク用サムネイルをスプライトに作り直し、技術選定と 009 の記述を合わせる

**Scope**: [research.md R-6](research.md#r-6-既存の個別-jpeg-からの移行) に従い、移行
`00015_seek_thumbnail_sprite.sql` で `seek_thumbnail_state = done` の動画を `pending` に戻し、解析が
終わり所在のある動画に未完了の `seek_thumbnail` ジョブが無ければ積む。
`docs/design-docs/tech-stack-selection.md` のシークプレビューの記述をスプライトシートに合わせ、
`specs/009-seek-thumbnail-preview` の `plan.md`・`research.md`・`contracts/seek-thumbnail.md`・
`quickstart.md` の 5 秒間隔・個別 JPEG・`positionMs` の記述を、この feature の契約への参照に
置き換える（要件 10）。

**Dependencies**: `シーク用サムネイルをスプライトシートで保存・配信し、プレイヤーで切り出して表示する`。

**Acceptance**: `internal/store` のテストが、`00015` が `done` の動画を `pending` に戻して
`seek_thumbnail` を積み、`failed` の動画と未完了のジョブがある動画には積まないこと、`up` / `down` が
通ること（`task migrations-check`）を検査する。変更前の個別 JPEG を置いたデータディレクトリで
`task preview` を起動し直すと、処理状況にシーク用サムネイルの残りが出て、終わると `seek/<p>/<s>/` に
シートと `sprite.json` だけが残り、その間も再生画面で再生・シーク・時刻の表示が使える
（[quickstart.md](quickstart.md) §3）。`task check` と `task check-docs` が通る。
