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

import { t, type UiText } from "../i18n";
import { nameReason, newlinePattern } from "../lib/tagName";
import { Command, CommandInput, CommandItem, CommandList } from "../ui/shadcn/command";

/** TagChoice は選択バーのタグの候補の 1 行である。 */
export interface TagChoice {
  id: string;
  label: string;
  /** 行の下に添える 1 行（シノニムで当たったときの「Synonym: …」）。 */
  hint?: ReactNode;
  /** 行の右に添える値（本数）。 */
  meta?: ReactNode;
  /** 読み上げ名。省くと label。 */
  ariaLabel?: string;
}

const createValue = "__create__";

/**
 * TagCommand は選択バーの「タグを付ける」「タグを外す」のポップオーバーの中身で、
 * shadcn/ui の Command（cmdk）にタグの名前の規則を足したものである
 * （web/registry/rules/components.md「Combobox and Command」）。
 *
 * - 候補の絞り込みと並び替えは呼び手が行い（`shouldFilter={false}`）、作成の行は先頭に
 *   置く。矢印で行を選ばずに Enter を押すと、打った綴りで決める: 空なら理由を出し、
 *   完全に一致する名前（`exactChoice`）があればそれを、無ければ作成の行があれば作る。
 * - 名前は打つたびに確かめ、作れない間（と送信中）は Enter もクリックも受けない。
 *   改行を含む貼り付けと落とし込みは止めて理由を出す。IME の変換中のキーは扱わない。
 */
export default function TagCommand({
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
}: {
  /** 入力と一覧の名前（「Add tag」）。 */
  label: UiText;
  value: string;
  onValueChange: (value: string) => void;
  choices: readonly TagChoice[];
  exactChoice: TagChoice | null;
  onSelect: (choice: TagChoice) => void;
  /** 非 null のときだけ、先頭に作成の行を出す（`exactChoice` があれば出さない）。 */
  createLabel?: ReactNode | null;
  onCreate?: (spelling: string) => void;
  icon: ReactNode;
  busy: boolean;
  inputRef?: Ref<HTMLInputElement>;
}) {
  const reasonId = useId();
  const [reason, setReason] = useState<UiText | null>(null);
  // 矢印で行を選んだか。選んでいなければ Enter は打った綴りで決める。
  const moved = useRef(false);
  const [selected, setSelected] = useState("");

  // 打つたびに確かめ直す（貼り付けで出した理由は次の変化で消える）。
  useEffect(() => {
    setReason(nameReason(value));
    moved.current = false;
  }, [value]);

  const blocked = busy || reason !== null;
  const trimmed = value.trim();
  const showCreate = createLabel !== null && exactChoice === null && trimmed !== "";

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
    if (trimmed === "") {
      setReason(t.tagName.required);
      return;
    }
    if (exactChoice !== null) onSelect(exactChoice);
    else if (showCreate) onCreate?.(trimmed);
  }

  function onPaste(event: ClipboardEvent<HTMLInputElement>) {
    if (newlinePattern.test(event.clipboardData.getData("text"))) {
      event.preventDefault();
      setReason(t.tagName.controlCharacters);
    }
  }

  function onBeforeInput(event: FormEvent<HTMLInputElement>) {
    const native = event.nativeEvent as InputEvent;
    if (native.inputType !== "insertFromDrop") return;
    if (newlinePattern.test(native.data ?? "")) {
      event.preventDefault();
      setReason(t.tagName.controlCharacters);
    }
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
      <CommandInput
        ref={inputRef}
        icon={icon}
        value={value}
        onValueChange={onValueChange}
        placeholder={label}
        aria-label={label}
        aria-busy={busy || undefined}
        aria-describedby={reason !== null ? reasonId : undefined}
        onPaste={onPaste}
        onBeforeInput={onBeforeInput}
      />
      {(showCreate || choices.length > 0) && (
        <CommandList label={label}>
          {showCreate && (
            <CommandItem
              value={createValue}
              disabled={blocked}
              onSelect={() => onCreate?.(trimmed)}
            >
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
              <span className="flex min-w-0 flex-col">
                <span className="truncate">{choice.label}</span>
                {choice.hint !== undefined && (
                  <span className="truncate text-xs text-muted-foreground">
                    {choice.hint}
                  </span>
                )}
              </span>
              {choice.meta !== undefined && (
                <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                  {choice.meta}
                </span>
              )}
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
      {reason !== null && (
        <p id={reasonId} className="pt-2 text-xs text-destructive">
          {reason}
        </p>
      )}
    </Command>
  );
}
