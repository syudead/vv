import { Trash2 } from "lucide-react";
import { useState } from "react";

import { t } from "@/i18n";
import { ConfirmDialog } from "@/ui/patterns/confirm-dialog";
import { Button } from "@/ui/shadcn/button";

// 確認ダイアログの型の見本（registry:block confirm-dialog-example）。タグを消す前の確認で、
// 写した画面は題・説明・動詞と、確かめた後の処理を差し替える
// （web/registry/rules/patterns.md の Confirm dialog）。

export function ConfirmDialogExample({ defaultOpen = false }: { defaultOpen?: boolean }) {
  const p = t.designSystem.pattern;
  const [open, setOpen] = useState(defaultOpen);
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button variant="outline" size="sm">
          <Trash2 aria-hidden="true" />
          {p.deleteTag}
        </Button>
      }
      title={p.deleteTagTitle(p.tagNames.travel)}
      description={p.deleteTagDescription}
      actionLabel={p.delete}
      cancelLabel={p.cancel}
      onConfirm={() => {
        setOpen(false);
      }}
    />
  );
}
