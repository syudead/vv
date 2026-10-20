import { ArrowRight, Check, Plus } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import type { ExternalId } from "../../../api/credits";
import type { PersonPluginFetchedPerson } from "../../../api/personPluginFetches";
import { t } from "../../../i18n";
import { cn } from "../../../lib/cn";
import { Button } from "../../../ui/shadcn/button";
import { Checkbox } from "../../../ui/shadcn/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../../ui/shadcn/collapsible";
import { Label } from "../../../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../../../ui/shadcn/radio-group";
import { Toggle } from "../../../ui/shadcn/toggle";
import { PersonFace } from "../../../ui/PersonFace";
import { Bio } from "../PersonProfile";
import { useLocalToday } from "../useLocalToday";
import {
  aliasList,
  compareRows,
  type CompareRow,
  faceChoices,
  idKey,
  idList,
  linkList,
  type ListChoices,
  type PersonCompareCurrent,
  type PersonSelection,
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
  selection: PersonSelection;
  /** While the apply runs or the result is stale. */
  locked: boolean;
  /** Offer stored values and the face for replacement, as the person page's apply writes over them. */
  replace?: boolean;
  /** In a lookup block, which is narrower than the dialog: rows stack their cells. */
  inBlock?: boolean;
  onChange: (selection: PersonSelection) => void;
}

function toggled<T>(
  values: readonly T[],
  value: T,
  on: boolean,
  key: (v: T) => string,
): T[] {
  const rest = values.filter((item) => key(item) !== key(value));
  return on ? [...rest, value] : rest;
}

function toggleItem(props: PersonCompareProps, item: RowItem, on: boolean) {
  props.onChange({
    ...props.selection,
    items: toggled(props.selection.items, item, on, String),
  });
}

