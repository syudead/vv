import { Building2, Plus, Shapes, Tag as TagIcon, TriangleAlert, X } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";

import { isAborted, RequestFailed } from "../api/client";
import { type Credit, type CreditKind, getCredit, listCredits } from "../api/credits";
import type { TagKind } from "../api/tagKinds";
import { creditFace } from "../credits/creditFace";
import { creditDisplayName } from "../credits/displayName";
import { t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Field, FieldDescription, FieldLabel } from "../ui/shadcn/field";
import { Skeleton } from "../ui/shadcn/skeleton";
import CreditCommand, { type CreditChoice } from "../ui/CreditCommand";
import TagCommand from "../ui/TagCommand";
import { useCreditRef, useTagNames } from "./filterNames";
import { buildTagAndKindOptions, pickOfChoice } from "./tagChoices";

// The tag, kind, person and organization fields shared by the library's filter popover and the
// smart folder conditions dialog.

const creditCandidateLimit = 8;

const addIcon = (
  <Plus className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" />
);

export function ConditionChip({
  icon,
  name,
  deleted,
  removeLabel,
  onRemove,
}: {
  icon: ReactNode;
  name: string | undefined;
  deleted?: UiText;
  removeLabel: UiText;
  onRemove: () => void;
}) {
  return (
    <Badge
      variant={deleted === undefined ? "secondary" : "warning"}
      title={deleted ?? name}
      className="max-w-full justify-start gap-0 pr-0 font-normal"
    >
      <span className="mr-1 inline-flex shrink-0 [&_svg]:size-3">
        {deleted === undefined ? icon : <TriangleAlert aria-hidden="true" />}
      </span>
      {deleted !== undefined ? (
        <span className="min-w-0 truncate">{deleted}</span>
      ) : name === undefined ? (
        <Skeleton className="h-3 w-12" />
      ) : (
        <span className="min-w-0 truncate">{name}</span>
      )}
      <span aria-hidden="true" className="ml-1.5 h-4 w-px shrink-0 bg-input" />
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={removeLabel}
        onClick={onRemove}
        className="size-6 rounded-l-none rounded-r-sm [&_svg]:size-3"
      >
        <X aria-hidden="true" />
      </Button>
    </Badge>
  );
}

const never = () => false;

export interface TagFilterFieldProps {
  tagIds: readonly number[];
  /** At most one kind; picking another kind replaces it. */
  kindId: number | undefined;
  /** Undefined until the shared kind list loads. */
  kinds: readonly TagKind[] | undefined;
  maxTags: number;
  onAddTag: (id: number) => void;
  onRemoveTag: (id: number) => void;
  onKindChange: (id: number | undefined) => void;
  isTagMissing?: (id: number) => boolean;
  kindMissing?: boolean;
}

/**
 * Chips of the chosen tags and kind, then one picker offering tags and kinds together. At the tag
 * limit the picker offers kinds only.
 */
export function TagFilterField({
  tagIds,
  kindId,
  kinds,
  maxTags,
  onAddTag,
  onRemoveTag,
  onKindChange,
  isTagMissing = never,
  kindMissing = false,
}: TagFilterFieldProps) {
  const labelId = useId();
  const hintId = useId();
  const byId = useTagNames(tagIds);
  const [value, setValue] = useState("");
  const full = tagIds.length >= maxTags;
  const availableTags = full
    ? []
    : Array.from(byId.values()).filter((tag) => !tagIds.includes(tag.id));
  const availableKinds = (kinds ?? []).filter((kind) => kind.id !== kindId);
  const { options, exactOption } = buildTagAndKindOptions(
    availableTags,
    availableKinds,
    value,
  );

  return (
    <Field>
      <FieldLabel id={labelId} asChild>
        <span>{t.library.tagRow.label}</span>
      </FieldLabel>
      <ul className="flex flex-wrap items-center gap-1" aria-labelledby={labelId}>
        {tagIds.map((id) => (
          <li key={id} className="max-w-full">
            <TagChip
              name={byId.get(id)?.name}
              missing={isTagMissing(id)}
              onRemove={() => onRemoveTag(id)}
            />
          </li>
        ))}
        {kindId !== undefined && (
          <li className="max-w-full">
            <KindChip
              name={kinds?.find((kind) => kind.id === kindId)?.name}
              missing={kindMissing}
              onRemove={() => onKindChange(undefined)}
            />
          </li>
        )}
        <li>
          <TagCommand
            layout="dropdown"
            label={t.player.tags.add}
            value={value}
            onValueChange={setValue}
            choices={options}
            exactChoice={exactOption}
            onSelect={(choice) => {
              setValue("");
              const pick = pickOfChoice(choice);
              if (pick.kind === "tag") onAddTag(pick.id);
              else onKindChange(pick.id);
            }}
            icon={addIcon}
            busy={false}
            describedBy={full ? hintId : undefined}
            onEscape={() => setValue("")}
          />
        </li>
      </ul>
      {full && (
        <FieldDescription id={hintId}>
          {t.smartFolders.dialog.tagsLimit(maxTags)}
        </FieldDescription>
      )}
    </Field>
  );
}

/** A chip for a named reference that may have been deleted since it was chosen. */
function RefChip({
  icon,
  name,
  text = name,
  missing,
  deleted,
  removeLabel,
  onRemove,
}: {
  icon: ReactNode;
  name: string | undefined;
  /** What the chip shows for the name; defaults to the name itself. */
  text?: string;
  missing: boolean;
  deleted: UiText;
  removeLabel: (name: string) => UiText;
  onRemove: () => void;
}) {
  return (
    <ConditionChip
      icon={icon}
      name={text}
      deleted={missing ? deleted : undefined}
      removeLabel={
        missing || name === undefined
          ? t.smartFolders.dialog.removeDeleted
          : removeLabel(name)
      }
      onRemove={onRemove}
    />
  );
}

