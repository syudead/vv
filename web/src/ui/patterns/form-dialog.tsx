import type { FormEventHandler, ReactNode } from "react";

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

// フォームダイアログ（骨格）。題・説明・欄と、Cancel と主操作 1 つの足を持つ、幅が最大
// 32rem（max-w-lg）のダイアログ。今の画面を離れずに少しの値を聞くときに使う。
// 規則は web/registry/rules/patterns.md の Form dialog。

export interface FormDialogProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  /** 開くボタン。開閉を呼び出し側が持つときは渡さない。 */
  trigger?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  onSubmit: FormEventHandler<HTMLFormElement>;
  /** Field の並び。 */
  children: ReactNode;
  /** 主操作の文言。何をするかの動詞（「Create」）。 */
  submitLabel: ReactNode;
  cancelLabel: ReactNode;
  /** 送信中。主操作を押せなくする。 */
  pending?: boolean;
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
  pending = false,
}: FormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent data-slot="form-dialog" showCloseButton={false}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description && <DialogDescription>{description}</DialogDescription>}
          </DialogHeader>
          <FieldGroup>{children}</FieldGroup>
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" size="sm">
                {cancelLabel}
              </Button>
            </DialogClose>
            <Button type="submit" size="sm" disabled={pending} aria-busy={pending}>
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
