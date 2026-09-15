# 契約: UI コンポーネント C1〜C16

**Feature**: [../spec.md](../spec.md) | **Plan**: [../plan.md](../plan.md) |
**Research**: [R-503](../research.md) / [R-506](../research.md) / [R-507](../research.md)

FR-004 は C1〜C16 をすべて実装することを、FR-005〜FR-008 は機能するものと表示のみの
ものを分け、操作できるものが 4 状態を持つことを求めている。本書はその**一覧・置き場所・
状態**と、**表示のみの要素の規約**を定める。

実装はこの表を満たしていればよく、文言やクラス名の書き方までは定めない。

## 1. 一覧（SC-002 の 16 個）

| # | 名前 | 置き場所 | 機能 | 4 状態 | 対応 |
| --- | --- | --- | --- | --- | --- |
| C1 | Sidebar | `layout/Sidebar.tsx` | 器 | — | FR-001・FR-003 |
| C2 | Logo | `layout/Logo.tsx` | 見出し（`h1`）。**リンクにしない** | — | FR-003・[R-508](../research.md) |
| C3 | NavItem | `layout/NavItem.tsx` | 「すべての動画」だけ**する** | `live` のみ持つ | FR-005・FR-007 |
| C4 | SidebarSection | `layout/SidebarSection.tsx` | 器（見出し + 項目群） | — | FR-004 |
| C5 | Header | `layout/Header.tsx` | 器 | — | FR-001・FR-002 |
| C6 | Tab | `layout/Tab.tsx` | 「動画」だけ**する** | `live` のみ持つ | FR-005 |
| C7 | IconButton | `layout/IconButton.tsx` | **しない**（設定・フィルタ・表示切替） | — | FR-005 |
| C8 | VideoCard | `components/VideoCard.tsx` | **する** | 持つ | FR-007・spec US3 |
| C9 | DurationBadge | `components/VideoCard.tsx` の中 | 表示 | — | FR-009 |
| C10 | SearchInput | `components/SearchInput.tsx` | **する**（既存の題名検索） | 持つ | FR-007・spec 5. |
| C11 | Select | `components/Select.tsx` | **する**（既存のソート 2 種） | 持つ | FR-007・spec 5. |
| C12 | DensitySlider | `components/DensitySlider.tsx` | **する**（既存の 3 段階） | 持つ | FR-007・[R-506](../research.md) |
| C13 | Notice | `components/StateNotice.tsx` | 表示 | — | spec US4-5 |
| C14 | InfoPanel | `components/InfoPanel.tsx` | 器 | — | FR-015 |
| C15 | CloseButton | `components/InfoPanel.tsx` の中 | **する**（一覧へ戻る） | 持つ | FR-017 |
| C16 | MetaList | `components/MetaList.tsx` | 表示 | — | FR-016・[R-507](../research.md) |

**機能する対話要素は 7 つ**（C3 の 1 行・C6 の 1 つ・C8・C10・C11・C12・C15）で、これは
004 の時点で利用者が行えた操作と**同じ集合**である。**増えていないことが FR-013・SC-004 の
要求**である。

C2 がこの 7 つに入らないのは、[spec 2.「操作できるコンポーネントの状態」](../spec.md)が
4 状態を持つものとして C8・C10・C11・C12・C15・C3・C6 だけを挙げており、**C2 を挙げて
いない**ためである。ロゴを `/` へのリンクにすると、フォーカス可能な要素と `Tab` の停止
位置が 1 つ増え、検索中に押せば絞り込みが消える — 行き先が既存でも、**操作は 1 つ増える**。
FR-013 はそれを禁じている。ロゴは `h1` を持つだけの非対話の見出しにする。

## 2. 4 状態（FR-007 / FR-008）

C3（`live`）・C6（`live`）・C8・C10・C11・C12・C15 は次の 4 つを視覚的に区別する。
C2 は対話要素ではないので対象外である。

| 状態 | 表現 |
| --- | --- |
| 通常 | 既定の配色 |
| hover | 背景を 1 段明るく（`surface-raised` → その上） |
| focus | `--color-focus` の輪郭。`:focus-visible` で出し、要素の**外側**に描く（`outline-offset`） |
| active | 背景を 1 段暗く |

- focus はキーボード操作で必ず見える（FR-008）。`:focus-visible` を使うのは、ポインタで
  押しただけでは出さないためである（004 と同じ）。
- hover / active の遷移は `motion-reduce:` で止める。**色の最終状態は常に適用する**
  （FR-012。動きだけを止め、操作結果は判別できる）。
- 押せる要素の当たり判定は `--size-tap`（44px）四方以上を保つ（004 の FR-022 由来、
  FR-018 で引き継ぐ）。

## 3. 表示のみの要素の規約（FR-005 / FR-006）

対象は [data-model.md 2.](../data-model.md) の `kind: "inert"` の 12 行と、C7 の 3 つ
（設定・フィルタ・表示切替）を合わせた **15 個**である。

