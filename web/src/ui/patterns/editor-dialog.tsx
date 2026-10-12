import { XIcon } from "lucide-react";
import { type ReactNode, type RefObject, useRef } from "react";

import { Button } from "@/ui/shadcn/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/ui/shadcn/dialog";

// 編集画面（骨格）。画面の端から端までを覆う、1 枚の画像を相手にする作業のためのモーダルの
// Dialog。上端の帯（題・薄い詳細・閉じる ×、下に線）、画像を置く段（stage）、入力を並べる
// 横の欄（aside）と、欄の主な操作の足（footer）を持つ。lg からは段が残りを埋め、欄は
// detail-aside（xl からは detail-aside-wide）の幅の列で左に線を引く。lg 未満は段を帯の下に
// editor-stage の高さで置き、その下に欄が続き、足は画面の下に貼り付く。
// Esc と × は onClose を呼ぶ。閉じる前に確かめるかは呼び出し側が決める（ConfirmDialog）。
// 規則は web/registry/rules/patterns.md の Editor dialog。

export interface EditorDialogProps {
  open: boolean;
  /** Esc と × で呼ぶ。閉じてよければ呼び出し側が open を false にする。 */
  onClose: () => void;
  /** 作業の名前（「Edit thumbnail」）。 */
  title: ReactNode;
  /** 題の後ろの薄い詳細（「Frame at 1:02」）。 */
  detail: ReactNode;
  /** 閉じる × の読み上げ名。 */
  closeLabel: string;
  /** × を押せなくする（送信中など）。 */
  closeDisabled?: boolean;
  /** 段の中身。段は中央に寄せ、muted の地に p-4 を取る。 */
  stage: ReactNode;
  /** 横の欄の入力と表示。上から入力の順に並べる。 */
  aside: ReactNode;
  /** 欄の足の主な操作。lg からは欄の下端、lg 未満は画面の下に貼り付く。 */
  footer: ReactNode;
  /** 開いたときにフォーカスを置く要素。無ければ最初に押せる要素。 */
  initialFocus?: RefObject<HTMLElement | null>;
}

export function EditorDialog({
  open,
  onClose,
  title,
  detail,
  closeLabel,
  closeDisabled = false,
  stage,
  aside,
  footer,
  initialFocus,
}: EditorDialogProps) {
  // 開く前にフォーカスを持っていた要素（入口のボタン）。閉じたらそこへ戻す。
  const previousFocus = useRef<HTMLElement | null>(null);
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent
        data-slot="editor-dialog"
        showCloseButton={false}
        className="inset-0 m-0 flex max-h-none max-w-none translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 text-foreground shadow-none"
        onEscapeKeyDown={(event) => {
          // IME の変換を取り消す Esc では閉じない。
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
        <div
          data-slot="editor-dialog-header"
          className="flex h-navbar shrink-0 items-center gap-3 border-b border-border px-4"
        >
          <div className="flex min-w-0 flex-1 items-baseline gap-3">
            <DialogTitle className="shrink-0 text-lg leading-none font-semibold">
              {title}
            </DialogTitle>
            <DialogDescription className="truncate text-sm text-muted-foreground tabular-nums">
              {detail}
            </DialogDescription>
          </div>
          <DialogClose asChild disabled={closeDisabled}>
            <Button variant="ghost" size="icon-sm" aria-label={closeLabel}>
              <XIcon aria-hidden="true" />
            </Button>
          </DialogClose>
        </div>
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain lg:flex-row lg:overflow-hidden">
          <div
            data-slot="editor-dialog-stage"
            className="flex h-editor-stage shrink-0 items-center justify-center overflow-hidden bg-muted p-4 lg:h-auto lg:min-w-0 lg:flex-1 lg:shrink"
          >
            {stage}
          </div>
          <div
            data-slot="editor-dialog-aside"
            className="flex flex-1 flex-col border-border lg:min-h-0 lg:w-detail-aside lg:flex-none lg:border-l xl:w-detail-aside-wide"
          >
            <div className="flex flex-1 flex-col gap-4 p-4 lg:min-h-0 lg:overflow-y-auto">
              {aside}
            </div>
            <div
              data-slot="editor-dialog-footer"
              className="sticky bottom-0 flex flex-col gap-2 border-t border-border bg-background p-4"
            >
              {footer}
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
