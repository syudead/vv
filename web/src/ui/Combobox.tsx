import { LoaderCircle } from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ClipboardEvent,
  type FormEvent,
  type InputHTMLAttributes,
  type KeyboardEvent,
  type MutableRefObject,
  type ReactNode,
  type Ref,
} from "react";

import { cn } from "../lib/cn";

/** 候補の1行。id は React のキーと aria-activedescendant に使う文字列である。 */
export interface ComboboxOption {
  id: string;
  /** 表示する名前（常に元の名前。ui-design.md「Combobox」）。 */
  label: string;
  /** 名前の下に添える従の1行（例: 「シノニム: アニメ」）。 */
  hint?: string;
  /** 行の右に置く従の情報（本数など）。 */
  meta?: ReactNode;
  /**
   * 行の読み上げ名。指定しなければ行の中の文字（label・hint・meta）から
   * 決まる既定の名前を使う。選択バーの「タグを外す」の一部の候補のように、
   * 見た目の文言と読み上げ名を変えたいときに使う
   * （ui-design.md「Accessibility」の読み上げ名）。
   */
  ariaLabel?: string;
}

export const newlinePattern = /[\r\n]/;
// eslint-disable-next-line no-control-regex -- タグ名では C0/C1 制御文字をすべて拒む。
const controlCharPattern = /[\u0000-\u001f\u007f-\u009f]/;

/** codePointLength は前後の空白を除いた符号位置の数を返す（`length` は使わない）。 */
function codePointLength(value: string): number {
  return Array.from(value.trim()).length;
}

/**
 * isComposingKey は、IME の変換中に打った特別なキー（Enter・Esc・矢印）かを、
 * `isComposing`・`keyCode` の組から判定する（keyCode 229 は isComposing を
 * 実装しない古いブラウザ向けの後方互換）。React の合成イベントとブラウザの
 * 生のイベントの両方から使えるよう、値だけを受け取る形にしている。
 */
function isComposingKey(isComposing: boolean, keyCode: number): boolean {
  return isComposing || keyCode === 229;
}

/**
 * isComposingKeyEvent は、React の合成イベント（`onKeyDown` など）が、IME の
 * 変換中に打った特別なキーかを返す。変換の確定・移動のためのキーで、
 * combobox やタグの名前を打つほかの入力（管理画面の作成・改名の入力、検索の
 * 入力）の操作にしてはいけない（`web/src/player/keyboard.ts` と同じ判定）。
 */
export function isComposingKeyEvent(event: KeyboardEvent<HTMLInputElement>): boolean {
  return isComposingKey(event.nativeEvent.isComposing, event.nativeEvent.keyCode);
}

/**
 * isComposingNativeKeyEvent は、`document.addEventListener` などで受け取る
 * 生の `KeyboardEvent` に対する同じ判定である。`ui/ModalFrame` の Esc の
 * 扱いが使う（IME の変換中の Esc で窓ごと閉じてしまわないため）。
 */
export function isComposingNativeKeyEvent(event: globalThis.KeyboardEvent): boolean {
  return isComposingKey(event.isComposing, event.keyCode);
}

/**
 * nameReason は、入力のたびに確かめる名前の検証理由を返す。空や空白だけは
 * 打っている間は理由を出さない（ui-design.md「Combobox」名前の検証）。タグの
 * 名前を打つすべての入力（この Combobox、管理画面の作成・改名・シノニムの
 * 追加）が同じ規則を使うので外へ公開する（ui-design.md「Combobox」末尾）。
 */
export function nameReason(raw: string): string | null {
  if (controlCharPattern.test(raw)) return "改行やタブは使えません";
  const length = codePointLength(raw);
  if (length > 100) return `100 文字以内にしてください（今 ${String(length)} 文字）`;
  return null;
}

/**
 * Combobox はタグの名前を打つ ARIA 1.2 の combobox（入力 + listbox、
 * `aria-activedescendant`）である（Plan の Structural Decisions 9）。依存は
 * 足さず、キーボード操作・名前の検証・改行の貼り付け／落とし込みの遮断を
 * 手で作る（ui-design.md「Combobox」）。
 *
 * 候補の絞り込み・並び替え・すでに付いているタグの除外は呼び出し元が行い、
 * `options` に絞り込み済みの候補を渡す。綴りが完全に一致する名前・シノニムが
 * あれば `exactOption` に渡す。これが無く `createLabel` があるときだけ、
 * 一覧の末尾に作成の行を出す。
 */
