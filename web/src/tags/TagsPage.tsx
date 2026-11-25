import { Plus, Shapes } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";

import {
  clearTagListSnapshot,
  saveTagListSnapshot,
  type TagListSnapshot,
  takeTagListSnapshot,
} from "../api/tagListSnapshot";
import type { Tag, TagListSort } from "../api/tags";
import { subscribeTagKindRefsChanged } from "../api/tagKinds";
import { t } from "../i18n";
import {
  readTagListPreferences,
  writeTagListPreferences,
} from "../preferences/tagListPreferences";
import { AdminListToolbar } from "../recordList/AdminListToolbar";
import { tagListView } from "../recordList/listView";
import { TagStaleList } from "../recordList/TagListNotices";
import { useSearchQuery } from "../recordList/usePagedRows";
import TopBarPortal from "../shell/TopBarPortal";
import { FloatingNotice } from "../ui/FloatingNotice";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";
import type { HistoryMode } from "../videoList/listCriteria";
import NewTagDialog from "./NewTagDialog";
import { tagPagePath } from "./TagCard";
import { TagCards } from "./TagCardGrid";
import TagListEmptyState from "./TagListEmptyState";
import { TagKindFilterGroup, TagTallyStrip } from "./TagListFilters";
import type { TagPageRows } from "./tagPageRows";
import {
  serializeTagListCriteria,
  type TagListCriteria,
  useTagListCriteria,
} from "./tagListUrl";
import { useTagKinds } from "./useTagKinds";
import { kindNameOf, useTagKindFilter } from "./useTagKindFilter";
import { useTagListChanges } from "./useTagListChanges";
import { useTagPage } from "./useTagPage";
import { useTagTallies } from "./useTagTallies";

const allTags = {
  query: "",
  tentativeOnly: false,
  unusedOnly: false,
  kind: null,
} as const;

function headerActions(onCreate: () => void, createDisabled: boolean) {
  return (
    <>
      <Button asChild variant="ghost" size="sm">
        <Link to="/tags/kinds">
          <Shapes aria-hidden="true" />
          {t.tags.kinds.title}
        </Link>
      </Button>
      <Button asChild variant="ghost" size="sm">
        <Link to="/tags/rejected">{t.tags.rejectedNames.heading}</Link>
      </Button>
      <Button size="sm" onClick={onCreate} disabled={createDisabled}>
        <Plus aria-hidden="true" />
        {t.tags.newTag}
      </Button>
    </>
  );
}

/**
 * The tag list: kind-filtered tag cards read a page of 100 at a time for the URL's criteria. A card
 * keeps the loaded list for Back from its tag page, and a `tags` event shows a notice instead of
 * moving the cards.
 */
