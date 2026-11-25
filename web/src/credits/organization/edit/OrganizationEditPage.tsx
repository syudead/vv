import { useRef, useState } from "react";
import { useParams } from "react-router";

import { type Organization, organizationLogoSrc } from "../../../api/credits";
import { t } from "../../../i18n";
import { BackLink } from "../../../player/detailsEditor/EditorAside";
import {
  IdRows,
  Problem,
  saveOnEnter,
  useEditorForm,
} from "../../../player/detailsEditor/fields";
import { OrganizationLogo } from "../../../ui/OrganizationLogo";
import { Skeleton } from "../../../ui/shadcn/skeleton";
import { maxExternalIds, problemOf } from "../../creditEditFields";
import {
  CreditEditor,
  CreditFormPage,
  type CreditFormProps,
  loadingPanel,
} from "../../edit/CreditEditor";
import { useCreditSave } from "../../edit/useCreditSave";
import { NameRows } from "../../NameRows";
import { OrganizationParentField } from "../../OrganizationParentField";
import { OrganizationProfileFields } from "../../OrganizationProfileFields";
import { parseCreditId } from "../../useCreditRecord";
import { OrganizationGone } from "../OrganizationPage";
import { organizationPath } from "../organizationPath";
import {
  buildOrganizationPatch,
  draftFromOrganization,
  organizationDraftChanged,
  type OrganizationEditDraft,
} from "./organizationDraft";

const text = t.credits.organizationEdit;
const personEdit = t.credits.personEdit;
const details = t.credits.details;

/** `/organizations/:id/edit`, keyed by id so a draft never saves to another organization. */
export default function OrganizationEditPage() {
  const id = parseCreditId(useParams().id);
  return <OrganizationEditor key={id ?? "none"} id={id} />;
}

function OrganizationEditor({ id }: { id: number | null }) {
  const backTo = id === null ? "/organizations" : organizationPath(id);
  return (
    <CreditEditor<Organization>
      kind="organization"
      id={id}
      backTo={backTo}
      failedTitle={t.credits.organizationPage.loadFailed}
      gone={<OrganizationGone />}
      asideLabel={t.credits.organizationPage.profile}
      aside={(organization, onBack) => (
        <OrganizationEditAside
          organization={organization}
          backTo={backTo}
          onBack={onBack}
        />
      )}
      loadingPanels={[
        loadingPanel("profile", personEdit.profile, [
          details.foundedDate,
          details.dissolvedDate,
          details.location,
          details.bio,
          details.links,
        ]),
        loadingPanel("names", text.namesAndParent, [
          t.credits.names.title,
          personEdit.externalIds,
          details.parent,
        ]),
      ]}
      form={(props) => <OrganizationForm {...props} />}
    />
  );
}

/** The organization being edited, read only; the logo is changed on the organization page. */
function OrganizationEditAside({
  organization,
  backTo,
  onBack,
}: {
  organization: Organization | undefined;
  backTo: string;
  onBack: () => void;
}) {
  return (
    <>
      <div className="w-1/3 md:w-1/4 lg:w-full">
        {organization === undefined ? (
          <Skeleton className="aspect-square w-full rounded-md" />
        ) : (
          <OrganizationLogo
            shape="profile"
            src={organizationLogoSrc(organization)}
            alt={details.logoAlt(organization.name)}
          />
        )}
      </div>
      {organization === undefined ? (
        <>
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-4 w-1/2" />
        </>
      ) : (
        <>
          <p className="font-medium wrap-anywhere">{organization.name}</p>
          <p className="text-muted-foreground tabular-nums">
            {t.credits.row.videoCount(organization.videoCount)}
          </p>
        </>
      )}
      <BackLink to={backTo} label={text.back} onBack={onBack} />
    </>
  );
}

function OrganizationForm({
  credit: organization,
  id,
  onLeave,
  onSaved,
  onGone,
  onChangedChange,
  onSavingChange,
}: CreditFormProps<Organization>) {
  const [loaded] = useState(() => draftFromOrganization(organization));
  const [draft, setDraft] = useState<OrganizationEditDraft>(loaded);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const { saving, failure, problem, save } = useCreditSave({
    kind: "organization",
    id,
    credit: organization,
    loaded,
    draftRef,
    build: buildOrganizationPatch,
    place: problemOf,
    onSaved,
    onGone,
  });
  const nameRef = useEditorForm({
    changed: organizationDraftChanged(draft, loaded),
    saving,
    problem,
    onChangedChange,
    onSavingChange,
  });
  const update = (change: Partial<OrganizationEditDraft>) =>
    setDraft((current) => ({ ...current, ...change }));
  const onEnter = saveOnEnter(save);
  const rows = { saving, problem, onEnter };

  return (
    <CreditFormPage
      name={organization.name}
      saving={saving}
      failure={failure}
      onLeave={onLeave}
      onSave={save}
      asideLabel={t.credits.organizationPage.profile}
      aside={
        <OrganizationEditAside
          organization={organization}
          backTo={organizationPath(id)}
          onBack={onLeave}
        />
      }
      panels={[
        {
          key: "profile",
          legend: personEdit.profile,
          content: (
            <OrganizationProfileFields
              draft={draft.profile}
              onChange={(profile) => update({ profile })}
              {...rows}
            />
          ),
        },
        {
          key: "names",
          legend: text.namesAndParent,
          content: (
            <>
              <NameRows
                rows={draft.names}
                onChange={(names) => update({ names })}
                title={t.credits.names.title}
                firstRef={nameRef}
                {...rows}
              />
              <div className="flex flex-col gap-1">
                <IdRows
                  rows={draft.externalIds}
                  onChange={(externalIds) => update({ externalIds })}
                  title={personEdit.externalIds}
                  max={maxExternalIds}
                  {...rows}
                />
                <Problem problem={problem} control="ids" />
              </div>
              <OrganizationParentField
                organization={organization}
                value={draft.parent}
                onChange={(parent) => update({ parent })}
                disabled={saving}
                problem={problem}
                title={details.parent}
              />
            </>
          ),
        },
      ]}
    />
  );
}
