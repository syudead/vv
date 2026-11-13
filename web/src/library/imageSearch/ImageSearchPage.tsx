import { ArrowLeft, CircleAlert, RotateCw, ScanSearch, SearchX } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router";

import { useAudience } from "../../auth/audience";
import { t } from "../../i18n";
import TopBarPortal from "../../shell/TopBarPortal";
import { LoadingState } from "../../ui/patterns/loading-state";
import { PageHeader } from "../../ui/patterns/page-header";
import { QueryResultsPage } from "../../ui/patterns/query-results-page";
import { Toolbar } from "../../ui/patterns/toolbar";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import SearchBox from "../../videoList/SearchBox";
import { EmptyState } from "../../videoList/states";
import { LIBRARY_PATH, libraryQueryPath } from "../libraryPath";
import {
  IMAGE_SEARCH_PATH,
  searchToken,
  useBackToLibrary,
  useGiveFiles,
} from "./imageSearchEntry";
import { ImageSearchDialog } from "./ImageSearchDialog";
import { ImageSearchResults } from "./ImageSearchResults";
import {
  type ImageSearchPhase,
  recall,
  type Refusal,
  rememberStrip,
  rememberView,
} from "./imageSearchState";
import { QueryChip } from "./QueryChip";
import { QueryPanel } from "./QueryPanel";
import { type ImageSearch, useImageSearch } from "./useImageSearch";

/** The image search screen at `/image-search`; a guest is sent to the library. */
export default function ImageSearchPage() {
  const owner = useAudience() === "owner";
  if (!owner) return <Navigate to={LIBRARY_PATH} replace />;
  return <ImageSearchScreen />;
}

/**
 * Below lg the panel is open until an answer arrives, then collapses so the first rows are on screen;
 * the owner's own toggle holds for that answer.
 */
function usePanelOpen(search: ImageSearch): [boolean, (open: boolean) => void] {
  const { phase, restored, token } = search;
  const [toggle, setToggle] = useState(() => ({ phase, open: restored?.panelOpen }));
  let current = toggle;
  if (toggle.phase !== phase) {
    current = { phase, open: restored?.panelOpen };
    setToggle(current);
  }
  const change = (open: boolean) => {
    setToggle({ phase, open });
    if (token !== undefined) rememberView(token, { panelOpen: open });
  };
  return [current.open ?? phase?.kind !== "results", change];
}

function ImageSearchScreen() {
  const text = t.library.imageSearch;
  const location = useLocation();
  const navigate = useNavigate();
  const token = searchToken(location.state);
  const search = useImageSearch(token);
  const giving = useGiveFiles("replace");
  const backToLibrary = useBackToLibrary();
  const field = useRef<HTMLInputElement | null>(null);
  const title = useRef<HTMLHeadingElement | null>(null);
  const [panelOpen, setPanelOpen] = usePanelOpen(search);
  const { phase } = search;

  // A search opened by a file keeps focus where the file was given: the field, or the title after the dialog.
  const givenFrom =
    token === undefined || search.restored !== undefined
      ? undefined
      : recall(token)?.from;
  useEffect(() => {
    if (givenFrom === "field") field.current?.focus();
    else if (givenFrom === "dialog") title.current?.focus();
  }, [givenFrom, token]);

  const choose = () => giving.setDialogOpen(true);
  return (
    <>
      <TopBarPortal>
        <Toolbar
          placement="topBar"
          search={
            <SearchBox
              query=""
              onCommit={(next) => {
                if (next !== "") void navigate(libraryQueryPath(next));
              }}
              inputRef={field}
              commitOn="enter"
              fileSearch={{
                onPick: choose,
                onFiles: giving.onFiles,
                onClear: backToLibrary,
                chip:
                  phase === null ? undefined : (
                    <QueryChip file={phase.file} onClear={backToLibrary} />
                  ),
              }}
            />
          }
        />
      </TopBarPortal>
      <ImageSearchDialog giving={giving} />
      <QueryResultsPage
        header={
          <PageHeader
            title={text.title}
            titleRef={title}
            leading={
              <Button variant="ghost" size="sm" onClick={backToLibrary}>
                <ArrowLeft aria-hidden="true" />
                {text.library}
              </Button>
            }
          />
        }
        query={
          phase === null ? undefined : (
            <QueryPanel
              phase={phase}
              open={panelOpen}
              onOpenChange={setPanelOpen}
              onChoose={choose}
              onCancel={backToLibrary}
            />
          )
        }
        queryLabel={text.query}
        queryCollapsed={phase !== null && !panelOpen}
      >
        <ResultsColumn search={search} onChoose={choose} onBack={backToLibrary} />
      </QueryResultsPage>
    </>
  );
}

