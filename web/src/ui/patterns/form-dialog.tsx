import {
  type FormEventHandler,
  type ReactNode,
  type Ref,
  type RefObject,
  useRef,
} from "react";

import { Button } from "@/ui/shadcn/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/ui/shadcn/dialog";
import { FieldGroup } from "@/ui/shadcn/field";
import { cn } from "@/lib/cn";

// Use wide for side-by-side comparisons; omit cancelLabel for final steps with only a primary action. See web/registry/rules/patterns.md, Form dialog.

export interface FormDialogProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  trigger?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  onSubmit: FormEventHandler<HTMLFormElement>;
  children: ReactNode;
  submitLabel: ReactNode;
  cancelLabel?: ReactNode;
  size?: "default" | "wide";
  pending?: boolean;
  submitDisabled?: boolean;
  submitRef?: Ref<HTMLButtonElement>;
  cancelRef?: Ref<HTMLButtonElement>;
  initialFocus?: RefObject<HTMLElement | null>;
}

export function FormDialog({
  open,
  onOpenChange,
  trigger,
  title,
  description,
  onSubmit,
  children,
  submitLabel,
  cancelLabel,
  size = "default",
  pending = false,
  submitDisabled = false,
  submitRef,
  cancelRef,
  initialFocus,
}: FormDialogProps) {
  // Restore pre-open focus for controlled dialogs without a trigger; Radix restores only DialogTrigger.
  const previousFocus = useRef<HTMLElement | null>(null);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent
        data-slot="form-dialog"
        showCloseButton={false}
        // On short screens, scroll the body while retaining the title and actions.
        className={cn("flex flex-col overflow-hidden", size === "wide" && "max-w-2xl")}
        onEscapeKeyDown={(event) => {
          // IME cancellation Escape must not close the dialog.
          if (event.isComposing || event.keyCode === 229) event.preventDefault();
        }}
        onCloseAutoFocus={(event) => {
          if (trigger) return;
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
        <form onSubmit={onSubmit} className="flex min-h-0 min-w-0 flex-1 flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description && <DialogDescription>{description}</DialogDescription>}
          </DialogHeader>
          <div
            data-slot="form-dialog-body"
            className="-mx-1 min-h-0 flex-1 overflow-y-auto px-1"
          >
            <FieldGroup>{children}</FieldGroup>
          </div>
          <DialogFooter>
            {cancelLabel !== undefined && (
              <DialogClose asChild>
                <Button ref={cancelRef} variant="outline" size="sm" disabled={pending}>
                  {cancelLabel}
                </Button>
              </DialogClose>
            )}
            <Button
              ref={submitRef}
              type="submit"
              size="sm"
              disabled={pending || submitDisabled}
              aria-busy={pending || undefined}
            >
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