export default function Combobox({
  value,
  onValueChange,
  options,
  exactOption = null,
  onSelect,
  createLabel = null,
  onCreate,
  placeholder,
  icon,
  busy = false,
  disabled = false,
  side = "bottom",
  onEscapeWhenClosed,
  onOpenChange,
  describedBy,
  inputRef: externalInputRef,
  "aria-label": ariaLabel,
  className,
  inputClassName,
  frameClassName,
  ...rest
}: {
  value: string;
  onValueChange: (value: string) => void;
  options: ComboboxOption[];
  exactOption?: ComboboxOption | null;
  onSelect: (option: ComboboxOption) => void;
  /** 非 null のときだけ、末尾に作成の行を出す（`exactOption` があれば出さない）。 */
  createLabel?: ReactNode | null;
  onCreate?: (spelling: string) => void;
  placeholder?: string;
  icon?: ReactNode;
  busy?: boolean;
  disabled?: boolean;
  /** 候補の一覧を入力の上に開く（選択バー）か下に開く（既定）か。 */
  side?: "top" | "bottom";
  /** 一覧が閉じているときの Esc。外側へは伝えない（呼び出し元がここで受ける）。 */
  onEscapeWhenClosed?: () => void;
  /**
   * 候補の一覧の開閉が変わるたびに呼ぶ。ポップオーバーの中で使うときに、
   * その一覧が開いているかを外側（PopoverContent の `onEscapeKeyDown`）から
   * 判定できるようにする（B2: Radix の DismissableLayer は document の
   * capture 段階で Esc を先に拾うため、combobox 自身の onKeyDown より先に
   * ポップオーバー全体を閉じてしまう。開いている間は外側で preventDefault
   * させ、一覧だけを閉じさせる）。
   */
  onOpenChange?: (open: boolean) => void;
  describedBy?: string;
  inputRef?: Ref<HTMLInputElement>;
  "aria-label"?: string;
  className?: string;
  inputClassName?: string;
  /** 入力を囲む枠の幅などを差し替える。既定は再生画面と同じ `w-40`。 */
  frameClassName?: string;
} & Omit<
  InputHTMLAttributes<HTMLInputElement>,
  | "value"
  | "onChange"
  | "placeholder"
  | "disabled"
  | "className"
  | "aria-label"
  | "aria-describedby"
  | "onSelect"
>) {
  const baseId = useId();
  const listboxId = `${baseId}-listbox`;
  const reasonId = `${baseId}-reason`;

  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const [reason, setReason] = useState<string | null>(null);

  const listRef = useRef<HTMLUListElement | null>(null);

  // open の変化を外側へ伝える（呼び出し元は effect の commit 後、次の
  // キー操作より前に確実に最新の値を読める。B2）。
  useEffect(() => {
    onOpenChange?.(open);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const showCreateRow = createLabel !== null && exactOption === null;
  const rowCount = options.length + (showCreateRow ? 1 : 0);

  // value が変わるたびに検証をやり直す（貼り付け・落とし込みで直接 setReason した
  // 理由は、そのあとの最初の変化で上書きされ、消える）。
  useEffect(() => {
    setReason(nameReason(value));
    setActiveIndex(-1);
  }, [value]);

  useEffect(() => {
    if (activeIndex < 0 || !open) return;
    const row = listRef.current?.querySelector(`[data-index="${String(activeIndex)}"]`);
    // jsdom には scrollIntoView が無い。
    row?.scrollIntoView?.({ block: "nearest" });
  }, [activeIndex, open]);

  const commit = useCallback((option: ComboboxOption) => onSelect(option), [onSelect]);

  function tryCommitSpelling() {
    const trimmed = value.trim();
    if (trimmed === "") {
      setReason("名前を入力してください");
      return;
    }
    if (exactOption !== null) {
      commit(exactOption);
      return;
    }
    if (createLabel !== null) onCreate?.(trimmed);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (
      isComposingKeyEvent(event) &&
      (event.key === "ArrowDown" ||
        event.key === "ArrowUp" ||
        event.key === "Enter" ||
        event.key === "Escape")
    ) {
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      if (!open) {
        setOpen(true);
        setActiveIndex(rowCount > 0 ? 0 : -1);
        return;
      }
      setActiveIndex((current) => Math.min(current + 1, rowCount - 1));
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) {
        setOpen(true);
        return;
      }
      // 上端（先頭の行）で止まる。まだ何も選んでいなければそのままにする
      // （ui-design.md「Combobox」端で止まる）。
      setActiveIndex((current) => (current <= 0 ? current : current - 1));
      return;
    }
    if (event.key === "Enter") {
      if (busy || reason !== null) {
        event.preventDefault();
        return;
      }
      event.preventDefault();
      if (open && activeIndex >= 0) {
        if (activeIndex < options.length) commit(options[activeIndex]!);
        else onCreate?.(value.trim());
        return;
      }
      tryCommitSpelling();
      return;
    }
    if (event.key === "Tab") {
      setOpen(false);
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      if (open) {
        setOpen(false);
        return;
      }
      onEscapeWhenClosed?.();
    }
  }

  function handlePaste(event: ClipboardEvent<HTMLInputElement>) {
    const text = event.clipboardData.getData("text");
    if (newlinePattern.test(text)) {
      event.preventDefault();
      setReason("改行やタブは使えません");
    }
  }

  function handleBeforeInput(event: FormEvent<HTMLInputElement>) {
    const native = event.nativeEvent as InputEvent;
    if (native.inputType !== "insertFromDrop") return;
    const text = native.data ?? "";
    if (newlinePattern.test(text)) {
      event.preventDefault();
      setReason("改行やタブは使えません");
    }
  }

  const activeId =
    open && activeIndex >= 0 ? `${listboxId}-option-${String(activeIndex)}` : undefined;

  // 送信中・名前が作れない間は、クリックでも Enter と同じく確定を受けない
  // （Enter だけを塞いでもクリックはすり抜けてしまう）。
  const blocked = busy || reason !== null;

  const rows = useMemo(() => {
    const items: { key: string; index: number; node: ReactNode }[] = options.map(
      (option, index) => ({
        key: option.id,
        index,
        node: (
          <li
            key={option.id}
            id={`${listboxId}-option-${String(index)}`}
            role="option"
            aria-selected={index === activeIndex}
            aria-disabled={blocked || undefined}
            aria-label={option.ariaLabel}
            data-index={index}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => {
              if (blocked) return;
              commit(option);
            }}
            onMouseEnter={() => setActiveIndex(index)}
            className={cn(
              "flex min-h-8 cursor-default items-center justify-between gap-2 px-2.5 py-1 text-sm text-fg select-none",
              index === activeIndex && "bg-hover-wash",
              blocked && "pointer-events-none opacity-50",
            )}
          >
            <span className="flex min-w-0 flex-col">
              <span className="truncate">{option.label}</span>
              {option.hint !== undefined && (
                <span className="truncate text-xs text-fg-muted">{option.hint}</span>
              )}
            </span>
            {option.meta !== undefined && (
              <span className="shrink-0 text-xs text-fg-muted tabular-nums">
                {option.meta}
              </span>
            )}
          </li>
        ),
      }),
    );
    if (showCreateRow) {
      const index = options.length;
      items.push({
        key: "__create__",
        index,
        node: (
          <li
            key="__create__"
            id={`${listboxId}-option-${String(index)}`}
            role="option"
            aria-selected={index === activeIndex}
            aria-disabled={blocked || undefined}
            data-index={index}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => {
              if (blocked) return;
              onCreate?.(value.trim());
            }}
            onMouseEnter={() => setActiveIndex(index)}
            className={cn(
              "flex h-8 cursor-default items-center gap-2 px-2.5 text-sm text-fg select-none",
              index === activeIndex && "bg-hover-wash",
              blocked && "pointer-events-none opacity-50",
            )}
          >
            {createLabel}
          </li>
        ),
      });
    }
    return items;
  }, [
    options,
    showCreateRow,
    createLabel,
    activeIndex,
    listboxId,
    value,
    blocked,
    commit,
    onCreate,
  ]);

  return (
    <div className={cn("relative", className)}>
      <div
        className={cn(
          "flex h-6 items-center gap-1 rounded-sm border bg-field px-1.5 text-xs",
          "focus-within:border-accent",
          disabled ? "border-border opacity-50" : "border-border",
          frameClassName ?? "w-40",
        )}
      >
        {icon}
        <input
          {...rest}
          ref={(node) => {
            if (typeof externalInputRef === "function") externalInputRef(node);
            else if (externalInputRef)
              (externalInputRef as MutableRefObject<HTMLInputElement | null>).current =
                node;
          }}
          type="text"
          role="combobox"
          aria-expanded={open}
          aria-controls={listboxId}
          aria-autocomplete="list"
          aria-activedescendant={activeId}
          aria-busy={busy || undefined}
          aria-describedby={reason !== null ? reasonId : describedBy}
          aria-label={ariaLabel}
          value={value}
          placeholder={placeholder}
          disabled={disabled}
          onChange={(event) => {
            onValueChange(event.target.value);
            // Esc で一覧を閉じたあとも、打ち直せば一覧を開き直す。
            setOpen(true);
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => setOpen(false)}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          onBeforeInput={handleBeforeInput}
          className={cn(
            "w-full min-w-0 bg-transparent text-fg outline-none placeholder:text-fg-muted",
            inputClassName,
          )}
        />
        {busy && (
          <LoaderCircle
            className="size-3 shrink-0 animate-spin text-fg-muted"
            aria-hidden="true"
          />
        )}
      </div>
      {open && rows.length > 0 && (
        <ul
          ref={listRef}
          id={listboxId}
          role="listbox"
          className={cn(
            "absolute z-50 max-h-64 w-64 overflow-y-auto rounded-md bg-elevated py-1 shadow-elevated",
            side === "top" ? "bottom-full mb-1" : "top-full mt-1",
          )}
        >
          {rows.map((row) => row.node)}
        </ul>
      )}
      {reason !== null && (
        <p id={reasonId} className="mt-1 text-xs text-danger">
          {reason}
        </p>
      )}
    </div>
  );
}