export default function TagsPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const [preferredSort] = useState<TagListSort>(() => readTagListPreferences().sort);
  const { criteria, apply: applyCriteria } = useTagListCriteria(preferredSort);
  const { query: search, tentativeOnly, unusedOnly, kind, sort } = criteria;
  const kindList = useTagKinds();
  const { tallies, reload: reloadTallies } = useTagTallies();
  const [creating, setCreating] = useState(false);
  const scrollToRef = useRef<number | null>(null);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const restoredRef = useRef<{ key: string; kept: TagListSnapshot<TagPageRows> } | null>(
    null,
  );
  const { reload: reloadKinds } = kindList;

  const resetRef = useRef<() => void>(() => undefined);
  const onFirstPage = useCallback(() => {
    // A page read from the server supersedes the list kept for Back.
    clearTagListSnapshot();
    resetRef.current();
    reloadTallies();
    reloadKinds();
    if (window.scrollY > 0) window.scrollTo({ top: 0 });
  }, [reloadTallies, reloadKinds]);
  const paged = useTagPage(onFirstPage);
  const { page, loadError, firstPending, more, appliedSearch, pageRef } = paged;
  const { load, reload, loadMore, retryMore, restore } = paged;

  const searchRef = useRef(appliedSearch);
  useLayoutEffect(() => {
    searchRef.current = appliedSearch;
  });
  const changes = useTagListChanges({ pageRef, searchRef, firstPending });
  const { check, reset } = changes;
  useLayoutEffect(() => {
    resetRef.current = reset;
  });

  const { normalizedQuery, searchRef: typedRef } = useSearchQuery(search);
  // Keyed by the folded query, as reloads are, so a snapshot survives a change of letter case.
  const criteriaKey = serializeTagListCriteria({
    ...criteria,
    query: normalizedQuery,
  }).toString();
  useEffect(() => {
    const restored = restoredRef.current;
    if (restored?.key === criteriaKey) {
      // StrictMode replays this effect after its cleanup aborted the reads; the list is already restored.
      reloadTallies();
      check(restored.kept.page, restored.kept.search);
      return;
    }
    const kept = takeTagListSnapshot<TagPageRows>(criteriaKey);
    if (kept !== undefined) {
      restoredRef.current = { key: criteriaKey, kept };
      restore(kept.page, kept.search);
      scrollToRef.current = kept.scrollY;
      reloadTallies();
      check(kept.page, kept.search);
      return;
    }
    restoredRef.current = null;
    clearTagListSnapshot();
    void load({
      query: normalizedQuery,
      search: typedRef.current,
      tentativeOnly,
      unusedOnly,
      kind,
      sort,
    });
  }, [
    criteriaKey,
    normalizedQuery,
    tentativeOnly,
    unusedOnly,
    kind,
    sort,
    load,
    restore,
    reloadTallies,
    check,
    typedRef,
  ]);

  // Cards embed kind refs; re-read them when a kind edit lands after this list loaded, as after Back mid-delete.
  useEffect(() => subscribeTagKindRefsChanged(() => void reload()), [reload]);

  // Return to where the card was opened once the restored cards are drawn.
  useLayoutEffect(() => {
    if (scrollToRef.current === null || page === undefined) return;
    window.scrollTo({ top: scrollToRef.current });
    scrollToRef.current = null;
  }, [page]);

  const { kindName } = useTagKindFilter({
    kind,
    kinds: kindList.kinds,
    fresh: kindList.fresh,
    onGone: () => {
      applyCriteria({ kind: null }, "replace");
      toast(t.tags.kindFilter.gone);
    },
  });

  const openCard = useCallback(() => {
    const shown = pageRef.current;
    if (shown === undefined) return;
    saveTagListSnapshot({
      key: criteriaKey,
      page: shown,
      search: searchRef.current,
      scrollY: window.scrollY,
    });
  }, [pageRef, criteriaKey]);

  function applyFilter(change: Partial<TagListCriteria>) {
    applyCriteria(change, "push");
  }

  function changeSort(next: TagListSort) {
    applyCriteria({ sort: next }, "push");
    writeTagListPreferences({ sort: next });
  }

  function commitQuery(next: string, mode: HistoryMode) {
    applyCriteria({ query: next }, mode);
  }

  function refresh() {
    reset();
    void reload();
  }

  function created(tag: Tag) {
    setCreating(false);
    void navigate(tagPagePath(tag.id));
  }

  const view = tagListView({
    page,
    loadError,
    more,
    visibleCount: page?.rows.length ?? 0,
    creating: false,
  });
  const appliedKind = page?.query.kind ?? null;

  return (
    <ListPage
      toolbar={
        <TopBarPortal>
          <AdminListToolbar
            query={search}
            onQueryCommit={commitQuery}
            searchRef={searchInputRef}
            searchDisabled={page !== undefined && page.totalAll === 0}
            searchLabel={t.tags.search.label}
            searchPlaceholder={t.tags.search.placeholder}
            sort={sort}
            onSortChange={changeSort}
            sortDisabled={view.noTags}
            compactSortName="tag-compact-sort"
          />
        </TopBarPortal>
      }
      header={
        <PageHeader
          title={t.tags.title}
          actions={headerActions(() => setCreating(true), page === undefined)}
        />
      }
      filter={
        <TagKindFilterGroup
          kind={kind}
          kinds={kindList.kinds}
          totalAll={page?.totalAll}
          noKind={tallies.noKind}
          onKindChange={(next) => applyFilter({ kind: next })}
        />
      }
      summary={
        <TagTallyStrip
          tallies={tallies}
          criteria={{ tentativeOnly, unusedOnly, kind }}
          onApply={applyFilter}
        />
      }
      notice={
        changes.changed && (
          <FloatingNotice
            message={t.tags.list.changed}
            actionLabel={t.tags.list.refresh}
            onAction={refresh}
          />
        )
      }
    >
      {view.staleList && loadError !== null && (
        <TagStaleList
          reason={loadError}
          pending={firstPending}
          onRetry={() => void reload()}
        />
      )}
      {page === undefined && loadError === null && (
        <LoadingState
          label={t.tags.list.loading}
          layout="grid"
          size="record"
          count={12}
        />
      )}
      {page === undefined && loadError !== null && (
        <ErrorState
          title={t.tags.loadFailed}
          retryLabel={t.common.retry}
          onRetry={() => void reload()}
        />
      )}
      {view.empty !== null && (
        <TagListEmptyState
          empty={view.empty}
          appliedSearch={appliedSearch}
          appliedTentative={page?.query.tentativeOnly ?? false}
          appliedKind={appliedKind}
          kindName={kindNameOf(kindList.kinds, appliedKind) ?? kindName}
          onCreate={() => setCreating(true)}
          onShowAll={() => applyFilter(allTags)}
        />
      )}
      {view.showRows && page !== undefined && (
        <TagCards
          tags={page.rows}
          tail={view.tail}
          onOpen={openCard}
          onNearEnd={loadMore}
          onRetryMore={retryMore}
          onReload={refresh}
        />
      )}
      {creating && (
        <NewTagDialog
          track={changes.track}
          onClose={() => setCreating(false)}
          onCreated={created}
        />
      )}
    </ListPage>
  );
}
