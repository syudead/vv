import { LoaderCircle } from "lucide-react";
import {
  type ClipboardEvent,
  type FormEvent,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";

import type { UiText } from "@/i18n";
import { cn } from "@/lib/cn";
import { isComposingKeyEvent } from "@/lib/ime";
import { newlinePattern } from "@/lib/tagName";
import { Command, CommandInput, CommandItem, CommandList } from "./shadcn/command";

export interface NameChoice {
  id: string;
  /** Always display the original name. */
  label: string;
  hint?: ReactNode;
  meta?: ReactNode;
  /** Defaults to the accessible name derived from row content. */
  ariaLabel?: string;
}

export interface NameRules {
  reason: (raw: string) => UiText | null;
  required: UiText;
  controlCharacters: UiText;
}

/**
 * Popover selects the first row and places creation first; dropdown and inline wait for arrow
 * selection and place creation last. Escape closes dropdown before calling onEscape on a
 * subsequent press.
 */
export type NameCommandLayout = "popover" | "dropdown" | "inline";

const createValue = "__create__";

interface Row {
  value: string;
  choice: NameChoice | null;
}

/**
 * Callers filter and sort candidates; pass exactChoice for exact names or synonyms. Unselected
 * Enter commits spelling, invalid or busy input rejects actions, and IME keys remain untouched;
 * see NameCommandLayout.
 */
export default function NameCommand({
  rules,
  label,
  value,
  onValueChange,
  choices,
  exactChoice,
  onSelect,
  createLabel = null,
  onCreate,
  icon,
  busy,
  inputRef,
  layout = "popover",
  placeholder = label,
  onEscape,
  describedBy,
  frameClassName,
  chosenId,
  emptyText,
  labelledBy,
  offerCreateWithExact = false,
  onCommitSpelling,
  searching = false,
  locked = false,
}: {
  rules: NameRules;
  label: UiText;
  value: string;
  onValueChange: (value: string) => void;
  choices: readonly NameChoice[];
  exactChoice: NameChoice | null;
  onSelect: (choice: NameChoice) => void;
  /** Creation requires a non-null label and no exactChoice. */
  createLabel?: ReactNode | null;
  onCreate?: (spelling: string) => void;
  icon: ReactNode;
  busy: boolean;
  inputRef?: Ref<HTMLInputElement>;
  layout?: NameCommandLayout;
  /** Defaults to label; null omits the placeholder. */
  placeholder?: UiText | null;
  /** Called on Escape only when the dropdown is already closed. */
  onEscape?: () => void;
  /** Used only while no validation error is shown. */
  describedBy?: string;
  frameClassName?: string;
  chosenId?: string;
  emptyText?: ReactNode;
  /** Use this visible label ID instead of cmdk's hidden duplicate label. */
  labelledBy?: string;
  /** Allow same-name creation for distinct people. */
  offerCreateWithExact?: boolean;
  /** Override unselected Enter handling; empty-input errors remain internal. */
  onCommitSpelling?: (spelling: string) => void;
  /** Loading shows a spinner but does not block actions, unlike busy. */
  searching?: boolean;
  /** Block actions without showing a spinner. */
  locked?: boolean;
}) {
  const reasonId = useId();
  const [reason, setReason] = useState<UiText | null>(null);

  useEffect(() => {
    setReason(rules.reason(value));
  }, [rules, value]);

  const blocked = busy || locked || reason !== null;
  const trimmed = value.trim();
  const showCreate =
    createLabel !== null &&
    (offerCreateWithExact || exactChoice === null) &&
    trimmed !== "";

  function commitSpelling(): void {
    if (trimmed === "") {
      setReason(rules.required);
      return;
    }
    if (onCommitSpelling !== undefined) onCommitSpelling(trimmed);
    else if (exactChoice !== null) onSelect(exactChoice);
    else if (showCreate) onCreate?.(trimmed);
  }

  function onPaste(event: ClipboardEvent<HTMLInputElement>) {
    if (newlinePattern.test(event.clipboardData.getData("text"))) {
      event.preventDefault();
      setReason(rules.controlCharacters);
    }
  }

  function onBeforeInput(event: FormEvent<HTMLInputElement>) {
    const native = event.nativeEvent as InputEvent;
    if (native.inputType !== "insertFromDrop") return;
    if (newlinePattern.test(native.data ?? "")) {
      event.preventDefault();
      setReason(rules.controlCharacters);
    }
  }

  const reasonText = reason !== null && (
    <p
      id={reasonId}
      className={
        layout === "popover"
          ? "pt-2 text-xs text-destructive"
          : "mt-1 text-xs text-destructive"
      }
    >
      {reason}
    </p>
  );
  const inputProps = {
    value,
    onValueChange,
    placeholder: placeholder ?? undefined,
    "aria-label": label,
    "aria-busy": busy || searching || undefined,
    "aria-describedby": reason !== null ? reasonId : describedBy,
    onPaste,
    onBeforeInput,
  };

  if (layout === "popover") {
    return (
      <PopoverNameCommand
        label={label}
        choices={choices}
        onSelect={onSelect}
        createLabel={showCreate ? createLabel : null}
        onCreate={() => onCreate?.(trimmed)}
        commitSpelling={commitSpelling}
        icon={icon}
        busy={busy || searching}
        blocked={blocked}
        inputRef={inputRef}
        inputProps={inputProps}
        valueKey={value}
        reasonText={reasonText}
      />
    );
  }
  return (
    <ListNameCommand
      label={label}
      inline={layout === "inline"}
      choices={choices}
      onSelect={onSelect}
      createLabel={showCreate ? createLabel : null}
      onCreate={() => onCreate?.(trimmed)}
      commitSpelling={commitSpelling}
      icon={icon}
      busy={busy || searching}
      blocked={blocked}
      inputRef={inputRef}
      inputProps={inputProps}
      valueKey={value}
      reasonText={reasonText}
      onEscape={onEscape}
      frameClassName={frameClassName}
      chosenId={chosenId}
      emptyText={emptyText}
      labelledBy={labelledBy}
    />
  );
}

interface InputProps {
  value: string;
  onValueChange: (value: string) => void;
  placeholder: UiText | undefined;
  "aria-label": UiText;
  "aria-busy": true | undefined;
  "aria-describedby": string | undefined;
  onPaste: (event: ClipboardEvent<HTMLInputElement>) => void;
  onBeforeInput: (event: FormEvent<HTMLInputElement>) => void;
}

interface SharedProps {
  label: UiText;
  choices: readonly NameChoice[];
  onSelect: (choice: NameChoice) => void;
  createLabel: ReactNode | null;
  onCreate: () => void;
  commitSpelling: () => void;
  icon: ReactNode;
  busy: boolean;
  blocked: boolean;
  inputRef?: Ref<HTMLInputElement>;
  inputProps: InputProps;
  /** Input changes clear arrow-selected rows. */
  valueKey: string;
  reasonText: ReactNode;
}

function ChoiceContent({
  choice,
  chosen = false,
}: {
  choice: NameChoice;
  chosen?: boolean;
}) {
  return (
    <>
      <span className="flex min-w-0 flex-col">
        <span className={cn("truncate", chosen && "font-medium")}>{choice.label}</span>
        {choice.hint !== undefined && (
          <span className="truncate text-xs text-muted-foreground">{choice.hint}</span>
        )}
      </span>
      {choice.meta !== undefined && (
        <span
          className={cn(
            "shrink-0 text-xs tabular-nums",
            chosen ? "text-primary" : "text-muted-foreground",
          )}
        >
          {choice.meta}
        </span>
      )}
    </>
  );
}

/**
 * Keep cmdk selection/navigation, but unselected Enter commits spelling rather than the default
 * first row.
 */
function PopoverNameCommand({
  label,
  choices,
  onSelect,
  createLabel,
  onCreate,
  commitSpelling,
  icon,
  busy,
  blocked,
  inputRef,
  inputProps,
  valueKey,
  reasonText,
}: SharedProps) {
  const moved = useRef(false);
  const [selected, setSelected] = useState("");

  useEffect(() => {
    moved.current = false;
  }, [valueKey]);

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.nativeEvent.isComposing || event.keyCode === 229) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      moved.current = true;
      return;
    }
    if (event.key !== "Enter") return;
    if (blocked) {
      event.preventDefault();
      return;
    }
    if (moved.current) return;
    event.preventDefault();
    commitSpelling();
  }

  return (
    <Command
      label={label}
      shouldFilter={false}
      value={selected}
      onValueChange={setSelected}
      onKeyDown={onKeyDown}
      className="bg-transparent"
    >
      <CommandInput ref={inputRef} icon={icon} {...inputProps} />
      {(createLabel !== null || choices.length > 0) && (
        <CommandList label={label}>
          {createLabel !== null && (
            <CommandItem value={createValue} disabled={blocked} onSelect={onCreate}>
              {createLabel}
            </CommandItem>
          )}
          {choices.map((choice) => (
            <CommandItem
              key={choice.id}
              value={choice.id}
              disabled={blocked}
              aria-label={choice.ariaLabel}
              onSelect={() => onSelect(choice)}
              className="justify-between"
            >
              <ChoiceContent choice={choice} />
            </CommandItem>
          ))}
        </CommandList>
      )}
      {busy && (
        <LoaderCircle
          aria-hidden="true"
          className="size-4 animate-spin self-center text-muted-foreground motion-reduce:animate-none"
        />
      )}
      {reasonText}
    </Command>
  );
}

