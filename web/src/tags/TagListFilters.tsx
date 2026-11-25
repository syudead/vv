import type { TagKind } from "../api/tagKinds";
import { formatNumber, t } from "../i18n";
import { KindToggleGroup } from "../ui/KindToggleGroup";
import { Skeleton } from "../ui/shadcn/skeleton";
import { TallyStrip } from "../ui/TallyStrip";
import type { TagKindFilter, TagListCriteria } from "./tagListUrl";
import type { TagTallies } from "./useTagTallies";

const countSkeleton = (
  <Skeleton asChild className="inline-block h-3 w-4 align-middle">
    <span />
  </Skeleton>
);

function countOf(value: number | undefined) {
  return value === undefined ? countSkeleton : formatNumber(value);
}

/** The kind filter beside the title; every count is of the whole vocabulary, not of the search. */
export function TagKindFilterGroup({
  kind,
  kinds,
  totalAll,
  noKind,
  onKindChange,
}: {
  kind: TagKindFilter;
  /** In natural name order; undefined while unread. */
  kinds: readonly TagKind[] | undefined;
  totalAll: number | undefined;
  noKind: number | undefined;
  onKindChange: (kind: TagKindFilter) => void;
}) {
  return (
    <KindToggleGroup
      label={t.tags.list.kindFilter}
      value={kind ?? "all"}
      onValueChange={(next) => onKindChange(next === "all" ? null : next)}
      options={[
        { value: "all", label: t.tags.list.all, count: countOf(totalAll) },
        ...(kinds ?? []).map((entry) => ({
          value: String(entry.id),
          label: entry.name,
          kind: entry.id,
          count: formatNumber(entry.tagCount),
        })),
        { value: "none", label: t.tags.list.noKind, count: countOf(noKind) },
      ]}
    />
  );
}

function tally(value: number | undefined) {
  return {
    count: value === undefined ? undefined : formatNumber(value),
    empty: value === 0,
  };
}

/** Tentative, no kind and unused: each count applies its filter, and pressed again clears it. */
export function TagTallyStrip({
  tallies,
  criteria,
  onApply,
}: {
  tallies: TagTallies;
  criteria: Pick<TagListCriteria, "tentativeOnly" | "unusedOnly" | "kind">;
  onApply: (change: Partial<TagListCriteria>) => void;
}) {
  const l = t.tags.list;
  return (
    <TallyStrip
      label={l.tallies}
      clearLabel={l.clear}
      tallies={[
        {
          id: "tentative",
          ...tally(tallies.tentative),
          label: l.tentative,
          hint: l.tentativeHint,
          action: l.review,
          pressed: criteria.tentativeOnly,
          onPressedChange: (on) => onApply({ tentativeOnly: on }),
        },
        {
          id: "no-kind",
          ...tally(tallies.noKind),
          label: l.noKind,
          hint: l.noKindHint,
          action: l.sort,
          pressed: criteria.kind === "none",
          onPressedChange: (on) => onApply({ kind: on ? "none" : null }),
        },
        {
          id: "unused",
          ...tally(tallies.unused),
          label: l.unused,
          hint: l.unusedHint,
          action: l.cleanUp,
          pressed: criteria.unusedOnly,
          onPressedChange: (on) => onApply({ unusedOnly: on }),
        },
      ]}
    />
  );
}
