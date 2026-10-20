import { type ReactNode, useId, useState } from "react";

import type {
  PersonPluginFetchedPerson,
  PersonPluginItem,
} from "../../../api/personPluginFetches";
import { t } from "../../../i18n";
import { cn } from "../../../lib/cn";
import { Cell, CompareHeadings } from "../../../player/workInfoFetch/CompareStep";
import TagChips, { type ChipItem } from "../../../player/workInfoFetch/TagChips";
import { Button } from "../../../ui/shadcn/button";
import { Checkbox } from "../../../ui/shadcn/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../../ui/shadcn/collapsible";
import { Label } from "../../../ui/shadcn/label";
import { PersonFace } from "../../../ui/PersonFace";
import { Bio, LinkList } from "../PersonProfile";
import { useLocalToday } from "../useLocalToday";
import {
  aliasOnPerson,
  compareRows,
  type CompareRow,
  idOnPerson,
  newAliases,
  newIds,
  type PersonCompareCurrent,
  profileFieldOf,
  profileValueText,
} from "./personCompareRows";

const text = t.credits.personPage.pluginFetch;

type Tone = "now" | "fetched";

export interface PersonCompareProps {
  current: PersonCompareCurrent;
  fetched: PersonPluginFetchedPerson;
  /** The person's face, for a kept face row. */
  nowFaceSrc?: string;
  fetchedFaceSrc?: string;
  selection: readonly PersonPluginItem[];
  /** While the apply runs or the result is stale. */
  locked: boolean;
  /** Offer stored values for replacement, as the person page's apply writes over them. */
  replace?: boolean;
  /** In a lookup block: no column headings, since the comparison around it has them, and each row's label above its two cells. */
  inBlock?: boolean;
  onToggle: (item: PersonPluginItem, on: boolean) => void;
}

function Muted({ children }: { children: ReactNode }) {
  return <span className="text-muted-foreground">{children}</span>;
}

function FaceValue({ src, tone }: { src: string | undefined; tone: Tone }) {
  return (
    <>
      <div className="w-16">
        <PersonFace shape="profile" name="" src={src} />
      </div>
      {tone === "now" && src === undefined && <Muted>{text.noFace}</Muted>}
    </>
  );
}

function aliasChips(
  props: PersonCompareProps,
  tone: Tone,
): { items: ChipItem[]; fresh: number } {
  const { current, fetched } = props;
  if (tone === "now")
    return {
      items: current.aliases.map((alias) => ({ name: alias.name, muted: true })),
      fresh: 0,
    };
  return {
    items: fetched.aliases.map((alias) => ({
      name: alias.name,
      mark: aliasOnPerson(current, alias) ? "onPerson" : undefined,
    })),
    fresh: newAliases(current, fetched),
  };
}

function idChips(
  props: PersonCompareProps,
  tone: Tone,
): { items: ChipItem[]; fresh: number } {
  const { current, fetched } = props;
  const ids = tone === "now" ? current.externalIds : fetched.externalIds;
  return {
    items: ids.map((id) => ({
      key: `${id.source}\u0000${id.value}`,
      name: t.credits.personPage.externalId(id.source, id.value),
      numeric: true,
      muted: tone === "now",
      mark: tone === "fetched" && idOnPerson(current, id) ? "onPerson" : undefined,
    })),
    fresh: tone === "now" ? 0 : newIds(current, fetched),
  };
}

function ProfileValue({
  item,
  props,
  tone,
}: {
  item: PersonPluginItem;
  props: PersonCompareProps;
  tone: Tone;
}) {
  const today = useLocalToday();
  const field = profileFieldOf(item);
  if (field === undefined) return null;
  const fetchedProfile = props.fetched.profile ?? {};
  const [profile, context] =
    tone === "now"
      ? [props.current.profile, fetchedProfile]
      : [fetchedProfile, props.current.profile];
  if (profile[field] === undefined) return <Muted>{text.empty}</Muted>;
  if (field === "bio") return <Bio bio={profile.bio ?? ""} />;
  if (field === "links") return <LinkList links={profile.links ?? []} />;
  return (
    <span
      className={cn(
        "min-w-0 tabular-nums wrap-anywhere",
        tone === "now" ? "text-muted-foreground" : "text-foreground",
      )}
    >
      {profileValueText(field, profile, context, today)}
    </span>
  );
}

