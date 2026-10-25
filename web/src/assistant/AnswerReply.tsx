import { Link } from "react-router";

import type { AssistantOutcome } from "../api/assistant";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { useDestinationName } from "./destinationName";
import { criteriaPath, destinationPath } from "./links";

/** At most this many titles; the server answers no more. */
const maxTitles = 20;

function InlineLink({ to, children }: { to: string; children: string }) {
  return (
    <Button
      asChild
      variant="link"
      size="sm"
      className="h-auto max-w-full justify-start p-0"
    >
      <Link to={to}>
        <span className="truncate">{children}</span>
      </Link>
    </Button>
  );
}

function Titles({ items }: { items: AssistantOutcome["items"] }) {
  if (items === undefined || items.length === 0) return null;
  return (
    <ul className="flex flex-col gap-1">
      {items.slice(0, maxTitles).map((item) => (
        <li key={item.id} className="flex min-w-0">
          <InlineLink to={`/videos/${String(item.id)}`}>{item.title}</InlineLink>
        </li>
      ))}
    </ul>
  );
}

function Figure({ children }: { children: string }) {
  return <p className="text-lg font-semibold tabular-nums">{children}</p>;
}

function Appearances({ outcome }: { outcome: AssistantOutcome }) {
  const destination = outcome.link?.destination;
  const name = useDestinationName(destination);
  const total = outcome.total ?? 0;
  return (
    <div className="flex flex-col gap-1">
      <Figure>
        {name === undefined
          ? t.assistant.answer.videos(total)
          : t.assistant.answer.videosWith(total, name)}
      </Figure>
      {destination !== undefined && (
        <InlineLink to={destinationPath(destination)}>
          {name === undefined ? t.assistant.open : t.assistant.answer.openNamed(name)}
        </InlineLink>
      )}
    </div>
  );
}

/** The number is the reply's prominent text, with a link to where it came from. */
export default function AnswerReply({ outcome }: { outcome: AssistantOutcome }) {
  const words = t.assistant.answer;
  if (outcome.question === "appearances") return <Appearances outcome={outcome} />;
  if (outcome.question === "recent") {
    return (
      <div className="flex flex-col gap-1">
        <p className="text-xs font-medium text-muted-foreground">{words.recent}</p>
        <Titles items={outcome.items} />
      </div>
    );
  }
  const total = outcome.total ?? 0;
  const shown = Math.min(outcome.items?.length ?? 0, maxTitles);
  const criteria = outcome.link?.criteria;
  return (
    <div className="flex flex-col gap-1">
      <Figure>
        {outcome.question === "list" && total > 0
          ? words.listed(total, shown)
          : words.videos(total)}
      </Figure>
      {outcome.question === "list" && <Titles items={outcome.items} />}
      {criteria !== undefined && (
        <InlineLink to={criteriaPath(criteria)}>{words.showInLibrary}</InlineLink>
      )}
    </div>
  );
}
