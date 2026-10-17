---
source: specs/032-card-scrub-preview/plan.md
sourceHash: 44f5187f3f201a503d4f4345465fc575d0a0b09ca2ace2c89e8e8439d711c552
---

# 実装計画: ライブラリのカードのサムネイルの下端をスクラブして動画をざっと見る {#implementation-plan-scrub-along-the-bottom-of-a-library-cards-thumbnail-to-skim-the-video}

**ブランチ**: `feature/032-card-scrub-preview` | **親 Issue**: #616

**入力**: 親 Issue。これがこの機能の仕様である。

## 概要 {#summary}

ライブラリの動画のカード (ライブラリのグリッド表示、フォルダの画面、検索結果) と、再生画面の関連動画のサムネイルに、下の 5 分の 1 を覆う透明な帯を加える。マウスのポインターが帯の中にある間、その横の位置を動画の長さに対応させ、その位置のフレームをシーク用サムネイルのスプライトからサムネイルの面に示し、再生時間の表示と視聴位置のバーをスクラブの位置に切り替える。サーバーと API は変わらない。

- フレームの選択 (`intervalMs` によるフレーム、シート、列、行) と切り抜きの計算を、再生画面のシークバーから `web/src/lib/seekSprite.ts` に移し、両方で共有する ([research.md R-1](research.md#r-1-shared-home-for-frame-selection-and-cropping))。
- 帯の状態 (いつ有効か、帯に初めて入ったときの取得、ポインターがカードを出たときの中断、失敗の後はポインターが戻るまで再試行しないこと、カードがビューポートを出たときの解放) は、`web/src/ui/ScrubPreview.tsx` のフックがカードごとに 1 つ持ち、このフックはループ再生の仕組みを知らない ([R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets)、[R-4](research.md#r-4-releasing-held-sheets))。
- 帯に入るとループ再生を `pause()` で止め、帯からカードの中へ出ると `play()` で再開する。400ms の待ちは帯の中では止まり、ポインターが出たところから数え直す。このために既存の 2 つのフック (`useCardPreview`、`useHoverPreview`) に `suspendPreview` / `resumePreview` を加える ([R-2](research.md#r-2-loop-playback-and-the-band))。
- フレームはサムネイルと同じ枠に同じ合わせ方で示し、帯は `Link` の中で再生時間の表示の手前に置く ([R-5](research.md#r-5-how-frames-fit-and-the-bands-shape))。時間の表示とバーは既存の要素を使い回し、視聴位置の `progressbar` は書き換えない ([R-6](research.md#r-6-swapping-the-time-display-and-the-bar))。
- 見た目 (スクラブの位置のバーの色、時間の区切り、動きを減らす設定での切り替え) は、`ui` ラベルのための design ステージが `ui-design.md` で決める。

## 技術的な文脈 {#technical-context}

**正本の定義**:

- Web の境界と依存の方向、`web/src/api/` だけがサーバーと通信する規則、`index.css` のトークン、`web/src/i18n/` の外に固定の文字列を置かない規則: [ARCHITECTURE.md](../../ARCHITECTURE.md) の "Web layer"
- ライブラリのホバーのプレビュー (400ms、一度に 1 つ、解放のきっかけ、選択モードに対する優先度、支援技術の扱い): [specs/010-hover-video-preview/ui-design.md](../010-hover-video-preview/ui-design.md)。実装は [web/src/videoList/cardPreview.tsx](../../web/src/videoList/cardPreview.tsx) (`useCardPreview`、`CardMedia`) と [web/src/videoList/usePreviewCoordination.ts](../../web/src/videoList/usePreviewCoordination.ts)。関連動画は [web/src/player/useHoverPreview.ts](../../web/src/player/useHoverPreview.ts) と [web/src/player/RelatedVideos.tsx](../../web/src/player/RelatedVideos.tsx) (`VideoThumbnail`)
- スプライトのレイアウトとシートの契約 (`SeekThumbnailSprite`、`intervalMs` によるフレームの選択、最後のシートの空のセル、`private, no-cache` と `ETag`): [api/openapi.yaml](../../api/openapi.yaml) (`getVideoSeekThumbnail`、`getVideoSeekThumbnailSheet`)、[specs/021-seek-thumbnail-sprite/contracts/seek-sprite-api.md](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)。今の生成 (81 フレーム、9×9、長辺 160px。古い形式は 6 シートに最大 600 フレーム): [docs/design-docs/seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)
- 再生画面の取得と切り抜き: [web/src/player/seekPreview.ts](../../web/src/player/seekPreview.ts) (`seekSpriteCell`、`seekPreviewTarget`、`showCell`)。取得の関数 `fetchSeekThumbnailSprite` と `fetchSeekThumbnailSheet` ([web/src/api/client.ts](../../web/src/api/client.ts))
- カードの寸法と重ねる要素、縦長の動画のぼかした背景: [web/src/videoList/VideoCard.tsx](../../web/src/videoList/VideoCard.tsx)、[web/src/ui/ThumbnailBackdrop.tsx](../../web/src/ui/ThumbnailBackdrop.tsx)。規則は [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
- 確認の入口: [Taskfile.yml](../../Taskfile.yml) (`task check`、`task check-docs`、`task test-e2e`)。実際のスプライトとホバーのプレビューを持つ e2e のフィクスチャ: [web/e2e/playback.e2e.ts](../../web/e2e/playback.e2e.ts) (`waitForSeekThumbnails`)、`web/e2e/hover-preview.e2e.ts` (削除済み)

**この機能に固有の文脈**:

- 依存は追加しない。サーバー、API、生成ファイルは変わらない (親 Issue の `対象外`)。
- 新しい依存の方向はない。共有の計算は `web/src/lib/` に、帯のフックとコンポーネントは `web/src/ui/` に置く。この機能で初めて `ui/` が `api/client` から取得の関数を import する ([R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets))。
- 帯は `pointerType === "mouse"` にだけ反応する (要件 10)。タッチ、ペン、キーボードは今の振る舞いのままである。
- 範囲: `VideoCard` (グリッド表示) と、`RelatedVideos` の `MemberItem` と `RelatedItem` のサムネイル。`VideoRow` (リスト表示)、`GroupCard`、`CurrentMember`、「次の動画」の案内は変わらない (要件 8、範囲外)。

## Constitution Check {#constitution-check}

- **Web の境界と依存の方向** (ARCHITECTURE.md の "Web layer"): 合格。`videoList/` と `player/` は互いを import しない。共有のコードは `lib/` (純粋な計算) と `ui/` (フックとコンポーネント) に置く。サーバーと通信するのは `api/client.ts` の既存の取得の関数だけである。
- **`index.css` のトークンを使い、生の色を使わない**: 合格。帯は透明で、フレームはシートの画像で、バーは既存のトークンを使う。design ステージが選ぶ色もトークンから取る。
- **`web/src/i18n/` の外に固定の文字列を置かない**: 合格。時間の「位置 / 長さ」は `t.list.card.scrubTime` として `en.ts` に足す ([R-6](research.md#r-6-swapping-the-time-display-and-the-bar))。
- **幅の分岐と動きを減らす設定は CSS で扱う** (library-ui.md の [Width breakpoints in CSS, and the sidebar exception](../../docs/design-docs/library-ui.md#width-breakpoints-in-css-and-the-sidebar-exception)): 合格。帯の高さは CSS の百分率で、JavaScript は位置を計算するために帯の矩形を読むだけである。フレームの切り替えにトランジションはない。
- **文書は変更と同じ PR で変える** (core-beliefs.md): 合格。最初の単位が ARCHITECTURE.md の "Web layer" に `lib/seekSprite.ts` と `ui/ScrubPreview.tsx` の 1 文を足し、design ステージが `docs/design-docs/index.md` に `ui-design.md` へのリンクを足す。

判定はフェーズ 1 の後も同じである。複雑度の追跡に挙げる違反はない。

## プロジェクトの構成 {#project-structure}

### 文書 (この機能) {#documentation-this-feature}

```text
specs/032-card-scrub-preview/
├── plan.md          # This file
│                    # No spec.md — the parent Issue is the specification
├── research.md      # R-1 to R-6: shared home, relation to the loop, fetch rules, release, fit, display swap
└── ui-design.md     # Added by the design stage (ui label)
```

`data-model.md` はない。実体も列も追加せず、画面の状態はフックの中にだけある。`contracts/` はない。API は変わらず、既存のレイアウトとシートの契約をそのまま使う。`quickstart.md` はない。受け入れ条件は `task test-e2e` で実際のスプライト (`playback.e2e.ts` のフィクスチャ) によって確かめ、人が確かめるのは画面の構成だけで、それは各単位の受け入れが述べる。

### ソースコード {#source-code}

**影響する境界**:

| パス | 変更 |
| --- | --- |
| `web/src/lib/` | スプライトの位置からフレーム、フレームから切り抜きへの純粋な計算 (`seekSprite.ts`、新規) |
| `web/src/ui/` | 帯のフック、フレームの層、帯のコンポーネント (`ScrubPreview.tsx`、新規) |
| `web/src/player/` | `seekPreview.ts` が `lib/seekSprite.ts` を使う (振る舞いは変わらない)。`useHoverPreview.ts` の一時停止と再開。`RelatedVideos.tsx` の `VideoThumbnail` の帯 |
| `web/src/videoList/` | `cardPreview.tsx` の一時停止と再開。`VideoCard.tsx` の帯と表示の切り替え |
| `web/src/i18n/en.ts` | `t.list.card.scrubTime` |
| `web/e2e/` | 帯の e2e (新しいファイル 1 つ) |
| `ARCHITECTURE.md` | "Web layer" の共有の置き場についての 1 文 |

**新しいパス**: `web/src/lib/seekSprite.ts`、`web/src/ui/ScrubPreview.tsx`、`web/e2e/card-scrub.e2e.ts`。

**構成の決定**: 共有の計算とコンポーネントは `lib/` と `ui/` に置き、`videoList/` と `player/` の間に依存を作らない ([R-1](research.md#r-1-shared-home-for-frame-selection-and-cropping)、[R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets))。

## 実装作業 {#implementation-work}

### シーク用スプライトのフレームの選択と切り抜きを共有し、カードの帯のフックを作る {#share-seek-sprite-frame-selection-and-cropping-and-build-the-card-band-hook}

**範囲**: `seekSpriteCell`、`seekPreviewTarget` の位置の計算、`showCell` のシートのレイアウトを `web/src/lib/seekSprite.ts` に移し、`web/src/player/seekPreview.ts` からそれらを呼ぶ ([R-1](research.md#r-1-shared-home-for-frame-selection-and-cropping)。再生画面の振る舞いは変わらない)。`web/src/ui/ScrubPreview.tsx` に次を足す:

- `useScrubPreview`: 帯が有効なとき、帯の矩形と `clientX` からの位置、帯に初めて入ったときのレイアウトとシートの取得、ポインターがカードを出たときの中断、失敗の後はポインターが戻るまで再試行しないこと、帯の外での表示の消去、カードがビューポートを出たときとアンマウントのときの解放。
- `ScrubFrame`: レイアウトとシートの両方があるときだけ、サムネイルと同じ枠に 1 つのフレームを示す層。
- `ScrubBand`: 下の 5 分の 1 を覆う透明な帯。`mouse` 以外のポインターは無視する。

([R-3](research.md#r-3-the-band-hook-and-rules-for-fetching-the-layout-and-sheets)、[R-4](research.md#r-4-releasing-held-sheets)、[R-5](research.md#r-5-how-frames-fit-and-the-bands-shape))。`useCardPreview` と `useHoverPreview` に `suspendPreview` / `resumePreview` を足す ([R-2](research.md#r-2-loop-playback-and-the-band))。まだ画面には何も付けない。ARCHITECTURE.md の "Web layer" に 2 つの置き場についての 1 文を足す。

**依存**: なし。

**受け入れ**: `task check` と `task check-docs` が通り、次のテストが通る:

- `web/src/lib/seekSprite.test.ts`: 帯の右端は位置 `ceil(durationMs) - 1` を与え、フレーム `frameCount - 1` を決して超えない。複数のシートがあるとき、指した位置はそのシートの番号を与える。再生画面の `seekPreview.test.ts` が今と同じ結果で通る。
- `web/src/ui/ScrubPreview.test.tsx`: 帯に入る前には取得しない。初めて入るとレイアウトを、次にそのフレームのシートを、それぞれ 1 回取得する。帯に出入りしても取得し直さない。別のシートの位置はその次のシートだけを取得する。取得が終わる前に帯を出るとフレームを示さず、カードを出ると取得を中断する。レイアウトかシートの取得中にカードを出て帯に戻ると、再び取得してフレームを示す。失敗の後はポインターが戻るまで取得しない。ビューポートを出るとオブジェクト URL を解放し、次に入ったときに再び取得する。`touch` と `pen` では何もしない。
- `cardPreview.test` と `useHoverPreview` のテスト: 待ちの間に一時停止するとタイマーを消し、再開すると 400ms を数え直す。再生中に一時停止すると `pause()` を呼び、再開すると `play()` を呼ぶ。読み込み中 (`attempting`、`playing` の前) に一時停止すると `src` と要素を保ち、再開まで再生は始まらず、再開すると `play()` を呼ぶ。

### ライブラリの動画のカードにスクラブの帯を加え、ループ再生と時間の表示を切り替える {#add-the-scrub-band-to-library-video-cards-swapping-loop-playback-and-the-time-display}

**範囲**: `VideoCard.tsx` と `cardPreview.tsx` で、`Link` の中のサムネイルの面に `ScrubBand` と `ScrubFrame` を置き、帯に出入りするときに `suspendPreview` / `resumePreview` を呼ぶ。帯の中にある間、再生時間の表示は `t.list.card.scrubTime` を示し、スクラブの位置のバーが視聴位置のバーに代わる ([R-6](research.md#r-6-swapping-the-time-display-and-the-bar)。見た目は `ui-design.md` にある)。`previewResetEpoch`、`activePreviewId`、`selectionMode` の変化と `releasePreview` もスクラブを終える。`seekThumbnailUrl` のない動画、面全体の警告を示す動画、選択モード、`VideoRow`、`GroupCard` には帯を付けない。`web/e2e/card-scrub.e2e.ts` を足し、`playback.e2e.ts` と同じ実際のスプライトで、ライブラリとフォルダの画面 (検索結果を含む) を確かめる。

**依存**: シーク用スプライトのフレームの選択と切り抜きを共有し、カードの帯のフックを作る。design ステージの `ui-design.md`。

**受け入れ**: `task check` と `task check-docs` が通る。`VideoCard.test.tsx` が、時間の表示とバーが帯の中にある間だけ切り替わり、出ると戻ること、帯のクリックがいつもどおり遷移すること、選択モードと警告のある動画には帯がないことを確かめる。`task test-e2e` の `card-scrub.e2e.ts` が、ライブラリ、フォルダの画面、フォルダの中の検索結果で受け入れ条件 1 から 7 と 9 を確かめる:

- 帯の左端から右端まで、`background-position` が最初から最後まで単調に変わる。
- 同じ割合で、シートと `background-position` が再生画面のシークバーのツールチップのものと等しい。
- 通り過ぎても `seek-thumbnail` にリクエストを送らない。帯に入るとレイアウトとシートをそれぞれ 1 回取得し、出入りしても増えない。
- ループ中に帯に入ると動画は `paused` になり、出ると再生に戻る。
- 帯のクリックは開始位置を変えずに再生画面を開く。
- 帯への出入りと取得の待ちは、カードの `getBoundingClientRect` も `scrollY` も変えない。

この単位は画面を変えるので、PR は 4 つの状態 (帯の中でのフレーム、時間、バー。縦長の動画のフレーム。取得の待ち。取得の失敗) の 360px、768px、1280px の画像を残し、帯を示す装飾がないことと、カードの寸法が変わらないことを確かめる。

### 再生画面の関連動画のサムネイルにスクラブの帯を加える {#add-the-scrub-band-to-related-video-thumbnails-on-the-player-screen}

**範囲**: `RelatedVideos.tsx` の `VideoThumbnail` に `ScrubBand` と `ScrubFrame` を足す。`MemberItem` と `RelatedItem` が `useScrubPreview` を持ち、`useHoverPreview` の `suspendPreview` / `resumePreview` を呼ぶ。右下の長さの表示と下端の進行のバーは、カードと同じ規則で切り替える ([R-6](research.md#r-6-swapping-the-time-display-and-the-bar)。見た目は `ui-design.md` にある)。`CurrentMember` と「次の動画」の案内は変わらない。関連動画の帯の確認を `playback.e2e.ts` に足す。

**依存**: シーク用スプライトのフレームの選択と切り抜きを共有し、カードの帯のフックを作る。design ステージの `ui-design.md`。

**受け入れ**: `task check` と `task check-docs` が通る。`RelatedVideos` のテストが、長さの表示とバーが帯の中にある間だけ切り替わること、`CurrentMember` に帯がないことを確かめる。`task test-e2e` の `playback.e2e.ts` が、関連動画の帯の同じ割合で、シートと `background-position` が再生画面のシークバーのツールチップのものと等しいこと (受け入れ条件 3)、通り過ぎてもリクエストを送らないこと、帯のクリックがその動画へ移ることを確かめる。この単位は画面を変えるので、PR は帯の中にある関連動画の 768px と 1280px の画像を残し、行の高さと列の幅が変わらないことを確かめる。