function Value({
  item,
  props,
  tone,
}: {
  item: PersonPluginItem;
  props: PersonCompareProps;
  tone: Tone;
}) {
  if (item === "image")
    return (
      <FaceValue
        src={tone === "now" ? props.nowFaceSrc : props.fetchedFaceSrc}
        tone={tone}
      />
    );
  if (item === "aliases" || item === "externalIds") {
    const { items, fresh } =
      item === "aliases" ? aliasChips(props, tone) : idChips(props, tone);
    if (items.length === 0) return <Muted>{text.empty}</Muted>;
    return (
      <TagChips
        items={items}
        trailing={fresh > 0 ? t.player.workInfoFetch.newCount(fresh) : undefined}
      />
    );
  }
  return <ProfileValue item={item} props={props} tone={tone} />;
}

function Row({
  row,
  props,
  selectable,
}: {
  row: CompareRow;
  props: PersonCompareProps;
  selectable: boolean;
}) {
  const id = useId();
  const label = text.items[row.item];
  const heading = t.player.workInfoFetch;
  const narrow = props.inBlock === true;
  return (
    <div
      className={cn(
        "grid gap-x-4 gap-y-1 py-2",
        narrow ? "sm:grid-cols-2" : "sm:grid-cols-5",
      )}
    >
      <div
        className={cn("flex items-center gap-2 self-start", narrow && "sm:col-span-2")}
      >
        {selectable ? (
          <>
            <Checkbox
              id={id}
              checked={props.selection.includes(row.item)}
              disabled={props.locked}
              onCheckedChange={(checked) => props.onToggle(row.item, checked === true)}
            />
            <Label htmlFor={id} className="font-normal text-muted-foreground">
              {label}
              {row.replaces === true && <span className="text-xs">{text.replaces}</span>}
            </Label>
          </>
        ) : (
          <span className="text-muted-foreground">{label}</span>
        )}
      </div>
      <Cell heading={heading.now} narrow={narrow}>
        <Value item={row.item} props={props} tone="now" />
      </Cell>
      <Cell heading={heading.fetched} narrow={narrow}>
        <Value item={row.item} props={props} tone="fetched" />
      </Cell>
    </div>
  );
}

function KeptRows({
  rows,
  props,
}: {
  rows: readonly CompareRow[];
  props: PersonCompareProps;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="flex flex-col gap-1">
      <CollapsibleTrigger asChild>
        <Button
          variant="link"
          size="sm"
          className="h-auto self-start px-0 py-0 text-muted-foreground hover:text-primary"
        >
          {open ? text.hideKept : text.kept(rows.length)}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-1">
        <p className="text-xs text-muted-foreground">
          {props.replace === true ? text.sameNote : text.keptNote}
        </p>
        <div className="flex flex-col divide-y divide-border border-y border-border">
          {rows.map((row) => (
            <Row key={row.item} row={row} props={props} selectable={false} />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

/**
 * A fetched person beside the person it would fill: one selectable row per item the apply would
 * write, and the values the apply would keep folded.
 */
export default function PersonCompare(props: PersonCompareProps) {
  const rows = compareRows(props.current, props.fetched, props.replace === true);
  const changes = rows.filter((row) => row.changes);
  const kept = rows.filter((row) => !row.changes);
  return (
    <div className="flex flex-col gap-2 text-sm">
      {changes.length === 0 ? (
        <p className="text-muted-foreground">{text.nothingNew}</p>
      ) : (
        <div className="flex flex-col">
          {props.inBlock !== true && <CompareHeadings />}
          <div className="flex flex-col divide-y divide-border border-y border-border">
            {changes.map((row) => (
              <Row key={row.item} row={row} props={props} selectable />
            ))}
          </div>
        </div>
      )}
      {kept.length > 0 && <KeptRows rows={kept} props={props} />}
    </div>
  );
}
