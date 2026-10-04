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

// フォームダイアログ（骨格）。題・説明・欄と、Cancel と主操作 1 つの足を持つ、幅が最大
// 32rem（max-w-lg）のダイアログ。今の画面を離れずに少しの値を聞くときに使う。IME の変換を
// 取り消す Esc では閉じない。
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
  /** 送信中。主操作と Cancel を押せなくする。 */
  pending?: boolean;
  /** 主操作をまだ押せない（必要な値を選んでいない、数えている間など）。 */
  submitDisabled?: boolean;
  /** 主操作のボタン。値を選んだ後や失敗の後にフォーカスを移すときに使う。 */
  submitRef?: Ref<HTMLButtonElement>;
  /** Cancel のボタン。 */
  cancelRef?: Ref<HTMLButtonElement>;
  /** 開いたときにフォーカスを置く要素。無ければ最初に押せる要素。 */
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
  pending = false,
  submitDisabled = false,
  submitRef,
  cancelRef,
  initialFocus,
}: FormDialogProps) {
  // 開く前にフォーカスを持っていた要素。開くボタン（trigger）を渡さずに開閉するときは、
  // 閉じたらここへ戻す（Radix は DialogTrigger にしか戻さない）。
  const previousFocus = useRef<HTMLElement | null>(null);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent
        data-slot="form-dialog"
        showCloseButton={false}
        onEscapeKeyDown={(event) => {
          // IME の変換を取り消す Esc では閉じない。
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
        <form onSubmit={onSubmit} className="flex min-w-0 flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description && <DialogDescription>{description}</DialogDescription>}
          </DialogHeader>
          <FieldGroup>{children}</FieldGroup>
          <DialogFooter>
            <DialogClose asChild>
              <Button ref={cancelRef} variant="outline" size="sm" disabled={pending}>
                {cancelLabel}
              </Button>
            </DialogClose>
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
