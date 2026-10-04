import type { ReactNode } from "react";

import { Field, FieldContent, FieldDescription, FieldLabel } from "@/ui/shadcn/field";

// フォームの行（区画）。左にラベルと説明、右に操作を置き、狭い幅では縦に積む。節
// （PageSection）の中に並べ、行の余白と区切り線は節が付ける。
// 規則は web/registry/rules/patterns.md の Form row。

export interface FormRowProps {
  /** 操作の見える名前。 */
  label: ReactNode;
  /** ラベルの下の説明。 */
  description?: ReactNode;
  /** ラベルを結び付ける操作の id。 */
  htmlFor: string;
  /** 操作（Switch、Select、Input、Button）。1 つにする。 */
  children: ReactNode;
}

export function FormRow({ label, description, htmlFor, children }: FormRowProps) {
  return (
    <Field data-slot="form-row" orientation="responsive" className="sm:gap-6">
      <FieldContent>
        <FieldLabel htmlFor={htmlFor}>{label}</FieldLabel>
        {description && <FieldDescription>{description}</FieldDescription>}
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