function Muted({ children }: { children: ReactNode }) {
  return <span className="text-muted-foreground">{children}</span>;
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
  const field = profileFieldOf(item);
  if (field === undefined || field === "links") return null;
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
        checked={props.selection.items.includes(row.item)}
        disabled={props.locked}
        onCheckedChange={(checked) => toggleItem(props, row.item, checked === true)}
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
  const struck = selectable && props.selection.items.includes(row.item);
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

/** A part of the comparison with a count and, over more than one choice, Select all or Clear. */
function Section({
  title,
  note,
  count,
  all,
  locked,
  onAll,
  children,
}: {
  title: string;
  note?: string;
  count: number;
  all: boolean;
  locked: boolean;
  onAll: (on: boolean) => void;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-1">
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="text-sm font-medium">
          {title} <span className="font-normal text-muted-foreground">{count}</span>
        </h3>
        {count > 1 && (
          <Button
            variant="link"
            size="sm"
            className="h-auto px-0 py-0"
            disabled={locked}
            onClick={() => onAll(!all)}
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
        value={props.selection.image === null ? "keep" : String(props.selection.image)}
        onValueChange={(value) =>
          props.onChange({
            ...props.selection,
            image: value === "keep" ? null : Number(value),
          })
        }
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

type ListKey = "aliases" | "externalIds" | "links";

interface ListSpec {
  key: ListKey;
  label: string;
  choices: ListChoices<string>;
  text: (value: string) => string;
}

/** External IDs are chosen by a string key, so every list toggles the same way. */
function listSpecs(props: PersonCompareProps): ListSpec[] {
  const ids = idList(props.current, props.fetched);
  const byKey = new Map(
    [...ids.fresh, ...ids.onPerson].map((id) => [idKey(id), id] as const),
  );
  const idText = (key: string) => {
    const id = byKey.get(key);
    return id === undefined ? key : t.credits.personPage.externalId(id.source, id.value);
  };
  const specs: ListSpec[] = [
    {
      key: "aliases",
      label: text.items.aliases,
      choices: aliasList(props.current, props.fetched),
      text: (value) => value,
    },
    {
      key: "externalIds",
      label: text.items.externalIds,
      choices: { fresh: ids.fresh.map(idKey), onPerson: ids.onPerson.map(idKey) },
      text: idText,
    },
    {
      key: "links",
      label: text.items["profile.links"],
      choices: linkList(props.current, props.fetched),
      text: (value) => value,
    },
  ];
  return specs.filter((spec) => spec.choices.fresh.length > 0);
}

function chosenKeys(selection: PersonSelection, key: ListKey): string[] {
  return key === "externalIds" ? selection.externalIds.map(idKey) : selection[key];
}

function withList(
  props: PersonCompareProps,
  key: ListKey,
  keys: readonly string[],
): PersonSelection {
  if (key !== "externalIds") return { ...props.selection, [key]: [...keys] };
  const ids: ExternalId[] = props.fetched.externalIds.filter((id) =>
    keys.includes(idKey(id)),
  );
  return { ...props.selection, externalIds: ids };
}

/** A fetched value the apply would add, pressed while selected; one the person has stays quiet. */
function ValueToggle({
  label,
  pressed,
  disabled,
  onPressedChange,
}: {
  label: string;
  pressed: boolean;
  disabled: boolean;
  onPressedChange: (pressed: boolean) => void;
}) {
  return (
    <Toggle
      variant="outline"
      size="sm"
      pressed={pressed}
      disabled={disabled}
      onPressedChange={onPressedChange}
      title={label}
      className="max-w-full font-normal"
    >
      {pressed ? <Check aria-hidden="true" /> : <Plus aria-hidden="true" />}
      <span className="max-w-chip-label truncate">{label}</span>
    </Toggle>
  );
}

function ListRow({ spec, props }: { spec: ListSpec; props: PersonCompareProps }) {
  const chosen = chosenKeys(props.selection, spec.key);
  return (
    <div
      className={cn(
        "grid items-baseline gap-x-4 gap-y-1 py-2",
        props.inBlock !== true && "sm:grid-cols-compare-fill",
      )}
    >
      <span>{spec.label}</span>
      <div className="flex min-w-0 flex-wrap items-center gap-1">
        {spec.choices.fresh.map((value) => (
          <ValueToggle
            key={value}
            label={spec.text(value)}
            pressed={chosen.includes(value)}
            disabled={props.locked}
            onPressedChange={(on) =>
              props.onChange(
                withList(
                  props,
                  spec.key,
                  on ? [...chosen, value] : chosen.filter((item) => item !== value),
                ),
              )
            }
          />
        ))}
        {spec.choices.onPerson.length > 0 && (
          <span className="text-xs text-muted-foreground">
            {text.alreadyOnPerson(spec.choices.onPerson.length)}
          </span>
        )}
      </div>
    </div>
  );
}

function ListSection({ specs, props }: { specs: ListSpec[]; props: PersonCompareProps }) {
  const count = specs.reduce((sum, spec) => sum + spec.choices.fresh.length, 0);
  const all = specs.every((spec) =>
    spec.choices.fresh.every((value) =>
      chosenKeys(props.selection, spec.key).includes(value),
    ),
  );
  return (
    <Section
      title={text.adds}
      count={count}
      all={all}
      locked={props.locked}
      onAll={(on) => {
        let next = props.selection;
        for (const spec of specs)
          next = withList(
            { ...props, selection: next },
            spec.key,
            on ? spec.choices.fresh : [],
          );
        props.onChange(next);
      }}
    >
      {specs.map((spec) => (
        <ListRow key={spec.key} spec={spec} props={props} />
      ))}
    </Section>
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

function rowsSection(
  props: PersonCompareProps,
  rows: readonly CompareRow[],
): { all: boolean; onAll: (on: boolean) => void } {
  return {
    all: rows.every((row) => props.selection.items.includes(row.item)),
    onAll: (on) => {
      const rest = props.selection.items.filter(
        (item) => !rows.some((row) => row.item === item),
      );
      props.onChange({
        ...props.selection,
        items: on ? [...rest, ...rows.map((row) => row.item)] : rest,
      });
    },
  };
}

/**
 * A fetched person beside the person it would fill: the face as a choice among the fetched images,
 * the values that fill empty fields, those that would replace stored ones, the values the lists
 * would add one by one, and the kept ones folded.
 */
export default function PersonCompare(props: PersonCompareProps) {
  const replace = props.replace === true;
  const rows = compareRows(props.current, props.fetched, replace);
  const fills = rows.filter((row) => row.kind === "fill");
  const replaces = rows.filter((row) => row.kind === "replace");
  const kept = rows.filter((row) => row.kind === "kept");
  const images = faceChoices(props.current, props.fetched, replace);
  const lists = listSpecs(props);
  const nothing =
    images === 0 && fills.length === 0 && replaces.length === 0 && lists.length === 0;
  return (
    <div className="flex flex-col gap-4 text-sm">
      {images > 0 && <FacePicker props={props} count={images} />}
      {fills.length > 0 && (
        <Section
          title={text.fills}
          count={fills.length}
          locked={props.locked}
          {...rowsSection(props, fills)}
        >
          {fills.map((row) => (
            <FillRow key={row.item} row={row} props={props} />
          ))}
        </Section>
      )}
      {replaces.length > 0 && (
        <Section
          title={text.replaces}
          note={text.replacesNote}
          count={replaces.length}
          locked={props.locked}
          {...rowsSection(props, replaces)}
        >
          {replaces.map((row) => (
            <PairRow key={row.item} row={row} props={props} selectable />
          ))}
        </Section>
      )}
      {lists.length > 0 && <ListSection specs={lists} props={props} />}
      {nothing && <p className="text-muted-foreground">{text.nothingNew}</p>}
      {kept.length > 0 && <KeptRows rows={kept} props={props} />}
    </div>
  );
}
