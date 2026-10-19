import { useState } from "react";
import { useNavigate, useParams } from "react-router";

import { type Credit, deleteCredit, isCreditGone, type Person } from "../../api/credits";
import { errorText, t, type UiText } from "../../i18n";
import { ErrorState } from "../../ui/patterns/error-state";
import { LoadingState } from "../../ui/patterns/loading-state";
import { PageHeader } from "../../ui/patterns/page-header";
import { ProfilePage } from "../../ui/patterns/profile-page";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { useToast } from "../../ui/Toast";
import { useDocumentTitle } from "../../videoList/pageHooks";
import DeleteCreditDialog from "../DeleteCreditDialog";
import MergeCreditDialog from "../MergeCreditDialog";
import PersonFaceControl from "./PersonFaceControl";
import { personPath } from "./personPath";
import PersonGone from "./PersonGone";
import PersonFetchEntry from "./pluginFetch/PersonFetchEntry";
import { useHandedPerson } from "./personHandoff";
import { PersonDetails, PersonIdentity } from "./PersonProfile";
import {
  PersonVideosBody,
  PersonVideosHeader,
  RoleFilter,
  usePersonVideos,
} from "./PersonVideos";
import { useLocalToday } from "./useLocalToday";
import { parsePersonId, usePerson } from "./usePerson";

const text = t.credits.personPage;

/** `/people/:id`: the person's face and profile beside their videos. */
export default function PersonPage() {
  const { id } = useParams();
  const personId = parsePersonId(id);
  // A new id (after a merge) starts from a fresh read.
  return <PersonScreen key={personId ?? "none"} id={personId} />;
}

function PersonScreen({ id }: { id: number | null }) {
  const { state, reload, replace, markGone } = usePerson(id, useHandedPerson(id));
  const person = state.kind === "ready" ? state.person : undefined;
  useDocumentTitle(person?.name);

  if (state.kind === "gone") {
    return (
      <ProfilePage
        profileLabel={text.profile}
        face={null}
        identity={null}
        header={null}
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
        header={<PageHeader title={text.videos} headingLevel={2} />}
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
      <Skeleton className="h-8 w-full" />
    </>
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
  const { role, setRole, videos } = usePersonVideos(person.id);
  const today = useLocalToday();
  const [merging, setMerging] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<UiText | null>(null);

  function merged(target: Credit) {
    setMerging(false);
    toast(text.mergedInto(target.name));
    void navigate(personPath(target.id));
  }

  async function performDelete() {
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteCredit("person", person.id);
      toast(t.credits.deleted(person.name));
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
          fetchEntry={
            <PersonFetchEntry person={person} onApplied={replace} onGone={markGone} />
          }
          onMerge={() => setMerging(true)}
          onDelete={() => {
            setDeleteError(null);
            setDeleting(true);
          }}
        />
      }
      details={<PersonDetails person={person} today={today} />}
      header={<PersonVideosHeader videos={videos} />}
      toolbar={<RoleFilter role={role} onChange={setRole} />}
    >
      <PersonVideosBody videos={videos} role={role} />
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
