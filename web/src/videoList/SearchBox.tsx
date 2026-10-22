import { Search, X } from "lucide-react";
import { type RefObject, useCallback, useEffect, useRef, useState } from "react";

import { MAX_QUERY_LENGTH } from "../api/client";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { limitCodePoints } from "../lib/codePointLimit";
import { isComposingKeyEvent } from "../lib/ime";
import { Button } from "../ui/shadcn/button";
import { Input } from "../ui/shadcn/input";
import { Kbd } from "../ui/shadcn/kbd";
import { type HistoryMode, normalizeQuery, SearchSession } from "./listCriteria";
import {
  type SearchSuggestions,
  type SuggestionOption,
  useSuggestionBox,
} from "./SearchSuggestions";
import SearchSyntaxHelp from "./SearchSyntaxHelp";

/** Debounce so each keystroke does not replace the list. */
export const searchDebounceMs = 250;

export interface SearchBoxProps {
  query: string;
  /** Push only the first commit within a focus session; later commits replace until blur or Escape. */
  onCommit: (next: string, mode: HistoryMode) => void;
  inputRef?: RefObject<HTMLInputElement | null>;
  label?: UiText;
  placeholder?: UiText;
  /** Disable syntax help for lists without video search syntax, such as tag management. */
  syntaxHelp?: boolean;
  /** Zero permits per-keystroke server search with obsolete request cancellation. */
  debounceMs?: number;
  /** Count code points; override for endpoints with different limits, such as activity's 200. */
  maxLength?: number;
  disabled?: boolean;
  className?: string;
  /** Filters offered under the field while typing; picking one clears the typed text. */
  suggestions?: SearchSuggestions;
  /**
   * `enter` searches only on Enter (or clearing), never while typing or on blur, so unconfirmed
   * IME text and half-typed words do not replace the list.
   */
  commitOn?: "typing" | "enter";
}

/**
 * Match server and URL code-point limits rather than HTML maxLength's UTF-16 units; truncate
 * pasted insertions without splitting emoji.
 */
export function limitQueryInput(
  next: string,
  previous = "",
  caret?: number,
  maxLength = MAX_QUERY_LENGTH,
): string {
  return limitCodePoints(next, maxLength, previous, caret);
}

export default function SearchBox({
  query,
  onCommit,
  inputRef,
  label = t.list.search.label,
  placeholder = t.list.search.placeholder,
  syntaxHelp = true,
  debounceMs = searchDebounceMs,
  maxLength = MAX_QUERY_LENGTH,
  disabled = false,
  className,
  suggestions,
  commitOn = "typing",
}: SearchBoxProps) {
  const onEnter = commitOn === "enter";
  const [input, setInputState] = useState(query);
  const [composing, setComposing] = useState(false);
  // Keep current input in a ref because blur may run before setInput renders.
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

  useEffect(() => {
    if (query !== committed.current) {
      committed.current = query;
      setInput(query);
      // External query changes end the typing session; the next input pushes history.
      session.current.end();
    }
  }, [query, setInput]);

  useEffect(() => {
    // Read the updated ref, not stale render input, after URL synchronization; otherwise changing onCommit identity can restore a cleared query.
    if (composing || onEnter) return;
    const next = normalizeQuery(latest.current, maxLength);
    if (next === committed.current) return;
    const timer = setTimeout(() => commit(next), debounceMs);
    return () => clearTimeout(timer);
  }, [commit, input, debounceMs, maxLength, composing, onEnter]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target as HTMLElement | null;
      if (target?.closest("input, textarea, [contenteditable=true]")) return;
      if (field.current === null || field.current.disabled) return;
      event.preventDefault();
      field.current?.focus();
      field.current?.select();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [field]);

  const pick = (option: SuggestionOption) => {
    // The page applies the filter and the cleared query in one history entry.
    setInput("");
    committed.current = "";
    session.current.end();
    suggestions?.onPick(option);
  };
  const box = useSuggestionBox({
    suggestions,
    text: normalizeQuery(input, maxLength),
    composing,
    debounceMs,
    onSearch: () => commit(normalizeQuery(latest.current, maxLength)),
    onPick: pick,
  });

  const clear = () => {
    setInput("");
    commit("");
    field.current?.focus();
  };

  return (
    <div className={cn("relative flex w-full items-center", className)}>
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute left-2 size-4 text-muted-foreground"
      />
      <Input
        ref={field}
        type="search"
        {...box.inputProps}
        value={input}
        onChange={(event) => {
          box.open();
          setInput(
            limitQueryInput(
              event.target.value,
              latest.current,
              event.target.selectionEnd ?? undefined,
              maxLength,
            ),
          );
        }}
        onCompositionStart={() => setComposing(true)}
        onCompositionEnd={(event) => {
          setComposing(false);
          setInput(
            limitQueryInput(
              event.currentTarget.value,
              latest.current,
              undefined,
              maxLength,
            ),
          );
        }}
        onFocus={() => session.current.start()}
        onBlur={() => {
          box.close();
          // Flush any pending commit before ending the typing session.
          if (!composing && !onEnter) commit(normalizeQuery(latest.current, maxLength));
          session.current.end();
        }}
        onKeyDown={(event) => {
          // IME Escape cancels composition, not the query.
          if (isComposingKeyEvent(event)) return;
          if (box.onKeyDown(event)) return;
          if (event.key === "Enter" && onEnter) {
            event.preventDefault();
            commit(normalizeQuery(latest.current, maxLength));
            return;
          }
          if (event.key === "Escape") {
            event.preventDefault();
            setInput("");
            commit("");
            field.current?.blur();
          }
        }}
        placeholder={placeholder}
        aria-label={label}
        disabled={disabled}
        autoComplete="off"
        spellCheck={false}
        className={cn(
          "h-8 pl-8 [&::-webkit-search-cancel-button]:hidden",
          !syntaxHelp ? "pr-8" : input === "" ? "pr-8 sm:pr-16" : "pr-16",
        )}
      />
      <div className="absolute right-1 flex items-center gap-0.5">
        {input !== "" && (
          <Button
            variant="ghost"
            size="icon-sm"
            // Keep focus on mousedown so blur does not commit and add an extra history entry before Clear.
            onMouseDown={(event) => event.preventDefault()}
            onClick={clear}
            aria-label={t.list.search.clear}
            className="size-6"
          >
            <X aria-hidden="true" />
          </Button>
        )}
        {input === "" && <Kbd className="mr-1 hidden sm:inline-flex">/</Kbd>}
        {syntaxHelp && <SearchSyntaxHelp />}
      </div>
      {box.list}
    </div>
  );
}
