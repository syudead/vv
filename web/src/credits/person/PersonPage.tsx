import { ScanFace } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { getFaceRecognitionSettings } from "../../api/client";
import { type Credit, deleteCredit, isCreditGone, type Person } from "../../api/credits";
import { errorText, t, type UiText } from "../../i18n";
import { ErrorState } from "../../ui/patterns/error-state";
import { LoadingState } from "../../ui/patterns/loading-state";
import { ProfilePage } from "../../ui/patterns/profile-page";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { useToast } from "../../ui/Toast";
import DeleteCreditDialog from "../DeleteCreditDialog";
import { creditDisplayName } from "../displayName";
import MergeCreditDialog from "../MergeCreditDialog";
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

  if (state.kind === "gone") {
    return (
      <ProfilePage
        profileLabel={text.profile}
        face={null}
        identity={null}
        status={<PersonGone />}
      >
        {null}
      </ProfilePage>
    );
  }
  if (person === undefined || id === null) {
    return (
      <ProfilePage
        profileLabel={text.profile}
        face={
          state.kind === "loading" ? (
            <Skeleton className="aspect-portrait w-full" />
          ) : null
        }
        identity={state.kind === "loading" ? <IdentitySkeleton /> : null}
      >
        {state.kind === "failed" ? (
          <ErrorState
            title={text.loadFailed(state.reason)}
            retryLabel={t.common.retry}
            onRetry={reload}
          />
        ) : (
          <LoadingState label={text.loading} layout="grid" size={0} count={6} />
        )}
      </ProfilePage>
    );
  }
  return <PersonShown person={person} replace={replace} markGone={markGone} />;
}

function IdentitySkeleton() {
  return (
    <>
      <Skeleton className="h-6 w-3/4" />
      <Skeleton className="h-4 w-1/2" />
      <Skeleton className="h-10 w-full" />
    </>
  );
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
  const toast = useToast();
  const navigate = useNavigate();
  const today = useLocalToday();
  const [merging, setMerging] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<UiText | null>(null);

  function merged(target: Credit) {
    setMerging(false);
    toast(text.mergedInto(creditDisplayName(target)));
    void navigate(personPath(target.id));
  }

  async function performDelete() {
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteCredit("person", person.id);
      toast(t.credits.deleted(creditDisplayName(person)));
      void navigate("/people");
    } catch (failure) {
      if (isCreditGone(failure)) {
        markGone();
        return;
      }
      setDeleteError(errorText(failure));
      setDeletePending(false);
    }
  }

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
          onMerge={() => setMerging(true)}
          onDelete={() => {
            setDeleteError(null);
            setDeleting(true);
          }}
        />
      }
      facts={personFactsCard(person, today)}
    >
      <PersonShelves personId={person.id} />
      {merging && (
        <MergeCreditDialog
          kind="person"
          source={person}
          onClose={() => setMerging(false)}
          onMerged={merged}
          onStale={markGone}
        />
      )}
      {deleting && (
        <DeleteCreditDialog
          kind="person"
          credit={person}
          pending={deletePending}
          error={deleteError}
          onClose={() => {
            if (!deletePending) setDeleting(false);
          }}
          onDelete={() => void performDelete()}
        />
      )}
    </ProfilePage>
  );
}
