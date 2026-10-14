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

/** NameChoice は名前の候補の 1 行（タグ、人物、団体）である。 */
export interface NameChoice {
  id: string;
  /** 表示する名前（常に元の名前）。 */
  label: string;
  /** 行の下に添える 1 行（シノニム・別名で当たったときの「Synonym: …」「Alias: …」）。 */
  hint?: ReactNode;
  /** 行の右に添える値（本数）。 */
  meta?: ReactNode;
  /** 読み上げ名。省くと行の中の文字から決まる名前。 */
  ariaLabel?: string;
}

/**
 * NameRules は名前の規則である。打つたびに確かめる理由（`reason`）と、空のまま Enter を
 * 押したとき・改行を貼り付けたときの理由を持つ。タグ（lib/tagName）と人物・団体
 * （lib/creditName）がそれぞれ渡す。
 */
export interface NameRules {
  reason: (raw: string) => UiText | null;
  required: UiText;
  controlCharacters: UiText;
}

/**
 * NameCommandLayout は候補の一覧の置き方である
 * （web/registry/rules/components.md「TagCommand」「CreditCommand」）。
 *
 * - `popover`: ポップオーバーの中身（選択バーの「タグを付ける」「タグを外す」）。一覧は
 *   いつも出し、作成の行は先頭。cmdk の既定どおり先頭の行が選ばれている。
 * - `dropdown`: 入力の下に重ねて開く一覧（再生画面の「Add tag」）。入力に触れると開き、
 *   Esc・Tab・フォーカスが外れると閉じる。閉じているときの Esc は `onEscape` を呼ぶ。
 * - `inline`: 入力の下の本文の中に、高さを固定して常に開いておく一覧（統合の窓の統合先）。
 *
 * `dropdown` と `inline` は、矢印を押すまでどの行も選ばない。作成の行は末尾に置く。
 */
export type NameCommandLayout = "popover" | "dropdown" | "inline";

const createValue = "__create__";

/** Row は一覧の 1 行（候補か作成の行）である。 */
interface Row {
  value: string;
  choice: NameChoice | null;
}

