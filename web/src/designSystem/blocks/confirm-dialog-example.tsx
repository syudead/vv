import { Trash2 } from "lucide-react";
import { useState } from "react";

import { t } from "@/i18n";
import { ConfirmDialog } from "@/ui/patterns/confirm-dialog";
import { Button } from "@/ui/shadcn/button";

// registry:block confirm-dialog-example. Replace the copy and confirmed action.

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
