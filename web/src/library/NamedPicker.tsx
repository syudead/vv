import { type LucideIcon, Plus } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { maxVideoTagsSelection } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { matchNames, nameTooLong } from "../playlists/pickerRules";
import { Button } from "../ui/shadcn/button";
import { Command, CommandInput, CommandItem, CommandList } from "../ui/shadcn/command";
import { PopoverContent } from "../ui/shadcn/popover";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";
import { overLimitMessage } from "./selectionErrors";

const createValue = "__create__";

export interface NamedTarget {
  id: number;
  name: string;
}

export interface PickerWords {
  label: UiText;
  search: UiText;
  list: UiText;
  create: (name: string) => UiText;
  none: UiText;
  nameTooLong: UiText;
  loading: UiText;
  loadFailed: UiText;
  failed: (reason: string) => UiText;
}

export type PickerChoice<T extends NamedTarget> = { target: T } | { create: string };

type Load<T> =
  { status: "loading" } | { status: "failed" } | { status: "ready"; rows: T[] };

type Pending = { kind: "target"; id: number } | { kind: "create" } | null;

/** Reload all choices on open; failures keep the picker open and refresh stale rows. Recheck selection limits and consume Escape without clearing selection. */
export default function NamedPicker<T extends NamedTarget>({
  open,
  onOpenChange,
  words,
  icon: Icon,
  memberCount,
  loadAll,
  submit,
  excludeIds,
  heading,
  createWhileLoading = false,
  side = "bottom",
  align = "end",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  words: PickerWords;
  icon: LucideIcon;
  memberCount: number;
  loadAll: (signal: AbortSignal) => Promise<T[]>;
  submit: (choice: PickerChoice<T>) => Promise<UiText>;
  excludeIds?: readonly number[];
  /** A line above the choices naming where they are, such as the peer's groups. */
  heading?: UiText;
  createWhileLoading?: boolean;
  side?: "top" | "bottom";
  align?: "start" | "end";
}) {
  const toast = useToast();
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [value, setValue] = useState("");
  const [load, setLoad] = useState<Load<T>>({ status: "loading" });
  const [pending, setPending] = useState<Pending>(null);
  const [failure, setFailure] = useState<UiText | null>(null);
  const loadAbort = useRef<AbortController | null>(null);
  // Opening/closing advances generation so late responses cannot affect a reopened picker.
  const session = useRef(0);
  // Read the latest loader through a ref without refetching on every render.
  const loadRef = useRef(loadAll);
  useEffect(() => {
    loadRef.current = loadAll;
  });

  const read = useCallback((quiet: boolean) => {
    loadAbort.current?.abort();
    const controller = new AbortController();
    loadAbort.current = controller;
    if (!quiet) setLoad({ status: "loading" });
    loadRef.current(controller.signal).then(
      (rows) => {
        if (!controller.signal.aborted) setLoad({ status: "ready", rows });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted || quiet) return;
        setLoad({ status: "failed" });
      },
    );
  }, []);

  useEffect(() => {
    if (!open) return;
    session.current += 1;
    setValue("");
    setFailure(null);
    setPending(null);
    read(false);
    return () => {
      session.current += 1;
      loadAbort.current?.abort();
    };
  }, [open, read]);

  const trimmed = value.trim();
  const tooLong = nameTooLong(trimmed);
  const all = load.status === "ready" ? load.rows : [];
  const excluded = new Set(excludeIds ?? []);
  const matches = matchNames(
    all.filter((row) => !excluded.has(row.id)),
    trimmed,
  );
  const exact = all.some((row) => row.name === trimmed);
  const showCreate =
    (createWhileLoading || load.status !== "loading") && trimmed !== "" && !exact;
  const blocked = pending !== null || tooLong;

  async function send(choice: PickerChoice<T>) {
    const mine = session.current;
    const current = () => session.current === mine;
    setFailure(null);
    // Recheck limits because selection can grow while the picker is open.
    if (memberCount > maxVideoTagsSelection) {
      setFailure(overLimitMessage());
      return;
    }
    setPending(
      "target" in choice ? { kind: "target", id: choice.target.id } : { kind: "create" },
    );
    try {
      // Report submitted results even if the picker has since closed.
      toast(await submit(choice));
      if (current()) onOpenChange(false);
    } catch (error: unknown) {
      if (!current()) return;
      setFailure(words.failed(errorText(error)));
      // Refresh silently to remove vanished targets and include newly created ones.
      read(true);
    } finally {
      if (current()) setPending(null);
    }
  }

  const rowIcon = (busy: boolean, icon: ReactNode) =>
    busy ? <Spinner className="text-muted-foreground" /> : icon;

  let body: ReactNode = null;
  if (load.status === "loading") {
    body = <p className="px-2 py-1.5 text-sm text-muted-foreground">{words.loading}</p>;
  } else if (load.status === "failed") {
    body = (
      <div className="flex items-center justify-between gap-2 px-2 py-1">
        <p className="text-sm text-muted-foreground">{words.loadFailed}</p>
        <Button variant="ghost" size="sm" onClick={() => read(false)}>
          {t.common.retry}
        </Button>
      </div>
    );
  } else if (matches.length === 0 && trimmed === "") {
    body = <p className="px-2 py-1.5 text-sm text-muted-foreground">{words.none}</p>;
  }

  const hasRows = showCreate || (load.status === "ready" && matches.length > 0);

  return (
    <PopoverContent
      side={side}
      align={align}
      aria-label={words.label}
      onOpenAutoFocus={(event) => {
        event.preventDefault();
        inputRef.current?.focus();
      }}
      onEscapeKeyDown={(event) => {
        event.preventDefault();
        onOpenChange(false);
      }}
    >
      <Command label={words.search} shouldFilter={false} className="bg-transparent">
        <CommandInput
          ref={inputRef}
          icon={<Icon aria-hidden="true" />}
          value={value}
          onValueChange={setValue}
          placeholder={words.search}
          aria-label={words.search}
          aria-busy={pending !== null || undefined}
        />
        {}
        {tooLong && (
          <p role="alert" className="py-1 text-xs text-destructive">
            {words.nameTooLong}
          </p>
        )}
        {failure !== null && (
          <p role="alert" className="py-1 text-xs text-destructive">
            {failure}
          </p>
        )}
        {heading !== undefined && (
          <p className="px-2 pt-1.5 text-xs text-muted-foreground">{heading}</p>
        )}
        {hasRows && (
          <CommandList label={words.list}>
            {showCreate && (
              <CommandItem
                value={createValue}
                disabled={blocked}
                onSelect={() => void send({ create: trimmed })}
              >
                {rowIcon(
                  pending?.kind === "create",
                  <Plus className="text-muted-foreground" aria-hidden="true" />,
                )}
                <span className="truncate">{words.create(trimmed)}</span>
              </CommandItem>
            )}
            {load.status === "ready" &&
              matches.map((row) => (
                <CommandItem
                  key={row.id}
                  value={String(row.id)}
                  disabled={blocked}
                  onSelect={() => void send({ target: row })}
                >
                  {rowIcon(
                    pending?.kind === "target" && pending.id === row.id,
                    <Icon className="text-muted-foreground" aria-hidden="true" />,
                  )}
                  <span className="truncate">{row.name}</span>
                </CommandItem>
              ))}
          </CommandList>
        )}
        {body}
      </Command>
    </PopoverContent>
  );
}
