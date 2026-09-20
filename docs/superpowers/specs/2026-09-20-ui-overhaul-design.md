# UI 全面刷新（フロントのみ・プレイヤー本体を除く）

作成: 2026-09-20

## 目的

現状の UI は余白・階層・タイポグラフィ・操作部品の質感が素のままで、
モダンなプロダクト（YouTube / Stash）の水準に達していない。既存 API のまま、
フロントのシェル・ライブラリ画面・再生画面（`<video>` 以外）を作り直す。

## 決定事項

- 範囲: フロントのみ。`<video controls>` は触らない。バックエンド変更なし。
- 骨格: YouTube 型（トップバー + 折りたたみサイドバー）に Stash 的な情報密度
  （カードのホバーメタ、ツールバー）を乗せる。
- 再生画面: シアターモード（シェル無し）。上に戻る導線、下にタイトル・メタ・詳細。
- 部品: Tailwind v4 + Radix UI primitives（dropdown-menu / popover / tooltip /
  toggle-group / checkbox / collapsible）+ lucide-react。フォントは
  `@fontsource-variable/inter` と `@fontsource-variable/noto-sans-jp` でセルフホスト。
- 既存の spec 契約（specs/004, 005）と対応テストは従わず、削除して作り直す。
  「生の色を書かない」検査だけは新トークン体系で維持する。

## 1. デザインシステム（`web/src/index.css`）

- 背景 3 層: `bg` / `surface` / `elevated`。境界線は `border`（白 8%）と
  `border-strong`。文字は `fg` / `fg-muted` / `fg-subtle`。
- アクセント 1 色（青系）: `accent` / `accent-hover` / `accent-fg`。
- 状態色: `danger` / `warning` / `success`（文字 + 淡い背景のペア）。
- タイポ: Inter + Noto Sans JP、本文 14px、`tabular-nums`。
- 角丸: sm 6 / md 10 / lg 12。影は elevated 用 1 段。モーション 150ms ease-out、
  reduced-motion で無効。

## 2. シェル & ライブラリ

- トップバー 56px: メニュー・ロゴ / 中央検索（`/` でフォーカス）/ 右にスキャン。
- サイドバー 240 / 72px。ライブラリのみ live。「最近追加」「視聴途中」は押すと
  「準備中」トースト。<1024px で自動折りたたみ、<640px でドロワー。
- ライブラリ: 見出し + 件数、ツールバー（並び順 DropdownMenu / 絞り込み Popover /
  密度 ToggleGroup 3 段）。auto-fill グリッド、カード幅 200 / 260 / 340。
- カード: 16:9、ホバーで微ズーム + 下端グラデーションにメタ 1 行、尺バッジ、
  視聴済みチェック、進捗バー、再生不可は暗転 + 理由。題名 2 行 + 相対日時。
  選択チェックは左上、選択時は accent 枠。選択バーは下部に浮く。
- 状態: スケルトン / 空 / 検索 0 件 / エラー（再試行）。無限スクロール維持。

## 3. 再生画面

- シェル無し。48px ヘッダに「← ライブラリ」（`from` state か `/`）。
- プレイヤー領域は幅いっぱい・`aspect-video`・最大高さ `100dvh - ヘッダ`。
- 下に最大幅 1280px: タイトル行 + メタチップ列 / 再開通知・視聴済みチップ /
  折りたたみ「ファイル情報」。既存の InfoPanel は廃止。
- 既存の振る舞い（5 秒ごと保存・離脱時 beacon・再開・document.title）は維持。

## 4. 構造

```
src/
  app/          App.tsx, router
  ui/           Button, IconButton, Tooltip, Menu, Popover, ToggleGroup, Checkbox, Toast, Skeleton
  shell/        AppShell, TopBar, Sidebar, useSidebar
  library/      LibraryPage, LibraryToolbar, VideoGrid, VideoCard, SelectionBar, states
  player/       VideoPage, VideoMeta, FileDetails
  api/          既存維持
  preferences/  既存維持（density は 3 段のまま）
  lib/          format.ts（尺・サイズ・相対日時）
```

テストは `lib/format`、`preferences`、`api`、トークン検査、LibraryPage と
VideoPage のスモークのみ。

## 改訂（2026-09-20、Stash 実物確認後）

初版は YouTube に寄りすぎていた。stashapp/stash の README スクリーンショットと
`ui/v2.5/src/styles/_theme.scss` を確認し、次に改めた。

- 配色: `#202b33` の地、`#30404d` のカード、`#394b59` の面、`#137cbd` のアクセント、
  `#48aff0` のリンク（Blueprint dark）。書体はシステム書体。角丸 3–6px。
- 骨格: 上部ナビバーのみ（サイドバー廃止）。ナビはアイコン + ラベルの横並び。
- ライブラリ: 中央寄せのフィルタ帯（検索・絞り込み・並び順・グリッド/リスト・
  ズームスライダー 4 段）、その下と一覧の下に「n 件（合計尺 · 合計サイズ）」。
  カードは箱型（サムネイル端まで、右下に `720P 59:11` の文字、下に題名と
  日付・大きさ・コーデック）。固定幅で中央寄せ。リスト表示を追加。
- 再生画面: ナビ色のヘッダ、題名は大きめ、日付（左）と太字の解像度（右）、
  「詳細 / ファイル情報」のタブ。
