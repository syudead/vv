import { useState } from "react";

import { RequestFailed } from "../api/client";
import { addTagSynonym, type Tag } from "../api/tags";
import { errorText, t } from "../i18n";
import { isRemoteId } from "../lib/remoteId";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { nameInputProps, useTagNameField, type TagFieldError } from "../ui/nameField";
import { tagFieldError } from "./tagNameField";
import { collectTags } from "./tagTargetSearch";
import type { TagRecord } from "./useTagRecord";

const text = t.tags.page;
const ownerPageLimit = 8;

interface MergeAsk {
  name: string;
  owner: Tag;
}

/**
 * This node's tag whose main name is name, read fresh so the merge names the current owner. The
 * search's exact match can be a tag holding name only as a synonym a paired VVMDM gave it.
 */
async function findOwner(name: string): Promise<Tag | undefined> {
  const isOwner = (item: Tag) => item.name === name && !isRemoteId(item.id);
  const { kept, exact } = await collectTags(name, ownerPageLimit, isOwner, 1);
  return exact !== null && isOwner(exact) ? exact : kept[0];
}

/** `Add a name`: Enter adds; a name that is another tag asks before merging that tag into this one. */
export function AddTagNameRow({ tag, record }: { tag: Tag; record: TagRecord }) {
  const [error, setError] = useState<TagFieldError | null>(null);
  const field = useTagNameField("", () => setError(null));
  const [ask, setAsk] = useState<MergeAsk | null>(null);
  const { busy, run, setTag } = record;

  async function askMerge(name: string, ownerName: string) {
    try {
      const owner = await findOwner(ownerName);
      if (owner === undefined) setError({ kind: "other", message: t.tags.gone });
      else setAsk({ name, owner });
    } catch (failure) {
      setError({ kind: "other", message: errorText(failure) });
    }
  }

  function add(name: string, mergeTagId?: number) {
    // Any failure closes the question; a new owner asks again once it is read.
    void run({
      key: mergeTagId === undefined ? "add" : "add-merge",
      write: () => addTagSynonym(tag.id, name, mergeTagId),
      done: (updated) => {
        setTag(updated);
        setAsk(null);
        if (field.value.trim() === name) field.setValue("");
      },
      failed: (failure) => {
        if (!(failure instanceof RequestFailed)) return false;
        if (failure.code === "tag_merge_required") {
          void askMerge(name, failure.tagName ?? name);
          return true;
        }
        if (failure.code !== "tag_name_taken" && failure.code !== "invalid_request") {
          return false;
        }
        setError(tagFieldError(failure, { submitted: name, ownTagName: tag.name }));
        return true;
      },
    }).then((added) => {
      if (!added) setAsk(null);
    });
  }

  function submit() {
    if (busy !== null) return;
    const name = field.trySpelling();
    if (name !== null) add(name);
  }

  const message = field.reason ?? error?.message ?? null;
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-2">
        <Input
          {...nameInputProps(field, submit)}
          aria-label={text.addName}
          placeholder={text.addName}
          aria-busy={busy === "add" || undefined}
          aria-invalid={message !== null || undefined}
          aria-describedby={message !== null ? "tag-add-name-error" : undefined}
          className="h-8"
        />
        {busy === "add" && <Spinner aria-hidden="true" />}
      </div>
      {message !== null && (
        <p
          id="tag-add-name-error"
          role={error?.kind === "other" ? "alert" : undefined}
          className="text-xs text-destructive"
        >
          {message}
        </p>
      )}
      {ask !== null && (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open && busy === null) setAsk(null);
          }}
          title={t.tags.mergeDialog.title(ask.owner.name)}
          description={t.tags.synonymsDialog.mergeWarning(
            ask.owner.name,
            ask.owner.videoCount,
            tag.name,
            ask.owner.synonyms.length > 0,
          )}
          cancelLabel={t.common.cancel}
          actionLabel={
            <>
              {busy === "add-merge" && <Spinner aria-hidden="true" />}
              {t.tags.synonymsDialog.merge}
            </>
          }
          pending={busy === "add-merge"}
          onConfirm={() => add(ask.name, ask.owner.id)}
        />
      )}
    </div>
  );
}