/**
 * NameCommand は名前を打って候補から選ぶか作る入力で、shadcn/ui の Command（cmdk）に
 * 名前の規則（`rules`）を足したものである。TagCommand と CreditCommand の基底で、画面は
 * そのどちらかを使う（web/registry/rules/components.md「TagCommand」「CreditCommand」）。
 *
 * - 候補の絞り込みと並び替えは呼び手が行う（`shouldFilter={false}`）。綴りが完全に一致する
 *   名前・シノニムがあれば `exactChoice` に渡す。これが無く `createLabel` があるときだけ
 *   作成の行を出す。
 * - 矢印で行を選ばずに Enter を押すと、打った綴りで決める: 空なら理由を出し、
 *   `exactChoice` があればそれを、無ければ作成の行があれば作る。
 * - 名前は打つたびに確かめ、作れない間（と送信中）は Enter もクリックも受けない。
 *   改行を含む貼り付けと落とし込みは止めて理由を出す。IME の変換中のキーは扱わない。
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
  /** 入力と一覧の名前（「Add tag」）。 */
  label: UiText;
  value: string;
  onValueChange: (value: string) => void;
  choices: readonly NameChoice[];
  exactChoice: NameChoice | null;
  onSelect: (choice: NameChoice) => void;
  /** 非 null のときだけ作成の行を出す（`exactChoice` があれば出さない）。 */
  createLabel?: ReactNode | null;
  onCreate?: (spelling: string) => void;
  icon: ReactNode;
  busy: boolean;
  inputRef?: Ref<HTMLInputElement>;
  layout?: NameCommandLayout;
  /** 入力の placeholder。既定は `label`、null で出さない。 */
  placeholder?: UiText | null;
  /** `dropdown` で一覧が閉じているときの Esc。 */
  onEscape?: () => void;
  /** 理由を出していないときの入力の aria-describedby。 */
  describedBy?: string;
  /** `dropdown`・`inline` の入力の枠の幅。既定は `w-combobox`。 */
  frameClassName?: string;
  /** `inline` で選び終えた候補の id。その行を目立たせる。 */
  chosenId?: string;
  /** `inline` の一覧に行が無いときに一覧の中に出す文言。 */
  emptyText?: ReactNode;
  /**
   * `dropdown`・`inline` で、入力の名前を見える名札（その id）から取る。cmdk の隠れた
   * 名札は出さない（同じ文言の名札が二つにならないように）。
   */
  labelledBy?: string;
  /** 綴りが完全に一致する候補があっても作成の行を出す（同名の人物を別に作れるように）。 */
  offerCreateWithExact?: boolean;
  /**
   * 矢印で行を選ばずに Enter を押したときの決め方を呼び手が持つ（空の名前の理由は
   * ここで出す）。省くと `exactChoice`、無ければ作成の行。
   */
  onCommitSpelling?: (spelling: string) => void;
  /** 候補を読んでいる。輪を回すが、`busy` と違って Enter とクリックは止めない。 */
  searching?: boolean;
  /** 周りの送信中。輪は回さずに、Enter とクリックを止める。 */
  locked?: boolean;
}) {
  const reasonId = useId();
  const [reason, setReason] = useState<UiText | null>(null);

  // 打つたびに確かめ直す（貼り付けで出した理由は次の変化で消える）。
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
  /** 出すときだけ非 null。 */
  createLabel: ReactNode | null;
  onCreate: () => void;
  commitSpelling: () => void;
  icon: ReactNode;
  busy: boolean;
  blocked: boolean;
  inputRef?: Ref<HTMLInputElement>;
  inputProps: InputProps;
  /** 打った値。変わるたびに、矢印で選んだ行を忘れる。 */
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
 * PopoverNameCommand はポップオーバーの中身の形である。行の選択と矢印の移動は cmdk の
 * ままで、作成の行を先頭に置く。矢印を押していなければ Enter は綴りで決める。
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
  // 矢印で行を選んだか。選んでいなければ Enter は打った綴りで決める。
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
    if (moved.current) return; // 選んだ行を cmdk が決める。
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
 * ListNameCommand は入力の下に一覧を置く形（`dropdown`・`inline`）である。矢印を押すまで
 * どの行も選ばないので、行の選択（`aria-selected`・`aria-activedescendant`）・一覧の開閉
 * （`aria-expanded`）と矢印の移動は cmdk に任せずここで持ち、入力と行の属性を `asChild`
 * で上書きする（cmdk は先頭の行を自分で選び、入力を常に開いたものとして、その属性を
 * 後から付けるため）。
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
  // inline の一覧は常に開いている（閉じる操作は無い）。
  const open = inline || openState;
  // 矢印かポインターで選んだ行。-1 はどの行も選んでいない。
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
    // jsdom には scrollIntoView が無い。
    row?.scrollIntoView?.({ block: "nearest" });
  }, [active]);

  function commitRow(row: Row) {
    if (row.choice === null) onCreate();
    else onSelect(row.choice);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    // Home と End は入力の中のカーソルを動かす。cmdk の先頭・末尾の行への移動へは渡さない。
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
    // 下の preventDefault は、cmdk の Command の矢印と Enter の扱いも止める。
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
      // 先頭の行で止まる。まだ何も選んでいなければそのままにする。
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
      // inline の一覧は閉じないので、Esc はいつも「一覧が閉じているとき」の扱い。
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
          // 選んだ統合先の行は、選択中でも primary-soft の面のまま輪だけを足す。
          data-selected={isActive && !chosen}
          data-chosen={chosen || undefined}
          data-index={index}
          // 押してもフォーカスを入力に残す。
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
          // フォーカスは入力そのものが共通の輪（index.css の :focus-visible）を出す。枠に
          // 独自の輪は付けない（web/registry/rules/components.md の Focus）。
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
        {/* cmdk は入力を常に開いた一覧の combobox として描くので、開閉と選んだ行はここで渡す。 */}
        <input
          aria-expanded={open}
          aria-activedescendant={activeId}
          {...(labelledBy !== undefined && { "aria-labelledby": labelledBy })}
          // Esc で一覧を閉じたあとも、打ち直せば一覧を開き直す。
          onChange={() => setOpen(true)}
        />
      </CommandInput>
      {inline ? (
        // 一覧の箱は候補の数によらず同じ高さで、窓のボタンへ重ならない。
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
