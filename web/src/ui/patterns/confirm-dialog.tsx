import { type ReactNode, type Ref, useRef } from "react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/ui/shadcn/alert-dialog";

// Destructive confirmation uses AlertDialog; see web/registry/rules/patterns.md, Confirm dialog.
// Confirm dialog。
//
// Pass pending for requests so callers close only on success; disable both actions while submitting and retain inline failures.

export interface ConfirmDialogProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  trigger?: ReactNode;
  title: ReactNode;
  description: ReactNode;
  /** Use an action verb, never OK. */
  actionLabel: ReactNode;
  cancelLabel: ReactNode;
  onConfirm: () => void;
  /** Supplying pending makes closing caller-controlled; otherwise the action closes immediately. */
  pending?: boolean;
  actionDisabled?: boolean;
  children?: ReactNode;
  actionRef?: Ref<HTMLButtonElement>;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  trigger,
  title,
  description,
  actionLabel,
  cancelLabel,
  onConfirm,
  pending,
  actionDisabled = false,
  children,
  actionRef,
}: ConfirmDialogProps) {
  const awaitsResult = pending !== undefined;
  // Restore pre-open focus for controlled dialogs without a trigger; Radix restores only AlertDialogTrigger.
  const previousFocus = useRef<HTMLElement | null>(null);
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      {trigger && <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>}
      <AlertDialogContent
        data-slot="confirm-dialog"
        onOpenAutoFocus={() => {
          previousFocus.current =
            document.activeElement instanceof HTMLElement ? document.activeElement : null;
        }}
        onCloseAutoFocus={(event) => {
          if (trigger) return;
          event.preventDefault();
          if (previousFocus.current?.isConnected === true) previousFocus.current.focus();
        }}
        onEscapeKeyDown={(event) => {
          // IME cancellation Escape must not close the dialog.
          if (event.isComposing || event.keyCode === 229) event.preventDefault();
        }}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {children}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending === true}>{cancelLabel}</AlertDialogCancel>
          <AlertDialogAction
            ref={actionRef}
            variant="destructive"
            disabled={pending === true || actionDisabled}
            aria-busy={pending === true || undefined}
            onClick={(event) => {
              if (awaitsResult) event.preventDefault();
              onConfirm();
            }}
          >
            {actionLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
