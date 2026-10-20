import { Building2, Plus, Shapes, Tag as TagIcon, TriangleAlert, X } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";

import { isAborted } from "../api/client";
import { type Credit, type CreditKind, listCredits } from "../api/credits";
import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import {
  currentTagKinds,
  getTagKinds,
  refreshTagKinds,
  subscribeTagKinds,
  type TagKind,
} from "../api/tagKinds";
import { creditFace } from "../credits/creditFace";
import { t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { AddedSelect, LengthSelect } from "../library/AddedLengthSelects";
import { useCreditRef, useTagNames } from "../library/filterNames";
import { buildAddOptions } from "../library/tagChoices";
import TagKindSelect from "../library/TagKindSelect";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Skeleton } from "../ui/shadcn/skeleton";
import CreditCommand, { type CreditChoice } from "../ui/CreditCommand";
import TagCommand from "../ui/TagCommand";
import { FilterCheckbox, WatchStatusField } from "../videoList/FilterMenu";
import { ruleWatch, SMART_RULE_MAX_TAGS, withWatch } from "./smartRule";

const creditCandidateLimit = 8;

export interface ConditionsFieldsProps {
  rule: SmartRule;
  onChange: (rule: SmartRule) => void;
  missing: readonly SmartRuleRef[];
  error: UiText | null;
  searchId?: string;
}

/** Omit empty conditions from the rule. */
export default function ConditionsFields({
  rule,
  onChange,
  missing,
  error,
  searchId,
}: ConditionsFieldsProps) {
  const fallbackId = useId();
  const inputId = searchId ?? fallbackId;
  const errorId = useId();
  const isMissing = (kind: SmartRuleRef["kind"], id: number) =>
    missing.some((ref) => ref.kind === kind && ref.id === id);
  const change = (patch: Partial<SmartRule>) => onChange({ ...rule, ...patch });

  return (
    <FieldSet data-invalid={error !== null || undefined}>
      <FieldLegend>{t.smartFolders.dialog.conditions}</FieldLegend>
      <FieldDescription>{t.smartFolders.dialog.conditionsHint}</FieldDescription>
      <div className="flex flex-col gap-3">
        <Field>
          <FieldLabel htmlFor={inputId}>{t.list.search.placeholder}</FieldLabel>
          <Input
            id={inputId}
            type="search"
            autoComplete="off"
            value={rule.q ?? ""}
            placeholder={t.list.search.placeholder}
            onChange={(event) => change({ q: event.target.value })}
            aria-describedby={error === null ? undefined : errorId}
            className="h-8"
          />
        </Field>
        <WatchStatusField
          value={ruleWatch(rule)}
          onValueChange={(watch) => onChange(withWatch(rule, watch))}
        />
        <div className="flex flex-col gap-2">
          <FilterCheckbox
            checked={rule.favorite === true}
            onCheckedChange={(favorite) => change({ favorite })}
            label={t.list.filter.favoritesOnly}
          />
          <FilterCheckbox
            checked={rule.playable === true}
            onCheckedChange={(playable) => change({ playable })}
            label={t.list.filter.playableOnly}
          />
        </div>
        <TagKindRow
          value={rule.tagKind}
          missing={rule.tagKind !== undefined && isMissing("tagKind", rule.tagKind)}
          onChange={(tagKind) => change({ tagKind })}
        />
        <TagsRow
          ids={rule.tags ?? []}
          isMissing={(id) => isMissing("tag", id)}
          onChange={(tags) => change({ tags })}
        />
        {(["person", "organization"] as const).map((kind) => (
          <CreditRow
            key={kind}
            kind={kind}
            value={rule[kind]}
            missing={rule[kind] !== undefined && isMissing(kind, rule[kind])}
            onChange={(id) => change({ [kind]: id })}
          />
        ))}
        <AddedSelect
          value={rule.addedWithin}
          onValueChange={(addedWithin) => change({ addedWithin })}
        />
        <LengthSelect
          value={rule}
          onValueChange={(range) =>
            change({ minDuration: range.minDuration, maxDuration: range.maxDuration })
          }
        />
      </div>
      {error !== null && <FieldError id={errorId}>{error}</FieldError>}
    </FieldSet>
  );
}

