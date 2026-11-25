import { Check, Merge, Plus, Shapes, Tags } from "lucide-react";
import { type MouseEvent, useState } from "react";

import { t } from "@/i18n";
import { FloatingNotice } from "@/ui/FloatingNotice";
import { KindToggleGroup } from "@/ui/KindToggleGroup";
import { CardGrid } from "@/ui/patterns/card-grid";
import { EmptyState } from "@/ui/patterns/empty-state";
import { ListPage } from "@/ui/patterns/list-page";
import { LoadingState } from "@/ui/patterns/loading-state";
import { PageHeader } from "@/ui/patterns/page-header";
import { SelectionBar } from "@/ui/patterns/selection-bar";
import { RecordCard } from "@/ui/RecordCard";
import { Button } from "@/ui/shadcn/button";
import { Skeleton } from "@/ui/shadcn/skeleton";
import { TallyStrip } from "@/ui/TallyStrip";
import TentativeMark from "@/ui/TentativeMark";

// registry:block record-list-example. Real pages use router links and read counts from the API; see web/registry/rules/patterns.md, List page.

type TagId = "travel" | "kyoto" | "night" | "trains" | "autumn" | "coast";

interface SampleTag {
  id: TagId;
  kind?: number;
  count: number;
  tentative?: boolean;
}

const tags: SampleTag[] = [
  { id: "travel", kind: 3, count: 42 },
  { id: "kyoto", kind: 1, count: 12, tentative: true },
  { id: "night", count: 7 },
  { id: "trains", kind: 3, count: 18 },
  { id: "autumn", kind: 2, count: 9 },
  { id: "coast", kind: 1, count: 0 },
];

const stay = (event: MouseEvent) => event.preventDefault();

interface Criteria {
  kind: string;
  tentative: boolean;
  unused: boolean;
}

const allTags: Criteria = { kind: "all", tentative: false, unused: false };

function kindKey(tag: SampleTag): string {
  return tag.kind === undefined ? "none" : String(tag.kind);
}

// All criteria apply together; the counts stay of every tag.
function matches(tag: SampleTag, criteria: Criteria): boolean {
  if (criteria.kind !== "all" && kindKey(tag) !== criteria.kind) return false;
  if (criteria.tentative && tag.tentative !== true) return false;
  return !criteria.unused || tag.count === 0;
}

function countOf(predicate: (tag: SampleTag) => boolean): number {
  return tags.filter(predicate).length;
}

const countSkeleton = (
  <Skeleton asChild className="inline-block h-3 w-2 align-middle">
    <span />
  </Skeleton>
);

export function sampleKinds() {
  const r = t.designSystem.pattern.record;
  return [
    { id: 1, name: r.kindNames.place },
    { id: 2, name: r.kindNames.season },
    { id: 3, name: r.kindNames.genre },
  ];
}

function kindName(kind: number | undefined): string {
  return (
    sampleKinds().find((entry) => entry.id === kind)?.name ??
    t.designSystem.pattern.record.noKind
  );
}

function SampleTagCard({
  tag,
  selected,
  selectionMode,
  onSelectedChange,
}: {
  tag: SampleTag;
  selected: boolean;
  selectionMode: boolean;
  onSelectedChange: (selected: boolean) => void;
}) {
  const r = t.designSystem.pattern.record;
  const name = r.tags[tag.id];
  const kind = kindName(tag.kind);
  const synonyms: Partial<Record<TagId, readonly string[]>> = r.synonyms;
  return (
    <RecordCard
      link={<a href="#record-list-example" onClick={stay} />}
      name={name}
      nameMarks={tag.tentative === true && <TentativeMark />}
      count={tag.count}
      countTitle={r.videoCount(tag.count)}
      detail={tag.tentative === true ? r.tentativeDetail(kind) : kind}
      secondary={synonyms[tag.id]}
      kind={tag.kind}
      tentative={tag.tentative}
      selection={{
        selected,
        selectionMode,
        label: r.select(name),
        onSelectedChange,
      }}
    />
  );
}

