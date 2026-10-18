import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";

import { t } from "../i18n";
import { cn } from "../lib/cn";
import { FieldError } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Popover, PopoverAnchor, PopoverContent } from "../ui/shadcn/popover";
import { nameFieldOutcome, nameTooLong } from "./videoDrop";

export interface GroupNameFieldProps {
  /** Select the server's provisional name on opening. */
  provisional: string;
  onRename: (name: string) => void;
  onClose: () => void;
  className?: string;
}

/**
 * Rename only changed nonempty names on Enter; Escape, blur and unchanged Enter close without
 * saving. Overlength input reports an error and keeps the field open.
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

/** Folders render no group card, so use this popover within the target card's relative box. */
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
