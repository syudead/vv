# Implementation Plan: 一覧のカードでサムネイル下端をなぞって動画の中身を見渡せるようにする

**Branch**: `feature/032-card-scrub-preview` | **Parent Issue**: #616

**Input**: The parent Issue. It is this feature's specification.

## Summary

一覧の動画カード（ライブラリ、フォルダ画面と検索結果の格子表示）と再生画面の関連動画の
サムネイルに、下端の 5 分の 1 を占める透明な帯を置く。マウスのポインタが帯にいる間、
横位置を動画の長さに対応させ、シーク用サムネイルのスプライトからその位置のコマを
サムネイルの面に出し、再生時間の表示と視聴位置のバーをスクラブ位置に差し替える。
サーバーと API は変えない。

- コマの選び方（`intervalMs` によるコマ、シート、列・行）と切り出しの計算は、プレイヤーの
  シークバーが使う関数を `web/src/lib/seekSprite.ts` へ移して両方で共有する
  （[research.md R-1](research.md#r-1-コマ選びと切り出しの共有の置き場)）。
- 帯の状態（有効条件、初回の帯への進入での取得、カードを出たときの打ち切り、失敗時は
  入り直すまで再試行しない、viewport から外れたら解放）は `web/src/ui/ScrubPreview.tsx` の
  hook が 1 カードにつき 1 つ持ち、ループ再生の作りを知らない
  （[R-3](research.md#r-3-帯の-hook-と配置情報シートの取得の規則)、
  [R-4](research.md#r-4-保持したシートの解放)）。
- ループ再生は帯に入ったら `pause()` で止め、帯からカードの中へ出たら `play()` で戻す。
  400ms の待ちは帯にいる間は止め、出たところから数え直す。既存の 2 つの hook
  （`useCardPreview`・`useHoverPreview`）にそのための `suspendPreview` / `resumePreview` を
  足す（[R-2](research.md#r-2-ループ再生と帯の関係)）。
- コマはサムネイルと同じ枠・同じ収め方で出し、帯は `Link` の中で再生時間の表示より前に
  置く（[R-5](research.md#r-5-コマの収め方と帯の形)）。時間表示とバーの差し替えは
  既存の要素を兼用し、視聴位置の `progressbar` は書き換えない
  （[R-6](research.md#r-6-時間表示とバーの差し替え)）。
- 見た目（スクラブ位置のバーの色、時刻の区切り、動きを減らす設定での切り替え）は
  `ui` ラベルの design 段階が `ui-design.md` に決める。

## Technical Context

**Canonical definitions**:

- Web の境界と依存の向き、`web/src/api/` だけがサーバーと話す規則、`index.css` の token、
  `web/src/i18n/` の外に固定の文字列を置かない規則: [ARCHITECTURE.md](../../ARCHITECTURE.md)
  「Web layer」
- 一覧のホバープレビュー（400ms、同時に 1 件、解放の契機、選択モードとの優先順位、
  支援技術の扱い）: [specs/010-hover-video-preview/ui-design.md](../010-hover-video-preview/ui-design.md)、
  実装は [web/src/videoList/cardPreview.tsx](../../web/src/videoList/cardPreview.tsx)
  （`useCardPreview`・`CardMedia`）、
  [web/src/videoList/usePreviewCoordination.ts](../../web/src/videoList/usePreviewCoordination.ts)、
  関連動画は [web/src/player/useHoverPreview.ts](../../web/src/player/useHoverPreview.ts) と
  [web/src/player/RelatedVideos.tsx](../../web/src/player/RelatedVideos.tsx)（`VideoThumbnail`）
- スプライトの配置情報とシートの契約（`SeekThumbnailSprite`、`intervalMs` によるコマの選び方、
  最後のシートの空き升目、`private, no-cache` と `ETag`）: [api/openapi.yaml](../../api/openapi.yaml)
  （`getVideoSeekThumbnail`・`getVideoSeekThumbnailSheet`）、
  [specs/021-seek-thumbnail-sprite/contracts/seek-sprite-api.md](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)、
  今の生成（81 コマ・9×9・長辺 160px、旧形式は最大 600 コマ・6 シート）:
  [docs/design-docs/seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)
- プレイヤーの取得と切り出し: [web/src/player/seekPreview.ts](../../web/src/player/seekPreview.ts)
  （`seekSpriteCell`・`seekPreviewTarget`・`showCell`）、取得関数
  `fetchSeekThumbnailSprite`・`fetchSeekThumbnailSheet`
  （[web/src/api/client.ts](../../web/src/api/client.ts)）
- カードの寸法と overlay、縦長の動画のぼかした背景: [web/src/videoList/VideoCard.tsx](../../web/src/videoList/VideoCard.tsx)、
  [web/src/ui/ThumbnailBackdrop.tsx](../../web/src/ui/ThumbnailBackdrop.tsx)、
  規則は [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task test-e2e`）。
  e2e の実物のスプライトとホバープレビューの fixture:
  [web/e2e/playback.e2e.ts](../../web/e2e/playback.e2e.ts)（`waitForSeekThumbnails`）、
  [web/e2e/hover-preview.e2e.ts](../../web/e2e/hover-preview.e2e.ts)

**Feature-specific context**:

- 追加する依存は無い。サーバー、API、生成物は変えない（親 Issue「対象外」）。
- 新しい依存の向きを作らない。共有する計算は `web/src/lib/`、帯の hook と部品は
  `web/src/ui/` に置く。`ui/` が `api/client` の取得関数を import するのはこの feature が
  初めてである（[R-3](research.md#r-3-帯の-hook-と配置情報シートの取得の規則)）。
- 帯は `pointerType === "mouse"` だけに反応する（要件 10）。タッチ・ペン・キーボードは今の
  操作のまま。
- 対象は `VideoCard`（格子表示）と `RelatedVideos` の `MemberItem`・`RelatedItem` の
  サムネイル。`VideoRow`（リスト表示）、`GroupCard`、`CurrentMember`、「次の動画」の
  案内は変えない（要件 8、対象外）。

## Constitution Check

- **Web の境界と依存の向き**（ARCHITECTURE.md「Web layer」）: 合格。`videoList/` と `player/` は
  互いを import せず、共有は `lib/`（純粋な計算）と `ui/`（hook と部品）に置く。サーバーと
  話すのは `api/client.ts` の既存の取得関数だけである。
- **`index.css` の token と生の色の禁止**: 合格。帯は透明、コマはシートの画像、バーは既存の
  token を使う。design 段階が選ぶ色も token から選ぶ。
- **`web/src/i18n/` の外に固定の文字列を置かない**: 合格。時刻の「位置 / 長さ」は
  `t.list.card.scrubTime` として `en.ts` に足す（[R-6](research.md#r-6-時間表示とバーの差し替え)）。
- **幅の分岐と動きを減らす設定は CSS で扱う**（library-ui.md §4）: 合格。帯の高さは CSS の
  割合で、JavaScript は帯の矩形を位置の計算に読むだけである。コマの切り替えに遷移は
  付けない。
- **文書は変更と同じ PR で直す**（core-beliefs.md）: 合格。ARCHITECTURE.md「Web layer」に
  `lib/seekSprite.ts` と `ui/ScrubPreview.tsx` の 1 文を足すのは最初の単位、
  `docs/design-docs/index.md` への `ui-design.md` のリンクは design 段階が足す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/032-card-scrub-preview/
├── plan.md          # This file
│                    # No spec.md — the parent Issue is the specification
├── research.md      # R-1〜R-6: 共有の置き場、ループとの関係、取得の規則、解放、収め方、表示の差し替え
└── ui-design.md     # design 段階が足す（ui ラベル）
```

`data-model.md` は作らない。エンティティも列も足さず、画面の状態は hook の中だけにある。
`contracts/` は作らない。API を変えず、既存の配置情報とシートの契約をそのまま使う。
`quickstart.md` は作らない。受け入れ条件は `task test-e2e` の実物のスプライト
（`playback.e2e.ts` の fixture）で確かめられ、人が確かめるのは画面の構図だけで、それは各単位の
Acceptance に書く。

### Source Code

**Affected boundaries**:

- `web/src/lib/`: スプライトの位置→コマ、コマ→切り出しの純粋な計算（`seekSprite.ts`、新規）。
- `web/src/ui/`: 帯の hook と、コマの層・帯の部品（`ScrubPreview.tsx`、新規）。
- `web/src/player/`: `seekPreview.ts` が `lib/seekSprite.ts` を使う（挙動は変えない）。
  `useHoverPreview.ts` に一時停止と再開、`RelatedVideos.tsx` の `VideoThumbnail` に帯。
- `web/src/videoList/`: `cardPreview.tsx` に一時停止と再開、`VideoCard.tsx` に帯と表示の
  差し替え。
- `web/src/i18n/en.ts`: `t.list.card.scrubTime`。
- `web/e2e/`: 帯の e2e（新規 1 ファイル）。
- `ARCHITECTURE.md`: 「Web layer」に共有の置き場の 1 文。

**New paths**: `web/src/lib/seekSprite.ts`、`web/src/ui/ScrubPreview.tsx`、
`web/e2e/card-scrub.e2e.ts`。

**Structure decision**: 共有の計算と部品を `lib/` と `ui/` に置き、`videoList/` と `player/` の
間に依存を作らない（[R-1](research.md#r-1-コマ選びと切り出しの共有の置き場)、
[R-3](research.md#r-3-帯の-hook-と配置情報シートの取得の規則)）。

## Implementation Work

### シーク用スプライトのコマ選びと切り出しを共有し、カードの帯の hook を作る

**Scope**: `seekSpriteCell`・`seekPreviewTarget` の位置の計算・`showCell` の敷き方を
`web/src/lib/seekSprite.ts` へ移し、`web/src/player/seekPreview.ts` はそれを呼ぶ
（[R-1](research.md#r-1-コマ選びと切り出しの共有の置き場)。プレイヤーの挙動は変えない）。
`web/src/ui/ScrubPreview.tsx` に `useScrubPreview`（有効条件、帯の矩形と `clientX` からの位置、
初回の帯への進入での配置情報とシートの取得、カードを出たときの打ち切り、失敗時は入り直す
まで再試行しない、帯の外での表示の消去、viewport から外れたときと unmount での解放）、
`ScrubFrame`（配置情報とシートが揃ったときだけ、サムネイルと同じ枠に 1 コマを出す層）、
`ScrubBand`（下端 5 分の 1 の透明な帯。`mouse` 以外のポインタは無視する）を足す
（[R-3](research.md#r-3-帯の-hook-と配置情報シートの取得の規則)、
[R-4](research.md#r-4-保持したシートの解放)、[R-5](research.md#r-5-コマの収め方と帯の形)）。
`useCardPreview` と `useHoverPreview` に `suspendPreview` / `resumePreview` を足す
（[R-2](research.md#r-2-ループ再生と帯の関係)）。画面にはまだ付けない。
ARCHITECTURE.md「Web layer」に 2 つの置き場の 1 文を足す。

**Dependencies**: None.

**Acceptance**: `web/src/lib/seekSprite.test.ts` が、帯の右端で `ceil(durationMs) - 1` の位置に
なり `frameCount - 1` のコマを超えないこと、複数シートで指した位置のシート番号になること、
プレイヤーの `seekPreview.test.ts` が今と同じ結果で通ることを検査する。
`web/src/ui/ScrubPreview.test.tsx` が、帯に入るまで取得が起きないこと、初回の進入で配置情報
→ そのコマのシートの順に 1 回ずつ取得すること、帯を出入りしても再取得しないこと、
シートをまたぐ位置で次のシートだけを取ること、取得が終わる前に帯を出たらコマを出さず
カードを出たら中断し、配置情報またはシートの取得中にカードを出て入り直し帯へ入ると
取得し直してコマを出すこと、失敗したら入り直すまで取得しないこと、viewport から外れたら
object URL が解放され次の進入で取り直すこと、`touch`・`pen` では何も起きないことを
検査する。`cardPreview.test` と `useHoverPreview` のテストが、待ちの間の一時停止で timer が
消え再開で 400ms から数え直すこと、再生中の一時停止で `pause()` され再開で `play()` される
こと、読み込み中（`attempting` で `playing` 前）の一時停止で `src` と要素を保ったまま再開まで
再生が始まらず、再開で `play()` されることを検査する。`task check` と `task check-docs` が通る。

### 一覧の動画カードにスクラブの帯を付け、ループ再生と時間表示を差し替える

**Scope**: `VideoCard.tsx` と `cardPreview.tsx` で、`Link` の中のサムネイルの面に `ScrubBand` と
`ScrubFrame` を置き、帯への出入りで `suspendPreview` / `resumePreview` を呼ぶ。帯にいる間、
再生時間の表示を `t.list.card.scrubTime` に、視聴位置のバーの場所にスクラブ位置のバーを
出す（[R-6](research.md#r-6-時間表示とバーの差し替え)、見た目は `ui-design.md`）。
`previewResetEpoch`・`activePreviewId`・`selectionMode` の変化と `releasePreview` で
スクラブも終える。`seekThumbnailUrl` の無い動画、全面の警告が出る動画、選択モード、
`VideoRow`、`GroupCard` では帯を付けない。`web/e2e/card-scrub.e2e.ts` を足し、
`playback.e2e.ts` と同じ実物のスプライトでライブラリとフォルダ画面（検索結果を含む）を
確かめる。

**Dependencies**: `シーク用スプライトのコマ選びと切り出しを共有し、カードの帯の hook を作る`、
design 段階の `ui-design.md`。

**Acceptance**: `VideoCard.test.tsx` が、帯にいる間だけ時間表示とバーが差し替わり出ると
戻ること、帯の上のクリックが通常どおり遷移すること、選択モードと警告の出る動画で帯が
無いことを検査する。`task test-e2e` の `card-scrub.e2e.ts` が、受け入れ条件 1〜7 と 9 を
ライブラリ・フォルダ画面・フォルダ内の検索結果で検査する: 帯の左端から右端で
`background-position` が先頭から末尾へ単調に変わる、同じ割合の位置で再生画面のシークバーの
吹き出しと同じシート・同じ `background-position` になる、通過だけでは `seek-thumbnail` への
要求が無く帯に入ると配置情報とシートが 1 回ずつで出入りしても増えない、ループ中に帯へ
入ると `paused` になり出ると再生に戻る、帯の上のクリックで再生画面が開き開始位置が
変わらない、帯の出入りと取得待ちでカードの `getBoundingClientRect` と `scrollY` が
変わらない。画面が変わるので、360px・768px・1280px で、帯にいるときのコマ・時刻・バー、
縦長の動画のコマ、取得待ち、取得失敗の 4 状態の画像を PR に残し、帯を示す装飾が無いことと
カードの寸法が変わらないことを確かめる。`task check` と `task check-docs` が通る。

### 再生画面の関連動画のサムネイルにスクラブの帯を付ける

**Scope**: `RelatedVideos.tsx` の `VideoThumbnail` に `ScrubBand` と `ScrubFrame` を足し、
`MemberItem` と `RelatedItem` が `useScrubPreview` を持って `useHoverPreview` の
`suspendPreview` / `resumePreview` を呼ぶ。右下の長さの表示と下端の進捗バーを、カードと
同じ規則で差し替える（[R-6](research.md#r-6-時間表示とバーの差し替え)、見た目は
`ui-design.md`）。`CurrentMember` と「次の動画」の案内は変えない。`playback.e2e.ts` に
関連動画の帯の検査を足す。

**Dependencies**: `シーク用スプライトのコマ選びと切り出しを共有し、カードの帯の hook を作る`、
design 段階の `ui-design.md`。

**Acceptance**: `RelatedVideos` のテストが、帯にいる間だけ長さの表示とバーが差し替わること、
`CurrentMember` に帯が無いことを検査する。`task test-e2e` の `playback.e2e.ts` が、関連動画の
帯の同じ割合の位置で、プレイヤーのシークバーの吹き出しと同じシート・同じ
`background-position` になること（受け入れ条件 3）、通過だけでは要求が無いこと、帯の上の
クリックでその動画へ移ることを検査する。画面が変わるので、768px と 1280px で関連動画の
帯にいるときの画像を PR に残し、行の高さと列の幅が変わらないことを確かめる。
`task check` と `task check-docs` が通る。
