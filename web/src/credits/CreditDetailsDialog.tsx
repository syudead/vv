import { useRef, useState } from "react";

import {
  type Credit,
  type CreditKind,
  type CreditPatch,
  isCreditGone,
  type Organization,
  updateCredit,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import type { IdRow } from "../player/detailsEditor/draft";
import {
  type FieldProblem,
  IdRows,
  Problem,
  saveOnEnter,
} from "../player/detailsEditor/fields";
import { DialogError } from "../tags/DialogError";
import { FormDialog } from "../ui/patterns/form-dialog";
import { FieldLegend, FieldSet } from "../ui/shadcn/field";
import { Spinner } from "../ui/shadcn/spinner";
import {
  idRowsOf,
  idsToSend,
  maxExternalIds,
  problemOf,
  type Sent,
} from "./creditEditFields";
import { type NameRow, NameRows, nameRowsOf, nameRowsToSend } from "./NameRows";
import {
  OrganizationParentField,
  parentIdOf,
  parentValueOf,
} from "./OrganizationParentField";
import {
  type OrganizationDraft,
  organizationDraftOf,
  OrganizationLogoField,
  organizationProfilePatch,
  OrganizationProfileFields,
} from "./OrganizationProfileFields";

/** Submit only fields edited since opening the dialog. */
interface Edited {
  names: boolean;
  ids: boolean;
  parent: boolean;
}

/** The organization-only state the patch compares; people have none. */
interface OrganizationEdit {
  draft: OrganizationDraft;
  loaded: OrganizationDraft;
}

/**
 * Patch only edited fields to preserve concurrent changes in other tabs; edited lists replace
 * their entire value. Names go as NameRows sends them, blank ID rows are left out, and only
 * organizations send parent, with null for None.
 */
function buildPatch(
  kind: CreditKind,
  names: readonly NameRow[],
  ids: readonly IdRow[],
  parent: string,
  edited: Edited,
  organization: OrganizationEdit | null,
): { patch: CreditPatch; sent: Sent } {
  const sentNames = nameRowsToSend(edited.names ? names : []);
  const sentIds = idsToSend(edited.ids ? ids : []);
  const patch: CreditPatch = {};
  if (edited.names) patch.names = sentNames.values;
  if (edited.ids) patch.externalIds = sentIds.values;
  if (kind === "organization" && edited.parent) patch.parentId = parentIdOf(parent);
  if (organization !== null) {
    const profile = organizationProfilePatch(organization.draft, organization.loaded);
    if (Object.keys(profile).length > 0) patch.organizationProfile = profile;
  }
  return { patch, sent: { nameKeys: sentNames.keys, idKeys: sentIds.keys } };
}

/** Keep field errors in the open dialog; missing records call onStale. */
export default function CreditDetailsDialog({
  kind,
  credit,
  onClose,
  onSaved,
  onStale,
  onLogoChanged,
}: {
  kind: CreditKind;
  credit: Credit;
  onClose: () => void;
  onSaved: (updated: Credit) => void;
  onStale: () => void;
  /** A logo saved at once, before or without Save. */
  onLogoChanged?: (updated: Credit) => void;
}) {
  const d = t.credits.details;
  const organization = kind === "organization" ? (credit as Organization) : null;
  const [organizationDraft, setOrganizationDraft] = useState<OrganizationDraft | null>(
    () => (organization === null ? null : organizationDraftOf(organization.profile)),
  );
  const [loadedOrganization] = useState(organizationDraft);
  const [names, setNames] = useState<NameRow[]>(() => nameRowsOf(credit.names));
  const [ids, setIds] = useState<IdRow[]>(() => idRowsOf(credit.externalIds));
  const [parent, setParent] = useState(() => parentValueOf(credit as Organization));
  const [edited, setEdited] = useState<Edited>({
    names: false,
    ids: false,
    parent: false,
  });
  const [pending, setPending] = useState(false);
  const [problem, setProblem] = useState<FieldProblem | null>(null);
  const [error, setError] = useState<UiText | null>(null);
  const submitRef = useRef<HTMLButtonElement>(null);

  function save() {
    void submit();
  }

  async function submit() {
    if (pending) return;
    const { patch, sent } = buildPatch(
      kind,
      names,
      ids,
      parent,
      edited,
      organizationDraft === null || loadedOrganization === null
        ? null
        : { draft: organizationDraft, loaded: loadedOrganization },
    );
    setPending(true);
    setProblem(null);
    setError(null);
    try {
      onSaved(await updateCredit(kind, credit.id, patch));
    } catch (failure) {
      if (isCreditGone(failure)) {
        onStale();
        return;
      }
      const placed = problemOf(failure, sent);
      if (placed === null) setError(errorText(failure));
      else setProblem(placed);
      setPending(false);
    }
  }

  const onEnter = saveOnEnter(save);
  const rowsProps = { saving: pending, problem, onEnter };
  const markEdited = (field: keyof Edited) =>
    setEdited((current) => (current[field] ? current : { ...current, [field]: true }));

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={d.title(credit.name)}
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? d.submitting : d.submit}
        </>
      }
      pending={pending}
      submitRef={submitRef}
    >
      <FieldSet>
        <FieldLegend>{t.credits.names.title}</FieldLegend>
        <NameRows
          rows={names}
          onChange={(next) => {
            setNames(next);
            markEdited("names");
          }}
          {...rowsProps}
        />
      </FieldSet>
      <FieldSet>
        <FieldLegend>{d.externalIds}</FieldLegend>
        <IdRows
          rows={ids}
          onChange={(next) => {
            setIds(next);
            markEdited("ids");
          }}
          title={null}
          max={maxExternalIds}
          {...rowsProps}
        />
        <Problem problem={problem} control="ids" />
      </FieldSet>
      {kind === "organization" && (
        <FieldSet>
          <FieldLegend>{d.parent}</FieldLegend>
          <OrganizationParentField
            organization={credit as Organization}
            value={parent}
            onChange={(next) => {
              setParent(next);
              markEdited("parent");
              if (problem?.control === "parent") setProblem(null);
            }}
            disabled={pending}
            problem={problem}
          />
        </FieldSet>
      )}
      {organization !== null && organizationDraft !== null && (
        <>
          <FieldSet>
            <FieldLegend>{d.logo}</FieldLegend>
            <OrganizationLogoField
              organization={organization}
              disabled={pending}
              onChanged={(updated) => onLogoChanged?.(updated)}
              onStale={onStale}
            />
          </FieldSet>
          <FieldSet>
            <FieldLegend>{d.profile}</FieldLegend>
            <OrganizationProfileFields
              draft={organizationDraft}
              onChange={(next) => {
                setOrganizationDraft(next);
                if (problem?.control.startsWith("organization-") === true)
                  setProblem(null);
              }}
              saving={pending}
              problem={problem}
              onEnter={onEnter}
            />
          </FieldSet>
        </>
      )}
      {error !== null && <DialogError message={error} />}
    </FormDialog>
  );
}
