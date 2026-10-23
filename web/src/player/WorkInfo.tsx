import { ExternalLink } from "lucide-react";
import { Fragment, type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { personImagePath } from "../api/credits";
import {
  type WorkInfo as WorkInfoValue,
  type WorkOrganizationRole,
  type WorkPersonRole,
  workDateKinds,
  workOrganizationRoles,
  workPersonRoles,
} from "../api/workInfo";
import { creditDisplayName } from "../credits/displayName";
import { personPath } from "../credits/person/personPath";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { libraryFilterPath } from "../library/libraryPath";
import { PersonFace } from "../ui/PersonFace";
import { type Fact, FactList } from "../ui/patterns/fact-list";
import { Button } from "../ui/shadcn/button";

const foldLimit = 3;

const foldButton = "h-auto px-0 py-0 text-muted-foreground hover:text-primary";

function dateLabel(kind: (typeof workDateKinds)[number]): UiText {
  const w = t.player.workInfo;
  if (kind === "release") return w.released;
  if (kind === "recorded") return w.recorded;
  return w.published;
}

function personRoleLabel(role: WorkPersonRole, count: number): UiText {
  const w = t.player.workInfo;
  if (role === "performer") return w.performer(count);
  if (role === "director") return w.director(count);
  if (role === "photographer") return w.photographer(count);
  return w.streamer(count);
}

function organizationRoleLabel(role: WorkOrganizationRole, count: number): UiText {
  const w = t.player.workInfo;
  return role === "maker" ? w.maker(count) : w.labelRole(count);
}

export function withoutScheme(url: string): string {
  return url.replace(/^https?:\/\//i, "");
}

export function Item({ label, children }: { label: UiText; children: ReactNode }) {
  return (
    <li className="flex max-w-full min-w-0 flex-wrap items-baseline gap-x-1.5">
      <span className="text-muted-foreground">{label}</span>
      {children}
    </li>
  );
}

function OrganizationLink({
  organization,
}: {
  organization: { id: number; name: string };
}) {
  return (
    <Link
      to={libraryFilterPath("organization", organization.id)}
      aria-label={t.player.workInfo.showVideosWith(organization.name)}
      className="text-foreground hover:text-primary"
    >
      {organization.name}
    </Link>
  );
}

/** The chip face is as tall as the `text-sm` line, so `align-top` keeps the row height. */
function PersonLink({ entry }: { entry: WorkInfoValue["people"][number] }) {
  const { person, imageRevision } = entry;
  const name = creditDisplayName(person);
  return (
    <Link
      to={personPath(person.id)}
      aria-label={t.player.workInfo.openPerson(name)}
      className="group inline-flex max-w-full items-center gap-1 align-top text-foreground hover:text-primary"
    >
      <PersonFace
        shape="chip"
        name={name}
        src={
          imageRevision === undefined
            ? undefined
            : personImagePath(person.id, imageRevision)
        }
      />
      <span
        className={cn(
          "min-w-0 wrap-anywhere",
          person.provisional && "text-muted-foreground group-hover:text-primary",
        )}
      >
        {name}
      </span>
    </Link>
  );
}

/** Expanding focuses the first hidden item's link, if present; expansion is not persisted. */
export function useFold<T extends HTMLElement>(count: number, limit: number) {
  const [open, setOpen] = useState(false);
  const nextRef = useRef<T | null>(null);
  const focusNext = useRef(false);
  const foldable = count > limit;
  const folded = !open && foldable;

  useLayoutEffect(() => {
    if (!open || !focusNext.current) return;
    focusNext.current = false;
    const entry = nextRef.current;
    (entry?.querySelector<HTMLElement>("a[href]") ?? entry)?.focus();
  }, [open]);

  return {
    foldable,
    folded,
    nextRef,
    toggle: () => {
      focusNext.current = folded;
      setOpen(folded);
    },
  };
}

export function FoldButton({
  folded,
  count,
  limit,
  label,
  onToggle,
}: {
  folded: boolean;
  count: number;
  limit: number;
  label: UiText;
  onToggle: () => void;
}) {
  return (
    <Button
      variant="link"
      size="sm"
      className={foldButton}
      aria-label={folded ? t.player.workInfo.showAll(count, label) : undefined}
      onClick={onToggle}
    >
      {folded ? t.player.workInfo.more(count - limit) : t.player.workInfo.showFewer}
    </Button>
  );
}

/** Entries joined by commas, the ones past `limit` folded behind `+{n} more`. */
function FoldedEntries({
  label,
  entries,
  limit = foldLimit,
  lines = false,
}: {
  label: UiText;
  entries: readonly { key: string; node: ReactNode }[];
  limit?: number;
  /** One entry per line instead of joined by commas. */
  lines?: boolean;
}) {
  const fold = useFold<HTMLSpanElement>(entries.length, limit);
  const shown = fold.folded ? entries.slice(0, limit) : entries;

  return (
    <span className={cn("min-w-0 wrap-anywhere", lines && "flex flex-col items-start")}>
      {shown.map((entry, index) => (
        <Fragment key={entry.key}>
          {index > 0 && !lines && ", "}
          <span
            ref={index === limit ? fold.nextRef : undefined}
            tabIndex={index === limit ? -1 : undefined}
            // A line is a flex item, which would otherwise grow to its content past the column.
            className={lines ? "max-w-full" : undefined}
          >
            {entry.node}
          </span>
        </Fragment>
      ))}
      {fold.foldable && " "}
      {fold.foldable && (
        <FoldButton
          folded={fold.folded}
          count={entries.length}
          limit={limit}
          label={label}
          onToggle={fold.toggle}
        />
      )}
    </span>
  );
}

/**
 * HTML line breaks as line feeds, for text saved before the server turned them into line feeds;
 * other markup stays literal text.
 */
export function withLineBreaks(text: string): string {
  return text.replace(/<br\s*\/?>/gi, "\n");
}

/** Measure overflow and remeasure on resize; callers remount for another video to reset expansion. */
export function Description({
  text,
  tone = "default",
  label,
  lines = "few",
}: {
  text: string;
  tone?: "default" | "muted";
  label?: UiText;
  /** `many` folds only text past eight lines, for a description that is a panel's main reading. */
  lines?: "few" | "many";
}) {
  const ref = useRef<HTMLParagraphElement | null>(null);
  const [open, setOpen] = useState(false);
  const [overflows, setOverflows] = useState(false);

  useLayoutEffect(() => {
    if (open) return;
    const element = ref.current;
    if (element === null) return;
    const measure = () => setOverflows(element.scrollHeight > element.clientHeight + 1);
    measure();
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, [open, text]);

  return (
    <div className="flex flex-col items-start gap-1">
      <p
        ref={ref}
        className={cn(
          "text-sm leading-relaxed wrap-anywhere whitespace-pre-line",
          tone === "muted" ? "text-muted-foreground" : "text-foreground",
          !open && (lines === "many" ? "line-clamp-8" : "line-clamp-3"),
        )}
      >
        {label !== undefined && <span className="mr-1.5">{label}</span>}
        {withLineBreaks(text)}
      </p>
      {(open || overflows) && (
        <Button
          variant="link"
          size="sm"
          className={foldButton}
          onClick={() => setOpen(!open)}
        >
          {open ? t.player.workInfo.showLess : t.player.workInfo.showMore}
        </Button>
      )}
    </div>
  );
}

interface Entry {
  key: string;
  node: ReactNode;
}

function listFact(id: string, label: UiText, entries: Entry[], lines = false): Fact {
  return {
    id,
    term: label,
    value: <FoldedEntries label={label} entries={entries} lines={lines} />,
  };
}

function peopleFacts(info: WorkInfoValue): Fact[] {
  // Performers are in Overview's summary and in People.
  return workPersonRoles.flatMap((role) => {
    const names = info.people.filter((entry) => entry.role === role);
    if (role === "performer" || names.length === 0) return [];
    return [
      listFact(
        role,
        personRoleLabel(role, names.length),
        names.map((entry) => ({
          key: String(entry.person.id),
          node: <PersonLink entry={entry} />,
        })),
      ),
    ];
  });
}

function organizationFacts(info: WorkInfoValue): Fact[] {
  return workOrganizationRoles.flatMap((role) => {
    const names = info.organizations.filter((entry) => entry.role === role);
    if (names.length === 0) return [];
    return [
      listFact(
        role,
        organizationRoleLabel(role, names.length),
        names.map((entry) => ({
          key: String(entry.organization.id),
          node: <OrganizationLink organization={entry.organization} />,
        })),
      ),
    ];
  });
}

function linkEntry(url: string): Entry {
  return {
    key: url,
    node: (
      <a
        href={url}
        title={url}
        target="_blank"
        rel="noreferrer"
        className="inline-flex max-w-full items-center gap-1 align-bottom text-primary hover:underline"
      >
        <span className="truncate">{withoutScheme(url)}</span>
        <ExternalLink className="size-3 shrink-0" aria-hidden="true" />
      </a>
    ),
  };
}

function linkFacts(info: WorkInfoValue): Fact[] {
  const w = t.player.workInfo;
  const facts: Fact[] = [];
  if (info.urls.length > 0)
    facts.push(listFact("links", w.links, info.urls.map(linkEntry), true));
  if (info.externalIds.length > 0) {
    facts.push(
      listFact(
        "ids",
        w.ids,
        info.externalIds.map((id) => ({
          key: `${id.source}\u0000${id.value}`,
          node: <span className="tabular-nums">{w.externalId(id.source, id.value)}</span>,
        })),
      ),
    );
  }
  return facts;
}

/**
 * The work's items other than the description and the performers. Dates keep their stored
 * strings rather than being formatted.
 */
export function workFacts(info: WorkInfoValue | undefined): Fact[] {
  if (info === undefined) return [];
  const facts: Fact[] = [];
  if (info.code !== undefined) {
    facts.push({
      id: "code",
      term: t.player.workInfo.code,
      value: <span className="tabular-nums wrap-anywhere">{info.code}</span>,
    });
  }
  for (const kind of workDateKinds) {
    const date = info.dates.find((entry) => entry.kind === kind);
    if (date === undefined) continue;
    facts.push({
      id: kind,
      term: dateLabel(kind),
      value: <span className="tabular-nums">{date.date}</span>,
    });
  }
  return [...facts, ...peopleFacts(info), ...organizationFacts(info), ...linkFacts(info)];
}

/** The Work tab's list; nothing when the work has no item to list. */
export default function WorkInfo({ info }: { info: WorkInfoValue | undefined }) {
  const facts = workFacts(info);
  return facts.length === 0 ? null : <FactList facts={facts} columns={2} />;
}
