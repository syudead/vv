import { type FormEventHandler, type ReactNode, useId } from "react";

import { FieldGroup } from "@/ui/shadcn/field";

// 中央フォーム（骨格）。囲む領域の中央に 1 枚のカードを置き、題・説明・欄・主操作を
// 縦に並べる。サインインや初期設定のように、その画面の目的が 1 つのフォームだけのときに
// 使う。規則は web/registry/rules/patterns.md の Centered form。

export interface CenteredFormProps {
  /** フォームの題。h1 になる。 */
  title: ReactNode;
  /** 題の下の説明。 */
  description?: ReactNode;
  onSubmit: FormEventHandler<HTMLFormElement>;
  /** Field の並び。 */
  children: ReactNode;
  /** 送信の Button（default、1 つ）。カードの幅いっぱいに伸びる。 */
  submit: ReactNode;
  /** カードの下の補足（別の画面へのリンクなど）。 */
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
