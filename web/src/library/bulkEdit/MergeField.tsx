import { ArrowRight } from "lucide-react";
import { useState } from "react";

import { personFaceSrc } from "../../api/credits";
import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import { PersonFace } from "../../ui/PersonFace";
import { Button } from "../../ui/shadcn/button";
import { FieldDescription, FieldLegend, FieldSet } from "../../ui/shadcn/field";
import { Skeleton } from "../../ui/shadcn/skeleton";
import type { MergeCandidate, MergeTarget } from "./merges";

/** Tiles drawn before `Show all`; a candidate set to merge always stays. */
const tilesShown = 12;

/**
 * The selection's unnamed performers under the Performers field. Pressing a face steps through
 * keeping the person apart and merging them into each added performer.
 */
export default function MergeField({
  candidates,
  targets,
  merges,
  total,
  loading,
  disabled,
  onCycle,
}: {
  candidates: readonly MergeCandidate[];
  targets: readonly MergeTarget[];
  merges: ReadonlyMap<number, string>;
  total: number;
  /** True while the suggestions are read; the tiles wait so a choice is not overwritten. */
  loading: boolean;
  disabled: boolean;
  onCycle: (candidate: MergeCandidate) => void;
}) {
  const b = t.library.bulkEdit;
  const [all, setAll] = useState(false);
  const shown =
    all || candidates.length <= tilesShown
      ? candidates
      : candidates.filter(
          (candidate, index) => index < tilesShown || merges.has(candidate.id),
        );
  return (
    <FieldSet className="gap-2">
      <FieldLegend variant="label">{b.unnamedPerformers}</FieldLegend>
      <FieldDescription className="text-xs">{b.mergeHint}</FieldDescription>
      {loading ? (
        <div role="status" aria-label={b.loading} className="flex flex-wrap gap-2">
          {candidates.slice(0, 4).map((candidate) => (
            <Skeleton key={candidate.id} className="size-card-face rounded-full" />
          ))}
        </div>
      ) : (
        <ul aria-label={b.unnamedPerformers} className="flex flex-wrap gap-2">
          {shown.map((candidate) => {
            const target = targets.find((item) => item.key === merges.get(candidate.id));
            return (
              <li key={candidate.id}>
                <Button
                  variant="outline"
                  data-merge={target === undefined ? undefined : "merge"}
                  aria-pressed={target !== undefined}
                  aria-label={b.mergeTileLabel(
                    candidate.label,
                    candidate.count,
                    total,
                    target?.label,
                  )}
                  aria-disabled={disabled || undefined}
                  onClick={() => {
                    if (!disabled) onCycle(candidate);
                  }}
                  className={cn(
                    "h-auto flex-col gap-1.5 p-2 font-normal",
                    target !== undefined &&
                      "border-primary bg-primary-soft hover:bg-primary-soft",
                  )}
                >
                  <PersonFace
                    src={personFaceSrc(candidate)}
                    name={candidate.label}
                    shape="judge"
                  />
                  <span className="flex w-card-face items-center justify-between gap-1 text-xs">
                    <span className="text-muted-foreground">{candidate.label}</span>
                    <span className="text-muted-foreground tabular-nums">
                      {b.ratio(candidate.count, total)}
                    </span>
                  </span>
                  <span
                    className={cn(
                      "flex w-card-face min-w-0 items-center justify-center gap-1 text-xs",
                      target === undefined ? "text-muted-foreground" : "text-primary",
                    )}
                  >
                    {target === undefined ? (
                      b.keepApart
                    ) : (
                      <>
                        <ArrowRight className="size-3 shrink-0" aria-hidden="true" />
                        <span className="min-w-0 truncate">{target.label}</span>
                      </>
                    )}
                  </span>
                </Button>
              </li>
            );
          })}
          {(candidates.length > shown.length || all) && (
            <li className="self-center">
              <Button
                variant="ghost"
                size="sm"
                aria-expanded={all}
                onClick={() => setAll((current) => !current)}
              >
                {all ? b.showFewer : b.showAll(candidates.length)}
              </Button>
            </li>
          )}
        </ul>
      )}
    </FieldSet>
  );
}
