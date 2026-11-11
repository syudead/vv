import { ScanSearch, Search, X } from "lucide-react";
import {
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";

import { MAX_QUERY_LENGTH } from "../api/client";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { limitCodePoints } from "../lib/codePointLimit";
import { isComposingKeyEvent } from "../lib/ime";
import { Button } from "../ui/shadcn/button";
import { Input } from "../ui/shadcn/input";
import { Kbd } from "../ui/shadcn/kbd";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { type HistoryMode, normalizeQuery, SearchSession } from "./listCriteria";
import {
  type SearchSuggestions,
  type SuggestionOption,
  useSuggestionBox,
} from "./SearchSuggestions";
import SearchSyntaxHelp from "./SearchSyntaxHelp";

/** Debounce so each keystroke does not replace the list. */
export const searchDebounceMs = 250;

/** The owner's library field also searches by a file; see library-ui.md, Image search. */
export interface FileSearchField {
  /** Opens the search-by-file dialog. */
  onPick: () => void;
  /** The query chip while an image search shows; it replaces the search icon. */
  chip?: ReactNode;
  /** Ends the image search: `Enter` with text and `Escape` on an empty field call it while the chip shows. */
  onClear: () => void;
}

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
  fileSearch?: FileSearchField;
}

/** Room for the controls at the field's end: clear or the `/` hint, syntax help and the file button. */
function endPadding(syntaxHelp: boolean, empty: boolean, pick: boolean): string {
  if (!syntaxHelp) return pick ? "pr-16" : "pr-8";
  if (pick) return empty ? "pr-16 sm:pr-24" : "pr-24";
  return empty ? "pr-8 sm:pr-16" : "pr-16";
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
  fileSearch,
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

  const chip = fileSearch?.chip;
  return (
    <div
      className={cn(
        "relative flex w-full items-center",
        // With a chip the frame holds the chip and the borderless input, so both read as one field.
        chip !== undefined &&
          "h-8 gap-1 rounded-md border border-input bg-muted pl-0.5 hover:border-muted-foreground",
        className,
      )}
    >
      {chip === undefined ? (
        <Search
          aria-hidden="true"
          className="pointer-events-none absolute left-2 size-4 text-muted-foreground"
        />
      ) : (
        <div className="flex max-w-1/2 min-w-0 shrink-0">{chip}</div>
      )}
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
            const text = normalizeQuery(latest.current, maxLength);
            if (chip !== undefined && text !== "") fileSearch?.onClear();
            commit(text);
            return;
          }
          if (event.key === "Escape" && chip !== undefined && latest.current === "") {
            event.preventDefault();
            fileSearch?.onClear();
            return;
          }
          if (event.key === "Escape") {
            event.preventDefault();
            setInput("");
            commit("");
            field.current?.blur();
          }
        }}
        placeholder={chip === undefined ? placeholder : t.library.imageSearch.placeholder}
        aria-label={label}
        disabled={disabled}
        autoComplete="off"
        spellCheck={false}
        className={cn(
          "[&::-webkit-search-cancel-button]:hidden",
          chip === undefined
            ? "h-8 pl-8"
            : "h-full border-0 bg-transparent pl-1 hover:border-0",
          endPadding(syntaxHelp, input === "", fileSearch !== undefined),
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
        {fileSearch !== undefined && <PickFileButton onPick={fileSearch.onPick} />}
        {syntaxHelp && <SearchSyntaxHelp />}
      </div>
      {box.list}
    </div>
  );
}

function PickFileButton({ onPick }: { onPick: () => void }) {
  const label = t.library.imageSearch.pick;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          onClick={onPick}
          className="size-6 text-muted-foreground"
        >
          <ScanSearch aria-hidden="true" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
