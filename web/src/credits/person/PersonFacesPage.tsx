import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router";

import type { Credit, Person } from "../../api/credits";
import {
  confirmFaceCandidate,
  type FaceAppearance,
  type FaceCandidatePerson,
  splitFaceAppearance,
} from "../../api/personFaces";
import { t } from "../../i18n";
import { ErrorState } from "../../ui/patterns/error-state";
import { LoadingState } from "../../ui/patterns/loading-state";
import { PageSection } from "../../ui/patterns/page-section";
import { RecordPage } from "../../ui/patterns/record-page";
import { Button } from "../../ui/shadcn/button";
import { useToast } from "../../ui/Toast";
import { creditDisplayName } from "../displayName";
import { PersonAppearances, usePersonAppearances } from "./PersonAppearances";
import { PersonCandidates, useFaceCandidates } from "./PersonCandidates";
import PersonGone from "./PersonGone";
import { personFacesPath, personPath } from "./personPath";
import { parsePersonId, useRoutedPerson } from "./usePerson";

const text = t.credits.personPage;

/** `/people/:id/faces`: the person's face recognition, kept off the person page. */
export default function PersonFacesPage() {
  const id = parsePersonId(useParams().id);
  // A new id (after a merge) starts from a fresh read.
  return <FacesScreen key={id ?? "none"} id={id} />;
}

function FacesScreen({ id }: { id: number | null }) {
  const { state, person, reload, replace, markGone } = useRoutedPerson(
    id,
    (shown) => `${text.facesPage.title} · ${creditDisplayName(shown)}`,
  );
  const back = (
    <Button asChild variant="ghost" size="sm">
      <Link to={id === null ? "/people" : personPath(id)}>
        <ArrowLeft aria-hidden="true" />
        {person === undefined
          ? text.backToPeople
          : text.facesPage.back(creditDisplayName(person))}
      </Link>
    </Button>
  );

  if (person !== undefined) {
    return (
      <FacesShown person={person} back={back} replace={replace} markGone={markGone} />
    );
  }
  return (
    <RecordPage back={back} title={text.facesPage.title} titleHidden>
      {state.kind === "gone" ? (
        <PersonGone />
      ) : state.kind === "failed" ? (
        <ErrorState
          title={text.loadFailed(state.reason)}
          retryLabel={t.common.retry}
          onRetry={reload}
        />
      ) : (
        <LoadingState label={text.loading} layout="cards" count={3} />
      )}
    </RecordPage>
  );
}

function FacesShown({
  person,
  back,
  replace,
  markGone,
}: {
  person: Person;
  back: ReactNode;
  replace: (next: Person) => void;
  markGone: () => void;
}) {
  const toast = useToast();
  const navigate = useNavigate();
  const candidates = useFaceCandidates(person.id, markGone);
  const appearances = usePersonAppearances(person.id, markGone);

  function merged(target: Credit) {
    toast(text.mergedInto(creditDisplayName(target)));
    void navigate(personFacesPath(target.id));
  }

  /** The shown person was merged away when it is not the target; otherwise it gained the other's faces. */
  async function confirmCandidate(other: FaceCandidatePerson) {
    const target = await confirmFaceCandidate(person.id, other.id);
    if (target.id !== person.id) {
      merged(target);
      return;
    }
    replace(target);
    candidates.remove(other.id);
    // The target may have gained the source's pairs and appearances.
    candidates.reload();
    appearances.reload();
    toast(text.candidates.merged(creditDisplayName(other)));
  }

  /** The face leaves this person's list; the person's candidates may have changed. */
  async function splitAppearance(appearance: FaceAppearance) {
    const created = await splitFaceAppearance(person.id, appearance.appearanceId);
    appearances.remove(appearance.appearanceId);
    candidates.reload();
    toast(text.faces.split, {
      action: {
        label: text.faces.openSplit,
        run: () => {
          void navigate(personPath(created.id));
          return Promise.resolve();
        },
      },
    });
  }

  return (
    <RecordPage
      back={back}
      title={text.facesPage.title}
      description={creditDisplayName(person)}
    >
      {candidates.items.length > 0 && (
        <PageSection
          title={text.candidates.heading}
          description={text.facesPage.candidatesText}
        >
          <PersonCandidates
            personId={person.id}
            items={candidates.items}
            actions={{
              confirm: confirmCandidate,
              remove: candidates.remove,
              onGone: markGone,
            }}
          />
        </PageSection>
      )}
      <PageSection title={text.faces.heading}>
        <PersonAppearances
          appearances={appearances}
          actions={{ split: splitAppearance, onGone: markGone }}
        />
      </PageSection>
    </RecordPage>
  );
}
