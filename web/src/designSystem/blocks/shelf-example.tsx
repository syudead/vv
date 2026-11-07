import { ChevronRight } from "lucide-react";

import { t } from "@/i18n";
import { CardGrid } from "@/ui/patterns/card-grid";
import { Shelf } from "@/ui/patterns/shelf";
import { Button } from "@/ui/shadcn/button";
import { Separator } from "@/ui/shadcn/separator";

// registry:block shelf-example. See web/registry/rules/patterns.md, Sections.

function SampleCard({ title }: { title: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <span className="aspect-video w-full rounded-md bg-card" />
      <span className="truncate text-sm font-medium">{title}</span>
    </div>
  );
}

/** A row shelf of a few highlighted items, its hairline, then a wrapping shelf of every item. */
export function ShelfExample() {
  const p = t.designSystem.pattern;
  const titles = Object.values(p.videoTitles);
  return (
    <div className="flex min-w-0 flex-col gap-3 p-4">
      <Shelf
        title={p.shelfRowTitle}
        count={p.shelfRowCount}
        layout="row"
        actions={
          <Button variant="ghost" size="sm">
            {p.shelfRowAll}
            <ChevronRight aria-hidden="true" />
          </Button>
        }
      >
        {titles.slice(0, 6).map((title) => (
          <SampleCard key={title} title={title} />
        ))}
      </Shelf>
      <Separator />
      <Shelf title={p.shelfGridTitle} count={p.shelfGridCount}>
        <CardGrid size={0}>
          {titles.map((title) => (
            <SampleCard key={title} title={title} />
          ))}
        </CardGrid>
      </Shelf>
    </div>
  );
}
