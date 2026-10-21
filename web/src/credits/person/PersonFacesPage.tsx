import { ArrowLeft, RefreshCw } from "lucide-react";
import { type ReactNode, useRef, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";

import { RequestFailed } from "../../api/client";
import {
  getCredit,
  mergeCredits,
  type Person,
  removePersonImage,
} from "../../api/credits";
import { undoActivity } from "../../api/activity";
import {
  confirmFaceCandidate,
  type FaceCandidatePerson,
  reopenFaceCandidate,
  splitFaceAppearance,
  splitFaceAppearances,
} from "../../api/personFaces";
import { errorText, formatNumber, t } from "../../i18n";
import { ErrorState } from "../../ui/patterns/error-state";
import { LoadingState } from "../../ui/patterns/loading-state";
import { PageSection } from "../../ui/patterns/page-section";
import { RecordPage } from "../../ui/patterns/record-page";
import { Button } from "../../ui/shadcn/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../../ui/shadcn/tabs";
import { useToast } from "../../ui/Toast";
import { creditDisplayName } from "../displayName";
import { PersonAppearances, usePersonAppearances } from "./PersonAppearances";
import { PersonCandidates, useFaceCandidates, useFaceSample } from "./PersonCandidates";
import PersonGone from "./PersonGone";
import { personFacesPath, personPath } from "./personPath";
import { parsePersonId, useRoutedPerson } from "./usePerson";

const text = t.credits.personPage;

type FacesTab = "candidates" | "faces";

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

/**
 * The tab in the address, so a reload keeps it; without one, the pairs unless none waited at the first read. The
 * default is settled once, so a Refresh that changes the count never moves the owner to the other tab.
 */
function useFacesTab(
  loaded: boolean,
  noCandidates: boolean,
): [FacesTab, (next: FacesTab) => void] {
  const [params, setParams] = useSearchParams();
  const [initial, setInitial] = useState<FacesTab | null>(null);
  if (initial === null && loaded) setInitial(noCandidates ? "faces" : "candidates");
  const asked = params.get("tab");
  const tab: FacesTab =
    asked === "faces" || asked === "candidates" ? asked : (initial ?? "candidates");
  return [
    tab,
    (next) => {
      setParams({ tab: next }, { replace: true });
    },
  ];
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
  // Merges already undone, so a retry after a failed reopen only reopens the pair.
  const undone = useRef(new Set<number>());

  async function undoConfirmOf(personId: number, otherId: number, activityId: number) {
    if (!undone.current.has(activityId)) {
      await undoActivity(activityId);
      undone.current.add(activityId);
    }
    await reopenFaceCandidate(personId, otherId);
  }
  // Each refresh remounts the lists, so decided rows and split tiles start over from the server.
  const [round, setRound] = useState(0);
  const candidates = useFaceCandidates(person.id, markGone);
  const appearances = usePersonAppearances(person.id, markGone);
  const sample = useFaceSample(person.id, round);
  const [tab, setTab] = useFacesTab(candidates.loaded, candidates.items.length === 0);

  function refresh() {
    candidates.reload();
    appearances.reload();
    setRound((value) => value + 1);
  }

  /** The shown person stays unless they have no name and the other has; then the page follows the merge. */
  async function confirmCandidate(other: FaceCandidatePerson) {
    const { person: target, undoActivityId } = await confirmFaceCandidate(
      person.id,
      other.id,
    );
    if (target.id !== person.id) {
      // The shown person is merged away, so their Undo travels with the toast to the named person's page.
      toast(
        text.mergedInto(creditDisplayName(target)),
        undoActivityId === undefined
          ? undefined
          : {
              action: {
                label: text.candidates.undo,
                run: async () => {
                  try {
                    await undoConfirmOf(person.id, target.id, undoActivityId);
                    void navigate(personFacesPath(person.id));
                  } catch (failure) {
                    toast(text.candidates.undoFailed(errorText(failure)));
                  }
                },
              },
            },
      );
      void navigate(personFacesPath(target.id));
      return undefined;
    }
    replace(target);
    return undoActivityId;
  }

  /** Undo brings the merged person back and dismisses the pair; reopening offers it again as before. */
  /** The undo can take back the face the merge gave the shown person, so the person is read again. */
  async function undoConfirm(other: FaceCandidatePerson, activityId: number) {
    await undoConfirmOf(person.id, other.id, activityId);
    replace((await getCredit("person", person.id)) as Person);
  }

  function splitAppearances(appearanceIds: readonly number[]): Promise<Person> {
    return appearanceIds.length === 1
      ? splitFaceAppearance(person.id, appearanceIds[0] as number)
      : splitFaceAppearances(person.id, appearanceIds);
  }

  /** Merges the split person back; a person who had no face gets none from the merge. */
  async function undoSplit(created: Person) {
    const { credit, notFoundIds } = await mergeCredits("person", person.id, [created.id]);
    if (notFoundIds.includes(created.id)) {
      throw new RequestFailed(404, "not_found", "The split person is gone.", {
        reason: "person_not_found",
      });
    }
    if (
      person.imageRevision === undefined &&
      "imageRevision" in credit &&
      credit.imageRevision !== undefined
    ) {
      replace(await removePersonImage(person.id));
    }
  }

  const total = appearances.state.kind === "ready" ? appearances.state.total : undefined;
  return (
    <RecordPage
      back={back}
      title={text.facesPage.title}
      description={creditDisplayName(person)}
    >
      <Tabs
        value={tab}
        onValueChange={(next) => setTab(next as FacesTab)}
        className="flex flex-col gap-4"
      >
        <div className="flex flex-wrap items-center gap-2">
          <TabsList
            variant="line"
            aria-label={text.facesPage.tabsLabel}
            className="flex-1"
          >
            <TabsTrigger value="candidates">
              {text.facesPage.candidatesTab}
              <span className="font-normal text-muted-foreground tabular-nums">
                {formatNumber(candidates.items.length - candidates.decided.size)}
              </span>
            </TabsTrigger>
            <TabsTrigger value="faces">
              {text.facesPage.facesTab}
              {total !== undefined && (
                <span className="font-normal text-muted-foreground tabular-nums">
                  {formatNumber(total)}
                </span>
              )}
            </TabsTrigger>
          </TabsList>
          <Button variant="ghost" size="sm" onClick={refresh}>
            <RefreshCw aria-hidden="true" />
            {text.facesPage.refresh}
          </Button>
        </div>
        {/* Both tabs stay mounted, so a decided row or a split tile keeps its result across tab switches. */}
        <TabsContent
          value="candidates"
          key={`candidates-${String(round)}`}
          forceMount
          className="data-[state=inactive]:hidden"
        >
          <PageSection
            title={text.candidates.heading}
            description={text.facesPage.refreshHint}
          >
            <PersonCandidates
              person={person}
              sample={sample}
              items={candidates.items}
              failed={
                candidates.failed === null
                  ? null
                  : { reason: candidates.failed, retry: candidates.reload }
              }
              actions={{
                confirm: confirmCandidate,
                undoConfirm,
                remove: candidates.remove,
                decide: candidates.decide,
                onGone: markGone,
              }}
            />
          </PageSection>
        </TabsContent>
        <TabsContent
          value="faces"
          key={`faces-${String(round)}`}
          forceMount
          className="data-[state=inactive]:hidden"
        >
          <PageSection
            title={text.faces.heading}
            description={text.facesPage.refreshHint}
          >
            <PersonAppearances
              appearances={appearances}
              actions={{ split: splitAppearances, undoSplit, onGone: markGone }}
            />
          </PageSection>
        </TabsContent>
      </Tabs>
    </RecordPage>
  );
}
