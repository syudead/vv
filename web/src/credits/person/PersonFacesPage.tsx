import { ArrowLeft, RefreshCw } from "lucide-react";
import { type ReactNode, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";

import { mergeCredits, type Person } from "../../api/credits";
import {
  confirmFaceCandidate,
  type FaceAppearance,
  type FaceCandidatePerson,
  splitFaceAppearance,
} from "../../api/personFaces";
import { formatNumber, t } from "../../i18n";
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

/** The tab in the address, so a reload keeps it; without one, the pairs unless none wait. */
function useFacesTab(noCandidates: boolean): [FacesTab, (next: FacesTab) => void] {
  const [params, setParams] = useSearchParams();
  const asked = params.get("tab");
  const tab: FacesTab =
    asked === "faces" || asked === "candidates"
      ? asked
      : noCandidates
        ? "faces"
        : "candidates";
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
  // Each refresh remounts the lists, so decided rows and split tiles start over from the server.
  const [round, setRound] = useState(0);
  const candidates = useFaceCandidates(person.id, markGone);
  const appearances = usePersonAppearances(person.id, markGone);
  const sample = useFaceSample(person.id, round);
  const [tab, setTab] = useFacesTab(candidates.loaded && candidates.items.length === 0);

  function refresh() {
    candidates.reload();
    appearances.reload();
    setRound((value) => value + 1);
  }

  /** The shown person stays unless they have no name and the other has; then the page follows the merge. */
  async function confirmCandidate(other: FaceCandidatePerson) {
    const target = await confirmFaceCandidate(person.id, other.id);
    if (target.id !== person.id) {
      toast(text.mergedInto(creditDisplayName(target)));
      void navigate(personFacesPath(target.id));
      return;
    }
    replace(target);
  }

  function splitAppearance(appearance: FaceAppearance): Promise<Person> {
    return splitFaceAppearance(person.id, appearance.appearanceId);
  }

  async function undoSplit(created: Person) {
    await mergeCredits("person", person.id, [created.id]);
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
                {formatNumber(candidates.items.length)}
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
        <TabsContent value="candidates" key={`candidates-${String(round)}`}>
          <PageSection
            title={text.candidates.heading}
            description={text.facesPage.refreshHint}
          >
            <PersonCandidates
              person={person}
              sample={sample}
              items={candidates.items}
              actions={{
                confirm: confirmCandidate,
                remove: candidates.remove,
                onGone: markGone,
              }}
            />
          </PageSection>
        </TabsContent>
        <TabsContent value="faces" key={`faces-${String(round)}`}>
          <PageSection
            title={text.faces.heading}
            description={text.facesPage.refreshHint}
          >
            <PersonAppearances
              appearances={appearances}
              actions={{ split: splitAppearance, undoSplit, onGone: markGone }}
            />
          </PageSection>
        </TabsContent>
      </Tabs>
    </RecordPage>
  );
}
