import {
  ArrowUpRight,
  Ban,
  ChevronRight,
  CircleHelp,
  Eye,
  FileSearch,
  Image,
  ListChecks,
  ListFilter,
  ListPlus,
  type LucideIcon,
  Scale,
  ScanEye,
  Wrench,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  stopAssistantTurn,
  type AssistantProgressItem,
  type AssistantTurn,
} from "../api/assistant";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration } from "../lib/format";
import { Button } from "../ui/shadcn/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../ui/shadcn/collapsible";
import { Progress } from "../ui/shadcn/progress";
import { Spinner } from "../ui/shadcn/spinner";
import { useAssistant } from "./AssistantProvider";
import { useDestinationName } from "./destinationName";
import { operationSentence } from "./proposal";

/** Names after the line's words; the rest are counted. */
const maxNames = 3;

/** After a minute the running line says the turn goes on without the panel. */
const longRunMs = 60_000;

interface LineText {
  icon: LucideIcon;
  text: string;
  /** Whether the labels' names follow the text; open_page names its page in the text instead. */
  names: boolean;
}

const builtinLines: Record<string, LineText> = {
  screen_context: { icon: ScanEye, text: t.assistant.tools.screen_context, names: true },
  find_tools: { icon: Wrench, text: t.assistant.tools.find_tools, names: true },
  app_help: { icon: CircleHelp, text: t.assistant.tools.app_help, names: true },
  show_in_library: {
    icon: ListFilter,
    text: t.assistant.tools.show_in_library,
    names: true,
  },
};

/** The line's icon and words, chosen by the item's tool and kind. */
function lineText(item: AssistantProgressItem, page: string, review: string): LineText {
  const words = t.assistant.tools;
  const images = item.images ?? 0;
  const builtin = builtinLines[item.tool];
  if (builtin !== undefined) return builtin;
  switch (item.tool) {
    case "clef":
      return { icon: Scale, text: words.clef(images), names: true };
    case "view_images":
      return { icon: Eye, text: words.view_images(images), names: true };
    case "open_page":
      return { icon: ArrowUpRight, text: words.open_page(page), names: false };
    case "start_review":
      return { icon: ListChecks, text: words.start_review(review), names: true };
  }
  switch (item.kind) {
    case "write":
      return {
        icon: ListPlus,
        text: words.write(operationSentence(item.tool)),
        names: true,
      };
    case "image":
      return { icon: Image, text: words.image(item.tool), names: true };
    case "excluded":
      return { icon: Ban, text: words.excluded(item.tool), names: true };
    default:
      return { icon: FileSearch, text: words.read(item.tool), names: true };
  }
}

/** open_page names an entity in its label, or a page the turn's navigate directive names. */
function usePageName(item: AssistantProgressItem, turn: AssistantTurn): string {
  const destination =
    item.tool === "open_page" && item.labels.length === 0
      ? turn.directive?.destination
      : undefined;
  const name = useDestinationName(destination);
  return item.labels[0]?.name ?? name ?? "";
}

function ToolLine({
  item,
  turn,
  current,
}: {
  item: AssistantProgressItem;
  turn: AssistantTurn;
  /** The item still running, which shows its progress bar. */
  current: boolean;
}) {
  const store = useAssistant();
  const page = usePageName(item, turn);
  const run =
    item.reviewRunId === undefined ? undefined : store?.reviews.get(item.reviewRunId);
  const review = run === undefined ? "" : t.assistant.review.kinds[run.kind];
  const { icon: Icon, text, names } = lineText(item, page, review);
  const words = t.assistant.tools;
  const shown = names ? item.labels.slice(0, maxNames) : [];
  const more = names ? item.labels.length - shown.length : 0;
  const counted = item.n !== undefined && item.m !== undefined;
  return (
    <li className="flex items-start gap-1.5">
      <Icon aria-hidden="true" className="mt-0.5 size-3 shrink-0" />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="wrap-anywhere">
          {text}
          {shown.length > 0 && " · "}
          {shown.map((label, index) => (
            <span key={`${label.kind}:${String(label.id)}:${String(index)}`}>
              {index > 0 && ", "}
              <span className="text-foreground">{label.name}</span>
            </span>
          ))}
          {more > 0 && ` ${words.more(more)}`}
          {counted && (
            <span className="tabular-nums">
              {` ${words.of(item.n ?? 0, item.m ?? 0)}`}
            </span>
          )}
          {item.error !== undefined && ` · ${words.failed}`}
        </p>
        {item.error !== undefined && (
          <p className="wrap-anywhere text-muted-foreground">{item.error}</p>
        )}
        {current && counted && (
          <Progress aria-label={words.progress} value={item.n} max={item.m} />
        )}
      </div>
    </li>
  );
}

/** Ticks once a second, for the running line's elapsed time. */
function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  return now;
}

/** The turn has no time limit, so the running line carries the owner's way to stop it. */
function RunningLine({ turn }: { turn: AssistantTurn }) {
  const now = useNow();
  const [stopping, setStopping] = useState(false);
  const elapsed = Math.max(0, now - Date.parse(turn.startedAt));
  const time = formatDuration(elapsed);
  const words = t.assistant.tools;
  const stop = () => {
    setStopping(true);
    stopAssistantTurn(turn.id).catch(() => setStopping(false));
  };
  return (
    <li className="flex items-center gap-1.5">
      <Spinner aria-hidden="true" className="size-3" />
      <span className="tabular-nums">
        {elapsed >= longRunMs ? words.workingLong(time) : words.working(time)}
      </span>
      <Button
        variant="ghost"
        size="sm"
        className="ml-auto h-6 px-2 text-xs"
        disabled={stopping}
        onClick={stop}
      >
        {words.stop}
      </Button>
    </li>
  );
}

function Lines({ turn }: { turn: AssistantTurn }) {
  const running = turn.status === "running";
  const last = turn.progress.length - 1;
  return (
    <ul
      aria-label={t.assistant.tools.list}
      className="flex flex-col gap-1 text-xs text-muted-foreground"
    >
      {turn.progress.map((item, index) => (
        <ToolLine
          // Items are only ever appended, so the position names one.
          key={index}
          item={item}
          turn={turn}
          current={running && index === last}
        />
      ))}
      {running && <RunningLine turn={turn} />}
    </ul>
  );
}

function Folded({ turn }: { turn: AssistantTurn }) {
  const [open, setOpen] = useState(false);
  const words = t.assistant.tools;
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="flex flex-col gap-1">
      <CollapsibleTrigger asChild>
        <Button
          variant="link"
          size="sm"
          className="h-auto gap-1.5 self-start p-0 text-xs has-[>svg]:px-0 font-normal text-muted-foreground [&_svg]:size-3"
        >
          <ChevronRight
            aria-hidden="true"
            className={cn("transition-transform", open && "rotate-90")}
          />
          {open ? words.hide : words.show(turn.progress.length)}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <Lines turn={turn} />
      </CollapsibleContent>
    </Collapsible>
  );
}

/**
 * The turn's tool calls, quieter than its answer: open while it runs, and when it stopped, so the
 * owner sees what was tried; folded to one line once it answered.
 */
export default function ToolCalls({ turn }: { turn: AssistantTurn }) {
  if (turn.status === "running") return <Lines turn={turn} />;
  if (turn.progress.length === 0) return null;
  if (turn.status === "stopped" || turn.status === "interrupted") {
    return <Lines turn={turn} />;
  }
  return <Folded turn={turn} />;
}
