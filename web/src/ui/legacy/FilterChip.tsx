import { X } from "lucide-react";
import type { ReactNode, Ref } from "react";

import type { UiText } from "../../i18n";
import { cn } from "../../lib/cn";

/**
 * FilterChip は「絞り込み中」の行に並べる、押すとその絞り込みを外すチップである。
 * ライブラリのタグの絞り込み（`library/ActiveTagFilters`）とタグ管理画面の
 * 「Tentative only」「Unused only」が同じ見た目を使う。末尾に × を描く。
 */
export default function FilterChip({
  label,
  onRemove,
  icon,
  children,
  ref,
}: {
  /** 読み上げ名（「〈名〉の絞り込みを外す」）。 */
  label: UiText;
  onRemove: () => void;
  /** 先頭の小さなアイコン。 */
  icon?: ReactNode;
  children: ReactNode;
  ref?: Ref<HTMLButtonElement>;
}) {
  return (
    <button
      ref={ref}
      type="button"
      aria-label={label}
      onClick={onRemove}
      className={cn(
        "flex h-6 items-center gap-1 rounded-sm bg-primary-soft px-1.5 text-xs text-primary",
        "hover:ring-1 hover:ring-inset hover:ring-input",
        "[&>svg]:size-3 [&>svg]:shrink-0",
      )}
    >
      {icon}
      {children}
      <X aria-hidden="true" />
    </button>
  );
}
