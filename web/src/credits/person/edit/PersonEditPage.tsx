import { useRef, useState } from "react";
import { useParams } from "react-router";

import type { Person } from "../../../api/credits";
import { t } from "../../../i18n";
import { useEditorForm } from "../../../player/detailsEditor/fields";
import { creditDisplayName } from "../../displayName";
import {
  CreditEditor,
  CreditFormPage,
  type CreditFormProps,
  loadingPanel,
} from "../../edit/CreditEditor";
import PersonGone from "../PersonGone";
import { handPerson, useHandedPerson } from "../personHandoff";
import { personPath } from "../personPath";
import { parsePersonId } from "../usePerson";
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
  return (
    <CreditEditor<Person>
      kind="person"
      id={id}
      handed={useHandedPerson(id)}
      backTo={id === null ? "/people" : personPath(id)}
      handBack={handPerson}
      failedTitle={t.credits.personPage.loadFailed}
      gone={<PersonGone />}
      asideLabel={t.credits.personPage.profile}
      aside={(person, onBack) => (
        <PersonEditAside
          person={person}
          backTo={id === null ? "/people" : personPath(id)}
          onBack={onBack}
        />
      )}
      loadingPanels={[
        loadingPanel("profile", text.profile, [
          text.gender,
          text.birthDate,
          text.birthplace,
          text.heightCm,
          text.bio,
        ]),
        loadingPanel("names", text.namesAndLinks, [
          t.credits.names.title,
          text.externalIds,
          t.player.detailsEditor.links,
        ]),
      ]}
      form={(props) => <PersonForm {...props} />}
    />
  );
}

function PersonForm({
  credit: person,
  id,
  onLeave,
  onSaved,
  onGone,
  onChangedChange,
  onSavingChange,
}: CreditFormProps<Person>) {
  const [loaded] = useState(() => draftFromPerson(person));
  const [draft, setDraft] = useState<PersonDraft>(loaded);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const { saving, failure, problem, save } = usePersonSave({
    id,
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
    <CreditFormPage
      name={creditDisplayName(person)}
      saving={saving}
      failure={failure}
      onLeave={onLeave}
      onSave={save}
      asideLabel={t.credits.personPage.profile}
      aside={<PersonEditAside person={person} backTo={personPath(id)} onBack={onLeave} />}
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
