import type { ReactNode } from "react";

import type { ExternalId } from "../../../api/credits";
import type { PersonPluginFetchedPerson } from "../../../api/personPluginFetches";
import { t } from "../../../i18n";
import { cn } from "../../../lib/cn";
import {
  ChangeSections,
  CompareSection,
  ImageChoice,
  KeptFold,
  PairRow,
  RowLabel,
  ValueRow,
  ValueToggle,
} from "../../../player/workInfoFetch/compareParts";
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
  onChange: (selection: PersonSelection) => void;
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

function withItem(
  props: PersonCompareProps,
  item: RowItem,
  on: boolean,
): PersonSelection {
  const rest = props.selection.items.filter((value) => value !== item);
  return { ...props.selection, items: on ? [...rest, item] : rest };
}

function ItemLabel({
  row,
  props,
  selectable,
}: {
  row: CompareRow;
  props: PersonCompareProps;
  selectable: boolean;
}) {
  return (
    <RowLabel
      label={text.items[row.item]}
      checked={selectable ? props.selection.items.includes(row.item) : undefined}
      locked={props.locked}
      onChange={(on) => props.onChange(withItem(props, row.item, on))}
    />
  );
}

/** A replacing or kept row: the stored value, struck through while it would be replaced, then the fetched one. */
function ProfilePair({
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
    <PairRow
      label={<ItemLabel row={row} props={props} selectable={selectable} />}
      now={<Value item={row.item} props={props} tone="now" struck={struck} />}
      fetched={<Value item={row.item} props={props} tone="fetched" />}
      indented={selectable}
    />
  );
}

/** The face is one of the fetched images or the person's own, so it is a choice rather than a checkbox. */
function FacePicker({ props, count }: { props: PersonCompareProps; count: number }) {
  const hasFace = props.current.imageRevision !== undefined;
  return (
    <ImageChoice
      title={text.face}
      value={props.selection.image === null ? "keep" : String(props.selection.image)}
      locked={props.locked}
      onChange={(value) =>
        props.onChange({
          ...props.selection,
          image: value === "keep" ? null : Number(value),
        })
      }
      options={[
        {
          value: "keep",
          label: hasFace ? text.keepFace : text.noNewFace,
          image: (
            <PersonFace
              shape="profile"
              name=""
              src={hasFace ? props.nowFaceSrc : undefined}
            />
          ),
        },
        ...Array.from({ length: count }, (_, index) => ({
          value: String(index),
          label: text.image(index + 1),
          image: <PersonFace shape="profile" name="" src={props.imageSrc(index)} />,
        })),
      ]}
    />
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
  selection: PersonSelection,
  fetched: PersonPluginFetchedPerson,
  key: ListKey,
  keys: readonly string[],
): PersonSelection {
  if (key !== "externalIds") return { ...selection, [key]: [...keys] };
  const ids: ExternalId[] = fetched.externalIds.filter((id) => keys.includes(idKey(id)));
  return { ...selection, externalIds: ids };
}

function ListRow({ spec, props }: { spec: ListSpec; props: PersonCompareProps }) {
  const chosen = chosenKeys(props.selection, spec.key);
  return (
    <ValueRow label={<span>{spec.label}</span>}>
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
                  props.selection,
                  props.fetched,
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
    </ValueRow>
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
    <CompareSection
      title={t.common.addsToLists}
      count={count}
      all={all}
      locked={props.locked}
      onAll={(on) =>
        props.onChange(
          specs.reduce(
            (next, spec) =>
              withList(next, props.fetched, spec.key, on ? spec.choices.fresh : []),
            props.selection,
          ),
        )
      }
    >
      {specs.map((spec) => (
        <ListRow key={spec.key} spec={spec} props={props} />
      ))}
    </CompareSection>
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
 * would add one by one, and the equal ones folded.
 */
export default function PersonCompare(props: PersonCompareProps) {
  const rows = compareRows(props.current, props.fetched, true);
  const fills = rows.filter((row) => row.kind === "fill");
  const replaces = rows.filter((row) => row.kind === "replace");
  const kept = rows.filter((row) => row.kind === "kept");
  const images = faceChoices(props.current, props.fetched, true);
  const lists = listSpecs(props);
  const nothing =
    images === 0 && fills.length === 0 && replaces.length === 0 && lists.length === 0;
  return (
    <div className="flex flex-col gap-4 text-sm">
      {images > 0 && <FacePicker props={props} count={images} />}
      <ChangeSections
        fills={fills}
        replaces={replaces}
        locked={props.locked}
        keyOf={(row) => row.item}
        section={(part) => rowsSection(props, part)}
        fill={(row) => (
          <ValueRow label={<ItemLabel row={row} props={props} selectable />}>
            <Value item={row.item} props={props} tone="fetched" />
          </ValueRow>
        )}
        replace={(row) => <ProfilePair row={row} props={props} selectable />}
      />
      {lists.length > 0 && <ListSection specs={lists} props={props} />}
      {nothing && <p className="text-muted-foreground">{text.nothingNew}</p>}
      {kept.length > 0 && (
        <KeptFold count={kept.length} note={text.sameNote}>
          {kept.map((row) => (
            <ProfilePair key={row.item} row={row} props={props} selectable={false} />
          ))}
        </KeptFold>
      )}
    </div>
  );
}
