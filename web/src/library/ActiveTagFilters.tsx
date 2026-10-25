import { Building2, Shapes, Tag as TagIcon, X } from "lucide-react";
import { type ReactNode, type RefObject, useEffect, useRef } from "react";

import type { CreditKind } from "../api/credits";
import { compareNatural } from "../api/tagOrder";
import { creditFace } from "../credits/creditFace";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Toggle } from "../ui/shadcn/toggle";
import { type CreditFilter, creditKinds } from "./creditCriteria";
import { conditionIcon } from "./conditionIcon";
import { useCreditRef, useTagNames } from "./filterNames";
import {
  type VideoConditionKind,
  videoConditionChips,
  type VideoConditions,
} from "./videoConditions";

export interface ActiveTagFiltersProps {
  tagIds: readonly number[];
  onRemove: (id: number) => void;
  credits?: CreditFilter;
  onRemoveCredit?: (kind: CreditKind) => void;
  /** Callers remove missing credit filters from the URL and notify the user. */
  onCreditGone?: (kind: CreditKind) => void;
  /** Undefined until kind names load; render a skeleton meanwhile. */
  tagKindName?: string;
  onRemoveTagKind?: () => void;
  /** Shown after the tag, credit and kind chips in the order duration, added date, watch state. */
  conditions?: VideoConditions;
  onRemoveCondition?: (kind: VideoConditionKind) => void;
  searchFieldRef: RefObject<HTMLInputElement | null>;
}

type ChipKey = string;

const tagKindKey: ChipKey = "tagKind";

function tagKey(id: number): ChipKey {
  return `tag:${String(id)}`;
}

interface ChipProps {
  icon: ReactNode;
  name: string | undefined;
  unnamedLabel: string;
  namedLabel?: (name: string) => string;
  text?: (name: string) => string;
  onPress: () => void;
  /** Condition texts are short and built here, so they skip the name width cap. */
  uncapped?: boolean;
  buttonRef: (node: HTMLButtonElement | null) => void;
}

function FilterChip({
  icon,
  name,
  unnamedLabel,
  namedLabel = t.library.activeTags.removeNamed,
  text = (shown) => shown,
  onPress,
  uncapped = false,
  buttonRef,
}: ChipProps) {
  // Keep an accessible remove name even before the filter's display name arrives.
  const label = name === undefined ? unnamedLabel : namedLabel(name);
  return (
    <li>
      <Toggle
        ref={buttonRef}
        variant="outline"
        size="sm"
        pressed
        onPressedChange={onPress}
        aria-label={label}
        title={label}
      >
        {icon}
        {name === undefined ? (
          <Skeleton className="h-3 w-12" />
        ) : (
          <span
            title={text(name)}
            className={cn("min-w-0 truncate", !uncapped && "max-w-chip-label")}
          >
            {text(name)}
          </span>
        )}
        <X aria-hidden="true" />
      </Toggle>
    </li>
  );
}

function CreditChip({
  kind,
  id,
  onGone,
  ...rest
}: Omit<ChipProps, "icon" | "name" | "unnamedLabel"> & {
  kind: CreditKind;
  id: number;
  onGone: (kind: CreditKind) => void;
}) {
  const credit = useCreditRef(kind, id, onGone);
  const person = kind === "person";
  return (
    <FilterChip
      {...rest}
      icon={person ? creditFace(kind, credit, "token") : <Building2 aria-hidden="true" />}
      name={credit?.name}
      unnamedLabel={
        person
          ? t.library.activeTags.removePerson
          : t.library.activeTags.removeOrganization
      }
    />
  );
}

const noCredits: CreditFilter = {};
const ignoreCredit = () => undefined;
const noConditions: VideoConditions = {};

function conditionKey(kind: VideoConditionKind): ChipKey {
  return `condition:${kind}`;
}

