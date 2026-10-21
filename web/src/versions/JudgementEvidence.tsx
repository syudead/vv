import { judgementFrameUrl, type VersionJudgeEvidence } from "../api/client";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration } from "../lib/format";
import { FactList } from "../ui/patterns/fact-list";
import { evidenceSentences } from "./evidenceSentences";

/**
 * Explain a duplicate judgement in sentences and show the frame pairs the AI saw; frames need the
 * version_judgement entry that serves them.
 */
export function JudgementEvidence({
  evidence,
  probability,
  activityId,
  columns = 2,
}: {
  evidence: VersionJudgeEvidence;
  probability: number | undefined;
  activityId: number | undefined;
  /** One column keeps each frame pair large enough to compare inside a narrow dialog. */
  columns?: 1 | 2;
}) {
  const j = t.versions.judgement;
  const frames = activityId === undefined ? [] : (evidence.frames ?? []);
  return (
    <div className="flex flex-col gap-2 text-sm text-muted-foreground">
      {evidenceSentences(evidence, probability).map((sentence, index) => (
        <p key={index}>{sentence}</p>
      ))}
      {evidence.facts !== undefined && evidence.facts.length > 0 && (
        <section aria-label={j.facts} className="flex flex-col gap-1.5">
          <h3 className="text-xs font-medium text-foreground">{j.facts}</h3>
          <FactList
            facts={evidence.facts.map((fact) => ({
              id: fact.kind,
              term: `${j.factKind[fact.kind]} · ${j.factMatch[fact.match]}`,
              value: j.factValues(fact.a, fact.b),
            }))}
          />
        </section>
      )}
      {activityId !== undefined && frames.length > 0 && (
        <figure className="flex flex-col gap-1.5">
          <figcaption className="text-xs font-medium text-foreground">
            {j.frames}
          </figcaption>
          <ul className={cn("grid gap-2", columns === 2 && "sm:grid-cols-2")}>
            {frames.map((frame, index) => {
              const left = formatDuration(frame.aMs);
              const right = formatDuration(frame.bMs);
              return (
                <li key={index} className="flex flex-col gap-1">
                  <img
                    src={judgementFrameUrl(activityId, index)}
                    alt={j.frameAlt(left, right)}
                    loading="lazy"
                    className="w-full rounded-md border border-border bg-muted"
                  />
                  <span className="text-xs tabular-nums">{j.frame(left, right)}</span>
                </li>
              );
            })}
          </ul>
        </figure>
      )}
    </div>
  );
}
