import type { ReactNode, Ref } from "react";

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
//
// 操作が要求を送るときは pending を渡す。そのときは操作を押しても閉じず、呼び出し側が
// 要求の成功で閉じる。送っている間は両方のボタンを押せなくし、失敗は children に出して
// 窓を開いたままにする。

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
  /**
   * 操作の要求を送っている間 true。渡すと操作で閉じず、呼び出し側が成功で閉じる。
   * 渡さなければ操作を押したときに閉じる。
   */
  pending?: boolean;
  /** 操作をまだ押せない（影響を数えている間など）。 */
  actionDisabled?: boolean;
  /** 説明の下に置く、数えている間の表示や失敗の Alert。 */
  children?: ReactNode;
  /** 操作のボタン。失敗の後にフォーカスを戻すときに使う。 */
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
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      {trigger && <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>}
      <AlertDialogContent
        data-slot="confirm-dialog"
        onEscapeKeyDown={(event) => {
          // IME の変換を取り消す Esc では閉じない。
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