function ResultsColumn({
  search,
  onChoose,
  onBack,
}: {
  search: ImageSearch;
  onChoose: () => void;
  onBack: () => void;
}) {
  const text = t.library.imageSearch;
  const { phase, token, restored } = search;
  if (phase === null || token === undefined) return <Idle onChoose={onChoose} />;
  switch (phase.kind) {
    case "searching":
      return <LoadingState label={text.searching} layout="rows" />;
    case "refused":
      return (
        <RefusedAlert
          refusal={phase.refusal}
          onChoose={onChoose}
          onRetry={search.retry}
        />
      );
    case "results":
      if (phase.result.items.length === 0) {
        return <NoMatch phase={phase} onChoose={onChoose} onBack={onBack} />;
      }
      return (
        <ImageSearchResults
          // Another search's results start from that search's view, not this one's.
          key={token}
          result={phase.result}
          backTo={IMAGE_SEARCH_PATH}
          entry={token}
          restored={restored}
          onCollapsedChange={(collapsed) => rememberView(token, { collapsed })}
          onStripChange={(row, frame) => rememberStrip(token, row, frame)}
          onOpen={(opener) => rememberView(token, { scrollY: window.scrollY, opener })}
        />
      );
  }
}

function Idle({ onChoose }: { onChoose: () => void }) {
  const text = t.library.imageSearch.idle;
  return (
    <EmptyState
      icon={ScanSearch}
      title={text.title}
      description={text.description}
      action={
        <Button size="sm" onClick={onChoose}>
          {text.choose}
        </Button>
      }
    />
  );
}

function NoMatch({
  phase,
  onChoose,
  onBack,
}: {
  phase: Extract<ImageSearchPhase, { kind: "results" }>;
  onChoose: () => void;
  onBack: () => void;
}) {
  const text = t.library.imageSearch;
  const { unindexedCount } = phase.result;
  return (
    <EmptyState
      icon={SearchX}
      title={text.noMatch}
      description={unindexedCount > 0 ? text.unindexed(unindexedCount) : text.allSearched}
      action={
        <>
          <Button variant="outline" size="sm" onClick={onChoose}>
            {text.chooseAnother}
          </Button>
          <Button variant="ghost" size="sm" onClick={onBack}>
            {text.backToLibrary}
          </Button>
        </>
      }
    />
  );
}

function RefusedAlert({
  refusal,
  onChoose,
  onRetry,
}: {
  refusal: Refusal;
  onChoose: () => void;
  onRetry: () => void;
}) {
  const text = t.library.imageSearch;
  return (
    <Alert variant="destructive" className="has-data-[slot=alert-action]:pr-3">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{refusal.title}</AlertTitle>
      {refusal.description !== undefined && (
        <AlertDescription>{refusal.description}</AlertDescription>
      )}
      {/* Two actions do not fit the corner, so they follow the text. */}
      <AlertAction className="static col-start-2 mt-1.5 flex flex-wrap gap-2">
        {refusal.retry && (
          <Button variant="outline" size="sm" onClick={onRetry}>
            <RotateCw aria-hidden="true" />
            {text.retry}
          </Button>
        )}
        <Button variant="outline" size="sm" onClick={onChoose}>
          {text.chooseAnother}
        </Button>
      </AlertAction>
    </Alert>
  );
}
