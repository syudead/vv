import { type KeyboardEvent, type ReactNode, useRef } from "react";

import type { UiText } from "../i18n";
import { cn } from "../lib/cn";

export interface TabItem<T extends string> {
  value: T;
  label: UiText;
  /** 名前の後ろに薄く添える数（件数）。 */
  count?: ReactNode;
  /** このタブが操作するパネルの id（`aria-controls`）。 */
  panelId: string;
}

/**
 * Tabs は下線のタブの並び（WAI-ARIA の tablist）である。選んでいるタブだけが Tab の
 * 順に入り、左右の矢印と Home・End でタブの間を動いて選ぶ（自動で切り替える）。
 * パネルは呼び手が `role="tabpanel"` と `aria-labelledby`（`tabId`）で描く。
 */
export default function Tabs<T extends string>({
  label,
  value,
  onValueChange,
  items,
  idPrefix,
  className,
}: {
  /** タブの並びの読み上げ名。 */
  label: UiText;
  value: T;
  /**
   * タブを選んだとき。切り替えを断るときは false を返す — 矢印・Home・End で選んだ
   * ときも、フォーカスを今のタブに残す（選ばれていないタブへ移さない）。
   */
  onValueChange: (value: T) => boolean | void;
  items: readonly TabItem<T>[];
  /** タブの id の接頭辞。`tabId(idPrefix, value)` がタブの id になる。 */
  idPrefix: string;
  className?: string;
}) {
  const refs = useRef(new Map<T, HTMLButtonElement>());

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = items.findIndex((item) => item.value === value);
    let next: number | undefined;
    if (event.key === "ArrowRight") next = (index + 1) % items.length;
    else if (event.key === "ArrowLeft") next = (index - 1 + items.length) % items.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = items.length - 1;
    if (next === undefined) return;
    event.preventDefault();
    const target = items[next]!.value;
    if (onValueChange(target) === false) return;
    refs.current.get(target)?.focus();
  }

  return (
    <div
      role="tablist"
      aria-label={label}
      onKeyDown={onKeyDown}
      className={cn("flex gap-6 border-b border-border", className)}
    >
      {items.map((item) => {
        const selected = item.value === value;
        return (
          <button
            key={item.value}
            ref={(node) => {
              if (node) refs.current.set(item.value, node);
              else refs.current.delete(item.value);
            }}
            type="button"
            role="tab"
            id={tabId(idPrefix, item.value)}
            aria-selected={selected}
            aria-controls={item.panelId}
            tabIndex={selected ? 0 : -1}
            onClick={() => onValueChange(item.value)}
            className={cn(
              "-mb-px flex h-10 items-center gap-1.5 border-b-2 px-0.5 text-sm font-medium whitespace-nowrap transition-colors",
              "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
              selected
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {item.label}
            {item.count !== undefined && (
              <span className="font-normal text-muted-foreground tabular-nums">
                {item.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}

/** tabId はタブの id である。パネルの `aria-labelledby` に使う。 */
export function tabId(prefix: string, value: string): string {
  return `${prefix}-tab-${value}`;
}
