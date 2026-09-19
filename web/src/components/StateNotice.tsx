import type { ReactNode } from "react";

import Icon, { type IconName } from "../layout/icons";

/**
 * Tone は知らせの段階である（contracts/components.md 4. の C13）。
 *
 * **3 つしかない**（spec US4-5）。004 にあった `empty` は落として `info` に
 * 寄せた ── 「蔵書が空」「該当なし」はどちらも利用者に伝える事実であって、
 * 失敗でも警告でもない。同じ見え方のものに 2 つの名前があると、呼び出し側
 * ごとに選び方が分かれて段階の意味が薄れる。
 */
export type Tone = "info" | "warning" | "danger";

/**
 * tones は段階ごとの地・文字・輪郭・アイコンである。
 *
 * **色だけで段階を分けない**（spec US4-5）。色覚の違いや暗い環境では地の色の
 * 差が届かないことがあるので、左端の色帯（`border-l-4`）と**形の違うアイコン**
 * （丸に i / 三角に ! / 丸に ×）を段階ごとに変える。どちらか一方が読み取れ
 * なくても、もう一方で段階が分かる。
 */
const tones: Record<Tone, { className: string; icon: IconName }> = {
  info: { className: "border-l-accent bg-surface-raised text-body", icon: "info" },
  warning: {
    className: "border-l-warning bg-warning-surface text-warning",
    icon: "alert",
  },
  danger: { className: "border-l-danger bg-danger-surface text-danger", icon: "error" },
};

/**
 * StateNotice は情報・警告・エラーの共通の枠である
 * （FR-002 / contracts/screen-states.md 1.・2.）。
 *
 * **この部品は画面全体を置き換えない。** FR-002 が求めているのは、取得に失敗
 * しても画面のほかの部分（固定の帯、一覧へ戻る道）が使えたままであることで、
 * 失敗のたびに画面を 1 枚の失敗表示に差し替えると、利用者は再試行も検索も
 * できなくなる。呼び出し側は帯や戻る道を残したまま、この枠だけを差し込むこと。
 *
 * 高さも抑える（spec US4-6）。再生画面では知らせがパネルの中に入るので、
 * 段階が増えても映像の大きさは変わらない（contracts/layout.md 4.）。
 */
export default function StateNotice({
  tone = "info",
  title,
  description,
  children,
}: {
  tone?: Tone;
  /** 見出し。何が起きたかを 1 行で示す。 */
  title: string;
  /** 本文。次に取れる操作の説明を置く。 */
  description?: ReactNode;
  /** 任意の操作（再試行、検索語を消す）。 */
  children?: ReactNode;
}) {
  const { className, icon } = tones[tone];

  return (
    <div
      className={`flex gap-3 rounded-card border border-l-4 border-border p-6 text-sm ${className}`}
    >
      {/* アイコンは段階の印である。意味は隣の見出しが持つので aria-hidden で
          よい（Icon が一律に付ける。contracts/components.md 3.）。 */}
      <Icon name={icon} className="mt-0.5 h-5 w-5 shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{title}</p>
        {description !== undefined && <div className="mt-2">{description}</div>}
        {children !== undefined && (
          <div className="mt-3 flex flex-wrap items-center gap-3">{children}</div>
        )}
      </div>
    </div>
  );
}
