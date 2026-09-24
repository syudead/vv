import { Search, X } from "lucide-react";
import { type RefObject, useEffect } from "react";

import { cn } from "../lib/cn";

/**
 * TagSearchBox はタグ管理画面の一覧をその場で絞る検索欄である（Issue 271「検索」）。
 * `web/src/videoList/SearchBox.tsx` と同じ外見（枠・アイコン・クリアボタン・`/`
 * ショートカット）だが、URL を読み書きせず、確定も待たずにその場で絞り込む
 * （API は増やさない）。
 */
export default function TagSearchBox({
  value,
  onChange,
  inputRef,
  disabled = false,
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  inputRef: RefObject<HTMLInputElement | null>;
  disabled?: boolean;
  className?: string;
}) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target as HTMLElement | null;
      if (target?.closest("input, textarea, [contenteditable=true]")) return;
      event.preventDefault();
      inputRef.current?.focus();
      inputRef.current?.select();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [inputRef]);

  return (
    <div className={cn("group relative flex h-9 items-center", className)}>
      <Search className="pointer-events-none absolute left-3 size-4 text-fg-subtle transition-colors group-focus-within:text-fg-muted" />
      <input
        ref={inputRef}
        type="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape" && value !== "") {
            event.preventDefault();
            onChange("");
          }
        }}
        placeholder="タグを検索"
        aria-label="タグを検索"
        disabled={disabled}
        autoComplete="off"
        spellCheck={false}
        className={cn(
          value === "" ? "pr-9" : "pr-9",
          "h-full w-full rounded-md border border-border bg-field pl-9 text-sm text-fg shadow-[inset_0_1px_2px_var(--color-border)]",
          "placeholder:text-fg-subtle transition-[border-color,box-shadow] duration-150",
          "focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent-soft",
          "disabled:opacity-50",
          "[&::-webkit-search-cancel-button]:hidden",
        )}
      />
      {value !== "" && (
        <button
          type="button"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onChange("")}
          aria-label="検索語をクリア"
          className="absolute right-1.5 flex size-6 items-center justify-center rounded-sm text-fg-muted transition-colors hover:bg-hover-wash hover:text-fg"
        >
          <X className="size-4" />
        </button>
      )}
    </div>
  );
}
