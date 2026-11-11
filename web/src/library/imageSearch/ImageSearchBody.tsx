import { CircleAlert, RotateCw, SearchX } from "lucide-react";
import { type ReactNode, useLayoutEffect } from "react";

import type { ImageSearchResult } from "../../api/imageSearch";
import { t } from "../../i18n";
import type { Zoom } from "../../preferences/viewPreferences";
import { LoadingState } from "../../ui/patterns/loading-state";
import { PageHeader } from "../../ui/patterns/page-header";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "../../ui/shadcn/alert";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { Grid } from "../../videoList/Grid";
import { EmptyState } from "../../videoList/states";
import type { ImageSearchPhase, Refusal } from "./imageSearchState";
import { ResultCard } from "./ResultCard";
import type { ImageSearch } from "./useImageSearch";

const skeletonCount = 12;

export interface ImageSearchBodyProps {
  search: ImageSearch;
  zoom: Zoom;
  /** The library URL the player goes back to. */
  backTo: string;
  /** The library's body, shown while no image search is active. */
  children: ReactNode;
}

/** The image search's header and state in place of the library's body, or the library while none is active. */
export function ImageSearchBody({
  search,
  zoom,
  backTo,
  children,
}: ImageSearchBodyProps) {
  const { phase } = search;
  if (phase === null) return children;
  return (
    <section
      aria-label={t.library.imageSearch.title}
      className="flex min-w-0 flex-col gap-3"
    >
      <Header phase={phase} onCancel={search.clear} />
      <PhaseBody phase={phase} search={search} zoom={zoom} backTo={backTo} />
    </section>
  );
}

function Header({ phase, onCancel }: { phase: ImageSearchPhase; onCancel: () => void }) {
  const text = t.library.imageSearch;
  if (phase.kind === "searching") {
    const video = phase.file.type.startsWith("video/");
    return (
      <PageHeader
        title={text.title}
        headingLevel={2}
        count={
          <span role="status" className="inline-flex items-center gap-1.5">
            <Spinner aria-hidden="true" />
            {text.searching}
          </span>
        }
        description={video ? text.readingVideo(phase.file.name) : undefined}
        actions={
          <Button variant="outline" size="sm" onClick={onCancel}>
            {text.cancel}
          </Button>
        }
      />
    );
  }
  const found = phase.kind === "results" && phase.result.items.length > 0;
  return (
    <PageHeader
      title={text.title}
      headingLevel={2}
      count={
        found ? (
          <span role="status">{text.matches(phase.result.items.length)}</span>
        ) : undefined
      }
      description={found ? unindexedText(phase.result) : undefined}
    />
  );
}

function unindexedText(result: ImageSearchResult): string | undefined {
  return result.unindexedCount > 0
    ? t.library.imageSearch.unindexed(result.unindexedCount)
    : undefined;
}

function PhaseBody({
  phase,
  search,
  zoom,
  backTo,
}: {
  phase: ImageSearchPhase;
  search: ImageSearch;
  zoom: Zoom;
  backTo: string;
}) {
  const text = t.library.imageSearch;
  switch (phase.kind) {
    case "searching":
      return (
        <LoadingState
          label={text.searching}
          layout="grid"
          size={zoom}
          count={skeletonCount}
        />
      );
    case "refused":
      return (
        <RefusedAlert
          refusal={phase.refusal}
          onChoose={() => search.setDialogOpen(true)}
          onRetry={search.retry}
        />
      );
    case "results":
      if (phase.result.items.length === 0) {
        return <NoMatch result={phase.result} onClear={search.clear} />;
      }
      return (
        <Results
          result={phase.result}
          zoom={zoom}
          backTo={backTo}
          entry={search.entry}
          scrollY={search.restoredScrollY}
          onOpen={search.keepScroll}
        />
      );
  }
}

function Results({
  result,
  zoom,
  backTo,
  entry,
  scrollY,
  onOpen,
}: {
  result: ImageSearchResult;
  zoom: Zoom;
  backTo: string;
  entry: string;
  scrollY: number | undefined;
  onOpen: () => void;
}) {
  // Back from the player, the cards are drawn from memory in this commit, so the position applies at once.
  useLayoutEffect(() => {
    if (scrollY !== undefined) window.scrollTo({ top: scrollY, behavior: "auto" });
  }, [scrollY]);
  return (
    <Grid zoom={zoom}>
      {result.items.map((item) => (
        <ResultCard
          key={`${String(item.video.id)}-${String(item.match.videoId)}`}
          item={item}
          backTo={backTo}
          entry={entry}
          onOpen={onOpen}
        />
      ))}
    </Grid>
  );
}

function NoMatch({
  result,
  onClear,
}: {
  result: ImageSearchResult;
  onClear: () => void;
}) {
  const text = t.library.imageSearch;
  return (
    <EmptyState
      icon={SearchX}
      title={text.noMatch}
      description={unindexedText(result) ?? text.allSearched}
      action={
        <Button variant="outline" size="sm" onClick={onClear}>
          {text.clear}
        </Button>
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
        <Button variant="outline" size="sm" onClick={onChoose}>
          {text.chooseAnother}
        </Button>
        {refusal.retry && (
          <Button variant="outline" size="sm" onClick={onRetry}>
            <RotateCw aria-hidden="true" />
            {text.retry}
          </Button>
        )}
      </AlertAction>
    </Alert>
  );
}
