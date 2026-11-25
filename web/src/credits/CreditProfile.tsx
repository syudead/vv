import { ExternalLink } from "lucide-react";
import { type ReactNode, useLayoutEffect, useRef, useState } from "react";

import { type CreditName, type ExternalId, otherNames } from "../api/credits";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { type Fact, FactList } from "../ui/patterns/fact-list";
import { Button } from "../ui/shadcn/button";

// The profile parts the person page and the organization page share.

const text = t.credits.personPage;

/** The display name as the page's `h1`, with the other names muted under it. */
export function CreditHeading({
  name,
  names,
  muted = false,
}: {
  name: string;
  names: readonly CreditName[];
  /** Draws the name muted, for a person without one of their own; hides the other names. */
  muted?: boolean;
}) {
  return (
    <div className="flex flex-col gap-1">
      <h1
        className={cn(
          "line-clamp-3 text-xl font-semibold break-words",
          muted && "text-muted-foreground",
        )}
      >
        {name}
      </h1>
      {!muted && names.length > 1 && (
        <p className="text-sm break-words text-muted-foreground">
          {text.aliases(otherNames({ names }).map((entry) => entry.name))}
        </p>
      )}
    </div>
  );
}

/** Six lines, then Show more only when the text is longer than that. */
export function Bio({ bio }: { bio: string }) {
  const ref = useRef<HTMLParagraphElement>(null);
  const [open, setOpen] = useState(false);
  const [overflows, setOverflows] = useState(false);
  useLayoutEffect(() => {
    const node = ref.current;
    if (node === null || open) return;
    const measure = () => setOverflows(node.scrollHeight > node.clientHeight);
    measure();
    // A narrower column wraps the same text onto more lines.
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [bio, open]);
  return (
    <div className="flex flex-col items-start gap-1">
      <p
        ref={ref}
        className={cn("text-sm break-words whitespace-pre-line", !open && "line-clamp-6")}
      >
        {bio}
      </p>
      {(overflows || open) && (
        <Button variant="link" size="sm" onClick={() => setOpen((value) => !value)}>
          {open ? text.showLess : text.showMore}
        </Button>
      )}
    </div>
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

/** One host per line, each opening its page in a new tab. */
export function LinkList({ links }: { links: readonly string[] }) {
  return (
    <ul aria-label={text.links} className="flex min-w-0 flex-col gap-1 text-sm">
      {links.map((url) => (
        <li key={url} className="min-w-0">
          <a
            href={url}
            target="_blank"
            rel="noreferrer"
            className="inline-flex max-w-full items-center gap-1 text-primary hover:underline"
          >
            <span className="truncate">{hostOf(url)}</span>
            <ExternalLink aria-hidden="true" className="size-3 shrink-0" />
            <span className="sr-only">{text.opensInNewTab}</span>
          </a>
        </li>
      ))}
    </ul>
  );
}

/**
 * The profile card: the facts, then links and external IDs. With `empty` (the record has no profile
 * at all) it offers `edit` instead; undefined when it has nothing left to show.
 */
export function creditFactsCard({
  facts,
  links,
  externalIds,
  empty,
  edit,
}: {
  facts: Fact[];
  links: readonly string[];
  externalIds: readonly ExternalId[];
  empty: boolean;
  edit: ReactNode;
}): ReactNode {
  if (!empty && facts.length === 0 && links.length === 0 && externalIds.length === 0)
    return undefined;
  return (
    <>
      {empty && (
        <p className="flex flex-wrap items-center gap-2 text-muted-foreground">
          {text.noProfile}
          {edit}
        </p>
      )}
      {facts.length > 0 && <FactList facts={facts} />}
      {links.length > 0 && <LinkList links={links} />}
      {externalIds.length > 0 && (
        <ul
          aria-label={text.externalIds}
          className="flex flex-col gap-1 text-xs break-words text-muted-foreground"
        >
          {externalIds.map((id) => (
            <li key={`${id.source}\0${id.value}`}>
              {text.externalId(id.source, id.value)}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
