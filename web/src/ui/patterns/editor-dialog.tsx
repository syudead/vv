import { XIcon } from "lucide-react";
import { type ReactNode, type RefObject, useRef } from "react";

import { Button } from "@/ui/shadcn/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/ui/shadcn/dialog";

// Full-screen image editor; see web/registry/rules/patterns.md, Editor dialog.
// Escape and close request onClose; callers decide whether confirmation is needed.

export interface EditorDialogProps {
  open: boolean;
  /** The caller sets open=false only after accepting the close request. */
  onClose: () => void;
  title: ReactNode;
  detail: ReactNode;
  closeLabel: string;
  closeDisabled?: boolean;
  stage: ReactNode;
  aside: ReactNode;
  footer: ReactNode;
  /** Defaults to the first focusable element. */
  initialFocus?: RefObject<HTMLElement | null>;
}

export function EditorDialog({
  open,
  onClose,
  title,
  detail,
  closeLabel,
  closeDisabled = false,
  stage,
  aside,
  footer,
  initialFocus,
}: EditorDialogProps) {
  // Restore the previously focused trigger on close.
  const previousFocus = useRef<HTMLElement | null>(null);
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent
        data-slot="editor-dialog"
        showCloseButton={false}
        className="inset-0 m-0 flex max-h-none max-w-none translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 text-foreground shadow-none"
        onEscapeKeyDown={(event) => {
          // IME cancellation Escape must not close the dialog.
          if (event.isComposing || event.keyCode === 229) event.preventDefault();
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (previousFocus.current?.isConnected === true) previousFocus.current.focus();
        }}
        onOpenAutoFocus={(event) => {
          previousFocus.current =
            document.activeElement instanceof HTMLElement ? document.activeElement : null;
          const target = initialFocus?.current;
          if (target === null || target === undefined) return;
          event.preventDefault();
          target.focus();
        }}
      >
        <div
          data-slot="editor-dialog-header"
          className="flex h-navbar shrink-0 items-center gap-3 border-b border-border px-4"
        >
          <div className="flex min-w-0 flex-1 items-baseline gap-3">
            <DialogTitle className="shrink-0 text-lg leading-none font-semibold">
              {title}
            </DialogTitle>
            <DialogDescription className="truncate text-sm text-muted-foreground tabular-nums">
              {detail}
            </DialogDescription>
          </div>
          <DialogClose asChild disabled={closeDisabled}>
            <Button variant="ghost" size="icon-sm" aria-label={closeLabel}>
              <XIcon aria-hidden="true" />
            </Button>
          </DialogClose>
        </div>
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain lg:flex-row lg:overflow-hidden">
          <div
            data-slot="editor-dialog-stage"
            className="flex h-editor-stage shrink-0 items-center justify-center overflow-hidden bg-muted p-4 lg:h-auto lg:min-w-0 lg:flex-1 lg:shrink"
          >
            {stage}
          </div>
          <div
            data-slot="editor-dialog-aside"
            className="flex flex-1 flex-col border-border lg:min-h-0 lg:w-editor-aside lg:flex-none lg:border-l xl:w-editor-aside-wide"
          >
            <div className="flex flex-1 flex-col gap-4 p-4 lg:min-h-0 lg:overflow-y-auto">
              {aside}
            </div>
            <div
              data-slot="editor-dialog-footer"
              className="sticky bottom-0 flex flex-col gap-2 border-t border-border bg-background p-4"
            >
              {footer}
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