function TagChip({
  name,
  missing,
  onRemove,
}: {
  name: string | undefined;
  missing: boolean;
  onRemove: () => void;
}) {
  return (
    <RefChip
      icon={<TagIcon aria-hidden="true" />}
      name={name}
      missing={missing}
      deleted={t.smartFolders.condition.deleted("tag")}
      removeLabel={t.smartFolders.dialog.removeTag}
      onRemove={onRemove}
    />
  );
}

function KindChip({
  name,
  missing,
  onRemove,
}: {
  name: string | undefined;
  missing: boolean;
  onRemove: () => void;
}) {
  return (
    <RefChip
      icon={<Shapes aria-hidden="true" />}
      name={name}
      text={name === undefined ? undefined : t.smartFolders.condition.kind(name)}
      missing={missing}
      deleted={t.smartFolders.condition.deleted("tagKind")}
      removeLabel={t.smartFolders.dialog.removeKind}
      onRemove={onRemove}
    />
  );
}

export function creditFieldLabel(kind: CreditKind): UiText {
  return kind === "person"
    ? t.smartFolders.dialog.person
    : t.smartFolders.dialog.organization;
}

/** The chosen person or organization as a chip, and a picker whose choice replaces it. */
export function CreditFilterField({
  kind,
  value,
  missing = false,
  onChange,
}: {
  kind: CreditKind;
  value: number | undefined;
  missing?: boolean;
  onChange: (id: number | undefined) => void;
}) {
  const labelId = useId();
  return (
    <Field>
      <FieldLabel id={labelId} asChild>
        <span>{creditFieldLabel(kind)}</span>
      </FieldLabel>
      <ul className="flex flex-wrap items-center gap-1" aria-labelledby={labelId}>
        {value !== undefined && (
          <li className="max-w-full">
            <CreditChip
              kind={kind}
              id={value}
              missing={missing}
              onRemove={() => onChange(undefined)}
            />
          </li>
        )}
        <li>
          <CreditPicker kind={kind} onPick={onChange} />
        </li>
      </ul>
    </Field>
  );
}

function CreditChip({
  kind,
  id,
  missing,
  onRemove,
}: {
  kind: CreditKind;
  id: number;
  missing: boolean;
  onRemove: () => void;
}) {
  const [gone, setGone] = useState(false);
  const credit = useCreditRef(kind, id, () => setGone(true));
  const person = kind === "person";
  return (
    <RefChip
      icon={person ? creditFace(kind, credit, "token") : <Building2 aria-hidden="true" />}
      name={credit?.name}
      missing={missing || gone}
      deleted={t.smartFolders.condition.deleted(kind)}
      removeLabel={
        person
          ? t.smartFolders.dialog.removePerson
          : t.smartFolders.dialog.removeOrganization
      }
      onRemove={onRemove}
    />
  );
}

const browseLimit = 20;

/**
 * Empty text browses the credits with the most videos, so people without a name, shown as `#<id>`
 * with their face, can be picked too; `#<id>` picks that one credit directly.
 */
async function loadCreditCandidates(
  kind: CreditKind,
  text: string,
  signal: AbortSignal,
): Promise<Credit[]> {
  const byId = /^#(\d+)$/.exec(text);
  if (byId !== null) {
    try {
      return [await getCredit(kind, Number(byId[1]), signal)];
    } catch (error) {
      if (error instanceof RequestFailed && error.status === 404) return [];
      throw error;
    }
  }
  const query =
    text === ""
      ? { sort: "countDesc" as const, limit: browseLimit }
      : { q: text, limit: creditCandidateLimit };
  return (await listCredits(kind, query, signal)).items;
}

/** Abort obsolete candidate requests and omit creation options. */
function CreditPicker({
  kind,
  onPick,
}: {
  kind: CreditKind;
  onPick: (id: number) => void;
}) {
  const [value, setValue] = useState("");
  const [found, setFound] = useState<{ q: string; items: Credit[] }>({
    q: "",
    items: [],
  });
  const [searching, setSearching] = useState(false);
  const trimmed = value.trim();

  useEffect(() => {
    const controller = new AbortController();
    setSearching(true);
    loadCreditCandidates(kind, trimmed, controller.signal)
      .then((items) => setFound({ q: trimmed, items }))
      .catch((error: unknown) => {
        if (!isAborted(error)) setFound({ q: trimmed, items: [] });
      })
      .finally(() => {
        if (!controller.signal.aborted) setSearching(false);
      });
    return () => controller.abort();
  }, [kind, trimmed]);

  // Hide old-query candidates until the current response arrives to prevent selecting the wrong credit.
  const items = found.q !== trimmed ? [] : found.items;
  const choices: CreditChoice[] = items.map((credit) => ({
    id: String(credit.id),
    label: creditDisplayName(credit),
    leading: creditFace(kind, credit, "option"),
  }));
  const folded = foldForMatch(trimmed);
  const exact = items.find(
    (credit) => foldForMatch(creditDisplayName(credit)) === folded,
  );
  const pick = (id: number) => {
    setValue("");
    onPick(id);
  };

  return (
    <CreditCommand
      layout="dropdown"
      label={creditFieldLabel(kind)}
      value={value}
      onValueChange={setValue}
      choices={choices}
      exactChoice={
        exact === undefined
          ? null
          : { id: String(exact.id), label: creditDisplayName(exact) }
      }
      onSelect={(choice) => pick(Number(choice.id))}
      icon={addIcon}
      busy={false}
      searching={searching}
      onEscape={() => setValue("")}
    />
  );
}
