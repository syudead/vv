import { type ReactNode, useState } from "react";

import type { Credit, CreditKind } from "../../api/credits";
import { t, type UiText } from "../../i18n";
import {
  EditorBand,
  FailureLine,
  failureText,
} from "../../player/detailsEditor/EditorBand";
import type { SaveFailure } from "../../player/detailsEditor/useDetailsSave";
import { useLeaving } from "../../player/detailsEditor/useLeaving";
import { ConfirmDialog } from "../../ui/patterns/confirm-dialog";
import { ErrorState } from "../../ui/patterns/error-state";
import { FormPage, type FormPagePanel } from "../../ui/patterns/form-page";
import { Field, FieldTitle } from "../../ui/shadcn/field";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { useDocumentTitle } from "../../videoList/pageHooks";
import { creditDisplayName } from "../displayName";
import { useCreditRecord } from "../useCreditRecord";

// The full-page editor of a person or an organization: the record's read, leaving with the discard
// check, and the band; each kind supplies its aside and panels.

const text = t.credits.personEdit;

/** What a kind's form receives once the record is read. */
export interface CreditFormProps<T extends Credit> {
  credit: T;
  /**
   * The route's id, where the save goes: a remote record read by its offset id answers with the id
   * of this node's record of its name, whose profile the form does not show.
   */
  id: number;
  onLeave: () => void;
  onSaved: (credit: T) => void;
  onGone: () => void;
  onChangedChange: (changed: boolean) => void;
  onSavingChange: (saving: boolean) => void;
}

export function CreditEditor<T extends Credit>({
  kind,
  id,
  handed,
  backTo,
  handBack,
  failedTitle,
  gone,
  aside,
  asideLabel,
  loadingPanels,
  form,
}: {
  kind: CreditKind;
  id: number | null;
  handed?: T;
  /** The record's page, or its list without an id. */
  backTo: string;
  /** Runs with the saved record before leaving, so the page can show it without a read. */
  handBack?: (saved: T) => void;
  failedTitle: (reason: UiText) => string;
  gone: ReactNode;
  /** The aside, read only; `credit` is undefined while loading. */
  aside: (credit: T | undefined, onBack: () => void) => ReactNode;
  asideLabel: string;
  loadingPanels: readonly FormPagePanel[];
  form: (props: CreditFormProps<T>) => ReactNode;
}) {
  const { state, reload, markGone } = useCreditRecord<T>(kind, id, handed);
  const [changed, setChanged] = useState(false);
  const [saving, setSaving] = useState(false);
  const leaving = useLeaving<T>({
    pagePath: backTo,
    changed: state.kind === "ready" && changed,
    saving,
    handBack: (saved) => {
      if (saved !== undefined) handBack?.(saved);
    },
  });
  const credit = state.kind === "ready" ? state.credit : undefined;
  useDocumentTitle(
    credit === undefined ? undefined : text.title(creditDisplayName(credit)),
  );

  if (credit !== undefined && id !== null) {
    return (
      <>
        {form({
          credit,
          id,
          onLeave: leaving.requestLeave,
          onSaved: (saved) => leaving.leave(saved),
          onGone: () => {
            // The form unmounts mid-save, so it never reports the save's end.
            setChanged(false);
            setSaving(false);
            markGone();
          },
          onChangedChange: setChanged,
          onSavingChange: setSaving,
        })}
        <ConfirmDialog
          open={leaving.confirmOpen}
          onOpenChange={(open) => {
            if (!open) leaving.keepEditing();
          }}
          title={text.discardTitle}
          description={text.discardText(creditDisplayName(credit))}
          actionLabel={t.player.detailsEditor.discard}
          cancelLabel={t.player.detailsEditor.keepEditing}
          onConfirm={() => leaving.leave()}
        />
      </>
    );
  }

  const status =
    state.kind === "failed" ? (
      <ErrorState
        title={failedTitle(state.reason)}
        retryLabel={t.common.retry}
        onRetry={reload}
      />
    ) : state.kind === "gone" ? (
      gone
    ) : undefined;

  return (
    <FormPage
      title={text.loadingTitle}
      actions={
        <EditorBand
          saving={false}
          saveDisabled
          onCancel={() => leaving.leave()}
          onSave={() => undefined}
        />
      }
      asideLabel={asideLabel}
      aside={aside(undefined, () => leaving.leave())}
      panels={loadingPanels}
      status={status}
    />
  );
}

/** A loading panel: each field's label over a skeleton input. */
export function loadingPanel(
  key: string,
  legend: UiText,
  labels: readonly UiText[],
): FormPagePanel {
  return {
    key,
    legend,
    content: labels.map((label) => (
      <Field key={label}>
        <FieldTitle>{label}</FieldTitle>
        <Skeleton className="h-8 w-full" />
      </Field>
    )),
  };
}

/** The read record's form page: the band with Save, a failure line under it, the aside and panels. */
export function CreditFormPage({
  name,
  saving,
  failure,
  onLeave,
  onSave,
  asideLabel,
  aside,
  panels,
}: {
  name: string;
  saving: boolean;
  failure: SaveFailure | null;
  onLeave: () => void;
  onSave: () => void;
  asideLabel: string;
  aside: ReactNode;
  panels: readonly FormPagePanel[];
}) {
  return (
    <FormPage
      title={text.title(name)}
      actions={
        <EditorBand
          saving={saving}
          saveDisabled={false}
          onCancel={onLeave}
          onSave={onSave}
        />
      }
      notice={
        failure === null ? undefined : <FailureLine>{failureText(failure)}</FailureLine>
      }
      asideLabel={asideLabel}
      aside={aside}
      panels={panels}
    />
  );
}
