import { SendHorizontal } from "lucide-react";
import { type FormEvent, type KeyboardEvent, useEffect, useRef } from "react";
import { useLocation } from "react-router";

import { t } from "../i18n";
import { cn } from "../lib/cn";
import { isComposingNativeKeyEvent } from "../lib/ime";
import { Button } from "../ui/shadcn/button";
import { Textarea } from "../ui/shadcn/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import type { AssistantStore } from "./AssistantProvider";
import { contextLine } from "./screenContext";

/** The server's limit on a request, in characters. */
export const maxRequestLength = 500;

export function requestLength(text: string): number {
  return Array.from(text).length;
}

/** The context line, the field and Send; the field stays editable while a turn runs. */
export default function Composer({
  store,
  checking,
}: {
  store: AssistantStore;
  checking: boolean;
}) {
  const location = useLocation();
  const field = useRef<HTMLTextAreaElement>(null);
  const { takeFocus } = store;
  useEffect(() => {
    if (takeFocus()) field.current?.focus();
  }, [takeFocus]);

  const length = requestLength(store.draft);
  const over = length > maxRequestLength;
  const blocked = store.draft.trim() === "" || over || store.waiting || checking;
  const submit = () => {
    if (blocked) return;
    store.send(store.draft.trim());
    store.setDraft("");
  };
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    submit();
  };
  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== "Enter" || event.shiftKey) return;
    if (isComposingNativeKeyEvent(event.nativeEvent)) return;
    event.preventDefault();
    submit();
  };

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-2">
      <p className="truncate text-xs text-muted-foreground">
        {contextLine(location, store.screen)}
      </p>
      <div className="flex items-end gap-2">
        <Textarea
          ref={field}
          aria-label={t.assistant.field}
          placeholder={t.assistant.placeholder}
          value={store.draft}
          aria-disabled={checking}
          aria-invalid={over}
          readOnly={checking}
          onChange={(event) => store.setDraft(event.target.value)}
          onKeyDown={onKeyDown}
          className="max-h-assistant-composer min-h-8 resize-none py-1.5"
        />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="submit"
              size="icon-sm"
              aria-label={t.assistant.send}
              aria-disabled={blocked}
            >
              <SendHorizontal aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t.assistant.send}</TooltipContent>
        </Tooltip>
      </div>
      <p
        className={cn(
          "text-xs",
          over ? "text-destructive" : "hidden text-muted-foreground sm:block",
        )}
      >
        {over ? t.assistant.tooLong(length) : t.assistant.hint}
      </p>
    </form>
  );
}
