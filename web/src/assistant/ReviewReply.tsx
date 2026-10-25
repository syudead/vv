import { TriangleAlert } from "lucide-react";
import { Link } from "react-router";

import type { AssistantReviewRun } from "../api/assistant";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Progress } from "../ui/shadcn/progress";
import { Spinner } from "../ui/shadcn/spinner";
import { useAssistant } from "./AssistantProvider";
import { reviewPath, wasAlreadyRunning } from "./review";
import type { Exchange } from "./thread";

/** The runs earlier replies in the thread answered, so asking again names the same run as running. */
function earlierRunIds(exchanges: readonly Exchange[], before: number): Set<number> {
  const ids = new Set<number>();
  for (const exchange of exchanges) {
    const reply = exchange.reply;
    if (exchange.id >= before || reply?.kind !== "outcome") continue;
    if (reply.outcome.run !== undefined) ids.add(reply.outcome.run.id);
  }
  return ids;
}

function ReviewState({ run }: { run: AssistantReviewRun }) {
  const words = t.assistant.review;
  if (run.status === "running") {
    return (
      <div className="flex flex-col gap-1">
        <p className="flex items-start gap-2 text-sm text-muted-foreground">
          <Spinner aria-hidden="true" className="mt-0.5" />
          <span className="tabular-nums">
            {words.looking(words.kinds[run.kind], run.judged, run.total)}
          </span>
        </p>
        <Progress aria-label={words.progress} value={run.judged} max={run.total} />
      </div>
    );
  }
  if (run.status === "done")
    return <p className="text-sm">{words.toReview(run.listed)}</p>;
  return (
    <p className="flex items-start gap-2 text-sm text-warning">
      <TriangleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      {run.status === "interrupted" ? words.stoppedByRestart : words.stopped}
    </p>
  );
}

/**
 * A review outcome's rows: the run's progress, which follows assistant events through the store,
 * and the link to its review list.
 */
export default function ReviewReply({
  run: answered,
  exchange,
}: {
  run: AssistantReviewRun;
  exchange: Exchange;
}) {
  const store = useAssistant();
  const run = store?.reviews.get(answered.id) ?? answered;
  const alreadyRunning = wasAlreadyRunning(
    answered,
    exchange.startedAt,
    earlierRunIds(store?.exchanges ?? [], exchange.id),
  );
  const words = t.assistant.review;
  return (
    <div className="flex flex-col gap-2">
      {alreadyRunning && (
        <p className="text-sm">{words.alreadyRunning(words.kinds[run.kind])}</p>
      )}
      <ReviewState run={run} />
      <Button asChild variant="outline" size="sm" className="self-start">
        <Link to={reviewPath(run.id)}>{words.openList}</Link>
      </Button>
    </div>
  );
}
