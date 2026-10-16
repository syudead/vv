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
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { type ConditionChip, conditionChips, refText } from "./conditions";
import type { AncestorRule } from "./pageRules";
import { smartFolderPagePath } from "./smartFolderPaths";

// スマートフォルダのページの条件の行（specs/052-smart-folders/ui-design.md「Conditions
// lines」）。所有者だけに出す（条件は所有者の応答の `settings` にだけ入る）。

const ignoreGone = () => undefined;

/** useTagKindNames は共有の種類の一覧を読み、種類の名前を引けるようにする。 */
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

/** Names はチップが名前を引く表である。 */
interface Names {
  tags: ReadonlyMap<number, Tag>;
  kinds: ReadonlyMap<number, TagKind>;
}

/**
 * chipClassName は条件のチップの文字である（枠だけで面を持たない outline の Badge）。行と同じ
 * 控えめな色にし、カードの題名より先に目を引かない。
 */
const chipClassName = "max-w-chip-label text-xs font-normal text-muted-foreground";

/** NamedChip は名前の分かったチップか、分かる前の骨組みである。 */
function NamedChip({ text }: { text: string | undefined }) {
  if (text === undefined) {
    return (
      <Badge variant="outline">
        {/* PageHeader の説明（p）の中に置くので、骨組みは span にする。 */}
        <Skeleton asChild className="h-3 w-12">
          <span />
        </Skeleton>
      </Badge>
    );
  }
  return (
    <Badge variant="outline" title={text} className={chipClassName}>
      <span className="truncate">{text}</span>
    </Badge>
  );
}

/** CreditChip は人物か団体のチップである。名前を読む。 */
function CreditChip({ kind, id }: { kind: "person" | "organization"; id: number }) {
  const name = useCreditName(kind, id, ignoreGone);
  return <NamedChip text={name === undefined ? undefined : refText(kind, name)} />;
}

/** Chip は条件 1 つのチップである。欠けた参照は `warning` の Badge に印を付けて出す。 */
function Chip({ chip, names }: { chip: ConditionChip; names: Names }) {
  if (chip.kind === "text") return <NamedChip text={chip.text} />;
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

/** ChipRow は 1 つのフォルダの条件のチップの並びである。読めていなければ骨組み 1 つ。 */
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

/** ruleRefs は条件たちが名指すタグの id と、種類を名指すかを返す。 */
function ruleRefs(rules: readonly (SmartRule | undefined)[]) {
  const tagIds = rules.flatMap((rule) => rule?.tags ?? []);
  const kinds = rules.some((rule) => rule?.tagKind !== undefined);
  return { tagIds, kinds };
}

/**
 * ConditionLines は見出しの件数の下の条件の行である。1 行目は「Conditions:」とこのフォルダの
 * 条件、続いて祖先ごとに近い順に「From "{parent}":」（その祖先のページへのリンク）とその
 * 祖先の条件。チップは読むだけで押せない。`missing` が名指すチップは `warning` になる。
 * PageHeader の説明（p）の中に置くので、行は span で組む。
 */
export default function ConditionLines({
  rule,
  ancestors,
  missing,
}: {
  rule: SmartRule;
  /** 祖先（近い順）。 */
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

/** ConditionLine は条件の 1 行（ラベルと、折り返すチップの並び）である。 */
function ConditionLine({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-1 text-xs text-muted-foreground">
      <span className="flex max-w-full min-w-0 shrink-0">{label}</span>
      {children}
    </span>
  );
}