export function RecordListExample({ loading = false }: { loading?: boolean }) {
  const r = t.designSystem.pattern.record;
  const p = t.designSystem.pattern;
  const [criteria, setCriteria] = useState<Criteria>(allTags);
  const { kind, tentative, unused } = criteria;
  const setKind = (next: string) => {
    setCriteria((current) => ({ ...current, kind: next }));
  };
  const [notice, setNotice] = useState(true);
  const [selected, setSelected] = useState<ReadonlySet<TagId>>(() => new Set(["trains"]));
  const kinds = sampleKinds();
  const kindCount = (key: string) =>
    loading ? countSkeleton : countOf((tag) => key === "all" || kindKey(tag) === key);
  const tally = (predicate: (tag: SampleTag) => boolean) => {
    const n = countOf(predicate);
    return { count: loading ? undefined : n, empty: !loading && n === 0 };
  };
  const visible = tags.filter((tag) => matches(tag, criteria));
  const toggle = (id: TagId, on: boolean) => {
    setSelected((current) => {
      const next = new Set(current);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  };
  return (
    <ListPage
      header={
        <PageHeader
          title={r.title}
          actions={
            <>
              <Button asChild variant="ghost" size="sm">
                <a href="#record-list-example" onClick={stay}>
                  <Shapes aria-hidden="true" />
                  {r.kinds}
                </a>
              </Button>
              <Button asChild variant="ghost" size="sm">
                <a href="#record-list-example" onClick={stay}>
                  {r.rejected}
                </a>
              </Button>
              <Button size="sm">
                <Plus aria-hidden="true" />
                {r.newTag}
              </Button>
            </>
          }
        />
      }
      filter={
        <KindToggleGroup
          label={r.filter}
          value={kind}
          onValueChange={setKind}
          options={[
            { value: "all", label: r.all, count: kindCount("all") },
            ...kinds.map((entry) => ({
              value: String(entry.id),
              label: entry.name,
              kind: entry.id,
              count: kindCount(String(entry.id)),
            })),
            { value: "none", label: r.noKind, count: kindCount("none") },
          ]}
        />
      }
      summary={
        <TallyStrip
          label={r.summary}
          clearLabel={r.clear}
          tallies={[
            {
              id: "tentative",
              ...tally((tag) => tag.tentative === true),
              label: r.tentative,
              hint: r.tentativeHint,
              action: r.review,
              pressed: tentative,
              onPressedChange: (on) => {
                setCriteria((current) => ({ ...current, tentative: on }));
              },
            },
            {
              id: "no-kind",
              ...tally((tag) => tag.kind === undefined),
              label: r.noKind,
              hint: r.noKindHint,
              action: r.sort,
              pressed: kind === "none",
              onPressedChange: (on) => {
                setKind(on ? "none" : "all");
              },
            },
            {
              id: "unused",
              ...tally((tag) => tag.count === 0),
              label: r.unused,
              hint: r.unusedHint,
              action: r.cleanUp,
              pressed: unused,
              onPressedChange: (on) => {
                setCriteria((current) => ({ ...current, unused: on }));
              },
            },
          ]}
        />
      }
      notice={
        notice && !loading ? (
          <FloatingNotice
            message={r.changed}
            actionLabel={r.refresh}
            onAction={() => {
              setNotice(false);
            }}
          />
        ) : undefined
      }
      selectionBar={
        selected.size > 0 &&
        !loading && (
          <SelectionBar
            count={p.selected(selected.size)}
            clearLabel={p.clearSelection}
            onClear={() => {
              setSelected(new Set());
            }}
          >
            <Button variant="ghost" size="sm">
              <Merge aria-hidden="true" />
              {r.merge}
            </Button>
            <Button variant="ghost" size="sm">
              <Shapes aria-hidden="true" />
              {r.setKind}
            </Button>
            <Button variant="ghost" size="sm">
              <Check aria-hidden="true" />
              {r.confirm}
            </Button>
          </SelectionBar>
        )
      }
    >
      {loading ? (
        <LoadingState label={r.loading} layout="grid" size="record" count={6} />
      ) : visible.length === 0 ? (
        <EmptyState
          icon={<Tags aria-hidden="true" />}
          title={r.noMatches}
          action={
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setCriteria(allTags);
              }}
            >
              {r.showAll}
            </Button>
          }
        />
      ) : (
        <CardGrid size="record">
          {visible.map((tag) => (
            <SampleTagCard
              key={tag.id}
              tag={tag}
              selected={selected.has(tag.id)}
              selectionMode={selected.size > 0}
              onSelectedChange={(on) => {
                toggle(tag.id, on);
              }}
            />
          ))}
        </CardGrid>
      )}
    </ListPage>
  );
}
