import { type ReactNode, type RefObject, useRef } from "react";

import { cn } from "@/lib/cn";
import { Dialog, DialogContent, DialogTitle } from "@/ui/shadcn/dialog";
import { FieldLegend, FieldSet } from "@/ui/shadcn/field";

// Full-page batch editing with independent wide-screen columns; see web/registry/rules/patterns.md, Form page.

export interface FormPagePanel {
  key: string;
  legend: ReactNode;
  content: ReactNode;
}

export interface FormPageProps {
  title: ReactNode;
  detail?: ReactNode;
  actions: ReactNode;
  notice?: ReactNode;
  aside?: ReactNode;
  asideLabel?: string;
  panels: readonly FormPagePanel[];
  /** Replaces columns with a body state such as a load failure. */
  status?: ReactNode;
}

export function FormPage({
  title,
  detail,
  actions,
  notice,
  aside,
  asideLabel,
  panels,
  status,
}: FormPageProps) {
  return (
    <div
      data-slot="form-page"
      className="flex min-h-dvh flex-col bg-background text-foreground lg:h-dvh lg:min-h-0 lg:overflow-hidden"
    >
      <header
        data-slot="form-page-header"
        className="sticky top-0 z-20 flex h-navbar shrink-0 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur-md sm:px-6"
      >
        <div className="flex min-w-0 flex-1 items-baseline gap-3">
          {/* A title that names what is edited truncates; beside a detail it keeps its width. */}
          <h1
            className={cn(
              "text-lg font-semibold",
              // Clipping needs the full line height, or descenders are cut off.
              detail === undefined
                ? "min-w-0 truncate leading-normal"
                : "shrink-0 leading-none",
            )}
          >
            {title}
          </h1>
          {detail !== undefined && (
            <p className="hidden min-w-0 truncate text-sm text-muted-foreground sm:block">
              {detail}
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      </header>
      <div
        data-slot="form-page-body"
        className="flex flex-1 flex-col gap-6 px-4 py-6 sm:px-6 lg:min-h-0"
      >
        {notice}
        {status ?? (
          <div className="grid flex-1 grid-cols-1 gap-6 lg:min-h-0 lg:grid-cols-form-page">
            {aside !== undefined && (
              <aside
                aria-label={asideLabel}
                className="flex min-w-0 flex-col gap-3 text-sm lg:-mx-1 lg:min-h-0 lg:overflow-y-auto lg:px-1"
              >
                {aside}
              </aside>
            )}
            {panels.map((panel, index) => (
              <div
                key={panel.key}
                data-slot="form-page-panel"
                className={cn(
                  // Pad scrolling fields so input focus rings are not clipped.
                  "min-w-0 lg:-mx-1 lg:min-h-0 lg:overflow-y-auto lg:px-1",
                  (panels.length === 1 || (aside === undefined && index === 0)) &&
                    "lg:col-span-2",
                )}
              >
                <FieldSet className="min-h-full min-w-0">
                  <FieldLegend>{panel.legend}</FieldLegend>
                  {panel.content}
                </FieldSet>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export interface FormPageDialogProps extends FormPageProps {
  open: boolean;
  /** Esc requests closing; the caller sets open=false once it accepts, or first asks to discard. */
  onClose: () => void;
  /** Defaults to the first focusable element. */
  initialFocus?: RefObject<HTMLElement | null>;
}

/**
 * A FormPage over the current screen, for a form that must keep the screen under it (its selection
 * and scroll position); see web/registry/rules/patterns.md, Form page.
 */
export function FormPageDialog({
  open,
  onClose,
  initialFocus,
  title,
  ...page
}: FormPageDialogProps) {
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
        data-slot="form-page-dialog"
        showCloseButton={false}
        aria-describedby={undefined}
        className="inset-0 m-0 block max-h-none max-w-none translate-y-0 overflow-y-auto overscroll-contain rounded-none bg-background p-0 text-foreground shadow-none"
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
        <FormPage
          {...page}
          title={
            <DialogTitle asChild>
              <span>{title}</span>
            </DialogTitle>
          }
        />
      </DialogContent>
    </Dialog>
  );
}
