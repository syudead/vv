import { useState } from "react";

import { mergeTag, type Tag } from "../api/tags";
import { t } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import { TagTargetField, useTagTarget } from "./TagTargetField";
import type { TagRecord } from "./useTagRecord";

const text = t.tags.page;

/** Merge this tag into the chosen one: its names become the target's synonyms and it leaves the list. */
export function TagMergeSection({
  tag,
  record,
  onMerged,
}: {
  tag: Tag;
  record: TagRecord;
  onMerged: (target: Tag) => void;
}) {
  const [asking, setAsking] = useState(false);
  const choice = useTagTarget(tag.id);
  const { target } = choice;
  const { busy, run } = record;
  const pending = busy === "merge";

  function merge() {
    if (target === null) return;
    void run({
      key: "merge",
      write: () => mergeTag(target.id, [tag.id]),
      done: ({ tag: merged, notFoundIds }) => {
        // This tag was gone before the merge, so nothing merged; the re-read shows the gone state.
        if (notFoundIds.includes(tag.id)) record.load();
        else onMerged(merged);
      },
    }).then(() => setAsking(false));
  }

  return (
    <PageSection title={text.mergeTitle}>
      <div className="flex flex-col gap-3">
        <p className="text-muted-foreground">{text.mergeDescription(tag.name)}</p>
        <TagTargetField label={text.tagToKeep} choice={choice} layout="dropdown" />
        <Button
          size="sm"
          className="self-start"
          disabled={target === null}
          aria-disabled={busy !== null || undefined}
          onClick={() => {
            if (busy === null) setAsking(true);
          }}
        >
          {text.mergeAction}
        </Button>
      </div>
      {asking && target !== null && (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open && !pending) setAsking(false);
          }}
          title={t.tags.mergeDialog.title(tag.name)}
          description={t.tags.mergeDialog.warning(tag.name, tag.videoCount, target.name)}
          cancelLabel={t.common.cancel}
          actionLabel={
            <>
              {pending && <Spinner aria-hidden="true" />}
              {pending ? t.tags.mergeDialog.submitting : t.tags.mergeDialog.submit}
            </>
          }
          pending={pending}
          onConfirm={merge}
        />
      )}
    </PageSection>
  );
}
