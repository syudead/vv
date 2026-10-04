import { LoaderCircle } from "lucide-react";
import {
  type ClipboardEvent,
  type FormEvent,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
  useId,
  useRef,
  useState,
} from "react";

import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Command, CommandInput, CommandItem, CommandList } from "../ui/command";
import {
  type ComboboxOption,
  isComposingKeyEvent,
  nameReason,
  newlinePattern,
} from "../ui/legacy/Combobox";

const createValue = "__create__";

function optionValue(option: ComboboxOption): string {
  return `tag-${option.id}`;
}

/**
 * TagCommand は選択バーの「タグを付ける」「タグを外す」の中身で、ポップオーバーの中に
 * 置く Command（打って絞る候補の一覧）である。デザインシステムの Combobox と同じく
 * Popover の中の Command で、一覧は入力の下に常に見えている。
 *
 * 候補の絞り込みと並びは呼び出し元（tagChoices）が決め、ここでは絞らない。Enter の
 * 既定の行は、綴りが一致する候補（`exactOption`）、無ければ作成の行で、どちらも先頭に
 * 置く。どちらも無いときは、矢印で行を選ぶまで Enter は何もしない（打ちかけの綴りで
 * 別のタグを付け外ししないため）。名前の検証と改行の貼り付け・落とし込みの遮断は
 * タグの名前を打つほかの入力と同じ規則（`nameReason`）である。
 */
export default function TagCommand({
  label,
  icon,
  value,
  onValueChange,
  options,
  exactOption,
  onSelect,
  createLabel = null,
  onCreate,
  busy,
  inputRef,
}: {
  /** 入力の読み上げ名と、空のときに入力に出す文言。 */
  label: UiText;
  icon: ReactNode;
  value: string;
  onValueChange: (value: string) => void;
  options: readonly ComboboxOption[];
  exactOption: ComboboxOption | null;
  onSelect: (option: ComboboxOption) => void;
  /** 非 null のときだけ、綴りが一致する候補が無ければ作成の行を出す。 */
  createLabel?: ReactNode | null;
  onCreate?: (spelling: string) => void;
  busy: boolean;
  inputRef?: Ref<HTMLInputElement>;
}) {
  const reasonId = useId();
  const [reason, setReason] = useState<UiText | null>(null);
  // 打ってから矢印で行を選んだか。選んでいなければ、先頭の行が作成でも一致でも
  // ないとき Enter を止める。
  const navigated = useRef(false);

  const trimmed = value.trim();
  const showCreate = createLabel !== null && exactOption === null && trimmed !== "";
  const rest =
    exactOption === null
      ? options
      : options.filter((option) => option.id !== exactOption.id);
  const blocked = busy || reason !== null;

  function change(next: string) {
    navigated.current = false;
    setReason(nameReason(next));
    onValueChange(next);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (isComposingKeyEvent(event)) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      navigated.current = true;
      return;
    }
    if (event.key !== "Enter") return;
    if (blocked) {
      event.preventDefault();
      return;
    }
    if (trimmed === "") {
      event.preventDefault();
      setReason(t.tagName.required);
      return;
    }
    if (exactOption === null && !showCreate && !navigated.current) {
      event.preventDefault();
    }
  }

  function handlePaste(event: ClipboardEvent<HTMLInputElement>) {
    if (newlinePattern.test(event.clipboardData.getData("text"))) {
      event.preventDefault();
      setReason(t.tagName.controlCharacters);
    }
  }

  function handleBeforeInput(event: FormEvent<HTMLInputElement>) {
    const native = event.nativeEvent as InputEvent;
    if (native.inputType !== "insertFromDrop") return;
    if (newlinePattern.test(native.data ?? "")) {
      event.preventDefault();
      setReason(t.tagName.controlCharacters);
    }
  }

  function row(option: ComboboxOption) {
    return (
      <CommandItem
        key={option.id}
        value={optionValue(option)}
        aria-label={option.ariaLabel}
        disabled={blocked}
        onSelect={() => onSelect(option)}
        className="justify-between"
      >
        <span className="flex min-w-0 flex-col">
          <span className="truncate">{option.label}</span>
          {option.hint !== undefined && (
            <span className="truncate text-xs text-muted-foreground">{option.hint}</span>
          )}
        </span>
        {option.meta !== undefined && (
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {option.meta}
          </span>
        )}
      </CommandItem>
    );
  }

  return (
    <div className="flex flex-col gap-1">
      <Command
        label={label}
        shouldFilter={false}
        className="rounded-md border border-input bg-muted"
      >
        <CommandInput
          ref={inputRef}
          value={value}
          onValueChange={change}
          placeholder={label}
          icon={
            busy ? (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            ) : (
              icon
            )
          }
          aria-busy={busy || undefined}
          aria-describedby={reason !== null ? reasonId : undefined}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          onBeforeInput={handleBeforeInput}
        />
        <CommandList
          label={label}
          className={cn(rest.length + (showCreate ? 1 : 0) === 0 && "hidden")}
        >
          <div className="p-1">
            {exactOption !== null && row(exactOption)}
            {showCreate && (
              <CommandItem
                value={createValue}
                disabled={blocked}
                onSelect={() => onCreate?.(trimmed)}
              >
                {createLabel}
              </CommandItem>
            )}
            {rest.map(row)}
          </div>
        </CommandList>
      </Command>
      {reason !== null && (
        <p id={reasonId} className="text-xs text-destructive">
          {reason}
        </p>
      )}
    </div>
  );
}