/**
 * Override cmdk through asChild to control selection and expanded state; cmdk otherwise selects
 * the first row and forces the input open.
 */
function ListNameCommand({
  label,
  inline,
  choices,
  onSelect,
  createLabel,
  onCreate,
  commitSpelling,
  icon,
  busy,
  blocked,
  inputRef,
  inputProps,
  valueKey,
  reasonText,
  onEscape,
  frameClassName,
  chosenId,
  emptyText,
  labelledBy,
}: SharedProps & {
  inline: boolean;
  onEscape?: () => void;
  frameClassName?: string;
  chosenId?: string;
  emptyText?: ReactNode;
  labelledBy?: string;
}) {
  const baseId = useId();
  const [openState, setOpen] = useState(false);
  const open = inline || openState;
  // Minus one means no row selected.
  const [activeIndex, setActiveIndex] = useState(-1);
  const listRef = useRef<HTMLDivElement | null>(null);

  const rows: Row[] = choices.map((choice) => ({ value: choice.id, choice }));
  if (createLabel !== null) rows.push({ value: createValue, choice: null });
  const rowCount = rows.length;

  useEffect(() => {
    setActiveIndex(-1);
  }, [valueKey]);

  const active = open && activeIndex >= 0 && activeIndex < rowCount ? activeIndex : -1;
  const rowId = (index: number) => `${baseId}-option-${String(index)}`;
  const activeId = active >= 0 ? rowId(active) : undefined;

  useEffect(() => {
    if (active < 0) return;
    const row = listRef.current?.querySelector(`[data-index="${String(active)}"]`);
    // jsdom lacks scrollIntoView.
    row?.scrollIntoView?.({ block: "nearest" });
  }, [active]);

  function commitRow(row: Row) {
    if (row.choice === null) onCreate();
    else onSelect(row.choice);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    // Home and End edit the input caret; do not pass them to cmdk row navigation.
    if (event.key === "Home" || event.key === "End") {
      event.stopPropagation();
      return;
    }
    if (
      isComposingKeyEvent(event) &&
      (event.key === "ArrowDown" ||
        event.key === "ArrowUp" ||
        event.key === "Enter" ||
        event.key === "Escape")
    ) {
      return;
    }
    // preventDefault also blocks cmdk's arrow and Enter handling.
    if (event.key === "ArrowDown") {
      event.preventDefault();
      if (!open) {
        setOpen(true);
        setActiveIndex(rowCount > 0 ? 0 : -1);
        return;
      }
      setActiveIndex(Math.min(active + 1, rowCount - 1));
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) {
        setOpen(true);
        return;
      }
      setActiveIndex(active <= 0 ? active : active - 1);
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      if (blocked) return;
      if (active >= 0) {
        commitRow(rows[active]!);
        return;
      }
      commitSpelling();
      return;
    }
    if (event.key === "Tab") {
      setOpen(false);
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      // Inline never closes, so Escape always follows the closed-list behavior.
      if (open && !inline) {
        setOpen(false);
        return;
      }
      onEscape?.();
    }
  }

  const items = rows.map((row, index) => {
    const chosen = row.choice !== null && row.choice.id === chosenId;
    const isActive = index === active;
    return (
      <CommandItem
        key={row.value}
        value={row.value}
        disabled={blocked}
        aria-label={row.choice?.ariaLabel}
        onSelect={() => commitRow(row)}
        asChild
        className={cn(
          "min-h-8 rounded-none py-1",
          row.choice !== null && "justify-between",
          inline && "min-h-9 rounded-md not-last:mb-0.5",
          chosen && "bg-primary-soft text-primary",
          chosen && isActive && "ring-1 ring-ring ring-inset",
        )}
      >
        <div
          id={rowId(index)}
          aria-selected={isActive}
          data-selected={isActive && !chosen}
          data-chosen={chosen || undefined}
          data-index={index}
          // Keep input focus on pointer selection.
          onMouseDown={(event) => event.preventDefault()}
          onMouseEnter={() => setActiveIndex(index)}
        >
          {row.choice === null ? (
            createLabel
          ) : (
            <ChoiceContent choice={row.choice} chosen={chosen} />
          )}
        </div>
      </CommandItem>
    );
  });

  const list = (
    <CommandList
      ref={listRef}
      label={label}
      className={
        inline
          ? "max-h-none overflow-visible"
          : "absolute top-full z-50 mt-1 max-h-combobox-list w-combobox-list rounded-md bg-popover py-1 shadow-elevated"
      }
    >
      {items}
    </CommandList>
  );

  return (
    <Command
      label={labelledBy === undefined ? label : undefined}
      shouldFilter={false}
      disablePointerSelection
      vimBindings={false}
      className={cn(
        "relative h-auto overflow-visible rounded-none bg-transparent",
        inline ? "w-full" : "w-auto",
      )}
    >
      <CommandInput
        ref={inputRef}
        icon={icon}
        {...inputProps}
        onKeyDown={onKeyDown}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        asChild
        wrapperClassName={cn(
          inline
            ? "rounded-md border bg-muted text-sm"
            : "h-6 gap-1 rounded-sm border bg-muted px-1.5 text-xs [&>svg]:size-3",
          // Use the input's shared focus ring, not a second custom wrapper ring.
          "border-input",
          frameClassName ?? "w-combobox",
        )}
        className={cn("h-auto rounded-none px-0", !inline && "text-xs")}
        trailing={
          busy && (
            <LoaderCircle
              className="size-3 shrink-0 animate-spin text-muted-foreground"
              aria-hidden="true"
            />
          )
        }
      >
        {/* Override cmdk's always-expanded combobox state with actual visibility and selection. */}
        <input
          aria-expanded={open}
          aria-activedescendant={activeId}
          {...(labelledBy !== undefined && { "aria-labelledby": labelledBy })}
          onChange={() => setOpen(true)}
        />
      </CommandInput>
      {inline ? (
        // Keep list height stable so candidates do not overlap dialog buttons.
        <div className="mt-2 h-combobox-panel max-h-combobox-panel-max overflow-y-auto rounded-md border border-border p-1">
          {list}
          {rowCount === 0 && emptyText !== undefined && (
            <p className="px-2 py-2 text-sm text-muted-foreground">{emptyText}</p>
          )}
        </div>
      ) : (
        open && rowCount > 0 && list
      )}
      {reasonText}
    </Command>
  );
}
