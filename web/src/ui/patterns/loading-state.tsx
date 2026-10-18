import { Skeleton } from "@/ui/shadcn/skeleton";

import { CardGrid, type CardSize } from "./card-grid";

// Match final content geometry with skeletons instead of a page-covering spinner; see web/registry/rules/patterns.md, States.

export interface LoadingStateProps {
  label: string;
  layout: "grid" | "table" | "grouped" | "cards";
  /** Match the loaded CardGrid size. */
  size?: CardSize;
  count?: number;
}

export function LoadingState({ label, layout, size = 1, count }: LoadingStateProps) {
  const keys = Array.from(
    { length: count ?? (layout === "grouped" ? 3 : 8) },
    (_, index) => index,
  );
  return (
    <div data-slot="loading-state" role="status" aria-label={label} aria-busy="true">
      {layout === "grid" ? (
        <CardGrid size={size}>
          {keys.map((key) => (
            <div key={key} className="flex flex-col gap-2">
              <Skeleton className="aspect-video w-full" />
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-3 w-1/2" />
            </div>
          ))}
        </CardGrid>
      ) : layout === "cards" ? (
        <div className="flex flex-col gap-2">
          {keys.map((key) => (
            <div
              key={key}
              className="flex min-h-activity-card items-center gap-3 rounded-lg border border-border bg-muted px-4 py-2"
            >
              <Skeleton className="size-5 rounded-full" />
              <Skeleton className="h-4 w-1/3" />
              <Skeleton className="ml-auto h-4 w-1/4" />
            </div>
          ))}
        </div>
      ) : layout === "grouped" ? (
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-2 py-2">
            <Skeleton className="h-4 w-16" />
            <Skeleton className="h-3 w-12" />
          </div>
          <div className="flex flex-col">
            {keys.map((key) => (
              <div key={key} className="flex items-center gap-3 py-2">
                <Skeleton className="aspect-video w-history-thumb-sm shrink-0 lg:w-history-thumb" />
                <div className="flex min-w-0 flex-1 flex-col gap-2">
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="h-3 w-1/2" />
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="flex flex-col divide-y divide-border overflow-hidden rounded-md border border-border bg-card">
          {keys.map((key) => (
            <div key={key} className="flex h-10 items-center gap-3 px-2">
              <Skeleton className="size-4 rounded-sm" />
              <Skeleton className="h-4 w-1/3" />
              <Skeleton className="ml-auto h-4 w-12" />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
