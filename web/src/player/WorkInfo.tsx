import { ExternalLink } from "lucide-react";
import { Fragment, type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  type WorkInfo as WorkInfoValue,
  type WorkOrganizationRole,
  type WorkPersonRole,
  workDateKinds,
  workOrganizationRoles,
  workPersonRoles,
} from "../api/workInfo";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { libraryFilterPath } from "../library/libraryPath";
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

export function hasWorkInfo(info: WorkInfoValue | undefined): info is WorkInfoValue {
  if (info === undefined) return false;
  return (
    info.code !== undefined ||
    info.description !== undefined ||
    info.dates.length > 0 ||
    info.people.length > 0 ||
    info.organizations.length > 0 ||
    info.urls.length > 0 ||
    info.externalIds.length > 0
  );
}

export function Item({ label, children }: { label: UiText; children: ReactNode }) {
  return (
    <li className="flex max-w-full min-w-0 flex-wrap items-baseline gap-x-1.5">
      <span className="text-muted-foreground">{label}</span>
      {children}
    </li>
  );
}

function CreditLink({
  kind,
  credit,
}: {
  kind: "person" | "organization";
  credit: { id: number; name: string };
}) {
  return (
    <Link
      to={libraryFilterPath(kind, credit.id)}
      aria-label={t.player.workInfo.showVideosWith(credit.name)}
      className="text-foreground hover:text-primary"
    >
      {credit.name}
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

function ListItem({
  label,
  entries,
  limit = foldLimit,
}: {
  label: UiText;
  entries: readonly { key: string; node: ReactNode }[];
  limit?: number;
}) {
  const fold = useFold<HTMLSpanElement>(entries.length, limit);
  const shown = fold.folded ? entries.slice(0, limit) : entries;

  return (
    <Item label={label}>
      <span className="min-w-0 wrap-anywhere">
        {shown.map((entry, index) => (
          <Fragment key={entry.key}>
            {index > 0 && ", "}
            <span
              ref={index === limit ? fold.nextRef : undefined}
              tabIndex={index === limit ? -1 : undefined}
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
    </Item>
  );
}

/** Measure overflow and remeasure on resize; callers remount for another video to reset expansion. */
export function Description({
  text,
  tone = "default",
  label,
}: {
  text: string;
  tone?: "default" | "muted";
  label?: UiText;
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
          "max-w-prose text-sm wrap-anywhere whitespace-pre-line",
          tone === "muted" ? "text-muted-foreground" : "text-foreground",
          !open && "line-clamp-3",
        )}
      >
        {label !== undefined && <span className="mr-1.5">{label}</span>}
        {text}
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

/** Render only populated fields; keep stored date strings unchanged rather than formatting them. */
export default function WorkInfo({ info }: { info: WorkInfoValue | undefined }) {
  if (!hasWorkInfo(info)) return null;
  const w = t.player.workInfo;
  const dates = workDateKinds.flatMap((kind) => {
    const date = info.dates.find((entry) => entry.kind === kind);
    return date === undefined ? [] : [date];
  });
  const people = workPersonRoles.flatMap((role) => {
    const names = info.people.filter((entry) => entry.role === role);
    return names.length === 0 ? [] : [{ role, names }];
  });
  const organizations = workOrganizationRoles.flatMap((role) => {
    const names = info.organizations.filter((entry) => entry.role === role);
    return names.length === 0 ? [] : [{ role, names }];
  });
  const hasRow =
    info.code !== undefined ||
    dates.length > 0 ||
    people.length > 0 ||
    organizations.length > 0 ||
    info.urls.length > 0 ||
    info.externalIds.length > 0;

  return (
    <>
      {hasRow && (
        <ul
          aria-label={w.label}
          className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm sm:gap-x-5"
        >
          {info.code !== undefined && (
            <Item label={w.code}>
              <span className="min-w-0 tabular-nums wrap-anywhere">{info.code}</span>
            </Item>
          )}
          {dates.map((date) => (
            <Item key={date.kind} label={dateLabel(date.kind)}>
              <span className="tabular-nums">{date.date}</span>
            </Item>
          ))}
          {people.map(({ role, names }) => (
            <ListItem
              key={role}
              label={personRoleLabel(role, names.length)}
              entries={names.map((entry) => ({
                key: String(entry.person.id),
                node: <CreditLink kind="person" credit={entry.person} />,
              }))}
            />
          ))}
          {organizations.map(({ role, names }) => (
            <ListItem
              key={role}
              label={organizationRoleLabel(role, names.length)}
              entries={names.map((entry) => ({
                key: String(entry.organization.id),
                node: <CreditLink kind="organization" credit={entry.organization} />,
              }))}
            />
          ))}
          {info.urls.length > 0 && (
            <ListItem
              label={w.links}
              entries={info.urls.map((url) => ({
                key: url,
                node: (
                  <a
                    href={url}
                    title={url}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex max-w-xs items-center gap-1 align-bottom hover:text-primary"
                  >
                    <span className="truncate">{withoutScheme(url)}</span>
                    <ExternalLink className="size-3 shrink-0" aria-hidden="true" />
                  </a>
                ),
              }))}
            />
          )}
          {info.externalIds.length > 0 && (
            <ListItem
              label={w.ids}
              entries={info.externalIds.map((id) => ({
                key: `${id.source}\u0000${id.value}`,
                node: (
                  <span className="tabular-nums">
                    {w.externalId(id.source, id.value)}
                  </span>
                ),
              }))}
            />
          )}
        </ul>
      )}
      {info.description !== undefined && <Description text={info.description} />}
    </>
  );
}
