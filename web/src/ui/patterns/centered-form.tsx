import { type FormEventHandler, type ReactNode, useId } from "react";

import { FieldGroup } from "@/ui/shadcn/field";

// For screens centered on one form; see web/registry/rules/patterns.md, Centered form.

export interface CenteredFormProps {
  title: ReactNode;
  description?: ReactNode;
  onSubmit: FormEventHandler<HTMLFormElement>;
  children: ReactNode;
  submit: ReactNode;
  footer?: ReactNode;
}

export function CenteredForm({
  title,
  description,
  onSubmit,
  children,
  submit,
  footer,
}: CenteredFormProps) {
  const titleId = useId();
  return (
    <div
      data-slot="centered-form"
      className="flex min-h-full flex-1 flex-col items-center justify-center gap-4 p-4 sm:p-8"
    >
      <form
        aria-labelledby={titleId}
        onSubmit={onSubmit}
        className="flex w-full max-w-sm flex-col gap-6 rounded-lg border border-border bg-card p-6 text-card-foreground"
      >
        <div className="flex flex-col gap-1">
          <h1 id={titleId} className="text-xl font-semibold">
            {title}
          </h1>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
        <FieldGroup>{children}</FieldGroup>
        <div data-slot="centered-form-submit" className="flex flex-col *:w-full">
          {submit}
        </div>
      </form>
      {footer && (
        <div data-slot="centered-form-footer" className="text-sm text-muted-foreground">
          {footer}
        </div>
      )}
    </div>
  );
}
