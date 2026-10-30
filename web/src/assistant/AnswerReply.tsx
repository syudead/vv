import { Scale } from "lucide-react";
import { useId } from "react";
import { Link } from "react-router";

import type { AssistantTurn } from "../api/assistant";
import { personImagePath } from "../api/credits";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { PersonFace } from "../ui/PersonFace";
import { Button } from "../ui/shadcn/button";
import { answerBlocks, type MentionedLabel, mentionedLabels } from "./answerText";
import { labelPath } from "./links";

function AnswerText({ answer }: { answer: string }) {
  return (
    <div className="flex flex-col gap-2 text-sm wrap-anywhere">
      {answerBlocks(answer).map((block, index) => {
        if (block.kind === "text") {
          return (
            <p key={index} className="whitespace-pre-wrap">
              {block.text}
            </p>
          );
        }
        const List = block.ordered ? "ol" : "ul";
        return (
          <List
            key={index}
            className={cn(
              "flex flex-col gap-1 pl-4",
              block.ordered ? "list-decimal" : "list-disc",
            )}
          >
            {block.items.map((item, itemIndex) => (
              <li key={itemIndex}>{item}</li>
            ))}
          </List>
        );
      })}
    </div>
  );
}

function linked(labels: readonly MentionedLabel[]) {
  return labels.flatMap((label) => {
    const path = labelPath(label.kind, label.id);
    return path === undefined ? [] : [{ label, path }];
  });
}

/** The entities the answer names, as links: video titles in a column, the rest wrapped. */
function Mentioned({ labels }: { labels: readonly MentionedLabel[] }) {
  const headingId = useId();
  const links = linked(labels);
  if (links.length === 0) return null;
  const videos = links.filter(({ label }) => label.kind === "video");
  const others = links.filter(({ label }) => label.kind !== "video");
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-1">
      <h4 id={headingId} className="text-xs font-medium text-muted-foreground">
        {t.assistant.mentioned}
      </h4>
      {videos.length > 0 && (
        <ul className="flex flex-col gap-1">
          {videos.map(({ label, path }) => (
            <li key={label.id} className="flex min-w-0">
              <Button
                asChild
                variant="link"
                size="sm"
                className="h-auto max-w-full justify-start p-0"
              >
                <Link to={path}>
                  <span className="truncate">{label.name}</span>
                </Link>
              </Button>
            </li>
          ))}
        </ul>
      )}
      {others.length > 0 && (
        <ul className="flex flex-wrap gap-x-2">
          {others.map(({ label, path }) => (
            <li key={`${label.kind}:${String(label.id)}`} className="flex min-w-0">
              <Button
                asChild
                variant="link"
                size="sm"
                className="max-w-full px-0 has-[>svg]:px-0"
              >
                <Link to={path}>
                  {label.kind === "person" && (
                    <PersonFace
                      shape="chip"
                      name={label.name}
                      src={personImagePath(label.id)}
                    />
                  )}
                  <span className="truncate">{label.name}</span>
                </Link>
              </Button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/**
 * The model's answer as plain text, marked as Clef's when it used its estimates, and the entities
 * it names as links. The panel adds no count of its own.
 */
export default function AnswerReply({ turn }: { turn: AssistantTurn }) {
  const answer = turn.answer ?? "";
  if (answer === "") return null;
  return (
    <div className="flex flex-col gap-2">
      {turn.judged && (
        <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
          <Scale aria-hidden="true" className="mt-0.5 size-3 shrink-0" />
          {t.assistant.judged}
        </p>
      )}
      <AnswerText answer={answer} />
      <Mentioned labels={mentionedLabels(answer, turn.progress)} />
    </div>
  );
}