export default function ActiveTagFilters({
  tagIds,
  onRemove,
  credits = noCredits,
  onRemoveCredit = ignoreCredit,
  onCreditGone = ignoreCredit,
  tagKindName,
  onRemoveTagKind = ignoreCredit,
  conditions = noConditions,
  onRemoveCondition = ignoreCredit,
  searchFieldRef,
}: ActiveTagFiltersProps) {
  const byId = useTagNames(tagIds);

  const buttonRefs = useRef(new Map<ChipKey, HTMLButtonElement>());
  const pendingFocus = useRef<ChipKey | "search" | null>(null);
  useEffect(() => {
    const pending = pendingFocus.current;
    if (pending === null) return;
    pendingFocus.current = null;
    if (pending === "search") {
      searchFieldRef.current?.focus();
      return;
    }
    buttonRefs.current.get(pending)?.focus();
  }, [
    tagIds,
    credits.person,
    credits.organization,
    credits.tagKind,
    conditions,
    searchFieldRef,
  ]);

  const orderedTags = [...tagIds].sort((a, b) => {
    const nameA = byId.get(a)?.name;
    const nameB = byId.get(b)?.name;
    if (nameA === undefined || nameB === undefined) return a - b;
    return compareNatural(nameA, nameB) || a - b;
  });
  const activeCredits = creditKinds.flatMap((kind) => {
    const id = credits[kind];
    return id === undefined ? [] : [{ kind, id }];
  });
  const conditionChips = videoConditionChips(conditions);
  const ordered: ChipKey[] = [
    ...orderedTags.map(tagKey),
    ...activeCredits.map(({ kind }) => kind),
    ...(credits.tagKind === undefined ? [] : [tagKindKey]),
    ...conditionChips.map((chip) => conditionKey(chip.kind)),
  ];

  // Focus the search input before removing the last chip because unmounting prevents the focus effect.
  function removeChip(key: ChipKey, remove: () => void) {
    const index = ordered.indexOf(key);
    const next = ordered[index + 1] ?? ordered[index - 1];
    if (next === undefined) {
      pendingFocus.current = null;
      searchFieldRef.current?.focus();
    } else {
      pendingFocus.current = next;
    }
    remove();
  }

  const refFor = (key: ChipKey) => (node: HTMLButtonElement | null) => {
    if (node) buttonRefs.current.set(key, node);
    else buttonRefs.current.delete(key);
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs text-muted-foreground">{t.library.activeTags.label}</span>
      <ul
        aria-label={t.library.activeTags.list}
        className="flex flex-wrap items-center gap-2"
      >
        {orderedTags.map((id) => (
          <FilterChip
            key={tagKey(id)}
            buttonRef={refFor(tagKey(id))}
            icon={<TagIcon aria-hidden="true" />}
            name={byId.get(id)?.name}
            unnamedLabel={t.library.activeTags.remove}
            onPress={() => removeChip(tagKey(id), () => onRemove(id))}
          />
        ))}
        {activeCredits.map(({ kind, id }) => (
          <CreditChip
            key={`${kind}:${String(id)}`}
            kind={kind}
            id={id}
            buttonRef={refFor(kind)}
            onGone={onCreditGone}
            onPress={() => removeChip(kind, () => onRemoveCredit(kind))}
          />
        ))}
        {credits.tagKind !== undefined && (
          <FilterChip
            buttonRef={refFor(tagKindKey)}
            icon={<Shapes aria-hidden="true" />}
            name={tagKindName}
            unnamedLabel={t.library.activeTags.removeKind}
            namedLabel={t.library.activeTags.removeKindNamed}
            text={t.library.activeTags.kindChip}
            onPress={() => removeChip(tagKindKey, onRemoveTagKind)}
          />
        )}
        {conditionChips.map((chip) => (
          <FilterChip
            key={conditionKey(chip.kind)}
            buttonRef={refFor(conditionKey(chip.kind))}
            icon={conditionIcon(chip.kind, conditions)}
            name={chip.text}
            unnamedLabel={chip.remove}
            namedLabel={() => chip.remove}
            uncapped
            onPress={() =>
              removeChip(conditionKey(chip.kind), () => onRemoveCondition(chip.kind))
            }
          />
        ))}
      </ul>
    </div>
  );
}
