import { Search, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";

import { MAX_QUERY_LENGTH } from "../api/client";
import { cn } from "../lib/cn";

/** searchDebounceMs は入力が落ち着くのを待つ時間。打鍵ごとに一覧が入れ替わらないようにする。 */
export const searchDebounceMs = 250;

/**
 * SearchBox は URL の `?q=` と結びついた検索欄である。
 * `/` でフォーカス、Esc でクリアしてフォーカスを外す。
 */
export default function SearchBox({ className }: { className?: string }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const query = (searchParams.get("q") ?? "").trim().slice(0, MAX_QUERY_LENGTH);

  const [input, setInput] = useState(query);
  const committed = useRef(query);
  const field = useRef<HTMLInputElement | null>(null);

  const commit = useCallback(
    (next: string) => {
      committed.current = next;
      setSearchParams(
        (current) => {
          const params = new URLSearchParams(current);
          if (next === "") params.delete("q");
          else params.set("q", next);
          return params;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  // 戻る・進むで URL 側が変わったら入力欄を追従させる
  useEffect(() => {
    if (query !== committed.current) {
      committed.current = query;
      setInput(query);
    }
  }, [query]);

  useEffect(() => {
    const next = input.trim();
    if (next === query) return;
    const timer = setTimeout(() => commit(next), searchDebounceMs);
    return () => clearTimeout(timer);
  }, [commit, input, query]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target as HTMLElement | null;
      if (target?.closest("input, textarea, [contenteditable=true]")) return;
      event.preventDefault();
      field.current?.focus();
      field.current?.select();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const clear = () => {
    setInput("");
    commit("");
    field.current?.focus();
  };

  return (
    <div className={cn("group relative flex h-9 w-full items-center", className)}>
      <Search className="pointer-events-none absolute left-3 size-4 text-fg-subtle transition-colors group-focus-within:text-fg-muted" />
      <input
        ref={field}
        type="search"
        value={input}
        onChange={(event) => setInput(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.preventDefault();
            clear();
            field.current?.blur();
          }
        }}
        maxLength={MAX_QUERY_LENGTH}
        placeholder="検索"
        aria-label="動画を検索"
        autoComplete="off"
        spellCheck={false}
        className={cn(
          "h-full w-full rounded-md border border-border bg-field pr-9 pl-9 text-sm text-fg shadow-[inset_0_1px_2px_var(--color-border)]",
          "placeholder:text-fg-subtle transition-[border-color,box-shadow] duration-150",
          "focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent-soft",
          "[&::-webkit-search-cancel-button]:hidden",
        )}
      />
      {input !== "" && (
        <button
          type="button"
          onClick={clear}
          aria-label="検索語をクリア"
          className="absolute right-1.5 flex size-6 items-center justify-center rounded-sm text-fg-muted transition-colors hover:bg-hover-wash hover:text-fg"
        >
          <X className="size-4" />
        </button>
      )}
      {input === "" && (
        <kbd className="pointer-events-none absolute right-3 hidden rounded-sm border border-border-strong px-1.5 font-sans text-[11px] text-fg-subtle sm:block">
          /
        </kbd>
      )}
    </div>
  );
}
