import type { ReactNode } from "react";

/** Tone は知らせの種類である。地と文字のトークンがこれで決まる。 */
export type Tone = "empty" | "danger" | "warning" | "info";

const tones: Record<Tone, string> = {
  empty: "bg-surface-raised text-body",
  info: "bg-surface-raised text-body",
  danger: "bg-danger-surface text-danger",
  warning: "bg-warning-surface text-warning",
};

/**
 * StateNotice は空・失敗・警告・お知らせの共通の枠である
 * （FR-002 / contracts/screen-states.md 1.・2.）。
 *
 * **この部品は画面全体を置き換えない。** FR-002 が求めているのは、取得に失敗
 * しても画面のほかの部分（固定の帯、一覧へ戻る道）が使えたままであることで、
 * 失敗のたびに画面を 1 枚の失敗表示に差し替えると、利用者は再試行も検索も
 * できなくなる。呼び出し側は帯や戻る道を残したまま、この枠だけを差し込むこと。
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
  /** 任意の操作（再試行、検索語を消す、取り込む）。 */
  children?: ReactNode;
}) {
  return (
    <div className={`rounded-card border border-border p-6 text-sm ${tones[tone]}`}>
      <p className="font-medium">{title}</p>
      {description !== undefined && <div className="mt-2">{description}</div>}
      {children !== undefined && (
        <div className="mt-3 flex flex-wrap items-center gap-3">{children}</div>
      )}
    </div>
  );
}
