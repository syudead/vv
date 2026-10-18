import { Ban, Check, Merge, Trash2 } from "lucide-react";
import { useId, type Ref } from "react";

import type { TagKind } from "../api/tagKinds";
import { maxTagBatch } from "../api/tags";
import { t } from "../i18n";
import { SelectionBar } from "../ui/patterns/selection-bar";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import TagSetKindMenu, { type KindChoice } from "./TagSetKindMenu";

/** Hide inapplicable batch actions and keep labels visible at every width. */
export default function TagSelectionBar({
  count,
  hasTentative,
  hasConfirmed,
  overLimit,
  busy,
  confirming,
  onConfirm,
  onReject,
  onDelete,
  onMerge,
  onClear,
  kinds,
  sharedKind,
  settingKind,
  onSetKind,
  setKindRef,
  confirmRef,
  mergeRef,
  rejectRef,
  deleteRef,
}: {
  count: number;
  hasTentative: boolean;
  hasConfirmed: boolean;
  overLimit: boolean;
  busy: boolean;
  confirming: boolean;
  onConfirm: () => void;
  onReject: () => void;
  onDelete: () => void;
  onMerge: () => void;
  onClear: () => void;
  kinds: readonly TagKind[] | undefined;
  /** Shared kind ID or none; undefined for mixed kinds. */
  sharedKind: string | undefined;
  settingKind: boolean;
  onSetKind: (choice: KindChoice) => void;
  setKindRef?: Ref<HTMLButtonElement>;
  confirmRef?: Ref<HTMLButtonElement>;
  mergeRef?: Ref<HTMLButtonElement>;
  rejectRef?: Ref<HTMLButtonElement>;
  deleteRef?: Ref<HTMLButtonElement>;
}) {
  const overLimitId = useId();
  const disabled = busy || overLimit;
  const reason = overLimit && !busy ? t.tags.selection.overLimit(maxTagBatch) : undefined;
  const describedBy = reason === undefined ? undefined : overLimitId;

  return (
    <div role="region" aria-label={t.tags.selection.region} className="contents">
      <SelectionBar
        count={t.tags.selection.count(count)}
        clearLabel={t.tags.selection.clear}
        clearDisabled={busy}
        onClear={onClear}
        placement="header"
      >
        {hasTentative && (
          <Button
            ref={confirmRef}
            variant="ghost"
            size="sm"
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onConfirm}
          >
            {confirming ? <Spinner aria-hidden="true" /> : <Check aria-hidden="true" />}
            <span>{t.tags.selection.confirm}</span>
          </Button>
        )}
        <TagSetKindMenu
          kinds={kinds}
          checked={sharedKind}
          disabled={disabled}
          pending={settingKind}
          title={reason}
          describedBy={describedBy}
          onChoose={onSetKind}
          buttonRef={setKindRef}
        />
        <Button
          ref={mergeRef}
          variant="ghost"
          size="sm"
          disabled={disabled}
          title={reason}
          aria-describedby={describedBy}
          onClick={onMerge}
        >
          <Merge aria-hidden="true" />
          <span>{t.tags.selection.mergeInto}</span>
        </Button>
        {hasTentative && (
          <Button
            ref={rejectRef}
            variant="ghost"
            size="sm"
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onReject}
            className="text-destructive"
          >
            <Ban aria-hidden="true" />
            <span>{t.tags.selection.reject}</span>
          </Button>
        )}
        {hasConfirmed && (
          <Button
            ref={deleteRef}
            variant="ghost"
            size="sm"
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onDelete}
            className="text-destructive"
          >
            <Trash2 aria-hidden="true" />
            <span>{t.tags.selection.delete}</span>
          </Button>
        )}
        {reason !== undefined && (
          <span id={overLimitId} className="sr-only">
            {reason}
          </span>
        )}
      </SelectionBar>
    </div>
  );
}
