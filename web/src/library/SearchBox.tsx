import { Search, X } from "lucide-react";
import { type RefObject, useCallback, useEffect, useRef, useState } from "react";

import { MAX_QUERY_LENGTH } from "../api/client";
import { cn } from "../lib/cn";
import { type HistoryMode, normalizeQuery, SearchSession } from "./listCriteria";

/** searchDebounceMs は入力が落ち着くのを待つ時間。打鍵ごとに一覧が入れ替わらないようにする。 */
export const searchDebounceMs = 250;

export interface SearchBoxProps {
  /** URL から読んだ今の検索語。戻る・進むで変わると入力欄が追従する。 */
  query: string;
  /**
   * onCommit は検索語を確定する。mode は履歴の増やし方で、フォーカスが入ってから
   * 外れるか Esc で抜けるまでの一続きで最初の確定だけが push になる
   * （specs/013-library-search/contracts/list-url.md §3）。
   */
  onCommit: (next: string, mode: HistoryMode) => void;
  /** 入力欄を外から指す（一致なしの「条件を解除」でフォーカスを戻す）。 */
  inputRef?: RefObject<HTMLInputElement | null>;
  /** 読み上げ名。 */
  label?: string;
  placeholder?: string;
  className?: string;
}

/**
 * SearchBox は一覧の条件の `q` を入力する検索欄である。
 * `/` でフォーカス、Esc でクリアしてフォーカスを外す。
 */
export default function SearchBox({
  query,
  onCommit,
  inputRef,
  label = "動画を検索",
  placeholder = "検索",
  className,
}: SearchBoxProps) {
  const [input, setInputState] = useState(query);
  // 最新の入力。blur は同じ操作の中の setInput より先に走ることがあるので、
  // 描画を待たずに読める控えを持つ。
  const latest = useRef(query);
  const setInput = useCallback((next: string) => {
    latest.current = next;
    setInputState(next);
  }, []);
  const committed = useRef(query);
  const ownField = useRef<HTMLInputElement | null>(null);
  const field = inputRef ?? ownField;
  const session = useRef(new SearchSession());

  const commit = useCallback(
    (next: string) => {
      if (next === committed.current) return;
      committed.current = next;
      onCommit(next, session.current.commit());
    },
    [onCommit],
  );

  // 戻る・進むや「条件を解除」で URL 側が変わったら入力欄を追従させる
  useEffect(() => {
    if (query !== committed.current) {
      committed.current = query;
      setInput(query);
      // 外から変わった検索語は、打鍵の一続きの外である。次の入力は履歴を1つ増やす。
      session.current.end();
    }
  }, [query, setInput]);

  useEffect(() => {
    const next = normalizeQuery(input);
    if (next === committed.current) return;
    const timer = setTimeout(() => commit(next), searchDebounceMs);
    return () => clearTimeout(timer);
  }, [commit, input]);

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
  }, [field]);

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
        onFocus={() => session.current.start()}
        onBlur={() => {
          // 待っている確定があれば、続きを閉じる前に済ませる。
          commit(normalizeQuery(latest.current));
          session.current.end();
        }}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.preventDefault();
            setInput("");
            commit("");
            field.current?.blur();
          }
        }}
        maxLength={MAX_QUERY_LENGTH}
        placeholder={placeholder}
        aria-label={label}
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
