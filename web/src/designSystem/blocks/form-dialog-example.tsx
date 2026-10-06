import { Plus } from "lucide-react";
import { type FormEvent, useState } from "react";

import { t } from "@/i18n";
import { FormDialog } from "@/ui/patterns/form-dialog";
import { Button } from "@/ui/shadcn/button";
import { Field, FieldDescription, FieldLabel } from "@/ui/shadcn/field";
import { Input } from "@/ui/shadcn/input";
import { Textarea } from "@/ui/shadcn/textarea";

// フォームダイアログの型の見本（registry:block form-dialog-example）。タグを作る形で、
// 写した画面は欄と送信の処理を差し替える（web/registry/rules/patterns.md の Form dialog）。

export function FormDialogExample({ defaultOpen = false }: { defaultOpen?: boolean }) {
  const p = t.designSystem.pattern;
  const [open, setOpen] = useState(defaultOpen);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setOpen(false);
  };
  return (
    <FormDialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button size="sm">
          <Plus aria-hidden="true" />
          {p.newTag}
        </Button>
      }
      title={p.newTag}
      description={p.newTagDescription}
      onSubmit={submit}
      submitLabel={p.createTag}
      cancelLabel={p.cancel}
    >
      <Field>
        <FieldLabel htmlFor="form-dialog-example-name">{p.tagName}</FieldLabel>
        <Input id="form-dialog-example-name" defaultValue={p.tagNames.travel} />
        <FieldDescription>{p.tagNameDescription}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor="form-dialog-example-notes">{p.tagNotes}</FieldLabel>
        <Textarea id="form-dialog-example-notes" />
      </Field>
    </FormDialog>
  );
}
