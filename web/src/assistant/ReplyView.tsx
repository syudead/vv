import { CircleAlert, TriangleAlert } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { Link, useLocation } from "react-router";

import type {
  AssistantCandidate,
  AssistantDestination,
  AssistantTurn,
} from "../api/assistant";
import { getOllamaSettings } from "../api/client";
import { errorText, formatNumber, t } from "../i18n";
import { settingsPagePath } from "../settings/settingsPages";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import AnswerReply from "./AnswerReply";
import { routePageName, useDestinationName } from "./destinationName";
import FilterReply from "./FilterReply";
import { atDestination, criteriaPath, destinationPath } from "./links";
import ReviewReply from "./ReviewReply";
import type { Exchange, LocalReply } from "./thread";
import ToolCalls from "./ToolCalls";

function WarningLine({ children }: { children: ReactNode }) {
  return (
    <p className="flex items-start gap-2 text-sm text-warning">
      <TriangleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      {children}
    </p>
  );
}

/** Under the alert's text: the action names a page, too wide for the alert's corner. */
export function ConnectionsLink() {
  return (
    <div className="col-start-2 pt-2">
      <Button asChild variant="outline" size="sm">
        <Link to={settingsPagePath("connections")}>
          {t.assistant.readiness.openConnections}
        </Link>
      </Button>
    </div>
  );
}

/** The saved generation model's name, read only when a line names it. */
export function useGenerationModel(wanted: boolean): string {
  const [model, setModel] = useState("");
  useEffect(() => {
    if (!wanted) return;
    const controller = new AbortController();
    getOllamaSettings(controller.signal).then(
      (settings) => setModel(settings.generationModel),
      () => undefined,
    );
    return () => controller.abort();
  }, [wanted]);
  return model;
}

function UnavailableReply({ reason }: { reason: "connection" | "model" }) {
  const model = useGenerationModel(reason === "model");
  const words = t.assistant.unavailable;
  return (
    <Alert variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle className="font-normal">
        {reason === "model" ? words.model(model) : words.connection}
      </AlertTitle>
      <ConnectionsLink />
    </Alert>
  );
}

/** The candidate's count is the videos it is on; other kinds carry 0, which says nothing. */
const countedKinds: ReadonlySet<AssistantCandidate["kind"]> = new Set([
  "tag",
  "person",
  "organization",
]);

function ClarifyReply({
  turn,
  onChoose,
}: {
  turn: AssistantTurn;
  onChoose?: (candidate: AssistantCandidate) => void;
}) {
  const clarify = turn.clarify;
  if (clarify === undefined) return null;
  const words = t.assistant.clarify;
  if (clarify.candidates.length === 0) {
    return <p className="text-sm">{words.notFound(clarify.name)}</p>;
  }
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm">{words.which(words.kinds[clarify.kind], clarify.name)}</p>
      <div className="flex flex-wrap gap-2">
        {clarify.candidates.map((candidate) => (
          <Button
            key={`${candidate.kind}:${String(candidate.id)}`}
            variant="outline"
            size="sm"
            aria-disabled={onChoose === undefined}
            onClick={() => onChoose?.(candidate)}
          >
            {candidate.name}
            {countedKinds.has(candidate.kind) && (
              <span className="text-muted-foreground tabular-nums">
                {formatNumber(candidate.count)}
              </span>
            )}
          </Button>
        ))}
      </div>
    </div>
  );
}

function NavigateReply({ destination }: { destination: AssistantDestination }) {
  const location = useLocation();
  const name = useDestinationName(destination);
  const path = destinationPath(destination);
  const left = !atDestination(location.pathname, location.search, path);
  return (
    <p className="flex flex-wrap items-center gap-x-2 text-sm">
      {name !== undefined && t.assistant.opened(name)}
      {left && (
        <Button asChild variant="link" size="sm" className="h-auto p-0">
          <Link to={path}>{t.assistant.open}</Link>
        </Button>
      )}
    </p>
  );
}

