import { Ellipsis, ExternalLink, Merge, Trash2 } from "lucide-react";
import { type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";

import type { Person } from "../../api/credits";
import { t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { personEditPath } from "../../lib/personEditPath";
import { isPlainClick } from "../../lib/plainClick";
import { libraryFilterPath } from "../../library/libraryPath";
import { FactList } from "../../ui/patterns/fact-list";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { kindTexts } from "../creditRows";
import { creditDisplayName } from "../displayName";
import { personFactGroups } from "./personFacts";
import { handPerson } from "./personHandoff";

const text = t.credits.personPage;

/**
 * Opens the edit page with this person, which returns along the history when it is left. A click
 * that opens another tab hands nothing, so this tab's next visit to the person reads it again.
 */
function EditLink({ person, label = text.edit }: { person: Person; label?: UiText }) {
  return (
    <Link
      to={personEditPath(person.id)}
      state={{ fromPage: true }}
      onClick={(event) => {
        if (isPlainClick(event)) handPerson(person);
      }}
    >
      {label}
    </Link>
  );
}

/** The name, aliases and actions; nothing on the page outranks viewing, so no default button. */
export function PersonIdentity({
  person,
  fetchEntry,
  onMerge,
  onDelete,
}: {
  person: Person;
  /** Fetch info…, after Edit; absent while no approved person plugin exists. */
  fetchEntry?: ReactNode;
  onMerge: () => void;
  onDelete: () => void;
}) {
  return (
    <>
      <h1
        className={cn(
          "line-clamp-3 text-xl font-semibold break-words",
          person.provisional && "text-muted-foreground",
        )}
      >
        {creditDisplayName(person)}
      </h1>
      {!person.provisional && person.aliases.length > 0 && (
        <p className="text-sm break-words text-muted-foreground">
          {text.aliases(person.aliases.map((alias) => alias.name))}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {/* The edit page focuses the name input, so Name… is Edit for a person without one. */}
        <Button asChild variant="outline" size="sm">
          <EditLink person={person} label={person.provisional ? text.name : text.edit} />
        </Button>
        {fetchEntry}
        <Button asChild variant="ghost" size="sm">
          <Link to={libraryFilterPath("person", person.id)}>{text.showInLibrary}</Link>
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={text.more}>
              <Ellipsis aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={onMerge}>
              <Merge aria-hidden="true" />
              {kindTexts("person").mergeInto}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={onDelete}>
              <Trash2 aria-hidden="true" />
              {text.delete}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </>
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

/** Candidates, fact groups with values, faces in videos, then links and external IDs. */
export function PersonDetails({
  person,
  today,
  candidates,
  faces,
}: {
  person: Person;
  today: Date;
  candidates?: ReactNode;
  faces?: ReactNode;
}) {
  const groups = personFactGroups(person.profile, today);
  const links = person.profile.links ?? [];
  return (
    <>
      {candidates}
      {groups.length === 0 ? (
        <p className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          {text.noProfile}
          <Button asChild variant="link" size="sm">
            <EditLink person={person} />
          </Button>
        </p>
      ) : (
        groups.map((group) => (
          <section key={group.id} className="flex flex-col gap-2 text-sm">
            <h2 className="text-sm font-semibold">{group.heading}</h2>
            {group.facts.length > 0 && <FactList facts={group.facts} />}
            {group.bio !== undefined && <Bio bio={group.bio} />}
          </section>
        ))
      )}
      {faces}
      {links.length > 0 && <LinkList links={links} />}
      {person.externalIds.length > 0 && (
        <ul
          aria-label={text.externalIds}
          className="flex flex-col gap-1 text-xs break-words text-muted-foreground"
        >
          {person.externalIds.map((id) => (
            <li key={`${id.source}\0${id.value}`}>
              {text.externalId(id.source, id.value)}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
