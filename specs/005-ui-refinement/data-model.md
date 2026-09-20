# Data Model: 原案デザインに合わせた UI の再構築

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-15

本機能は**永続化する情報を 1 つも増やさない**。SQLite の表も、`api/openapi.yaml` の
スキーマも、`localStorage` の鍵も値の形も変わらない（FR-014）。004 が定義した
[表示設定](../004-library-ui/data-model.md)・[一覧の復元状態](../004-library-ui/data-model.md)・
[視聴状況](../004-library-ui/data-model.md)は、そのまま引き継ぐ。

ここで定義するのは、画面の**骨格が持つ静的な表**と、既存の値に対する**新しい読み替え**
だけである。

| 名前 | 置き場所 | 寿命 | 失われたときの振る舞い |
| --- | --- | --- | --- |
| [画面の領域](#1-画面の領域-region) | CSS のみ（状態を持たない） | — | — |
| [ナビゲーション項目](#2-ナビゲーション項目-navitem) | ソースの定数表 | ビルド時に固定 | — |
| [密度の位置](#3-密度の位置-densityindex読み替え) | 導出のみ（保存しない） | — | 既定（`standard` = 1） |
| [情報パネルの項目](#4-情報パネルの項目-metaitem読み替え) | 導出のみ（保存しない） | — | 言い分けを出す（004 のまま） |

---

## 1. 画面の領域 (Region)

spec の「1. レイアウト構造」に対応する。**状態を持たない** — どの領域が出るかは画面
（一覧か再生か）と画面幅だけで決まり、JavaScript の変数にはならない
（[R-501](./research.md) / [R-502](./research.md)）。

| 領域 | 一覧 `/` | 再生 `/videos/:id` | 寸法 | スクロール |
| --- | --- | --- | --- | --- |
| サイドバー | 幅 640px 以上でのみ表示 | 表示しない（FR-015） | 幅 `--size-sidebar`（240px）、高さ全画面 | 中身が溢れたら独立して流れる |
| ヘッダー | 常に表示 | 表示しない（FR-015） | 高さ `--size-header`（56px）、幅は残り全体 | しない |
| コンテンツ | 残り全体 | 全体（映像 + パネル） | — | 文書（ウィンドウ）が流れる |

寸法と境界の詳細は [contracts/layout.md](./contracts/layout.md) にある。

---

## 2. ナビゲーション項目 (NavItem)

spec の「3. 表示のみの要素」の表を、ソース上の 1 つの定数
（`web/src/layout/navigation.ts`）にする。**この表が「機能する / 表示のみ」の唯一の
真実**であり、描画側は `kind` で分岐するだけにする。機能が付くときは行を書き換える
（[plan.md](./plan.md) の G7）。

### 項目

| 項目 | 型 | 説明 |
| --- | --- | --- |
| `id` | 文字列 | 表の中で一意。テストが指す名前 |
| `label` | 文字列 | 画面に出る文言。原案のものをそのまま使う（spec の Assumptions） |
| `icon` | アイコン名 | `web/src/layout/icons.tsx` の 1 つ |
| `kind` | `"live"` / `"inert"` | `live` は機能する。`inert` は表示のみ（[R-503](./research.md)） |
| `to` | 文字列 / なし | `live` のときの行き先。`inert` では持たない |
| `section` | `"library"` / `"collection"` / `"tag"` / `"tab"` | 置かれる区画。並び順は表の順序 |

### 値

`kind` が `live` なのは 2 つだけである（spec 3. の表）。

| `section` | `id` | `label` | `kind` | 件数 |
| --- | --- | --- | --- | --- |
| `library` | `all-videos` | すべての動画 | `live` | 一覧の `total`（実数） |
| `library` | `recent` | 最近追加 | `inert` | 出さない |
| `library` | `favorites` | お気に入り | `inert` | 出さない |
| `library` | `unsorted` | 未整理 | `inert` | 出さない |
| `collection` | `collection-trip` | 旅行 | `inert` | 出さない |
| `collection` | `collection-live` | ライブ | `inert` | 出さない |
| `collection` | `collection-docs` | 資料 | `inert` | 出さない |
| `tag` | `tag-scenery` | 風景 | `inert` | 出さない |
| `tag` | `tag-trip` | 旅行 | `inert` | 出さない |
| `tag` | `tag-sea` | 海 | `inert` | 出さない |
| `tab` | `tab-videos` | 動画 | `live` | — |
| `tab` | `tab-images` | 画像 | `inert` | — |
| `tab` | `tab-collections` | コレクション | `inert` | — |
| `tab` | `tab-tags` | タグ | `inert` | — |

ヘッダーとツールバーのアイコンボタン（設定・フィルタ・表示切替）も `inert` である。
`section` を持たないので同じ表には入れず、[contracts/components.md](./contracts/components.md) 3. の
規約だけを共有する。

### 不変条件

- `kind: "inert"` の行は `to` を持たない。持っていれば型で落ちる。
- `kind: "inert"` の行は件数を受け取らない（描画側が渡さない。FR-005）。
- `live` は `all-videos` と `tab-videos` の 2 つだけである。**この 2 つ以外が `live` に
  なったら、それは利用者が行える操作が増えたということ**で、FR-013 の違反である。
  `layout/placeholders.test.tsx` がこの数を確かめる。

---

## 3. 密度の位置 (DensityIndex)（読み替え）

C12 DensitySlider のための読み替えである（[R-506](./research.md)）。**保存されない。**
保存されるのは 004 のままの `ViewPreferences.density`（`"dense"` / `"standard"` /
`"relaxed"`）である。

```text
steps = ["dense", "standard", "relaxed"]

位置 → 密度:  steps[index] ?? "standard"
密度 → 位置:  steps.indexOf(density)   （見つからなければ 1）
```

| 位置 | `density` | スライダーの読み上げ（`aria-valuetext`） |
| --- | --- | --- |
| 0 | `dense` | 細かい |
| 1 | `standard` | 標準 |
| 2 | `relaxed` | ゆったり |

### 不変条件

- 往復が恒等である（`toDensity(toIndex(d)) === d`）。3 つすべてで確かめる。
- 範囲外・非数値の位置は `standard` に落ちる。`<input type="range">` が範囲外を出すことは
  無いが、落とし先を決めておかないと壊れた値で画面が消える（004 の FR-019 と同じ考え）。
- `--size-tile-*` との対応は [004 の contracts/design-tokens.md](../004-library-ui/contracts/design-tokens.md) 3.
  のままで、本機能は格子の式に触れない。

---

## 4. 情報パネルの項目 (MetaItem)（読み替え）

C16 MetaList が描く 6 項目である。**004 の `VideoFacts` と同じ値・同じ言い分け**で、
体裁だけが変わる（[R-507](./research.md) / FR-016）。

| ラベル | 値の出どころ | 取れていないとき |
| --- | --- | --- |
| 長さ | `durationMs` を `formatDuration` で整形 | 言い分け（下記） |
| 解像度 | `width × height` | 言い分け |
| 形式 | `container` | 言い分け |
| 映像 | `videoCodec` | 言い分け |
| 音声 | `audioCodec` | 言い分け |
| 大きさ | `sizeBytes` を単位付きで整形 | 必須なので言い分けに入らない |

言い分けは 004 のまま変えない。

| `probeState` | 出す文言 |
| --- | --- |
| `pending` | 確認中 |
| `failed` | 読み取れませんでした（理由があれば括弧で添える） |
| それ以外（解析済みで値が無い） | なし |

### 不変条件

- 6 項目が**この順序**で並ぶ。
- ラベルと値が対で読み上げに渡る（`<dl>` / `<dt>` / `<dd>` を保つ）。
- 値に等幅フォントを使わない（FR-016）。

---

## 5. 変わらないもの

本機能で**変わらない**ことを明示しておく。差分レビューの基準になる。

| 事項 | 置き場所 |
| --- | --- |
| `api/openapi.yaml` とその生成物 | `api/`、`web/src/api/gen/` |
| `localStorage` の鍵と値の形（`vv.view.v1`） | `web/src/preferences/viewPreferences.ts` |
| 一覧の復元状態（メモリ、鍵は `q` と `sort`） | `web/src/api/listSnapshot.ts` |
| URL のクエリ（`q`・`sort`）の意味 | `web/src/pages/LibraryPage.tsx` |
| 1 ページ 60 件・カーソルの引き継ぎ | `web/src/api/useVideos.ts` |
| 検索の待ち合わせ 250ms と打ち切り | `web/src/pages/LibraryPage.tsx` |
| 再生位置の送信（5 秒ごと・離脱時の `keepalive` fetch）と再開の下限 5 秒 | `web/src/pages/VideoPage.tsx` |
| 取り込みの巡回（2 秒ごと、実行中だけ） | `web/src/components/ScanStatus.tsx` |
