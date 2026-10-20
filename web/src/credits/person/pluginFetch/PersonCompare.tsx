import { ArrowRight } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import type {
  PersonPluginFetchedPerson,
  PersonPluginItem,
} from "../../../api/personPluginFetches";
import { t } from "../../../i18n";
import { cn } from "../../../lib/cn";
import TagChips, { type ChipItem } from "../../../player/workInfoFetch/TagChips";
import { Button } from "../../../ui/shadcn/button";
import { Checkbox } from "../../../ui/shadcn/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../../ui/shadcn/collapsible";
import { Label } from "../../../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../../../ui/shadcn/radio-group";
import { PersonFace } from "../../../ui/PersonFace";
import { Bio, LinkList } from "../PersonProfile";
import { useLocalToday } from "../useLocalToday";
import {
  aliasOnPerson,
  compareRows,
  type CompareRow,
  faceChoices,
  idOnPerson,
  newAliases,
  newIds,
  type PersonCompareCurrent,
  profileFieldOf,
  profileValueText,
  type RowItem,
} from "./personCompareRows";

const text = t.credits.personPage.pluginFetch;

type Tone = "now" | "fetched";

export interface PersonCompareProps {
  current: PersonCompareCurrent;
  fetched: PersonPluginFetchedPerson;
  /** The person's face, shown as the choice that keeps it. */
  nowFaceSrc?: string;
  /** The URL of the fetched image at an index below the result's image count. */
  imageSrc: (index: number) => string;
  selection: readonly PersonPluginItem[];
  /** The image chosen as the face, or null to keep the person's. */
  image: number | null;
  /** While the apply runs or the result is stale. */
  locked: boolean;
  /** Offer stored values and the face for replacement, as the person page's apply writes over them. */
  replace?: boolean;
  /** In a lookup block, which is narrower than the dialog: rows stack their cells. */
  inBlock?: boolean;
  onToggle: (item: PersonPluginItem, on: boolean) => void;
  onImage: (index: number | null) => void;
}

function Muted({ children }: { children: ReactNode }) {
  return <span className="text-muted-foreground">{children}</span>;
}

function aliasChips(props: PersonCompareProps, tone: Tone): ChipItem[] {
  const { current, fetched } = props;
  if (tone === "now")
    return current.aliases.map((alias) => ({ name: alias.name, muted: true }));
  return fetched.aliases.map((alias) => ({
    name: alias.name,
    mark: aliasOnPerson(current, alias) ? "onPerson" : undefined,
  }));
}

function idChips(props: PersonCompareProps, tone: Tone): ChipItem[] {
  const { current, fetched } = props;
  const ids = tone === "now" ? current.externalIds : fetched.externalIds;
  return ids.map((id) => ({
    key: `${id.source}\u0000${id.value}`,
    name: t.credits.personPage.externalId(id.source, id.value),
    numeric: true,
    muted: tone === "now",
    mark: tone === "fetched" && idOnPerson(current, id) ? "onPerson" : undefined,
  }));
}

function ListValue({
  item,
  props,
  tone,
}: {
  item: "aliases" | "externalIds";
  props: PersonCompareProps;
  tone: Tone;
}) {
  const items = item === "aliases" ? aliasChips(props, tone) : idChips(props, tone);
  if (items.length === 0) return <Muted>{text.empty}</Muted>;
  const fresh =
    tone === "now"
      ? 0
      : (item === "aliases" ? newAliases : newIds)(props.current, props.fetched).length;
  return (
    <TagChips
      items={items}
      trailing={fresh > 0 ? t.player.workInfoFetch.newCount(fresh) : undefined}
    />
  );
}

function Value({
  item,
  props,
  tone,
  struck = false,
}: {
  item: RowItem;
  props: PersonCompareProps;
  tone: Tone;
  struck?: boolean;
}) {
  const today = useLocalToday();
  if (item === "aliases" || item === "externalIds")
    return <ListValue item={item} props={props} tone={tone} />;
  const field = profileFieldOf(item);
  if (field === undefined) return null;
  const fetchedProfile = props.fetched.profile ?? {};
  const [profile, context] =
    tone === "now"
      ? [props.current.profile, fetchedProfile]
      : [fetchedProfile, props.current.profile];
  if (profile[field] === undefined) return <Muted>{text.empty}</Muted>;
  const quiet = cn(tone === "now" && "text-muted-foreground", struck && "line-through");
  if (field === "bio")
    return (
      <div className={quiet}>
        <Bio bio={profile.bio ?? ""} />
      </div>
    );
  if (field === "links")
    return (
      <div className={quiet}>
        <LinkList links={profile.links ?? []} />
      </div>
    );
  return (
    <span className={cn("min-w-0 tabular-nums wrap-anywhere", quiet)}>
      {profileValueText(field, profile, context, today)}
    </span>
  );
}