/** A directive that applied shows what it did; one that did not offers its link instead. */
function DirectiveReply({ turn, applied }: { turn: AssistantTurn; applied: boolean }) {
  const directive = turn.directive;
  if (directive?.kind === "navigate" && directive.destination !== undefined) {
    return <NavigateReply destination={directive.destination} />;
  }
  if (directive?.kind !== "filter" || directive.criteria === undefined) return null;
  if (applied) return <FilterReply criteria={directive.criteria} />;
  return (
    <Button asChild variant="link" size="sm" className="h-auto self-start p-0">
      <Link to={criteriaPath(directive.criteria)}>{t.assistant.showInLibrary}</Link>
    </Button>
  );
}

/** The review runs the turn's start_review calls named, once each. */
function reviewRunIds(turn: AssistantTurn): number[] {
  const ids = turn.progress.flatMap((item) =>
    item.reviewRunId === undefined ? [] : [item.reviewRunId],
  );
  return [...new Set(ids)];
}

function ExcludedReply({ turn }: { turn: AssistantTurn }) {
  const excluded = turn.stop?.excluded;
  if (excluded === undefined) return null;
  const operation = t.assistant.excluded[excluded.operation] ?? t.assistant.excludedOther;
  const page = routePageName(excluded.page);
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm">{t.assistant.excludedReply(operation)}</p>
      {page !== undefined && (
        <Button asChild variant="outline" size="sm" className="self-start">
          <Link to={excluded.page}>{t.assistant.openPage(page)}</Link>
        </Button>
      )}
    </div>
  );
}

type WarningStop = keyof typeof t.assistant.stopped;

/** Why the turn ended without its result; connection and model keep the shipped alert. */
function StopReply({ turn }: { turn: AssistantTurn }) {
  if (turn.stop?.excluded !== undefined) return <ExcludedReply turn={turn} />;
  const reason: WarningStop | "connection" | "model" | undefined =
    turn.status === "interrupted" ? "interrupted" : turn.stop?.reason;
  if (reason === undefined) return null;
  if (reason === "connection" || reason === "model") {
    return <UnavailableReply reason={reason} />;
  }
  return <WarningLine>{t.assistant.stopped[reason]}</WarningLine>;
}

/** The rows a finished turn shows above its card, in the order the design gives them. */
function TurnReply({
  exchange,
  turn,
  onChoose,
}: {
  exchange: Exchange;
  turn: AssistantTurn;
  onChoose?: (candidate: AssistantCandidate) => void;
}) {
  if (turn.status === "running") return null;
  return (
    <>
      <AnswerReply turn={turn} />
      <DirectiveReply turn={turn} applied={exchange.applied !== undefined} />
      {reviewRunIds(turn).map((id) => (
        <ReviewReply key={id} runId={id} turn={turn} exchange={exchange} />
      ))}
      <ClarifyReply turn={turn} onChoose={onChoose} />
      <StopReply turn={turn} />
    </>
  );
}

function LocalReplyView({ reply }: { reply: LocalReply }) {
  switch (reply.kind) {
    case "busy":
      return <p className="text-sm text-warning">{t.assistant.busy}</p>;
    case "expired":
      return <p className="text-xs text-muted-foreground">{t.assistant.expired}</p>;
    case "tooManyVideos":
      return <WarningLine>{t.assistant.tooManyVideos}</WarningLine>;
    case "failed":
      return (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle className="font-normal">
            {t.assistant.failed(errorText(reply.error))}
          </AlertTitle>
        </Alert>
      );
  }
}

/**
 * Everything under one request but its card: the tool calls, then the turn's rows, or the reply
 * the panel drew itself. `onChoose` is absent while another turn runs.
 */
export default function ReplyView({
  exchange,
  onChoose,
}: {
  exchange: Exchange;
  onChoose?: (candidate: AssistantCandidate) => void;
}) {
  const { turn, reply } = exchange;
  // An expired turn's reply replaces everything it showed.
  if (reply?.kind === "expired") return <LocalReplyView reply={reply} />;
  return (
    <>
      {turn !== undefined && <ToolCalls turn={turn} />}
      {turn !== undefined && (
        <TurnReply exchange={exchange} turn={turn} onChoose={onChoose} />
      )}
      {reply !== undefined && <LocalReplyView reply={reply} />}
    </>
  );
}
