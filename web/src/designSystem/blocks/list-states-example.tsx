import { FolderPlus, Film } from "lucide-react";

import { t } from "@/i18n";
import { CardGrid } from "@/ui/patterns/card-grid";
import { EmptyState } from "@/ui/patterns/empty-state";
import { ErrorState } from "@/ui/patterns/error-state";
import { ListPage } from "@/ui/patterns/list-page";
import { LoadingState } from "@/ui/patterns/loading-state";
import { LoadMoreRow } from "@/ui/patterns/load-more-row";
import { Button } from "@/ui/shadcn/button";

import {
  ExampleHeader,
  ExampleToolbar,
  ExampleTopBar,
  SampleVideoCard,
  sampleVideos,
} from "./list-page-example";

// 一覧ページの状態の見本（registry:block list-states-example）。見出し行とツールバーは
// 動かさず、本体の位置だけが読み込み中・空・エラー・追加読み込み・続きの失敗に変わる
// （web/registry/rules/patterns.md の States）。

export type ListState = "loading" | "empty" | "error" | "loadingMore" | "loadMoreFailed";

export function ListStatesExample({ state }: { state: ListState }) {
  return (
    <div className="flex flex-col">
      <ExampleTopBar>
        <ExampleToolbar />
      </ExampleTopBar>
      <ListPage header={<ExampleHeader />}>
        <Body state={state} />
      </ListPage>
    </div>
  );
}

function Body({ state }: { state: ListState }) {
  const p = t.designSystem.pattern;
  const retry = () => undefined;
  switch (state) {
    case "loading":
      return <LoadingState label={p.loading} layout="grid" count={4} />;
    case "empty":
      return (
        <EmptyState
          icon={<Film aria-hidden="true" />}
          title={p.emptyTitle}
          description={p.emptyDescription}
          action={
            <Button size="sm">
              <FolderPlus aria-hidden="true" />
              {p.addFolder}
            </Button>
          }
        />
      );
    case "error":
      return (
        <ErrorState
          title={p.errorTitle}
          description={p.errorDescription}
          retryLabel={p.retry}
          onRetry={retry}
        />
      );
    case "loadingMore":
    case "loadMoreFailed":
      return (
        <>
          <CardGrid>
            {sampleVideos()
              .slice(0, 4)
              .map((video) => (
                <SampleVideoCard
                  key={video.id}
                  title={video.title}
                  meta={video.meta}
                  duration={video.duration}
                  selected={false}
                  onSelectedChange={retry}
                />
              ))}
          </CardGrid>
          {state === "loadingMore" ? (
            <LoadMoreRow status="loading" label={p.loadingMore} />
          ) : (
            <LoadMoreRow
              status="failed"
              title={p.loadMoreFailedTitle}
              description={p.loadMoreFailedDescription}
              retryLabel={p.retry}
              onRetry={retry}
            />
          )}
        </>
      );
  }
}
