import { useEffect, useId, useState } from "react";

import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import {
  currentTagKinds,
  getTagKinds,
  refreshTagKinds,
  subscribeTagKinds,
  type TagKind,
} from "../api/tagKinds";
import { t, type UiText } from "../i18n";
import { CreditFilterField, TagFilterField } from "../library/FilterFields";
import {
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldSet,
  FieldLegend,
  Field,
} from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { FilterCheckbox } from "../videoList/FilterMenu";
import { SMART_RULE_MAX_TAGS } from "./smartRule";

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
        <FilterCheckbox
          checked={rule.favorite === true}
          onCheckedChange={(favorite) => change({ favorite })}
          label={t.list.filter.favoritesOnly}
        />
        <TagsField rule={rule} isMissing={isMissing} onChange={change} />
        {(["person", "organization"] as const).map((kind) => (
          <CreditFilterField
            key={kind}
            kind={kind}
            value={rule[kind]}
            missing={rule[kind] !== undefined && isMissing(kind, rule[kind])}
            onChange={(id) => change({ [kind]: id })}
          />
        ))}
      </div>
      {error !== null && <FieldError id={errorId}>{error}</FieldError>}
    </FieldSet>
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

/**
 * Tags and the kind in one field; a kind missing from a potentially stale cache is not declared
 * deleted until the refresh completes.
 */
function TagsField({
  rule,
  isMissing,
  onChange,
}: {
  rule: SmartRule;
  isMissing: (kind: SmartRuleRef["kind"], id: number) => boolean;
  onChange: (patch: Partial<SmartRule>) => void;
}) {
  const ids = rule.tags ?? [];
  const kindId = rule.tagKind;
  const { kinds, unconfirmed } = useTagKindList(kindId);
  const kindGone =
    kindId !== undefined &&
    (isMissing("tagKind", kindId) ||
      (kinds !== undefined && !unconfirmed && !kinds.some((kind) => kind.id === kindId)));
  return (
    <TagFilterField
      tagIds={ids}
      kindId={kindId}
      kinds={kinds}
      maxTags={SMART_RULE_MAX_TAGS}
      onAddTag={(id) => {
        if (!ids.includes(id)) onChange({ tags: [...ids, id] });
      }}
      onRemoveTag={(id) => {
        const next = ids.filter((existing) => existing !== id);
        onChange({ tags: next.length === 0 ? undefined : next });
      }}
      onKindChange={(tagKind) => onChange({ tagKind })}
      isTagMissing={(id) => isMissing("tag", id)}
      kindMissing={kindGone}
    />
  );
}