function ConditionChip({
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

const ignore = () => undefined;

/**
 * Refresh when the cached list lacks the selected kind, which may have been created in another
 * tab; unconfirmed stays true until that read completes.
 */
function useTagKindList(value: number | undefined): {
  kinds: TagKind[] | undefined;
  unconfirmed: boolean;
} {
  const [kinds, setKinds] = useState<TagKind[] | undefined>(currentTagKinds);
  const [cachedAtMount] = useState<TagKind[] | undefined>(currentTagKinds);
  useEffect(() => {
    getTagKinds().then(setKinds).catch(ignore);
    return subscribeTagKinds(setKinds);
  }, []);
  const unconfirmed =
    value !== undefined &&
    kinds !== undefined &&
    kinds === cachedAtMount &&
    !kinds.some((kind) => kind.id === value);
  useEffect(() => {
    if (unconfirmed) refreshTagKinds().catch(ignore);
  }, [unconfirmed]);
  return { kinds, unconfirmed };
}

function TagKindRow({
  value,
  missing,
  onChange,
}: {
  value: number | undefined;
  missing: boolean;
  onChange: (id: number | undefined) => void;
}) {
  const { kinds, unconfirmed } = useTagKindList(value);
  // Do not declare a kind missing from a potentially stale pre-refresh cache.
  const gone =
    value !== undefined &&
    (missing ||
      (kinds !== undefined && !unconfirmed && !kinds.some((kind) => kind.id === value)));
  if (!gone)
    return <TagKindSelect value={value} kinds={kinds} onValueChange={onChange} />;
  return (
    <Field>
      <FieldLabel asChild>
        <span>{t.library.tagKindFilter.label}</span>
      </FieldLabel>
      <div>
        <ConditionChip
          icon={<Shapes aria-hidden="true" />}
          name={undefined}
          deleted={t.smartFolders.condition.deleted("tagKind")}
          removeLabel={t.smartFolders.dialog.removeDeleted}
          onRemove={() => onChange(undefined)}
        />
      </div>
    </Field>
  );
}

/** Conditions reference existing tags only; disable entry at the 16-tag limit. */
function TagsRow({
  ids,
  isMissing,
  onChange,
}: {
  ids: readonly number[];
  isMissing: (id: number) => boolean;
  onChange: (ids: number[] | undefined) => void;
}) {
  const labelId = useId();
  const hintId = useId();
  const byId = useTagNames(ids);
  const [value, setValue] = useState("");
  const full = ids.length >= SMART_RULE_MAX_TAGS;
  const available = Array.from(byId.values()).filter((tag) => !ids.includes(tag.id));
  const { options, exactOption } = buildAddOptions(available, value);
  const remove = (id: number) => {
    const next = ids.filter((existing) => existing !== id);
    onChange(next.length === 0 ? undefined : next);
  };
  const add = (id: number) => {
    setValue("");
    if (ids.includes(id) || full) return;
    onChange([...ids, id]);
  };

  return (
    <Field>
      <FieldLabel id={labelId} asChild>
        <span>{t.library.tagRow.label}</span>
      </FieldLabel>
      <ul className="flex flex-wrap items-center gap-1" aria-labelledby={labelId}>
        {ids.map((id) => {
          const name = byId.get(id)?.name;
          return (
            <li key={id} className="max-w-full">
              <ConditionChip
                icon={<TagIcon aria-hidden="true" />}
                name={name}
                deleted={
                  isMissing(id) ? t.smartFolders.condition.deleted("tag") : undefined
                }
                removeLabel={
                  isMissing(id) || name === undefined
                    ? t.smartFolders.dialog.removeDeleted
                    : t.smartFolders.dialog.removeTag(name)
                }
                onRemove={() => remove(id)}
              />
            </li>
          );
        })}
        <li>
          <TagCommand
            layout="dropdown"
            label={t.player.tags.add}
            value={value}
            onValueChange={setValue}
            choices={options}
            exactChoice={exactOption}
            onSelect={(option) => add(Number(option.id))}
            icon={
              <Plus
                className="size-3 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            }
            busy={false}
            locked={full}
            describedBy={full ? hintId : undefined}
            onEscape={() => setValue("")}
          />
        </li>
      </ul>
      {full && (
        <FieldDescription id={hintId}>
          {t.smartFolders.dialog.tagsLimit(SMART_RULE_MAX_TAGS)}
        </FieldDescription>
      )}
    </Field>
  );
}

function creditLabel(kind: CreditKind): UiText {
  return kind === "person"
    ? t.smartFolders.dialog.person
    : t.smartFolders.dialog.organization;
}

function CreditRow({
  kind,
  value,
  missing,
  onChange,
}: {
  kind: CreditKind;
  value: number | undefined;
  missing: boolean;
  onChange: (id: number | undefined) => void;
}) {
  return (
    <Field>
      <FieldLabel asChild>
        <span>{creditLabel(kind)}</span>
      </FieldLabel>
      {value === undefined ? (
        <CreditConditionPicker kind={kind} onPick={onChange} />
      ) : (
        <div>
          <CreditChip
            kind={kind}
            id={value}
            missing={missing}
            onRemove={() => onChange(undefined)}
          />
        </div>
      )}
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
  const name = credit?.name;
  const deleted = missing || gone;
  const removeLabel =
    deleted || name === undefined
      ? t.smartFolders.dialog.removeDeleted
      : kind === "person"
        ? t.smartFolders.dialog.removePerson(name)
        : t.smartFolders.dialog.removeOrganization(name);
  return (
    <ConditionChip
      icon={
        kind === "person" ? (
          creditFace(kind, credit, "token")
        ) : (
          <Building2 aria-hidden="true" />
        )
      }
      name={name}
      deleted={deleted ? t.smartFolders.condition.deleted(kind) : undefined}
      removeLabel={removeLabel}
      onRemove={onRemove}
    />
  );
}

/** Abort obsolete candidate requests and omit creation options. */
function CreditConditionPicker({
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
    if (trimmed === "") {
      setSearching(false);
      return;
    }
    const controller = new AbortController();
    setSearching(true);
    listCredits(kind, { q: trimmed, limit: creditCandidateLimit }, controller.signal)
      .then((page) => setFound({ q: trimmed, items: page.items }))
      .catch((error: unknown) => {
        if (!isAborted(error)) setFound({ q: trimmed, items: [] });
      })
      .finally(() => {
        if (!controller.signal.aborted) setSearching(false);
      });
    return () => controller.abort();
  }, [kind, trimmed]);

  // Hide old-query candidates until the current response arrives to prevent selecting the wrong credit.
  const items = trimmed === "" || found.q !== trimmed ? [] : found.items;
  const choices: CreditChoice[] = items.map((credit) => ({
    id: String(credit.id),
    label: credit.name,
    leading: creditFace(kind, credit, "option"),
  }));
  const folded = foldForMatch(trimmed);
  const exact = items.find((credit) => foldForMatch(credit.name) === folded);
  const pick = (id: number) => {
    setValue("");
    onPick(id);
  };

  return (
    <CreditCommand
      layout="dropdown"
      label={creditLabel(kind)}
      value={value}
      onValueChange={setValue}
      choices={choices}
      exactChoice={
        exact === undefined ? null : { id: String(exact.id), label: exact.name }
      }
      onSelect={(choice) => pick(Number(choice.id))}
      icon={<Plus className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" />}
      busy={false}
      searching={searching}
      onEscape={() => setValue("")}
    />
  );
}
