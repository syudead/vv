import { ArrowLeft, CircleCheck, Play, Target } from "lucide-react";
import { useState } from "react";

import { t } from "@/i18n";
import { PageHeader } from "@/ui/patterns/page-header";
import { QueryResultsPage, ResultBand } from "@/ui/patterns/query-results-page";
import { ResultRow } from "@/ui/patterns/result-row";
import { RateRing } from "@/ui/RateRing";
import { Badge } from "@/ui/shadcn/badge";
import { Button } from "@/ui/shadcn/button";

// registry:block query-results-page-example. See web/registry/rules/patterns.md, Query results page.

interface SampleFrame {
  rate: number;
  time: string;
}

interface SampleRow {
  title: string;
  frames: SampleFrame[];
}

const frameTimes = ["1:12", "1:14", "0:48", "2:03", "2:05", "3:30", "4:01", "4:20"];

function frames(rates: number[]): SampleFrame[] {
  return rates.map((rate, index) => ({ rate, time: frameTimes[index] ?? "0:00" }));
}

function sampleBands() {
  const v = t.designSystem.pattern.videoTitles;
  const q = t.designSystem.pattern.queryResults;
  return [
    {
      id: "strong",
      heading: q.strong,
      range: q.strongRange,
      rows: [
        { title: v.harbour, frames: frames([96, 95, 93, 92, 91, 91, 90, 90]) },
        { title: v.lighthouse, frames: frames([91, 90, 90]) },
      ],
    },
    {
      id: "partial",
      heading: q.partial,
      range: q.partialRange,
      rows: [
        { title: v.train, frames: frames([74]) },
        { title: v.market, frames: frames([62, 61]) },
      ],
    },
  ];
}

function QueryPanel() {
  const q = t.designSystem.pattern.queryResults;
  return (
    <div className="flex flex-col gap-3 rounded-md border border-border bg-card p-3">
      <div className="aspect-video max-h-query-preview-max-sm w-full rounded-sm bg-linear-to-br from-primary-soft to-secondary lg:max-h-query-preview-max" />
      <div className="flex min-w-0 flex-col gap-1">
        <span className="truncate text-sm font-medium">{q.fileName}</span>
        <span className="text-xs text-muted-foreground">{q.fileMeta}</span>
      </div>
      <div className="flex min-w-0 items-center gap-2">
        <CircleCheck aria-hidden="true" className="size-4 shrink-0 text-success" />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{q.status}</span>
        <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
          {q.took}
        </span>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" size="sm">
          {q.another}
        </Button>
      </div>
    </div>
  );
}

function SampleResultRow({
  row,
  best,
  tone,
}: {
  row: SampleRow;
  best: boolean;
  tone: "primary" | "muted";
}) {
  const q = t.designSystem.pattern.queryResults;
  const [expanded, setExpanded] = useState(true);
  const top = row.frames[0] ?? { rate: 0, time: "0:00" };
  return (
    <ResultRow
      mark={<RateRing value={top.rate} tone={tone} />}
      title={row.title}
      badge={
        best ? (
          <Badge variant="soft">
            <Target aria-hidden="true" />
            {q.best}
          </Badge>
        ) : undefined
      }
      meta={<span className="truncate">{q.meta(row.frames.length, top.time)}</span>}
      actions={
        <Button variant={best ? "default" : "secondary"} size="sm">
          <Play aria-hidden="true" />
          {q.play}
        </Button>
      }
      expanded={expanded}
      onExpandedChange={setExpanded}
      toggleLabel={
        expanded
          ? { tooltip: q.hideFrames, name: q.hideFramesFor(row.title) }
          : { tooltip: q.showFrames, name: q.showFramesFor(row.title) }
      }
    >
      <ul className="flex gap-3 overflow-x-auto">
        {row.frames.map((frame) => (
          <li key={frame.time} className="flex w-related-thumb shrink-0 flex-col gap-1">
            <span
              aria-label={q.playFrom(frame.time)}
              role="img"
              className="aspect-video w-full rounded-sm bg-linear-to-br from-secondary to-muted"
            />
            <span className="flex items-center gap-2 text-xs">
              <span className="font-medium text-foreground tabular-nums">
                {frame.rate}%
              </span>
              <span className="ml-auto text-muted-foreground tabular-nums">
                {frame.time}
              </span>
            </span>
          </li>
        ))}
      </ul>
    </ResultRow>
  );
}

export function QueryResultsPageExample() {
  const q = t.designSystem.pattern.queryResults;
  const bands = sampleBands();
  return (
    <QueryResultsPage
      header={
        <PageHeader
          title={q.title}
          leading={
            <Button variant="ghost" size="sm">
              <ArrowLeft aria-hidden="true" />
              {q.back}
            </Button>
          }
        />
      }
      query={<QueryPanel />}
      queryLabel={q.query}
    >
      <p role="status" className="text-sm text-muted-foreground">
        {q.summary}
      </p>
      {bands.map((band, bandIndex) => (
        <ResultBand
          key={band.id}
          heading={band.heading}
          detail={band.range}
          count={q.count(band.rows.length)}
        >
          {band.rows.map((row, rowIndex) => (
            <SampleResultRow
              key={row.title}
              row={row}
              best={bandIndex === 0 && rowIndex === 0}
              tone={bandIndex === 0 ? "primary" : "muted"}
            />
          ))}
        </ResultBand>
      ))}
    </QueryResultsPage>
  );
}
