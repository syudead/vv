# UI Design: Library Hover Preview

## Design Basis

この artifact は [GitHub Issue #134](https://github.com/syudead/vv/issues/134) の `UI品質とアクセシビリティ` を、実装と画像で判定できる形へ具体化する。カード幅、配色、書体、overlay、focus、selection の既存規則は [library-ui.md](../../docs/design-docs/library-ui.md) と `web/src/index.css` の role token を正本とし、ここでは変更しない。

利用者が改善を感じる点は、似た thumbnail/title の動画を開き直さず、一覧の位置と操作を保ったまま全尺の内容を短時間で判別できることにある。現在の不便は、本編へ遷移しないと静止画の外側を確認できないこと。preview を目立つ新機能として飾るのではなく、静止画の場所が自然に動き出すことを価値とする。

## Screen Boundary

- 対象は library の grid `VideoCard` にある 16:9 thumbnail surface だけ。list row、toolbar、selection bar、player は変えない。
- preview video は thumbnail と同じ inset、crop、aspect ratio を使う absolute layer とし、card、grid、metadata の寸法計算へ参加しない。
- title、added/size/codec、quality/duration、watched、progress、selection checkbox、focus ring の意味と位置を維持する。probe pending/failed または duration/codec 不足で既存の全面 warning が出る card は preview job 自体が成立しないため、preview eligibility と排他的である。原本が browser-incompatible でも probe metadata が揃い preview が done なら、現在の `unplayableText` は warning を返さず preview を覆わない。
- 新しい text、badge、spinner、toolbar、audio/seek control は出さない。生成中・未生成・失敗・再生 error は現在の thumbnail/placeholder のまま表す。

## Interaction

1. `pointerType === "mouse"` の pointer が eligible card に入り、同じ card 内に 400ms 留まると、その card を active preview として取得開始する。400ms は既存 tooltip の hover delay と同じで、一覧を横切るだけの操作を media request にしない。
2. thumbnail/placeholder は最初の `playing` event まで表示し続ける。metadata 待ちや初回 buffering のために blank surface や loading indicator を見せない。
3. 再生開始後は同じ surface 内で muted、inline の preview を表示し、clip 終端では loop する。再生後の `waiting`/`stalled` では現在の video frame を維持し、`playing` でそのまま再開する。error になった場合だけ thumbnail へ戻す。動く内容そのものを preview 中の印とし、静止 frame だけでは新しい状態表示を増やさない。
4. pointer leave、別 card の activation、click navigation、filter/sort/page/list追加、grid/list 切替、zoom/viewport resize、unmount で即座に timer と再生を止め、media resource を解放して thumbnail/placeholder へ戻す。
5. `play()` rejection、network/media error、asset 404 でも thumbnail/placeholder を残し、error overlay や toast を増やさない。pointer が一度離れて再度入れば新しい試行を許可する。
6. selection mode では既存 checkbox と card click selection が優先される。checkbox 上の pointer は preview timer を開始せず、preview 中に selection mode が始まった場合も checkbox は最前面に残る。

`LibraryPage` は active card ID と reset epoch だけを調停する。card は timer、video element、playing/error state、resource cleanup を所有する。同時に見える active preview は1件だけとする。

## Visual Hierarchy

- thumbnail または preview が card の一次視覚情報、title と duration/progress が判別を支える情報である。preview は surface の外へ出ず、title より強い外枠や label を追加しない。
- quality/duration badge、watched mark、progress bar、selection checkbox は preview layer より前面に置く。preview 中の quality/duration badge と watched mark は半透明 `navbar` 背景を不透明な `navbar` 背景へ、progress track は半透明 `fg-subtle` を不透明な `fg-subtle` へ、未選択 checkbox は半透明 `navbar` を不透明な `navbar` へ切り替え、frame の明暗に依存しない識別性を持つ。実装は text/icon contrast の `fg/navbar`、`accent/navbar`、`success/navbar` を `tokens.test.ts` の pairs に追加する。checked checkbox の `accent-fg/accent` は既存 pair を使う。通常 thumbnail 状態の見た目は変えない。
- preview 中も card hover shadow とわずかな lift は現行どおりで、feature 固有の glow、色変更、拡大は足さない。thumbnail と video は同じ media-layer wrapper 内に置き、既存の hover scale を wrapper へ適用するため、`playing` への交換で crop/scale が跳ねない。

## Information Density

- card が同時に見せる text と control の数は増やさない。通常時、delay 中、preview 中、fallback 後で grid の card 数と折り返し位置を同じにする。
- preview が unavailable な理由は一覧へ追加表示しない。既存 thumbnail placeholder と状態表示だけを使い、詳細な失敗理由は job 確認経路が所有する。

## Spacing Rhythm

- `aspect-video`、card width token、grid gap、metadata padding、card radius をそのまま使う。
- video は `h-full w-full object-cover object-top` で thumbnail と同じ framing にする。開始・停止・buffering で surface 高、card 高、隣接 card、toolbar、selection bar、scroll positionを動かさない。
- 360px、768px、1280px の各 viewport で、preview 前後の同じ card の bounding box と grid wrapping が変わらないことを画像で確認する。

## Typography

- title、metadata、duration/quality、状態表示の font family、size、weight、line height、line clamp、tabular numbers を変更しない。
- preview 状態を可視 text や icon label として追加しないため、新しい typography token は不要。追加する contrast pairs は既存 overlay の不透明背景を検査する前節の3組だけとする。将来 visible label を追加する変更は、この design の再承認対象とする。

## Responsive And Motion

- viewport 幅による eligibility の分岐を作らない。touch 主体端末でも mouse event なら preview でき、touch/pen/focus では開始しない。
- layout は既存 CSS breakpoint と card width token に従う。JavaScript の resize handling は playback reset のためだけに使い、幅や配置を決定しない。
- `prefers-reduced-motion: reduce` では既存 CSS rule により card lift、thumbnail scale、opacity transition を実質停止する。利用者が明示的に mouse を留めた結果である video playback は利用できるが、thumbnail と first playing frame の交換、および停止時の復帰は fade/scale せず即時に行う。

## Accessibility

- keyboard focus だけでは preview を開始しない。Tab で card link と checkbox に到達でき、focus-visible outline、Enter navigation、Space/click selection、Esc selection clear を既存どおり使える。
- preview video は装飾的な補助内容として accessibility tree へ重複した題名や control を追加しない。card link の accessible name は title のままにする。
- video は常に muted、`playsInline`、controls なし。音声を有効にする経路と progress update を持たない。
- screen reader では preview の開始・停止を live announcement しない。pointer 専用の一時的視覚情報であり、keyboard 操作の読み上げを割り込ませない。

## System States

| State | Visible result |
| --- | --- |
| eligible, idle | 現在の thumbnail/placeholder と overlays |
| 400ms delay | idle と同一。request、spinner、label なし |
| initial loading/buffering | thumbnail/placeholder を維持 |
| playing | thumbnail surface の内容だけを loop preview に置換。既存 overlays は前面 |
| waiting/stalled after playing | 現在の preview frame を維持し、再開を待つ |
| pending/failed/missing preview | idle のまま。source stream fallback なし |
| play/media error | idle へ即時復帰し、再 hover を許可 |
| focus/touch/pen、checkbox の直接操作 | 既存 interaction のみ。preview 開始なし |

## Observable Review Criteria

### Screenshots

- 360px、768px、1280px で、同じ fixture card の `before`、`playing`、`error fallback` を撮る。
- 各幅で card 外形、grid gap、toolbar、selection bar、title/metadata、duration/quality、progress、watched、checkbox に overlap または layout shift がない。
- thumbnail と異なる sampled frame が surface 内に表示され、外側の新しい badge/control がないことで、内容確認が card density を変えずに成立している。
- browser-incompatible source だが preview 生成済みの fixture と、preview pending/failed fixture を同じ一覧に置く。画像では前者が thumbnail と異なる sampled frame を表示し、全面 warning に覆われないこと、後者が既存表示のままであることを示す。動作の有無は下の browser evidence で証明する。

### Interaction

- 400ms 未満の横切りでは request/再生がなく、400ms 留まると active card 1件だけが再生する。
- mouse を別 card へ移す、filter/sort/load-more/zoom/view を変える、viewport を resize する、card を開く各操作で前の video が停止し、thumbnail に戻る。
- selection mode、checkbox、card click、Enter、Esc、touch scroll/tap、pen input が preview より優先される。
- network log は preview endpoint だけを示し、source stream、live transcode、progress PUT/beacon を示さない。
- browser test または Playwright trace で、done fixture の video `currentTime` が進むこと、pending/failed fixture は video element と preview request を作らないこと、active video が常に1件以下であることを示す。

### Accessibility And Motion

- keyboard-only path で card focus、Enter navigation、checkbox selection、Esc clear を確認し、focus だけで media request が起きない。
- accessible tree に preview 固有の重複 link、button、video control、live region が増えない。
- 親 Issue が受け入れ条件とする keyboard/focus semantics は、card focus、Enter navigation、checkbox selection、Esc clear を通る keyboard-only path の request log と、mouse preview の開始前・再生中・停止後で page の accessible tree が不変である Playwright ARIA snapshot を必須証跡とする。preview 固有 control/live region がなく、focus だけでは media request が発生しないことを検査し、この証跡がない実装 PR は Design acceptance 未達とする。
- 実際の読み上げ音は ARIA snapshot では証明できないため、自動証跡の代替済みとは扱わない。対話可能な Windows 環境では追加の探索的確認として Narrator で card link の題名、checkbox、focus 順を読み上げ、mouse preview の開始・loop・停止が announcement を割り込ませないことを確認する。native screen reader を操作できない実装環境では未確認であることを PR の残余リスクへ明記するが、親 Issue が spoken-output の確認を受け入れ条件に含めていないため、この探索的確認だけは merge gate にしない。
- `prefers-reduced-motion: reduce` で card/thumbnail の装飾 transition が止まり、400ms 後の preview、停止、error fallback はちらつきや blank frame なしで利用できる。

### Human Visual Review

実装 PR の画像を、要求、通常 card、preview、fallback の順に並べ、次を人が判定する。

- 動く内容が thumbnail の代替として自然で、title/状態表示より強い別 UI に見えない。
- 一覧の情報量、scan rhythm、card 間隔が通常状態と同じで、管理画面としての密度を損なわない。
- title と metadata の読みやすさ、overlay の contrast、操作対象の優先順位が preview の frame 内容に左右されない。
- 静止画から動画への交換が「壊れた画像」「読み込み中」「本編が始まった」と誤認されず、card click が本再生であることを既存 interaction が保っている。
