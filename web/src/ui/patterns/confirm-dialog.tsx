import type { ReactNode } from "react";

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

// 確認ダイアログ（骨格）。取り消せない操作の前に、何が起きるかと、Cancel と動詞で名付けた
// destructive の操作を出す AlertDialog。規則は web/registry/rules/patterns.md の
// Confirm dialog。

export interface ConfirmDialogProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  /** 開くボタン。開閉を呼び出し側が持つときは渡さない。 */
  trigger?: ReactNode;
  /** 何が起きるか（「Delete "Travel"?」）。 */
  title: ReactNode;
  /** 何に及ぶか、何が残るか。 */
  description: ReactNode;
  /** 操作の動詞（「Delete」）。「OK」にしない。 */
  actionLabel: ReactNode;
  cancelLabel: ReactNode;
  onConfirm: () => void;
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
}: ConfirmDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      {trigger && <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>}
      <AlertDialogContent data-slot="confirm-dialog">
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{cancelLabel}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            {actionLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
