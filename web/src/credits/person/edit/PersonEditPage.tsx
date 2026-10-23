import { useRef, useState } from "react";
import { useParams } from "react-router";

import type { Person } from "../../../api/credits";
import { t, type UiText } from "../../../i18n";
import {
  EditorBand,
  FailureLine,
  failureText,
} from "../../../player/detailsEditor/EditorBand";
import { useEditorForm } from "../../../player/detailsEditor/fields";
import { useLeaving } from "../../../player/detailsEditor/useLeaving";
import { ConfirmDialog } from "../../../ui/patterns/confirm-dialog";
import { ErrorState } from "../../../ui/patterns/error-state";
import { FormPage } from "../../../ui/patterns/form-page";
import { Field, FieldTitle } from "../../../ui/shadcn/field";
import { Skeleton } from "../../../ui/shadcn/skeleton";
import { useDocumentTitle } from "../../../videoList/pageHooks";
import { creditDisplayName } from "../../displayName";
import PersonGone from "../PersonGone";
import { handPerson, useHandedPerson } from "../personHandoff";
import { personPath } from "../personPath";
import { parsePersonId, usePerson } from "../usePerson";
import NamesPanel from "./NamesPanel";
import PersonEditAside from "./PersonEditAside";
import { draftChanged, draftFromPerson, type PersonDraft } from "./personDraft";
import ProfilePanel from "./ProfilePanel";
import { usePersonSave } from "./usePersonSave";

const text = t.credits.personEdit;

/** `/people/:id/edit`, keyed by id so a draft never saves to another person. */
export default function PersonEditPage() {
  const id = parsePersonId(useParams().id);
  return <PersonEditor key={id ?? "none"} id={id} />;
}

function PersonEditor({ id }: { id: number | null }) {
  const { state, reload, markGone } = usePerson(id, useHandedPerson(id));
  const [changed, setChanged] = useState(false);
  const [saving, setSaving] = useState(false);
  const backTo = id === null ? "/people" : personPath(id);
  const leaving = useLeaving<Person>({
    pagePath: backTo,
    changed: state.kind === "ready" && changed,
    saving,
    handBack: (saved) => {
      if (saved !== undefined) handPerson(saved);
    },
  });
  const person = state.kind === "ready" ? state.person : undefined;
  useDocumentTitle(
    person === undefined ? undefined : text.title(creditDisplayName(person)),
  );

  if (person !== undefined) {
    return (
      <>
        <PersonForm
          person={person}
          backTo={backTo}
          onLeave={leaving.requestLeave}
          onSaved={(saved) => leaving.leave(saved)}
          onGone={() => {
            setChanged(false);
            markGone();
          }}
          onChangedChange={setChanged}
          onSavingChange={setSaving}
        />
        <ConfirmDialog
          open={leaving.confirmOpen}
          onOpenChange={(open) => {
            if (!open) leaving.keepEditing();
          }}
          title={text.discardTitle}
          description={text.discardText(creditDisplayName(person))}
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
        title={t.credits.personPage.loadFailed(state.reason)}
        retryLabel={t.common.retry}
        onRetry={reload}
      />
    ) : state.kind === "gone" ? (
      <PersonGone />
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
      asideLabel={t.credits.personPage.profile}
      aside={
        <PersonEditAside
          person={undefined}
          backTo={backTo}
          onBack={() => leaving.leave()}
        />
      }
      panels={loadingPanels()}
      status={status}
    />
  );
}

function loadingPanels() {
  const field = (label: UiText) => (
    <Field key={label}>
      <FieldTitle>{label}</FieldTitle>
      <Skeleton className="h-8 w-full" />
    </Field>
  );
  return [
    {
      key: "profile",
      legend: text.profile,
      content: [
        text.gender,
        text.birthDate,
        text.birthplace,
        text.heightCm,
        text.bio,
      ].map(field),
    },
    {
      key: "names",
      legend: text.namesAndLinks,
      content: [
        text.name,
        text.aliases,
        text.externalIds,
        t.player.detailsEditor.links,
      ].map(field),
    },
  ];
}

function PersonForm({
  person,
  backTo,
  onLeave,
  onSaved,
  onGone,
  onChangedChange,
  onSavingChange,
}: {
  person: Person;
  backTo: string;
  onLeave: () => void;
  onSaved: (person: Person) => void;
  onGone: () => void;
  onChangedChange: (changed: boolean) => void;
  onSavingChange: (saving: boolean) => void;
}) {
  const [loaded] = useState(() => draftFromPerson(person));
  const [draft, setDraft] = useState<PersonDraft>(loaded);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const { saving, failure, problem, save } = usePersonSave({
    person,
    loaded,
    draftRef,
    onSaved,
    onGone,
  });
  const nameRef = useEditorForm({
    changed: draftChanged(draft, loaded),
    saving,
    problem,
    onChangedChange,
    onSavingChange,
  });

  const panel = {
    draft,
    update: (change: (current: PersonDraft) => PersonDraft) =>
      setDraft((current) => change(current)),
    saving,
    problem,
    save,
  };

  return (
    <FormPage
      title={text.title(creditDisplayName(person))}
      actions={
        <EditorBand
          saving={saving}
          saveDisabled={false}
          onCancel={onLeave}
          onSave={save}
        />
      }
      notice={
        failure === null ? undefined : <FailureLine>{failureText(failure)}</FailureLine>
      }
      asideLabel={t.credits.personPage.profile}
      aside={<PersonEditAside person={person} backTo={backTo} onBack={onLeave} />}
      panels={[
        { key: "profile", legend: text.profile, content: <ProfilePanel {...panel} /> },
        {
          key: "names",
          legend: text.namesAndLinks,
          content: (
            <NamesPanel
              {...panel}
              nameRef={nameRef}
              namePlaceholder={person.provisional ? creditDisplayName(person) : undefined}
            />
          ),
        },
      ]}
    />
  );
}
