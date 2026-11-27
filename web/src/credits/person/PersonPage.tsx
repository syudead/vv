import { ScanFace } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";

import { useAssistantScreen } from "../../assistant/AssistantProvider";
import { getFaceRecognitionSettings } from "../../api/client";
import type { Person } from "../../api/credits";
import { t } from "../../i18n";
import { ProfilePage } from "../../ui/patterns/profile-page";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { CreditPageStatus } from "../CreditPageStatus";
import { creditDisplayName } from "../displayName";
import { useCreditMergeDelete } from "../useCreditMergeDelete";
import { useFaceCandidates } from "./PersonCandidates";
import PersonFaceControl from "./PersonFaceControl";
import { personFacesPath, personPath } from "./personPath";
import PersonGone from "./PersonGone";
import PersonFetchEntry from "./pluginFetch/PersonFetchEntry";
import { personFactsCard, PersonIdentity } from "./PersonProfile";
import { PersonShelves } from "./PersonVideos";
import { useLocalToday } from "./useLocalToday";
import { parsePersonId, useRoutedPerson } from "./usePerson";

const text = t.credits.personPage;

/** `/people/:id`: the person's face and profile above their videos, one shelf per role. */
export default function PersonPage() {
  const { id } = useParams();
  const personId = parsePersonId(id);
  // A new id (after a merge) starts from a fresh read.
  return <PersonScreen key={personId ?? "none"} id={personId} />;
}

function PersonScreen({ id }: { id: number | null }) {
  const { state, person, reload, replace, markGone } = useRoutedPerson(
    id,
    creditDisplayName,
  );
  useAssistantScreen({
    name: person === undefined ? undefined : creditDisplayName(person),
  });

  if (state.kind !== "ready") {
    return (
      <CreditPageStatus
        state={state}
        profileLabel={text.profile}
        faceClassName="aspect-portrait w-full"
        failedTitle={text.loadFailed}
        gone={<PersonGone />}
        onRetry={reload}
      />
    );
  }
  return <PersonShown person={state.credit} replace={replace} markGone={markGone} />;
}

/** Whether face recognition is on; off, unknown or unreadable all hide its entry. */
function useFaceRecognitionOn(): boolean {
  const [on, setOn] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    getFaceRecognitionSettings(controller.signal).then(
      (settings) => {
        if (!controller.signal.aborted) setOn(settings.enabled);
      },
      () => undefined,
    );
    return () => controller.abort();
  }, []);
  return on;
}

/** The way to the person's face recognition, shown only while it is on. */
function FacesEntry({ personId, onGone }: { personId: number; onGone: () => void }) {
  return useFaceRecognitionOn() ? (
    <FacesLink personId={personId} onGone={onGone} />
  ) : null;
}

/** Faces, with the pairs waiting for a decision. */
function FacesLink({ personId, onGone }: { personId: number; onGone: () => void }) {
  const toReview = useFaceCandidates(personId, onGone).items.length;
  return (
    <Button asChild variant="secondary" size="sm">
      <Link
        to={personFacesPath(personId)}
        aria-label={text.facesPage.entryLabel(toReview)}
      >
        <ScanFace aria-hidden="true" />
        {text.facesPage.entry}
        {toReview > 0 && <Badge>{text.facesPage.toReview(toReview)}</Badge>}
      </Link>
    </Button>
  );
}

function PersonShown({
  person,
  replace,
  markGone,
}: {
  person: Person;
  replace: (next: Person | ((current: Person) => Person)) => void;
  markGone: () => void;
}) {
  const today = useLocalToday();
  const { openMerge, openDelete, dialogs } = useCreditMergeDelete({
    kind: "person",
    credit: person,
    pageOf: personPath,
    listPath: "/people",
    markGone,
  });

  return (
    <ProfilePage
      profileLabel={text.profile}
      face={<PersonFaceControl person={person} onChanged={replace} onGone={markGone} />}
      identity={
        <PersonIdentity
          person={person}
          today={today}
          fetchEntry={
            <PersonFetchEntry person={person} onApplied={replace} onGone={markGone} />
          }
          facesEntry={<FacesEntry personId={person.id} onGone={markGone} />}
          onMerge={openMerge}
          onDelete={openDelete}
        />
      }
      facts={personFactsCard(person, today)}
    >
      <PersonShelves personId={person.id} />
      {dialogs}
    </ProfilePage>
  );
}
