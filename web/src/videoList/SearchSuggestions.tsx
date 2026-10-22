import { Search } from "lucide-react";
import {
  type KeyboardEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useId,
  useState,
} from "react";

import { t, type UiText } from "../i18n";
import { Command, CommandGroup, CommandItem, CommandList } from "../ui/shadcn/command";

export interface SuggestionOption {
  /** Unique across groups; the owner of the suggestions decodes it on pick. */
  id: string;
  label: string;
  leading?: ReactNode;
  ariaLabel?: string;
}

export interface SuggestionGroup {
  heading: UiText;
  options: readonly SuggestionOption[];
}

/** Filters suggested under a search field while the viewer types. */
export interface SearchSuggestions {
  /** The text `groups` were found for; groups for any other text are not shown. */
  term: string;
  groups: readonly SuggestionGroup[];
  /** Receives the debounced, normalized text once IME composition has ended. */
  onTermChange: (term: string) => void;
  onPick: (option: SuggestionOption) => void;
}

interface Row {
  key: string;
  option: SuggestionOption | null;
}

const searchRow: Row = { key: "search", option: null };

export interface SuggestionBox {
  /** Combobox attributes for the input; empty without suggestions. */
  inputProps: Record<string, string | boolean | undefined>;
  /** Handles a key that is not part of IME composition; true when the key was used. */
  onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => boolean;
  open: () => void;
  close: () => void;
  list: ReactNode;
}

/**
 * The listbox under a search field: first `Search for "text"`, which keeps plain text search,
 * then each group's options. Arrow keys move the active row, Enter takes it (or searches when no
 * suggestion is active), Escape closes the list before it clears the field.
 */
export function useSuggestionBox({
  suggestions,
  text,
  composing,
  debounceMs,
  onSearch,
  onPick,
}: {
  suggestions: SearchSuggestions | undefined;
  /** The normalized text in the field. */
  text: string;
  composing: boolean;
  debounceMs: number;
  onSearch: () => void;
  onPick: (option: SuggestionOption) => void;
}): SuggestionBox {
  const [opened, setOpened] = useState(false);
  const [active, setActive] = useState(-1);
  const [listId, setListId] = useState<string | undefined>(undefined);
  const baseId = useId();
  const onTermChange = suggestions?.onTermChange;

  useEffect(() => {
    if (onTermChange === undefined || composing) return;
    const timer = setTimeout(() => onTermChange(text), debounceMs);
    return () => clearTimeout(timer);
  }, [onTermChange, text, composing, debounceMs]);

  useEffect(() => {
    setActive(-1);
  }, [text]);

  const groups =
    suggestions !== undefined && suggestions.term === text ? suggestions.groups : [];
  const rows: Row[] = [
    searchRow,
    ...groups.flatMap((group) =>
      group.options.map((option) => ({ key: option.id, option })),
    ),
  ];
  const visible = suggestions !== undefined && opened && !composing && text !== "";
  const current = visible && active < rows.length ? active : -1;
  const rowId = (index: number) => `${baseId}-suggestion-${String(index)}`;

  const take = (row: Row) => {
    setOpened(false);
    if (row.option === null) onSearch();
    else onPick(row.option);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>): boolean => {
    if (suggestions === undefined) return false;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!visible) {
        setOpened(true);
        setActive(0);
        return true;
      }
      const step = event.key === "ArrowDown" ? 1 : -1;
      setActive(Math.min(Math.max(current + step, -1), rows.length - 1));
      return true;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      take(rows[Math.max(current, 0)] ?? searchRow);
      return true;
    }
    if (event.key === "Escape" && visible) {
      event.preventDefault();
      setOpened(false);
      return true;
    }
    return false;
  };

  const open = useCallback(() => setOpened(true), []);
  const close = useCallback(() => setOpened(false), []);
  const captureList = useCallback((node: HTMLDivElement | null) => {
    setListId(node?.id);
  }, []);

  const inputProps =
    suggestions === undefined
      ? {}
      : {
          role: "combobox",
          "aria-autocomplete": "list",
          "aria-expanded": visible,
          "aria-controls": visible ? listId : undefined,
          "aria-activedescendant": current >= 0 ? rowId(current) : undefined,
        };

  let index = 0;
  const renderRow = (row: Row) => {
    const at = index;
    index += 1;
    return (
      <CommandItem
        key={row.key}
        value={row.key}
        aria-label={row.option?.ariaLabel}
        onSelect={() => take(row)}
        asChild
      >
        <div
          id={rowId(at)}
          aria-selected={at === current}
          data-selected={at === current}
          // Keep focus in the field so blur does not close the list before the pick.
          onMouseDown={(event) => event.preventDefault()}
          onMouseEnter={() => setActive(at)}
        >
          {row.option === null ? (
            <>
              <Search aria-hidden="true" className="text-muted-foreground" />
              <span className="min-w-0 truncate">
                {t.list.suggestions.searchFor(text)}
              </span>
            </>
          ) : (
            <>
              {row.option.leading}
              <span className="min-w-0 truncate">{row.option.label}</span>
            </>
          )}
        </div>
      </CommandItem>
    );
  };

  const list = visible && (
    <Command
      label={t.list.suggestions.label}
      shouldFilter={false}
      disablePointerSelection
      vimBindings={false}
      className="absolute top-full left-0 z-50 mt-1 h-auto w-full rounded-md py-1 shadow-elevated"
    >
      <CommandList
        ref={captureList}
        label={t.list.suggestions.label}
        className="max-h-combobox-list px-1"
      >
        {renderRow(searchRow)}
        {groups.map(
          (group) =>
            group.options.length > 0 && (
              <CommandGroup key={group.heading} heading={group.heading} className="p-0">
                {group.options.map((option) => renderRow({ key: option.id, option }))}
              </CommandGroup>
            ),
        )}
      </CommandList>
    </Command>
  );

  return { inputProps, onKeyDown, open, close, list };
}
