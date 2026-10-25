import { CircleAlert } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useLocation } from "react-router";

import type { AssistantCandidate, AssistantOutcome } from "../api/assistant";
import { getOllamaSettings } from "../api/client";
import { errorText, formatNumber, t } from "../i18n";
import { settingsPagePath } from "../settings/settingsPages";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import AnswerReply from "./AnswerReply";
import { useDestinationName } from "./destinationName";
import FilterReply from "./FilterReply";
import { atDestination } from "./links";
import type { Exchange, Reply, UnavailableReason } from "./thread";

/** After a minute the waiting row says the request goes on without the panel. */
const stillWaitingAfterMs = 60_000;

export function WaitingRow({ startedAt }: { startedAt: number }) {
  const [still, setStill] = useState(() => Date.now() - startedAt >= stillWaitingAfterMs);
  useEffect(() => {
    if (still) return;
    const timer = setTimeout(
      () => setStill(true),
      Math.max(0, stillWaitingAfterMs - (Date.now() - startedAt)),
    );
    return () => clearTimeout(timer);
  }, [still, startedAt]);
  return (
    <p role="status" className="flex items-start gap-2 text-sm text-muted-foreground">
      <Spinner aria-hidden="true" className="mt-0.5" />
      {still ? t.assistant.stillWaiting : t.assistant.waiting}
    </p>
  );
}

function Line({ children, warning = false }: { children: string; warning?: boolean }) {
  return <p className={warning ? "text-sm text-warning" : "text-sm"}>{children}</p>;
}

function NavigateReply({ outcome, path }: { outcome: AssistantOutcome; path?: string }) {
  const location = useLocation();
  const name = useDestinationName(outcome.destination);
  const left =
    path !== undefined && !atDestination(location.pathname, location.search, path);
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

/** The candidate's count is the videos it is on; other kinds carry 0, which says nothing. */
const countedKinds: ReadonlySet<AssistantCandidate["kind"]> = new Set([
  "tag",
  "person",
  "organization",
]);

function ClarifyReply({
  outcome,
  exchange,
  onChoose,
}: {
  outcome: AssistantOutcome;
  exchange: Exchange;
  onChoose?: (candidate: AssistantCandidate) => void;
}) {
  const words = t.assistant.clarify;
  const candidates = outcome.candidates ?? [];
  const first = candidates[0];
  if (first !== undefined) {
    return (
      <div className="flex flex-col gap-2">
        <p className="text-sm">{words.which(words.kinds[first.kind])}</p>
        <div className="flex flex-wrap gap-2">
          {candidates.map((candidate) => (
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
  const missing = outcome.names?.[0];
  if (missing !== undefined) return <Line>{words.notFound(missing)}</Line>;
  if (outcome.about === "target" && !exchange.hadSelection) {
    return <Line>{words.nothingSelected}</Line>;
  }
  return <Line>{words[outcome.about ?? "operation"]}</Line>;
}

function unsupportedText(operation: string | undefined): string {
  const excluded: Record<string, string> = t.assistant.excluded;
  const name = operation === undefined ? undefined : excluded[operation];
  return name === undefined
    ? t.assistant.unsupported
    : t.assistant.unsupportedKnown(name);
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

function UnavailableReply({ reason }: { reason: UnavailableReason }) {
  const [model, setModel] = useState("");
  useEffect(() => {
    if (reason !== "model") return;
    const controller = new AbortController();
    getOllamaSettings(controller.signal).then(
      (settings) => setModel(settings.generationModel),
      () => undefined,
    );
    return () => controller.abort();
  }, [reason]);
  const words = t.assistant.unavailable;
  const text = reason === "model" ? words.model(model) : words[reason];
  return (
    <Alert variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle className="font-normal">{text}</AlertTitle>
      {reason !== "timeout" && <ConnectionsLink />}
    </Alert>
  );
}

function OutcomeReply({
  outcome,
  path,
  exchange,
  onChoose,
}: {
  outcome: AssistantOutcome;
  path?: string;
  exchange: Exchange;
  onChoose?: (candidate: AssistantCandidate) => void;
}) {
  switch (outcome.kind) {
    case "filter":
      return outcome.criteria === undefined ? null : (
        <FilterReply criteria={outcome.criteria} />
      );
    case "navigate":
      return <NavigateReply outcome={outcome} path={path} />;
    case "answer":
      return <AnswerReply outcome={outcome} />;
    case "clarify":
      return <ClarifyReply outcome={outcome} exchange={exchange} onChoose={onChoose} />;
    case "unsupported":
      return <Line>{unsupportedText(outcome.operation)}</Line>;
    case "unreadable":
      return <Line>{t.assistant.unreadable}</Line>;
    default:
      // The confirmation card for a proposal is drawn by the card's own unit.
      return null;
  }
}

/** The reply rows under one request. `onChoose` is absent while another turn runs. */
export default function ReplyView({
  exchange,
  reply,
  onChoose,
}: {
  exchange: Exchange;
  reply: Reply;
  onChoose?: (candidate: AssistantCandidate) => void;
}) {
  switch (reply.kind) {
    case "outcome":
      return (
        <OutcomeReply
          outcome={reply.outcome}
          path={reply.path}
          exchange={exchange}
          onChoose={onChoose}
        />
      );
    case "unavailable":
      return <UnavailableReply reason={reply.reason} />;
    case "tooManyVideos":
      return <Line warning>{t.assistant.tooManyVideos}</Line>;
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
