import { TriangleAlert } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { Link } from "react-router";

import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import {
  currentTagKinds,
  getTagKinds,
  subscribeTagKinds,
  type TagKind,
} from "../api/tagKinds";
import type { Tag } from "../api/tags";
import { t } from "../i18n";
import { useCreditName, useTagNames } from "../library/filterNames";
import { cn } from "../lib/cn";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { type ConditionChip, conditionChips, refText } from "./conditions";
import type { AncestorRule } from "./pageRules";
import { smartFolderPagePath } from "./smartFolderPaths";

// Owner-only: guest responses omit the conditions settings.

const ignoreGone = () => undefined;

function useTagKindNames(wanted: boolean): ReadonlyMap<number, TagKind> {
  const [kinds, setKinds] = useState<TagKind[] | undefined>(currentTagKinds);
  useEffect(() => subscribeTagKinds(setKinds), []);
  useEffect(() => {
    if (!wanted) return;
    let alive = true;
    getTagKinds().then(
      (loaded) => {
        if (alive) setKinds(loaded);
      },
      () => undefined,
    );
    return () => {
      alive = false;
    };
  }, [wanted]);
  return new Map((kinds ?? []).map((kind) => [kind.id, kind]));
}

interface Names {
  tags: ReadonlyMap<number, Tag>;
  kinds: ReadonlyMap<number, TagKind>;
}

/**
 * A condition chip; undefined text shows a placeholder while a name loads. Uncapped chips hold
 * short texts built here, such as a date range, which the name cap would cut.
 */
export function NamedChip({
  text,
  uncapped = false,
}: {
  text: string | undefined;
  uncapped?: boolean;
}) {
  if (text === undefined) {
    return (
      <Badge variant="outline">
        {/* Use a span skeleton because PageHeader description renders a p. */}
        <Skeleton asChild className="h-3 w-12">
          <span />
        </Skeleton>
      </Badge>
    );
  }
  return (
    <Badge
      variant="outline"
      title={text}
      className={cn(
        "max-w-full text-xs font-normal text-muted-foreground",
        !uncapped && "max-w-chip-label",
      )}
    >
      <span className="truncate">{text}</span>
    </Badge>
  );
}

function CreditChip({ kind, id }: { kind: "person" | "organization"; id: number }) {
  const name = useCreditName(kind, id, ignoreGone);
  return <NamedChip text={name === undefined ? undefined : refText(kind, name)} />;
}

function Chip({ chip, names }: { chip: ConditionChip; names: Names }) {
  if (chip.kind === "text")
    return <NamedChip text={chip.text} uncapped={chip.uncapped} />;
  if (chip.missing) {
    return (
      <Badge variant="warning" className="max-w-chip-label text-xs font-normal">
        <TriangleAlert aria-hidden="true" />
        {t.smartFolders.condition.deleted(chip.ref)}
      </Badge>
    );
  }
  if (chip.ref === "person" || chip.ref === "organization") {
    return <CreditChip kind={chip.ref} id={chip.id} />;
  }
  const name =
    chip.ref === "tag" ? names.tags.get(chip.id)?.name : names.kinds.get(chip.id)?.name;
  return <NamedChip text={name === undefined ? undefined : refText(chip.ref, name)} />;
}

function ChipRow({
  rule,
  missing,
  names,
}: {
  rule: SmartRule | undefined;
  missing: readonly SmartRuleRef[];
  names: Names;
}) {
  if (rule === undefined) return <NamedChip text={undefined} />;
  return (
    <>
      {conditionChips(rule, missing).map((chip) => (
        <Chip key={chip.key} chip={chip} names={names} />
      ))}
    </>
  );
}

function ruleRefs(rules: readonly (SmartRule | undefined)[]) {
  const tagIds = rules.flatMap((rule) => rule?.tags ?? []);
  const kinds = rules.some((rule) => rule?.tagKind !== undefined);
  return { tagIds, kinds };
}

/**
 * Use spans because these condition lines sit inside PageHeader's p description; ancestor
 * conditions are nearest-first.
 */
export default function ConditionLines({
  rule,
  ancestors,
  missing,
}: {
  rule: SmartRule;
  ancestors: readonly AncestorRule[];
  missing: readonly SmartRuleRef[];
}) {
  const refs = ruleRefs([rule, ...ancestors.map((ancestor) => ancestor.rule)]);
  const names: Names = {
    tags: useTagNames(refs.tagIds),
    kinds: useTagKindNames(refs.kinds),
  };
  return (
    <span className="flex flex-col gap-1">
      <ConditionLine label={t.smartFolders.detail.conditions}>
        <ChipRow rule={rule} missing={missing} names={names} />
      </ConditionLine>
      {ancestors.map((ancestor) => (
        <ConditionLine
          key={ancestor.folder.id}
          label={
            <Button
              variant="link"
              size="sm"
              asChild
              className="h-auto max-w-full min-w-0 p-0 text-xs"
            >
              <Link to={smartFolderPagePath(ancestor.folder.id)}>
                <span className="truncate">
                  {t.smartFolders.detail.fromParent(ancestor.folder.name)}
                </span>
              </Link>
            </Button>
          }
        >
          <ChipRow rule={ancestor.rule} missing={missing} names={names} />
        </ConditionLine>
      ))}
    </span>
  );
}

function ConditionLine({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-1 text-xs text-muted-foreground">
      <span className="flex max-w-full min-w-0 shrink-0">{label}</span>
      {children}
    </span>
  );
}
