import type { ReactNode } from "react";

import { cn } from "../lib/cn";

const tones = {
  neutral: "bg-elevated text-fg",
  // bg-elevated の面（ModalFrame など）の中で使うと neutral と色が重なって面が
  // 消えるための代わり（ui-design.md「Synonyms」、tokens.test.ts の pairs
  // ["fg", "bg"] で検査済み）。
  onElevated: "bg-bg text-fg",
  accent: "bg-accent-soft text-link",
  success: "bg-success-soft text-success",
  warning: "bg-warning-soft text-warning",
  danger: "bg-danger-soft text-danger",
};

export default function Chip({
  children,
  tone = "neutral",
  className,
  title,
}: {
  children: ReactNode;
  tone?: keyof typeof tones;
  className?: string;
  /** 省略されるときに全体を見せる。 */
  title?: string;
}) {
  return (
    <span
      title={title}
      className={cn(
        "inline-flex h-6 items-center gap-1 rounded-sm px-2 text-xs font-medium whitespace-nowrap tabular-nums [&>svg]:size-3.5",
        tones[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}
