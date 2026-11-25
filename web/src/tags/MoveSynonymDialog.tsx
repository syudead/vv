import { useRef } from "react";

import { moveTagSynonym, type Tag } from "../api/tags";
import { t } from "../i18n";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Spinner } from "../ui/shadcn/spinner";
import { MergeSummary } from "../ui/MergeParts";
import { useToast } from "../ui/Toast";
import { TagTargetField, useTagTarget } from "./TagTargetField";
import type { TagRecord } from "./useTagRecord";

const text = t.tags.page.moveDialog;

/**
 * Moves one synonym to a tag this node holds: a move names its target by id, and a tag only the paired
 * VVMDM holds answers 404.
 */
export function MoveSynonymDialog({
  tag,
  name,
  record,
  onClose,
}: {
  tag: Tag;
  name: string;
  record: TagRecord;
  onClose: () => void;
}) {
  const toast = useToast();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const choice = useTagTarget(tag.id, true);
  const { target } = choice;
  const { busy, run, setTag } = record;
  const pending = busy === "move";

  function submit() {
    if (target === null) return;
    void run({
      key: "move",
      name,
      write: () => moveTagSynonym(tag.id, name, target.id),
      done: ({ source, target: moved }) => {
        setTag(source);
        toast(t.tags.page.moved(name, moved.name));
      },
    }).then(onClose);
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={text.title(name)}
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? text.submitting : text.submit}
        </>
      }
      pending={pending}
      submitDisabled={target === null || pending}
      cancelRef={cancelRef}
      initialFocus={cancelRef}
    >
      <TagTargetField label={text.target} choice={choice} />
      {target !== null && <MergeSummary source={name} target={target.name} />}
    </FormDialog>
  );
}
