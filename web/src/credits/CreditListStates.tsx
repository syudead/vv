import { Building2, Plus, SearchX, Users } from "lucide-react";
import type { Ref } from "react";

import type { CreditKind } from "../api/credits";
import { t, type UiText } from "../i18n";
import { TagsEmpty } from "../tags/TagListEmptyState";
import {
  TagListChanged,
  TagLoadFailed,
  TagLoadingMore,
  TagLoadMoreFailed,
  TagStaleList,
} from "../tags/TagListNotices";
import type { TagListEmpty } from "../tags/tagListView";
import type { MoreState } from "../tags/useTagPage";
import { LoadingState } from "../ui/patterns/loading-state";
import { Button } from "../ui/shadcn/button";
import { TableHead, TableHeader, TableRow } from "../ui/shadcn/table";
import { kindTexts } from "./creditRows";

export function CreditListStates({
  kind,
  loaded,
  loadError,
  firstPending,
  staleList,
  empty,
  appliedSearch,
  onRetry,
  onCreate,
  onClearSearch,
}: {
  kind: CreditKind;
  loaded: boolean;
  loadError: UiText | null;
  firstPending: boolean;
  staleList: boolean;
  empty: TagListEmpty | null;
  appliedSearch: string;
  onRetry: () => void;
  onCreate: () => void;
  onClearSearch: () => void;
}) {
  const texts = kindTexts(kind);
  if (staleList && loadError !== null) {
    return (
      <TagStaleList
        reason={loadError}
        pending={firstPending}
        onRetry={onRetry}
        message={texts.staleList}
      />
    );
  }
  if (!loaded) {
    return loadError === null ? (
      <LoadingState label={t.credits.loading} layout="table" count={6} />
    ) : (
      <TagLoadFailed
        pending={firstPending}
        onRetry={onRetry}
        title={texts.loadFailed(loadError)}
      />
    );
  }
  if (empty === "tags") {
    const Icon = kind === "person" ? Users : Building2;
    return (
      <TagsEmpty
        icon={<Icon aria-hidden="true" />}
        title={texts.emptyTitle}
        description={t.credits.empty.description}
        action={
          <Button size="sm" onClick={onCreate}>
            <Plus aria-hidden="true" />
            {texts.newButton}
          </Button>
        }
      />
    );
  }
  if (empty !== null) {
    return (
      <TagsEmpty
        icon={<SearchX aria-hidden="true" />}
        title={texts.noMatches(appliedSearch)}
        action={
          <Button variant="outline" size="sm" onClick={onClearSearch}>
            {t.credits.clearSearch}
          </Button>
        }
      />
    );
  }
  return null;
}

export function CreditListTail({
  kind,
  tail,
  onRetryMore,
  onReload,
}: {
  kind: CreditKind;
  tail: MoreState;
  onRetryMore: () => void;
  onReload: () => void;
}) {
  const texts = kindTexts(kind);
  switch (tail.kind) {
    case "loading":
      return <TagLoadingMore label={texts.loadingMore} />;
    case "failed":
      return <TagLoadMoreFailed reason={tail.error} onRetry={onRetryMore} />;
    case "inconsistent":
      return <TagListChanged onReload={onReload} message={texts.listChanged} />;
    case "idle":
      return null;
  }
}

export function CreditListHead({ headRef }: { headRef: Ref<HTMLTableSectionElement> }) {
  return (
    <TableHeader ref={headRef}>
      <TableRow className="hover:bg-transparent">
        <TableHead className="w-full">{t.credits.columns.name}</TableHead>
        <TableHead className="text-right">{t.credits.columns.videos}</TableHead>
        <TableHead aria-hidden="true" />
      </TableRow>
    </TableHeader>
  );
}