| 事項 | 規約 |
| --- | --- |
| 要素 | **対話要素にしない。** `<button>` にも `<a>` にもせず、`<span>` / `<li>` で描く |
| 属性 | `disabled` も `aria-disabled` も `role="button"` も付けない（[R-503](../research.md)） |
| tab 順 | 現れない（`tabIndex` を持たない） |
| 色 | `--color-inert`。機能する要素より淡い |
| 件数 | **出さない**（FR-005） |
| 読み上げ | ラベルの直後に `<span class="sr-only">（未実装）</span>` を添える（FR-006） |
| 4 状態 | 持たない。hover でも変化しない（押せないものが手応えを返すと誤解を生む） |

`disabled` / `aria-disabled` を使わない理由は「いまは使えない」という誤った意味を持つ
ためである。裏側の機能そのものが無いので、条件は永遠に揃わない。

### 機能が付くときの手順

[data-model.md 2.](../data-model.md) の表の `kind` を `"live"` に変え、`to` を足す。
描画側（C3 / C6）は `kind` だけで分岐しているので、部品の書き換えは要らない。

## 4. 部品ごとの要点

### C8 VideoCard / C9 DurationBadge（spec US3）

| 事項 | 規則 |
| --- | --- |
| 枠 | サムネイルとタイトル行を**1 つの枠**にまとめ、枠全体に `--radius-card` を掛ける |
| 比率 | サムネイルは 16:9 固定。サムネイルの有無で大きさが変わらない（004 の SC-003 を引き継ぐ） |
| 時間バッジ | サムネイルの右下。**尺が未取得なら出さない**（spec US3-3。現行の挙動） |
| 読める | バッジの地は `--color-badge`（不透明）。半透明にしない（FR-009） |
| 題名 | 2 行で省略。全文への到達手段 3 つ（004 の R-409）を保つ |
| 進捗線・視聴済み・再生できない印 | 004 のまま（spec のスコープ外） |

### C10 SearchInput / C11 Select / C12 DensitySlider（spec US4）

| 部品 | 規則 |
| --- | --- |
| C10 | 左端に虫眼鏡アイコン（`aria-hidden`）。入力そのものは `<input type="search">` のまま。待ち合わせ 250ms と `maxLength` を変えない |
| C11 | ブラウザ既定の見た目を外す（`appearance: none` + 自前の下向き記号）。**選択肢は従来どおり 2 つ**（`addedDesc` / `titleAsc`） |
| C12 | `<input type="range" min="0" max="2" step="1">`。位置と `Density` の読み替えは [data-model.md 3.](../data-model.md)。`aria-valuetext` に段階名を入れる。保存の形は変えない |

### C13 Notice（spec US4-5）

3 段階に整理する。004 の `Tone` から `empty` を落とし、`info` に寄せる。

| 段階 | 地 / 文字 | 使う場面 |
| --- | --- | --- |
| 情報 | `surface-raised` / `body` | 蔵書が空、該当なし、続きから再開した知らせ |
| 警告 | `warning-surface` / `warning` | 再生できない形式 |
| エラー | `danger-surface` / `danger` | 一覧の取得失敗、再生の失敗 |

色だけでなく**形からも判別できる**こと（左端の色帯やアイコン）が spec US4-5 の要求である。

再開の知らせは**動画プレーヤーを画面外へ押し出さない高さ**にする（spec US4-6）。
再生画面ではパネルの中に置くので、映像の大きさは変わらない（[layout.md 4.](./layout.md)）。

### C14 InfoPanel / C15 CloseButton / C16 MetaList（spec US5）

| 事項 | 規則 |
| --- | --- |
| 並び | × （右上）→ 題名 → 知らせ → 動画の情報 |
| C15 | パネルの右上。**映像に重ねない**（FR-017）。押すと遷移元の一覧へ戻り、スクロール位置も復元される（004 の振る舞いを変えない） |
| C16 | `<dl>` / `<dt>` / `<dd>` を保つ。値に等幅フォントを使わない。ラベルは `muted`、値は `body`、1 項目 1 行 |
| 項目 | 6 つを [data-model.md 4.](../data-model.md) の順序で。取れていない値の言い分けは 004 のまま |

## 5. この契約の検査

| 事項 | 検査 |
| --- | --- |
| C1〜C16 が存在する（SC-002） | 受け入れ検証 [S3](../quickstart.md)・[S4](../quickstart.md)・[S5](../quickstart.md)・[S6](../quickstart.md)（人） |
| 表示のみの要素が対話要素でなく、tab 順に無い | 単体テスト `layout/placeholders.test.tsx` |
| `live` が `all-videos` と `tab-videos` の 2 つだけである（FR-013） | 単体テスト（同上） |
| 表示のみの要素に件数が出ない | 単体テスト（同上） |
| 密度の読み替えが往復で恒等 | 単体テスト `components/DensitySlider.test.tsx` |
| C16 が 6 項目を順に持ち、値が等幅でない | 単体テスト `components/MetaList.test.tsx` |
| 4 状態と hover の見え方 | 受け入れ検証 [S3](../quickstart.md)（人） |
| 読み上げに「（未実装）」が届く | 受け入れ検証 [S7](../quickstart.md)（人） |
