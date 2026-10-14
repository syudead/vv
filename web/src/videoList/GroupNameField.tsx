import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";

import { t } from "../i18n";
import { cn } from "../lib/cn";
import { FieldError } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Popover, PopoverAnchor, PopoverContent } from "../ui/shadcn/popover";
import { nameFieldOutcome, nameTooLong } from "./videoDrop";

/** GroupNameFieldProps は新しいグループの名前の欄の props である。 */
export interface GroupNameFieldProps {
  /** サーバーが付けた仮の名前。欄はこの値を選んだ状態で始まる。 */
  provisional: string;
  /** 変えた名前で Enter を押したときに呼ぶ。 */
  onRename: (name: string) => void;
  /** 欄が閉じたら呼ぶ（Enter、Esc、フォーカスが離れた）。 */
  onClose: () => void;
  className?: string;
}

/**
 * GroupNameField は「Make a group」の後に新しいグループのカードの名前の位置に出す欄である
 * （specs/050-drag-organize/ui-design.md「The new group's name」）。仮の名前を選んだ状態で
 * フォーカスを受け、変えた名前の Enter で `onRename`、変えない Enter・空・Esc・フォーカスが
 * 離れたら何も送らずに閉じる。100 文字を超えると規則を出し、Enter では閉じない。
 */
export function GroupNameField({
  provisional,
  onRename,
  onClose,
  className,
}: GroupNameFieldProps) {
  const [value, setValue] = useState(provisional);
  const input = useRef<HTMLInputElement>(null);
  const closed = useRef(false);
  const hintId = useId();
  const errorId = useId();
  const tooLong = nameTooLong(value);

  useEffect(() => {
    input.current?.focus();
    input.current?.select();
  }, []);

  const close = () => {
    if (closed.current) return;
    closed.current = true;
    onClose();
  };

  const keyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
      return;
    }
    if (event.key !== "Enter") return;
    event.preventDefault();
    const outcome = nameFieldOutcome(value, provisional);
    if (outcome.kind === "invalid") return;
    if (outcome.kind === "rename" && !closed.current) onRename(outcome.name);
    close();
  };

  return (
    <div className={cn("flex min-w-0 flex-col gap-1", className)}>
      <Input
        ref={input}
        value={value}
        onChange={(event) => setValue(event.target.value)}
        onKeyDown={keyDown}
        onBlur={close}
        aria-label={t.organize.name.label}
        aria-describedby={tooLong ? `${hintId} ${errorId}` : hintId}
        aria-invalid={tooLong || undefined}
        className="h-8 text-sm"
      />
      <span id={hintId} className="sr-only">
        {t.organize.name.hint(provisional)}
      </span>
      {tooLong && <FieldError id={errorId}>{t.groups.nameRule}</FieldError>}
    </div>
  );
}

/**
 * GroupNamePopover は名前の欄を、落とした先のカードの下の Popover に出す。フォルダ画面は
 * グループのカードを描かないので、こちらを使う（ui-design.md「The new group's name」の
 * Folder screen）。カードの箱（`relative`）の中に置く。
 */
export function GroupNamePopover(props: GroupNameFieldProps) {
  return (
    <Popover
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <PopoverAnchor asChild>
        <span
          aria-hidden="true"
          className="pointer-events-none absolute inset-x-0 bottom-0"
        />
      </PopoverAnchor>
      <PopoverContent
        side="bottom"
        onOpenAutoFocus={(event) => event.preventDefault()}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <GroupNameField {...props} />
      </PopoverContent>
    </Popover>
  );
}
