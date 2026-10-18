import type { ReactNode } from "react";

import { Field, FieldContent, FieldDescription, FieldLabel } from "@/ui/shadcn/field";

// See web/registry/rules/patterns.md, Form row.

export interface FormRowProps {
  label: ReactNode;
  description?: ReactNode;
  /** Referenced by the control's aria-describedby. */
  descriptionId?: string;
  htmlFor: string;
  /** Supply exactly one control. */
  children: ReactNode;
}

export function FormRow({
  label,
  description,
  descriptionId,
  htmlFor,
  children,
}: FormRowProps) {
  return (
    <Field data-slot="form-row" orientation="responsive" className="sm:gap-6">
      <FieldContent>
        <FieldLabel htmlFor={htmlFor}>{label}</FieldLabel>
        {description && (
          <FieldDescription id={descriptionId}>{description}</FieldDescription>
        )}
      </FieldContent>
      <div
        data-slot="form-row-control"
        className="flex items-center sm:shrink-0 sm:justify-end"
      >
        {children}
      </div>
    </Field>
  );
}
