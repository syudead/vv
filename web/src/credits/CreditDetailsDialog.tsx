import { useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed } from "../api/client";
import {
  type Credit,
  type CreditKind,
  type CreditPatch,
  type ExternalId,
  isCreditGone,
  type Organization,
  listCredits,
  updateCredit,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { type IdRow, newRowKey } from "../player/detailsEditor/draft";
import {
  controlId,
  type FieldProblem,
  IdRows,
  Problem,
  saveOnEnter,
} from "../player/detailsEditor/fields";
import { DialogError } from "../tags/DialogError";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Combobox, type ComboboxOption } from "../ui/shadcn/combobox";
import { Field, FieldError, FieldLegend, FieldSet } from "../ui/shadcn/field";
import { Spinner } from "../ui/shadcn/spinner";
import { parentOf } from "./creditRows";
import {
  type NameRow,
  NameRows,
  nameRowsControl,
  nameRowsOf,
  nameRowsToSend,
} from "./NameRows";
import {
  type OrganizationDraft,
  organizationDraftOf,
  OrganizationLogoField,
  organizationProfileControl,
  organizationProfilePatch,
  OrganizationProfileFields,
} from "./OrganizationProfileFields";

export const maxExternalIds = 20;

/** Combobox values cannot be empty, so None needs a sentinel. */
const noParent = "none";

/** Map submitted array indexes back to stable row keys for field errors. */
export interface Sent {
  nameKeys: string[];
  idKeys: string[];
}

export function problemOf(failure: unknown, sent: Sent): FieldProblem | null {
  if (!(failure instanceof RequestFailed)) return null;
  const text = errorText(failure);
  if (failure.reason === "organization_parent_cycle" || failure.field === "parentId") {
    return { control: "parent", text };
  }
  const field = failure.field ?? "";
  const profileControl = organizationProfileControl(field);
  if (profileControl !== undefined) return { control: profileControl, text };
  const name = nameRowsControl(field, sent.nameKeys);
  if (name !== undefined) return { control: name, text };
  if (field === "externalIds") return { control: "ids", text };
  const id = /^externalIds\[(\d+)\](?:\.(source|value))?$/.exec(field);
  if (id !== null) {
    const key = sent.idKeys[Number(id[1])];
    if (key !== undefined) return { control: `id-${id[2] ?? "value"}-${key}`, text };
  }
  return null;
}

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

/** Ignore newly added ID rows whose inputs are all blank. */
function blank(...values: string[]): boolean {
  return values.every((value) => value.trim() === "");
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
  if (kind === "organization" && edited.parent)
    patch.parentId = parent === noParent ? null : Number(parent);
  if (organization !== null) {
    const profile = organizationProfilePatch(organization.draft, organization.loaded);
    if (Object.keys(profile).length > 0) patch.organizationProfile = profile;
  }
  return { patch, sent: { nameKeys: sentNames.keys, idKeys: sentIds.keys } };
}

export function idsToSend(rows: readonly IdRow[]): {
  values: ExternalId[];
  keys: string[];
} {
  const kept = rows.filter((row) => !blank(row.source, row.value));
  return {
    values: kept.map((row) => ({ source: row.source.trim(), value: row.value.trim() })),
    keys: kept.map((row) => row.key),
  };
}

export function idRowsOf(ids: readonly ExternalId[]): IdRow[] {
  return ids.map((id) => ({ key: newRowKey("i"), source: id.source, value: id.value }));
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
  const [parent, setParent] = useState(() => {
    const current = parentOf(credit);
    return current === undefined ? noParent : String(current.id);
  });
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
          <ParentField
            credit={credit}
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

/**
 * Load all other organizations as parent candidates; until loaded, show only the current parent
 * and disable selection.
 */
function ParentField({
  credit,
  value,
  onChange,
  disabled,
  problem,
}: {
  credit: Credit;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  problem: FieldProblem | null;
}) {
  const d = t.credits.details;
  const [organizations, setOrganizations] = useState<readonly Credit[] | null>(null);
  const [failure, setFailure] = useState<UiText | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    listCredits("organization", {}, controller.signal).then(
      (page) => {
        if (!controller.signal.aborted) setOrganizations(page.items);
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        setFailure(d.parentsFailed(errorText(error)));
      },
    );
    return () => controller.abort();
  }, [d]);

  const current = parentOf(credit);
  const others =
    organizations ??
    (current === undefined ? [] : [{ id: current.id, name: current.name }]);
  const options: ComboboxOption[] = [
    { value: noParent, label: d.parentNone },
    ...others
      .filter((organization) => organization.id !== credit.id)
      .map((organization) => ({
        value: String(organization.id),
        label: organization.name,
      })),
  ];

  return (
    <Field data-invalid={problem?.control === "parent" || undefined}>
      <div id={controlId("parent")}>
        <Combobox
          options={options}
          value={value}
          onValueChange={onChange}
          placeholder={d.parentNone}
          searchPlaceholder={d.parentSearch}
          emptyText={d.parentEmpty}
          disabled={disabled || organizations === null}
          aria-label={d.parent}
          className="w-full"
        />
      </div>
      <Problem problem={problem} control="parent" />
      {failure !== null && <FieldError>{failure}</FieldError>}
    </Field>
  );
}
