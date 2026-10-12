import { ImageIcon, WandSparkles } from "lucide-react";
import { useRef, useState } from "react";

import { t } from "@/i18n";
import { EditorDialog } from "@/ui/patterns/editor-dialog";
import { Button } from "@/ui/shadcn/button";
import { Field, FieldDescription, FieldLabel } from "@/ui/shadcn/field";
import { Textarea } from "@/ui/shadcn/textarea";

// 編集画面の型の見本（registry:block editor-dialog-example）。1 枚の画像を段に置き、横の欄に
// 指示の欄、足に主な操作を置く。写した画面は段の中身・欄・操作を差し替える
// （web/registry/rules/patterns.md の Editor dialog）。

export function EditorDialogExample({ defaultOpen = false }: { defaultOpen?: boolean }) {
  const p = t.designSystem.pattern;
  const [open, setOpen] = useState(defaultOpen);
  const notes = useRef<HTMLTextAreaElement | null>(null);
  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <WandSparkles aria-hidden="true" />
        {p.editImage}
      </Button>
      <EditorDialog
        open={open}
        onClose={() => setOpen(false)}
        title={p.editImage}
        detail={p.editImageDetail}
        closeLabel={p.close}
        initialFocus={notes}
        stage={
          <div
            role="img"
            aria-label={p.editImageStage}
            className="flex aspect-video w-full max-w-full items-center justify-center rounded-sm bg-card text-muted-foreground"
          >
            <ImageIcon className="size-8" aria-hidden="true" />
          </div>
        }
        aside={
          <Field>
            <FieldLabel htmlFor="editor-dialog-example-notes">
              {p.editImageNotes}
            </FieldLabel>
            <Textarea id="editor-dialog-example-notes" ref={notes} rows={4} />
            <FieldDescription>{p.editImageNotesHint}</FieldDescription>
          </Field>
        }
        footer={
          <Button size="lg" className="w-full" onClick={() => setOpen(false)}>
            {p.editImageApply}
          </Button>
        }
      />
    </>
  );
}
