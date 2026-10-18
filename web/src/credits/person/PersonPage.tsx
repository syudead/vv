import { UserRoundX } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { type Credit, deleteCredit, isCreditGone, type Person } from "../../api/credits";
import { errorText, t, type UiText } from "../../i18n";
import { EmptyState } from "../../ui/patterns/empty-state";
import { ErrorState } from "../../ui/patterns/error-state";
import { LoadingState } from "../../ui/patterns/loading-state";
import { PageHeader } from "../../ui/patterns/page-header";
import { ProfilePage } from "../../ui/patterns/profile-page";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { useToast } from "../../ui/Toast";
import { useDocumentTitle } from "../../videoList/pageHooks";
import DeleteCreditDialog from "../DeleteCreditDialog";
import MergeCreditDialog from "../MergeCreditDialog";
import PersonFaceControl from "./PersonFaceControl";
import { PersonDetails, PersonIdentity } from "./PersonProfile";
import {
  PersonVideosBody,
  PersonVideosHeader,
  RoleFilter,
  usePersonVideos,
} from "./PersonVideos";
import { useLocalToday } from "./useLocalToday";
import { usePerson } from "./usePerson";

const text = t.credits.personPage;

function parseId(value: string | undefined): number | null {
  if (value === undefined || !/^[1-9]\d*$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : null;
}

/** `/people/:id`: the person's face and profile beside their videos. */
export default function PersonPage() {
  const { id } = useParams();
  const personId = parseId(id);
  // A new id (after a merge) starts from a fresh read.
  return <PersonScreen key={personId ?? "none"} id={personId} />;
}

function PersonScreen({ id }: { id: number | null }) {
  const { state, reload, replace, markGone } = usePerson(id);
  const person = state.kind === "ready" ? state.person : undefined;
  useDocumentTitle(person?.name);

  if (state.kind === "gone") {
    return (
      <ProfilePage
        profileLabel={text.profile}
        face={null}
        identity={null}
        header={null}
        status={
          <EmptyState
            icon={<UserRoundX aria-hidden="true" />}
            title={text.gone}
            action={
              <Button asChild variant="outline" size="sm">
                <Link to="/people">{text.backToPeople}</Link>
              </Button>
            }
          />
        }
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
    void navigate(`/people/${String(target.id)}`);
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
