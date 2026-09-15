import type { ReactElement, ReactNode } from "react";

/**
 * 画面で使うアイコンである（R-510）。
 *
 * 外部のアイコン集（lucide-react 等）を依存に足さない。必要なのは 15 個で、
 * そのために数百個を抱える集を入れると実行時の依存と版上げの対象が 1 つ増える。
 * 本機能は「依存を 1 つも増やさない」と宣言できるほど必要数が少ない。
 *
 * 塗りと線は currentColor にしてトークンに従わせる。色をここに書かないので、
 * 呼び出し側が text-inert・text-accent などを与えればそのまま染まる
 * （生の色の禁止は theme/noRawColors.test.ts が見ている）。
 *
 * aria-hidden と focusable は **このファイルの中で一律に付ける**。呼び出し側に
 * 書かせると、後から足したアイコンが読み上げに漏れる。アイコンは意味を持たない
 * 装飾で、意味は必ず隣の文言か aria-label が持つ（contracts/components.md 3.）。
 */
export type IconName =
  | "film"
  | "clock"
  | "heart"
  | "folder"
  | "tag"
  | "search"
  | "filter"
  | "settings"
  | "grid"
  | "list"
  | "close"
  | "chevron"
  | "info"
  | "alert"
  | "error";

/**
 * shapes は輪郭だけを持つ。viewBox・線の太さ・読み上げの扱いは Icon が一律に
 * 与えるので、ここに繰り返さない。すべて 24×24 の座標系で描く。
 */
const shapes: Record<IconName, ReactNode> = {
  // 映画 — フィルムの帯。左右の送り穴を縦線で表す
  film: (
    <>
      <rect x="2.5" y="4.5" width="19" height="15" rx="2" />
      <path d="M7.5 4.5v15M16.5 4.5v15M2.5 12h19" />
    </>
  ),

  // 時計 — 最近追加
  clock: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 7.5V12l3 2" />
    </>
  ),

  // ハート — お気に入り
  heart: <path d="M12 20.5 4.5 13a4.6 4.6 0 0 1 7.5-5.2 4.6 4.6 0 0 1 7.5 5.2Z" />,

  // フォルダ — コレクション
  folder: (
    <path d="M3 6.5a1.5 1.5 0 0 1 1.5-1.5h4l2 2.5h8A1.5 1.5 0 0 1 20 9v9a1.5 1.5 0 0 1-1.5 1.5h-14A1.5 1.5 0 0 1 3 18Z" />
  ),

  // タグ
  tag: (
    <>
      <path d="M11 3H3v8l10 10 8-8L11 3Z" />
      <circle cx="7" cy="7" r="1.4" />
    </>
  ),

  // 虫眼鏡 — 検索
  search: (
    <>
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="m15.5 15.5 4.5 4.5" />
    </>
  ),

  // 漏斗 — 絞り込み
  filter: <path d="M3.5 5h17l-6.5 7.5v6.5l-4 2v-8.5L3.5 5Z" />,

  // 歯車 — 設定
  settings: (
    <>
      <circle cx="12" cy="12" r="3" />
      <circle cx="12" cy="12" r="7" />
      <path d="M12 5V2.5M12 19v2.5M19 12h2.5M2.5 12H5M16.95 7.05l1.77-1.77M5.28 18.72l1.77-1.77M16.95 16.95l1.77 1.77M5.28 5.28l1.77 1.77" />
    </>
  ),

  // 格子 — 一覧の表示切替（タイル）
  grid: (
    <>
      <rect x="3.5" y="3.5" width="7" height="7" rx="1.5" />
      <rect x="13.5" y="3.5" width="7" height="7" rx="1.5" />
      <rect x="3.5" y="13.5" width="7" height="7" rx="1.5" />
      <rect x="13.5" y="13.5" width="7" height="7" rx="1.5" />
    </>
  ),

  // 一覧 — 表示切替（行）
  list: (
    <>
      <path d="M9 6h11M9 12h11M9 18h11" />
      <circle cx="4.5" cy="6" r="1.3" />
      <circle cx="4.5" cy="12" r="1.3" />
      <circle cx="4.5" cy="18" r="1.3" />
    </>
  ),

  // × — 閉じる・打ち消し
  close: <path d="m5.5 5.5 13 13M18.5 5.5l-13 13" />,

  // 下向きの山 — 選択欄の既定の矢印の代わり（C11）
  chevron: <path d="m6 9.5 6 6 6-6" />,

  /*
   * 知らせの 3 段階（C13 / spec US4-5）。
   *
   * 段階の区別を色だけに載せない。色覚の違いや暗い画面では、地の色と文字色の
   * 違いだけでは「情報」と「警告」を読み分けられない ── 丸・三角・丸に × と
   * **輪郭の形そのもの**を変えることで、色を見なくても段階が分かる。
   *
   * 点（`M12 8h.01` のような長さ 0 の線）は strokeLinecap="round" が丸く
   * 描く。感嘆符の点と「i」の点はこの書き方で足す。
   */

  // 丸に i — 情報
  info: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 11.5v4.5M12 8h.01" />
    </>
  ),

  // 三角に ! — 警告
  alert: (
    <>
      <path d="M12 4.5 21 19.5H3Z" />
      <path d="M12 10v3.5M12 16.5h.01" />
    </>
  ),

  // 丸に × — エラー
  error: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="m9 9 6 6M15 9l-6 6" />
    </>
  ),
};

/**
 * Icon は輪郭を 1 つ描く。
 *
 * 大きさは className で与える（既定は本文に添える 1rem 四方）。色は与えない —
 * currentColor なので、囲みの文字色がそのまま伝わる。
 */
export default function Icon({
  name,
  className = "h-4 w-4",
}: {
  name: IconName;
  className?: string;
}): ReactElement {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {shapes[name]}
    </svg>
  );
}
