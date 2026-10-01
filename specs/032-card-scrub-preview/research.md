# Research: 一覧のカードでサムネイル下端をなぞって動画の中身を見渡せるようにする

継承する技術の決定は [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
と [ARCHITECTURE.md](../../ARCHITECTURE.md)（Web layer）にある。ここに書くのは、この feature が
足す決定だけである。親 Issue は #616。

## R-1: コマ選びと切り出しの共有の置き場

**Decision**: `web/src/player/seekPreview.ts` にある位置からコマを決める純粋関数
`seekSpriteCell` と、シートをコマの箱に敷いて列・行の分だけずらす計算（`showCell` の
`background-size` / `background-position` と `offsetPercent`）を `web/src/lib/seekSprite.ts` へ
移し、プレイヤーのシークバーとカードの帯の両方がそこから使う。帯の横位置から位置
（ミリ秒）を決める規則も同じファイルに置き、`seekPreviewTarget` の位置の計算（右端で
`ceil(durationMs) - 1` に丸める）を共有する。`attachSeekPreview` 自体は DOM を直に組み立てる
プレイヤー専用の取り付けなので動かさない。

**Rationale**: 要件 5 は「プレイヤーと一覧で同じ位置に別のコマが出ない」ことを求める。
同じ関数を呼ぶのが、規則を 2 か所に書いて揃え続けるより確実で、テストも 1 か所で済む。
`web/src/lib/` は locale に依らない計算の置き場で、`api/client` の型だけを読む前例がある
（ARCHITECTURE.md「Web layer」）。`videoList/` と `player/` は互いを import していないので、
どちらかに置くと新しい依存の向きが生まれる。

**Alternatives considered**:

- `attachSeekPreview` をカードにも取り付ける: 吹き出し（`.vv-seek-preview`）とその時刻表示を
  自分で作り、`pointerdown` でポインタを捕まえる作りで、サムネイルの面にコマを出す帯には
  合わない。React の状態（時間表示の差し替え、ループの一時停止）とも噛み合わない。却下。
- 規則を `web/src/ui/` に書き写す: 同じ式が 2 か所になり、`intervalMs` の扱いを片方だけ
  変える事故を招く。却下。

## R-2: ループ再生と帯の関係

**Decision**: 帯に入ったら、ループの状態に応じてこうする。

- 400ms の待ちの間（timer が動いている）: timer を消す。帯からカードの中へ出たら、
  そこから 400ms を数え直す。
- 再生中（`playing`）または読み込み中（`attempting` で `playing` 前）: `video` 要素を
  `pause()` し、`src` と要素は保つ。帯からカードの中へ出たら `play()` で再開する。
- 帯からカードの外へ出た、別のカードのプレビューが始まった、`previewResetEpoch` が
  変わった、選択モードに入った: 今の `releasePreview` と同じく解放する。

この「一時停止」と「再開」を `useCardPreview`（`web/src/videoList/cardPreview.tsx`）と
`useHoverPreview`（`web/src/player/useHoverPreview.ts`）のそれぞれに
`suspendPreview()` / `resumePreview()` として足し、帯の hook（R-3）は帯への出入りで
それを呼ぶだけにする。帯の hook はループの作りを知らない。

**Rationale**: 要件 3 は「帯から出てカードの中に留まっていればループ再生に戻る」と
「待ちは帯にいる間は進まず、帯から出たところから数え直す」を求める。`pause()` で止めると、
Edge Cases の「ループ再生中に帯へ入ったなら、止めた時点の画面を見せ続ける」が
`video` 要素の最後のフレームでそのまま満たせる。解放して 400ms から始め直すと、戻りが
遅く、取得待ちの間に見せる画面も失う。

**Alternatives considered**:

- 帯に入ったらループを解放し、出たら 400ms から始め直す: 要件 3 の「ループ再生に戻る」を
  満たさない（戻るまで 400ms 以上かかり、読み込みもやり直す）。却下。
- `useHoverPreview` を `useCardPreview` に統合してから帯を付ける: 2 つの hook は 010 と
  012 で別々に育ち、調整（`activePreviewId`）の有無が違う。統合は別の変更で、この feature
  の範囲（要件 8）を超える。両方に同じ 2 つの操作を足す方が小さい。却下。

## R-3: 帯の hook と、配置情報・シートの取得の規則

**Decision**: `web/src/ui/ScrubPreview.tsx` に hook `useScrubPreview` と、コマを出す層
`ScrubFrame`、帯 `ScrubBand` を置く。hook は 1 カードにつき 1 つで、次を持つ。

- 帯の有効条件: `video.seekThumbnailUrl` があり、`durationMs` が正で、
  `unplayableText(video) === null`、選択モードでなく、`pointerType === "mouse"`。
  `previewState` には依らない（Edge Cases: ループが無くてもスプライトがあれば帯は働く）。
- 取得: そのカードでポインタが帯に**初めて**入ったとき `fetchSeekThumbnailSprite` を呼び、
  配置情報が来たら今の位置のコマが載るシートを `fetchSeekThumbnailSheet` で取り、
  object URL で持つ。複数シートの旧形式（最大 6 シート）は、指した位置のシートだけを
  必要になったときに取る。持っているシートと配置情報は再取得しない（受け入れ条件 5）。
- 打ち切り: ポインタがカードを出たら進行中の取得を `AbortController` で打ち切る。帯を出て
  カードの中に留まっている間は取得を続け、終わってもコマは出さない（次に帯へ入ったときに
  使う）。
- 失敗（`409`・`404`・ネットワーク）: 状態を「使えない」にし、サムネイルのまま何も出さない。
  ポインタがカードを出て入り直すまで取得し直さない。プレイヤーの 5 秒の再試行は使わない。
- 表示: 取得が終わった時点の最新のポインタ位置のコマから出す。取得待ちの間は何も
  描かない（`ScrubFrame` は配置情報とシートが揃うまで要素を出さない）。
- 帯の外へ出たら、位置と表示を消す。配置情報とシートは保つ。

**Rationale**: 要件 7 と Edge Cases の「帯に入らずに通り過ぎるだけでは取得しない」
「カードを出た場合は取得を打ち切る」「ポインタがカードを出て入り直せば改めて取得を試す」
をそのまま状態にした。プレイヤーの再試行（5 秒）は、同じシークバーの上に留まり続ける
操作のためのもので、カードを離れれば入り直すという一覧の操作とは合わない。
`web/src/ui/` は `videoList/`（`CardMedia`）と `player/`（`VideoThumbnail`）の両方が既に
import している唯一の置き場（`ThumbnailBackdrop`）で、ここに置けば新しい依存の向きを
作らない。`ui/` が `api/client` の取得関数を呼ぶのは、「サーバーと話すのは `web/src/api/`
だけ」（ARCHITECTURE.md）に沿う。

**Alternatives considered**:

- `web/src/videoList/` に置いて `player/` から import する: 再生画面がライブラリとフォルダ
  画面の共有部品に依存する新しい向きになる。却下。
- シートを object URL にせず `sheets[n]` の URL をそのまま `background-image` に使う:
  シートは `private, no-cache` と `ETag` で返るので、帯を出入りするたびに再検証の要求が
  出て受け入れ条件 5 を破る。却下。
- 配置情報だけを一覧の応答に載せて事前に取る: サーバーと API は対象外。却下。

## R-4: 保持したシートの解放

**Decision**: hook が持つ object URL は、カードの unmount に加えて、カードが viewport から
外れたとき（`IntersectionObserver`、`rootMargin` は viewport 1 つ分）に解放する。
解放したら配置情報も捨て、次に帯へ入ったときに取り直す。

**Rationale**: Edge Cases は「保持していたシートは、カードが画面から外れたときに解放する」
と言う。一覧は無限スクロールでカードを足し続け、unmount しないので、unmount だけでは
数百枚のシート（復号済みの bitmap）を持ち続ける。

**Alternatives considered**:

- unmount でだけ解放する: 上のとおり、スクロールした分だけ増え続ける。却下。
- ページ全体で LRU の上限（例: 20 シート）を持つ: 画面に見えているカードのシートまで
  捨てることがあり、受け入れ条件 5（同じカードで再取得しない）と衝突する。却下。

## R-5: コマの収め方と帯の形

**Decision**: `ScrubFrame` はサムネイルの面に重ねる 1 つの要素で、
`aspect-ratio: frameWidth / frameHeight`・`max-width: 100%`・`max-height: 100%` を中央に
置き、その箱にシートを `columns × 100% / rows × 100%` で敷いて R-1 の位置にずらす。
これで `object-contain` の `<img>` と同じ枠・同じ収め方になり、縦長の動画では既存の
`ThumbnailBackdrop`（ぼかしたサムネイル）がそのまま後ろに残る。コマの切り替えに遷移は
付けない（動きを減らす設定でも同じ）。

帯 `ScrubBand` はサムネイルの面の下端に置く透明な要素（高さは面の 5 分の 1）で、
`Link` の中、再生時間の表示と視聴位置のバーより前（z 順）、選択のチェックより後ろに置く。
hover で拡大する media 層（`group-hover:scale-[1.03]`）の外に置き、帯の幅は拡大に
影響されない。帯はポインタの出入りと移動を受け取るだけで、クリックは `Link` に届く
（要件 9）。

**Rationale**: 要件 6 は「サムネイルと同じ枠・同じ収め方」、UI 品質は「帯の存在を示す
外枠・ラベル・アイコンを追加しない」。再生時間の表示は面の右下（`bottom-2`）にあって
帯の高さに入るので、帯を前に置かないと、表示の上にポインタが来たときに帯から出た扱いに
なり、コマが消える。

**Alternatives considered**:

- `<canvas>` にコマを描く: シートの復号と描画を自前で持ち、拡大のぼけ方が `<img>` と
  変わる。CSS の背景で足りる。却下。
- `<img src=sheet>` を `object-fit: none` と `object-position` で切り出す: `object-position`
  はコマの大きさの整数倍のずれを割合で表しにくく、プレイヤーと別の計算になる。却下。

## R-6: 時間表示とバーの差し替え

**Decision**: 帯にいる間、カードの再生時間の表示を「スクラブ位置 / 動画の長さ」に、視聴位置の
バーをスクラブ位置のバーに差し替える。文字列は `web/src/i18n/en.ts` に
`t.list.card.scrubTime(position, duration)` を足して作る。バーは既存の `role="progressbar"`
の要素を使い回さず、帯にいる間だけ `aria-hidden` の別の要素を同じ場所に出す。
色・区切り・端の見え方は design 段階の `ui-design.md` が決める。

**Rationale**: 要件 4 と UI 品質（既存の表示を兼用し、新しい文字の指定を増やさない）。
視聴位置の `progressbar` は支援技術に「視聴した割合」を伝える要素で、ポインタの位置で
値を書き換えると読み上げが意味を失う（010 の ui-design.md「Accessibility」: ポインタ専用の
一時的な視覚情報は読み上げに割り込ませない）。固定の文字列（` / `）は
`web/src/i18n/` の外に置けない（ARCHITECTURE.md「Web layer」の ESLint の規則）。

**Alternatives considered**:

- 既存の `progressbar` の `aria-valuenow` をスクラブ位置で上書きする: 上のとおり読み上げの
  意味が変わる。却下。
- 時刻を帯の上の吹き出し（プレイヤーと同じ `.vv-seek-preview-time`）で出す: UI 品質は
  「時刻は既存の再生時間の表示を兼用」と言う。却下。
