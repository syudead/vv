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

/** NamedTarget はピッカーが並べる行（プレイリストやグループ）である。 */
export interface NamedTarget {
  id: number;
  name: string;
}

/** PickerWords はピッカーの文言である（各機能の）。 */
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

/** PickerChoice は押された行である。既にある行か、打った名前で作る行。 */
export type PickerChoice<T extends NamedTarget> = { target: T } | { create: string };

type Load<T> =
  { status: "loading" } | { status: "failed" } | { status: "ready"; rows: T[] };

/** Pending は返事を待っている行（選んだ行か作成の行）である。 */
type Pending = { kind: "target"; id: number } | { kind: "create" } | null;

/**
 * NamedPicker は「Add to playlist」と「Add to group」のポップオーバーの中身である。呼び出し側がそれぞれの `Popover` の中に置く。
 *
 * - 開くたびに `loadAll` で全部を読み、打った文字を名前に含むものを読んだ順に出す
 *   （`excludeIds` の行は出さない）。打った名前と同じ名前の行が無ければ、先頭に
 *   「Create "{name}"」の行を出す。`createWhileLoading` なら読み込み中と失敗の間も出す。
 * - 行を選ぶと `submit` を呼ぶ。返した文言をトースト 1 つで伝えて閉じる。投げた失敗は開いた
 *   まま、入力の下に理由を出し、途中で消えた行を除くため一覧を静かに読み直す。
 * - 送る数（`memberCount`）が上限を超えていれば送らずに理由を出す（「Favorite」と同じ）。
 * - Esc はポップオーバーだけを閉じる。一覧の Esc（選択の解除）には「使った」と伝える。
 */
export default function NamedPicker<T extends NamedTarget>({
  open,
  onOpenChange,
  words,
  icon: Icon,
  memberCount,
  loadAll,
  submit,
  excludeIds,
  createWhileLoading = false,
  side = "bottom",
  align = "end",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  words: PickerWords;
  /** 入力と行のアイコン。 */
  icon: LucideIcon;
  memberCount: number;
  loadAll: (signal: AbortSignal) => Promise<T[]>;
  /** 送って結果のトーストの文言を返す。失敗は投げる。 */
  submit: (choice: PickerChoice<T>) => Promise<UiText>;
  excludeIds?: readonly number[];
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
  // 開くたび・閉じるたびに進める。返事が来たときに数が変わっていれば、その送信は閉じた
  // ピッカーのもので、開き直したピッカーを閉じたり理由を書き換えたりしてはいけない。
  const session = useRef(0);
  // 読み込みは開くときだけ始めるので、最新の関数を ref で持つ。
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

  // 開くたびに読み直す（別の画面で作ったものもそこにある）。
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
    // 開いている間に選択が増えて上限を超えたら、送らずに理由を出す（「Favorite」と同じ）。
    if (memberCount > maxVideoTagsSelection) {
      setFailure(overLimitMessage());
      return;
    }
    setPending(
      "target" in choice ? { kind: "target", id: choice.target.id } : { kind: "create" },
    );
    try {
      // 結果は、ピッカーを閉じた後でもトーストで伝える。
      toast(await submit(choice));
      if (current()) onOpenChange(false);
    } catch (error: unknown) {
      if (!current()) return;
      setFailure(words.failed(errorText(error)));
      // 途中で消えた行を除き、作った分を足すため、一覧を静かに読み直す。
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
        {/* 理由は入力のすぐ下に出す。 */}
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
