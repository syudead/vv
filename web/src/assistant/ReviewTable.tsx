import type {
  AssistantProposal,
  AssistantReviewCandidate,
  AssistantReviewRun,
} from "../api/assistant";
import { formatNumber, t } from "../i18n";
import { cn } from "@/lib/cn";
import { DataTable } from "../ui/patterns/data-table";
import { Badge } from "../ui/shadcn/badge";
import { Checkbox } from "../ui/shadcn/checkbox";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../ui/shadcn/table";
import { RunState } from "./ProposalCard";
import { candidateName, commonTitle, destinationIndex } from "./review";

/**
 * Two names with ↔ between them; the merge's destination, the more used side, is font-medium.
 * Each name keeps one line, so a narrow cell puts the second name under the first.
 */
function PairCell({ candidate }: { candidate: AssistantReviewCandidate }) {
  const destination = destinationIndex(candidate.members);
  return (
    <div className="flex flex-wrap items-baseline gap-x-1 text-sm">
      {candidate.members.map((member, index) => (
        <span
          key={`${member.kind}:${String(member.id)}`}
          className="flex max-w-full min-w-0 items-baseline gap-1"
        >
          {index > 0 && (
            <span aria-hidden="true" className="text-muted-foreground">
              ↔
            </span>
          )}
          <span className={cn("truncate", index === destination && "font-medium")}>
            {member.name}
          </span>
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {formatNumber(member.count)}
          </span>
        </span>
      ))}
    </div>
  );
}

/** The adopted mark and, once adopted here, the run's state line. */
function Outcome({
  candidate,
  proposal,
  withResult,
}: {
  candidate: AssistantReviewCandidate;
  proposal?: AssistantProposal;
  withResult: boolean;
}) {
  if (candidate.state !== "adopted") return null;
  return (
    <div className="flex flex-col items-start gap-1">
      <Badge variant="success">{t.assistant.review.adopted}</Badge>
      {withResult && proposal !== undefined && <RunState proposal={proposal} />}
    </div>
  );
}

function WorksCell({ candidate }: { candidate: AssistantReviewCandidate }) {
  return (
    <div className="flex min-w-0 flex-col">
      <span className="truncate text-sm">{commonTitle(candidate.members)}</span>
      <span className="truncate text-xs text-muted-foreground">
        {candidate.members.map((member) => member.name).join(" · ")}
      </span>
    </div>
  );
}

interface RowProps {
  run: AssistantReviewRun;
  candidate: AssistantReviewCandidate;
  proposal?: AssistantProposal;
  selected: boolean;
  onSelect: (id: number, selected: boolean) => void;
}

function CandidateRow({ run, candidate, proposal, selected, onSelect }: RowProps) {
  const words = t.assistant.review;
  const listed = candidate.state === "listed";
  const percent = t.activity.percent(candidate.probability);
  return (
    <TableRow data-state={selected ? "selected" : undefined}>
      <TableCell>
        <div className="flex size-8 items-center justify-center">
          <Checkbox
            checked={selected}
            disabled={!listed}
            onCheckedChange={(next) => onSelect(candidate.id, next === true)}
            aria-label={words.selectRow(candidateName(run, candidate))}
          />
        </div>
      </TableCell>
      {/* max-w-0 keeps long names from widening the table. */}
      <TableCell className="w-full max-w-0 whitespace-normal">
        <div className="flex flex-col gap-1">
          {run.kind === "works" ? (
            <WorksCell candidate={candidate} />
          ) : (
            <PairCell candidate={candidate} />
          )}
          {/* Below sm the state and the result fold under the names, so the names keep the width. */}
          <div className="sm:hidden">
            <Outcome candidate={candidate} proposal={proposal} withResult />
          </div>
        </div>
      </TableCell>
      <TableCell className="text-right text-sm tabular-nums">
        <span className="sr-only">{words.confidence(percent)}</span>
        <span aria-hidden="true">{percent}</span>
      </TableCell>
      {/* Below lg the result folds under the state in the same cell. */}
      <TableCell className="hidden whitespace-normal sm:table-cell lg:hidden">
        <Outcome candidate={candidate} proposal={proposal} withResult />
      </TableCell>
      <TableCell className="hidden whitespace-normal lg:table-cell">
        <Outcome candidate={candidate} proposal={proposal} withResult={false} />
      </TableCell>
      <TableCell className="hidden whitespace-normal lg:table-cell">
        {proposal !== undefined && <RunState proposal={proposal} />}
      </TableCell>
    </TableRow>
  );
}

export interface ReviewTableProps {
  run: AssistantReviewRun;
  rows: readonly AssistantReviewCandidate[];
  proposals: ReadonlyMap<number, AssistantProposal>;
  selected: ReadonlySet<number>;
  /** The listed rows the header checkbox selects. */
  selectable: readonly number[];
  onSelect: (id: number, selected: boolean) => void;
  onSelectAll: (selected: boolean) => void;
  stickyHeaderTop?: number;
}

export default function ReviewTable({
  run,
  rows,
  proposals,
  selected,
  selectable,
  onSelect,
  onSelectAll,
  stickyHeaderTop,
}: ReviewTableProps) {
  const words = t.assistant.review;
  const all = selectable.length > 0 && selectable.every((id) => selected.has(id));
  const some = !all && selectable.some((id) => selected.has(id));
  return (
    <DataTable label={words.table} stickyHeaderTop={stickyHeaderTop}>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead>
            <div className="flex size-8 items-center justify-center">
              <Checkbox
                checked={all ? true : some ? "indeterminate" : false}
                disabled={selectable.length === 0}
                onCheckedChange={() => onSelectAll(!all)}
                aria-label={all ? words.clearSelection : words.selectAll}
              />
            </div>
          </TableHead>
          <TableHead className="w-full">{words.columns.candidates}</TableHead>
          <TableHead className="text-right">{words.columns.confidence}</TableHead>
          <TableHead className="hidden sm:table-cell">
            <span className="sr-only">{words.columns.state}</span>
          </TableHead>
          <TableHead className="hidden lg:table-cell">
            <span className="sr-only">{words.columns.result}</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((candidate) => (
          <CandidateRow
            key={candidate.id}
            run={run}
            candidate={candidate}
            proposal={proposals.get(candidate.id)}
            selected={selected.has(candidate.id)}
            onSelect={onSelect}
          />
        ))}
      </TableBody>
    </DataTable>
  );
}