function RowLabel({
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
  if (!selectable) return <span className="text-muted-foreground">{label}</span>;
  return (
    <div className="flex items-center gap-2 self-start">
      <Checkbox
        id={id}
        checked={props.selection.includes(row.item)}
        disabled={props.locked}
        onCheckedChange={(checked) => props.onToggle(row.item, checked === true)}
      />
      <Label htmlFor={id} className="font-normal">
        {label}
      </Label>
    </div>
  );
}

/** Stacked under a checkbox, a value lines up with the label's text. */
function indent(props: PersonCompareProps): string {
  return cn("pl-6", props.inBlock !== true && "sm:pl-0");
}

/** A filled row shows only the fetched value, since the person has none to compare it with. */
function FillRow({ row, props }: { row: CompareRow; props: PersonCompareProps }) {
  return (
    <div
      className={cn(
        "grid items-baseline gap-x-4 gap-y-1 py-2",
        props.inBlock !== true && "sm:grid-cols-compare-fill",
      )}
    >
      <RowLabel row={row} props={props} selectable />
      <div className={cn("min-w-0", indent(props))}>
        <Value item={row.item} props={props} tone="fetched" />
      </div>
    </div>
  );
}

/** A replacing or kept row reads left to right: now, then what the apply would write. */
function PairRow({
  row,
  props,
  selectable,
}: {
  row: CompareRow;
  props: PersonCompareProps;
  selectable: boolean;
}) {
  const struck = selectable && props.selection.includes(row.item);
  return (
    <div
      className={cn(
        "grid items-baseline gap-x-3 gap-y-1 py-2",
        props.inBlock !== true && "sm:grid-cols-compare-replace",
      )}
    >
      <RowLabel row={row} props={props} selectable={selectable} />
      {/* Below sm the pair stays on one line under the label; from sm its parts join the row's grid. */}
      <div
        className={cn(
          "flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1",
          selectable && indent(props),
          props.inBlock !== true && "sm:contents",
        )}
      >
        <div className="min-w-0">
          <Value item={row.item} props={props} tone="now" struck={struck} />
        </div>
        <ArrowRight
          aria-hidden="true"
          className="size-4 shrink-0 self-center text-muted-foreground"
        />
        <div className="min-w-0">
          <Value item={row.item} props={props} tone="fetched" />
        </div>
      </div>
    </div>
  );
}

function Section({
  title,
  note,
  rows,
  props,
  children,
}: {
  title: string;
  note?: string;
  rows: readonly CompareRow[];
  props: PersonCompareProps;
  children: ReactNode;
}) {
  const all = rows.every((row) => props.selection.includes(row.item));
  return (
    <section className="flex flex-col gap-1">
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="text-sm font-medium">
          {title} <span className="font-normal text-muted-foreground">{rows.length}</span>
        </h3>
        {rows.length > 1 && (
          <Button
            variant="link"
            size="sm"
            className="h-auto px-0 py-0"
            disabled={props.locked}
            onClick={() => rows.forEach((row) => props.onToggle(row.item, !all))}
          >
            {all ? text.clearAll : text.selectAll}
          </Button>
        )}
      </div>
      {note !== undefined && <p className="text-xs text-muted-foreground">{note}</p>}
      <div className="flex flex-col divide-y divide-border border-y border-border">
        {children}
      </div>
    </section>
  );
}

function FaceTile({
  value,
  label,
  src,
  disabled,
}: {
  value: string;
  label: string;
  src: string | undefined;
  disabled: boolean;
}) {
  const id = useId();
  return (
    <Label
      htmlFor={id}
      className="flex w-card-face cursor-pointer flex-col items-stretch gap-1 rounded-md p-1 font-normal has-data-[state=checked]:bg-accent"
    >
      <PersonFace shape="profile" name="" src={src} />
      <span className="flex items-center gap-1 text-xs">
        <RadioGroupItem id={id} value={value} disabled={disabled} />
        <span className="min-w-0 truncate">{label}</span>
      </span>
    </Label>
  );
}

/** The face is one of the fetched images or the person's own, so it is a choice rather than a checkbox. */
function FacePicker({ props, count }: { props: PersonCompareProps; count: number }) {
  const hasFace = props.current.imageRevision !== undefined;
  return (
    <section className="flex flex-col gap-1">
      <h3 className="text-sm font-medium">{text.face}</h3>
      <RadioGroup
        aria-label={text.face}
        value={props.image === null ? "keep" : String(props.image)}
        onValueChange={(value) => props.onImage(value === "keep" ? null : Number(value))}
        className="flex flex-wrap gap-2"
      >
        <FaceTile
          value="keep"
          label={hasFace ? text.keepFace : text.noNewFace}
          src={hasFace ? props.nowFaceSrc : undefined}
          disabled={props.locked}
        />
        {Array.from({ length: count }, (_, index) => (
          <FaceTile
            key={index}
            value={String(index)}
            label={text.image(index + 1)}
            src={props.imageSrc(index)}
            disabled={props.locked}
          />
        ))}
      </RadioGroup>
    </section>
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
            <PairRow key={row.item} row={row} props={props} selectable={false} />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

/**
 * A fetched person beside the person it would fill: the face as a choice among the fetched images,
 * the values that fill empty fields, those that would replace stored ones, and the kept ones folded.
 */
export default function PersonCompare(props: PersonCompareProps) {
  const replace = props.replace === true;
  const rows = compareRows(props.current, props.fetched, replace);
  const fills = rows.filter((row) => row.kind === "fill");
  const replaces = rows.filter((row) => row.kind === "replace");
  const kept = rows.filter((row) => row.kind === "kept");
  const images = faceChoices(props.current, props.fetched, replace);
  return (
    <div className="flex flex-col gap-4 text-sm">
      {images > 0 && <FacePicker props={props} count={images} />}
      {fills.length > 0 && (
        <Section title={text.fills} rows={fills} props={props}>
          {fills.map((row) => (
            <FillRow key={row.item} row={row} props={props} />
          ))}
        </Section>
      )}
      {replaces.length > 0 && (
        <Section
          title={text.replaces}
          note={text.replacesNote}
          rows={replaces}
          props={props}
        >
          {replaces.map((row) => (
            <PairRow key={row.item} row={row} props={props} selectable />
          ))}
        </Section>
      )}
      {images === 0 && fills.length === 0 && replaces.length === 0 && (
        <p className="text-muted-foreground">{text.nothingNew}</p>
      )}
      {kept.length > 0 && <KeptRows rows={kept} props={props} />}
    </div>
  );
}
